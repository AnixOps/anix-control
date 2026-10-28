### Added

- forward.v1 ENGINE_NETWORK routes are planned on the network model: the
  SDK validates them when `validate.Options.EnableNetwork` is set (every
  hop ENGINE_NETWORK on one device, an explicit listen port, each link a
  `network_transport_id`, the exit a `network_egress_id`, PROXY protocol
  v1 or v2) and the forward.v1 planner leaves them out of NodeForwardState.
  Control turns each into a port forward of its entry device's
  NetworkPlan in the same transaction as the route change, pausing it
  while the kernel enforces the route's expiry or quota.
- The port forwards' byte counters in NetworkStatus meter the route's
  entry hop, so traffic statistics and quotas work as for other engines;
  a lower value than the last one starts a new counter epoch.
- `POST /api/v4/kernel/network/migrate-gost` moves every gost forward route
  onto the network model in one plan (`dry_run` reports without changing
  anything): adjacent hops get a network model transport (bare `anx_line`
  between nodes labelled `link=iepl`/`link=iplc`, the A2-jacketed `anx`
  kind with the gost link's server name otherwise) with the hop egress and
  ingress it needs, the exit an internet egress, and the entry keeps its
  allocated port. nftables routes stay; routes the model cannot carry yet
  (several nodes on a hop, a device without a line, an untrusted link
  without a server name) are listed with the reasons.
