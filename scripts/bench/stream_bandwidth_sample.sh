#!/usr/bin/env bash
set -euo pipefail

LOG_FILE="${1:-/tmp/termteam_ws.log}"
DURATION_SECONDS="${2:-30}"

if [[ ! -f "$LOG_FILE" ]]; then
  echo "log file not found: $LOG_FILE" >&2
  exit 1
fi

BYTES=$(wc -c < "$LOG_FILE" | tr -d ' ')

awk -v b="$BYTES" -v s="$DURATION_SECONDS" 'BEGIN {
  if (s <= 0) {
    print "duration must be > 0";
    exit 1;
  }
  printf("bytes_total=%d\n", b);
  printf("duration_seconds=%d\n", s);
  printf("avg_bytes_per_sec=%.2f\n", b / s);
}'
