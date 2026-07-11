#!/usr/bin/env bash
set -euo pipefail

head_sha="${1:-}"
output_dir="${2:-}"
repository="${GITHUB_REPOSITORY:-}"

[[ "$head_sha" =~ ^[0-9a-f]{40}$ && -n "$output_dir" && -n "$repository" ]] || {
  echo "usage: GITHUB_REPOSITORY=owner/repo $0 <head-sha> <output-dir>" >&2
  exit 2
}

mkdir -p "$output_dir"
pr_json="$(gh api --paginate "repos/${repository}/pulls?state=open&base=main&per_page=100" --slurp)"
jq -r --arg sha "$head_sha" '[.[][] | select(.head.sha == $sha)] | sort_by(.number) | .[].number' \
  <<<"$pr_json" >"$output_dir/pr-numbers"
jq -r --arg sha "$head_sha" '[.[][] | select(.head.sha == $sha)] | sort_by(.number) | .[].html_url' \
  <<<"$pr_json" >"$output_dir/pr-urls"
[[ -s "$output_dir/pr-numbers" ]] || {
  echo "対象head SHAを持つopenなmain向けPRが見つかりません。" >&2
  exit 1
}
