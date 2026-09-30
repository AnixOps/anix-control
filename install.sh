#!/usr/bin/env bash
# AnixOps Control installer entry point.
#
# Container deployment (Docker Compose or Kubernetes) is the primary path:
# see docs/DEPLOYMENT.md. This script keeps the one-command systemd install,
# which is frozen: it still works but gets no new features. It downloads the
# release installer (scripts/install.sh) and never builds on the target host.
set -euo pipefail

if [[ "${ANIX_CONTROL_LEGACY_SOURCE_INSTALL:-${V2BOARD_LEGACY_SOURCE_INSTALL:-0}}" == "1" ]]; then
  printf '[ERROR] The source/Docker Compose installer was removed because it built images on the target host.\n' >&2
  printf '        Deploy the released image instead: https://github.com/AnixOps/anix-control/blob/go_dev/docs/DEPLOYMENT.md\n' >&2
  exit 1
fi

script_dir="$(CDPATH='' cd -- "$(dirname -- "$0")" 2>/dev/null && pwd || true)"
if [[ -n "${script_dir}" && -f "${script_dir}/scripts/install.sh" ]]; then
  exec bash "${script_dir}/scripts/install.sh" "$@"
fi

command -v curl >/dev/null 2>&1 || {
  printf '[ERROR] curl is required to download the release installer.\n' >&2
  exit 1
}
install_ref="${INSTALL_REF:-go_dev}"
installer_url="https://raw.githubusercontent.com/AnixOps/anix-control/${install_ref}/scripts/install.sh"
installer_file="$(mktemp)"
trap 'rm -f "${installer_file}"' EXIT
curl -fsSL "${installer_url}" -o "${installer_file}"
bash "${installer_file}" "$@"
