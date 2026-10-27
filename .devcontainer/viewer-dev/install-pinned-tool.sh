#!/usr/bin/env bash
set -euo pipefail

tool="${1:?tool is required}"
version="${2:?version is required}"
install_dir="${3:?install directory is required}"

case "${tool}:${version}:$(uname -m)" in
  bun:1.4.4:x86_64)
    archive="bun-linux-x64.zip"
    checksum="bf8ccbfb2b0b80159e0f9a6d15c55317add8077534dbc6130a3edb52fccb395c"
    ;;
  bun:1.4.4:aarch64|bun:1.4.4:arm64)
    archive="bun-linux-aarch64.zip"
    checksum="baa6bb36f51d6e58561e02bbea825038a9df454f673d6d776e8bbf86f8c7850e"
    ;;
  uv:0.12.23:x86_64)
    archive="uv-x86_64-unknown-linux-gnu.tar.gz"
    checksum="9167d72b3319674b6303c4cbe071854bba13ebdf3d76b1a7cbdc175471fb66d6"
    ;;
  uv:0.12.23:aarch64|uv:0.12.23:arm64)
    archive="uv-aarch64-unknown-linux-gnu.tar.gz"
    checksum="6524bd338177ed50d035d39354e12545e993bbeba2ecbddf0480c5b3a81d313f"
    ;;
  *)
    echo "Unsupported tool version or architecture; review and pin its checksum first: ${tool}@${version}" >&2
    exit 1
    ;;
esac

case "${tool}" in
  bun) url="https://github.com/oven-sh/bun/releases/download/bun-v${version}/${archive}" ;;
  uv) url="https://github.com/astral-sh/uv/releases/download/${version}/${archive}" ;;
esac

temporary_dir="$(mktemp -d)"
trap 'rm -rf "$temporary_dir"' EXIT
curl -fsSL "${url}" -o "${temporary_dir}/${archive}"
echo "${checksum}  ${temporary_dir}/${archive}" | sha256sum -c -
mkdir -p "${install_dir}"

case "${tool}" in
  bun)
    unzip -q "${temporary_dir}/${archive}" -d "${temporary_dir}"
    install -m 0755 "${temporary_dir}/${archive%.zip}/bun" "${install_dir}/bun"
    ;;
  uv)
    tar -xzf "${temporary_dir}/${archive}" -C "${temporary_dir}"
    install -m 0755 "${temporary_dir}/${archive%.tar.gz}/uv" "${install_dir}/uv"
    install -m 0755 "${temporary_dir}/${archive%.tar.gz}/uvx" "${install_dir}/uvx"
    ;;
esac
