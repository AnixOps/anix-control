### Added

- Node groups hold their region relays (`relays`, host:port) and the
  relays' release caches (`release_caches`, https), migration 0049. The
  group's install command and a group node's enrollment command carry
  `--relay` and `--release-cache`; `install.sh` checks Control through the
  relays (one working is enough, none fails the preflight), joins through
  them and writes `GRPCRelays` and `ReleaseCaches` into the Agent's
  configuration. Nodes that already joined get the route in their desired
  configuration as `control_route` (an Agent without support ignores it).
