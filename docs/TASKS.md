# Movie Night Showdown — Junior Developer Build Checklist

This is a step-by-step build sheet. Do the steps **in order, top to bottom**.
Each `- [ ]` is one task; check it off (`- [x]`) only after it works. Do not skip
ahead. Do not substitute libraries or versions — the choices below are final and
the rest of the plan depends on them.

If you were told "continue": read `docs/STATE.md` first to find which step you are
on, then come here.

---

## How to work (read this once, then follow it every step)

- **One step at a time.** Finish and verify a step before starting the next.
- **Copy the code blocks exactly.** Where a file's full contents are given,
      create the file with exactly that content. Where a command is given, run it
      exactly.
- **Verify after every step.** Each step ends with a "Verify:" line. If it
      does not pass, fix it before moving on. Do not check the box until Verify
      passes.
- **Tick the box in THIS file the moment Verify passes.** Edit the task's
      `- [ ]` to `- [x]` in `docs/TASKS.md` immediately — not only in
      `docs/STATE.md`. `docs/TASKS.md` is the running record of what is actually
      done; a completed task with an unchecked box is a bug. Before you hand back,
      confirm every task you finished this session is checked here.
- **Commit after each numbered group** (1.x, 2.x, …) with a Conventional
      Commit message, e.g. `feat(server): add health endpoint and SPA embed`.
- **Update `docs/STATE.md` before you stop** (any reason): set the current
      phase/status, set the single `Next action` to the next unchecked task here,
      and append a handback entry. Follow `docs/HANDOFF.md`.
- **Never delete or rewrite** past entries in the `docs/STATE.md` handoff log.
- **If a step fails and you cannot fix it in 3 tries:** stop, write exactly
      what failed (command + full error) into the `docs/STATE.md` handback log,
      set `Build: red (<reason>)`, and hand back. Do not guess-thrash.
- **Do not add libraries** that are not listed below. If you think you need
      one, stop and hand back a note instead.

---

## Locked tech choices (do NOT substitute)

**Backend**
- Go **1.23**. Module path: `github.com/eiladin/movie-night-showdown`.
- Router: **standard library** `net/http.ServeMux` with method+path patterns
  (Go 1.22+ style, e.g. `mux.HandleFunc("GET /healthz", ...)`). No chi, no gin.
- WebSocket: **`github.com/gorilla/websocket`** (latest v1.5.x). Nothing else.
- IDs: **`github.com/google/uuid`** for participant/session internal IDs.
- Session join codes: 4-char uppercase, custom helper (given in Phase 3).
- Storage: **in-memory only** (a `map` + `sync.Mutex`). No database in v1.

**Frontend** (in `web/`, package manager = **npm**)
- **Vite + React 18 + TypeScript**.
- Routing: **`react-router-dom`** v6.
- State: **`zustand`**.
- Swipe cards: **`react-tinder-card`** (has built-in swipe + programmatic
  `swipe()` / `restoreCard()` for undo). No custom gesture engine.
- Confetti: **`canvas-confetti`**.
- QR code: **`react-qr-code`** (renders an SVG component).
- HTTP: native `fetch`. WebSocket: native `WebSocket`. No axios, no socket.io.
- Styling: **plain CSS** files (mobile-first). No Tailwind, no CSS-in-JS.

---

## Locked product rules (decided by the owner — do NOT change or interpret)

- **Quorum:** When the admin clicks "Begin", the current lobby roster is **locked**
  and `RequiredCount` = the number of locked-in participants (the admin may lower
  it before starting, never raise above the roster). A movie wins when **all
  locked participants** voted "yes" on it. New participants may join during Lobby
  but are **rejected once the session is Active**.
- **Deck cap:** The admin sets a "max movies" value in the filter panel,
  **default 50**. The deck is shuffled, then truncated to that many movies.
- **Admin access:** **Open on the LAN** — no password, no auth. Anyone who can
  reach the app can start a session and be admin.
- **Module path:** `github.com/eiladin/movie-night-showdown` (already set).

---

## Target repo layout (what it looks like when done)

