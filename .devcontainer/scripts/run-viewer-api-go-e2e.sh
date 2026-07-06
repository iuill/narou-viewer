#!/usr/bin/env sh

set -eu

prebuilt_binary="${VIEWER_API_GO_E2E_BINARY_PATH:-}"

if [ -n "${prebuilt_binary}" ]; then
  if [ ! -x "${prebuilt_binary}" ]; then
    echo "[viewer-api-go-e2e] prebuilt binary is not executable: ${prebuilt_binary}" >&2
    exit 1
  fi

  echo "[viewer-api-go-e2e] starting prebuilt ${prebuilt_binary}"
  exec "${prebuilt_binary}"
fi

if ! command -v go >/dev/null 2>&1; then
  echo "[viewer-api-go-e2e] go is required when VIEWER_API_GO_E2E_BINARY_PATH is not set" >&2
  exit 1
fi

exec go run ./cmd/viewer-api
