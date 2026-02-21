#!/usr/bin/env bash
set -euo pipefail
mkdir -p /workspace/logs /workspace/logs/playwright-artifacts
mkdir -p /workspace/tmp/e2e-repo

if [[ "${1:-}" == "--dry-run" ]]; then
  echo "dry-run: wait_ready + playwright"
  exit 0
fi

[[ -n "${OPENAI_ENDPOINT:-}" ]] || { echo "missing OPENAI_ENDPOINT" | tee -a /workspace/logs/playwright.log; exit 2; }
[[ -n "${OPENAI_MODEL:-}" ]] || { echo "missing OPENAI_MODEL" | tee -a /workspace/logs/playwright.log; exit 2; }
[[ -n "${OPENAI_API_KEY:-}" ]] || { echo "missing OPENAI_API_KEY" | tee -a /workspace/logs/playwright.log; exit 2; }

# e2e seed API requires repo_root to be a valid git work tree in cli container.
if [[ ! -d /workspace/tmp/e2e-repo/.git ]]; then
  git -C /workspace/tmp/e2e-repo init >/dev/null
fi

bash /workspace/scripts/e2e/docker/wait_ready.sh

cd /workspace/webui

export E2E_VISIT_URL="http://cli:4621"
export E2E_API_BASE="http://cli:4621"
export E2E_REPO_ROOT="/workspace/tmp/e2e-repo"

npx playwright test e2e/full-real-tmux.spec.ts 2>&1 | tee /workspace/logs/playwright.log