```
movie-night-showdown/
  main.go                 # entrypoint: server, routes, embed
  server/
    server.go             # Server struct, router wiring
    health.go             # GET /healthz
    static.go             # SPA embed + fallback handler
    jellyfin.go           # Jellyfin REST client (Phase 2)
    images.go             # GET /api/images/{id} proxy (Phase 2)
    session.go            # Session/Participant types + store (Phase 3)
    hub.go                # WebSocket hub + client pumps (Phase 3)
    messages.go           # WS message envelope + payload types (Phase 3)
    match.go              # vote recording + match detection (Phase 4)
  go.mod / go.sum
  web/
    index.html
    package.json
    vite.config.ts
    src/
      main.tsx, App.tsx, store.ts, ws.ts, api.ts
      pages/  Landing.tsx  AdminSetup.tsx  Lobby.tsx  Swipe.tsx  Result.tsx
      components/  Card.tsx  Confetti.tsx  QRJoin.tsx
      styles/  *.css
    dist/                 # build output (gitignored; built before `go build`)
  Dockerfile
  docker-compose.yml
  .env.example
```

---

## Phase 1 — Scaffold

Goal: one container that serves a placeholder React page and `GET /healthz`.

### 1.1 Initialize the Go module
- [x] From the repo root run: `go mod init github.com/eiladin/movie-night-showdown`
- [x] Confirm Go version: `go version` shows 1.23.x (install via mise if not).
- Verify: `go.mod` exists with the module path and `go 1.23`.

### 1.2 Scaffold the Vite React app
- [x] Run: `npm create vite@latest web -- --template react-ts`
- [x] Run: `cd web && npm install`
- [x] Install runtime deps:
      `npm install react-router-dom zustand react-tinder-card canvas-confetti react-qr-code`
- [x] Install types where needed: `npm install -D @types/canvas-confetti`
- [x] Edit `web/vite.config.ts` to set `base: './'` so embedded assets load from
      any path. Keep the rest of the generated config.
- [x] Replace `web/src/App.tsx` body with a placeholder: a `<h1>Movie Night
      Showdown</h1>` and the text `Coming soon`.
- Verify: `cd web && npm run build` succeeds and creates `web/dist/index.html`.

### 1.3 Backend: server package + health endpoint
- [x] Create `server/server.go`:
```go
package server

import "net/http"

type Server struct {
	mux *http.ServeMux
}

func New() *Server {
	s := &Server{mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler { return s.mux }

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealth)
	// static handler is registered in main.go via SetStatic (needs the embed.FS)
}
```
- [x] Create `server/health.go`:
```go
package server

import (
	"encoding/json"
	"net/http"
)

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
```
- Verify: nothing to run yet; it compiles after 1.4/1.5.

### 1.4 Backend: SPA embed + fallback handler
- [x] Create `server/static.go`:
```go
package server

import (
	"io/fs"
	"net/http"
	"strings"
)

// SetStatic registers the SPA handler using the embedded dist filesystem.
// dist must be the sub-filesystem rooted at the built web/dist directory.
func (s *Server) SetStatic(dist fs.FS) {
	fileServer := http.FileServer(http.FS(dist))
	s.mux.Handle("/", spaFallback(dist, fileServer))
}

// spaFallback serves the requested file if it exists, otherwise index.html
// (so client-side routes like /join/ABCD work on refresh).
func spaFallback(dist fs.FS, fileServer http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			p = "index.html"
		}
		if _, err := fs.Stat(dist, p); err != nil {
			// not a real file -> serve index.html for the SPA router
			r = r.Clone(r.Context())
			r.URL.Path = "/"
		}
		fileServer.ServeHTTP(w, r)
	})
}
```

### 1.5 Backend: entrypoint with embed
- [x] Create `main.go` at repo root:
```go
package main

import (
	"io/fs"
	"log"
	"net/http"
	"os"

	"embed"

	"github.com/eiladin/movie-night-showdown/server"
)

//go:embed all:web/dist
var webDist embed.FS

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	dist, err := fs.Sub(webDist, "web/dist")
	if err != nil {
		log.Fatalf("embed: %v", err)
	}

	s := server.New()
	s.SetStatic(dist)

	log.Printf("movie-night-showdown listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, s.Handler()))
}
```
- [x] IMPORTANT: `//go:embed all:web/dist` needs `web/dist` to exist. Always run
      `cd web && npm run build` **before** `go build`.
