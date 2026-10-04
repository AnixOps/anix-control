#!/usr/bin/env bash
# Prepare the secrets of a docker-compose.prod.yml deployment, idempotently.
#
#   sudo bash init-secrets.sh [--dir <compose dir>] \
#     [--grpc-name <host>] [--grpc-tls-cert <fullchain.pem> --grpc-tls-key <privkey.pem>]
#
# - secrets/jwt_secret: created when missing (32 random bytes, hex).
# - secrets/module_ca_kek: module_runtime.ca_kek, the key that seals Control's
#   built-in CA (it signs Agent certificates). Created when missing (32 random
#   bytes, base64); an existing one is never replaced, and only its
#   fingerprint is printed. Refused while control.env sets the key itself.
# - secrets/db_password is yours: the script only checks that it exists.
# - gRPC TLS: with a certificate (--grpc-tls-cert/--grpc-tls-key, or Let's
#   Encrypt's for --grpc-name), copies it to tls/ and enables gRPC with TLS in
#   control.env. Agents verify Control's certificate against the node's
#   system roots, so it must be publicly trusted for the name they dial: a
#   self-signed certificate is refused. Re-run after a renewal to copy the new
#   certificate (then restart control).
#
# Files are written for the container user (uid 10001), mode 0400 (the
# certificate 0444). Run as root.
set -Eeuo pipefail

DIR="."
GRPC_NAME=""
GRPC_TLS_CERT=""
GRPC_TLS_KEY=""
SECRET_UID="${SECRET_UID:-10001}"
LETSENCRYPT_LIVE_DIR="${LETSENCRYPT_LIVE_DIR:-/etc/letsencrypt/live}"
# Tests verify against their own CA instead of the system roots.
GRPC_TLS_TRUST_CA_FILE="${GRPC_TLS_TRUST_CA_FILE:-}"
# Tests run unprivileged and skip the ownership change.
NO_CHOWN="${ANIX_INIT_SECRETS_NO_CHOWN:-0}"
CONTAINER_TLS_DIR="/run/anix-control/tls"
KEK_VARIABLE="ANIX_CONTROL_MODULE_RUNTIME_CA_KEK"

info() { printf '[INFO] %s\n' "$*"; }
warn() { printf '[WARN] %s\n' "$*" >&2; }
die() { printf '[ERROR] %s\n' "$*" >&2; exit 1; }

usage() { sed -n '2,23p' "$0" | sed 's/^# \{0,1\}//'; }

while [[ "$#" -gt 0 ]]; do
  case "$1" in
    --dir) DIR="${2:-}"; shift 2 ;;
    --grpc-name) GRPC_NAME="${2:-}"; shift 2 ;;
    --grpc-tls-cert) GRPC_TLS_CERT="${2:-}"; shift 2 ;;
    --grpc-tls-key) GRPC_TLS_KEY="${2:-}"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) die "Unknown argument: $1" ;;
  esac
done

