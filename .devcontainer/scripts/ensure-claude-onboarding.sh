#!/usr/bin/env bash

set -euo pipefail

config_file="${1:-${HOME}/.claude.json}"

if [ -e "${config_file}" ] && [ ! -f "${config_file}" ]; then
  echo "Claude config path is not a regular file: ${config_file}" >&2
  exit 1
fi

umask 077
node - "${config_file}" <<'NODE'
const fs = require("node:fs");

const configFile = process.argv[2];
const config = fs.existsSync(configFile)
  ? JSON.parse(fs.readFileSync(configFile, "utf8"))
  : {};

config.hasCompletedOnboarding = true;
fs.writeFileSync(configFile, `${JSON.stringify(config, null, 2)}\n`, {
  mode: 0o600,
});
NODE

chmod 600 "${config_file}"
