#!/usr/bin/env bash
set -euo pipefail

bash scripts/e2e/run_full_real_e2e.sh || true
[[ -f /tmp/shellman-e2e/worker.log ]]
[[ -f /tmp/shellman-e2e/cli.log ]]
[[ -f /tmp/shellman-e2e/webui.log ]]
[[ -f /tmp/shellman-e2e/playwright.log ]]
