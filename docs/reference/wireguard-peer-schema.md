# WireGuard Peer Schema

Date: 2026-07-10

The P0 WireGuard panel slice stores one managed peer per user and node protocol
in `v2_wireguard_peer`. Development and test environments create this table via
AutoMigrate. Production mode skips AutoMigrate, so operators must apply an
explicit schema migration before enabling WireGuard subscriptions.

Required table:

```sql
CREATE TABLE v2_wireguard_peer (
  id INTEGER PRIMARY KEY,
  node_protocol_id INTEGER NOT NULL,
  user_id INTEGER NOT NULL,
  peer_ip VARCHAR(64) NOT NULL,
  private_key VARCHAR(64) NOT NULL,
  public_key VARCHAR(64) NOT NULL,
  preshared_key VARCHAR(64) NOT NULL,
  created_at DATETIME,
  updated_at DATETIME
);

CREATE UNIQUE INDEX idx_wg_peer_protocol_user
  ON v2_wireguard_peer (node_protocol_id, user_id);

CREATE UNIQUE INDEX idx_wg_peer_protocol_ip
  ON v2_wireguard_peer (node_protocol_id, peer_ip);

CREATE INDEX idx_v2_wireguard_peer_node_protocol_id
  ON v2_wireguard_peer (node_protocol_id);

CREATE INDEX idx_v2_wireguard_peer_user_id
  ON v2_wireguard_peer (user_id);
```

Operational notes:

- Back up the panel database before applying the migration.
- Keep `private_key` and `preshared_key` treated as secrets in exports and logs.
- UniProxy HTTP and gRPC user-list responses expose only the runtime fields V2bX
  needs for entry termination: `wireguard_peer_ip`, `wireguard_public_key`, and
  `wireguard_preshared_key`. The user private key remains subscription-only.
- Do not mark the full WireGuard relay path complete until V2bX full dual-node
  runtime, traffic accounting evidence, speed limits, GOST relay+QUIC, WSS
  compatibility mode, overseas exit NAT, and GitHub Actions verification are
  also implemented. V2bX v2.3.3 only covers initial peer online-state reporting
  from recent WireGuard handshakes.
