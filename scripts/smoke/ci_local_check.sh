#!/usr/bin/env bash
set -euo pipefail

cd cli && go test ./...
cd ..
bash scripts/bench/check_diffsim_reset_ratio.sh
cd edge/cloudflare && npm test
cd ../../webui && npm test
cd ..
echo "run local ws parity smoke"
bash scripts/smoke/shellman_local_web_smoke.sh
if [[ "${RUN_FULL_REAL_E2E:-0}" == "1" ]]; then
  echo "run e2e: full-real-tmux.spec.ts"
  bash scripts/e2e/run_full_real_e2e.sh
else
  echo "skip full-real e2e (set RUN_FULL_REAL_E2E=1 to enable)"
fi
