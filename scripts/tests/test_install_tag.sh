#!/usr/bin/env bash
# The systemd installer installs the release tag it is given and never resolves
# a moving "latest" release. The one-command forwarders (install.sh and
# panel_install.sh) fetch the installer at that tag, not from a branch.

# COMMAND and VERSION are read by the sourced installer's functions.
# shellcheck disable=SC1091,SC2034
set -Eeuo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
workdir="$(mktemp -d)"
cleanup() { find "${workdir}" -depth -delete; }
trap cleanup EXIT

fail() {
  printf 'FAIL: %s\n' "$*" >&2
  exit 1
}

# scripts/install.sh: the version is checked before the host is touched.
(
  source "${REPO_ROOT}/scripts/install.sh"

  if declare -F latest_release >/dev/null; then
    fail "latest_release is back: the installer must not resolve a moving latest release"
  fi

  COMMAND="install"
  VERSION=""
  if (require_version) >/dev/null 2>"${workdir}/no-version.err"; then
    fail "install without --version must fail"
  fi
  grep -F -- "--version" "${workdir}/no-version.err" >/dev/null || fail "the error must name --version"

  VERSION="latest"
  if (require_version) >/dev/null 2>&1; then
    fail "--version latest must be refused: it is not a release tag"
  fi

  VERSION="v4.2.0"
  require_version
  VERSION="v4.2.0-rc.4"
  require_version
)

if grep -nF "releases/latest" "${REPO_ROOT}/scripts/install.sh"; then
  fail "scripts/install.sh must not read releases/latest"
fi

# The forwarders run in a directory that has no scripts/install.sh next to
# them, so they take the download path. A stub curl records the URL.
mkdir -p "${workdir}/bin"
CURL_LOG="${workdir}/curl.log"
cat >"${workdir}/bin/curl" <<'EOF'
#!/usr/bin/env bash
# Record the URL, and write a stub installer to the -o target.
out=""
url=""
while [[ "$#" -gt 0 ]]; do
  case "$1" in
    -o) out="$2"; shift 2 ;;
    -*) shift ;;
    *) url="$1"; shift ;;
  esac
done
printf '%s\n' "${url}" >>"${CURL_LOG}"
printf '#!/usr/bin/env bash\nprintf "stub-installer %%s\\n" "$*"\n' >"${out}"
EOF
chmod 0755 "${workdir}/bin/curl"

base="https://raw.githubusercontent.com/AnixOps/anix-control"

for forwarder in install.sh panel_install.sh; do
  dir="${workdir}/${forwarder%.sh}"
  mkdir -p "${dir}"
  cp "${REPO_ROOT}/${forwarder}" "${dir}/${forwarder}"
  script="${dir}/${forwarder}"

  # The tag comes from --version, and the installer is fetched at that tag.
  : >"${CURL_LOG}"
  out="$(env -u INSTALL_REF -u ANIX_CONTROL_VERSION -u V2BOARD_VERSION -u REPO_OWNER -u REPO_NAME \
    PATH="${workdir}/bin:${PATH}" CURL_LOG="${CURL_LOG}" \
    bash "${script}" install --version v4.2.0 --admin-email admin@example.com)"
  [[ "$(head -n 1 "${CURL_LOG}")" == "${base}/v4.2.0/scripts/install.sh" ]] ||
    fail "${forwarder}: --version v4.2.0 must fetch the installer at v4.2.0, got $(head -n 1 "${CURL_LOG}")"
  [[ "${out}" == "stub-installer install --version v4.2.0 --admin-email admin@example.com" ]] ||
    fail "${forwarder}: the installer must receive the original arguments, got: ${out}"

  # A command that names no tag is refused, and nothing is downloaded.
  : >"${CURL_LOG}"
  if env -u INSTALL_REF -u ANIX_CONTROL_VERSION -u V2BOARD_VERSION -u REPO_OWNER -u REPO_NAME \
    PATH="${workdir}/bin:${PATH}" CURL_LOG="${CURL_LOG}" \
    bash "${script}" install --admin-email admin@example.com >/dev/null 2>"${workdir}/forwarder.err"; then
    fail "${forwarder}: a command without a tag must fail"
  fi
  [[ ! -s "${CURL_LOG}" ]] || fail "${forwarder}: nothing may be downloaded without a tag"
  grep -F -- "--version" "${workdir}/forwarder.err" >/dev/null ||
    fail "${forwarder}: the error must name --version"

  # "latest" is not a tag.
  : >"${CURL_LOG}"
  if env -u INSTALL_REF -u ANIX_CONTROL_VERSION -u V2BOARD_VERSION -u REPO_OWNER -u REPO_NAME \
    PATH="${workdir}/bin:${PATH}" CURL_LOG="${CURL_LOG}" \
    bash "${script}" install --version latest >/dev/null 2>&1; then
    fail "${forwarder}: --version latest must be refused"
  fi
  [[ ! -s "${CURL_LOG}" ]] || fail "${forwarder}: nothing may be downloaded for --version latest"

  # The tag may come from the environment instead of the flag.
  : >"${CURL_LOG}"
  env -u INSTALL_REF -u V2BOARD_VERSION -u REPO_OWNER -u REPO_NAME \
    PATH="${workdir}/bin:${PATH}" CURL_LOG="${CURL_LOG}" ANIX_CONTROL_VERSION=v4.2.0-rc.4 \
    bash "${script}" install >/dev/null
  [[ "$(head -n 1 "${CURL_LOG}")" == "${base}/v4.2.0-rc.4/scripts/install.sh" ]] ||
    fail "${forwarder}: ANIX_CONTROL_VERSION must select the tag, got $(head -n 1 "${CURL_LOG}")"

  # INSTALL_REF overrides, for commands that take no --version.
  : >"${CURL_LOG}"
  env -u ANIX_CONTROL_VERSION -u V2BOARD_VERSION -u REPO_OWNER -u REPO_NAME \
    PATH="${workdir}/bin:${PATH}" CURL_LOG="${CURL_LOG}" INSTALL_REF=v4.1.0 \
    bash "${script}" enable-agents --grpc-name grpc.example.com >/dev/null
  [[ "$(head -n 1 "${CURL_LOG}")" == "${base}/v4.1.0/scripts/install.sh" ]] ||
    fail "${forwarder}: INSTALL_REF must select the ref, got $(head -n 1 "${CURL_LOG}")"
done

echo "install-by-tag test passed"
