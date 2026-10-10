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
# The forwarders download the installer into a temporary file; keep it (and
# whatever the forwarders leave behind) inside the work directory.
mkdir -p "${workdir}/tmp"
export TMPDIR="${workdir}/tmp"

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

# A flag that needs a value and gets none says so. `shift 2` with one argument
# left used to end the script with status 1 and no message. Each run is a
# separate process under a timeout, with errexit as the installer has it.
parse_flags() {
  # shellcheck disable=SC2016  # the program is for the inner shell
  timeout 20 bash -c 'set -Eeuo pipefail; source "$1"; shift; parse_args "$@"' _ \
    "${REPO_ROOT}/scripts/install.sh" "$@"
}
for flag in --version --admin-email --admin-password --install-dir --health-url --grpc-name; do
  status=0
  parse_flags install "${flag}" >/dev/null 2>"${workdir}/bare-flag.err" || status=$?
  [[ "${status}" -eq 1 ]] || fail "a bare ${flag} must exit 1, got ${status}"
  grep -F -- "${flag} requires" "${workdir}/bare-flag.err" >/dev/null ||
    fail "a bare ${flag} must say that it requires a value, got: $(cat "${workdir}/bare-flag.err")"
done
parse_flags install --version >/dev/null 2>"${workdir}/bare-flag.err" || true
grep -F -- "for example --version v4.2.0" "${workdir}/bare-flag.err" >/dev/null ||
  fail "a bare --version must name an example tag, got: $(cat "${workdir}/bare-flag.err")"
# A flag with its value still parses.
parse_flags install --version v4.2.0 --admin-email admin@example.com >/dev/null 2>&1 ||
  fail "flags with values must still parse"

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

  # "--version" with no tag is refused before anything is downloaded, even when
  # the environment names one: the flag is not silently ignored.
  for tag_env in "" v4.2.0; do
    : >"${CURL_LOG}"
    if env -u INSTALL_REF -u ANIX_CONTROL_VERSION -u V2BOARD_VERSION -u REPO_OWNER -u REPO_NAME \
      PATH="${workdir}/bin:${PATH}" CURL_LOG="${CURL_LOG}" ${tag_env:+ANIX_CONTROL_VERSION="${tag_env}"} \
      bash "${script}" install --admin-email admin@example.com --version >/dev/null 2>"${workdir}/forwarder.err"; then
      fail "${forwarder}: a bare --version must fail (ANIX_CONTROL_VERSION='${tag_env}')"
    fi
    [[ ! -s "${CURL_LOG}" ]] || fail "${forwarder}: nothing may be downloaded for a bare --version"
    grep -F -- "--version needs a release tag" "${workdir}/forwarder.err" >/dev/null ||
      fail "${forwarder}: a bare --version must say what is missing, got: $(cat "${workdir}/forwarder.err")"
  done

  # Piped ("curl ... | bash") or run from a process substitution, the forwarder
  # is not a checkout, whatever the caller's directory holds: $0 is "bash" and
  # dirname is ".", so a ./scripts/install.sh (or ./install.sh) there used to be
  # executed instead of the installer at the tag.
  decoy="${workdir}/decoy-${forwarder%.sh}"
  mkdir -p "${decoy}/scripts"
  for file in scripts/install.sh install.sh; do
    printf '#!/usr/bin/env bash\necho "decoy-installer-ran"\n' >"${decoy}/${file}"
  done
  : >"${CURL_LOG}"
  out="$(cd "${decoy}" && env -u INSTALL_REF -u ANIX_CONTROL_VERSION -u V2BOARD_VERSION -u REPO_OWNER -u REPO_NAME \
    PATH="${workdir}/bin:${PATH}" CURL_LOG="${CURL_LOG}" \
    bash -s -- install --version v4.2.0 --admin-email admin@example.com <"${script}")"
  [[ "$(head -n 1 "${CURL_LOG}")" == "${base}/v4.2.0/scripts/install.sh" ]] ||
    fail "${forwarder}: piped from a directory with scripts/install.sh it must fetch the installer at the tag, got '$(head -n 1 "${CURL_LOG}")', output: ${out}"
  [[ "${out}" == "stub-installer install --version v4.2.0 --admin-email admin@example.com" ]] ||
    fail "${forwarder}: piped from a directory with scripts/install.sh it ran the wrong installer: ${out}"

  : >"${CURL_LOG}"
  out="$(cd "${decoy}" && env -u INSTALL_REF -u ANIX_CONTROL_VERSION -u V2BOARD_VERSION -u REPO_OWNER -u REPO_NAME \
    PATH="${workdir}/bin:${PATH}" CURL_LOG="${CURL_LOG}" \
    bash <(cat "${script}") install --version v4.2.0)"
  [[ "$(head -n 1 "${CURL_LOG}")" == "${base}/v4.2.0/scripts/install.sh" && "${out}" == "stub-installer install --version v4.2.0" ]] ||
    fail "${forwarder}: run from a process substitution it must fetch the installer at the tag, got: ${out}"

  # A real checkout (this file next to scripts/install.sh) uses its own
  # installer, however it is invoked, and downloads nothing.
  checkout="${workdir}/checkout-${forwarder%.sh}"
  mkdir -p "${checkout}/scripts"
  cp "${REPO_ROOT}/install.sh" "${REPO_ROOT}/panel_install.sh" "${checkout}/"
  printf '#!/usr/bin/env bash\necho "local-installer $*"\n' >"${checkout}/scripts/install.sh"
  for invocation in "${checkout}/${forwarder}" "./${forwarder}" "${forwarder}"; do
    : >"${CURL_LOG}"
    out="$(cd "${checkout}" && env -u INSTALL_REF -u ANIX_CONTROL_VERSION -u V2BOARD_VERSION -u REPO_OWNER -u REPO_NAME \
      PATH="${workdir}/bin:${PATH}" CURL_LOG="${CURL_LOG}" \
      bash "${invocation}" install --version v4.2.0)"
    [[ "${out}" == "local-installer install --version v4.2.0" ]] ||
      fail "${forwarder}: a checkout run as 'bash ${invocation}' must use its scripts/install.sh, got: ${out}"
    [[ ! -s "${CURL_LOG}" ]] || fail "${forwarder}: a checkout must not download the installer"
  done
done

echo "install-by-tag test passed"
