#!/usr/bin/env bash
set -euo pipefail

./scripts/smoke/mvp_local_smoke.sh --dry-run | grep -q "worker ok"
./scripts/smoke/mvp_local_smoke.sh --dry-run | grep -q "cli ok"
./scripts/smoke/mvp_local_smoke.sh --dry-run | grep -q "webui ok"
./scripts/smoke/mvp_local_smoke.sh --dry-run | grep -q "bridge ok"
