#!/usr/bin/env bash
set -euo pipefail

artifact_name="${ARTIFACT_NAME:?ARTIFACT_NAME is required}"
output_dir="${ARTIFACT_OUTPUT_DIR:-.tmp/e2e-binaries}"
timeout_seconds="${ARTIFACT_WAIT_TIMEOUT_SECONDS:-300}"
executable_path="${ARTIFACT_EXECUTABLE_PATH:-}"

mkdir -p "${output_dir}"
deadline=$((SECONDS + timeout_seconds))
attempt=1
artifact_url=""

while [[ -z "${artifact_url}" ]]; do
  artifact_url="$(
    gh api "repos/${GITHUB_REPOSITORY}/actions/runs/${GITHUB_RUN_ID}/artifacts?per_page=100" \
      --jq ".artifacts[] | select(.name == \"${artifact_name}\" and .expired == false) | .archive_download_url" |
      head -n 1
  )"

  if [[ -n "${artifact_url}" ]]; then
    break
  fi

  if ((SECONDS >= deadline)); then
    echo "Timed out waiting for ${artifact_name}" >&2
    exit 1
  fi

  echo "Waiting for ${artifact_name} (${attempt})"
  attempt=$((attempt + 1))
  sleep 5
done

zip_path="${output_dir}/${artifact_name}.zip"
gh api "${artifact_url}" >"${zip_path}"
unzip -o "${zip_path}" -d "${output_dir}"
rm "${zip_path}"

if [[ -n "${executable_path}" ]]; then
  chmod +x "${executable_path}"
fi
