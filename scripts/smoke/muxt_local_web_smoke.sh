#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
LOG_DIR="/tmp/muxt-smoke"
CLI_PORT="${CLI_PORT:-4621}"
WEBUI_PORT="${WEBUI_PORT:-5173}"
API_BASE="http://127.0.0.1:${CLI_PORT}"
WEBUI_URL="http://127.0.0.1:${WEBUI_PORT}"

mkdir -p "$LOG_DIR/bin"

cat > "$LOG_DIR/bin/tmux" <<'TMUX'
#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" == "split-window" ]]; then
  echo "mock:0.1"
  exit 0
fi
exit 0
TMUX
chmod +x "$LOG_DIR/bin/tmux"

CLI_PID=""
WEBUI_PID=""
PROJECT_ROOT="${PROJECT_ROOT:-$(mktemp -d /tmp/muxt-project-XXXXXX)}"

cleanup() {
  for pid in "$WEBUI_PID" "$CLI_PID"; do
    if [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null; then
      kill "$pid" 2>/dev/null || true
      wait "$pid" 2>/dev/null || true
    fi
  done
}
trap cleanup EXIT

wait_http() {
  local url="$1"
  local retries="${2:-60}"
  local delay="${3:-0.2}"
  for _ in $(seq 1 "$retries"); do
    if curl -fsS "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep "$delay"
  done
  return 1
}

(
  cd "$ROOT_DIR/cli"
  PATH="$LOG_DIR/bin:$PATH" TERMTEAM_MODE=local TERMTEAM_LOCAL_PORT="$CLI_PORT" go run ./cmd/termteam >"$LOG_DIR/cli.log" 2>&1
) &
CLI_PID=$!

wait_http "$API_BASE/healthz" 80 0.2
curl -fsS "$API_BASE/api/v1/config" >"$LOG_DIR/config.json"

cat >/tmp/muxt-ws-probe.go <<'GOWS'
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"nhooyr.io/websocket"
)

type Message struct {
	Type    string `json:"type"`
	Op      string `json:"op"`
	Payload struct {
		Sessions []string `json:"sessions"`
	} `json:"payload"`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "missing ws url")
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, os.Args[1], nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	req := map[string]any{
		"id":      "smoke_tmux_list_1",
		"type":    "req",
		"op":      "tmux.list",
		"payload": map[string]any{"scope": "all"},
	}
	raw, _ := json.Marshal(req)
	if err := conn.Write(ctx, websocket.MessageText, raw); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	for {
		_, raw, err := conn.Read(ctx)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		var msg Message
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue
		}
		if msg.Type == "res" && msg.Op == "tmux.list" {
			return
		}
	}
}
GOWS

(
  cd "$ROOT_DIR/cli"
  go run /tmp/muxt-ws-probe.go "ws://127.0.0.1:${CLI_PORT}/ws/client/local"
)

(
  cd "$ROOT_DIR/webui"
  npm run dev -- --host 127.0.0.1 --port "$WEBUI_PORT" >"$LOG_DIR/webui.log" 2>&1
) &
WEBUI_PID=$!

wait_http "$WEBUI_URL" 80 0.25

curl -fsS -X POST "$API_BASE/api/v1/projects/active" \
  -H 'content-type: application/json' \
  -d "{\"project_id\":\"p1\",\"repo_root\":\"$PROJECT_ROOT\"}" >/dev/null

ROOT_RESP="$(curl -fsS -X POST "$API_BASE/api/v1/tasks" -H 'content-type: application/json' -d '{"project_id":"p1","title":"root"}')"
ROOT_TASK_ID="$(node -e 'const d=JSON.parse(process.argv[1]); process.stdout.write(d.data.task_id);' "$ROOT_RESP")"

SIB_RESP="$(curl -fsS -X POST "$API_BASE/api/v1/tasks/$ROOT_TASK_ID/panes/sibling" -H 'content-type: application/json' -d '{"title":"sib"}')"
CHILD_RESP="$(curl -fsS -X POST "$API_BASE/api/v1/tasks/$ROOT_TASK_ID/panes/child" -H 'content-type: application/json' -d '{"title":"child"}')"

SIB_TASK_ID="$(node -e 'const d=JSON.parse(process.argv[1]); process.stdout.write(d.data.task_id);' "$SIB_RESP")"
CHILD_TASK_ID="$(node -e 'const d=JSON.parse(process.argv[1]); process.stdout.write(d.data.task_id);' "$CHILD_RESP")"

TREE_RESP="$(curl -fsS "$API_BASE/api/v1/projects/p1/tree")"
node -e '
const d=JSON.parse(process.argv[1]);
if(!d.ok) process.exit(1);
const ids=(d.data.nodes||[]).map((n)=>n.task_id);
if(ids.length<3) process.exit(2);
' "$TREE_RESP"

PANES_FILE="$PROJECT_ROOT/.muxt/state/panes.json"
if [[ ! -f "$PANES_FILE" ]]; then
  echo "missing panes file: $PANES_FILE" >&2
  exit 1
fi

grep -q "$SIB_TASK_ID" "$PANES_FILE"
grep -q "$CHILD_TASK_ID" "$PANES_FILE"

echo "muxt local web smoke passed"
echo "logs: $LOG_DIR"
