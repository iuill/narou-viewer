#!/usr/bin/env bash
set -euo pipefail

sha="${1:-}"
state="${2:-}"
description="${3:-機微情報検査}"
repository="${GITHUB_REPOSITORY:-}"

[[ "$sha" =~ ^[0-9a-f]{40}$ && "$state" =~ ^(pending|success|failure|error)$ && -n "$repository" ]] || {
  echo "usage: GITHUB_REPOSITORY=owner/repo $0 <sha> <pending|success|failure|error> [description]" >&2
  exit 2
}

gh api --method POST "repos/${repository}/statuses/${sha}" \
  -f state="$state" \
  -f context="trusted-sensitive-information/metadata" \
  -f description="${description:0:140}" >/dev/null
