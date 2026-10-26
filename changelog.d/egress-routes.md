### Added

- Domain egress routes (转发 → 分流). An egress route sends what a proxy
  node's users open on chosen domains out through another proxy node, for
  example AI services from Hong Kong through Japan, while other traffic keeps
  leaving from the node itself. `GET/POST /api/v4/kernel/egress-routes` and
  `PUT/DELETE /api/v4/kernel/egress-routes/{id}` manage them; the list shows
  whether each node's Agent applied the route. Both nodes get the route in
  the member `egress` of their desired configuration (additive: a node
  without routes has no such member and keeps its hash). The hop is a
  Shadowsocks 2022 relay on the exit node with a key Control generates; it is
  never answered, audited or put in a subscription. The member names no
  kernel (domains, relay protocol, host, port, method, key), so a later Agent
  core such as networkcore takes the same routes. Needs an Agent with egress
  support; only protocols on its Xray core take routes.
