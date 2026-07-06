#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${script_dir}/e2e-compose-common.sh"

initialize_e2e_compose_environment

run_e2e_compose "$@"