- [x] Run: `go mod tidy`
- [x] Run: `cd web && npm run build && cd .. && go build ./...`
- Verify: `go run .` starts; `curl -fsS localhost:8080/healthz` returns
      `{"status":"ok"}`; opening `http://localhost:8080/` shows the placeholder.
- [x] Commit: `feat(server): scaffold Go server with health check and embedded SPA`

### 1.6 Dockerfile
- [x] Create `Dockerfile`:
```dockerfile
# --- Build frontend ---
FROM node:22-alpine AS web
WORKDIR /web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# --- Build backend ---
FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /web/dist ./web/dist
RUN CGO_ENABLED=0 go build -o /out/showdown .

# --- Runtime ---
FROM gcr.io/distroless/static-debian12
COPY --from=build /out/showdown /showdown
EXPOSE 8080
ENTRYPOINT ["/showdown"]
```

### 1.7 docker-compose.yml
- [x] Create `docker-compose.yml`:
```yaml
services:
  showdown:
    build: .
    ports:
      - "8080:8080"
    environment:
      JELLYFIN_URL: ${JELLYFIN_URL}
      JELLYFIN_API_KEY: ${JELLYFIN_API_KEY}
      JELLYFIN_USER_ID: ${JELLYFIN_USER_ID:-}
      PUBLIC_URL: ${PUBLIC_URL:-http://localhost:8080}
      PORT: "8080"
      SESSION_TTL: ${SESSION_TTL:-4h}
    restart: unless-stopped
```

### 1.8 .env.example
- [x] Create `.env.example`:
```
JELLYFIN_URL=https://jellyfin.example.com
JELLYFIN_API_KEY=replace-me
JELLYFIN_USER_ID=
PUBLIC_URL=http://localhost:8080
PORT=8080
SESSION_TTL=4h
```
- Verify (whole phase): `docker compose up --build -d`, then
      `curl -fsS localhost:8080/healthz` returns `{"status":"ok"}` and `/` renders.
      Then `docker compose down`.
- [x] Commit: `chore(deploy): add Dockerfile, compose, and env example`
- [x] Update `docs/STATE.md`: Phase 1 done, Next action = Phase 2 step 2.1.

---

## Phase 2 — Jellyfin client + image proxy

Goal: query/filter the Jellyfin library and proxy posters; admin can preview.

Read `docs/PLAN.md > Jellyfin integration` for the exact query and fields.

### 2.1 Config loader
- [x] Create a small `Config` struct (in `server/server.go` or a `config.go`)
      read from env: `JellyfinURL`, `JellyfinAPIKey`, `JellyfinUserID`,
      `PublicURL`, `Port`, `SessionTTL`. Pass `Config` into `server.New`.
- Verify: log the loaded config (mask the API key) on startup.

### 2.2 Jellyfin client
- [x] Create `server/jellyfin.go` with a `JellyfinClient` holding `baseURL`,
      `apiKey`, `userID`, and an `*http.Client` with a 10s timeout.
- [x] Method `Movies(ctx, filters) ([]Movie, error)`. Build this request:
      `GET {baseURL}/Items?IncludeItemTypes=Movie&Recursive=true&Fields=Genres,Overview,ProductionYear,OfficialRating,CommunityRating,RunTimeTicks`
      Send the API key in the header `X-Emby-Token: <apiKey>` (not the query
      string). Add `userId` when set (needed for unwatched).
- [x] Map Jellyfin JSON items to the `Movie` struct from `docs/PLAN.md`. Convert
      `RunTimeTicks` to minutes: `ticks / 10_000_000 / 60`.
- [x] Set each `Movie.PosterURL` to the **proxied** path `/api/images/<ItemId>`
      (never the raw Jellyfin URL).
- Verify: a temporary unit test or log prints N movies from real Jellyfin.
      (`go test ./server/... -run TestJellyfinClient_Movies -v` against the real
      server: no filters -> 508 movies (TotalRecordCount); note the method
      signature was extended to `([]Movie, int, error)` — the second value is
      Jellyfin's true `TotalRecordCount`, needed so the 2.4 preview `count` is
      accurate even though the movie list itself is capped via `Limit`.)

