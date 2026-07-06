#!/usr/bin/env bash

set -euo pipefail

if [[ "$#" -lt 2 ]]; then
  printf '%s\n' "Usage: wait-for-path-and-run.sh <path> <command> [args...]" >&2
  exit 1
fi

ready_path="$1"
shift

wait_seconds=0
log_interval_seconds=5

while [[ ! -e "${ready_path}" ]]; do
  if (( wait_seconds == 0 )); then
    printf '%s\n' "Waiting for workspace dependency: ${ready_path}"
  elif (( wait_seconds % log_interval_seconds == 0 )); then
    printf '%s\n' "Still waiting for workspace dependency: ${ready_path}"
  fi

  sleep 1
  wait_seconds=$((wait_seconds + 1))
done

exec "$@"
