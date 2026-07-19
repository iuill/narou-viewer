#!/usr/bin/env bash

set -euo pipefail

config_dir="${1:?GitHub CLI config directory path is required}"

umask 077
mkdir -p "${config_dir}"

if [ ! -d "${config_dir}" ]; then
  echo "GitHub CLI config path is not a directory: ${config_dir}" >&2
  exit 1
fi

chmod 700 "${config_dir}"
