#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

readonly expected_betterleaks_version="1.6.1"
detected_betterleaks_version="$(betterleaks version 2>/dev/null | grep -Eo '[0-9]+(\.[0-9]+)+' | head -n1 || true)"
if [[ "$detected_betterleaks_version" != "$expected_betterleaks_version" ]]; then
  bash ./scripts/install-betterleaks.sh
  export PATH="${HOME}/.local/bin:${PATH}"
fi
betterleaks="$(command -v betterleaks)"
actual_betterleaks_version="$($betterleaks version 2>/dev/null | grep -Eo '[0-9]+(\.[0-9]+)+' | head -n1)"
[[ "$actual_betterleaks_version" == "$expected_betterleaks_version" ]] || {
  echo "Betterleaks v${expected_betterleaks_version} が必要です。" >&2
  exit 1
}
mode="${1:-}"

run_betterleaks() {
  GIT_CONFIG_COUNT=1 \
    GIT_CONFIG_KEY_0=safe.directory \
    GIT_CONFIG_VALUE_0="$repo_root" \
    "$betterleaks" "$@"
}

check_added_content() {
  awk '
    /^diff --/ { in_hunk = 0; next }
    /^@@/ { in_hunk = 1; next }
    in_hunk && /^\+/ { print substr($0, 2) }
  ' |
    bash ./scripts/check-sensitive-content.sh
}

scan_range() {
  local range="$1"
  local log_opts="--diff-merges=remerge $range"
  git log --diff-merges=remerge --no-renames --format= --name-only --diff-filter=ACMR -z "$range" |
    bash ./scripts/check-sensitive-paths.sh --stdin0
  git log --diff-merges=remerge --format= -p --unified=0 --no-color "$range" | check_added_content
  run_betterleaks git --git-workers=1 --redact=100 --no-banner --log-opts="$log_opts" .
}

case "$mode" in
  staged)
    git diff --cached --no-renames --name-only --diff-filter=ACMR -z |
      bash ./scripts/check-sensitive-paths.sh --stdin0
    git diff --cached --unified=0 --no-ext-diff --no-color | check_added_content
    run_betterleaks git --staged --redact=100 --no-banner .
    ;;
  range)
    [[ $# -eq 3 ]] || { echo "usage: $0 range <base> <head>" >&2; exit 2; }
    scan_range "$2..$3"
    ;;
  pre-push)
    remote_name="${2:-origin}"
    while read -r _local_ref local_sha _remote_ref remote_sha; do
      [[ "$local_sha" =~ ^0+$ ]] && continue
      if [[ "$remote_sha" =~ ^0+$ ]]; then
        log_opts="--diff-merges=remerge $local_sha --not --remotes=$remote_name"
        git log --diff-merges=remerge --no-renames --format= --name-only --diff-filter=ACMR -z \
          "$local_sha" --not --remotes="$remote_name" |
          bash ./scripts/check-sensitive-paths.sh --stdin0
        git log --diff-merges=remerge --format= -p --unified=0 --no-color \
          "$local_sha" --not --remotes="$remote_name" |
          check_added_content
      else
        range="$remote_sha..$local_sha"
        log_opts="--diff-merges=remerge $range"
        git log --diff-merges=remerge --no-renames --format= --name-only --diff-filter=ACMR -z "$range" |
          bash ./scripts/check-sensitive-paths.sh --stdin0
        git log --diff-merges=remerge --format= -p --unified=0 --no-color "$range" |
          check_added_content
      fi
      run_betterleaks git --git-workers=1 --redact=100 --no-banner --log-opts="$log_opts" .
    done
    ;;
  history)
    git log --all --diff-merges=remerge --format= --name-only -z |
      bash ./scripts/check-sensitive-paths.sh --stdin0
    run_betterleaks git --git-workers=1 --redact=100 --no-banner \
      --log-opts="--diff-merges=remerge --all" .
    ;;
  *)
    echo "usage: $0 {staged|pre-push <remote>|range <base> <head>|history}" >&2
    exit 2
    ;;
esac
