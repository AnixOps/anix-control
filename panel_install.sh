#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)"

if [[ -f "${SCRIPT_DIR}/install.sh" ]]; then
  exec bash "${SCRIPT_DIR}/install.sh" "$@"
fi

REPO_OWNER="${REPO_OWNER:-AnixOps}"
REPO_NAME="${REPO_NAME:-v2board_AnixOps}"
INSTALL_REF="${INSTALL_REF:-go_dev}"
RAW_URL="https://raw.githubusercontent.com/${REPO_OWNER}/${REPO_NAME}/${INSTALL_REF}/install.sh"

tmp_script="$(mktemp)"
trap 'rm -f "${tmp_script}"' EXIT

curl -fsSL "${RAW_URL}" -o "${tmp_script}"
exec bash "${tmp_script}" "$@"
