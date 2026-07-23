# Movie Night Showdown — State Ledger

This is the single source of truth for progress. Read it first. Update it last.
See `docs/HANDOFF.md` for how to read and update this file.

## Current status
Phase: 3 — Sessions + WebSocket hub        Status: not_started
Updated: 2026-07-22 by claude-code        Build: green

## Next action
Start `docs/TASKS.md` Phase 3, task 3.1

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

Phase 2 — Jellyfin client + image proxy (done):
- [x] Config loader (`server/config.go`), masked in startup log
- [x] Jellyfin client (`server/jellyfin.go`): Movies() with real X-Emby-Token
      auth, Movie mapping, proxied PosterURL
- [x] Filters (`server/filters.go`): parsed from query params, mapped onto
      Jellyfin's Genres/Years/MinCommunityRating/OfficialRatings/
      Filters=IsUnplayed/ParentId/Limit; RuntimeMax filtered client-side
- [x] Preview endpoint `GET /api/library/preview`
- [x] Image proxy `GET /api/images/{id}` (`server/images.go`)
- [x] Admin preview UI (`web/src/api.ts`, `AdminSetup.tsx`, `Landing.tsx`,
      routes in `App.tsx`)

## Handoff log (append-only, newest first)

### 2026-07-22 — claude-code — handback
- Done: Phase 2 complete (tasks 2.1–2.6), verified against the real Jellyfin
  server at every step. 2.1: env-based Config (masked API key in startup
  log). 2.2/2.3: JellyfinClient.Movies + Filters — no filter gave 508 movies
  (TotalRecordCount), `genres=Action` gave 205, confirmed via a
  skip-if-unconfigured integration test (`server/jellyfin_test.go`). Note:
  `Movies` returns `([]Movie, int, error)` (added a total-count return)
  rather than the `([]Movie, error)` sketched in TASKS.md 2.2, so
  `/api/library/preview`'s `count` stays the true Jellyfin total even though
  the movie list itself is capped via `Limit=maxMovies` — otherwise a
  genre filter and no filter would both report `maxMovies` (50) once both
  exceed the cap, breaking 2.3's own "reduces the count" verify. 2.4:
  `GET /api/library/preview?genres=Action` -> `.count`=205, exactly matching
  Jellyfin's own count. 2.5: `GET /api/images/<id>` -> `200`,
  `Content-Type: image/jpeg`, `Cache-Control: public, max-age=86400`; bad id
  -> `404`. 2.6: admin preview UI (filters + poster grid) built and
  exercised the full request path via curl (browser UI itself not visually
  confirmed — no interactive browser available in this environment).
  Also fixed an environment bug: `.env`'s `JELLYFIN_URL` had a hostname
  typo (`jellyfin.eiladinx.com`, which does not resolve) instead of the
  real server `jellyfin.eiladin.xyz`; corrected in the local, gitignored
  `.env` (not committed).
- In-flight: none.
- Next: Start `docs/TASKS.md` Phase 3, task 3.1.
- Files touched: server/{config,jellyfin,filters,jellyfin_test,library,images,server}.go,
  main.go, web/src/{App.tsx,api.ts}, web/src/pages/{AdminSetup,Landing}.tsx,
  web/src/styles/admin.css, docs/TASKS.md, docs/STATE.md, .env (local only,
  not committed)
- Verify: `go build ./... && go vet ./...` clean; `go test ./server/... -run
  TestJellyfinClient_Movies -v` passes against the real server; `cd web &&
  npm run build` succeeds; with the server running and real Jellyfin env
  loaded, `curl -fsS "localhost:8080/api/library/preview?genres=Action" |
  jq '.count'` -> `205`; `curl -I localhost:8080/api/images/<id>` -> `200`
  `Content-Type: image/jpeg`.

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