### 2.3 Filters
- [x] Define a `Filters` struct parsed from query params on `/api/library/preview`:
      `genres` (repeatable), `yearMin`, `yearMax`, `ratingMin` (community),
      `officialRating`, `runtimeMax`, `unwatched` (bool), `libraryId`,
      `maxMovies` (deck cap, **default 50**).
- [x] Map them onto the Jellyfin request: `Genres`, `Years`, `MinCommunityRating`,
      `OfficialRatings`, `Filters=IsUnplayed` (requires `userId`), `ParentId`,
      `Limit`. Filter `runtimeMax` client-side after mapping runtime to minutes.
- Verify: adding `?genres=Action` reduces the count vs no filter.
      (Confirmed against real Jellyfin: no filter TotalRecordCount=508,
      `genres=Action` TotalRecordCount=205 — same test run as 2.2.)

### 2.4 Preview endpoint
- [x] Add `GET /api/library/preview` -> returns JSON `{ "count": N, "movies": [...] }`.
- Verify: `curl -fsS "localhost:8080/api/library/preview?genres=Action" | jq '.count'`
      returns a number matching Jellyfin's Action count.
      (Ran against the real server: no filter `.count`=508; `genres=Action`
      `.count`=205, an exact match to Jellyfin's own `Items?Genres=Action`
      `TotalRecordCount`. `.movies` length capped at the default `maxMovies`=50.)

### 2.5 Image proxy
- [x] Create `server/images.go`, add `GET /api/images/{id}`:
      fetch `{baseURL}/Items/{id}/Images/Primary?maxWidth=600` with the
      `X-Emby-Token` header, stream the body through, copy `Content-Type`, and set
      `Cache-Control: public, max-age=86400`.
- Verify: `curl -I localhost:8080/api/images/<realItemId>` -> `200` and
      `Content-Type: image/*`.
      (Confirmed: `HTTP/1.1 200 OK`, `Content-Type: image/jpeg`,
      `Cache-Control: public, max-age=86400`. A bad id returns `404`.)

### 2.6 Minimal admin preview UI
- [x] Add a `web/src/api.ts` with `getPreview(filters)` using `fetch`.
- [x] Add an `AdminSetup.tsx` page with a few filter inputs (genre multiselect,
      year range, unwatched checkbox) and a "Preview" button that shows the count
      and a grid of poster thumbnails (`<img src={movie.posterURL} />`).
- [x] Add routes in `App.tsx` with `react-router-dom` (`/` Landing, `/admin`).
- Verify: in the browser, applying a filter shows the correct count and posters
      load through `/api/images/...`.
      (`cd web && npm run build` succeeds. Verified the full request path with
      curl against the running server + real Jellyfin instead of a GUI
      browser: `GET /admin` returns the SPA shell (200); the built JS bundle
      calls `api/library/preview`; `GET /api/library/preview?genres=Comedy`
      returns `count=215` with proxied `posterURL` fields; `GET
      /api/images/<id>` for a returned movie returns `200`
      `Content-Type: image/jpeg`. This exercises the same request path the
      browser UI makes; no interactive browser session was available in this
      environment to visually confirm the rendered grid.)
- [x] Commit each group; then update `docs/STATE.md` (Phase 2 done, Next = 3.1).

---

## Phase 3 — Sessions + WebSocket hub

Goal: create sessions, join by code/QR, live lobby, reconnect.

### 3.1 Types + store
- [ ] Create `server/session.go` with the `Session`, `Participant`, `Movie`,
      `Swipe`, `Status` types from `docs/PLAN.md`. Add a `Locked bool` field on
      `Session` (set true when the admin starts) and leave `RequiredCount` at 0
      until start.
- [ ] Create a `Store` with `map[string]*Session` guarded by `sync.Mutex` and
      methods: `Create(adminName) *Session`, `Get(code) (*Session, bool)`,
      `sweepExpired()` (run in a goroutine on a ticker, using `SESSION_TTL`).
- [ ] Add a code generator: 4 chars from the alphabet `ABCDEFGHJKLMNPQRSTUVWXYZ`
      + digits `23456789` (no confusing `0/O/1/I`). Regenerate on collision.
