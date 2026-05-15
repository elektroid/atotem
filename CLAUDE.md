# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**Totem** is an website/app that acts as a shamanic guide, helping users discover their animal totem through a series of multiple choice questions. It determines the totem animal of the user and present the profile to the user. It has no build system — it is a data/config layer intended to be consumed by a frontend and an LLM backend for advanced features.

## Repository Structure

```
animals.json           — Animal database (15 animals with psychological profiles)
questions.json         — Offline quiz: 13 scored questions + 8 tiebreaker pairs
llm_config/
  system_prompt.md     — System prompt defining the guide's persona and conversation flow
```

## Architecture

The app has two moving parts:

**`animals.json`** is the knowledge base. Each animal entry contains:
- `id`, `name`, `emoji` — identity
- `archetype_jung` — Jungian archetype
- `keywords`, `element`, `traits` (strengths, shadows, life_theme)
- `symbolism` — cross-cultural (Amerindian, Celtic, Greek, etc.)
- `medicine` — the animal's core teaching
- `shadow_animal` — the paired opposite animal (referenced by `id`)
- `lover_profile` — relational style (style, strengths, watch_out, in_bed, dealbreaker)
- `personality_profile` *(partial)* — MBTI-like float scores (0–1) for introvert/extrovert, intuition/sensing, thinking/feeling, structure/freedom
- `response_patterns` *(partial)* — obstacle, environment, value, fear arrays

**`llm_config/system_prompt.md`** defines a 3-phase conversation:
1. **Ancrage** (2–3 exchanges) — open projective questions to identify dominant element and spatial relationship
2. **Approfondissement** (3–5 exchanges) — narrow to 2–3 candidate archetypes, probe differentiating axes
3. **Révélation** (1 final response) — structured output the frontend must parse:

```
TON_ANIMAL_TOTEM: [animal_id]
TON_ANIMAL_OMBRE: [animal_id]
RESUME_PERSONNEL: [2–3 sentences connecting user's answers to the animal]
```

The system prompt also includes a signal table mapping response patterns to probable animals.

## Offline Quiz (`questions.json`)

**`questions.json`** is the data layer for a multiple-choice quiz that determines a user's totem without an LLM.

**Scoring model:** Dimensional Euclidean distance across 5 axes:
- `element` — categorical (terre / air / feu / eau), encoded as a 4-component unit vector, weighted 2× relative to the personality axes
- `introvert_extrovert` — 0 = introvert, 1 = extrovert
- `intuition_sensing` — 0 = sensing, 1 = intuition *(note: high value = intuitive)*
- `thinking_feeling` — 0 = thinking, 1 = feeling
- `structure_freedom` — 0 = freedom, 1 = structure *(note: low value = freedom-oriented)*

**Algorithm:** After 13 core questions (grouped: element → introvert → intuition → thinking → structure), compute `total_distance = sqrt(element_weight² × ||element_diff||² + Σ personality_dim_diff²)` for each animal. If the gap between the top-two animals is < `tiebreaker_threshold` (0.15), present the pair-specific tiebreaker question. Return `{ totem: id, shadow: shadow_animal }`.

**Tiebreaker pairs authored:** owl/spider, owl/jaguar, jaguar/serpent, wolf/serpent, eagle/raven, fox/horse, bear/starfish, wolf/owl.

**Language:** French only.

## Known Data Issues

- Several animals have **duplicate `lover_profile` keys** (deer, raven, salmon, jaguar, platypus, starfish, spider). JSON parsers silently use the last value; the first entry is lost. These should be merged or the keys deduplicated.
- `personality_profile` now exists on all 15 animals. The 12 profiles added to wolf–jaguar were inferred from each animal's `traits`, `keywords`, and `archetype_jung` — treat them as a defensible first draft, not authoritative values. Axis conventions were reverse-engineered from the 3 original profiles (platypus, starfish, spider).
- `response_patterns` fields exist only on **platypus, starfish, and spider** — they are absent from the other 12 animals.
- `meta.total_animals` is set to `15`, which is correct.

## Content Language

All animal content and the system prompt are in **French**. The system prompt instructs the LLM to always reply in the user's language, but the structured revelation format tags (`TON_ANIMAL_TOTEM`, etc.) are fixed French strings that the frontend parser depends on.

## Agent skills

### Issue tracker

Issues are tracked as local markdown files under `.scratch/`. See `docs/agents/issue-tracker.md`.

### Triage labels

Uses the default five-role vocabulary (needs-triage, needs-info, ready-for-agent, ready-for-human, wontfix). See `docs/agents/triage-labels.md`.

### Domain docs

Single-context layout — one `CONTEXT.md` and `docs/adr/` at the repo root. See `docs/agents/domain.md`.
