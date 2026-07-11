#!/usr/bin/env bash
set -euo pipefail

source_root="$(git rev-parse --show-toplevel)"
tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

fake_bin="$tmpdir/bin"
mkdir -p "$fake_bin"
cat >"$fake_bin/gitleaks" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" == "version" ]]; then
  echo "8.30.1"
  exit 0
fi
if [[ "${1:-}" == "git" && " $* " != *" --staged " && " $* " != *"--diff-merges=remerge"* ]]; then
  echo "gitleaks git must receive --diff-merges=remerge" >&2
  exit 1
fi
EOF
chmod +x "$fake_bin/gitleaks"

new_repo() {
  local repo="$1"
  git init -q -b main "$repo"
  git -C "$repo" config user.name test
  git -C "$repo" config user.email test@example.invalid
  mkdir -p "$repo/scripts"
  cp "$source_root/scripts/check-sensitive-content.sh" "$repo/scripts/"
  cp "$source_root/scripts/check-sensitive-paths.sh" "$repo/scripts/"
  cp "$source_root/scripts/install-git-hooks.sh" "$repo/scripts/"
  cp "$source_root/scripts/install-gitleaks.sh" "$repo/scripts/"
  cp "$source_root/scripts/scan-sensitive-changes.sh" "$repo/scripts/"
}

scan_passes() {
  local repo="$1"
  shift
  (cd "$repo" && PATH="$fake_bin:$PATH" bash ./scripts/scan-sensitive-changes.sh "$@") >/dev/null 2>&1
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

public_ip_a="8.8.4.$((2 + 2))"
public_ip_b="9.9.9.$((8 + 1))"

# A remerge diff must detect content added only while resolving a merge conflict.
repo="$tmpdir/merge-resolution"
new_repo "$repo"
printf 'grandfathered=%s\nvalue=base\n' "$public_ip_a" >"$repo/example.txt"
commit_all "$repo" baseline
base_sha="$(git -C "$repo" rev-parse HEAD)"
git -C "$repo" switch -qc feature
printf 'grandfathered=%s\nvalue=feature\n' "$public_ip_a" >"$repo/example.txt"
commit_all "$repo" feature
git -C "$repo" switch -q main
printf 'grandfathered=%s\nvalue=main\n' "$public_ip_a" >"$repo/example.txt"
commit_all "$repo" main
if git -C "$repo" merge --no-edit feature >/dev/null 2>&1; then
  echo "expected a merge conflict" >&2
  exit 1
fi
printf 'grandfathered=%s\nvalue=resolved\n++ %s\n' "$public_ip_a" "$public_ip_b" >"$repo/example.txt"
commit_all "$repo" merge
scan_fails "$repo" range "$base_sha" HEAD

# Inheriting a grandfathered address through a merge must not fail by itself.
repo="$tmpdir/merge-grandfathered"
new_repo "$repo"
printf 'grandfathered=%s\nvalue=base\n' "$public_ip_a" >"$repo/example.txt"
commit_all "$repo" baseline
base_sha="$(git -C "$repo" rev-parse HEAD)"
git -C "$repo" switch -qc feature
printf 'grandfathered=%s\nvalue=feature\n' "$public_ip_a" >"$repo/example.txt"
commit_all "$repo" feature
git -C "$repo" switch -q main
printf 'grandfathered=%s\nvalue=main\n' "$public_ip_a" >"$repo/example.txt"
commit_all "$repo" main
git -C "$repo" merge -q --no-edit -s ours feature
scan_passes "$repo" range "$base_sha" HEAD

# A line whose content starts with "++ " must still reach the added-line checker.
repo="$tmpdir/plus-prefix"
new_repo "$repo"
printf 'safe\n' >"$repo/example.txt"
commit_all "$repo" baseline
printf 'safe\n++ %s\n' "$public_ip_b" >"$repo/example.txt"
git -C "$repo" add example.txt
scan_fails "$repo" staged

# Deleting or renaming a prohibited path out is remediation; adding or renaming in is rejected.
repo="$tmpdir/path-remediation"
new_repo "$repo"
mkdir -p "$repo/data"
printf 'synthetic\n' >"$repo/data/private.txt"
commit_all "$repo" baseline
base_sha="$(git -C "$repo" rev-parse HEAD)"
git -C "$repo" mv data/private.txt safe.txt
commit_all "$repo" rename-out
scan_passes "$repo" range "$base_sha" HEAD
base_sha="$(git -C "$repo" rev-parse HEAD)"
mkdir -p "$repo/data"
git -C "$repo" mv safe.txt data/private.txt
commit_all "$repo" rename-in
scan_fails "$repo" range "$base_sha" HEAD
base_sha="$(git -C "$repo" rev-parse HEAD)"
git -C "$repo" rm -q data/private.txt
commit_all "$repo" delete
scan_passes "$repo" range "$base_sha" HEAD

# Existing-branch (including force-push) and new-branch pre-push ranges inspect added content.
repo="$tmpdir/pre-push"
remote="$tmpdir/pre-push.git"
new_repo "$repo"
git init -q --bare "$remote"
git -C "$repo" remote add origin "$remote"
printf 'safe\n' >"$repo/example.txt"
commit_all "$repo" baseline
git -C "$repo" push -q origin main
remote_sha="$(git -C "$repo" rev-parse HEAD)"
printf 'safe\n++ %s\n' "$public_ip_b" >"$repo/example.txt"
commit_all "$repo" sensitive
local_sha="$(git -C "$repo" rev-parse HEAD)"
zero_sha="0000000000000000000000000000000000000000"
if printf 'refs/heads/main %s refs/heads/main %s\n' "$local_sha" "$remote_sha" |
  scan_passes "$repo" pre-push origin; then
  echo "expected existing-branch pre-push scan to fail" >&2
  exit 1
fi
if printf 'refs/heads/new %s refs/heads/new %s\n' "$local_sha" "$zero_sha" |
  scan_passes "$repo" pre-push origin; then
  echo "expected new-branch pre-push scan to fail" >&2
  exit 1
fi

# An effective global or worktree hooksPath must not be shadowed or reported as installed.
repo="$tmpdir/hooks-path"
new_repo "$repo"
printf 'synthetic\n' >"$repo/example.txt"
commit_all "$repo" baseline
global_config="$tmpdir/global.gitconfig"
git config --file "$global_config" core.hooksPath global-hooks
if (cd "$repo" && GIT_CONFIG_GLOBAL="$global_config" PATH="$fake_bin:$PATH" bash ./scripts/install-git-hooks.sh) >/dev/null 2>&1; then
  echo "expected global core.hooksPath to be preserved" >&2
  exit 1
fi
[[ -z "$(git -C "$repo" config --local --get core.hooksPath || true)" ]]
GIT_CONFIG_GLOBAL=/dev/null git -C "$repo" config extensions.worktreeConfig true
GIT_CONFIG_GLOBAL=/dev/null git -C "$repo" config --worktree core.hooksPath worktree-hooks
if (cd "$repo" && GIT_CONFIG_GLOBAL=/dev/null PATH="$fake_bin:$PATH" bash ./scripts/install-git-hooks.sh) >/dev/null 2>&1; then
  echo "expected worktree core.hooksPath to be preserved" >&2
  exit 1
fi

echo "sensitive information checks: ok"
