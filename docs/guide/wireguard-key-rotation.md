# WireGuard peer key rotation

`cmd/wgrotate` rotates panel-managed WireGuard client credentials without
changing peer IDs, users, protocol assignments, or tunnel IP addresses. A
rotation replaces the client private key, public key, and preshared key in one
database transaction. Every previously downloaded WireGuard profile becomes
invalid and users must refresh their subscriptions.

The command exits non-zero if database initialization, rotation, or result
output fails. Automation must treat any non-zero exit as an incomplete
operation and must not continue to node verification.

Back up the panel database before a production rotation. Preview the affected
rows first:

```bash
go run ./cmd/wgrotate -config /etc/v2board/config.yaml
```

Stop the panel to prevent concurrent subscription allocation, then run the
destructive operation with the explicit confirmation phrase:

```bash
systemctl stop v2board.service
go run ./cmd/wgrotate \
  -config /etc/v2board/config.yaml \
  -dry-run=false \
  -confirm ROTATE-ALL-WIREGUARD-PEERS
systemctl start v2board.service
```

After the panel restarts, restart every affected WireGuard entry V2bX service
to force a full peer rebuild; an incremental user sync may not detect a change
that modifies only peer credentials. Verify that each entry node has the same
peer public-key set as the panel database. Do not print private or preshared
keys during verification. Roll back by restoring the pre-rotation database
backup and restarting both the panel and the affected entry nodes.
