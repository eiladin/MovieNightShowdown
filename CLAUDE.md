# Movie Night Showdown — Agent Bootstrap

This file is auto-loaded by Claude Code for any agent working in this repository.
It is the standing instruction set. Read it fully before doing anything.

## On "continue" (or any resume): do NOT ask the user for context

The user may switch agents at any time and simply say **"continue"**. When that
happens:

1. Do **not** ask the user where things stand, what the layout is, or what is
   next. Everything you need is in this repo.
2. Read, in order:
   - `docs/STATE.md` — the single source of truth for current phase, status, and
     the one concrete `Next action`.
   - The current phase in `docs/EXECUTION.md` — goal, tasks, definition of done,
     verification, and continuation logic for where work was interrupted.
   - `docs/TASKS.md` — the granular, spoon-fed step-by-step checklist with locked
     tech/product choices and copy-paste code. **This is where the actual build
     work happens**; `STATE.md > Next action` points at a specific task here.
   - `docs/PLAN.md` — design/architecture reference, as needed.
3. Run the current phase's **smoke check / verification** (see that phase in
   `docs/EXECUTION.md`) to confirm reality matches what `STATE.md` claims.
   Reconcile `STATE.md` if it does not.
4. Execute `Next action`.
5. Before you stop, perform **handback** exactly as described in
   `docs/HANDOFF.md` (update `STATE.md`, append a dated log entry).

## What this project is

A self-hosted "movie matcher" for a home network. An admin starts a session
against a Jellyfin library, filters it, and sets a headcount N. Up to N people
join from their own devices (session code / QR, no accounts), swipe through a
shared deck of movie posters, and when one movie collects a "yes" from all N
participants, every device shows the winning poster with a confetti splash.
Full product/design detail lives in `docs/PLAN.md`.

## Repository layout

```
CLAUDE.md            This bootstrap (auto-loaded).
README.md            Human quickstart: run, configure, env vars.
docs/
  PLAN.md            Design + architecture (the "what" and "why").
  EXECUTION.md       Phased execution plan; each phase self-contained.
  TASKS.md           Granular step-by-step checklist (locked choices; copy-paste code).
  HANDOFF.md         Handoff/handback protocol between agents.
  STATE.md           Living ledger. Read first, update last.
main.go / go.mod     Go backend (added in Phase 1).
web/                 React + Vite (TypeScript) frontend (added in Phase 1).
Dockerfile           Multi-stage build (added in Phase 1).
docker-compose.yml   Single-service deploy (added in Phase 1).
```

Until Phase 1 lands, only the docs exist.

## Build / run / test (once code exists)

- Build + run everything: `docker compose up --build`
- Backend only: `go build ./... && go run .`
- Frontend dev: `cd web && npm install && npm run dev`
- Frontend production build (embedded by Go): `cd web && npm run build`
- Health check: `curl -fsS localhost:${PORT:-8080}/healthz`

The current phase in `docs/EXECUTION.md` always lists the exact verification
commands that apply right now.

## Conventions

- **Commits:** Conventional Commits (`feat:`, `fix:`, `docs:`, `chore:`, …).
- **Every phase ends green:** the build passes and the phase's verification
  succeeds before handback.
- **`docs/STATE.md` is authoritative** and reflects the true state at the moment
  of handback — never aspirational.
- The docs (`CLAUDE.md`, `README.md`, `docs/*`) are written in a neutral,
  professional voice, since they are read by other agents and humans.

## Configuration (environment variables)

| Var | Required | Purpose |
|---|---|---|
| `JELLYFIN_URL` | yes | Base URL of the Jellyfin server |
| `JELLYFIN_API_KEY` | yes | Jellyfin API key (stays server-side, never sent to clients) |
| `JELLYFIN_USER_ID` | optional | Needed for "unwatched" filtering |
| `PUBLIC_URL` | yes | Base URL used to build QR/join links |
| `PORT` | optional | Listen port (default 8080) |
| `SESSION_TTL` | optional | Session expiry (default a few hours) |
| `CACHE_DIR` | optional | Directory for the on-disk poster cache (default a temp dir); mount a volume in Docker to persist it across restarts |
