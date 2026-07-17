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
#   4. installs a certbot deploy-hook that restarts anix-control.service so
#      renewals (run automatically by certbot's systemd timer) take effect
#      without you doing anything
#
# Put the resulting paths into the AnixOps Control config under
# grpc.tls_cert_file/grpc.tls_key_file, then restart the service.
set -euo pipefail

DOMAIN="${DOMAIN:-}"
CREDENTIALS_FILE="/etc/letsencrypt/cloudflare.ini"
DEPLOY_HOOK="${DEPLOY_HOOK:-/etc/letsencrypt/renewal-hooks/deploy/restart-anix-control.sh}"
SERVICE_NAME="${SERVICE_NAME:-anix-control.service}"
CONFIG_FILE="${CONFIG_FILE:-/opt/anixops/control/config/config.yaml}"

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
echo "Add these paths to grpc.tls_cert_file/tls_key_file in ${CONFIG_FILE},"
echo "then restart ${SERVICE_NAME}."
