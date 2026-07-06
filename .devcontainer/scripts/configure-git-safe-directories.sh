#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(readlink -f "${script_dir}/../..")"
gitmodules_path="${repo_root}/.gitmodules"
can_update_system=false

if command -v sudo >/dev/null 2>&1 && sudo -n true >/dev/null 2>&1; then
  can_update_system=true
fi

declare -A registered_paths=()

warn() {
  printf 'configure-git-safe-directories: %s\n' "$*" >&2
}

is_within_repo_root() {
  local target_path="$1"

  [[ "${target_path}" == "${repo_root}" ]] && return 0
  [[ "${target_path}" == "${repo_root}/"* ]]
}

ensure_safe_directory() {
  local scope="$1"
  local target_path="$2"

  if git config "${scope}" --get-all safe.directory 2>/dev/null | grep -Fqx "${target_path}"; then
    return 0
  fi

  git config "${scope}" --add safe.directory "${target_path}"
}

ensure_safe_directory_system() {
  local target_path="$1"

  if ! ${can_update_system}; then
    return 0
  fi

  if sudo -n git config --system --get-all safe.directory 2>/dev/null | grep -Fqx "${target_path}"; then
    return 0
  fi

  sudo -n git config --system --add safe.directory "${target_path}"
}

register_safe_path() {
  local candidate_path="$1"
  local resolved_path=""

  [[ -n "${candidate_path}" ]] || return 0
  [[ -e "${candidate_path}" ]] || return 0

  resolved_path="$(readlink -f "${candidate_path}")"
  [[ -n "${resolved_path}" ]] || return 0

  if ! is_within_repo_root "${resolved_path}"; then
    warn "skip path outside repo root: ${resolved_path}"
    return 0
  fi

  if [[ -n "${registered_paths[${resolved_path}]:-}" ]]; then
    return 0
  fi

  registered_paths["${resolved_path}"]=1
  ensure_safe_directory --global "${resolved_path}"
  ensure_safe_directory_system "${resolved_path}"
}

register_repo_safe_paths() {
  local repo_path="$1"
  local git_dir=""
  local git_common_dir=""

  [[ -d "${repo_path}" ]] || return 0

  register_safe_path "${repo_path}"
  register_safe_path "${repo_path}/.git"

  if ! git -C "${repo_path}" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    return 0
  fi

  git_dir="$(git -C "${repo_path}" rev-parse --path-format=absolute --git-dir 2>/dev/null || true)"
  git_common_dir="$(git -C "${repo_path}" rev-parse --path-format=absolute --git-common-dir 2>/dev/null || true)"

  register_safe_path "${git_dir}"
  register_safe_path "${git_common_dir}"
}

register_repo_safe_paths "${repo_root}"

if [[ ! -f "${gitmodules_path}" ]]; then
  exit 0
fi

while IFS= read -r submodule_path; do
  [[ -n "${submodule_path}" ]] || continue
  register_repo_safe_paths "${repo_root}/${submodule_path}"
done < <(
  git config --null --file "${gitmodules_path}" --get-regexp '^submodule\..*\.path$' |
    while IFS= read -r -d '' submodule_entry; do
      printf '%s\n' "${submodule_entry#*$'\n'}"
    done
)
