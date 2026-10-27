#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
modules=(
  "apps/viewer-api-go"
  "services/novel-fetcher"
)

status=0
for module in "${modules[@]}"; do
  echo "Running govulncheck for ${module}"
  if ! (
    cd "${repo_root}/${module}" || exit 1
    go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...
  ); then
    status=1
  fi
done

exit "$status"
