# Movie Night Showdown

A self-hosted movie matcher for a home network. Stop arguing about what to watch:
an admin filters a Jellyfin library and sets a headcount, everyone joins from
their own phone/tablet, and swipes through movie posters (left = no, right = yes).
The moment one movie gets a "yes" from **everyone**, all devices light up with the
winning poster and a confetti splash.

- Runs entirely on your own network — no cloud, no external accounts.
- Ships as a single container that talks to your existing Jellyfin server.
- Guests join by session code or QR; no logins.

## Quickstart

```bash
cp .env.example .env      # then edit with your Jellyfin details (created in Phase 1)
docker compose up --build
```

Then open the app at `PUBLIC_URL` (e.g. `https://showdown.example.com`), start a
session as admin, and share the code/QR with everyone else.

> Status: under construction. See `docs/STATE.md` for the current phase.

## Configuration

| Variable | Required | Description |
|---|---|---|
| `JELLYFIN_URL` | yes | Base URL of your Jellyfin server |
| `JELLYFIN_API_KEY` | yes | Jellyfin API key (kept server-side; never exposed to clients) |
| `JELLYFIN_USER_ID` | optional | Required to filter by "unwatched" |
| `PUBLIC_URL` | yes | Base URL used to build join links and QR codes |
| `PORT` | optional | Listen port (default `8080`) |
| `SESSION_TTL` | optional | How long idle sessions live (default a few hours) |
| `CACHE_DIR` | optional | Directory for the on-disk poster cache (default a temp dir); mount a volume in Docker to persist it across restarts |

TLS and the `showdown.<domain>` subdomain are expected to be handled by your
existing reverse proxy.

## How it works

- **Backend (Go):** session engine, WebSocket hub, Jellyfin REST client, and a
  poster image proxy. Serves the React frontend embedded in the binary.
- **Frontend (React + Vite):** admin filter/lobby screens and the swipe deck.
- **The Jellyfin API key never leaves the server** — all library queries and
  poster images are proxied through the backend.

## Documentation

- `docs/PLAN.md` — full design and architecture.
- `docs/EXECUTION.md` — the phased build plan.
- `docs/HANDOFF.md` — how work is handed between contributors/agents.
- `docs/STATE.md` — current progress.
