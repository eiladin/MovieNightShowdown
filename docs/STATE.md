# Movie Night Showdown — State Ledger

This is the single source of truth for progress. Read it first. Update it last.
See `docs/HANDOFF.md` for how to read and update this file.

## Current status
Phase: 1 — Scaffold        Status: done
Updated: 2026-07-22 by claude-code        Build: green

## Next action
Start `docs/TASKS.md` Phase 2, task **2.1** ("Config loader"):
Create a Config struct to read from environment variables (JellyfinURL,
JellyfinAPIKey, JellyfinUserID, PublicURL, Port, SessionTTL). Pass Config
into server.New and log the loaded config on startup (mask the API key).

## Phase checklist
Phase 0 — Docs & repo init (done):
- [x] Write CLAUDE.md
- [x] Write README.md
- [x] Write docs/PLAN.md
- [x] Write docs/EXECUTION.md
- [x] Write docs/HANDOFF.md
- [x] Write docs/STATE.md
- [x] git init + .gitignore + initial commit

Phase 1 — Scaffold (done):
- [x] go mod init
- [x] net/http server with GET /healthz -> 200
- [x] Vite React + TS app in web/ (placeholder page)
- [x] //go:embed web/dist + SPA fallback handler
- [x] Multi-stage Dockerfile
- [x] docker-compose.yml (single service, env, port)
- [x] .env.example with all env vars

## Handoff log (append-only, newest first)

### 2026-07-22 — claude-code — handback
- Done: Phase 1 complete (tasks 1.1–1.8). Initialized Go module, scaffolded Vite React app with placeholder, created server package with health endpoint and SPA embed+fallback, built multi-stage Dockerfile and docker-compose.yml. Fixed npm registry issue (custom Tyler registry → public registry) and React version (19 → 18 for react-tinder-card compat). Verified whole phase: docker compose up/down, health check `{"status":"ok"}`, root page renders.
- In-flight: none.
- Next: Start `docs/TASKS.md` Phase 2, task 2.1.
- Files touched: main.go, server/{server,health,static}.go, web/src/App.tsx, web/vite.config.ts, web/package.json, web/package-lock.json, Dockerfile, docker-compose.yml, .env.example, go.mod, go.sum, docs/STATE.md
- Verify: `docker compose up --build -d && curl -fsS localhost:8080/healthz` returns `{"status":"ok"}` and `curl -fsS localhost:8080/` renders HTML; `docker compose down`; `git log --oneline | head -2` shows two commits (1.5 scaffold + 1.6-8 deploy).

### 2026-07-22 — claude-code — handback
- Done: Added `docs/TASKS.md` (granular junior checklist with locked tech + copy-paste code). Locked four owner decisions into PLAN.md/TASKS.md: roster locks at Begin (RequiredCount = locked headcount, adjustable down); admin-set deck cap default 50; admin access open on LAN (no auth); module path `github.com/eiladin/movie-night-showdown`. Threaded TASKS.md into CLAUDE.md resume order.
- In-flight: none.
- Next: Start `docs/TASKS.md` Phase 1, task 1.1.
- Files touched: docs/TASKS.md, docs/PLAN.md, CLAUDE.md, docs/STATE.md
- Verify: `docs/TASKS.md` exists; `git log --oneline` shows the tasks commit.

### 2026-07-22 — claude-code — handback
- Done: Phase 0 complete. Wrote CLAUDE.md, README.md, and docs/{PLAN,EXECUTION,HANDOFF,STATE}.md; `git init`; added .gitignore; initial commit.
- In-flight: none.
- Next: Begin Phase 1 (Scaffold) — see `Next action` above.
- Files touched: CLAUDE.md, README.md, docs/PLAN.md, docs/EXECUTION.md, docs/HANDOFF.md, docs/STATE.md, .gitignore
- Verify: `ls CLAUDE.md docs/` shows all docs; `git log --oneline` shows the initial commit.
