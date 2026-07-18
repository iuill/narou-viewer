#!/usr/bin/env bash
set -euo pipefail

source_root="$(git rev-parse --show-toplevel)"
tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

fake_bin="$tmpdir/bin"
mkdir -p "$fake_bin"
cat >"$fake_bin/betterleaks" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" == "version" ]]; then
  echo "1.6.1"
  exit 0
fi
[[ "${1:-}" == "--ignore-gitleaks-allow" ]] || {
  echo "all Betterleaks scans must disable allow markers" >&2
  exit 1
}
shift
if [[ "${1:-}" == "stdin" ]]; then
  if grep -q 'SECRET_TEST_TOKEN'; then
    echo "secret detected (redacted)" >&2
    exit 1
  fi
  exit 0
fi
if [[ "${1:-}" != "git" ]]; then
  exit 0
fi
if [[ " $* " == *" --staged "* ]]; then
  content="$(git diff --cached)"
else
  [[ " $* " == *" --git-workers=1 "* ]] || {
    echo "history scans must use one Betterleaks git worker" >&2
    exit 1
  }
  log_opts=""
  for argument in "$@"; do
    if [[ "$argument" == --log-opts=* ]]; then
      log_opts="${argument#--log-opts=}"
    fi
  done
  [[ "$log_opts" == *"--diff-merges=remerge"* ]] || {
    echo "history scans must include remerge diffs" >&2
    exit 1
  }
  read -r -a log_args <<<"$log_opts"
  content="$(git log "${log_args[@]}" -p)"
fi
if grep -q 'SECRET_TEST_TOKEN' <<<"$content"; then
  echo "secret detected (redacted)" >&2
  exit 1
fi
EOF
chmod +x "$fake_bin/betterleaks"

path_scan_fails() {
  local path="$1"
  if printf '%s\0' "$path" |
    bash "$source_root/scripts/check-sensitive-paths.sh" --stdin0 >/dev/null 2>&1; then
    echo "expected sensitive path scan to fail: $path" >&2
    return 1
  fi
}

path_scan_fails data/private.yaml
path_scan_fails backups/private.tar.gz
path_scan_fails backup-synthetic.tar.gz.age
path_scan_fails tmp/backups/private.tar.gz
path_scan_fails tmp/backup-synthetic.tar.gz.age
path_scan_fails config/.env
path_scan_fails keys/id_ed25519
printf '%s\0' data/.gitkeep config/.env.example src/example.ts |
  bash "$source_root/scripts/check-sensitive-paths.sh" --stdin0

public_ip="8.8.4.$((2 + 2))"
if printf '%s\n' "$public_ip" |
  bash "$source_root/scripts/check-sensitive-content.sh" >/dev/null 2>&1; then
  echo "expected public IPv4 scan to fail" >&2
  exit 1
fi
printf '%s\n' '192.0.2.1' '198.51.100.2' '203.0.113.3' |
  bash "$source_root/scripts/check-sensitive-content.sh"

new_repo() {
  local repo="$1"
  git init -q -b main "$repo"
  git -C "$repo" config user.name test
  git -C "$repo" config user.email test@example.invalid
  mkdir -p "$repo/scripts"
  cp "$source_root/scripts/check-sensitive-content.sh" "$repo/scripts/"
  cp "$source_root/scripts/check-sensitive-paths.sh" "$repo/scripts/"
  cp "$source_root/scripts/install-betterleaks.sh" "$repo/scripts/"
  cp "$source_root/scripts/scan-sensitive-changes.sh" "$repo/scripts/"
}

scan_passes() {
  local repo="$1"
  shift
  (cd "$repo" && PATH="$fake_bin:$PATH" bash ./scripts/scan-sensitive-changes.sh "$@") \
    >/dev/null 2>&1
}

scan_fails() {
  if scan_passes "$@"; then
    echo "expected sensitive information scan to fail: $*" >&2
    return 1
  fi
}

commit_all() {
  local repo="$1"
  local message="$2"
  git -C "$repo" add -A
  git -C "$repo" commit -qm "$message"
}

repo="$tmpdir/staged-and-range"
new_repo "$repo"
printf 'safe\n' >"$repo/example.txt"
commit_all "$repo" baseline
base_sha="$(git -C "$repo" rev-parse HEAD)"

printf 'still safe\n' >"$repo/example.txt"
git -C "$repo" add example.txt
scan_passes "$repo" staged
commit_all "$repo" clean-change
clean_sha="$(git -C "$repo" rev-parse HEAD)"
scan_passes "$repo" range "$base_sha" "$clean_sha"

printf 'SECRET_TEST_TOKEN # betterleaks:allow\n' >"$repo/example.txt"
git -C "$repo" add example.txt
scan_fails "$repo" staged
commit_all "$repo" allow-marker
scan_fails "$repo" range "$clean_sha" HEAD

repo="$tmpdir/pre-push"
remote="$tmpdir/pre-push.git"
new_repo "$repo"
git init -q --bare "$remote"
git -C "$repo" remote add origin "$remote"
printf 'safe\n' >"$repo/example.txt"
commit_all "$repo" baseline
git -C "$repo" push -q origin main
remote_sha="$(git -C "$repo" rev-parse HEAD)"

printf 'SECRET_TEST_TOKEN\n' >"$repo/example.txt"
commit_all "$repo" sensitive-change
local_sha="$(git -C "$repo" rev-parse HEAD)"
if printf 'refs/heads/main %s refs/heads/main %s\n' "$local_sha" "$remote_sha" |
  scan_passes "$repo" pre-push origin; then
  echo "expected pre-push scan to reject a sensitive commit" >&2
  exit 1
fi

echo "sensitive information regression tests passed"
