#!/usr/bin/env bash
# Regenerates the README screenshots end-to-end:
#   1. builds the frontend (including any working-tree changes)
#   2. starts the mock Jellyfin server (scripts/screenshots/mock-jellyfin)
#   3. starts the app against the mock
#   4. drives the app with Playwright (scripts/screenshots/capture.mjs)
#   5. optimizes the regenerated PNGs with oxipng
#
# No real Jellyfin server, network access, or personal data is used. See
# scripts/screenshots/README.md for details.
#
# Usage: bash scripts/screenshots/run.sh   (or: make screenshots)
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

MOCK_PORT="${MOCK_PORT:-8099}"
PORT="${PORT:-8080}"
# BARE_PORT serves a second app with no sources configured at all, which is the
# only state the /setup guide describes. Capturing that page from the configured
# instance would show every source already available — the opposite of what the
# page is for.
BARE_PORT="${BARE_PORT:-8081}"
PUBLIC_URL="${PUBLIC_URL:-http://localhost:${PORT}}"

LOG_DIR="$(mktemp -d)"
MOCK_LOG="${LOG_DIR}/mock-jellyfin.log"
APP_LOG="${LOG_DIR}/app.log"
BARE_LOG="${LOG_DIR}/app-bare.log"

# RUN_DIR holds the paths the settings screen displays back, so they have to be
# stable. The screen reports CACHE_DIR and CONFIG_FILE verbatim in its read-only
# Container section: pointed at mktemp -d, every regenerated screenshot would embed
# a different random path and the committed PNG would change on every run.
#
# It is deleted and recreated below, so it carries no state between runs, which is
# the property mktemp was providing.
#
# The paths are *relative* to the repository root, which the app runs from, because
# the settings screen prints them back verbatim. An absolute path would embed whoever
# ran the pipeline — their home directory, in a committed public screenshot — and
# would differ on every machine, so the PNG could never be reproduced.
RUN_DIR=".screenshots-run"
# A fresh, per-run poster cache. The app caches posters on disk keyed by movie id +
# image tag; a persistent cache (the default is a shared temp dir) would serve stale
# art after the fixtures change.
CACHE_DIR="${RUN_DIR}/poster-cache"
# The app writes its config file (and its setup token) here. Without this it would
# default to ./config/config.yaml relative to the working directory, leaving a
# credential-bearing file in the repository and carrying state between runs, which a
# deterministic pipeline cannot have.
CONFIG_FILE="${RUN_DIR}/config.yaml"
BARE_CONFIG_FILE="${RUN_DIR}/config-bare.yaml"

MOCK_PID=""
APP_PID=""
BARE_PID=""

log() { printf '==> %s\n' "$*"; }

cleanup() {
  local status=$?
  if [[ -n "$BARE_PID" ]] && kill -0 "$BARE_PID" 2>/dev/null; then
    kill "$BARE_PID" 2>/dev/null || true
    wait "$BARE_PID" 2>/dev/null || true
  fi
  if [[ -n "$APP_PID" ]] && kill -0 "$APP_PID" 2>/dev/null; then
    kill "$APP_PID" 2>/dev/null || true
    wait "$APP_PID" 2>/dev/null || true
  fi
  if [[ -n "$MOCK_PID" ]] && kill -0 "$MOCK_PID" 2>/dev/null; then
    kill "$MOCK_PID" 2>/dev/null || true
    wait "$MOCK_PID" 2>/dev/null || true
  fi
  exit "$status"
}
trap cleanup EXIT INT TERM

port_in_use() {
  # Plain TCP connect check (not an HTTP request) so it correctly detects any
  # listener on the port, regardless of what it serves at "/".
  (exec 3<>"/dev/tcp/127.0.0.1/$1") 2>/dev/null
  local ok=$?
  exec 3>&- 2>/dev/null || true
  exec 3<&- 2>/dev/null || true
  return "$ok"
}

for p in "$MOCK_PORT" "$PORT" "$BARE_PORT"; do
  if port_in_use "$p"; then
    echo "error: something is already listening on port $p (stop it, or set MOCK_PORT/PORT to a free port)" >&2
    exit 1
  fi
done

# A clean run directory. The paths inside it are displayed in a committed
# screenshot, so they are fixed; the state inside must not be.
rm -rf "$RUN_DIR"
mkdir -p "$CACHE_DIR"
# Relative paths are resolved against the working directory, which is REPO_ROOT.

log "building frontend (web/npm run build)"
(cd web && npm install --no-audit --no-fund && npm run build)

log "installing screenshot pipeline dependencies"
(cd scripts/screenshots && npm install --no-audit --no-fund)
(cd scripts/screenshots && npx playwright install chromium)

log "building mock jellyfin + app binaries"
# Built to real binaries (rather than run via `go run`) so the PIDs below are
# the actual server processes: `go run` spawns a child process and killing
# the wrapper can leave it running as an orphan holding the port open.
MOCK_BIN="${LOG_DIR}/mock-jellyfin"
APP_BIN="${LOG_DIR}/app"
go build -o "$MOCK_BIN" ./scripts/screenshots/mock-jellyfin
go build -o "$APP_BIN" .

