### Added

- A subscription group edits the nodes its protocols produce: name, address,
  port, SNI and other display fields (and an advanced JSON), per node and per
  entry, without changing the source node. "Edit nodes" is in the group's
  protocol list; "Reset all edits" restores the source values.
  `GET/PUT /api/v4/admin/subscription-groups/:id/protocols/:protocol_id/nodes`
  and `DELETE /api/v4/admin/subscription-groups/:id/node-overrides`. The
  protocol type and the credentials stay the source's.
