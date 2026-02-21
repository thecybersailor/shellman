#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
WORKER_PORT="${WORKER_PORT:-8787}"
WEBUI_PORT="${WEBUI_PORT:-5173}"
WORKER_BASE_URL="http://127.0.0.1:${WORKER_PORT}"
WEBUI_URL="http://127.0.0.1:${WEBUI_PORT}"

WORKER_PID=""
CLI_PID=""
WEBUI_PID=""

cleanup() {
  for pid in "$WEBUI_PID" "$CLI_PID" "$WORKER_PID"; do
    if [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null; then
      kill "$pid" 2>/dev/null || true
      wait "$pid" 2>/dev/null || true
    fi
  done
}
trap cleanup EXIT

wait_http() {
  local url="$1"
  local retries="${2:-50}"
  local delay="${3:-0.2}"
  for _ in $(seq 1 "$retries"); do
    if curl -fsS "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep "$delay"
  done
  return 1
}

if [[ "${1:-}" == "--dry-run" ]]; then
  echo "worker ok"
  echo "cli ok"
  echo "webui ok"
  echo "bridge ok"
  exit 0
fi

if [[ "${1:-}" == "--muxt" ]]; then
  exec bash "$ROOT_DIR/scripts/smoke/muxt_local_web_smoke.sh"
fi

command -v npm >/dev/null
command -v go >/dev/null
command -v curl >/dev/null
command -v node >/dev/null

(
  cd "$ROOT_DIR/edge/cloudflare"
  npm run dev > /tmp/termteam-worker-smoke.log 2>&1
) &
WORKER_PID=$!

for _ in $(seq 1 80); do
  if curl -fsS -X POST "$WORKER_BASE_URL/api/register" >/dev/null 2>/dev/null; then
    break
  fi
  sleep 0.25
done
if ! curl -fsS -X POST "$WORKER_BASE_URL/api/register" >/dev/null 2>/dev/null; then
  echo "worker not ready; see /tmp/termteam-worker-smoke.log" >&2
  exit 1
fi

echo "worker ok"

(
  cd "$ROOT_DIR/webui"
  npm run dev -- --host 127.0.0.1 --port "$WEBUI_PORT" > /tmp/termteam-webui-smoke.log 2>&1
) &
WEBUI_PID=$!

if ! wait_http "$WEBUI_URL" 80 0.25; then
  echo "webui not ready; see /tmp/termteam-webui-smoke.log" >&2
  exit 1
fi

echo "webui ok"

(
  cd "$ROOT_DIR/cli"
  TERMTEAM_WORKER_BASE_URL="$WORKER_BASE_URL" go run ./cmd/termteam > /tmp/termteam-cli-smoke.log 2>&1
) &
CLI_PID=$!

for _ in $(seq 1 60); do
  if grep -q "visit_url=" /tmp/termteam-cli-smoke.log 2>/dev/null && grep -q "agent_ws_url=" /tmp/termteam-cli-smoke.log 2>/dev/null; then
    break
  fi
  sleep 0.2
done
if ! grep -q "visit_url=" /tmp/termteam-cli-smoke.log 2>/dev/null; then
  echo "cli did not print visit_url; see /tmp/termteam-cli-smoke.log" >&2
  exit 1
fi
if ! grep -q "agent_ws_url=" /tmp/termteam-cli-smoke.log 2>/dev/null; then
  echo "cli did not print agent_ws_url; see /tmp/termteam-cli-smoke.log" >&2
  exit 1
fi

echo "cli ok"

visit_url="$(sed -n 's/^visit_url=//p' /tmp/termteam-cli-smoke.log | tail -n1)"
agent_ws_url="$(sed -n 's/^agent_ws_url=//p' /tmp/termteam-cli-smoke.log | tail -n1)"
if [[ -z "$visit_url" || -z "$agent_ws_url" ]]; then
  echo "missing URLs from cli output" >&2
  exit 1
fi
if ! curl -fsS "$visit_url" >/dev/null 2>&1; then
  echo "visit_url not reachable: $visit_url" >&2
  exit 1
fi

turn_uuid="${agent_ws_url##*/}"
bridge_ws_url="ws://127.0.0.1:${WORKER_PORT}/ws/client/${turn_uuid}"
(
  cd "$ROOT_DIR/cli"
  cat >/tmp/termteam-ws-probe.go <<'GOWS'
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"nhooyr.io/websocket"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "missing ws url")
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, os.Args[1], nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	req := map[string]any{
		"id":      "smoke_1",
		"type":    "req",
		"op":      "tmux.list",
		"payload": map[string]any{"scope": "all"},
	}

	buf, _ := json.Marshal(req)
	if err := conn.Write(ctx, websocket.MessageText, buf); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	for {
		_, msg, err := conn.Read(ctx)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

		var m map[string]any
		if err := json.Unmarshal(msg, &m); err != nil {
			continue
		}
		if m["type"] == "res" && m["op"] == "tmux.list" {
			return
		}
	}
}
GOWS
  go run /tmp/termteam-ws-probe.go "$bridge_ws_url"
)

echo "bridge ok"

if [[ "${1:-}" == "--with-e2e" ]]; then
  bash "$ROOT_DIR/scripts/e2e/run_full_real_e2e.sh"
fi
