# Movie Night Showdown — Design & Architecture

## Problem

Several people, several devices, no agreement on what to watch. This app ends the
standoff. An admin starts a session against a Jellyfin library, filters it down,
and sets a headcount N. Everyone joins from their own device, swipes through the
same deck of movie posters (left = no, right = yes), and the moment a single movie
collects a "yes" from all N participants, swiping stops and every device shows the
winning poster with a confetti splash.

Runs entirely on the home network — no cloud, no external accounts. Ships as a
single container that talks to an existing Jellyfin server.

## Locked decisions

| Decision | Choice |
|---|---|
| Name | Movie Night Showdown (subdomain: `showdown`) |
| Backend | Go (WebSocket server + Jellyfin proxy) |
| Frontend | React + Vite (TypeScript), embedded into the Go binary via `embed.FS` |
| Join model | Session code + QR, no accounts — guests enter a display name |
| Deploy | Docker Compose (single container) |
| Match rule | Win = a movie gets a "yes" from all N participants. A "no" secretly zeroes a movie's chance (it can't reach N), but cards are **not** removed from the client — users still swipe them normally |
| No match | Session ends with a ranked leaderboard (most yeses) for the admin to break the tie |
| Undo | One-step undo of the last swipe (reverses the vote) |
| Quorum | Roster locks when the admin hits "Begin"; required yeses = the locked lobby headcount (admin may adjust it down). Match = all locked participants said yes |
| Deck cap | Admin-set "max movies" per session, default 50, shuffled. Late joiners are allowed during Lobby, rejected once Active |
| Admin access | Open on the LAN — no password; anyone who can reach the app can start a session and be admin |

## Architecture

```
                 Home network (behind showdown.<domain>)
   +-----------------------------------------------------------+
   |  Docker container: movie-night-showdown (single Go binary)|
   |                                                           |
   |   +--------------+   REST + WS    +--------------------+  |
   |   | React SPA    |<-------------->| Go server          |  |
   |   | (embed.FS)   |                |  - session engine  |  |
   |   +--------------+                |  - WebSocket hub   |  |
   |                                   |  - Jellyfin client |  |
   |                                   |  - image proxy     |  |
   |                                   +---------+----------+  |
   +-------------------------------------------- | ------------+
                                                 | REST + API key
                                        +--------v---------+
                                        |  Jellyfin server |
                                        +------------------+

  Phones / iPads / tablets -- all hit the same URL, join by code/QR
```

**Key principle:** the Jellyfin URL and API key never leave the server. All
library queries and poster images are proxied through the Go backend, so guest
devices only ever talk to Showdown.

## Tech stack

**Backend (Go)**
- Router: `net/http` (stdlib `http.ServeMux`, Go 1.22+) or `chi`.
- WebSockets: `nhooyr.io/websocket` (or `gorilla/websocket`).
- Jellyfin: thin REST client using `X-Emby-Token` / `?api_key=` auth.
- Session state: in-memory, guarded by a mutex (sessions are ephemeral — one
  movie night). SQLite persistence is deferred (see Optional/deferred).
- Static assets: `//go:embed` the built React `dist/`.

**Frontend (React + Vite + TS)**
- State: `zustand` for session/deck/vote state.
- WebSocket client with auto-reconnect + resume-by-token.
- Swipe gestures: `@use-gesture/react` + `react-spring` for a custom card stack
  (or `react-tinder-card` off-the-shelf). Custom gives finer control over the
  undo animation.
- Confetti: `canvas-confetti` (bundled, no CDN).
- QR generation: `qrcode` (client-side, from the join URL).

**Deploy**
- Multi-stage Dockerfile: node build -> go build -> distroless/static final image.
- `docker-compose.yml` with env config; joins the same network as Jellyfin.

## Data model (in-memory)

```go
type Session struct {
    Code          string                       // short, e.g. "SHOW-7K2Q"
    AdminID       string
    RequiredCount int                           // N participants that must agree
    Status        Status                        // Lobby | Active | Matched | Ended
    Deck          []Movie                       // shuffled once at start
    Participants  map[string]*Participant       // id -> participant
    Votes         map[string]map[string]bool    // movieID -> (participantID -> yes?)
    LastSwipe     map[string]Swipe              // participantID -> last swipe (for undo)
    WinnerID      string
    CreatedAt     time.Time
}

type Participant struct { ID, Name string; IsAdmin bool; Connected bool }
type Movie struct {
    ID, Title string; Year int; Genres []string; Overview string;
    Runtime int; CommunityRating float64; PosterURL string /* proxied */
}
```

Sessions expire via a TTL sweeper (`SESSION_TTL`, default a few hours).

## Match engine

`RequiredCount` is set when the admin hits "Begin": the current lobby roster is
locked, and `RequiredCount` defaults to the number of locked-in participants (the
admin may adjust it down before starting). New participants cannot join once the
session is Active.

On every swipe the server records `Votes[movieID][participantID] = (dir == yes)`
and updates `LastSwipe`.

- **Win check** (after a "yes"): `count(yes for movie) == RequiredCount` and no
  "no" votes exist for it -> the movie wins. Set `Status = Matched`, `WinnerID`,
  broadcast `match`.
- **Secret kill:** a "no" makes the movie mathematically unreachable (only N
  voters). It is **not** removed from anyone's client deck — they keep swiping it.
  It simply can never win.
- **Undo:** reverses the participant's last vote (deletes the `Votes` entry and
  rewinds `LastSwipe`); the client re-inserts the card at the front. Because undo
  can reverse a "no", a previously-dead movie can come back.
