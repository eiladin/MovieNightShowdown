# Movie Night Showdown — Phased Execution Plan

This is the build sequence. Each phase is self-contained and written with the
same schema so any agent can resume it cold. `docs/STATE.md` says which phase is
current and what the next concrete action is. **Read `docs/STATE.md` first.**

Phase schema:
- **Goal** — one sentence.
- **Entry criteria** — what must be true/green before starting.
- **Tasks** — ordered, concrete steps.
- **Definition of done (DoD)** — checkable exit conditions.
- **Verification** — exact command(s) and expected result.
- **Continuation logic** — how to resume if interrupted mid-phase.

Global rules: every phase ends with a green build and passing verification;
commit with Conventional Commits; update `docs/STATE.md` and append a handback
entry per `docs/HANDOFF.md` before stopping.

---

## Phase 0 — Docs & repo init

- **Goal:** Establish the self-sufficient docs and initialize the repository.
- **Entry criteria:** Empty project folder.
- **Tasks:**
  1. Write `CLAUDE.md`, `README.md`, and `docs/{PLAN,EXECUTION,HANDOFF,STATE}.md`.
  2. `git init`; add a `.gitignore` (Node/Go artifacts).
  3. Commit the docs (`docs: initialize project docs and handoff protocol`).
- **DoD:** All docs exist and are self-sufficient (a cold agent given only
  "continue" could act); repo is initialized with an initial commit.
- **Verification:** `ls CLAUDE.md docs/` shows all files; `git log --oneline`
  shows the initial commit.
- **Continuation logic:** If interrupted, missing docs are listed in
  `STATE.md > Next action`. Re-run remaining writes; the docs are independent
  files, so partial completion is safe to finish in any order.

---

## Phase 1 — Scaffold

- **Goal:** A single container that serves a placeholder React app and a health
  endpoint, with the embed + Docker pipeline working end to end.
- **Entry criteria:** Phase 0 committed.
- **Tasks:**
  1. `go mod init` (module path e.g. `github.com/eiladin/movie-night-showdown`).
  2. Minimal `net/http` server: `GET /healthz` -> 200 `{"status":"ok"}`; serve
     embedded static files for everything else.
  3. `web/`: Vite React + TS app (`npm create vite@latest`), a placeholder page.
  4. Wire `//go:embed web/dist` and an SPA fallback handler.
  5. Multi-stage `Dockerfile` (node build -> go build -> distroless) and
     `docker-compose.yml` (single service, env vars, port).
  6. `.env.example` with all env vars from `CLAUDE.md`.
- **DoD:** `docker compose up --build` serves the placeholder page and
  `/healthz` returns 200.
- **Verification:**
  - `docker compose up --build -d`
  - `curl -fsS localhost:${PORT:-8080}/healthz` -> `{"status":"ok"}`
  - Open `localhost:${PORT:-8080}/` -> placeholder page renders.
- **Continuation logic:** Sub-steps are independent (Go server / Vite app /
  Docker). `STATE.md > Next action` names the next sub-step. If the Go build
  fails because `web/dist` is missing, run `cd web && npm install && npm run
  build` first, or guard the embed with a committed placeholder `web/dist`.

---

## Phase 2 — Jellyfin client + image proxy

- **Goal:** The backend can query and filter the Jellyfin library and proxy
  posters, and the admin can preview a filtered list.
- **Entry criteria:** Phase 1 green; Jellyfin reachable with `JELLYFIN_URL` +
  `JELLYFIN_API_KEY` set.
- **Tasks:**
  1. Go Jellyfin client: fetch movies with the `Fields` list from `PLAN.md`.
  2. Map admin filters (genre, year range, min community rating, official rating,
     max runtime, unwatched, library, cap) to Jellyfin query params.
  3. `GET /api/library/preview?<filters>` -> filtered count + list.
  4. `GET /api/images/{itemId}` -> stream the Jellyfin `Primary` image with cache
     headers; API key stays server-side.
  5. Minimal admin filter UI that calls `/api/library/preview` and shows the
     count + poster thumbnails (via the proxy).
- **DoD:** Admin can apply filters and see a correct count and posters loaded
  through the proxy against real Jellyfin.
- **Verification:**
  - `curl -fsS "localhost:8080/api/library/preview?genres=Action" | jq '.count'`
    returns a plausible number matching Jellyfin.
  - A proxied poster URL returns image bytes (`curl -I .../api/images/<id>` ->
    200, `Content-Type: image/*`).
- **Continuation logic:** Client, endpoints, and UI are independent. If only the
  client exists, `Next action` is the preview endpoint; if endpoints exist, it is
  the UI. Verify against Jellyfin before moving on — a wrong `Fields`/filter
  mapping is the most likely partial-state bug.

---

## Phase 3 — Sessions + WebSocket hub

- **Goal:** Admins create sessions; guests join by code/QR into a live lobby;
  connections resume after a drop.
- **Entry criteria:** Phase 2 green.
- **Tasks:**
  1. In-memory session store (`map[code]*Session`, mutex, TTL sweeper).
  2. `POST /api/sessions` -> create session, return code + join URL.
  3. WebSocket hub (`/ws?code=&token=`): handle `join`, issue resume `token`,
     broadcast `session_state` and `participant_update`.
  4. Lobby UI: admin sees code + QR (via `qrcode`) and the live participant list;
     guests enter code/name and appear in the lobby.
  5. Reconnect/resume on the client (store token, replay `join`).
