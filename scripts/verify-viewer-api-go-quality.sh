#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${repo_root}/apps/viewer-api-go"

files="$(gofmt -l ./cmd ./internal)"
if [ -n "${files}" ]; then
  printf '%s\n' "${files}"
  exit 1
fi

coverage_threshold="${VIEWER_API_GO_COVERAGE_THRESHOLD:-89.6}"
coverage_profile="${VIEWER_API_GO_COVERAGE_PROFILE:-${TMPDIR:-/tmp}/viewer-api-go-internal.cover}"
coverage_coverpkg="${VIEWER_API_GO_COVERPKG:-./internal/...}"
coverage_args=()
if [ -n "${coverage_coverpkg}" ]; then
  coverage_args=("-coverpkg=${coverage_coverpkg}")
fi

go test ./internal/... "${coverage_args[@]}" -coverprofile="${coverage_profile}"

coverage_total="$(
  go tool cover -func="${coverage_profile}" |
    awk '/^total:/ { gsub(/%/, "", $3); print $3 }'
)"

if [ -z "${coverage_total}" ]; then
  echo "viewer-api-go coverage total could not be read" >&2
  exit 1
fi

awk -v actual="${coverage_total}" -v threshold="${coverage_threshold}" 'BEGIN { exit (actual + 0 >= threshold + 0) ? 0 : 1 }' || {
  printf 'viewer-api-go internal coverage %.1f%% is below %.1f%%\n' "${coverage_total}" "${coverage_threshold}" >&2
  exit 1
}

printf 'viewer-api-go internal coverage %.1f%% >= %.1f%%\n' "${coverage_total}" "${coverage_threshold}"

go test ./cmd/...
