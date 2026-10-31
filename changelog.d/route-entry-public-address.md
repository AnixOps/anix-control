### Added

- A route's entry node behind NAT can be given a public address by hand. The
  route editor has a public address field per entry node (host, host:port or
  [v6]:port), stored as the route label `entry-public/<node ref>`. Without it
  the entry follows the node's NAT entry (public address and mapped port).

### Fixed

- Control's probe of an entry listener dials the node's NAT entry or the
  route's public address at the mapped port, not the route's listen port on
  the node's first public address; subscriptions offer the same address.
