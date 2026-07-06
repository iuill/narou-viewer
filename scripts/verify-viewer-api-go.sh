#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

bash "${repo_root}/scripts/verify-viewer-api-go-quality.sh"
bash "${repo_root}/scripts/build-viewer-api-go.sh"
