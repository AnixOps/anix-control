# WireGuard Peer Schema

Date: 2026-07-10

The P0 WireGuard panel slice stores one managed peer per user and node protocol
in `v2_wireguard_peer`. The server calls the idempotent
`service.EnsureWireGuardPeerSchema` hook in every environment, including
production, while the full application schema remains protected from automatic
production migration. The PostgreSQL migration tool also includes this table.

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
- UniProxy HTTP and gRPC user-list responses expose only the runtime fields
  AnixOps Agent needs for entry termination: `wireguard_peer_ip`,
  `wireguard_public_key`, and `wireguard_preshared_key`. The user private key
  remains subscription-only.
- Node config responses expose relay runtime contract fields under `relay`,
  including `role`, `server`, `server_port`, `tun_port`, `entry_tun_address`,
  `exit_tun_address`, `outbound_iface`, `routing_table`, and
  `routing_priority`. AnixOps Agent uses these to plan GOST TUN entry/exit runtime,
  source-based routing, and exit NAT command application.
- The first relay runtime is intentionally IPv4-only: peer CIDRs, relay TUN
  CIDRs, and default AllowedIPs must use IPv4 until an IPv6 route/NAT path is
  implemented.
- Do not mark the full WireGuard relay path complete until successful
  release-gated GitHub Actions QUIC and verified-WSS namespace-acceptance
  artifacts, real WSS compatibility, and geographically separated entry/exit evidence are
  recorded. The namespace test exercises the route in one runner but does not
  replace privileged two-machine evidence.
