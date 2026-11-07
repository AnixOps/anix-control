### Changed

- A forward route or reverse proxy on the network model is refused when it
  is saved if its listen port is taken on the same node by another forward,
  reverse proxy or kernel forward hop, a network-model ingress, a node
  protocol or an egress relay. The v4 forwarding API answers `409 refused`
  with a `port_in_use` violation on `listen.port` naming the owner, a
  reverse proxy answers `400`. Before, the route was stored and waited in
  the plan. Routes that already conflict are left as they are until they
  are saved again.
