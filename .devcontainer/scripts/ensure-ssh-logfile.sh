#!/usr/bin/env bash

set -euo pipefail

config_file="/etc/default/ssh"
log_file="/tmp/sshd.log"
desired_opts="-E ${log_file}"
tmp_file="$(mktemp)"
updated_file="$(mktemp)"

cleanup() {
  rm -f -- "${tmp_file}"
  rm -f -- "${updated_file}"
}

trap cleanup EXIT

if ! command -v sudo >/dev/null 2>&1 || ! sudo -n true >/dev/null 2>&1; then
  echo "Passwordless sudo is required to configure sshd logging inside the Dev Container." >&2
  exit 1
fi

sudo -n touch "${config_file}"

if sudo -n test -f "${config_file}"; then
  cat "${config_file}" >"${tmp_file}"
fi

found_sshd_opts=false
while IFS= read -r line || [[ -n "${line}" ]]; do
  if [[ "${line}" == SSHD_OPTS=* ]]; then
    printf 'SSHD_OPTS="%s"\n' "${desired_opts}" >>"${updated_file}"
    found_sshd_opts=true
    continue
  fi

  printf '%s\n' "${line}" >>"${updated_file}"
done <"${tmp_file}"

if ! ${found_sshd_opts}; then
  printf '\nSSHD_OPTS="%s"\n' "${desired_opts}" >>"${updated_file}"
fi

mv "${updated_file}" "${tmp_file}"

if sudo -n cmp -s "${tmp_file}" "${config_file}"; then
  exit 0
fi

sudo -n install -m 0644 "${tmp_file}" "${config_file}"
sudo -n install -m 0600 /dev/null "${log_file}"
sudo -n /etc/init.d/ssh restart
