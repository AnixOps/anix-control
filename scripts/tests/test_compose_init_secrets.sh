#!/usr/bin/env bash
# Tests of config/deploy/compose/init-secrets.sh in a temporary Compose
# directory (unprivileged: ownership changes are skipped).

set -Eeuo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
script="${REPO_ROOT}/config/deploy/compose/init-secrets.sh"
temporary="$(mktemp -d)"
cleanup() { find "${temporary}" -depth -delete; }
trap cleanup EXIT
export ANIX_INIT_SECRETS_NO_CHOWN=1 LETSENCRYPT_LIVE_DIR="${temporary}/letsencrypt/live"

failures=0
check() {
  local name="$1"
  shift
  if "$@"; then printf 'ok   %s\n' "${name}"; else printf 'FAIL %s\n' "${name}"; failures=$((failures + 1)); fi
}
mode_of() { stat -c '%a' "$1"; }
lacks() { ! grep -qF -- "$1" <<<"$2"; }
run() { bash "${script}" "$@" 2>&1; }
refused() {
  local expected="$1" output status=0
  shift
  output="$(bash "${script}" "$@" 2>&1)" || status=$?
  [[ "${status}" -ne 0 && "${output}" == *"${expected}"* ]]
}

pki="${temporary}/pki"
mkdir -p "${pki}"
openssl req -x509 -newkey rsa:2048 -nodes -days 2 -subj "/CN=Test Root" \
  -keyout "${pki}/ca.key" -out "${pki}/ca.crt" \
  -addext "basicConstraints=critical,CA:TRUE" -addext "keyUsage=critical,keyCertSign,cRLSign" >/dev/null 2>&1
openssl req -newkey rsa:2048 -nodes -subj "/CN=grpc.test" -keyout "${pki}/server.key" -out "${pki}/server.csr" >/dev/null 2>&1
printf '%s\n' "subjectAltName=DNS:grpc.test" "extendedKeyUsage=serverAuth" > "${pki}/server.ext"
openssl x509 -req -in "${pki}/server.csr" -CA "${pki}/ca.crt" -CAkey "${pki}/ca.key" -CAcreateserial \
  -days 1 -extfile "${pki}/server.ext" -out "${pki}/server.crt" >/dev/null 2>&1
openssl req -x509 -newkey rsa:2048 -nodes -days 1 -subj "/CN=grpc.test" -addext "subjectAltName=DNS:grpc.test" \
  -keyout "${pki}/self.key" -out "${pki}/self.crt" >/dev/null 2>&1

# 1. Fresh directory: secrets created, the key never printed.
dir="${temporary}/fresh"
mkdir -p "${dir}"
cp "${REPO_ROOT}/config/deploy/compose/control.env.example" "${dir}/control.env"
output="$(run --dir "${dir}")"
kek="${dir}/secrets/module_ca_kek"
check "fresh: jwt_secret created 0400" test "$(mode_of "${dir}/secrets/jwt_secret")" = 400
check "fresh: module_ca_kek created 0400" test "$(mode_of "${kek}")" = 400
check "fresh: key is 32 bytes, base64" test "$(base64 -d < "${kek}" | wc -c)" -eq 32
check "fresh: fingerprint printed" grep -qF "fingerprint $(sha256sum "${kek}" | cut -c1-16)" <<<"${output}"
check "fresh: key never printed" lacks "$(cat "${kek}")" "${output}"
check "fresh: TLS warning" grep -qF "Agents cannot connect yet" <<<"${output}"
check "fresh: db_password reminder" grep -qF "db_password is missing" <<<"${output}"
check "fresh: no certificate created" test ! -e "${dir}/tls/control.crt"

# 2. Re-run keeps every secret.
before="$(sha256sum "${dir}/secrets/jwt_secret" "${kek}")"
output="$(run --dir "${dir}")"
check "rerun: secrets unchanged" test "$(sha256sum "${dir}/secrets/jwt_secret" "${kek}")" = "${before}"
check "rerun: reports keeping the key" grep -qF "Keeping the CA key-encryption key" <<<"${output}"

