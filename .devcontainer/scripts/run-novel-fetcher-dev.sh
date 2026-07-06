#!/usr/bin/env bash

set -euo pipefail

watch_interval="${NOVEL_FETCHER_DEV_WATCH_INTERVAL_SECONDS:-1}"
binary_dir="${NOVEL_FETCHER_DEV_BINARY_DIR:-/tmp/novel-fetcher-dev}"
binary_path="${binary_dir}/novel-fetcher"
tmp_binary_path="${binary_dir}/novel-fetcher.next"
pid=""

mkdir -p "${binary_dir}" "${GOCACHE:-/tmp/go-build-cache}" "${GOMODCACHE:-/tmp/go-mod-cache}"

fingerprint_sources() {
  find . \
    -type f \
    \( -name "*.go" -o -name "go.mod" -o -name "go.sum" \) \
    -printf "%T@ %p\n" |
    sort |
    sha256sum |
    awk '{print $1}'
}

stop_running_binary() {
  if [ -n "${pid}" ] && kill -0 "${pid}" 2>/dev/null; then
    kill "${pid}"
    wait "${pid}" 2>/dev/null || true
  fi
  pid=""
}

build_binary() {
  echo "[novel-fetcher-dev] building ./cmd/novel-fetcher"
  CGO_ENABLED=0 go build -buildvcs=false -trimpath -o "${tmp_binary_path}" ./cmd/novel-fetcher
  mv "${tmp_binary_path}" "${binary_path}"
}

start_binary() {
  echo "[novel-fetcher-dev] starting ${binary_path}"
  "${binary_path}" &
  pid="$!"
}

rebuild_and_restart() {
  if ! build_binary; then
    echo "[novel-fetcher-dev] build failed; keeping current process" >&2
    return 1
  fi

  stop_running_binary
  start_binary
}

cleanup() {
  stop_running_binary
}

trap cleanup EXIT INT TERM

build_binary
start_binary
last_fingerprint="$(fingerprint_sources)"

while true; do
  sleep "${watch_interval}"

  if [ -n "${pid}" ] && ! kill -0 "${pid}" 2>/dev/null; then
    wait "${pid}"
  fi

  next_fingerprint="$(fingerprint_sources)"
  if [ "${next_fingerprint}" != "${last_fingerprint}" ]; then
    echo "[novel-fetcher-dev] source change detected"
    rebuild_and_restart || true
    last_fingerprint="${next_fingerprint}"
  fi
done
