#!/usr/bin/env bash
# AnixOps Control installer entry point.
#
# Container deployment (Docker Compose or Kubernetes) is the primary path:
# see docs/DEPLOYMENT.md. This script keeps the one-command systemd install.
# It downloads the release installer (scripts/install.sh) at the release tag it
# installs and never builds on the target host.
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
# The installer is fetched at the tag it installs, never from a moving branch.
# The tag is --version (or ANIX_CONTROL_VERSION). INSTALL_REF overrides it, for
# a developer who knowingly installs another ref, and for commands that take no
# --version (preflight, enable-agents).
install_ref="${INSTALL_REF:-}"
if [[ -z "${install_ref}" ]]; then
  install_ref="${ANIX_CONTROL_VERSION:-${V2BOARD_VERSION:-}}"
  previous=""
  for arg in "$@"; do
    if [[ "${previous}" == "--version" ]]; then
      install_ref="${arg}"
    fi
    previous="${arg}"
  done
  if [[ ! "${install_ref}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-(alpha|beta|rc)(\.[0-9]+)?)?$ ]]; then
    printf '[ERROR] Name the release tag to install: --version vX.Y.Z (for example v4.2.0).\n' >&2
    printf '        The installer is fetched at that tag, never from a moving branch.\n' >&2
    printf '        Commands without --version (preflight, enable-agents) take INSTALL_REF=<tag>.\n' >&2
    exit 1
  fi
fi
installer_url="https://raw.githubusercontent.com/AnixOps/anix-control/${install_ref}/scripts/install.sh"
installer_file="$(mktemp)"
trap 'rm -f "${installer_file}"' EXIT
curl -fsSL "${installer_url}" -o "${installer_file}"
bash "${installer_file}" "$@"