- Verify: a unit test creates a session and fetches it by code.

### 3.2 Create-session endpoint
- [ ] `POST /api/sessions` body `{ "adminName": "..." }` -> creates a session,
      returns `{ "code": "ABCD", "joinURL": "<PUBLIC_URL>/join/ABCD",
      "participantId": "...", "token": "..." }`. The admin is participant #1.
- Verify: `curl -XPOST .../api/sessions -d '{"adminName":"Nate"}'` returns a code.

### 3.3 Message types
- [ ] Create `server/messages.go` with an envelope `{ "type": string, "payload":
      json.RawMessage }` and typed structs for every message in
      `docs/PLAN.md > API + WebSocket protocol`. Marshal helpers included.

### 3.4 WebSocket hub
- [ ] Create `server/hub.go`. Upgrade at `GET /ws?code=&token=` with
      `gorilla/websocket`. **Concurrency rule (important):** each connected client
      gets its own `send chan []byte`; a single `writePump` goroutine per client
      is the ONLY thing that writes to that socket. A `readPump` goroutine reads
      client messages. Never write to a gorilla socket from two goroutines.
- [ ] On `join`: attach the connection to the session's participant (match by
      `token`, or create a new participant if `token` is empty and status is
      Lobby). Broadcast `participant_update` + send `session_state` to the joiner.
- [ ] On disconnect: mark `Connected=false`, broadcast `participant_update`. Keep
      the participant record so they can resume with their `token`.
- Verify: connect two `wscat`/browser clients to one code; both appear in the
      participant list broadcasts.

### 3.5 Lobby UI + reconnect
- [ ] `web/src/ws.ts`: a small WebSocket wrapper that connects, stores the
      `token` in `localStorage`, auto-reconnects with backoff, and replays `join`.
- [ ] `web/src/store.ts` (zustand): session, participants, deck, status, myVoteState.
- [ ] `Landing.tsx`: "Start a Showdown" (calls `POST /api/sessions`, routes to
      `/admin`) and a "Join" box (enter code -> `/join/:code`).
- [ ] `Lobby.tsx`: admin view shows code + `<QRCode value={joinURL} />`
      (react-qr-code) + participant list; guest view shows name entry then the
      participant list. Reached at `/join/:code` and from `/admin`.
- Verify: open the session on two devices/tabs; both show in the lobby; kill one
      socket (devtools "Offline"), restore, and it rejoins as the same participant.
- [ ] Commit groups; update `docs/STATE.md` (Phase 3 done, Next = 4.1).

---

## Phase 4 — Swipe deck + vote engine

Goal: shared deck, swipe/undo, server-side votes, match detection.

### 4.1 Start + deck
- [ ] Handle `admin:start {filters, maxMovies, requiredCount?}`. Reject if the
      caller is not the admin. Then:
      1. **Lock the roster:** set `session.Locked = true`. Count the current
         participants; set `session.RequiredCount = requiredCount` if provided and
         `1 <= requiredCount <= rosterCount`, otherwise `= rosterCount`.
      2. Fetch movies via the Jellyfin client using `filters`.
      3. Shuffle (`math/rand.Shuffle`), then **truncate to `maxMovies`** (default
         50 if not provided). Store as `session.Deck`.
      4. Set `Status=Active`; broadcast `deck` to everyone.
- [ ] Once `session.Locked`/`Active`, the WS `join` handler must **reject new
      participants** (return a clear "session already started" error). Existing
      participants may still reconnect with their `token`.
- [ ] Admin UI (in `Lobby.tsx`): a "Begin" button, a "max movies" number input
      (default 50), and a "required to agree" number input (default = current
      participant count, min 1, max = participant count). "Begin" sends
      `admin:start` with `{filters, maxMovies, requiredCount}` and routes everyone
      to the swipe screen.
- Verify: after start, all clients receive the same ordered deck (length <=
      maxMovies) and `RequiredCount` equals the locked headcount; a brand-new join
      attempt is rejected.

