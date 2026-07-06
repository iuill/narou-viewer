#!/usr/bin/env bash

ensure_docker_host() {
  if docker version >/dev/null 2>&1; then
    return 0
  fi

  local current_docker_host="${DOCKER_HOST:-}"
  local proxy_socket="${DEVCONTAINER_DOCKER_PROXY_SOCKET:-/run/devcontainer-docker-proxy/docker.sock}"
  local direct_socket="/var/run/docker.sock"
  local ensure_proxy_script
  ensure_proxy_script="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/.devcontainer/scripts/ensure-docker-socket-proxy.sh"

  if [[ -x "${ensure_proxy_script}" ]]; then
    if [[ -z "${current_docker_host}" || "${current_docker_host}" == "unix://${proxy_socket}" ]]; then
      if bash "${ensure_proxy_script}" >/dev/null 2>&1 && docker version >/dev/null 2>&1; then
        return 0
      fi
    fi
  fi

  if [[ -S "${direct_socket}" ]]; then
    if DOCKER_HOST="unix://${direct_socket}" docker version >/dev/null 2>&1; then
      if [[ -n "${current_docker_host}" && "${current_docker_host}" != "unix://${direct_socket}" ]]; then
        printf '%s\n' \
          "Docker proxy (${current_docker_host}) is unavailable. Falling back to unix://${direct_socket} for this command." >&2
      fi
      export DOCKER_HOST="unix://${direct_socket}"
      return 0
    fi
  fi

  return 1
}

ensure_docker_api_compat() {
  if [[ -n "${DOCKER_API_VERSION:-}" ]]; then
    return 0
  fi

  local docker_version_output
  docker_version_output="$(docker version --format '{{.Server.APIVersion}}' 2>&1 || true)"

  if [[ "${docker_version_output}" =~ Maximum\ supported\ API\ version\ is\ ([0-9.]+) ]]; then
    export DOCKER_API_VERSION="${BASH_REMATCH[1]}"
  fi
}

resolve_compose_project() {
  if [[ -n "${COMPOSE_PROJECT_NAME:-}" ]]; then
    printf '%s\n' "${COMPOSE_PROJECT_NAME}"
    return 0
  fi

  local current_container_id
  current_container_id="$(hostname 2>/dev/null || true)"
  if [[ -n "${current_container_id}" ]]; then
    local current_project
    current_project="$(
      docker inspect "${current_container_id}" \
        --format '{{ index .Config.Labels "com.docker.compose.project" }}' \
        2>/dev/null || true
    )"
    if [[ -n "${current_project}" && "${current_project}" != "<no value>" ]]; then
      printf '%s\n' "${current_project}"
      return 0
    fi
  fi

  local viewer_dev_ids
  viewer_dev_ids="$(
    docker ps \
      --filter label=com.docker.compose.service=viewer-dev \
      --format '{{.ID}}' \
      2>/dev/null || true
  )"

  if [[ -n "${viewer_dev_ids}" ]]; then
    local viewer_dev_count
    viewer_dev_count="$(printf '%s\n' "${viewer_dev_ids}" | sed '/^$/d' | wc -l | tr -d ' ')"
    if [[ "${viewer_dev_count}" == "1" ]]; then
      local viewer_dev_id
      viewer_dev_id="$(printf '%s\n' "${viewer_dev_ids}" | head -n1)"
      local viewer_dev_project
      viewer_dev_project="$(
        docker inspect "${viewer_dev_id}" \
          --format '{{ index .Config.Labels "com.docker.compose.project" }}' \
          2>/dev/null || true
      )"
      if [[ -n "${viewer_dev_project}" && "${viewer_dev_project}" != "<no value>" ]]; then
        printf '%s\n' "${viewer_dev_project}"
        return 0
      fi
    fi
  fi

  return 1
}

resolve_workspace_source() {
  local current_container_id
  current_container_id="$(hostname 2>/dev/null || true)"
  if [[ -n "${current_container_id}" ]]; then
    docker inspect "${current_container_id}" \
      --format '{{range .Mounts}}{{if eq .Destination "/workspace"}}{{.Source}}{{end}}{{end}}' \
      2>/dev/null || true
  fi
}

initialize_e2e_compose_environment() {
  local compose_project_name
  local devcontainer_env_file=".devcontainer/.env"
  local workspace_source

  e2e_compose_project_name=""
  e2e_compose_args=(-f .devcontainer/docker-compose.yml)

  if ! ensure_docker_host; then
    cat >&2 <<'EOF'
Docker is unavailable for E2E compose.

If this is a Dev Container, restore the Docker socket proxy and retry:
  bash .devcontainer/scripts/ensure-docker-socket-proxy.sh

Then confirm Docker is reachable:
  docker version
EOF
    return 1
  fi
  ensure_docker_api_compat

  if compose_project_name="$(resolve_compose_project)"; then
    e2e_compose_project_name="${compose_project_name}"
    e2e_compose_args=(-p "${compose_project_name}" "${e2e_compose_args[@]}")
  fi

  if [[ -f "${devcontainer_env_file}" ]]; then
    e2e_compose_args=(--env-file "${devcontainer_env_file}" "${e2e_compose_args[@]}")
  fi

  if workspace_source="$(resolve_workspace_source)"; then
    if [[ -n "${workspace_source}" ]]; then
      export HOST_WORKSPACE_DIR="${workspace_source}"
      export HOST_DATA_DIR="${workspace_source}/data"
      export HOST_E2E_DATA_DIR="${workspace_source}/data_e2e"
    fi
  fi
}

run_e2e_compose() {
  docker compose "${e2e_compose_args[@]}" "$@"
}
