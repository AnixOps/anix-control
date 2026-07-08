#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SSL_DIR="${SCRIPT_DIR}/ssl"
SITES_DIR="${SCRIPT_DIR}/sites-available"

TARGET_SSL_DIR="/etc/nginx/ssl"
TARGET_SITES_AVAILABLE="/etc/nginx/sites-available"
TARGET_SITES_ENABLED="/etc/nginx/sites-enabled"

DOMAIN="sub.5555133.xyz"

if [[ "${EUID}" -ne 0 ]]; then
  echo "run as root: sudo $0" >&2
  exit 1
fi

CRT_SOURCE="${SSL_DIR}/${DOMAIN}.crt"
KEY_SOURCE="${SSL_DIR}/${DOMAIN}.key"
HTTP_TEMPLATE="${SITES_DIR}/${DOMAIN}.conf"
HTTPS_TEMPLATE="${SITES_DIR}/${DOMAIN}-https.conf"

for path in "${CRT_SOURCE}" "${KEY_SOURCE}" "${HTTP_TEMPLATE}" "${HTTPS_TEMPLATE}"; do
  if [[ ! -f "${path}" ]]; then
    echo "missing required file: ${path}" >&2
    exit 1
  fi
done

if ! grep -q "BEGIN CERTIFICATE" "${CRT_SOURCE}"; then
  echo "certificate file does not look like a PEM certificate: ${CRT_SOURCE}" >&2
  exit 1
fi

if ! grep -Eq "BEGIN (RSA )?PRIVATE KEY" "${KEY_SOURCE}"; then
  echo "key file does not look like a PEM private key: ${KEY_SOURCE}" >&2
  exit 1
fi

mkdir -p "${TARGET_SSL_DIR}"
install -m 644 "${CRT_SOURCE}" "${TARGET_SSL_DIR}/${DOMAIN}.crt"
install -m 600 "${KEY_SOURCE}" "${TARGET_SSL_DIR}/${DOMAIN}.key"

install -m 644 "${HTTP_TEMPLATE}" "${TARGET_SITES_AVAILABLE}/${DOMAIN}.conf"
install -m 644 "${HTTPS_TEMPLATE}" "${TARGET_SITES_AVAILABLE}/${DOMAIN}-https.conf"

ln -sfn "${TARGET_SITES_AVAILABLE}/${DOMAIN}.conf" "${TARGET_SITES_ENABLED}/${DOMAIN}.conf"
ln -sfn "${TARGET_SITES_AVAILABLE}/${DOMAIN}-https.conf" "${TARGET_SITES_ENABLED}/${DOMAIN}-https.conf"

/usr/sbin/nginx -t
systemctl reload nginx

echo "applied subscription-only nginx config for ${DOMAIN}"