log "starting mock jellyfin on :${MOCK_PORT} (log: ${MOCK_LOG})"
MOCK_PORT="$MOCK_PORT" "$MOCK_BIN" >"$MOCK_LOG" 2>&1 &
MOCK_PID=$!

for _ in $(seq 1 30); do
  if ! kill -0 "$MOCK_PID" 2>/dev/null; then
    echo "error: mock jellyfin exited early; log follows:" >&2
    cat "$MOCK_LOG" >&2
    exit 1
  fi
  if curl -fsS "http://localhost:${MOCK_PORT}/Items" >/dev/null 2>&1; then
    break
  fi
  sleep 0.5
done
if ! curl -fsS "http://localhost:${MOCK_PORT}/Items" >/dev/null 2>&1; then
  echo "error: mock jellyfin never became ready; log follows:" >&2
  cat "$MOCK_LOG" >&2
  exit 1
fi

log "starting app on :${PORT} (JELLYFIN_URL=http://localhost:${MOCK_PORT}, log: ${APP_LOG})"
JELLYFIN_URL="http://localhost:${MOCK_PORT}" \
JELLYFIN_API_KEY="dev" \
PUBLIC_URL="$PUBLIC_URL" \
PORT="$PORT" \
CACHE_DIR="$CACHE_DIR" \
CONFIG_FILE="$CONFIG_FILE" \
"$APP_BIN" >"$APP_LOG" 2>&1 &
APP_PID=$!

for _ in $(seq 1 60); do
  if ! kill -0 "$APP_PID" 2>/dev/null; then
    echo "error: app exited early; log follows:" >&2
    cat "$APP_LOG" >&2
    exit 1
  fi
  if curl -fsS "http://localhost:${PORT}/healthz" >/dev/null 2>&1; then
    break
  fi
  sleep 0.5
done
if ! curl -fsS "http://localhost:${PORT}/healthz" >/dev/null 2>&1; then
  echo "error: app never became healthy; log follows:" >&2
  cat "$APP_LOG" >&2
  exit 1
fi

log "starting a second app with no sources on :${BARE_PORT} (log: ${BARE_LOG})"
# Every source variable is explicitly cleared, not merely left unset. The pipeline
# inherits the caller's environment, so a developer with TMDB_READ_TOKEN exported —
# which is the normal state for anyone working on this — would otherwise hand the
# "unconfigured" app a working streaming source, and the capture would show the setup
# guide reporting sources already configured. That is the opposite of what the page
# documents.
env -u JELLYFIN_URL -u JELLYFIN_API_KEY -u JELLYFIN_USER_ID -u JELLYFIN_LIBRARIES \
    -u PLEX_URL -u PLEX_TOKEN -u PLEX_LIBRARY_SECTION -u PLEX_LIBRARY_SECTIONS \
    -u TMDB_READ_TOKEN -u STREAMING_PROVIDERS -u TMDB_WATCH_REGION \
PUBLIC_URL="http://localhost:${BARE_PORT}" \
PORT="$BARE_PORT" \
CACHE_DIR="$CACHE_DIR" \
CONFIG_FILE="$BARE_CONFIG_FILE" \
"$APP_BIN" >"$BARE_LOG" 2>&1 &
BARE_PID=$!

for _ in $(seq 1 60); do
  if ! kill -0 "$BARE_PID" 2>/dev/null; then
    echo "error: the unconfigured app exited early; log follows:" >&2
    cat "$BARE_LOG" >&2
    exit 1
  fi
  if curl -fsS "http://localhost:${BARE_PORT}/healthz" >/dev/null 2>&1; then
    break
  fi
  sleep 0.5
done
if ! curl -fsS "http://localhost:${BARE_PORT}/healthz" >/dev/null 2>&1; then
  echo "error: the unconfigured app never became healthy; log follows:" >&2
  cat "$BARE_LOG" >&2
  exit 1
fi

# The settings screen is behind the setup token, which is generated on first start
# and printed to the log. That log line is the only place it appears, so the capture
# is handed the token rather than left to find it.
SETUP_TOKEN="$(grep -oE 'setup: token for configuration changes: [0-9a-f]+' "$APP_LOG" \
  | tail -1 | awk '{print $NF}')"
if [[ -z "$SETUP_TOKEN" ]]; then
  echo "error: could not read the setup token from ${APP_LOG}" >&2
  exit 1
fi

log "capturing screenshots"
(cd scripts/screenshots \
  && CAPTURE_BASE_URL="http://localhost:${PORT}" \
     CAPTURE_BARE_URL="http://localhost:${BARE_PORT}" \
     CAPTURE_SETUP_TOKEN="$SETUP_TOKEN" \
     node capture.mjs)

log "optimizing regenerated PNGs with oxipng"
oxipng -o max --strip safe \
  docs/screenshots/01-landing.png \
  docs/screenshots/02-host.png \
  docs/screenshots/03-lobby.png \
  docs/screenshots/04-swipe.png \
  docs/screenshots/05-result.png \
  docs/screenshots/06-setup.png \
  docs/screenshots/07-settings.png

# The run directory holds a config file with a setup token in it. It is not needed
# once the captures exist, and leaving a credential-bearing file in the working tree
# is how it ends up in a commit.
rm -rf "$RUN_DIR"

log "done — see docs/screenshots/0{1-7}-*.png"
