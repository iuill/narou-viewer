#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${repo_root}/apps/viewer-api-go"

output_path="${VIEWER_API_GO_BUILD_OUTPUT:-${TMPDIR:-/tmp}/viewer-api-go-check}"
if [[ "${output_path}" != /* ]]; then
  output_path="${repo_root}/${output_path}"
fi
mkdir -p "$(dirname "${output_path}")"

go build -o "${output_path}" ./cmd/viewer-api
