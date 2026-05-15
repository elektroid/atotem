# 🐺 Totem

A shamanic quiz app that reveals your animal totem. Answer 13 questions, meet your spirit animal, and optionally generate a personalized mystical portrait — all in French.

## What it does

- **Quiz** — 13 multiple-choice questions scored across 5 psychological axes (element, introversion, intuition, thinking, structure). Tiebreaker questions resolve close calls between specific animal pairs.
- **15 animals** — each with Jungian archetype, traits, shadow animal, life theme, and a full relationship profile.
- **Image generation** — optional Mistral AI integration creates a personalized totem portrait with your name woven into it.
- **Shareable result** — every result gets a permanent URL you can send to friends.

## Stack

- **Backend**: Go + [Chi](https://github.com/go-chi/chi) + SQLite (via [modernc/sqlite](https://gitlab.com/cznic/sqlite), zero CGO)
- **Frontend**: Vanilla JS, Tailwind CDN, Cinzel font — no build step
- **Images**: Mistral AI agents API (optional, works without it)
- **Deploy**: single binary, frontend embedded via `go:embed`

## Run locally

```bash
cp .env.example .env
# add your MISTRAL_API_KEY if you want image generation
go run .
# open http://localhost:8080
```

## Configuration

| Variable | Default | Description |
|---|---|---|
| `PORT` | `8080` | HTTP port |
| `HOST` | *(all interfaces)* | Bind address, e.g. `127.0.0.1` |
| `DB_PATH` | `data/totem.db` | SQLite database path |
| `IMAGES_DIR` | `data/images` | Generated images directory |
| `MISTRAL_API_KEY` | — | Required for image generation |
| `MISTRAL_AGENT_ID` | — | Reuse an existing agent across restarts |

## Build

```bash
go build -o totem .
./totem
```

## Content

All animal profiles and questions are in French. The quiz uses a 5-dimensional Euclidean distance model — see [`questions.json`](questions.json) (`meta.algorithm`) for the full scoring spec.
