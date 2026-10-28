### Added

- `anixops.network.v1` port forwards carry their route's policy:
  `PortForward.policy` (`ForwardPolicy`: balance `round_robin` / `random` /
  `ip_hash` / `least_conn` / `failover`, TCP health checks, circuit breaker,
  PROXY header `off` / `v1` / `v2`, `bandwidth_bps`, `max_conns`) and target
  `weight` / `priority`, as the kernel's forward inbound runs them
  (networkcore #139). `sdk/network` checks the values and bounds.
