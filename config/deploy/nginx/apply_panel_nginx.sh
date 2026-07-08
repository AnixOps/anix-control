#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SSL_DIR="${SCRIPT_DIR}/ssl"
SITES_DIR="${SCRIPT_DIR}/sites-available"
TARGET_SSL_DIR="/etc/nginx/ssl"
TARGET_SITES_AVAILABLE="/etc/nginx/sites-available"
TARGET_SITES_ENABLED="/etc/nginx/sites-enabled"
LEGACY_SITE_LINKS=(
  "${TARGET_SITES_ENABLED}/anixops-node-platform.conf"
  "${TARGET_SITES_ENABLED}/anixops-node-platform-https.conf"
)

if [[ "${EUID}" -ne 0 ]]; then
  echo "run as root: sudo $0" >&2
  exit 1
fi

CRT_SOURCE="${SSL_DIR}/panel-origin.crt"
KEY_SOURCE="${SSL_DIR}/panel-origin.key"
SERVER_NAMES_SOURCE="${SSL_DIR}/server-names.txt"
HTTP_TEMPLATE="${SITES_DIR}/x.kalijerry.uk.conf"
HTTPS_TEMPLATE="${SITES_DIR}/x.kalijerry.uk-https.conf"

for path in "${CRT_SOURCE}" "${KEY_SOURCE}" "${SERVER_NAMES_SOURCE}" "${HTTP_TEMPLATE}" "${HTTPS_TEMPLATE}"; do
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

SERVER_NAMES="$(grep -v '^[[:space:]]*#' "${SERVER_NAMES_SOURCE}" | awk 'NF {print $1}' | paste -sd' ' -)"
if [[ -z "${SERVER_NAMES}" ]]; then
  echo "no server names found in ${SERVER_NAMES_SOURCE}" >&2
  exit 1
fi

mkdir -p "${TARGET_SSL_DIR}"
install -m 644 "${CRT_SOURCE}" "${TARGET_SSL_DIR}/panel-origin.crt"
install -m 600 "${KEY_SOURCE}" "${TARGET_SSL_DIR}/panel-origin.key"

tmp_http="$(mktemp)"
tmp_https="$(mktemp)"
trap 'rm -f "${tmp_http}" "${tmp_https}"' EXIT

sed "s/__SERVER_NAMES__/${SERVER_NAMES}/g" "${HTTP_TEMPLATE}" > "${tmp_http}"
sed "s/__SERVER_NAMES__/${SERVER_NAMES}/g" "${HTTPS_TEMPLATE}" > "${tmp_https}"

install -m 644 "${tmp_http}" "${TARGET_SITES_AVAILABLE}/x.kalijerry.uk.conf"
install -m 644 "${tmp_https}" "${TARGET_SITES_AVAILABLE}/x.kalijerry.uk-https.conf"

ln -sfn "${TARGET_SITES_AVAILABLE}/x.kalijerry.uk.conf" "${TARGET_SITES_ENABLED}/x.kalijerry.uk.conf"
ln -sfn "${TARGET_SITES_AVAILABLE}/x.kalijerry.uk-https.conf" "${TARGET_SITES_ENABLED}/x.kalijerry.uk-https.conf"

for legacy_link in "${LEGACY_SITE_LINKS[@]}"; do
  if [[ -L "${legacy_link}" || -f "${legacy_link}" ]]; then
    rm -f "${legacy_link}"
  fi
done

/usr/sbin/nginx -t
systemctl reload nginx

echo "applied nginx config for: ${SERVER_NAMES}"
