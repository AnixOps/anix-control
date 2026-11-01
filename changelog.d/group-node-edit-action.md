### Fixed

- Subscription groups: each protocol row's actions offer 编辑节点 again.
  It edits the nodes the protocol gives in the group one by one (one per
  node entry), so NAT entries that map the same port can get their own
  names. The action had slipped out of the row menu, leaving only the
  protocol-wide rename.
