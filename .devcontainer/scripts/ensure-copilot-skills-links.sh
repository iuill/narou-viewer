#!/usr/bin/env bash

set -euo pipefail

warn() {
  printf '%s\n' "$1" >&2
}

if [[ "${GITHUB_ACTIONS:-}" == "true" ]]; then
  exit 0
fi

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${script_dir}/../.." && pwd)"
source_root="${repo_root}/.agents/skills"
target_path="${repo_root}/.github/skills"

if [[ ! -d "${source_root}" ]]; then
  exit 0
fi

if ! mkdir -p "${repo_root}/.github"; then
  warn "Failed to prepare .github directory for project skills. Skipping."
  exit 0
fi

if [[ -L "${target_path}" ]]; then
  current_path="$(readlink -f "${target_path}" 2>/dev/null || true)"
  source_path="$(readlink -f "${source_root}" 2>/dev/null || true)"
  if [[ -n "${current_path}" && -n "${source_path}" && "${current_path}" == "${source_path}" ]]; then
    exit 0
  fi

  if ! rm -f -- "${target_path}"; then
    warn "Project skills link already exists at ${target_path}, but could not be replaced. Skipping."
    exit 0
  fi
fi

if [[ -e "${target_path}" ]]; then
  warn "Project skills path already exists and is not a symlink: ${target_path}. Skipping."
  exit 0
fi

if ! ln -s "${source_root}" "${target_path}"; then
  warn "Failed to create project skills symlink at ${target_path}. Skipping."
  exit 0
fi
