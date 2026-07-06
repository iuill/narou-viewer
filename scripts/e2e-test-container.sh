#!/usr/bin/env bash

set -euo pipefail

if [[ ! -t 1 || ! -t 2 ]]; then
  if [[ "${E2E_PTY_WRAPPED:-0}" != "1" && "${CI:-}" != "true" && "${GITHUB_ACTIONS:-}" != "true" ]] && command -v script >/dev/null 2>&1; then
    quoted_cwd="$(printf '%q' "$(pwd)")"
    quoted_script="$(printf '%q' "$0")"
    quoted_args=""
    if [[ $# -gt 0 ]]; then
      printf -v quoted_args ' %q' "$@"
    fi

    exec script -qefc "cd ${quoted_cwd} && E2E_PTY_WRAPPED=1 bash ${quoted_script}${quoted_args}" /dev/null
  fi
fi

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
compose_script="${script_dir}/e2e-compose.sh"
playwright_artifacts_owner="${PLAYWRIGHT_ARTIFACTS_OWNER:-$(id -u):$(id -g)}"
compose_run_tty_args=()

if [[ ! -t 1 || ! -t 2 ]]; then
  compose_run_tty_args+=("-T")
fi

if [[ "${E2E_RESET_STATE:-1}" != "0" ]]; then
  bun run e2e:state:reset
fi

if [[ "${E2E_SKIP_SERVICES_UP:-0}" != "1" ]]; then
  bun run e2e:services:up
fi

exec bash "${compose_script}" --profile e2e run "${compose_run_tty_args[@]}" --rm --no-deps \
  -e PLAYWRIGHT_ARTIFACTS_OWNER="${playwright_artifacts_owner}" \
  -e CI \
  -e GITHUB_ACTIONS \
  -e PLAYWRIGHT_TRACE_MODE \
  -e PLAYWRIGHT_SCREENSHOT_MODE \
  playwright-e2e bash ./scripts/run-playwright-e2e.sh "$@"
