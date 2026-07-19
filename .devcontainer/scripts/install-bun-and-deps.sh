#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

ensure_line_in_file() {
  local file_path="$1"
  local line="$2"

  touch "${file_path}"

  if ! grep -Fqx "${line}" "${file_path}"; then
    printf '%s\n' "${line}" >>"${file_path}"
  fi
}

remove_line_from_file() {
  local file_path="$1"
  local line="$2"
  local temp_file
  local grep_status

  if [[ -f "${file_path}" ]]; then
    temp_file="$(mktemp)"
    if grep -Fvx -- "${line}" "${file_path}" >"${temp_file}"; then
      :
    else
      grep_status=$?
      if ((grep_status != 1)); then
        rm -f "${temp_file}"
        return "${grep_status}"
      fi
    fi
    mv "${temp_file}" "${file_path}"
  fi
}

export BUN_INSTALL="${BUN_INSTALL:-$HOME/.bun}"
export PATH="${BUN_INSTALL}/bin:${PATH}"
# 以下の CLI バージョンは viewer-dev/Dockerfile の ARG と一致させること。
# Dockerfile 側で焼き込み済みならこのスクリプトはインストールをスキップする。
OPENAI_CODEX_VERSION="${OPENAI_CODEX_VERSION:-0.144.6}"
GITHUB_COPILOT_VERSION="${GITHUB_COPILOT_VERSION:-1.0.71}"
PLAYWRIGHT_CLI_VERSION="${PLAYWRIGHT_CLI_VERSION:-0.1.17}"
SERENA_AGENT_VERSION="${SERENA_AGENT_VERSION:-1.3.0}"
SCC_VERSION="${SCC_VERSION:-v3.7.0}"
SCC_LINUX_X86_64_SHA256="${SCC_LINUX_X86_64_SHA256:-3d9d65b00ca874c2b29151abe7e1480736f5229edc3ce8e4b2791460cdfabf5a}"
BETTERLEAKS_VERSION="1.6.1"
# gopls は postCreate 専用で導入する。
GOPLS_VERSION="${GOPLS_VERSION:-v0.22.0}"
CODEX_HOME_DIR="${CODEX_HOME:-${HOME}/.codex}"
CODEX_SKILLS_DIR="${CODEX_SKILLS_DIR:-${CODEX_HOME_DIR}/skills}"
LOCAL_BIN_DIR="${HOME}/.local/bin"
export PATH="${LOCAL_BIN_DIR}:${PATH}"

get_codex_version() {
  codex --version 2>/dev/null | awk '{print $NF}'
}

get_copilot_version() {
  copilot --version 2>/dev/null | head -n1 | grep -Eo '[0-9]+(\.[0-9]+)+'
}

get_global_npm_package_version() {
  local package_name="$1"
  local npm_root
  npm_root="$(npm root -g 2>/dev/null || true)"

  if [[ -z "${npm_root}" ]]; then
    return 0
  fi

  node -e '
    const fs = require("node:fs");
    const path = require("node:path");
    const packageName = process.argv[1];
    const npmRoot = process.argv[2];
    const packageJson = path.join(npmRoot, packageName, "package.json");

    if (!fs.existsSync(packageJson)) {
      process.exit(0);
    }

    process.stdout.write(JSON.parse(fs.readFileSync(packageJson, "utf8")).version);
  ' "${package_name}" "${npm_root}" 2>/dev/null || true
}

get_gopls_version() {
  "${LOCAL_BIN_DIR}/gopls" version 2>/dev/null | awk 'NR == 1 { print $2 }'
}

get_serena_version() {
  serena --version 2>/dev/null | grep -Eo '[0-9]+(\.[0-9]+)+' | head -n1
}

get_scc_version() {
  local version
  version="$(scc --version 2>/dev/null | grep -Eo 'v?[0-9]+(\.[0-9]+)+' | head -n1 || true)"
  if [ -n "${version}" ]; then
    printf 'v%s\n' "${version#v}"
  fi
}

get_betterleaks_version() {
  betterleaks version 2>/dev/null | grep -Eo '[0-9]+(\.[0-9]+)+' | head -n1
}

playwright_cli_browser_installed() {
  local browser_name="$1"
  local install_list
  install_list="$(playwright-cli install-browser --list 2>/dev/null || true)"

  case "${browser_name}" in
    chromium)
      grep -q '/chromium-' <<<"${install_list}"
      ;;
    webkit)
      grep -q '/webkit-' <<<"${install_list}"
      ;;
    *)
      return 1
      ;;
  esac
}

