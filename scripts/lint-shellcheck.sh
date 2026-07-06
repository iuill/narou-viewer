#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${script_dir}/.." && pwd)"

search_roots=(
  "${repo_root}/scripts"
  "${repo_root}/.devcontainer"
)

if [ -d "${repo_root}/ops" ]; then
  search_roots+=("${repo_root}/ops")
fi

mapfile -d '' -t files < <(find "${search_roots[@]}" -type f -name "*.sh" -print0)
if [ -f "${repo_root}/scripts/serena-query" ]; then
  files+=("${repo_root}/scripts/serena-query")
fi

if [ "${#files[@]}" -eq 0 ]; then
  exit 0
fi

platform="$(uname -s)"
arch="$(uname -m)"

case "${platform}/${arch}" in
  Linux/x86_64)
    shellcheck_bin="${repo_root}/node_modules/shellcheck-binaries/binaries/linux/x64/shellcheck"
    ;;
  Linux/aarch64 | Linux/arm64)
    shellcheck_bin="${repo_root}/node_modules/shellcheck-binaries/binaries/linux/arm64/shellcheck"
    ;;
  Darwin/x86_64)
    shellcheck_bin="${repo_root}/node_modules/shellcheck-binaries/binaries/darwin/x64/shellcheck"
    ;;
  Darwin/arm64)
    shellcheck_bin="${repo_root}/node_modules/shellcheck-binaries/binaries/darwin/arm64/shellcheck"
    ;;
  *)
    echo "Unsupported platform for shellcheck-binaries: ${platform}/${arch}" >&2
    exit 1
    ;;
esac

"${shellcheck_bin}" -x -S warning -e SC2034 "${files[@]}"
