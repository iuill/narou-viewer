#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

current_hooks_path="$(git config --local --get core.hooksPath || true)"
if [[ -n "$current_hooks_path" && "$current_hooks_path" != ".githooks" ]]; then
  echo "既存の core.hooksPath ($current_hooks_path) は上書きしません。" >&2
  echo "内容を確認し、不要なら git config --local --unset core.hooksPath の後に再実行してください。" >&2
  exit 1
fi

if ! command -v gitleaks >/dev/null 2>&1; then
  echo "Gitleaks が PATH にありません。先に scripts/install-gitleaks.sh を実行してください。" >&2
  exit 1
fi
if [[ "$(gitleaks version 2>/dev/null | grep -Eo '[0-9]+(\.[0-9]+)+' | head -n1)" != "8.30.1" ]]; then
  echo "Gitleaks v8.30.1 が必要です。" >&2
  exit 1
fi
git config --local core.hooksPath .githooks

echo "Git hooks を有効化しました: .githooks"
