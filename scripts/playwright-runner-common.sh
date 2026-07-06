#!/usr/bin/env bash

set -euo pipefail

common_script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
playwright_repo_root_dir="$(cd "${common_script_dir}/.." && pwd)"

read_devcontainer_env_value() {
  local key="$1"
  local env_file="${playwright_repo_root_dir}/.devcontainer/.env"

  if [[ ! -f "${env_file}" ]]; then
    return 1
  fi

  local line value
  while IFS= read -r line || [[ -n "${line}" ]]; do
    line="${line#"${line%%[![:space:]]*}"}"
    if [[ "${line}" == "${key}="* ]]; then
      value="${line#*=}"
      value="${value%$'\r'}"
      value="${value#"${value%%[![:space:]]*}"}"

      case "${value}" in
        \"*)
          value="${value#\"}"
          value="${value%%\"*}"
          ;;
        \'*)
          value="${value#\'}"
          value="${value%%\'*}"
          ;;
        *)
          value="${value%%[[:space:]]#*}"
          value="${value%"${value##*[![:space:]]}"}"
          ;;
      esac

      printf '%s\n' "${value}"
      return 0
    fi
  done <"${env_file}"

  return 1
}

resolve_default_playwright_base_url() {
  if [[ -n "${PLAYWRIGHT_BASE_URL:-}" ]]; then
    printf '%s\n' "${PLAYWRIGHT_BASE_URL}"
    return 0
  fi

  if [[ -f "/.dockerenv" || "${CODESPACES:-}" == "true" ]]; then
    printf '%s\n' "http://viewer-web-e2e:15173"
    return 0
  fi

  local host_port="${VIEWER_WEB_E2E_HOST_PORT:-}"
  if [[ -z "${host_port}" ]]; then
    host_port="$(read_devcontainer_env_value VIEWER_WEB_E2E_HOST_PORT || true)"
  fi
  printf 'http://127.0.0.1:%s\n' "${host_port:-15173}"
}

wait_for_playwright_base_url() {
  local raw_url="${PLAYWRIGHT_BASE_URL:-}"
  if [[ -z "${raw_url}" || "${raw_url}" != http*://* ]]; then
    return 0
  fi

  if ! command -v curl >/dev/null 2>&1; then
    printf '%s\n' "curl is required to probe PLAYWRIGHT_BASE_URL." >&2
    return 1
  fi

  local wait_seconds=0
  local max_wait_seconds="${PLAYWRIGHT_BASE_URL_WAIT_SECONDS:-60}"
  local curl_args=(
    --silent
    --show-error
    --output /dev/null
    --write-out '%{http_code}'
    --max-time 5
  )

  while true; do
    local status_code
    if status_code="$(curl "${curl_args[@]}" "${raw_url}" 2>/dev/null)"; then
      if [[ "${status_code}" =~ ^[0-9]+$ ]] && [[ "${status_code}" != "000" ]] && (( status_code < 500 )); then
        printf '%s\n' "Confirmed PLAYWRIGHT_BASE_URL is reachable: ${raw_url}"
        return 0
      fi
    else
      status_code=""
    fi

    if (( wait_seconds == 0 )); then
      printf '%s\n' "Waiting for PLAYWRIGHT_BASE_URL to become reachable: ${raw_url}"
    elif (( wait_seconds % 5 == 0 )); then
      printf '%s\n' "Still waiting for PLAYWRIGHT_BASE_URL: ${raw_url}"
    fi

    if (( wait_seconds >= max_wait_seconds )); then
      printf '%s\n' "Timed out waiting for PLAYWRIGHT_BASE_URL: ${raw_url}" >&2
      return 1
    fi

    sleep 1
    wait_seconds=$((wait_seconds + 1))
  done
}

list_target_names() {
  local targets_file="${PLAYWRIGHT_TARGETS_FILE:-${playwright_repo_root_dir}/playwright.targets.ts}"

  if [[ ! -f "${targets_file}" ]]; then
    printf '%s\n' "Playwright targets file was not found: ${targets_file}" >&2
    return 1
  fi

  if command -v rg >/dev/null 2>&1; then
    rg --no-filename --only-matching 'name:\s*"[^"]+"' "${targets_file}" | sed -E 's/.*"([^"]+)"/\1/'
    return 0
  fi

  grep -oE 'name:[[:space:]]*"[^"]+"' "${targets_file}" | sed -E 's/.*"([^"]+)"/\1/'
}
