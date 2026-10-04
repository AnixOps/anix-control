#!/usr/bin/env bash
# Fake-root tests of the Control installer's Agent access setup
# (scripts/install.sh): the CA key-encryption key and gRPC TLS. Nothing
# outside a temporary directory is touched and no service is started.

set -Eeuo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck source=scripts/install.sh
source "${REPO_ROOT}/scripts/install.sh"
set +E
trap - ERR

temporary="$(mktemp -d)"
cleanup() { find "${temporary}" -depth -delete; }
trap cleanup EXIT

# The installer runs as root; keep the test unprivileged.
chown() { :; }
install() {
  local arguments=()
  while [[ "$#" -gt 0 ]]; do
    case "$1" in
      -o|-g) shift 2 ;;
      *) arguments+=("$1"); shift ;;
    esac
  done
  command install "${arguments[@]}"
}
systemctl() { :; }

failures=0
pass() { printf 'ok   %s\n' "$1"; }
fail() { printf 'FAIL %s\n' "$1"; failures=$((failures + 1)); }
check() {
  local name="$1"
  shift
  if "$@"; then pass "${name}"; else fail "${name}"; fi
}
mode_of() { stat -c '%a' "$1"; }
lacks() { ! grep -qF -- "$1" <<<"$2"; }
file_lacks() { ! grep -qF -- "$1" "$2"; }

APP_USER="$(id -un)"
SERVICE_NAME="anix-control"
SYSTEMD_UNIT_DIR="${temporary}/systemd"
mkdir -p "${SYSTEMD_UNIT_DIR}"

# new_install <name> prepares an installation root with the shipped template.
new_install() {
  INSTALL_DIR="${temporary}/$1"
  set_install_paths
  mkdir -p "${INSTALL_DIR}/config"
  cp "${REPO_ROOT}/config/config.yaml.example" "${CONFIG_FILE}"
  GRPC_NAME="" GRPC_TLS_CERT="" GRPC_TLS_KEY=""
  BACKUP_DIR=""
}

# --- Test PKI: a CA the test trusts, a server certificate, a self-signed one.
pki="${temporary}/pki"
mkdir -p "${pki}"
openssl req -x509 -newkey rsa:2048 -nodes -days 2 -subj "/CN=Test Root" \
  -keyout "${pki}/ca.key" -out "${pki}/ca.crt" \
  -addext "basicConstraints=critical,CA:TRUE" -addext "keyUsage=critical,keyCertSign,cRLSign" >/dev/null 2>&1
openssl req -newkey rsa:2048 -nodes -subj "/CN=grpc.test" -keyout "${pki}/server.key" -out "${pki}/server.csr" >/dev/null 2>&1
printf '%s\n' "subjectAltName=DNS:grpc.test" "extendedKeyUsage=serverAuth" "keyUsage=critical,digitalSignature,keyEncipherment" > "${pki}/server.ext"
openssl x509 -req -in "${pki}/server.csr" -CA "${pki}/ca.crt" -CAkey "${pki}/ca.key" -CAcreateserial \
  -days 1 -extfile "${pki}/server.ext" -out "${pki}/server.crt" >/dev/null 2>&1
openssl req -x509 -newkey rsa:2048 -nodes -days 1 -subj "/CN=grpc.test" -addext "subjectAltName=DNS:grpc.test" \
  -keyout "${pki}/self.key" -out "${pki}/self.crt" >/dev/null 2>&1
GRPC_TLS_TRUST_CA_FILE="${pki}/ca.crt"