sync_playwright_cli_skill_for_codex() {
  local npm_root skill_source_dir skill_dest_dir
  npm_root="$(npm root -g 2>/dev/null || true)"
  skill_source_dir="${npm_root}/@playwright/cli/skills/playwright-cli"
  skill_dest_dir="${CODEX_SKILLS_DIR}/playwright-cli"

  if [[ ! -d "${skill_source_dir}" ]]; then
    printf '%s\n' "playwright-cli skill source not found: ${skill_source_dir}" >&2
    return 1
  fi

  mkdir -p "${CODEX_SKILLS_DIR}"
  rm -rf "${skill_dest_dir}"
  cp -R "${skill_source_dir}" "${skill_dest_dir}"
}

ensure_serena_project_registered() {
  local project_root="$1"
  local serena_home="${SERENA_HOME:-${HOME}/.serena}"
  local config_path="${serena_home}/serena_config.yml"

  serena project index "${project_root}" --name narou-viewer --language typescript --language go --log-level INFO

  if ! serena_project_config_contains_root "${config_path}" "${project_root}"; then
    printf '%s\n' "Serena project registration was not written to ${config_path}: ${project_root}" >&2
    return 1
  fi
}

serena_project_config_contains_root() {
  local config_path="$1"
  local project_root="$2"
  local in_projects=false
  local line trimmed_line registered_path

  [[ -f "${config_path}" ]] || return 1

  while IFS= read -r line; do
    if [[ "${line}" == "projects:" ]]; then
      in_projects=true
      continue
    fi

    if [[ "${in_projects}" != true ]]; then
      continue
    fi

    trimmed_line="${line#"${line%%[![:space:]]*}"}"

    case "${trimmed_line}" in
      ""|\#*) continue ;;
      "- "*)
        registered_path="${trimmed_line#- }"
        registered_path="${registered_path%\"}"
        registered_path="${registered_path#\"}"
        registered_path="${registered_path%\'}"
        registered_path="${registered_path#\'}"
        if [[ -e "${registered_path}" && "${registered_path}" -ef "${project_root}" ]]; then
          return 0
        fi
        ;;
    esac

    case "${line}" in
      [![:space:]]*:*) break ;;
    esac
  done <"${config_path}"

  return 1
}

ensure_serena() {
  if ! command -v uv >/dev/null 2>&1; then
    curl -LsSf https://astral.sh/uv/install.sh | sh
  fi

  if ! command -v uv >/dev/null 2>&1 && [ -x "${LOCAL_BIN_DIR}/uv" ]; then
    sudo ln -sf "${LOCAL_BIN_DIR}/uv" /usr/local/bin/uv
  fi

  if [ "$(get_serena_version)" != "${SERENA_AGENT_VERSION}" ]; then
    uv tool install -p 3.13 "serena-agent==${SERENA_AGENT_VERSION}"
  fi

  if ! command -v serena >/dev/null 2>&1 && [ -x "${LOCAL_BIN_DIR}/serena" ]; then
    sudo ln -sf "${LOCAL_BIN_DIR}/serena" /usr/local/bin/serena
  fi

  if ! command -v serena-hooks >/dev/null 2>&1 && [ -x "${LOCAL_BIN_DIR}/serena-hooks" ]; then
    sudo ln -sf "${LOCAL_BIN_DIR}/serena-hooks" /usr/local/bin/serena-hooks
  fi

  serena init
  ensure_serena_project_registered "${REPO_ROOT}"
}

ensure_gopls() {
  if ! command -v go >/dev/null 2>&1; then
    printf '%s\n' "go is required to install gopls" >&2
    return 1
  fi

  mkdir -p "${LOCAL_BIN_DIR}"

  if [ "$(get_gopls_version)" != "${GOPLS_VERSION}" ]; then
    GOTOOLCHAIN=auto GOBIN="${LOCAL_BIN_DIR}" go install "golang.org/x/tools/gopls@${GOPLS_VERSION}"
  fi
}

ensure_scc() {
  if [ "$(get_scc_version)" = "${SCC_VERSION}" ]; then
    return
  fi

  local arch
  arch="$(dpkg --print-architecture)"
  if [ "${arch}" != "amd64" ]; then
    printf '%s\n' "scc install is skipped for unsupported architecture: ${arch}" >&2
    return
  fi

  local tmpdir archive_path
  tmpdir="$(mktemp -d)"
  archive_path="${tmpdir}/scc_Linux_x86_64.tar.gz"
  trap 'rm -rf "${tmpdir}"' RETURN

  curl -fsSL "https://github.com/boyter/scc/releases/download/${SCC_VERSION}/scc_Linux_x86_64.tar.gz" -o "${archive_path}"
  echo "${SCC_LINUX_X86_64_SHA256}  ${archive_path}" | sha256sum -c -
  tar -xzf "${archive_path}" -C "${tmpdir}"
  sudo install -m 0755 "${tmpdir}/scc" /usr/local/bin/scc
}

