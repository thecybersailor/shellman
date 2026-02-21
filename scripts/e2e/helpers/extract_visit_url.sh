#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 1 ]]; then
  echo "usage: $0 <cli_log_file>" >&2
  exit 1
fi

log_file="$1"
if [[ ! -f "$log_file" ]]; then
  echo "log file not found: $log_file" >&2
  exit 1
fi

visit_url="$(sed -n 's/^visit_url=//p' "$log_file" | tail -n1)"
if [[ -z "$visit_url" ]]; then
  echo "visit_url not found in $log_file" >&2
  exit 1
fi

echo "$visit_url"