# 3. A key already in control.env is never shadowed by a new one.
dir="${temporary}/envkey"
mkdir -p "${dir}"
printf 'ANIX_CONTROL_MODULE_RUNTIME_CA_KEK=c2VjcmV0\n' > "${dir}/control.env"
check "env key: refused" refused "already sets ANIX_CONTROL_MODULE_RUNTIME_CA_KEK" --dir "${dir}"
check "env key: no key file" test ! -e "${dir}/secrets/module_ca_kek"

# 4. A trusted certificate enables gRPC TLS once.
dir="${temporary}/tls"
mkdir -p "${dir}"
cp "${REPO_ROOT}/config/deploy/compose/control.env.example" "${dir}/control.env"
export GRPC_TLS_TRUST_CA_FILE="${pki}/ca.crt"
run --dir "${dir}" --grpc-name grpc.test --grpc-tls-cert "${pki}/server.crt" --grpc-tls-key "${pki}/server.key" >/dev/null
run --dir "${dir}" --grpc-name grpc.test --grpc-tls-cert "${pki}/server.crt" --grpc-tls-key "${pki}/server.key" >/dev/null
check "tls: certificate 0444" test "$(mode_of "${dir}/tls/control.crt")" = 444
check "tls: key 0400" test "$(mode_of "${dir}/tls/control.key")" = 400
check "tls: grpc enabled once" test "$(grep -c '^ANIX_CONTROL_GRPC_ENABLED=true$' "${dir}/control.env")" -eq 1
check "tls: certificate path" grep -qx 'ANIX_CONTROL_GRPC_TLS_CERT_FILE=/run/anix-control/tls/control.crt' "${dir}/control.env"
check "tls: key path" grep -qx 'ANIX_CONTROL_GRPC_TLS_KEY_FILE=/run/anix-control/tls/control.key' "${dir}/control.env"
check "tls: agent grpc target" grep -qx 'ANIX_CONTROL_AGENT_INSTALL_GRPC_TARGET=grpc.test:50051' "${dir}/control.env"

# 5. Let's Encrypt by name.
dir="${temporary}/letsencrypt"
mkdir -p "${dir}" "${LETSENCRYPT_LIVE_DIR}/grpc.test"
cp "${pki}/server.crt" "${LETSENCRYPT_LIVE_DIR}/grpc.test/fullchain.pem"
cp "${pki}/server.key" "${LETSENCRYPT_LIVE_DIR}/grpc.test/privkey.pem"
run --dir "${dir}" --grpc-name grpc.test >/dev/null
check "letsencrypt: copied" cmp -s "${dir}/tls/control.key" "${pki}/server.key"

# 6. Certificates Agents would reject are refused before anything is written.
dir="${temporary}/refused"
mkdir -p "${dir}"
check "refuse: self-signed" refused "does not verify against this host's system CA roots" --dir "${dir}" --grpc-tls-cert "${pki}/self.crt" --grpc-tls-key "${pki}/self.key"
check "refuse: wrong name" refused "is not valid for other.test" --dir "${dir}" --grpc-name other.test --grpc-tls-cert "${pki}/server.crt" --grpc-tls-key "${pki}/server.key"
check "refuse: wrong key" refused "is not the private key" --dir "${dir}" --grpc-tls-cert "${pki}/server.crt" --grpc-tls-key "${pki}/self.key"
check "refuse: nothing written" test ! -e "${dir}/secrets"
unset GRPC_TLS_TRUST_CA_FILE
check "refuse: test CA is not a system root" refused "does not verify" --dir "${dir}" --grpc-tls-cert "${pki}/server.crt" --grpc-tls-key "${pki}/server.key"

if [[ "${failures}" -ne 0 ]]; then
  printf '%d check(s) failed\n' "${failures}"
  exit 1
fi
printf 'compose init-secrets: all checks passed\n'