ensure_betterleaks() {
  if [ "$(get_betterleaks_version)" = "${BETTERLEAKS_VERSION}" ]; then
    return
  fi

  BETTERLEAKS_INSTALL_DIR="${LOCAL_BIN_DIR}" bash "${REPO_ROOT}/scripts/install-betterleaks.sh"

  if [ "$(get_betterleaks_version)" != "${BETTERLEAKS_VERSION}" ]; then
    printf '%s\n' "Betterleaks ${BETTERLEAKS_VERSION} was installed but is not selected from PATH." >&2
    return 1
  fi
}

if ! command -v rg >/dev/null 2>&1 || ! command -v bwrap >/dev/null 2>&1; then
  sudo apt-get update
  DEBIAN_FRONTEND=noninteractive sudo apt-get install -y \
    bubblewrap \
    ripgrep
fi

if ! command -v bun >/dev/null 2>&1; then
  curl -fsSL https://bun.sh/install | bash
  export PATH="${BUN_INSTALL}/bin:${PATH}"
fi

remove_line_from_file "${HOME}/.bashrc" 'export PATH="$(go env GOPATH 2>/dev/null)/bin:$PATH"'
remove_line_from_file "${HOME}/.profile" 'export PATH="$(go env GOPATH 2>/dev/null)/bin:$PATH"'
remove_line_from_file "${HOME}/.bashrc" 'if command -v go >/dev/null 2>&1; then go_path="$(go env GOPATH 2>/dev/null || true)"; if [ -n "$go_path" ]; then export PATH="$go_path/bin:$PATH"; fi; unset go_path; fi'
remove_line_from_file "${HOME}/.profile" 'if command -v go >/dev/null 2>&1; then go_path="$(go env GOPATH 2>/dev/null || true)"; if [ -n "$go_path" ]; then export PATH="$go_path/bin:$PATH"; fi; unset go_path; fi'
remove_line_from_file "${HOME}/.bashrc" 'if [[ "${PS1:-}" != *"[DEV]"* ]]; then PS1="\[\033[1;36m\][DEV]\[\033[0m\] ${PS1:-}"; fi'
ensure_line_in_file "${HOME}/.bashrc" 'export BUN_INSTALL="${BUN_INSTALL:-$HOME/.bun}"'
ensure_line_in_file "${HOME}/.bashrc" 'export PATH="$BUN_INSTALL/bin:$PATH"'
ensure_line_in_file "${HOME}/.bashrc" 'export PATH="$HOME/.local/bin:$PATH"'
ensure_line_in_file "${HOME}/.bashrc" 'export PATH="/workspace/.devcontainer/bin:$PATH"'
ensure_line_in_file "${HOME}/.bashrc" 'if [[ "${PS1:-}" != *"[DEV]"* ]]; then PS1="[DEV] ${PS1:-}"; fi'
ensure_line_in_file "${HOME}/.profile" 'export BUN_INSTALL="${BUN_INSTALL:-$HOME/.bun}"'
ensure_line_in_file "${HOME}/.profile" 'export PATH="$BUN_INSTALL/bin:$PATH"'
ensure_line_in_file "${HOME}/.profile" 'export PATH="$HOME/.local/bin:$PATH"'
ensure_line_in_file "${HOME}/.profile" 'export PATH="/workspace/.devcontainer/bin:$PATH"'

ensure_gopls
ensure_scc
ensure_betterleaks
ensure_serena

bun run install:locked

# Keep container-scoped CLIs out of the workspace lockfile while making them
# available from the standard Bun global bin directory.
# @playwright/cli is the coding-agent screenshot/browser-control CLI.
# It is intentionally separate from E2E Playwright, which is pinned by
# @playwright/test, PLAYWRIGHT_TEST_VERSION, and the playwright-e2e image tag.
packages_to_install=()

if [ "$(get_codex_version)" != "${OPENAI_CODEX_VERSION}" ]; then
  packages_to_install+=("@openai/codex@${OPENAI_CODEX_VERSION}")
fi

if [ "$(get_copilot_version)" != "${GITHUB_COPILOT_VERSION}" ]; then
  packages_to_install+=("@github/copilot@${GITHUB_COPILOT_VERSION}")
fi

if [ ${#packages_to_install[@]} -gt 0 ]; then
  bun add -g "${packages_to_install[@]}"
fi

if [ "$(get_global_npm_package_version "@playwright/cli")" != "${PLAYWRIGHT_CLI_VERSION}" ]; then
  npm install -g "@playwright/cli@${PLAYWRIGHT_CLI_VERSION}"
fi

if command -v playwright-cli >/dev/null 2>&1; then
  if ! playwright_cli_browser_installed chromium; then
    playwright-cli install-browser chromium --with-deps
  fi

  if ! playwright_cli_browser_installed webkit; then
    playwright-cli install-browser webkit --with-deps
  fi

  sync_playwright_cli_skill_for_codex
fi

if ! bash "${REPO_ROOT}/scripts/install-git-hooks.sh"; then
  printf '%s\n' "Git hooks were not changed; review the existing core.hooksPath setting." >&2
fi