# --- 1. Fresh install without a certificate: the key is generated, never printed.
new_install fresh
output="$(configure_agent_access 1 2>&1)"
check "fresh: key file created" test -s "${CA_KEK_FILE}"
check "fresh: key file mode 0600" test "$(mode_of "${CA_KEK_FILE}")" = 600
check "fresh: key directory mode 0700" test "$(mode_of "$(dirname "${CA_KEK_FILE}")")" = 700
check "fresh: key is 32 bytes, base64" test "$(base64 -d < "${CA_KEK_FILE}" | wc -c)" -eq 32
key="$(cat "${CA_KEK_FILE}")"
fingerprint="$(sha256sum "${CA_KEK_FILE}" | cut -c1-16)"
check "fresh: fingerprint printed" grep -qF "fingerprint ${fingerprint}" <<<"${output}"
check "fresh: key never printed" lacks "${key}" "${output}"
check "fresh: TLS warning printed" grep -qF "Agents cannot connect yet" <<<"${output}"
check "fresh: no self-signed certificate created" test ! -e "${TLS_DIR}"
write_systemd_unit
check "fresh: unit passes the key file" grep -qxF "Environment=ANIX_CONTROL_MODULE_RUNTIME_CA_KEK_FILE=${CA_KEK_FILE}" "${SYSTEMD_UNIT_DIR}/anix-control.service"
check "fresh: config has no key in it" file_lacks "${key}" "${CONFIG_FILE}"

# --- 2. Re-run and upgrade keep the key.
output="$(configure_agent_access 1 2>&1)"
check "rerun: key unchanged" test "$(cat "${CA_KEK_FILE}")" = "${key}"
check "rerun: reports keeping it" grep -qF "Keeping the existing CA key-encryption key" <<<"${output}"
output="$(configure_agent_access 0 2>&1)"
check "update: key unchanged" test "$(cat "${CA_KEK_FILE}")" = "${key}"
chmod 0644 "${CA_KEK_FILE}"
configure_agent_access 0 >/dev/null 2>&1
check "update: key mode restored to 0600" test "$(mode_of "${CA_KEK_FILE}")" = 600
BACKUP_DIR="${temporary}/backup"
mkdir -p "${BACKUP_DIR}"
backup_ca_kek
check "backup: key copied with the release backup" cmp -s "${CA_KEK_FILE}" "${BACKUP_DIR}/module_ca_kek"

# --- 3. An existing install without a key is told, not changed.
new_install existing
output="$(configure_agent_access 0 2>&1)"
check "existing: no key generated on update" test ! -e "${CA_KEK_FILE}"
check "existing: enable-agents named" grep -qF "install.sh enable-agents" <<<"${output}"

# --- 4. A key already in config.yaml (or a drop-in) is never shadowed.
new_install configured
printf '%s\n' 'module_runtime:' '  ca_kek: "c2VjcmV0LWtleS1zZWNyZXQta2V5LXNlY3JldC1rZXk="' >> "${CONFIG_FILE}"
output="$(configure_agent_access 1 2>&1)"
check "configured: no key file created" test ! -e "${CA_KEK_FILE}"
check "configured: keeps config.yaml" grep -qF "keeping it" <<<"${output}"
write_systemd_unit
check "configured: unit sets no key file" file_lacks CA_KEK_FILE "${SYSTEMD_UNIT_DIR}/anix-control.service"
new_install dropin
mkdir -p "${SYSTEMD_UNIT_DIR}/anix-control.service.d"
printf '%s\n' '[Service]' 'Environment=ANIX_CONTROL_MODULE_RUNTIME_CA_KEK_FILE=/srv/kek' > "${SYSTEMD_UNIT_DIR}/anix-control.service.d/kek.conf"
configure_agent_access 1 >/dev/null 2>&1
check "drop-in: no key file created" test ! -e "${CA_KEK_FILE}"
rm -r "${SYSTEMD_UNIT_DIR}/anix-control.service.d"

