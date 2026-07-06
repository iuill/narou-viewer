#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
modules=(
  "apps/viewer-api-go"
  "services/novel-fetcher"
)

for module in "${modules[@]}"; do
  echo "Running govulncheck for ${module}"
  (
    cd "${repo_root}/${module}"
    go run golang.org/x/vuln/cmd/govulncheck@v1.2.0 ./...
  )
done
