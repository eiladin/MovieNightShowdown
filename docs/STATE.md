# Movie Night Showdown — State Ledger

This is the single source of truth for progress. Read it first. Update it last.
See `docs/HANDOFF.md` for how to read and update this file.

## Current status
Phase: 4 — Swipe deck + vote engine        Status: not_started
Updated: 2026-07-22 by claude-code        Build: green

## Next action
Start `docs/TASKS.md` Phase 4, task 4.1

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

Phase 3 — Sessions + WebSocket hub (done):
- [x] Session/Participant/Store types + TTL sweeper + join-code generator
      (`server/session.go`, `server/session_test.go`)
- [x] `POST /api/sessions` create-session endpoint
- [x] WS message envelope + typed payloads (`server/messages.go`)
- [x] WebSocket hub with per-client send channels, single reader/writer
      goroutines, join/resume/disconnect handling (`server/hub.go`)
- [x] Frontend: `web/src/ws.ts`, `web/src/store.ts`, `Landing.tsx`,
      `Lobby.tsx`, `web/src/components/QRJoin.tsx`, `/join/:code` route

## Handoff log (append-only, newest first)

### 2026-07-22 — claude-code — handback
- Done: Phase 3 complete (tasks 3.1–3.5). 3.1: `server/session.go` —
  Status/Swipe/Participant/Session types (reusing the `Movie` type already
  defined in `server/library.go` from Phase 2 rather than redefining it),
  `Store` (`map[string]*Session` + `sync.Mutex`, `Create`/`Get`/
  `sweepExpired` on a 1-minute ticker against `SESSION_TTL`), and a 4-char
  join-code generator over `ABCDEFGHJKLMNPQRSTUVWXYZ23456789` regenerating
  on collision; `server/session_test.go` (`TestStore_CreateAndGet`) passes.
  3.2: `POST /api/sessions` — confirmed against the running server:
  `{"code":"2TJZ","joinURL":"http://localhost:8080/join/2TJZ",
  "participantId":"...","token":"<present>"}`. 3.3: `server/messages.go` —
  `Envelope{type,payload}` + `newEnvelope` helper, typed payloads for every
  message in `docs/PLAN.md`'s protocol table, plus a pragmatic `error`
  payload (not in that table) so the hub can report a rejected join back to
  the one client that caused it. 3.4: `server/hub.go` — gorilla/websocket
  hub; each `Client` owns its own `send chan []byte`, exactly one
  `writePump` per client is the sole writer to its socket, one `readPump`
  reads and dispatches; `join` matches an existing participant by the
  `?token=` query value or creates one while `Status==Lobby`; disconnect
  marks `Connected=false` without deleting the participant. Verified with a
  throwaway Go/gorilla-websocket script (not committed, lived only under
  the scratch dir): created a session via REST, connected client A with the
  admin's token (session_state.yourParticipantId matched the REST
  participantId), connected client B fresh (no token) — both A and B then
  saw `participant_update` with 2 participants; dropped B's socket — A saw
  the roster stay at 2 with B `Connected:false`; reconnected B with its
  saved `yourToken` — B's `session_state.yourParticipantId` was identical
  to its original id, and A's next `participant_update` showed 2
  participants (no duplicate) with B back to `Connected:true`. 3.5: added
  `web/src/ws.ts` (native WebSocket wrapper: persists the resume token in
  `localStorage`, replays `join` on connect, reconnects with exponential
  backoff), `web/src/store.ts` (zustand: code/status/requiredCount/
  participants/deck/myParticipantId/myVoteState), `web/src/components/
  QRJoin.tsx`, `Lobby.tsx` (name-entry for guests, then code + QR (admin
  only) + live roster; reached at `/join/:code`, also linked from `/admin`
  once a session exists), and updated `Landing.tsx`/`AdminSetup.tsx`/
  `App.tsx` accordingly. Note: `web/package.json` already pinned
  `react-router-dom` to `^7.18.1` (a Phase 1 decision, predating this
  session) rather than the `v6` named in Locked tech choices; left as-is —
  out of Phase 3's scope to relitigate, and the `useParams`/`useSearchParams`
  APIs used here are unaffected by that major-version difference.
- In-flight: none.
- Next: Start `docs/TASKS.md` Phase 4, task 4.1.
- Files touched: server/{session,session_test,messages,hub,server}.go,
  go.mod, web/src/{ws,store,api,App}.ts(x), web/src/components/QRJoin.tsx,
  web/src/pages/{Landing,Lobby,AdminSetup}.tsx, web/src/styles/{admin,
  landing,lobby}.css, docs/TASKS.md, docs/STATE.md
- Verify: `go build ./... && go vet ./...` clean; `go test ./...` passes
  (`TestStore_CreateAndGet`); `cd web && npm run build` succeeds (`tsc -b`
  + `vite build`, no errors); with the server running against the real
  Jellyfin env, `curl -XPOST localhost:8080/api/sessions -d
  '{"adminName":"Nate"}'` returns a code/joinURL/participantId/token;
  `curl -I localhost:8080/join/ABCD` and `/admin` both return `200` (SPA
  fallback); the headless WS script (2 clients, drop + reconnect via saved
  token) reproduced the exact scenario in the task instructions and passed,
  as detailed above. Not done: visual confirmation of the rendered Lobby/
  Landing UI in an actual browser — no interactive browser is available in
  this environment; the underlying protocol and page compilation were
  verified instead, as instructed.

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
