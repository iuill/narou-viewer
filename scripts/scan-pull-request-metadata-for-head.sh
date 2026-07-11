#!/usr/bin/env bash
set -euo pipefail

head_sha="${1:-}"
output_dir="${2:-}"

bash ./scripts/list-pull-requests-for-head.sh "$head_sha" "$output_dir"
while IFS= read -r pr_url; do
  bash ./scripts/scan-pull-request-content.sh "$pr_url"
done <"$output_dir/pr-urls"
