package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const mistralBase = "https://api.mistral.ai/v1"

var (
	cachedAgentID string
	agentMu       sync.Mutex
	animalIndex   map[string]animalMeta
)

type animalMeta struct {
	Name    string `json:"name"`
	Element string `json:"element"`
}

// initAnimalIndex builds a lookup map from the embedded animals.json so
// generateImage can build rich prompts without re-parsing on every call.
func initAnimalIndex() {
	var db struct {
		Animals []struct {
			animalMeta
			ID string `json:"id"`
		} `json:"animals"`
	}
	if err := json.Unmarshal(animalsJSON, &db); err != nil {
		log.Printf("warn: could not parse animals.json for image index: %v", err)
		return
	}
	animalIndex = make(map[string]animalMeta, len(db.Animals))
	for _, a := range db.Animals {
		animalIndex[a.ID] = a.animalMeta
	}
}

// ensureAgent returns a Mistral agent ID suitable for image generation.
// It prefers the MISTRAL_AGENT_ID env var so you can reuse a pre-created
// agent and avoid creating one on every cold start.
func ensureAgent(ctx context.Context) (string, error) {
	agentMu.Lock()
	defer agentMu.Unlock()

	if id := os.Getenv("MISTRAL_AGENT_ID"); id != "" {
		return id, nil
	}
	if cachedAgentID != "" {
		return cachedAgentID, nil
	}

	body, _ := json.Marshal(map[string]any{
		"model":       "mistral-medium-latest",
		"name":        "Totem Image Generator",
		"description": "Generates mystical shamanic totem images.",
		"instructions": "Use the image generation tool to create every image the user requests. " +
			"Always produce a single high-quality illustration.",
		"tools": []map[string]string{{"type": "image_generation"}},
		"completion_args": map[string]any{
			"temperature": 0.7,
			"top_p":       0.95,
		},
	})

	data, err := mistralDo(ctx, "POST", "/agents", body)
	if err != nil {
		return "", fmt.Errorf("create agent: %w", err)
	}

	var resp struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(data, &resp); err != nil || resp.ID == "" {
		return "", fmt.Errorf("parse agent response (body: %s): %w", data, err)
	}

	cachedAgentID = resp.ID
	log.Printf("mistral: agent created %s", cachedAgentID)
	return cachedAgentID, nil
}

// generateImage calls the Mistral conversations API to produce a totem image,
// saves it to disk, and returns the local URL path.  Returns "" on any error
// so the caller can continue without an image.
func generateImage(animalID, name, uuid string) string {
	if os.Getenv("MISTRAL_API_KEY") == "" {
		return ""
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	agentID, err := ensureAgent(ctx)
	if err != nil {
		log.Printf("generateImage: ensureAgent: %v", err)
		return ""
	}

	meta := animalIndex[animalID]
	prompt := buildImagePrompt(meta.Name, meta.Element, name)

	convBody, _ := json.Marshal(map[string]any{
		"agent_id": agentID,
		"inputs":   prompt,
	})
	respBytes, err := mistralDo(ctx, "POST", "/conversations", convBody)
	if err != nil {
		log.Printf("generateImage: conversation: %v", err)
		return ""
	}

	fileID := extractFileID(respBytes)
	if fileID == "" {
		log.Printf("generateImage: no tool_file chunk in response: %.200s", respBytes)
		return ""
	}

	imgBytes, err := mistralDownloadFile(ctx, fileID)
	if err != nil {
		log.Printf("generateImage: download file %s: %v", fileID, err)
		return ""
	}

	imgDir := os.Getenv("IMAGES_DIR")
	if imgDir == "" {
		imgDir = "data/images"
	}
	if err := os.MkdirAll(imgDir, 0o755); err != nil {
		log.Printf("generateImage: mkdir %s: %v", imgDir, err)
		return ""
	}

	dest := filepath.Join(imgDir, uuid+".png")
	if err := os.WriteFile(dest, imgBytes, 0o644); err != nil {
		log.Printf("generateImage: write %s: %v", dest, err)
		return ""
	}

	return "/images/" + uuid + ".png"
}

// extractFileID searches all outputs for a tool_file chunk and returns
// its file_id.  The structure mirrors what the Mistral Python SDK exposes
// as response.outputs[-1].content[i] (ToolFileChunk).
func extractFileID(respBytes []byte) string {
	var envelope struct {
		Outputs []struct {
			Content []json.RawMessage `json:"content"`
		} `json:"outputs"`
	}
	if err := json.Unmarshal(respBytes, &envelope); err != nil {
		return ""
	}
	for _, out := range envelope.Outputs {
		for _, raw := range out.Content {
			var chunk struct {
				Type   string `json:"type"`
				FileID string `json:"file_id"`
			}
			if json.Unmarshal(raw, &chunk) == nil && chunk.Type == "tool_file" && chunk.FileID != "" {
				return chunk.FileID
			}
		}
	}
	return ""
}

// ── HTTP helpers ──────────────────────────────────────────────────────────────

func mistralDownloadFile(ctx context.Context, fileID string) ([]byte, error) {
	// File content endpoint returns raw binary — no JSON Accept header.
	return mistralDo(ctx, "GET", "/files/"+fileID+"/content", nil)
}

// mistralDo performs a single authenticated Mistral API call.
// Pass a non-nil body to send a JSON POST/PUT; nil body for GET.
func mistralDo(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, mistralBase+path, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+os.Getenv("MISTRAL_API_KEY"))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, data)
	}
	return data, nil
}

// ── Prompt builder ────────────────────────────────────────────────────────────

var elementHints = map[string]string{
	"terre":     "earthy tones, deep forest greens, rich browns, ancient stone and moss",
	"air":       "ethereal pale blues and silvers, wind and clouds, feathers, vast open sky",
	"feu":       "deep oranges and crimsons, glowing embers, golden light, dancing flames",
	"eau":       "deep ocean blues and teals, water reflections, mist, flowing currents",
	"terre/feu": "warm earth tones with fiery accents, volcanic rock, glowing lava veins",
	"eau/terre": "deep sea greens and blues, coral and stone, underwater light",
	"air/terre": "misty mountain blues and stone greys, ancient peaks, wind-carved rock",
}

func buildImagePrompt(animalName, element, name string) string {
	hint := elementHints[element]
	if hint == "" {
		hint = "rich mystical colors, sacred natural elements"
	}

	if name == "" {
		return fmt.Sprintf(
			"Mystical shamanic illustration of a %s spirit totem. "+
				"Sacred indigenous-inspired art, dramatic spiritual atmosphere, %s. "+
				"The animal is majestic and otherworldly, radiating spiritual power. "+
				"Intricate details, high quality digital painting, sacred geometry accents.",
			animalName, hint,
		)
	}

	return fmt.Sprintf(
		"Mystical shamanic illustration of a %s spirit totem. "+
			"Sacred indigenous-inspired art, dramatic spiritual atmosphere, %s. "+
			"The animal is majestic and otherworldly, radiating spiritual power. "+
			"The name \"%s\" is prominently displayed in the image as large, clearly legible glowing rune letters — "+
			"this is mandatory and must be readable. "+
			"The letters are integrated into the composition: etched in glowing stone or formed by roots and branches, "+
			"but always large enough to read clearly. "+
			"Intricate details, high quality digital painting, sacred geometry accents.",
		animalName, hint, name,
	)
}
