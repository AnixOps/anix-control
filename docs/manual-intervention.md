# Manual Intervention Register

Date: 2026-07-08

The project can continue automated code, test, and CI hardening without asking for routine confirmation. The items below still require operator or owner action because they involve credentials, money, infrastructure, legal exposure, or destructive production changes.

## Credentials And Secrets

Manual action required:

- Production JWT secret.
- Admin bootstrap password policy.
- Node API keys and rotation policy.
- gRPC API token and TLS materials.
- Payment gateway credentials.
- Telegram bot token or notification credentials.
- Docker registry credentials.
- SSH keys and Ansible inventory secrets.

Rules:

- Do not commit real secrets.
- Use environment variables, secret stores, or deployment-specific config files.
- Rotate any credential that was ever pasted into logs, chat, CI output, or a public repository.

## Database And Migrations

Manual action required:

- Confirm the production database driver and location.
- Take a backup before schema changes.
- Approve SQLite-to-PostgreSQL migration on production data.
- Approve rollback windows and maintenance windows.
- Confirm retention policy for traffic logs and audit logs.

Required evidence before production migration:

- Migration dry run on a copy of production data.
- Rollback procedure.
- Application version that understands both old and new schema where needed.
- Health checks after migration.

## Domain, TLS, And Reverse Proxy

Manual action required:

- Confirm production domain names.
- Configure TLS certificates.
- Configure trusted proxies.
- Configure Nginx/Caddy headers for `X-Forwarded-Proto`, client IP, and WebSocket upgrade.
- Confirm HSTS and redirect policy.

## Payment And Legal/Compliance

Manual action required:

- Enable or disable payment providers.
- Confirm callback URLs and signatures.
- Confirm refund, invoice, tax, and compliance requirements.
- Confirm acceptable use policy for proxy and forwarding features.
- Confirm whether forwarding products can be sold automatically.

## Production Deployment

Manual action required:

- Choose deployment method: binary, Docker, or orchestration platform.
- Confirm service manager: systemd, Docker Compose, Kubernetes, or another supervisor.
- Confirm database backup and restore location.
- Confirm log retention and monitoring.
- Confirm health check endpoint and alerting.
- Confirm release rollback procedure.

Current state:

- CI can build binaries and Docker images.
- The release workflow intentionally does not deploy to production.
- GitHub Releases include `OPERATOR_DEPLOYMENT.md` with the manual deployment, verification, and rollback flow.

## High-Risk Operations

Manual approval required before:

- Deleting or rewriting production database records.
- Running destructive migration or rollback commands.
- Rotating production secrets.
- Enabling automatic payments or paid forwarding products.
- Opening new public TCP/UDP forwarding ranges.
- Disabling authentication, rate limiting, or audit logging.
- Replacing production reverse proxy or TLS configuration.

## Current Known Local Deployment Command

For the current server layout, root deployment can use:

```bash
cd /home/dev/anixops/v2board_AnixOps
PATH=/usr/local/go/bin:/home/dev/.local/opt/node/bin:$PATH \
GO_BIN=/usr/local/go/bin/go \
NPM_BIN=/home/dev/.local/opt/node/bin/npm \
./config/deploy/deploy_panel.sh
```

This command still depends on the local Go and Node paths being valid on the target host. If npm and node are not in the same directory, also set `NODE_BIN=/path/to/node`.
