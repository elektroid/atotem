package main

import (
	"crypto/rand"
	"database/sql"
	"embed"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	_ "modernc.org/sqlite"
)

//go:embed frontend
var frontendFS embed.FS

//go:embed animals.json
var animalsJSON []byte

//go:embed questions.json
var questionsJSON []byte

var db *sql.DB

// ── Models ────────────────────────────────────────────────────────────────────

type RevealRequest struct {
	AnimalID string          `json:"animal_id"`
	Name     string          `json:"name"`
	Answers  json.RawMessage `json:"answers"`
}

type RevealResponse struct {
	UUID     string `json:"uuid"`
	ImageURL string `json:"image_url"`
}

type ResultRow struct {
	UUID      string    `json:"uuid"`
	AnimalID  string    `json:"animal_id"`
	Name      string    `json:"name"`
	ImageURL  string    `json:"image_url"`
	Answers   string    `json:"answers"`
	CreatedAt time.Time `json:"created_at"`
}

// ── Database ──────────────────────────────────────────────────────────────────

func initDB() error {
	path := os.Getenv("DB_PATH")
	if path == "" {
		path = "data/totem.db"
	}
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	var err error
	db, err = sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS results (
		uuid         TEXT PRIMARY KEY,
		animal_id    TEXT    NOT NULL,
		name         TEXT    NOT NULL DEFAULT '',
		image_url    TEXT    NOT NULL DEFAULT '',
		answers_json TEXT    NOT NULL DEFAULT '{}',
		created_at   DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	return err
}

func newID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// ── Handlers ──────────────────────────────────────────────────────────────────

func handleReveal(w http.ResponseWriter, r *http.Request) {
	var req RevealRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if req.AnimalID == "" {
		http.Error(w, "animal_id required", http.StatusBadRequest)
		return
	}

	id := newID()
	answersJSON := "{}"
	if len(req.Answers) > 0 {
		answersJSON = string(req.Answers)
	}

	imageURL := generateImage(req.AnimalID, req.Name, id)

	_, err := db.Exec(
		`INSERT INTO results (uuid, animal_id, name, image_url, answers_json) VALUES (?, ?, ?, ?, ?)`,
		id, req.AnimalID, req.Name, imageURL, answersJSON,
	)
	if err != nil {
		log.Printf("db insert error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(RevealResponse{UUID: id, ImageURL: imageURL})
}

func handleGetResult(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "uuid")
	var row ResultRow
	err := db.QueryRow(
		`SELECT uuid, animal_id, name, image_url, answers_json, created_at FROM results WHERE uuid = ?`,
		id,
	).Scan(&row.UUID, &row.AnimalID, &row.Name, &row.ImageURL, &row.Answers, &row.CreatedAt)
	if err == sql.ErrNoRows {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Printf("db query error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(row)
}

// ── Main ──────────────────────────────────────────────────────────────────────

func main() {
	loadDotEnv(".env")

	if err := initDB(); err != nil {
		log.Fatalf("db init: %v", err)
	}
	defer db.Close()

	initAnimalIndex()

	imgDir := os.Getenv("IMAGES_DIR")
	if imgDir == "" {
		imgDir = "data/images"
	}
	os.MkdirAll(imgDir, 0o755)

	sub, err := fs.Sub(frontendFS, "frontend")
	if err != nil {
		log.Fatalf("embed sub: %v", err)
	}

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Data endpoints (static JSON embedded at build time)
	r.Get("/data/animals.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(animalsJSON)
	})
	r.Get("/data/questions.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(questionsJSON)
	})

	// Generated images (written to disk at runtime, not embedded)
	r.Handle("/images/*", http.StripPrefix("/images/", http.FileServer(http.Dir(imgDir))))

	// API
	r.Post("/api/reveal", handleReveal)
	r.Get("/api/result/{uuid}", handleGetResult)

	// Shared result page: serve the SPA shell, JS loads the result via /api/result/:uuid
	r.Get("/result/{uuid}", func(w http.ResponseWriter, r *http.Request) {
		content, err := frontendFS.ReadFile("frontend/index.html")
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(content)
	})

	// All other paths: serve embedded frontend files
	r.Handle("/*", http.FileServer(http.FS(sub)))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	host := os.Getenv("HOST")
	addr := host + ":" + port
	log.Printf("Listening on http://%s", addr)
	log.Fatal(http.ListenAndServe(addr, r))
}
