#!/usr/bin/env bash
set -euo pipefail

# A local checkout is used only when this file is one: a regular file with the
# repository's install.sh next to it. Piped ("curl ... | bash") $0 is "bash" and
# BASH_SOURCE[0] is empty, so an ./install.sh in the caller's directory is never
# taken for the installer and the tag pin below always applies. The sibling
# install.sh, not scripts/install.sh, is what runs: it refuses the removed
# source installer (ANIX_CONTROL_LEGACY_SOURCE_INSTALL=1) and then runs the
# checkout's scripts/install.sh.
source_file="${BASH_SOURCE[0]:-}"
if [[ -n "${source_file}" && -f "${source_file}" ]]; then
  SCRIPT_DIR="$(CDPATH='' cd -- "$(dirname -- "${source_file}")" && pwd)"
  if [[ -f "${SCRIPT_DIR}/install.sh" ]]; then
    exec bash "${SCRIPT_DIR}/install.sh" "$@"
  fi
fi

REPO_OWNER="${REPO_OWNER:-AnixOps}"
REPO_NAME="${REPO_NAME:-anix-control}"
# The installer is fetched at the tag it installs, never from a moving branch.
# The tag is --version (or ANIX_CONTROL_VERSION). INSTALL_REF overrides it, for
# a developer who knowingly installs another ref, and for commands that take no
# --version (preflight, enable-agents).
INSTALL_REF="${INSTALL_REF:-}"
if [[ -z "${INSTALL_REF}" ]]; then
  INSTALL_REF="${ANIX_CONTROL_VERSION:-${V2BOARD_VERSION:-}}"
  previous=""
  for arg in "$@"; do
    if [[ "${previous}" == "--version" ]]; then
      INSTALL_REF="${arg}"
    fi
    previous="${arg}"
  done
  if [[ "${previous}" == "--version" ]]; then
    printf '[ERROR] --version needs a release tag: --version vX.Y.Z (for example v4.2.0).\n' >&2
    exit 1
  fi
  if [[ ! "${INSTALL_REF}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-(alpha|beta|rc)(\.[0-9]+)?)?$ ]]; then
    printf '[ERROR] Name the release tag to install: --version vX.Y.Z (for example v4.2.0).\n' >&2
    printf '        The installer is fetched at that tag, never from a moving branch.\n' >&2
    printf '        Commands without --version (preflight, enable-agents) take INSTALL_REF=<tag>.\n' >&2
    exit 1
  fi
fi
RAW_URL="https://raw.githubusercontent.com/${REPO_OWNER}/${REPO_NAME}/${INSTALL_REF}/scripts/install.sh"

tmp_script="$(mktemp)"
trap 'rm -f "${tmp_script}"' EXIT

curl -fsSL "${RAW_URL}" -o "${tmp_script}"
exec bash "${tmp_script}" "$@"
