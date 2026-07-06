#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
runner="${E2E_RUNNER:-auto}"

docker_is_available() {
  command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1
}

is_container_environment() {
  [[ -f "/.dockerenv" || "${CODESPACES:-}" == "true" ]]
}

ensure_supported_local_runner_environment() {
  if [[ -n "${PLAYWRIGHT_BASE_URL:-}" ]]; then
    return 0
  fi

  if ! is_container_environment; then
    return 0
  fi

  if docker_is_available; then
    return 0
  fi

  cat >&2 <<'EOF'
Docker is unavailable in this container, and the local Playwright runner has no supported default PLAYWRIGHT_BASE_URL here.
Set PLAYWRIGHT_BASE_URL explicitly and rerun, or use the container runner from an environment where Docker is available.
If we decide to support dockerless-container local runs later, update scripts/e2e-test.sh and scripts/playwright-runner-common.sh together.
EOF
  return 1
}

should_use_container_runner() {
  case "${runner}" in
    container)
      return 0
      ;;
    local)
      return 1
      ;;
    auto)
      if [[ -n "${PLAYWRIGHT_BASE_URL:-}" ]]; then
        return 1
      fi

      if docker_is_available; then
        return 0
      fi

      return 1
      ;;
    *)
      printf '%s\n' "Unsupported E2E_RUNNER value: ${runner}" >&2
      exit 1
      ;;
  esac
}

should_bootstrap_e2e_services() {
  if [[ "${E2E_SKIP_SERVICES_UP:-0}" == "1" || -n "${PLAYWRIGHT_BASE_URL:-}" ]]; then
    return 1
  fi

  if docker_is_available; then
    return 0
  fi

  if is_container_environment; then
    printf '%s\n' "Docker is unavailable in this container; skipping e2e:services:up and using the local Playwright runner." >&2
    return 1
  fi

  return 0
}

if [[ "${E2E_RESET_STATE:-1}" != "0" ]]; then
  bun run e2e:state:reset
fi

if should_use_container_runner; then
  exec env E2E_RESET_STATE=0 bash "${script_dir}/e2e-test-container.sh" "$@"
fi

ensure_supported_local_runner_environment

if should_bootstrap_e2e_services; then
  bun run e2e:services:up
fi

exec bash "${script_dir}/run-playwright-local.sh" "$@"
