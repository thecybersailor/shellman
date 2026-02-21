#!/usr/bin/env bash
set -euo pipefail
mkdir -p /workspace/logs
cd /workspace
node /workspace/scripts/e2e/docker/mock_worker.mjs 2>&1 | tee /workspace/logs/worker.log
