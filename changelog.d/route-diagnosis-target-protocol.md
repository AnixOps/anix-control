### Added

- Route diagnosis checks a target that is a proxy node's address against
  the node's protocols (Control's records, `CONFIG` step with the target).
  It fails as `target_no_protocol` when no enabled protocol of the node
  listens on the target's port, listing the node's ports (a route to
  `:11113` on a node whose Shadowsocks runs on 11111 and 11112 connected and
  then failed in the client, and its UDP probe only ever said
  inconclusive), and as `target_l4_mismatch` when the route does not
  forward the protocol's transport (a UDP-only route to Shadowsocks, or a
  TCP-only route to Hysteria2, TUIC, WireGuard or iWAN). A matching target
  reports `target_protocol` with the node and protocol.