- **DoD:** Two or more devices join one session and see each other in the lobby;
  a dropped device reconnects and reappears without a new identity.
- **Verification:** Create a session; open it in two browser contexts; both show
  in the participant list; kill one socket (devtools offline) and confirm it
  rejoins as the same participant.
- **Continuation logic:** Session store -> create endpoint -> WS hub -> lobby UI
  -> reconnect, in that dependency order. `Next action` names the next link.
  Reconnect can be finished last without blocking the visible lobby.

---

## Phase 4 — Swipe deck + vote engine

- **Goal:** Participants swipe a shared deck; the server records votes, supports
  undo, and detects a match (including secret-kill semantics).
- **Entry criteria:** Phase 3 green.
- **Tasks:**
  1. `admin:start {requiredCount, filters}` -> build + shuffle the deck once,
     broadcast `deck`, set `Status = Active`.
  2. Swipe deck UI (`@use-gesture` + `react-spring` card stack) with yes/no drag
     + buttons and a one-step undo control.
  3. `swipe` and `undo` handlers server-side; record `Votes`, track `LastSwipe`.
  4. Match detection: on a "yes", win when `count(yes)==RequiredCount` and no
     "no" votes. Secret-kill: never remove killed movies from the client deck.
  5. `progress` broadcasts for the HUD (counts only, no per-user votes leaked).
- **DoD:** N yeses on one movie produce a `match` server-side; undo reverses a
  vote (and can revive a secretly-killed movie).
- **Verification:** With `requiredCount=2`, two contexts swipe "yes" on the same
  movie -> server emits `match`. Swipe then undo -> the vote is gone from state
  and the card returns.
- **Continuation logic:** Vote engine and match detection are the correctness
  core — land and unit-test them before the gesture polish. If the UI is partial
  but the engine is done, drive it with `wscat`/a test client to verify match
  logic. `Next action` distinguishes "engine" vs "gesture UI".

---

## Phase 5 — Reveal + confetti + leaderboard

- **Goal:** A match reveals the winner with confetti on every device; a no-match
  ends in a leaderboard the admin resolves.
- **Entry criteria:** Phase 4 green.
- **Tasks:**
  1. On `match`: freeze all decks, full-screen winning poster + metadata, fire
     `canvas-confetti`. Admin controls: "Start another" / "Done".
  2. No-match: when all participants exhaust the deck, emit `session_ended` with a
     leaderboard (yes-count desc, tie-break `CommunityRating`).
  3. Leaderboard UI; admin taps a movie to declare the winner -> same reveal path.
- **DoD:** Match fires the reveal + confetti on every connected device; the
  no-match path yields a working leaderboard and manual pick.
- **Verification:** Force a match -> confetti + poster on all contexts. Exhaust a
  small deck with no consensus -> leaderboard appears; admin pick triggers the
  reveal.
- **Continuation logic:** Match-reveal and no-match-leaderboard are two
  independent paths sharing the reveal component. Build the reveal component
  first (used by both). `Next action` names which path remains.

---

## Phase 6 — Polish + deploy

- **Goal:** Production-ready: robust reconnection, session cleanup, good mobile
  feel, finalized deployment.
- **Entry criteria:** Phase 5 green.
- **Tasks:**
  1. Reconnection edge cases (mid-swipe drop, admin drop, late joiner).
  2. TTL sweeper for abandoned sessions; graceful shutdown.
  3. Mobile gesture feel (thresholds, haptics-free animations), HUD polish.
  4. Finalize `docker-compose.yml` and README run instructions.
- **DoD:** The full end-to-end verification in `PLAN.md` passes on the real
  network.
- **Verification:** Execute all eight steps of `PLAN.md > End-to-end verification`.
- **Continuation logic:** Items are independent hardening tasks; `STATE.md`
  checklist tracks which remain. Ship only when the full E2E passes.

---

## Phase 7 — Poster caching & proactive warming

- **Goal:** Serve posters from an on-disk cache keyed by Jellyfin's image tag
  (cache-busting on artwork change), de-duplicate concurrent fetches, and warm
  the cache at the "go to the Lobby" transition so a session starts warm.
- **Entry criteria:** Phase 6 green (project shipped).
- **Tasks:**
  1. Thread Jellyfin's Primary image tag into `posterURL` (`?tag=`).
  2. Add an on-disk poster cache (`server/imagecache.go`) with singleflight
     de-duplication, atomic writes, and old-tag pruning.
  3. Make `GET /api/images/{id}` read through the cache; serve `immutable` when a
     tag is present.
  4. Add `POST /api/library/warm` that background-prefetches the filtered set.
  5. Fire the warm from the frontend's "go to the Lobby" action.
  6. Add `CACHE_DIR` config + a compose volume; document it.
- **DoD:** Posters serve from disk on repeat requests; changed artwork produces a
  new `?tag=` and a pruned old file; concurrent requests for one poster hit
  Jellyfin once; clicking "go to the Lobby" warms the cache without blocking.
- **Verification:** See Phase 7 verify steps in `docs/TASKS.md` (7.10).
- **Continuation logic:** Tasks are ordered to keep the Go build green per
  group; `STATE.md > Next action` names the next unchecked 7.x task. The
  frontend (7.6) and docs (7.8) are independent of each other.
