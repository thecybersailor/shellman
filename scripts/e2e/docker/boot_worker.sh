#!/usr/bin/env bash
set -euo pipefail
mkdir -p /workspace/logs
cd /workspace/edge/cloudflare
npm install --include=dev
npx wrangler dev --local --ip 0.0.0.0 --port 8787 2>&1 | tee /workspace/logs/worker.log
