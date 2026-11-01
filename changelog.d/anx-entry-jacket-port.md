### Fixed

- ANX links offered through a node entry or a published forward route now
  carry the jacket port subscribers can reach. The link kept the exit node's
  own `wg_port`, so a client dialed that UDP port at the entry's address,
  where nothing listens, and fell back to A2; behind an entry that only lets
  the UDP jacket through, the node never connected. A NAT entry now maps the
  jacket port through its port ranges, a route that forwards the jacket port
  offers it at the route's port, and an entry or route that does not carry the
  jacket leaves `wg_port` out of the link.
