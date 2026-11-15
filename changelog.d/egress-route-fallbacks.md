### Added

- Egress routes (转发 → 分流) take ordered backup exits (`fallbacks`: node,
  optional address, relay port). Every exit serves a relay with the route's
  key; the source node's kernel tries the exits in order, skips one that
  fails to connect for 30 seconds and takes it back once it recovers, so the
  route's domains keep working when the exit node is down. Migration
  `0038_egress_route_fallbacks` adds `v5_egress_route.fallbacks`. Needs an
  anx-node with egress fallbacks; an older one keeps using only the exit node.
