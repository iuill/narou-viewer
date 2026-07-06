#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${script_dir}/playwright-runner-common.sh"

stage_dir="${PLAYWRIGHT_STAGE_DIR:-/playwright-runtime}"
playwright_test_version="${PLAYWRIGHT_TEST_VERSION:-1.58.2}"
workspace_dir="${PLAYWRIGHT_WORKSPACE_DIR:-/workspace}"
workspace_report_dir="${workspace_dir}/playwright-report"
workspace_test_results_dir="${workspace_dir}/test-results"
stage_report_dir="${stage_dir}/playwright-report"
stage_test_results_dir="${stage_dir}/test-results"

run_playwright_for_project() {
  local project_name="$1"
  shift
  local playwright_args=("$@")

  printf '%s\n' "Running Playwright project: ${project_name}"
  PLAYWRIGHT_HTML_OUTPUT_DIR="${stage_report_dir}/${project_name}" \
    PLAYWRIGHT_RESULTS_ROOT_DIR="${stage_test_results_dir}" \
    ./node_modules/.bin/playwright test --project="${project_name}" "${playwright_args[@]}"
  local project_exit_code=$?
  printf '%s\n' "Finished Playwright project: ${project_name} (exit=${project_exit_code})"
  return "${project_exit_code}"
}

sync_playwright_artifacts() {
  rm -rf "${workspace_report_dir}" "${workspace_test_results_dir}"

  if [[ -d "${stage_report_dir}" ]]; then
    cp -R "${stage_report_dir}" "${workspace_report_dir}"
  fi

  if [[ -d "${stage_test_results_dir}" ]]; then
    cp -R "${stage_test_results_dir}" "${workspace_test_results_dir}"
  fi

  if [[ -n "${PLAYWRIGHT_ARTIFACTS_OWNER:-}" ]]; then
    chown -R "${PLAYWRIGHT_ARTIFACTS_OWNER}" "${workspace_report_dir}" "${workspace_test_results_dir}" 2>/dev/null || true
  fi
}

mkdir -p "${stage_dir}"
wait_for_playwright_base_url

installed_version=""
if [[ -f "${stage_dir}/node_modules/@playwright/test/package.json" ]]; then
  installed_version="$(
    bun -e 'const fs = require("node:fs"); const packagePath = process.argv[1]; console.log(JSON.parse(fs.readFileSync(packagePath, "utf8")).version);' \
      "${stage_dir}/node_modules/@playwright/test/package.json" 2>/dev/null || true
  )"
fi

if [[ "${installed_version}" != "${playwright_test_version}" ]]; then
  rm -rf "${stage_dir}/node_modules" "${stage_dir}/package.json" "${stage_dir}/package-lock.json" "${stage_dir}/bun.lock"
  (
    cd "${stage_dir}"
    PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1 bun add --exact --no-save "@playwright/test@${playwright_test_version}"
  )
fi

rm -rf "${stage_dir}/e2e" "${stage_dir}/playwright.config.ts" "${stage_dir}/playwright.targets.ts" "${stage_dir}/playwright-report" "${stage_dir}/test-results"
cp -R /workspace/e2e "${stage_dir}/e2e"
cp /workspace/playwright.config.ts "${stage_dir}/playwright.config.ts"
cp /workspace/playwright.targets.ts "${stage_dir}/playwright.targets.ts"

cd "${stage_dir}"
set +e
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
  ./node_modules/.bin/playwright test "${forward_args[@]}"
  playwright_exit_code=$?
else
  playwright_exit_code=0
  for project_name in "${unique_project_filters[@]}"; do
    run_playwright_for_project "${project_name}" "${forward_args[@]}"
    project_exit_code=$?
    if [[ ${project_exit_code} -ne 0 ]]; then
      playwright_exit_code=${project_exit_code}
    fi
  done
fi
set -e

sync_playwright_artifacts
exit "${playwright_exit_code}"
