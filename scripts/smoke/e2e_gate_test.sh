#!/usr/bin/env bash
set -euo pipefail

out="$(bash scripts/smoke/ci_local_check.sh)"
echo "$out" | grep -q "full-real-tmux.spec.ts"
