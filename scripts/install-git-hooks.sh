#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

current_hooks_path="$(git config --get core.hooksPath || true)"
current_hooks_origin="$(git config --show-origin --get core.hooksPath 2>/dev/null | awk '{print $1}' || true)"
if [[ -n "$current_hooks_path" && "$current_hooks_path" != ".githooks" ]]; then
  echo "既存の core.hooksPath ($current_hooks_path, ${current_hooks_origin:-origin不明}) は上書きしません。" >&2
  echo "内容を確認し、不要なら git config --local --unset core.hooksPath の後に再実行してください。" >&2
  exit 1
fi

if ! command -v betterleaks >/dev/null 2>&1; then
  echo "Betterleaks が PATH にありません。先に scripts/install-betterleaks.sh を実行してください。" >&2
  exit 1
fi
if [[ "$(betterleaks version 2>/dev/null | grep -Eo '[0-9]+(\.[0-9]+)+' | head -n1)" != "1.6.1" ]]; then
  echo "Betterleaks v1.6.1 が必要です。" >&2
  exit 1
fi
git config --local core.hooksPath .githooks

effective_hooks_path="$(git config --get core.hooksPath || true)"
if [[ "$effective_hooks_path" != ".githooks" ]]; then
  echo "core.hooksPath の有効値を .githooks に設定できませんでした。" >&2
  exit 1
fi

echo "Git hooks を有効化しました: .githooks"
