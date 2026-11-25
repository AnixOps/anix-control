### Added

- Nodes can be moved into a node group by hand, one row at a time or the
  selected rows together (移动到节点组), or taken out of every group
  (`POST /api/v4/kernel/node-groups/move`, audited per node as
  `node_group_move`). The group's join defaults are not applied; its relays
  reach the nodes at their next configuration refresh. A group's node cap
  limits joins only.
- The node list shows the node group column by default and filters by group
  (`?group=<id>`, `0` for nodes without a group; `GET /api/v4/admin/nodes`
  takes `node_group_id=0`).
- The groups tab (now 分组) lists the mesh regions as network groups next to
  the node groups: a node is in one node group and, independently, in one
  mesh region.

### Changed

- The node group sheet labels its defaults as applying only to nodes that
  join later.

### Removed

- The node tag `legacy-node` (the only tag of the nodes from before the 5.0
  Agent) is cleared (migration 0053, data only).
