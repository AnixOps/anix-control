# Manual Intervention Register

Date: 2026-07-10

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
- Plugin Secret AES-256-GCM keyring in `plugins.secret_encryption`.

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
- Copy `config/production-acceptance.yaml.example` to the ignored
  `config/production-acceptance.yaml`, record real provider/change references,
  and pass `anix-control -config config/config.yaml -check-production-evidence
  config/production-acceptance.yaml` before handoff.

Current state:

- CI can build binaries and Docker images.
- Production startup is fail-closed: `-check-config` rejects placeholders and
  normal startup rejects a missing or outdated schema. Schema changes require
  the separate `-migrate-schema` command in an approved maintenance window.
- The release workflow intentionally does not deploy to production.
- GitHub Releases include `OPERATOR_DEPLOYMENT.md` with the manual deployment, verification, and rollback flow, `UPGRADE.md` with upgrade and rollback instructions, `RELEASE_NOTES.md` generated from `CHANGELOG.md`, `migration-dry-run.txt` as the CI migration dry-run evidence for that tag, and `RELEASE_MANIFEST.json` with GitHub Actions run metadata and artifact hashes.

## High-Risk Operations

Manual approval required before:

- Deleting or rewriting production database records.
- Running destructive migration or rollback commands.
- Rotating production secrets.
- Enabling global MFA enforcement for all users or admins before confirming enrollment and recovery procedures.
- Enabling automatic payments or paid forwarding products.
- Opening new public TCP/UDP forwarding ranges.
- Disabling authentication, rate limiting, or audit logging.
- Replacing production reverse proxy or TLS configuration.

MFA enforcement note:

- Global `enforce_for_all` and `enforce_for_admin` login policies now block JWT issuance for covered accounts that have not enabled MFA.
- Operators must confirm an administrator recovery path and user enrollment communication before enabling the policy in production.

## Release Build Policy

- All release binaries and frontend assets must be produced by GitHub Actions.
- The CI baseline uses Go 1.26.6, runs `govulncheck` against the full module graph, and rejects frontend dependencies reported at moderate severity or above. A newly published advisory must be resolved in the pinned toolchain or dependency versions before release evidence is accepted.
- Frontend CI runs the mocked browser suite and the isolated real-Control signed WebUI suite as separate gates; both must pass before frontend artifacts are accepted.
- The frontend job and Docker build use the same committed lockfile installation policy; both dependency paths must pass before a container artifact is accepted.
- Operators must deploy GitHub Release artifacts after reviewing `RELEASE_NOTES.md`, verifying `SHA256SUMS.txt`, and checking `RELEASE_MANIFEST.json`.
- Do not build release artifacts on the production host or from a local checkout.
- `config/deploy/deploy_panel.sh` performs a local source-tree build and is guarded by `ALLOW_LOCAL_BUILD=1`; use it only for explicitly approved development or emergency operator work, not release builds.
- Legacy local entrypoints `config/scripts/deploy.sh` and `config/scripts/pre-deploy.sh` are also guarded by `ALLOW_LOCAL_BUILD=1`; they are not approved release build paths.
- V2bX Ansible rollout helper `config/deploy/ansible/nodes/deploy_from_inventory.sh` must use prebuilt artifacts with `--skip-build` for release or production rollouts; its local build path is also guarded by `ALLOW_LOCAL_BUILD=1`.
- CI enforces the local deployment build guard with `config/deploy/check_release_build_policy.sh`.

Root-only cleanup for old local build outputs:

```bash
cd /home/dev/anixops/v2board_AnixOps
bash config/deploy/clean_local_build_artifacts.sh --dry-run
bash config/deploy/clean_local_build_artifacts.sh
# Optional: remove ignored local deploy archive leftovers after reviewing dry-run.
bash config/deploy/clean_local_build_artifacts.sh --dry-run --include-deploy-backups
bash config/deploy/clean_local_build_artifacts.sh --include-deploy-backups
# If the script reports root-owned artifacts, run the exact sudo command it prints.
```

The deploy-backup cleanup mode removes stale archive leftovers in the checkout
only. It keeps database backups, config, certificates, and `web/node_modules`.
