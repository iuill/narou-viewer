#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${script_dir}/playwright-runner-common.sh"

workspace_dir="${PLAYWRIGHT_WORKSPACE_DIR:-$(pwd)}"
stage_dir="${PLAYWRIGHT_STAGE_DIR:-$(mktemp -d "${TMPDIR:-/tmp}/playwright-local.XXXXXX")}"
stage_report_dir="${stage_dir}/playwright-report"
stage_test_results_dir="${stage_dir}/test-results"
workspace_report_dir="${workspace_dir}/playwright-report"
workspace_test_results_dir="${workspace_dir}/test-results"
fallback_workspace_report_dir="${workspace_dir}/.tmp/playwright-report"
fallback_workspace_test_results_dir="${workspace_dir}/.tmp/test-results"
cleanup_stage_dir=0

if [[ -z "${PLAYWRIGHT_STAGE_DIR:-}" ]]; then
  cleanup_stage_dir=1
fi

cleanup() {
  if [[ "${cleanup_stage_dir}" == "1" ]]; then
    rm -rf "${stage_dir}"
  fi
}
trap cleanup EXIT

resolve_workspace_artifact_dir() {
  local preferred="$1"
  local fallback="$2"
  local parent_dir
  parent_dir="$(dirname "${preferred}")"

  if [[ -e "${preferred}" && ! -w "${preferred}" ]]; then
    printf '%s\n' "${fallback}"
    return 0
  fi

  if [[ ! -e "${preferred}" && ! -w "${parent_dir}" ]]; then
    printf '%s\n' "${fallback}"
    return 0
  fi

  printf '%s\n' "${preferred}"
}

list_required_playwright_linux_packages() {
  local dry_run_output
  dry_run_output="$(
    cd "${workspace_dir}" &&
      ./node_modules/.bin/playwright install-deps --dry-run chromium webkit 2>/dev/null | tr '\n' ' ' | tr -s ' '
  )"

  local install_args
  install_args="$(printf '%s\n' "${dry_run_output}" | sed -n 's/.*apt-get install -y \([^"]*\)".*/\1/p')"
  install_args="${install_args#--no-install-recommends }"

  if [[ -z "${install_args}" ]]; then
    printf '%s\n' "Failed to resolve Playwright browser dependencies from 'playwright install-deps --dry-run'." >&2
    return 1
  fi

  printf '%s\n' "${install_args}" | tr ' ' '\n' | sed '/^$/d'
}

