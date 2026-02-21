#!/usr/bin/env bash
set -euo pipefail

if [[ "${1:-}" == "--help" ]]; then
  echo "usage: wait_ready.sh"
  exit 0
fi

[[ -n "${OPENAI_ENDPOINT:-}" ]] || { echo "wait_ready: OPENAI_ENDPOINT is empty" >&2; exit 2; }
[[ -n "${OPENAI_MODEL:-}" ]] || { echo "wait_ready: OPENAI_MODEL is empty" >&2; exit 2; }
[[ -n "${OPENAI_API_KEY:-}" ]] || { echo "wait_ready: OPENAI_API_KEY is empty" >&2; exit 2; }

DEFAULT_RETRIES="${E2E_READY_RETRIES:-120}"
WEBUI_RETRIES="${E2E_WEBUI_READY_RETRIES:-${DEFAULT_RETRIES}}"
CLI_RETRIES="${E2E_CLI_READY_RETRIES:-600}"

wait_http() {
  local url="$1"
  local retries="${2:-$DEFAULT_RETRIES}"
  for _ in $(seq 1 "$retries"); do
    if curl -fsS "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.5
  done
  return 1
}

wait_http_with_host() {
  local url="$1"
  local host="$2"
  local retries="${3:-$DEFAULT_RETRIES}"
  for _ in $(seq 1 "$retries"); do
    if curl -fsS -H "Host: ${host}" "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.5
  done
  return 1
}

wait_post() {
  local url="$1"
  local retries="${2:-$DEFAULT_RETRIES}"
  for _ in $(seq 1 "$retries"); do
    if curl -fsS -X POST "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.5
  done
  return 1
}

wait_post "http://worker:8787/api/register" || { echo "worker not ready" >&2; exit 1; }
wait_http_with_host "http://webui:15173" "localhost" "$WEBUI_RETRIES" || { echo "webui not ready" >&2; exit 1; }
wait_http "http://cli:4621" "$CLI_RETRIES" || { echo "cli not ready" >&2; exit 1; }

echo "ready"
