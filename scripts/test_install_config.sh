#!/usr/bin/env bash

set -Eeuo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "${repo_root}/scripts/install.sh"

test_root="$(mktemp -d)"
trap 'rm -rf "${test_root}"' EXIT

TMP_DIR="${test_root}/download"
INSTALL_DIR="${test_root}/opt/anixops/control"
CONFIG_FILE="${INSTALL_DIR}/config/config.yaml"
APP_USER="test-user"
ADMIN_EMAIL="owner@acme.test"
ADMIN_PASSWORD="0123456789abcdef0123456789abcdef"
mkdir -p "${TMP_DIR}" "${INSTALL_DIR}/config"

curl() {
  local output=""
  while [[ $# -gt 0 ]]; do
    case "$1" in
      -o)
        output="$2"
        shift 2
        ;;
      *) shift ;;
    esac
  done
  cp "${repo_root}/config/config.prod.yaml" "${output}"
}

chown() { :; }

write_fresh_config

[[ "${FRESH_CONFIG}" -eq 1 ]]
[[ "$(stat -c '%a' "${CONFIG_FILE}")" == "640" ]]
grep -Fq 'email: "owner@acme.test"' "${CONFIG_FILE}"
grep -Fq 'password: "0123456789abcdef0123456789abcdef"' "${CONFIG_FILE}"
grep -Eq 'secret: "[0-9a-f]{64}"' "${CONFIG_FILE}"
grep -Eq 'api_token: "[0-9a-f]{64}"' "${CONFIG_FILE}"
grep -Fq 'database:' "${CONFIG_FILE}"
grep -Fq 'password: "REPLACE-WITH-AT-LEAST-32-RANDOM-CHARACTERS"' "${CONFIG_FILE}"
grep -Fq 'bot_token: "REPLACE-WITH-TELEGRAM-BOT-TOKEN"' "${CONFIG_FILE}"
[[ "$(cat "${INSTALL_DIR}/.bootstrap-admin-password")" == "${ADMIN_PASSWORD}" ]]

echo "control production installer config test passed"
