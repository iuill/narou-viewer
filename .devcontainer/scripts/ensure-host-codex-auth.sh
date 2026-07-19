#!/usr/bin/env bash

set -euo pipefail

auth_file="${1:?Codex auth file path is required}"
auth_dir="$(dirname "${auth_file}")"

umask 077
mkdir -p "${auth_dir}"

if [ ! -e "${auth_file}" ]; then
  printf '{}\n' >"${auth_file}"
fi

if [ ! -f "${auth_file}" ]; then
  echo "Codex auth path is not a regular file: ${auth_file}" >&2
  exit 1
fi

chmod 600 "${auth_file}"
