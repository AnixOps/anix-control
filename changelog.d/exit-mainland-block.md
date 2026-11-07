### Added

- Exit nodes can refuse mainland-China destinations. 节点 → 协议 → 出口策略
  has a switch per proxy node, off by default
  (`GET`/`PUT /api/v4/kernel/nodes/<node>/exit-policy`, audited as
  `exit_policy_set`; migration `0029_node_exit_policy`). While it is on, the
  node's desired configuration carries `exit_policy`, the Agent has the
  anx-node kernel refuse connections to mainland-China addresses (the list
  ships with the Agent) and reports the refusals on its heartbeat. Control
  counts them on the section and raises the alert
  `exit_mainland_destinations` when the node refused some within 24 hours:
  mainland traffic reaches the exit, so check the entries and the clients'
  routing. It needs an Agent and kernel of 5.0.2 or later; older ones ignore
  the setting.
