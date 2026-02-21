#!/usr/bin/env bash
set -euo pipefail

out="$(cd cli && go run ./cmd/termteam-diffsim fullscreen_redraw)"
resets="$(echo "$out" | sed -n 's/.*resets=\([0-9][0-9]*\).*/\1/p')"

if [[ -z "$resets" ]]; then
  echo "failed to parse resets from: $out" >&2
  exit 1
fi

if [[ "$resets" -gt 0 ]]; then
  echo "reset regression: $out" >&2
  exit 1
fi

echo "ok: $out"
