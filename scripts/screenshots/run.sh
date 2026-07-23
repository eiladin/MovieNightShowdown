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
PUBLIC_URL="${PUBLIC_URL:-http://localhost:${PORT}}"

LOG_DIR="$(mktemp -d)"
MOCK_LOG="${LOG_DIR}/mock-jellyfin.log"
APP_LOG="${LOG_DIR}/app.log"

MOCK_PID=""
APP_PID=""

log() { printf '==> %s\n' "$*"; }

cleanup() {
  local status=$?
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

for p in "$MOCK_PORT" "$PORT"; do
  if port_in_use "$p"; then
    echo "error: something is already listening on port $p (stop it, or set MOCK_PORT/PORT to a free port)" >&2
    exit 1
  fi
done

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

log "capturing screenshots"
(cd scripts/screenshots && CAPTURE_BASE_URL="http://localhost:${PORT}" node capture.mjs)

log "optimizing regenerated PNGs with oxipng"
oxipng -o max --strip safe \
  docs/screenshots/01-landing.png \
  docs/screenshots/02-admin.png \
  docs/screenshots/03-lobby.png \
  docs/screenshots/04-swipe.png \
  docs/screenshots/05-result.png

log "done — see docs/screenshots/0{1-5}-*.png"
