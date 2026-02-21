#!/usr/bin/env bash
set -euo pipefail
mkdir -p /workspace/logs
cd /workspace/webui
npm install --include=dev
VITE_WORKER_ORIGIN="http://worker:8787" npm run dev -- --host 0.0.0.0 --port 15173 2>&1 | tee /workspace/logs/webui.log