[[ "${NO_CHOWN}" == 1 || "${EUID}" -eq 0 ]] || die "Run as root (the secret files belong to uid ${SECRET_UID})."
[[ -d "${DIR}" ]] || die "No such directory: ${DIR}"
SECRETS_DIR="${ANIX_CONTROL_SECRETS_DIR:-${DIR}/secrets}"
[[ "${SECRETS_DIR}" == /* ]] || SECRETS_DIR="${DIR}/${SECRETS_DIR#./}"
TLS_DIR="${ANIX_CONTROL_TLS_DIR:-${DIR}/tls}"
[[ "${TLS_DIR}" == /* ]] || TLS_DIR="${DIR}/${TLS_DIR#./}"
ENV_FILE="${DIR}/${ANIX_CONTROL_ENV_FILE:-control.env}"

# put <mode> <target>: writes stdin to target for the container user.
put() {
  local mode="$1" target="$2" owner=()
  [[ "${NO_CHOWN}" == 1 ]] || owner=(-o "${SECRET_UID}" -g "${SECRET_UID}")
  [[ ! -L "${target}" ]] || die "${target} must not be a symbolic link"
  install -m "${mode}" "${owner[@]}" /dev/stdin "${target}.new"
  mv -f "${target}.new" "${target}"
}

fingerprint() { sha256sum "$1" | cut -c1-16; }

env_sets() { [[ -f "${ENV_FILE}" ]] && grep -Eq "^[[:space:]]*(export[[:space:]]+)?$1=" "${ENV_FILE}"; }

name_is_ip() { [[ "$1" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ || "$1" == *:* ]]; }

verify_certificate() {
  local cert="$1" key="$2" name="$3" output
  [[ -f "${cert}" && -r "${cert}" ]] || die "gRPC certificate not found: ${cert}"
  [[ -f "${key}" && -r "${key}" ]] || die "gRPC private key not found: ${key}"
  command -v openssl >/dev/null 2>&1 || die "openssl is required to check the gRPC certificate"
  openssl x509 -in "${cert}" -noout >/dev/null 2>&1 || die "${cert} is not a PEM certificate"
  [[ "$(openssl x509 -in "${cert}" -noout -pubkey 2>/dev/null | openssl pkey -pubin -outform DER 2>/dev/null | sha256sum)" == \
     "$(openssl pkey -in "${key}" -pubout -outform DER 2>/dev/null | sha256sum)" ]] || die "${key} is not the private key of ${cert}"
  local verify=(openssl verify -purpose sslserver -untrusted "${cert}")
  [[ -n "${GRPC_TLS_TRUST_CA_FILE}" ]] && verify+=(-CAfile "${GRPC_TLS_TRUST_CA_FILE}")
  if ! output="$("${verify[@]}" "${cert}" 2>&1)"; then
    warn "${output//$'\n'/ }"
    die "${cert} does not verify against this host's system CA roots. Agents verify Control's gRPC certificate against their system roots and cannot be told to trust a private or self-signed CA: use a publicly trusted certificate (Let's Encrypt, or your reverse proxy's certificate for the same name)."
  fi
  if [[ -n "${name}" ]]; then
    local check=(-checkhost "${name}")
    name_is_ip "${name}" && check=(-checkip "${name}")
    output="$(openssl x509 -in "${cert}" -noout "${check[@]}" 2>&1 || true)"
    [[ "${output}" == *"does match"* ]] || die "${cert} is not valid for ${name}, the name Agents dial"
  fi
}

# --- Check the TLS input before writing anything.
if [[ -n "${GRPC_NAME}" ]]; then
  [[ "${GRPC_NAME}" =~ ^[A-Za-z0-9]([A-Za-z0-9.-]*[A-Za-z0-9])?$ ]] || name_is_ip "${GRPC_NAME}" || \
    die "--grpc-name must be a DNS name or an IP address"
fi
if [[ -n "${GRPC_TLS_CERT}" || -n "${GRPC_TLS_KEY}" ]]; then
  [[ -n "${GRPC_TLS_CERT}" && -n "${GRPC_TLS_KEY}" ]] || die "--grpc-tls-cert and --grpc-tls-key go together"
elif [[ -n "${GRPC_NAME}" && -r "${LETSENCRYPT_LIVE_DIR}/${GRPC_NAME}/fullchain.pem" && -r "${LETSENCRYPT_LIVE_DIR}/${GRPC_NAME}/privkey.pem" ]]; then
  GRPC_TLS_CERT="${LETSENCRYPT_LIVE_DIR}/${GRPC_NAME}/fullchain.pem"
  GRPC_TLS_KEY="${LETSENCRYPT_LIVE_DIR}/${GRPC_NAME}/privkey.pem"
  info "Using the Let's Encrypt certificate ${LETSENCRYPT_LIVE_DIR}/${GRPC_NAME}"
fi
[[ -z "${GRPC_TLS_CERT}" ]] || verify_certificate "${GRPC_TLS_CERT}" "${GRPC_TLS_KEY}" "${GRPC_NAME}"

# --- Secrets.
[[ ! -L "${SECRETS_DIR}" ]] || die "${SECRETS_DIR} must not be a symbolic link"
install -d -m 0750 "${SECRETS_DIR}"

if [[ -s "${SECRETS_DIR}/jwt_secret" ]]; then
  info "Keeping ${SECRETS_DIR}/jwt_secret"
else
  od -An -N32 -tx1 /dev/urandom | tr -d ' \n' | put 0400 "${SECRETS_DIR}/jwt_secret"
  info "Generated ${SECRETS_DIR}/jwt_secret"
fi

if [[ -s "${SECRETS_DIR}/module_ca_kek" ]]; then
  if env_sets "${KEK_VARIABLE}" || env_sets "${KEK_VARIABLE}_FILE"; then
    die "${ENV_FILE} sets ${KEK_VARIABLE} and ${SECRETS_DIR}/module_ca_kek exists: keep the key that sealed the CA in secrets/module_ca_kek and remove it from ${ENV_FILE}."
  fi
  info "Keeping the CA key-encryption key ${SECRETS_DIR}/module_ca_kek (fingerprint $(fingerprint "${SECRETS_DIR}/module_ca_kek"))"
else
  if env_sets "${KEK_VARIABLE}" || env_sets "${KEK_VARIABLE}_FILE"; then
    die "${ENV_FILE} already sets ${KEK_VARIABLE}: move that key into ${SECRETS_DIR}/module_ca_kek (printf '%s' \"\$key\" | install -m 0400 -o ${SECRET_UID} -g ${SECRET_UID} /dev/stdin ${SECRETS_DIR}/module_ca_kek) and remove it from ${ENV_FILE}. A new key would make the existing CA unusable."
  fi
  head -c 32 /dev/urandom | base64 | tr -d '\n' | put 0400 "${SECRETS_DIR}/module_ca_kek"
  warn "Generated the CA key-encryption key ${SECRETS_DIR}/module_ca_kek (fingerprint $(fingerprint "${SECRETS_DIR}/module_ca_kek"))."
  warn "Back it up with control.env and secrets/: without it the CA that signs Agent certificates cannot be used, and every Agent must enroll again."
fi

[[ -s "${SECRETS_DIR}/db_password" ]] || \
  warn "${SECRETS_DIR}/db_password is missing: printf '%s' '<database password>' | install -m 0400 -o ${SECRET_UID} -g ${SECRET_UID} /dev/stdin ${SECRETS_DIR}/db_password"

# --- gRPC TLS.
[[ ! -L "${TLS_DIR}" ]] || die "${TLS_DIR} must not be a symbolic link"
install -d -m 0755 "${TLS_DIR}"
if [[ -n "${GRPC_TLS_CERT}" ]]; then
  put 0444 "${TLS_DIR}/control.crt" < "${GRPC_TLS_CERT}"
  put 0400 "${TLS_DIR}/control.key" < "${GRPC_TLS_KEY}"
  touch "${ENV_FILE}"
  added=()
  for entry in \
    "ANIX_CONTROL_GRPC_ENABLED=true" \
    "ANIX_CONTROL_GRPC_TLS_CERT_FILE=${CONTAINER_TLS_DIR}/control.crt" \
    "ANIX_CONTROL_GRPC_TLS_KEY_FILE=${CONTAINER_TLS_DIR}/control.key"; do
    env_sets "${entry%%=*}" || added+=("${entry}")
  done
  if [[ -n "${GRPC_NAME}" ]] && ! env_sets ANIX_CONTROL_AGENT_INSTALL_GRPC_TARGET; then
    if [[ "${GRPC_NAME}" == *:* ]]; then
      added+=("ANIX_CONTROL_AGENT_INSTALL_GRPC_TARGET=[${GRPC_NAME}]:${ANIX_CONTROL_GRPC_PORT:-50051}")
    else
      added+=("ANIX_CONTROL_AGENT_INSTALL_GRPC_TARGET=${GRPC_NAME}:${ANIX_CONTROL_GRPC_PORT:-50051}")
    fi
  fi
  if [[ "${#added[@]}" -gt 0 ]]; then
    printf '\n# gRPC with TLS for AnixOps Agents (init-secrets.sh)\n' >> "${ENV_FILE}"
    printf '%s\n' "${added[@]}" >> "${ENV_FILE}"
  fi
  info "gRPC TLS: ${TLS_DIR}/control.crt (mounted at ${CONTAINER_TLS_DIR}); publish the port with ANIX_CONTROL_GRPC_BIND=0.0.0.0 in .env, then: docker compose -f docker-compose.prod.yml up -d"
elif ! env_sets ANIX_CONTROL_GRPC_TLS_CERT_FILE; then
  warn "Agents cannot connect yet: gRPC has no TLS certificate."
  warn "Agents verify Control's certificate against the node's system CA roots and cannot pin a private CA, so it must be publicly trusted for the name they dial; a self-signed certificate does not work."
  warn "Get one (for example certbot for grpc.example.com), then re-run: sudo bash init-secrets.sh --grpc-name grpc.example.com (or --grpc-tls-cert <fullchain.pem> --grpc-tls-key <privkey.pem>). docs/DEPLOYMENT.md, \"Agent access\"."
fi
