#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

echo "[e2e-guard-check] scanning docs/plans for forbidden e2e direct-run commands..."

PATTERN='playwright[[:space:]]+test[[:space:]]+webui/e2e|playwright[[:space:]]+test[[:space:]]+e2e/|bash[[:space:]]+scripts/e2e/run_full_real_e2e\.sh|npm[[:space:]]+run[[:space:]]+test:e2e:real'

MATCHES="$(rg -n --pcre2 "$PATTERN" docs/plans || true)"
if [[ -n "$MATCHES" ]]; then
  echo "[e2e-guard-check] ERROR: found forbidden e2e direct-run command(s):" >&2
  echo "$MATCHES" >&2
  echo "[e2e-guard-check] Please use: make e2e-ui-docker" >&2
  exit 1
fi

echo "[e2e-guard-check] passed."
