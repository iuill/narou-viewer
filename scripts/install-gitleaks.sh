#!/usr/bin/env bash
set -euo pipefail

readonly version="8.30.1"
install_dir="${GITLEAKS_INSTALL_DIR:-${HOME}/.local/bin}"
binary="$install_dir/gitleaks"

if [[ -x "$binary" && "$($binary version 2>/dev/null | grep -Eo '[0-9]+(\.[0-9]+)+' | head -n1)" == "$version" ]]; then
  exit 0
fi

case "$(uname -s)" in
  Linux) os="linux" ;;
  *) echo "Gitleaks の自動導入に未対応のOSです: $(uname -s)" >&2; exit 1 ;;
esac

case "$(uname -m)" in
  x86_64|amd64) arch="x64" ;;
  arm64|aarch64) arch="arm64" ;;
  *) echo "Gitleaks の自動導入に未対応のCPUです: $(uname -m)" >&2; exit 1 ;;
esac

archive="gitleaks_${version}_${os}_${arch}.tar.gz"
case "${os}_${arch}" in
  linux_x64) checksum="551f6fc83ea457d62a0d98237cbad105af8d557003051f41f3e7ca7b3f2470eb" ;;
  linux_arm64) checksum="e4a487ee7ccd7d3a7f7ec08657610aa3606637dab924210b3aee62570fb4b080" ;;
esac

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT
curl -fsSL "https://github.com/gitleaks/gitleaks/releases/download/v${version}/${archive}" -o "$tmpdir/$archive"
echo "$checksum  $tmpdir/$archive" | sha256sum -c - >/dev/null
mkdir -p "$install_dir"
tar -xzf "$tmpdir/$archive" -C "$tmpdir" gitleaks
install -m 0755 "$tmpdir/gitleaks" "$binary"

echo "Gitleaks v${version} を $binary に導入しました。"
