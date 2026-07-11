#!/usr/bin/env bash
set -euo pipefail

pr_number="${1:-}"
output_file="${GITHUB_OUTPUT:-/dev/stdout}"
repository="${GITHUB_REPOSITORY:-}"

[[ "$pr_number" =~ ^[1-9][0-9]*$ && -n "$repository" ]] || {
  echo "pull request number と GITHUB_REPOSITORY が必要です。" >&2
  exit 2
}

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
