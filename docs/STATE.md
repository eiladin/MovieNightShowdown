# Movie Night Showdown — State Ledger

This is the single source of truth for progress. Read it first. Update it last.
See `docs/HANDOFF.md` for how to read and update this file.

## Current status
Phase: 1 — Scaffold        Status: not_started
Updated: 2026-07-22 by claude-code        Build: n/a (no code yet)

## Next action
Start `docs/TASKS.md` Phase 1, task **1.1** ("Initialize the Go module"):
`go mod init github.com/eiladin/movie-night-showdown`. Then work straight down
`docs/TASKS.md` in order — it has the locked tech/product choices and copy-paste
code. Check off each `- [ ]` only after its "Verify:" passes.

## Phase checklist
Phase 0 — Docs & repo init (done):
- [x] Write CLAUDE.md
- [x] Write README.md
- [x] Write docs/PLAN.md
- [x] Write docs/EXECUTION.md
- [x] Write docs/HANDOFF.md
- [x] Write docs/STATE.md
- [x] git init + .gitignore + initial commit

Phase 1 — Scaffold (not started):
- [ ] go mod init
- [ ] net/http server with GET /healthz -> 200
- [ ] Vite React + TS app in web/ (placeholder page)
- [ ] //go:embed web/dist + SPA fallback handler
- [ ] Multi-stage Dockerfile
- [ ] docker-compose.yml (single service, env, port)
- [ ] .env.example with all env vars

## Handoff log (append-only, newest first)

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
