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
    "$betterleaks" --ignore-gitleaks-allow "$@"
}

scan_message_file() {
  local message_file="$1"
  [[ -f "$message_file" ]] || {
    echo "commit message file が見つかりません: $message_file" >&2
    return 1
  }
  bash ./scripts/check-sensitive-content.sh <"$message_file"
  run_betterleaks stdin --redact=100 --no-banner <"$message_file"
}

scan_commit_messages() (
  local message_file
  message_file="$(mktemp)"
  trap 'rm -f "$message_file"' EXIT
  git log --format='author-name: %an%nauthor-email: %ae%ncommitter-name: %cn%ncommitter-email: %ce%nmessage:%n%B' \
    "$@" >"$message_file"
  scan_message_file "$message_file"
)

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
  scan_commit_messages "$range"
  run_betterleaks git --git-workers=1 --redact=100 --no-banner --log-opts="$log_opts" .
}

resolve_branch_base() {
  local requested_base="${1:-}"
  local remote_name="${SECURITY_SCAN_REMOTE:-origin}"
  local remote_default=""
  local candidate

  if [[ -n "$requested_base" ]]; then
    git rev-parse --verify --quiet "${requested_base}^{commit}" >/dev/null || {
      echo "branch scan の base ref が見つかりません: $requested_base" >&2
      return 1
    }
    printf '%s\n' "$requested_base"
    return
  fi

  remote_default="$(git symbolic-ref --quiet --short "refs/remotes/${remote_name}/HEAD" 2>/dev/null || true)"
  for candidate in "$remote_default" "${remote_name}/main" "${remote_name}/master"; do
    [[ -n "$candidate" ]] || continue
    if git rev-parse --verify --quiet "${candidate}^{commit}" >/dev/null; then
      printf '%s\n' "$candidate"
      return
    fi
  done

  echo "branch scan の base ref を解決できません。base ref を明示してください。" >&2
  return 1
}

case "$mode" in
  staged)
    git diff --cached --no-renames --name-only --diff-filter=ACMR -z |
      bash ./scripts/check-sensitive-paths.sh --stdin0
    git diff --cached --unified=0 --no-ext-diff --no-color | check_added_content
    run_betterleaks git --staged --redact=100 --no-banner .
    ;;
  message)
    [[ $# -eq 2 ]] || { echo "usage: $0 message <message-file>" >&2; exit 2; }
    scan_message_file "$2"
    ;;
  range)
    [[ $# -eq 3 ]] || { echo "usage: $0 range <base> <head>" >&2; exit 2; }
    scan_range "$2..$3"
    ;;
  branch)
    [[ $# -le 2 ]] || { echo "usage: $0 branch [base-ref]" >&2; exit 2; }
    inferred_base=1
    [[ -n "${2:-}" ]] && inferred_base=0
    base_ref="$(resolve_branch_base "${2:-}")"
    merge_base="$(git merge-base "$base_ref" HEAD)"
    commit_count="$(git rev-list --count "$merge_base..HEAD")"
    printf 'branch scan: base=%s merge-base=%s commits=%s\n' "$base_ref" "$merge_base" "$commit_count" >&2
    if [[ "$inferred_base" == "1" && "$commit_count" == "0" ]]; then
      echo "推測したbaseでは走査対象commitがありません。PRのbaseを明示してください: bun run security:scan:branch -- <base-ref>" >&2
      exit 1
    fi
    scan_range "$merge_base..HEAD"
    ;;
  pre-push)
    remote_name="${2:-origin}"
    while read -r _local_ref local_sha _remote_ref _remote_sha; do
      [[ "$local_sha" =~ ^0+$ ]] && continue
      log_opts="--diff-merges=remerge $local_sha --not --remotes=$remote_name"
      git log --diff-merges=remerge --no-renames --format= --name-only --diff-filter=ACMR -z \
        "$local_sha" --not --remotes="$remote_name" |
        bash ./scripts/check-sensitive-paths.sh --stdin0
      git log --diff-merges=remerge --format= -p --unified=0 --no-color \
        "$local_sha" --not --remotes="$remote_name" |
        check_added_content
      scan_commit_messages "$local_sha" --not --remotes="$remote_name"
      run_betterleaks git --git-workers=1 --redact=100 --no-banner --log-opts="$log_opts" .
    done
    ;;
  history)
    git log --all --diff-merges=remerge --format= --name-only -z |
      bash ./scripts/check-sensitive-paths.sh --stdin0
    scan_commit_messages --all
    run_betterleaks git --git-workers=1 --redact=100 --no-banner \
      --log-opts="--diff-merges=remerge --all" .
    ;;
  *)
    echo "usage: $0 {staged|message <message-file>|branch [base-ref]|pre-push <remote>|range <base> <head>|history}" >&2
    exit 2
    ;;
esac
