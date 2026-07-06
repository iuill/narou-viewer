#!/usr/bin/env bash

set -euo pipefail

proxy_runtime_dir="/run/devcontainer-docker-proxy"
proxy_socket="${DEVCONTAINER_DOCKER_PROXY_SOCKET:-}"
proxy_log_file="${proxy_runtime_dir}/socat.log"
source_socket_candidates=(
  "/var/run/docker-host.sock"
  "/var/run/docker.sock"
)

if [[ -z "${proxy_socket}" && "${DOCKER_HOST:-}" == unix://* ]]; then
  proxy_socket="${DOCKER_HOST#unix://}"
fi

if [[ -z "${proxy_socket}" ]]; then
  proxy_socket="${proxy_runtime_dir}/docker.sock"
fi

if [[ "${proxy_socket}" != "${proxy_runtime_dir}/docker.sock" ]]; then
  echo "Unsupported Dev Container Docker proxy socket path: ${proxy_socket}" >&2
  exit 1
fi

if DOCKER_HOST="unix://${proxy_socket}" docker version >/dev/null 2>&1; then
  exit 0
fi

if ! command -v nohup >/dev/null 2>&1; then
  echo "nohup is required to keep the Docker socket proxy running inside the Dev Container." >&2
  exit 1
fi

if ! command -v socat >/dev/null 2>&1; then
  echo "socat is required to expose the Docker socket proxy inside the Dev Container." >&2
  exit 1
fi

if ! command -v ss >/dev/null 2>&1; then
  echo "ss is required to inspect the Docker socket proxy inside the Dev Container." >&2
  exit 1
fi

if ! command -v sudo >/dev/null 2>&1 || ! sudo -n true >/dev/null 2>&1; then
  echo "Passwordless sudo is required to repair the Docker socket proxy inside the Dev Container." >&2
  exit 1
fi

source_socket=""
for candidate in "${source_socket_candidates[@]}"; do
  if [[ -S "${candidate}" ]]; then
    source_socket="${candidate}"
    break
  fi
done

if [[ -z "${source_socket}" ]]; then
  echo "No Docker socket source found for Dev Container proxy." >&2
  exit 1
fi

find_proxy_pids() {
  sudo -n ss -xlpnH |
    awk -v socket="${proxy_socket}" '$5 == socket { print }' |
    grep -oE 'pid=[0-9]+' |
    cut -d= -f2
}

proxy_user="$(id -un)"
proxy_group="$(id -gn)"

sudo -n install -d -m 0755 -o root -g root "${proxy_runtime_dir}"

if DOCKER_HOST="unix://${proxy_socket}" docker version >/dev/null 2>&1; then
  exit 0
fi

while IFS= read -r proxy_pid; do
  [[ -n "${proxy_pid}" ]] || continue

  proxy_command="$(sudo -n ps -o comm= -p "${proxy_pid}" 2>/dev/null | tr -d '[:space:]')"
  if [[ "${proxy_command}" == "socat" ]]; then
    sudo -n kill "${proxy_pid}" 2>/dev/null || true
  fi
done < <(find_proxy_pids)

sudo -n rm -f -- "${proxy_socket}"

sudo -n nohup socat \
  -lf "${proxy_log_file}" \
  "UNIX-LISTEN:${proxy_socket},fork,mode=0660,user=${proxy_user},group=${proxy_group}" \
  "UNIX-CONNECT:${source_socket}" \
  </dev/null >/dev/null 2>&1 &

for _ in $(seq 1 20); do
  if DOCKER_HOST="unix://${proxy_socket}" docker version >/dev/null 2>&1; then
    exit 0
  fi
  sleep 0.5
done

echo "Failed to initialize Docker socket proxy for the Dev Container." >&2
exit 1
