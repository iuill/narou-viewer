#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${script_dir}/e2e-compose-common.sh"
services=(novel-fetcher-e2e viewer-api-e2e)
if [[ "${E2E_SKIP_VIEWER_WEB:-0}" != "1" ]]; then
  services+=(viewer-web-e2e)
fi
playwright_service="playwright-e2e"
playwright_image_version="${PLAYWRIGHT_IMAGE_VERSION:-1.58.2-node20-bookworm-slim-chromium-headless-shell-webkit-curl-amd64}"
playwright_base_image="${PLAYWRIGHT_BASE_IMAGE:-ghcr.io/iuill/narou-viewer-playwright:${playwright_image_version}}"
playwright_image_version_label="io.narou-viewer.playwright-image-version"
playwright_base_image_label="io.narou-viewer.playwright-base-image"
fixture_refresh_flag="${script_dir}/../data_e2e/tmp/novel-fetcher-fixture-refreshed"
fixture_refresh_requires_recreate=0
compose_up_args=(--profile e2e up -d --no-build)

initialize_e2e_compose_environment

compose_up_args+=(--no-deps)

if [[ -f "${fixture_refresh_flag}" ]]; then
  printf '%s\n' "E2E novel-fetcher fixture changed. Recreating E2E services so SQLite readers reopen the refreshed fixture."
  fixture_refresh_requires_recreate=1
  compose_up_args+=(--force-recreate)
fi

clear_fixture_refresh_flag() {
  if [[ "${fixture_refresh_requires_recreate}" == "1" ]]; then
    rm -f "${fixture_refresh_flag}"
  fi
}

ensure_playwright_e2e_image_version() {
  local image_ref=""
  if [[ -n "${e2e_compose_project_name:-}" ]]; then
    image_ref="${e2e_compose_project_name}-${playwright_service}"
  fi

  if [[ -z "${image_ref}" ]]; then
    return 0
  fi

  local actual_version
  actual_version="$(
    docker image inspect \
      --format "{{ index .Config.Labels \"${playwright_image_version_label}\" }}" \
      "${image_ref}" 2>/dev/null || true
  )"

  local actual_base_image
  actual_base_image="$(
    docker image inspect \
      --format "{{ index .Config.Labels \"${playwright_base_image_label}\" }}" \
      "${image_ref}" 2>/dev/null || true
  )"

  if [[ "${actual_version}" == "${playwright_image_version}" && "${actual_base_image}" == "${playwright_base_image}" ]]; then
    return 0
  fi

  printf '%s\n' \
    "Playwright E2E image is stale (version have: ${actual_version:-unknown}, want: ${playwright_image_version}; base image have: ${actual_base_image:-unknown}, want: ${playwright_base_image}). Rebuilding ${playwright_service}." >&2
  run_e2e_compose --profile e2e build "${playwright_service}"
}

wait_for_service_http_ready() {
  local service="$1"
  local label="$2"
  local url="$3"
  local timeout_seconds="${E2E_SERVICE_READY_TIMEOUT_SECONDS:-180}"
  local started_at="${SECONDS}"

  printf 'Waiting for %s: %s\n' "${label}" "${url}"

  until run_e2e_compose exec -T "${service}" sh -lc \
    "if command -v curl >/dev/null 2>&1; then curl -fsS '${url}' >/dev/null; elif command -v wget >/dev/null 2>&1; then wget -q -O /dev/null '${url}'; else READY_URL='${url}' bun --eval 'const res = await fetch(process.env.READY_URL); if (!res.ok) process.exit(1);' >/dev/null; fi" >/dev/null 2>&1; do
    if ((SECONDS - started_at >= timeout_seconds)); then
      printf 'Timed out waiting for %s after %s seconds: %s\n' "${label}" "${timeout_seconds}" "${url}" >&2
      return 1
    fi

    sleep 2
  done
}

wait_for_e2e_services_ready() {
  wait_for_service_http_ready "viewer-api-e2e" "viewer-api library" "http://127.0.0.1:18080/api/library/novels"
  if [[ "${E2E_SKIP_VIEWER_WEB:-0}" != "1" ]]; then
    wait_for_service_http_ready "viewer-web-e2e" "viewer-web" "http://127.0.0.1:15173/"
  fi
}

if [[ "${E2E_SKIP_PLAYWRIGHT_IMAGE:-0}" != "1" ]]; then
  ensure_playwright_e2e_image_version
fi

if [[ "${CODESPACES:-}" == "true" ]]; then
  running_services="$(run_e2e_compose ps --services --status running 2>/dev/null || true)"
  missing_services=()

  for service in "${services[@]}"; do
    if ! printf '%s\n' "${running_services}" | grep -qx "${service}"; then
      missing_services+=("${service}")
    fi
  done

  if [[ "${#missing_services[@]}" == "0" && "${fixture_refresh_requires_recreate}" != "1" ]]; then
    printf '%s\n' "Codespaces detected: E2E services are already running. Skipping docker compose up."
    wait_for_e2e_services_ready
    exit 0
  fi
fi

run_e2e_compose up --no-build go-cache-init

attempt_no_build_output_file="$(mktemp)"
trap 'rm -f "${attempt_no_build_output_file}"' EXIT

if run_e2e_compose "${compose_up_args[@]}" "${services[@]}" >"${attempt_no_build_output_file}" 2>&1; then
  cat "${attempt_no_build_output_file}"
  wait_for_e2e_services_ready
  clear_fixture_refresh_flag
  exit 0
fi

cat "${attempt_no_build_output_file}" >&2

if grep -Fq "No such image" "${attempt_no_build_output_file}"; then
  printf '%s\n' "E2E service images are missing. Retrying with docker compose build enabled."
  retry_compose_up_args=("${compose_up_args[@]}")
  for index in "${!retry_compose_up_args[@]}"; do
    if [[ "${retry_compose_up_args[${index}]}" == "--no-build" ]]; then
      retry_compose_up_args[${index}]="--build"
    fi
  done
  run_e2e_compose "${retry_compose_up_args[@]}" "${services[@]}"
  wait_for_e2e_services_ready
  clear_fixture_refresh_flag
  exit 0
fi

exit 1
