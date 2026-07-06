#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
port="${VIEWER_API_GO_CONTRACT_PORT:-18081}"
fetcher_port="${VIEWER_API_GO_CONTRACT_FETCHER_PORT:-18082}"
base_url="http://127.0.0.1:${port}"
fetcher_base_url="http://127.0.0.1:${fetcher_port}"
contract_fetcher_base_url="${VIEWER_API_GO_CONTRACT_FETCHER_BASE_URL:-${fetcher_base_url}}"
data_dir="${VIEWER_API_GO_CONTRACT_DATA_DIR:-${repo_root}/data_e2e}"
binary_path="${RUNNER_TEMP:-/tmp}/viewer-api-go-contract"
fetcher_binary_path="${RUNNER_TEMP:-/tmp}/novel-fetcher-contract"
log_path="${RUNNER_TEMP:-/tmp}/viewer-api-go-contract.log"
fetcher_log_path="${RUNNER_TEMP:-/tmp}/novel-fetcher-contract.log"
api_pid=""
fetcher_pid=""

cleanup() {
  if [[ -n "${api_pid}" ]]; then
    kill "${api_pid}" 2>/dev/null || true
    wait "${api_pid}" 2>/dev/null || true
  fi
  if [[ -n "${fetcher_pid}" ]]; then
    kill "${fetcher_pid}" 2>/dev/null || true
    wait "${fetcher_pid}" 2>/dev/null || true
  fi
}
trap cleanup EXIT

cd "${repo_root}"

bun run e2e:fixture:init
bun run e2e:state:reset

(cd services/novel-fetcher && go build -o "${fetcher_binary_path}" ./cmd/novel-fetcher)
(cd apps/viewer-api-go && go build -o "${binary_path}" ./cmd/viewer-api)

NOVEL_FETCHER_HOST="127.0.0.1" \
NOVEL_FETCHER_PORT="${fetcher_port}" \
NOVEL_FETCHER_DATA_DIR="${data_dir}/novel-fetcher" \
  "${fetcher_binary_path}" >"${fetcher_log_path}" 2>&1 &
fetcher_pid="$!"

fetcher_ready=0
for _ in {1..60}; do
  if curl -fsS "${fetcher_base_url}/health" >/dev/null; then
    fetcher_ready=1
    break
  fi
  if ! kill -0 "${fetcher_pid}" 2>/dev/null; then
    cat "${fetcher_log_path}" >&2
    exit 1
  fi
  sleep 1
done
if [[ "${fetcher_ready}" != "1" ]]; then
  cat "${fetcher_log_path}" >&2
  exit 1
fi

VIEWER_API_GO_ADDR="127.0.0.1:${port}" \
VIEWER_API_DATA_DIR="${data_dir}" \
NOVEL_FETCHER_API_BASE_URL="${contract_fetcher_base_url}" \
  "${binary_path}" >"${log_path}" 2>&1 &
api_pid="$!"

for _ in {1..60}; do
  if curl -fsS "${base_url}/api/health" >/dev/null; then
    API_BASE_URL="${base_url}" \
    API_CONTRACT_MUTATING=1 \
    API_CONTRACT_REQUIRE_FIXTURE=1 \
      bun run test:api-contract
    exit 0
  fi
  if ! kill -0 "${api_pid}" 2>/dev/null; then
    cat "${log_path}" >&2
    exit 1
  fi
  sleep 1
done

cat "${log_path}" >&2
exit 1
