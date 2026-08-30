#!/usr/bin/env bash

set -euo pipefail

install_dir="${1:?Codex standalone installation directory path is required}"
codex_binary="${install_dir}/current/bin/codex"

if [ ! -f "${codex_binary}" ]; then
  printf '%s\n' \
    "Codex standalone binary was not found: ${codex_binary}" \
    'Install Codex on the host before starting this Dev Container:' \
    '  curl -fsSL https://chatgpt.com/codex/install.sh | sh' >&2
  exit 1
fi

if [ ! -x "${codex_binary}" ]; then
  echo "Codex standalone binary is not executable: ${codex_binary}" >&2
  exit 1
fi

if ! "${codex_binary}" --version >/dev/null; then
  echo "Codex standalone binary could not run on the host: ${codex_binary}" >&2
  exit 1
fi
