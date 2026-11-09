### Added

- Cross-border standby channels (节点 → 过境应急, `internal/l2standby`):
  each cross-border transport of the network model (kind `anx`, not
  trusted) gets public-protocol standby channels between the same proxy
  nodes, tier 1 VLESS + REALITY + Vision and tier 2 VLESS + XHTTP behind our
  own domain's TLS front, prepared ahead (`POST
  /api/v4/kernel/network/standby/prepare`) and switched per transport or all
  at once, and back (`POST /api/v4/kernel/network/standby/switch`, tier
  `reality`, `xhttp` or `off`). A switch reaches both nodes within a minute
  as VLESS egress relays and a catch-all egress route. Needs node kernels
  with VLESS egress relays (networkcore node-runtime) and an Agent that
  passes the relays' `settings` and `listen` through. The drill manual and
  the manual plan are in `docs/guide/cross-border-standby.md`.
- Alerts `cross_border_link_down` (a cross-border transport down for 10
  minutes; critical without a standby, warning once switched) and
  `cross_border_wide_outage` (half or more down at once).
- Entry pools (`internal/entrypool`, `docs/guide/entry-pools.md`): a node
  entry can belong to a pool and is then offered only to that pool's
  subscribers; subscribers are dealt into `shard-N` pools by user id, new
  and plan-less accounts into a quarantine pool, or assigned by an
  administrator (canary addresses of one subscriber). `GET
  /api/v4/kernel/entry-pools/leak` names the subscribers who were given a
  leaked address, `POST /api/v4/kernel/entry-pools/split` halves a pool to
  bisect a leak. The node entries sheet has a pool field.
- AnixOps Net client failure reports: `POST /{subscribe_path}/{token}/report`
  takes per-node connection outcomes (ok, fallback, timeout, refused,
  reset, after handshake, keepalive lost, stopped), once per 10 minutes per
  subscriber; `GET /api/v4/kernel/client-failures` sums them per node.