# --- 5. A publicly trusted certificate (here: the test CA) enables gRPC TLS.
new_install tls
GRPC_NAME="grpc.test" GRPC_TLS_CERT="${pki}/server.crt" GRPC_TLS_KEY="${pki}/server.key"
accepted() { (preflight_grpc_tls) >/dev/null 2>&1; }
check "tls: preflight accepts a trusted certificate" accepted
output="$(preflight_grpc_tls 2>&1; configure_agent_access 1 2>&1)"
check "tls: certificate installed 0644" test "$(mode_of "${TLS_DIR}/control.crt")" = 644
check "tls: key installed 0600" test "$(mode_of "${TLS_DIR}/control.key")" = 600
check "tls: grpc enabled" test "$(config_section_value grpc enabled)" = true
check "tls: grpc listens on all interfaces" test "$(config_section_value grpc host)" = 0.0.0.0
check "tls: grpc certificate configured" test "$(config_section_value grpc tls_cert_file)" = "${TLS_DIR}/control.crt"
check "tls: grpc key configured" test "$(config_section_value grpc tls_key_file)" = "${TLS_DIR}/control.key"
check "tls: agent grpc target" test "$(config_section_value agent_install grpc_target)" = "grpc.test:50051"
check "tls: one grpc section" test "$(grep -c '^grpc:' "${CONFIG_FILE}")" -eq 1
check "tls: one tls_cert_file" test "$(grep -c '^  tls_cert_file:' "${CONFIG_FILE}")" -eq 1
check "tls: no TLS warning" lacks "Agents cannot connect yet" "${output}"
check "tls: KEK generated too" test -s "${CA_KEK_FILE}"

# --- 6. Certificates Agents would reject are refused before anything changes.
refused() {
  local cert="$1" key="$2" name="$3" expected="$4"
  new_install refused
  GRPC_NAME="${name}" GRPC_TLS_CERT="${cert}" GRPC_TLS_KEY="${key}"
  local result status=0
  result="$( (preflight_grpc_tls) 2>&1)" || status=$?
  [[ "${status}" -ne 0 && "${result}" == *"${expected}"* && ! -e "${CA_KEK_FILE}" ]]
}
check "refuse: self-signed certificate" refused "${pki}/self.crt" "${pki}/self.key" grpc.test "does not verify against this host's system CA roots"
check "refuse: wrong name" refused "${pki}/server.crt" "${pki}/server.key" other.test "is not valid for other.test"
check "refuse: key of another certificate" refused "${pki}/server.crt" "${pki}/self.key" grpc.test "is not the private key"
check "refuse: certificate without key" refused "${pki}/server.crt" "" grpc.test "go together"
check "refuse: bad name" refused "${pki}/server.crt" "${pki}/server.key" 'bad name;' "must be a DNS name"
GRPC_TLS_TRUST_CA_FILE=""
check "refuse: test CA is not a system root" refused "${pki}/server.crt" "${pki}/server.key" grpc.test "does not verify"
GRPC_TLS_TRUST_CA_FILE="${pki}/ca.crt"

# --- 7. Let's Encrypt certificates are found by name.
new_install letsencrypt
LETSENCRYPT_LIVE_DIR="${temporary}/letsencrypt/live"
mkdir -p "${LETSENCRYPT_LIVE_DIR}/grpc.test"
cp "${pki}/server.crt" "${LETSENCRYPT_LIVE_DIR}/grpc.test/fullchain.pem"
cp "${pki}/server.key" "${LETSENCRYPT_LIVE_DIR}/grpc.test/privkey.pem"
GRPC_NAME="grpc.test"
preflight_grpc_tls >/dev/null 2>&1
check "letsencrypt: certificate found" test "${GRPC_TLS_CERT}" = "${LETSENCRYPT_LIVE_DIR}/grpc.test/fullchain.pem"
configure_agent_access 1 >/dev/null 2>&1
check "letsencrypt: copied for the service user" cmp -s "${TLS_DIR}/control.key" "${pki}/server.key"

# --- 8. The configuration the installer writes is valid YAML.
new_install load
GRPC_NAME="grpc.test" GRPC_TLS_CERT="${pki}/server.crt" GRPC_TLS_KEY="${pki}/server.key"
configure_agent_access 1 >/dev/null 2>&1
if ! python3 -c 'import yaml' 2>/dev/null; then
  printf 'skip config: sections parse as YAML (no PyYAML)\n'
else
check "config: sections parse as YAML" python3 -c '
import sys, yaml
data = yaml.safe_load(open(sys.argv[1]))
assert data["grpc"]["enabled"] is True and data["grpc"]["host"] == "0.0.0.0"
assert data["agent_install"]["grpc_target"] == "grpc.test:50051"
'  "${CONFIG_FILE}"
fi

if [[ "${failures}" -ne 0 ]]; then
  printf '%d check(s) failed\n' "${failures}"
  exit 1
fi
printf 'control installer agent access: all checks passed\n'
