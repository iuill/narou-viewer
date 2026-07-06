#!/usr/bin/env sh

set -eu

prebuilt_binary="${NOVEL_FETCHER_E2E_BINARY_PATH:-}"

if [ -n "${prebuilt_binary}" ]; then
  if [ ! -x "${prebuilt_binary}" ]; then
    echo "[novel-fetcher-e2e] prebuilt binary is not executable: ${prebuilt_binary}" >&2
    exit 1
  fi

  echo "[novel-fetcher-e2e] starting prebuilt ${prebuilt_binary}"
  exec "${prebuilt_binary}"
fi

if ! command -v bash >/dev/null 2>&1; then
  echo "[novel-fetcher-e2e] bash is required when NOVEL_FETCHER_E2E_BINARY_PATH is not set" >&2
  exit 1
fi

exec bash /workspace/.devcontainer/scripts/run-novel-fetcher-dev.sh