### 4.2 Vote engine + match detection
- [ ] Create `server/match.go`:
```go
// recordSwipe records a vote and returns the winning movie if this swipe caused
// a match. Caller must hold the session lock.
func (s *Session) recordSwipe(participantID, movieID string, yes bool) (winner *Movie, matched bool) {
	if s.Votes[movieID] == nil {
		s.Votes[movieID] = map[string]bool{}
	}
	s.Votes[movieID][participantID] = yes
	s.LastSwipe[participantID] = Swipe{MovieID: movieID, Yes: yes}

	if !yes {
		return nil, false // a "no" can never create a match (secret-kill)
	}
	// Win = every participant voted, and all votes are "yes".
	votes := s.Votes[movieID]
	if len(votes) != s.RequiredCount {
		return nil, false
	}
	for _, v := range votes {
		if !v {
			return nil, false
		}
	}
	return s.findMovie(movieID), true
}
```
- [ ] Handle `swipe {movieID, dir}`: call `recordSwipe`. If matched, set
      `Status=Matched`, `WinnerID`, and broadcast `match {movie}`. Otherwise
      broadcast `progress`.
- [ ] Handle `undo`: delete the participant's `LastSwipe` vote from
      `Votes[movieID]`, clear `LastSwipe[participantID]`, broadcast `progress`.
      (Undo can revive a secretly-killed movie — that is correct.)
- Verify: with `requiredCount=2`, two clients swipe "yes" on the same movie ->
      server emits `match`. Swipe then undo -> the vote is gone from state.

### 4.3 Swipe UI
- [ ] `components/Card.tsx`: wrap `react-tinder-card`. Props: the movie; call
      `onSwipe(dir)` -> send `swipe` (`right`=yes, `left`=no). Show poster
      (`/api/images/id`), title, year, genres, runtime.
- [ ] `Swipe.tsx`: render the deck as a stack of `Card`s; yes/no buttons that call
      the card ref's `.swipe('right'|'left')`; an Undo button that calls the last
      card's `.restoreCard()` and sends `undo`. A HUD from `progress` ("2 of 4
      still swiping", cards left) — never show other people's individual votes.
- Verify: swiping on the phone updates server state; undo restores the last card.
- [ ] Commit groups; update `docs/STATE.md` (Phase 4 done, Next = 5.1).

---

## Phase 5 — Reveal + confetti + leaderboard

Goal: match reveal with confetti on all devices; no-match leaderboard.

### 5.1 Reveal component
- [ ] `components/Confetti.tsx`: on mount, call `canvas-confetti` a few times
      (burst + a short interval), then stop after ~3s.
- [ ] `Result.tsx`: full-screen winning poster + metadata. When `store.status`
      becomes `Matched`, render this over everything and mount `Confetti`.
- Verify: forcing a `match` shows the poster + confetti on every connected client.

### 5.2 No-match leaderboard
- [ ] Server: when every connected participant has swiped the whole deck with no
      match, emit `session_ended {leaderboard}` sorted by yes-count desc, tie-break
      by `CommunityRating`.
- [ ] `Result.tsx` also handles `Ended`: show the ranked leaderboard; the admin
      can tap a movie to declare the winner -> server sets `Matched`+`WinnerID`
      and broadcasts `match` -> same reveal path (confetti).
- Verify: exhaust a small deck with no consensus -> leaderboard shows; admin pick
      triggers the reveal.
- [ ] Commit groups; update `docs/STATE.md` (Phase 5 done, Next = 6.1).

---

## Phase 6 — Polish + deploy

Goal: robustness + shippable.

- [ ] 6.1 Reconnection edge cases: a participant dropping mid-swipe keeps their
      votes; admin dropping does not end the session; a late joiner during Lobby
      is allowed, during Active is rejected with a clear message.
- [ ] 6.2 TTL sweeper verified: abandoned sessions are removed after `SESSION_TTL`;
      add graceful shutdown (`http.Server.Shutdown` on SIGINT/SIGTERM).
- [ ] 6.3 Mobile feel: swipe thresholds tuned; buttons large; layout fills the
      viewport with no page scroll; posters use `object-fit: cover`.
- [ ] 6.4 Finalize `docker-compose.yml` + README run steps.
- [ ] 6.5 Run the full **End-to-end verification** in `docs/PLAN.md` (all 8 steps)
      on the real network and record the result in `docs/STATE.md`.
- [ ] Final commit and mark the project done in `docs/STATE.md`.
```
```
