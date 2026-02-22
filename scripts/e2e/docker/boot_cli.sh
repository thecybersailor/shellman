#!/usr/bin/env bash
set -euo pipefail
mkdir -p /workspace/logs
export DEBIAN_FRONTEND=noninteractive
apt-get update >/dev/null
apt-get install -y --no-install-recommends tmux curl ca-certificates >/dev/null

# Install e2e codex mock command into PATH for tmux shell.
install -m 0755 /workspace/scripts/e2e/codex_mock_command.sh /usr/local/bin/codex

if [[ -z "${OPENAI_ENDPOINT:-}" || -z "${OPENAI_MODEL:-}" || -z "${OPENAI_API_KEY:-}" ]]; then
  echo "agent openai env missing: OPENAI_ENDPOINT / OPENAI_MODEL / OPENAI_API_KEY" | tee -a /workspace/logs/cli.log
fi

# Isolate e2e sqlite and rebuild on every container start.
export SHELLMAN_CONFIG_DIR="/tmp/shellman-e2e-config"
rm -rf "${SHELLMAN_CONFIG_DIR}"
mkdir -p "${SHELLMAN_CONFIG_DIR}"

tmux -L tt_e2e kill-server >/dev/null 2>&1 || true
tmux -L tt_e2e -f /dev/null new-session -d -s e2e 'bash --noprofile --norc'

cd /workspace/cli
SHELLMAN_MODE=local \
SHELLMAN_LOCAL_HOST=0.0.0.0 \
SHELLMAN_LOCAL_PORT=4621 \
SHELLMAN_WEBUI_MODE=dev \
SHELLMAN_WEBUI_DEV_PROXY_URL="http://webui:15173" \
SHELLMAN_WEBUI_DIST_DIR="../webui/dist" \
SHELLMAN_TMUX_SOCKET="tt_e2e" \
SHELLMAN_TRACE_STREAM=1 \
go run ./cmd/shellman 2>&1 | tee /workspace/logs/cli.log
