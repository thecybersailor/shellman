#!/usr/bin/env bash
set -euo pipefail

token="${1:-T00}"
lines="${2:-5000}"

printf '\033[0m\033[H\033[2J'
printf 'OpenAI Codex (v0.104.0)\n'
printf 'Find and fix a bug in @filename\n'
printf '? for shortcuts\n'

i=1
while [[ "$i" -le "$lines" ]]; do
  printf 'PANE_%s_LINE_%05d\n' "$token" "$i"
  i=$((i + 1))
done
