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
