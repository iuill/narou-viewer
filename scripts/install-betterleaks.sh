#!/usr/bin/env bash
set -euo pipefail

readonly version="1.6.1"
install_dir="${BETTERLEAKS_INSTALL_DIR:-${HOME}/.local/bin}"
binary="$install_dir/betterleaks"

if [[ -x "$binary" && "$($binary version 2>/dev/null | grep -Eo '[0-9]+(\.[0-9]+)+' | head -n1)" == "$version" ]]; then
  exit 0
fi

case "$(uname -s)" in
  Linux) os="linux" ;;
  *) echo "Betterleaks の自動導入に未対応のOSです: $(uname -s)" >&2; exit 1 ;;
esac

case "$(uname -m)" in
  x86_64|amd64) arch="x64" ;;
  arm64|aarch64) arch="arm64" ;;
  *) echo "Betterleaks の自動導入に未対応のCPUです: $(uname -m)" >&2; exit 1 ;;
esac

archive="betterleaks_${version}_${os}_${arch}.tar.gz"
case "${os}_${arch}" in
  linux_x64) checksum="fbefc700a0bd4522cc952dd2a8f259cdb80526d7e60114aca19bb2d6fdc80f81" ;;
  linux_arm64) checksum="bab9688ba968264ace67b608fc7a7d8f5e61218cde70029d32cbc894e3808fdf" ;;
esac

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT
curl -fsSL "https://github.com/betterleaks/betterleaks/releases/download/v${version}/${archive}" -o "$tmpdir/$archive"
echo "$checksum  $tmpdir/$archive" | sha256sum -c - >/dev/null
mkdir -p "$install_dir"
tar -xzf "$tmpdir/$archive" -C "$tmpdir" betterleaks
install -m 0755 "$tmpdir/betterleaks" "$binary"

echo "Betterleaks v${version} を $binary に導入しました。"
