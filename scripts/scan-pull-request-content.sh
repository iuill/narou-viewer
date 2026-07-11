#!/usr/bin/env bash
set -euo pipefail

pr_url="${1:-}"
if [[ ! "$pr_url" =~ ^https://github\.com/[^/]+/[^/]+/pull/[0-9]+$ ]]; then
  echo "GitHub pull request URLを指定してください。" >&2
  exit 2
fi

readonly expected_version="1.6.1"
detected_version="$(betterleaks version 2>/dev/null | grep -Eo '[0-9]+(\.[0-9]+)+' | head -n1 || true)"
if [[ "$detected_version" != "$expected_version" ]]; then
  repo_root="$(git rev-parse --show-toplevel)"
  bash "$repo_root/scripts/install-betterleaks.sh"
  export PATH="${HOME}/.local/bin:${PATH}"
fi

betterleaks --ignore-gitleaks-allow github --redact=100 --no-banner "$pr_url"
