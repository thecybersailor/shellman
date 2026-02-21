#!/usr/bin/env bash
set -euo pipefail
mkdir -p /workspace/logs
export DEBIAN_FRONTEND=noninteractive
apt-get update >/dev/null
apt-get install -y --no-install-recommends tmux curl ca-certificates >/dev/null

if [[ -z "${OPENAI_ENDPOINT:-}" || -z "${OPENAI_MODEL:-}" || -z "${OPENAI_API_KEY:-}" ]]; then
  echo "agent openai env missing: OPENAI_ENDPOINT / OPENAI_MODEL / OPENAI_API_KEY" | tee -a /workspace/logs/cli.log
fi

# Isolate e2e sqlite and rebuild on every container start.
export TERMTEAM_CONFIG_DIR="/tmp/muxt-e2e-config"
rm -rf "${TERMTEAM_CONFIG_DIR}"
mkdir -p "${TERMTEAM_CONFIG_DIR}"

tmux -L tt_e2e kill-server >/dev/null 2>&1 || true
tmux -L tt_e2e -f /dev/null new-session -d -s e2e 'bash --noprofile --norc'

cd /workspace/cli
TERMTEAM_MODE=local \
TERMTEAM_LOCAL_HOST=0.0.0.0 \
TERMTEAM_LOCAL_PORT=4621 \
TERMTEAM_WEBUI_MODE=dev \
TERMTEAM_WEBUI_DEV_PROXY_URL="http://webui:15173" \
TERMTEAM_WEBUI_DIST_DIR="../webui/dist" \
TERMTEAM_TMUX_SOCKET="tt_e2e" \
TERMTEAM_TRACE_STREAM=1 \
go run ./cmd/termteam 2>&1 | tee /workspace/logs/cli.log
