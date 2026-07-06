#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${repo_root}/services/novel-fetcher"

files="$(gofmt -l .)"
if [ -n "${files}" ]; then
  printf '%s\n' "${files}"
  exit 1
fi

coverage_threshold="${NOVEL_FETCHER_COVERAGE_THRESHOLD:-85.0}"
coverage_profile="${NOVEL_FETCHER_COVERAGE_PROFILE:-${TMPDIR:-/tmp}/novel-fetcher-internal.cover}"

go test ./internal/... -coverprofile="${coverage_profile}"

coverage_total="$(
  go tool cover -func="${coverage_profile}" |
    awk '/^total:/ { gsub(/%/, "", $3); print $3 }'
)"

if [ -z "${coverage_total}" ]; then
  echo "novel-fetcher internal coverage total could not be read" >&2
  exit 1
fi

awk -v actual="${coverage_total}" -v threshold="${coverage_threshold}" 'BEGIN { exit (actual + 0 >= threshold + 0) ? 0 : 1 }' || {
  printf 'novel-fetcher internal coverage %.1f%% is below %.1f%%\n' "${coverage_total}" "${coverage_threshold}" >&2
  exit 1
}

printf 'novel-fetcher internal coverage %.1f%% >= %.1f%%\n' "${coverage_total}" "${coverage_threshold}"

go test ./cmd/...
go build -o /tmp/novel-fetcher-check ./cmd/novel-fetcher
