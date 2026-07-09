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
- Do not mark the full WireGuard relay path complete until V2bX runtime, traffic
  accounting, online state, limits, GOST relay+QUIC, WSS compatibility mode, and
  GitHub Actions verification are also implemented.
