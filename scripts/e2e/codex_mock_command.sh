#!/usr/bin/env bash
set -euo pipefail

token="${CODEX_MOCK_TOKEN:-T00}"
lines="${CODEX_MOCK_LINES:-5000}"
rows="$(tput lines 2>/dev/null || echo 24)"

printf '\033[0m\033[H\033[2J'
printf 'OpenAI Codex (v0.104.0)\n'
printf 'Find and fix a bug in @filename\n'
printf '? for shortcuts\n'

i=1
while [[ "$i" -le "$lines" ]]; do
  printf 'CODEX_%s_LINE_%05d\n' "$token" "$i"
  i=$((i + 1))
done

# Keep initial long output visible briefly before entering repaint loop.
sleep 1

# Keep running and emit codex-style repaint/reset frames periodically.
while true; do
  printf '\033[0m\033[H\033[2J'
  printf 'OpenAI Codex (v0.104.0)\n'
  printf 'Find and fix a bug in @filename\n'
  printf '? for shortcuts\n'
  printf '\033[%s;1H' "$rows"
  printf '> '
  sleep 0.25
done