ensure_local_playwright_host_dependencies() {
  local os_name
  os_name="$(uname -s)"
  if [[ "${os_name}" != "Linux" ]]; then
    return 0
  fi

  local required_packages=()
  local missing_packages=()

  mapfile -t required_packages < <(list_required_playwright_linux_packages)
  if [[ ${#required_packages[@]} -eq 0 ]]; then
    printf '%s\n' "No Playwright browser dependency packages were resolved." >&2
    return 1
  fi

  for package_name in "${required_packages[@]}"; do
    if ! dpkg-query -W -f='${Status}' "${package_name}" 2>/dev/null | grep -q '^install ok installed$'; then
      missing_packages+=("${package_name}")
    fi
  done

  if [[ ${#missing_packages[@]} -eq 0 ]]; then
    return 0
  fi

  if [[ "${PLAYWRIGHT_SKIP_BROWSER_DEPS_INSTALL:-0}" == "1" ]]; then
    printf '%s\n' "Playwright browser dependencies are missing: ${missing_packages[*]}" >&2
    printf '%s\n' "Install them manually or unset PLAYWRIGHT_SKIP_BROWSER_DEPS_INSTALL." >&2
    return 1
  fi

  if ! command -v sudo >/dev/null 2>&1 || ! sudo -n true >/dev/null 2>&1; then
    printf '%s\n' "Playwright browser dependencies are missing and passwordless sudo is unavailable: ${missing_packages[*]}" >&2
    return 1
  fi

  printf '%s\n' "Installing missing Playwright browser dependencies: ${missing_packages[*]}"
  sudo env DEBIAN_FRONTEND=noninteractive apt-get update
  sudo env DEBIAN_FRONTEND=noninteractive apt-get install -y "${missing_packages[@]}"
}

ensure_local_playwright_browsers() {
  if bun -e '
const fs = require("node:fs");
const workspaceDir = process.argv[1];
const { chromium, webkit } = require(`${workspaceDir}/node_modules/@playwright/test`);

for (const browserType of [chromium, webkit]) {
  if (!fs.existsSync(browserType.executablePath())) {
    process.exit(1);
  }
}
' "${workspace_dir}" >/dev/null 2>&1; then
    return 0
  fi

  if [[ "${PLAYWRIGHT_SKIP_BROWSER_INSTALL:-0}" == "1" ]]; then
    printf '%s\n' "Playwright browser binaries are missing. Run 'bun run playwright install chromium webkit' or unset PLAYWRIGHT_SKIP_BROWSER_INSTALL." >&2
    return 1
  fi

  printf '%s\n' "Playwright browser binaries are missing. Installing chromium and webkit..."
  (
    cd "${workspace_dir}"
    ./node_modules/.bin/playwright install chromium webkit
  )
}

run_playwright_for_project() {
  local project_name="$1"
  shift
  local playwright_args=("$@")

  (
    cd "${workspace_dir}"
    PLAYWRIGHT_HTML_OUTPUT_DIR="${stage_report_dir}/${project_name}" \
      PLAYWRIGHT_RESULTS_ROOT_DIR="${stage_test_results_dir}" \
      ./node_modules/.bin/playwright test --project="${project_name}" "${playwright_args[@]}"
  )
}

sync_playwright_artifacts() {
  local resolved_report_dir
  local resolved_test_results_dir

  resolved_report_dir="$(resolve_workspace_artifact_dir "${workspace_report_dir}" "${fallback_workspace_report_dir}")"
  resolved_test_results_dir="$(resolve_workspace_artifact_dir "${workspace_test_results_dir}" "${fallback_workspace_test_results_dir}")"

  if [[ "${resolved_report_dir}" != "${workspace_report_dir}" || "${resolved_test_results_dir}" != "${workspace_test_results_dir}" ]]; then
    printf '%s\n' "Playwright artifacts will be written under .tmp because workspace report directories are not writable."
  fi

  rm -rf "${resolved_report_dir}" "${resolved_test_results_dir}"

  if [[ -d "${stage_report_dir}" ]]; then
    mkdir -p "$(dirname "${resolved_report_dir}")"
    cp -R "${stage_report_dir}" "${resolved_report_dir}"
  fi

  if [[ -d "${stage_test_results_dir}" ]]; then
    mkdir -p "$(dirname "${resolved_test_results_dir}")"
    cp -R "${stage_test_results_dir}" "${resolved_test_results_dir}"
  fi

  printf '%s\n' "Playwright report: ${resolved_report_dir}"
  printf '%s\n' "Playwright test results: ${resolved_test_results_dir}"
}

export PLAYWRIGHT_BASE_URL="${PLAYWRIGHT_BASE_URL:-$(resolve_default_playwright_base_url)}"

mkdir -p "${stage_dir}"
wait_for_playwright_base_url
ensure_local_playwright_host_dependencies
ensure_local_playwright_browsers

project_filters=()
forward_args=()

while [[ $# -gt 0 ]]; do
  case "$1" in
    --project)
      shift
      if [[ $# -eq 0 ]]; then
        printf '%s\n' "Missing value for --project" >&2
        exit 1
      fi
      project_filters+=("$1")
      ;;
    --project=*)
      project_filters+=("${1#--project=}")
      ;;
    *)
      forward_args+=("$1")
      ;;
  esac
  shift
done

if [[ ${#project_filters[@]} -eq 0 ]]; then
  mapfile -t project_filters < <(list_target_names)
fi

declare -A seen_projects=()
unique_project_filters=()
for project_name in "${project_filters[@]}"; do
  if [[ -n "${project_name}" && -z "${seen_projects[${project_name}]:-}" ]]; then
    unique_project_filters+=("${project_name}")
    seen_projects["${project_name}"]=1
  fi
done

if [[ ${#unique_project_filters[@]} -eq 0 ]]; then
  printf '%s\n' "No Playwright projects were resolved. Pass --project explicitly or check playwright.targets.ts." >&2
  exit 1
fi

set +e
playwright_exit_code=0
for project_name in "${unique_project_filters[@]}"; do
  run_playwright_for_project "${project_name}" "${forward_args[@]}"
  project_exit_code=$?
  if [[ ${project_exit_code} -ne 0 ]]; then
    playwright_exit_code=${project_exit_code}
  fi
done
set -e

sync_playwright_artifacts
exit "${playwright_exit_code}"
