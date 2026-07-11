#!/usr/bin/env bash
set -euo pipefail

sha="${1:-}"
expected_target_url="${2:-}"
repository="${GITHUB_REPOSITORY:-}"

[[ "$sha" =~ ^[0-9a-f]{40}$ && "$expected_target_url" == https://github.com/* && -n "$repository" ]] || {
  echo "usage: GITHUB_REPOSITORY=owner/repo $0 <sha> <workflow-run-url>" >&2
  exit 2
}

latest_target_url="$(gh api "repos/${repository}/commits/${sha}/statuses" \
  --jq '[.[] | select(.context == "sensitive-information/metadata-advisory")][0].target_url // ""')"
[[ "$latest_target_url" == "$expected_target_url" ]] || {
  echo "より新しい機微情報検査が開始されたため、この結果は発行しません。" >&2
  exit 1
}
