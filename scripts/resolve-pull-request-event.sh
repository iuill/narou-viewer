#!/usr/bin/env bash
set -euo pipefail

event_name="${1:-${GITHUB_EVENT_NAME:-}}"
event_path="${2:-${GITHUB_EVENT_PATH:-}}"
output_file="${GITHUB_OUTPUT:-/dev/stdout}"
repository="${GITHUB_REPOSITORY:-}"

[[ -n "$event_name" && -f "$event_path" && -n "$repository" ]] || {
  echo "event name, event payload, GITHUB_REPOSITORY が必要です。" >&2
  exit 2
}

case "$event_name" in
  pull_request_target|pull_request_review|pull_request_review_comment)
    pr_number="$(jq -er '.pull_request.number' "$event_path")"
    ;;
  issue_comment)
    jq -e '.issue.pull_request != null' "$event_path" >/dev/null || exit 78
    pr_number="$(jq -er '.issue.number' "$event_path")"
    ;;
  *)
    echo "未対応のeventです: $event_name" >&2
    exit 2
    ;;
esac

pr_json="$(gh api "repos/${repository}/pulls/${pr_number}")"
state="$(jq -r '.state' <<<"$pr_json")"
base_ref="$(jq -r '.base.ref' <<<"$pr_json")"
[[ "$state" == "open" && "$base_ref" == "main" ]] || {
  echo "openなmain向けPRではないため検査を終了します。" >&2
  exit 78
}

{
  printf 'number=%s\n' "$pr_number"
  printf 'url=%s\n' "$(jq -r '.html_url' <<<"$pr_json")"
  printf 'head_sha=%s\n' "$(jq -r '.head.sha' <<<"$pr_json")"
  printf 'base_sha=%s\n' "$(jq -r '.base.sha' <<<"$pr_json")"
} >>"$output_file"
