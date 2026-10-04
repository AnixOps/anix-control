#!/usr/bin/env bash
# One-time setup of a Let's Encrypt certificate for the node-facing gRPC
# server, issued via DNS-01 against Cloudflare (no port 80 needed).
#
# Usage:
#   DOMAIN=grpc.example.com CLOUDFLARE_API_TOKEN=xxxxx sudo -E ./setup_certbot.sh
#
# The token needs Zone:DNS:Edit permission on the requested domain's zone only.
#
# What this does:
#   1. installs certbot + the cloudflare DNS plugin
#   2. writes /etc/letsencrypt/cloudflare.ini (mode 600) from the token
#   3. issues/renews the cert for DOMAIN
#   4. installs a certbot deploy-hook that copies the renewed certificate
#      into Control's TLS directory (INSTALL_DIR/config/tls, readable by the
#      service user, which cannot read /etc/letsencrypt) and restarts
#      anix-control.service, so renewals (run automatically by certbot's
#      systemd timer) take effect without you doing anything
#
# Then enable gRPC with it (the installer finds /etc/letsencrypt/live/DOMAIN):
#   sudo bash scripts/install.sh enable-agents --grpc-name DOMAIN
#
# Agents verify Control's gRPC certificate against the node's system roots, so
# a publicly trusted certificate like this one is what they need; a
# self-signed certificate does not work.
set -euo pipefail

DOMAIN="${DOMAIN:-}"
CREDENTIALS_FILE="/etc/letsencrypt/cloudflare.ini"
DEPLOY_HOOK="${DEPLOY_HOOK:-/etc/letsencrypt/renewal-hooks/deploy/restart-anix-control.sh}"
SERVICE_NAME="${SERVICE_NAME:-anix-control.service}"
INSTALL_DIR="${INSTALL_DIR:-/opt/anixops/control}"
APP_USER="${APP_USER:-anixops}"

if [[ "${EUID}" -ne 0 ]]; then
  echo "run as root: sudo -E $0" >&2
  exit 1
fi

if [[ -z "${CLOUDFLARE_API_TOKEN:-}" ]]; then
  echo "CLOUDFLARE_API_TOKEN is not set. Run as:" >&2
  echo "  CLOUDFLARE_API_TOKEN=xxxxx sudo -E $0" >&2
  exit 1
fi

if [[ -z "${DOMAIN}" ]]; then
  echo "DOMAIN is not set. Example: DOMAIN=grpc.example.com" >&2
  exit 1
fi

echo "==> Installing certbot + cloudflare DNS plugin"
apt-get update -qq
apt-get install -y certbot python3-certbot-dns-cloudflare

echo "==> Writing ${CREDENTIALS_FILE}"
umask 077
cat > "${CREDENTIALS_FILE}" <<EOF
dns_cloudflare_api_token = ${CLOUDFLARE_API_TOKEN}
EOF
chmod 600 "${CREDENTIALS_FILE}"

echo "==> Installing renewal deploy hook: ${DEPLOY_HOOK}"
mkdir -p "$(dirname "${DEPLOY_HOOK}")"
cat > "${DEPLOY_HOOK}" <<EOF
#!/usr/bin/env bash
# Installed by setup_certbot.sh: copy the renewed gRPC certificate into
# AnixOps Control's TLS directory and restart Control to load it.
set -euo pipefail
live="\${RENEWED_LINEAGE:-/etc/letsencrypt/live/${DOMAIN}}"
tls_dir="${INSTALL_DIR}/config/tls"
if [[ "\${live}" == */${DOMAIN} && -d "\${tls_dir}" && ! -L "\${tls_dir}" ]]; then
  install -m 0644 -o ${APP_USER} -g ${APP_USER} "\${live}/fullchain.pem" "\${tls_dir}/control.crt.new"
  install -m 0600 -o ${APP_USER} -g ${APP_USER} "\${live}/privkey.pem" "\${tls_dir}/control.key.new"
  mv -f "\${tls_dir}/control.crt.new" "\${tls_dir}/control.crt"
  mv -f "\${tls_dir}/control.key.new" "\${tls_dir}/control.key"
fi
systemctl restart ${SERVICE_NAME}
EOF
chmod 755 "${DEPLOY_HOOK}"

echo "==> Requesting certificate for ${DOMAIN}"
certbot certonly \
  --non-interactive --agree-tos --register-unsafely-without-email \
  --dns-cloudflare \
  --dns-cloudflare-credentials "${CREDENTIALS_FILE}" \
  --dns-cloudflare-propagation-seconds 30 \
  -d "${DOMAIN}" \
  --deploy-hook "${DEPLOY_HOOK}"

echo "==> Done. Cert: /etc/letsencrypt/live/${DOMAIN}/fullchain.pem"
echo "           Key: /etc/letsencrypt/live/${DOMAIN}/privkey.pem"
echo "Enable gRPC TLS for Agents with it:"
echo "  sudo bash scripts/install.sh enable-agents --grpc-name ${DOMAIN}"
echo "Renewals are copied into ${INSTALL_DIR}/config/tls and ${SERVICE_NAME} restarts."