- **No match:** when all connected participants exhaust the deck with no winner,
  the server sends `session_ended` with a leaderboard sorted by yes-count (ties
  broken by `CommunityRating`). Admin picks the winner manually.

## API + WebSocket protocol

**REST**
- `POST /api/sessions` -> admin creates a session (returns code + join URL).
- `GET  /api/library/preview?<filters>` -> admin previews the filtered movie
  count/list before starting (proxied Jellyfin query).
- `GET  /api/images/{itemId}` -> poster proxy; Go fetches from Jellyfin and
  streams (with cache headers). Keeps the API key server-side.
- `GET  /healthz` -> liveness.

**WebSocket** (`/ws?code=...&token=...`) — JSON envelopes `{type, payload}`:

| Direction | type | payload |
|---|---|---|
| C->S | `join` | `{ code, name }` -> server issues a resume `token` |
| C->S | `admin:start` | `{ filters, maxMovies, requiredCount? }` (locks roster; `requiredCount` defaults to locked headcount, may be lowered; `maxMovies` defaults to 50) |
| C->S | `swipe` | `{ movieID, dir: "yes"\|"no" }` |
| C->S | `undo` | `{}` |
| C->S | `admin:end` | `{}` (force end -> leaderboard) |
| S->C | `session_state` | full session snapshot (status, participants) |
| S->C | `deck` | ordered `[]Movie` for this session |
| S->C | `participant_update` | join/leave/connection changes |
| S->C | `progress` | per-participant swipe progress (lobby/HUD) |
| S->C | `match` | `{ movie }` -> triggers confetti + reveal on all devices |
| S->C | `session_ended` | `{ leaderboard: [{movie, yesCount}] }` |

Reconnect: the client stores the resume `token`; on drop it reconnects and
replays `join` to get a fresh snapshot without losing its place.

## Jellyfin integration

- Config via env: `JELLYFIN_URL`, `JELLYFIN_API_KEY`, optionally
  `JELLYFIN_USER_ID` (needed for "unwatched" filtering).
- Fetch movies:
  `GET {JELLYFIN_URL}/Items?IncludeItemTypes=Movie&Recursive=true&Fields=Genres,Overview,ProductionYear,OfficialRating,CommunityRating,RunTimeTicks&api_key=...`
  (scoped to a library `ParentId` and/or `userId` as needed).
- **Admin filters** map to Jellyfin query params: genre(s), year range, min
  community rating, official rating (MPAA), max runtime, unwatched only
  (`IsPlayed=false`, requires user), library/collection, and a result cap.
- Posters: `GET /Items/{id}/Images/Primary?...` — always served to clients
  through the `/api/images/{itemId}` proxy, never directly.

## Frontend flows

**Admin**
1. Landing -> "Start a Showdown".
2. Filter panel (genres/year/rating/runtime/unwatched) with a live "N movies
   match" preview count.
3. Set required headcount (defaults to detected participant count once people
   join).
4. Lobby: shows the session code + QR, live list of who has joined. "Begin" is
   enabled when ready (admin swipes too).

**Guest**
1. Scan QR / enter code -> type display name -> land in lobby.
2. On start: receive the shared shuffled deck; swipe cards (drag or tap yes/no
   buttons), with undo for the last swipe.
3. HUD shows progress ("2 of 4 still swiping", cards remaining) without leaking
   others' individual votes.

**Match**
- All devices receive `match` -> deck freezes, winning poster fills the screen
  with metadata, `canvas-confetti` fires. Admin gets "Start another" / "Done".

**No match**
- All devices receive `session_ended` -> leaderboard; admin taps a movie to
  declare the winner (which triggers the same reveal + confetti).

## Deployment

- **Dockerfile** (multi-stage): `node:22` builds `web/dist` -> `golang:1.2x`
  builds the static binary with embedded assets -> distroless/`scratch` runtime.
- **docker-compose.yml:** single service, env for Jellyfin config + `PUBLIC_URL`
  (base URL for QR/join links) + `PORT`, on the same Docker network as Jellyfin.
  TLS/subdomain (`showdown.<domain>`) handled by the existing reverse proxy.

Env summary: `JELLYFIN_URL`, `JELLYFIN_API_KEY`, `JELLYFIN_USER_ID` (optional),
`PUBLIC_URL`, `PORT`, `SESSION_TTL`.

## Optional / deferred

- SQLite persistence so sessions survive a container restart (v1 is in-memory).
- Poster prefetch/caching warm-up when a session starts.
- "Admin lowers the required count mid-session" as an alternate tie-break.

## End-to-end verification (final acceptance)

Run on the real network:
1. `docker compose up`, pointed at the live Jellyfin server.
2. Open the admin UI, apply filters, confirm the preview count matches Jellyfin.
3. Start a session; join from a phone via QR and from 2-3 more devices via code.
4. **Match path:** all N devices swipe "yes" on one movie -> swiping halts and the
   winning poster + confetti appear on every device.
5. **Secret-kill path:** one device votes "no" on a movie the others "yes" -> it
   never wins, but the card still appears/swipes normally for others.
6. **No-match path:** exhaust the deck with no consensus -> the leaderboard appears
   and the admin's manual pick triggers the reveal.
7. **Undo:** swipe, undo, confirm the card returns and the vote is reversed
   (including reversing a "no").
8. **Reconnect:** kill and reopen a device mid-session -> it resumes without
   losing its place.
