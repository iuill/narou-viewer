#!/usr/bin/env bash
set -euo pipefail

failed=0

check_path() {
  path="$1"
  [[ -z "$path" ]] && return
  basename="${path##*/}"

  case "$path" in
    data/.gitkeep) return ;;
  esac

  if [[ "$path" == data/* || "$path" == deploy/runtime/* ]]; then
    echo "commit禁止のruntime/private data pathです: $path" >&2
    failed=1
  fi

  case "$basename" in
    .env.sample|.env.example) ;;
    .env|.env.*|.envrc|id_rsa|id_ed25519|credentials|credentials.json|secrets.yml|secrets.yaml)
      echo "commit禁止の機微ファイル名です: $path" >&2
      failed=1
      ;;
  esac

  case "$basename" in
    *.pem|*.p12|*.pfx)
      echo "commit禁止の秘密鍵・証明書候補です: $path" >&2
      failed=1
      ;;
  esac
}

if [[ "${1:-}" == "--stdin0" ]]; then
  while IFS= read -r -d '' path; do
    check_path "$path"
  done
else
  for path in "$@"; do
    check_path "$path"
  done
fi

exit "$failed"
