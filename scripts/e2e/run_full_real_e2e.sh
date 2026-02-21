#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
WORKER_PORT="${WORKER_PORT:-8787}"
WEBUI_PORT="${WEBUI_PORT:-5173}"
WORKER_BASE_URL="http://127.0.0.1:${WORKER_PORT}"
TMUX_SOCKET="${TMUX_SOCKET:-tt_e2e}"
LOG_DIR="${LOG_DIR:-/tmp/termteam-e2e}"

WORKER_PID=""
WEBUI_PID=""
CLI_PID=""

cleanup() {
  for pid in "$CLI_PID" "$WEBUI_PID" "$WORKER_PID"; do
    if [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null; then
      kill "$pid" 2>/dev/null || true
      wait "$pid" 2>/dev/null || true
    fi
  done
  tmux -L "$TMUX_SOCKET" kill-server 2>/dev/null || true
}
trap cleanup EXIT

wait_http() {
  local url="$1"
  local retries="${2:-80}"
  for _ in $(seq 1 "$retries"); do
    if curl -fsS "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.25
  done
  return 1
}

wait_register() {
  local url="$1"
  local retries="${2:-80}"
  for _ in $(seq 1 "$retries"); do
    if curl -fsS -X POST "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.25
  done
  return 1
}

mkdir -p "$LOG_DIR"
: >"$LOG_DIR/worker.log"
: >"$LOG_DIR/webui.log"
: >"$LOG_DIR/cli.log"
: >"$LOG_DIR/playwright.log"

(
  cd "$ROOT_DIR/edge/cloudflare"
  npx wrangler dev --local --port "$WORKER_PORT" >"$LOG_DIR/worker.log" 2>&1
) &
WORKER_PID=$!

if ! wait_register "$WORKER_BASE_URL/api/register"; then
  echo "worker not ready; see $LOG_DIR/worker.log" >&2
  exit 1
fi

(
  cd "$ROOT_DIR/webui"
  VITE_WORKER_ORIGIN="$WORKER_BASE_URL" npm run dev -- --host 127.0.0.1 --port "$WEBUI_PORT" >"$LOG_DIR/webui.log" 2>&1
) &
WEBUI_PID=$!

if ! wait_http "http://127.0.0.1:${WEBUI_PORT}"; then
  echo "webui not ready; see $LOG_DIR/webui.log" >&2
  exit 1
fi

tmux -L "$TMUX_SOCKET" -f /dev/null new-session -d -s e2e "bash --noprofile --norc"

(
  cd "$ROOT_DIR/cli"
  TERMTEAM_MODE=turn TERMTEAM_WORKER_BASE_URL="$WORKER_BASE_URL" TERMTEAM_TMUX_SOCKET="$TMUX_SOCKET" go run ./cmd/termteam >"$LOG_DIR/cli.log" 2>&1
) &
CLI_PID=$!

for _ in $(seq 1 120); do
  if grep -q "^visit_url=" "$LOG_DIR/cli.log" 2>/dev/null; then
    break
  fi
  sleep 0.25
done

E2E_VISIT_URL="$("$ROOT_DIR/scripts/e2e/helpers/extract_visit_url.sh" "$LOG_DIR/cli.log")"
export E2E_VISIT_URL

(
  cd "$ROOT_DIR/webui"
  npx playwright test e2e/full-real-tmux.spec.ts
) | tee "$LOG_DIR/playwright.log"

echo "playwright video dir: $ROOT_DIR/webui/test-results"
