#!/usr/bin/env bash
set -euo pipefail

sha="${1:-}"
state="${2:-}"
description="${3:-機微情報検査}"
target_url="${4:-}"
repository="${GITHUB_REPOSITORY:-}"
context="${STATUS_CONTEXT:-sensitive-information/metadata-advisory}"

[[ "$sha" =~ ^[0-9a-f]{40}$ && "$state" =~ ^(pending|success|failure|error)$ && -n "$repository" ]] || {
  echo "usage: GITHUB_REPOSITORY=owner/repo $0 <sha> <pending|success|failure|error> [description]" >&2
  exit 2
}

args=(
  --method POST
  "repos/${repository}/statuses/${sha}"
  -f "state=$state"
  -f "context=$context"
  -f "description=${description:0:140}"
)
[[ -z "$target_url" ]] || args+=(-f "target_url=$target_url")
gh api "${args[@]}" >/dev/null
