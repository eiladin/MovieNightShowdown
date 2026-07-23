# Screenshot pipeline

Regenerates the five app screenshots in `docs/screenshots/` (`01-landing.png`
through `05-result.png`) deterministically, with no dependency on a real
Jellyfin server or any real/personal data.

## What it does

The pipeline runs the actual app (Go server + built React frontend) against
a small, self-contained mock Jellyfin server, then drives it with Playwright
through the real user flow — landing, start a session, lobby, swipe, match —
and screenshots each screen. Nothing here calls out to a real Jellyfin
instance or the network.

- **Mock Jellyfin** (`mock-jellyfin/main.go`): a stdlib-only Go HTTP server
  implementing just the three endpoints the app's Jellyfin client calls
  (`GET /Items`, `GET /Items/Filters`, `GET /Items/{id}/Images/Primary`). It
  serves a hardcoded catalog of **15 fictional movies** (invented titles,
  genres, years, ratings — none of them real films) and placeholder poster
  art from `fixtures/posters/`.
- **Placeholder posters** (`fixtures/posters/*.png`): generated title-card
  images, one per fixture movie, committed as fixtures so the pipeline
  doesn't need to regenerate them on every run.
- **Capture driver** (`capture.mjs`): a Playwright script that opens the app
  in a mobile-sized, dark-mode browser context and walks through the full
  flow, capturing a screenshot at each screen. It waits on real signals
  (selectors, visible text, WebSocket-driven status transitions) rather than
  fixed sleeps.
- **Orchestration** (`run.sh`): builds the frontend, starts the mock Jellyfin
  server and the app against it, runs the capture driver, optimizes the
  output PNGs with `oxipng`, and tears everything down on exit.

The placeholder host name used throughout the flow is **"Alex"** — the only
name that appears anywhere in the generated screenshots.

## Running it

```
make screenshots
```

This is equivalent to `bash scripts/screenshots/run.sh` and requires Node,
npm, and Go on `PATH` (Playwright's Chromium build is installed
automatically on first run). It overwrites:

```
docs/screenshots/01-landing.png
docs/screenshots/02-host.png
docs/screenshots/03-lobby.png
docs/screenshots/04-swipe.png
docs/screenshots/05-result.png
```

`docs/screenshots/hero.png` is a hand-curated image and is never touched by
this pipeline.

Node dependencies for this pipeline (`playwright`) live only in
`scripts/screenshots/package.json` — they are not added to `web/` or the
repo root.

## Regenerating the poster art

The placeholder posters are generated (also via Playwright, rendering an
HTML title card per movie) and committed as fixtures:

```
cd scripts/screenshots
npm install
node gen-posters.mjs
```

This overwrites every PNG in `fixtures/posters/`. The movie list in
`gen-posters.mjs` must stay in sync (same IDs, titles, years) with the
fixture catalog hardcoded in `mock-jellyfin/main.go`.

## Capture settings

| Screenshot | Viewport | Device pixel ratio | Mode | Notes |
|---|---|---|---|---|
| `01-landing.png` | 390×844 | 3x | viewport | Landing page, before any input |
| `02-host.png` | 390×844 | 2x | full page | Host filter screen with Action/Adventure/Comedy + PG-13 selected, after "Preview" |
| `03-lobby.png` | 390×844 | 3x | viewport | Lobby with QR code and the host roster entry |
| `04-swipe.png` | 390×844 | 3x | viewport | Swipe deck, first card |
| `05-result.png` | 390×844 | 3x | viewport | Match screen after a single "Yes" swipe (`requiredCount=1`) |

All contexts use `colorScheme: 'dark'` and an iPhone-class mobile user
agent/touch profile (Playwright's `devices['iPhone 12 Pro']`). Pass 1
(01/03/04/05) captures at 3x device scale; pass 2 (02) captures at 2x with a
full-page screenshot, since the filtered poster grid is taller than one
viewport.

## Troubleshooting

- **Port already in use**: the pipeline listens on `8099` (mock Jellyfin)
  and `8080` (the app) by default. Override with `MOCK_PORT` / `PORT`
  environment variables if either is taken.
- **A step fails**: `capture.mjs` prints which named step failed (e.g.
  `FAILED at step "begin the session"`) — check `web/src/pages/*.tsx` for
  the selector/text it's waiting on if the UI has changed.
- **Chromium not installed**: `run.sh` runs
  `npx playwright install chromium` on every invocation; it's a fast no-op
  once installed.
