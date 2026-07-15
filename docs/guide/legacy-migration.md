# Legacy Panel Migration Plan

This document separates supported upgrades from data migrations that require
operator review. A release installer can safely replace panel binaries and
frontend files; it cannot safely guess the schema, payment behavior, password
format, or node credential semantics of an unrelated historical panel.

## Supported Migration Classes

| Source | Automation level | Required path |
|---|---|---|
| Earlier `v2board_AnixOps` native install | Supported | Version-pinned release installer plus backup and health check |
| Current AnixOps SQLite to PostgreSQL | Supported with operator approval | SQLite-to-PostgreSQL dry run, backup, import, verification, rollback plan |
| Existing AnixOps Docker deployment | Operator-guided | Preserve volumes/config, deploy matching release image or native target in staging |
| PHP V2Board, XBoard, or another fork | No automatic import | Export, field mapping, staged import, credential rotation, and cutover validation |

Do not label a foreign-schema import as complete merely because users appear in
the new database. Orders, payment callbacks, subscription tokens, node keys,
traffic counters, and password formats must all be individually validated.

## Common Preparation

Perform these steps before every migration:

1. Freeze panel configuration changes and record the currently running version.
2. Choose a maintenance window and assign a rollback owner.
3. Back up configuration, database, certificates, reverse-proxy configuration,
   and payment/provider credentials outside the panel host.
4. Record row counts for users, plans, nodes, node protocols, orders, active
   subscriptions, coupons, tickets, and traffic data.
5. Lower DNS TTL only when the cutover changes a hostname or IP address.
6. Build a staging copy from a scrubbed backup and rehearse the full procedure.

For a native AnixOps installation, a minimum SQLite backup is:

```bash
sudo systemctl stop v2board
sudo install -d -m 0700 /root/v2board-migration-backup
sudo cp -a /opt/v2board/config /root/v2board-migration-backup/config
sudo cp -a /opt/v2board/.release-version /root/v2board-migration-backup/ 2>/dev/null || true
sudo sha256sum /root/v2board-migration-backup/config/data/v2board.db \
  | sudo tee /root/v2board-migration-backup/v2board.db.sha256
sudo systemctl start v2board
```

Verify the backup on a separate location before proceeding. Do not leave a
plaintext database or private keys in a public object store.

## Earlier AnixOps Native Releases

This is an application upgrade, not a database import. The release installer
preserves `/opt/v2board/config`, including `config/data/v2board.db`, and creates
a binary/frontend snapshot before it changes files.

1. Create and verify the backup above.
2. Run the target-tag installer with `update --version <target>`.
3. Confirm `/health`, administrator login, a user subscription download, and
   a node config request using a non-production test user/node where possible.
4. Observe traffic and online reporting for at least one sync interval.
5. Keep the pre-upgrade backup until the rollback window expires.

If the new release fails its automatic health check, the installer restores the
previous binary/frontend snapshot. If a later functional check fails, install
the prior known-good application tag and restore the configuration/database only
when the incident analysis shows it is necessary.

## SQLite To PostgreSQL

Use the dedicated procedure in
[`../reference/sqlite-to-postgres-migration.md`](../reference/sqlite-to-postgres-migration.md).
The required order is:

1. Run the dry run against a copy of the SQLite database.
2. Take a PostgreSQL backup before import.
3. Stop the panel only for the approved import window.
4. Import and compare critical row counts.
5. Start the panel, verify `/health`, users, plans, subscriptions, nodes, and
   traffic reporting.
6. Restore the PostgreSQL backup if verification fails.

Do not use the SQLite-to-PostgreSQL tool to import a PHP V2Board/XBoard schema.
It is for the current AnixOps schema only.

## PHP V2Board, XBoard, And Other Forks

There is intentionally no one-command converter for foreign panels. Use this
staged plan instead:

1. Export source data read-only: users, plans, active entitlements, nodes,
   node protocol settings, orders, coupons, tickets, knowledge articles, and
   historical traffic if it is needed for billing/audit.
2. Produce a written mapping sheet for every source field and its destination.
   Include units for transfer quotas, timestamps/timezones, node group rules,
   order states, and protocol-specific JSON fields.
3. Create a disposable AnixOps staging database and import a small anonymized
   cohort first. Validate subscription output and node user sync before bulk
   import.
4. Do not blindly copy passwords, subscription tokens, JWTs, API keys, payment
   secrets, or certificate private keys. Require password resets where hash
   compatibility is not proven; rotate node API keys and all payment/webhook
   credentials during cutover.
5. Recreate payment gateways disabled until their callback behavior is tested.
   Do not mark old orders paid based only on a copied status flag.
6. Bulk-import only after the pilot has passed the validation checklist.
7. Keep the legacy panel read-only during final delta import, then switch DNS or
   reverse proxy routing after health, login, subscription, and node-report
   checks pass.

The source panel remains the rollback authority until the new installation has
passed a full billing cycle or the rollback window approved by the operator.

## Coordinated Node Migration

Move nodes one at a time, not all at once:

1. Keep the old node online while installing the matching AnixOps Agent release
   on a canary host or a drained node.
2. Create a new node API key in the new panel; do not reuse an old secret by
   copying it through chat or shell history.
3. Verify HTTP or gRPC configuration pull, user sync, traffic reporting, and
   online reporting.
4. For WireGuard, validate one entry/exit pair, real client import, QUIC relay,
   WSS fallback, NAT, and speed-limit behavior before moving more nodes.
5. Drain the old node only after the new node has reported correctly for an
   agreed observation period.

The corresponding node procedure is documented in
[`AnixOps Agent migration guide`](https://github.com/AnixOps/anix-agent/blob/v3.0.0-alpha.1/docs/ANIX_AGENT_MIGRATION.md).

## Cutover Acceptance Checklist

- `/health` responds through the intended reverse proxy.
- Administrator login and MFA behavior are correct.
- A normal user can log in and receive a subscription.
- Plans, quotas, active orders, and expiry calculations are correct.
- At least one node of each enabled transport reports configuration, users,
  traffic, online state, and heartbeat successfully.
- Payment callbacks are either verified or intentionally disabled.
- Backups, release checksums, imported row-count evidence, and rollback steps
  are stored with the migration record.
