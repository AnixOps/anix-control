### Added

- Reachability probe (`internal/reachprobe`): mark proxy nodes as domestic
  or overseas vantages (`PUT /api/v4/kernel/reach/vantages`, audited), and
  every 10 minutes their Agents (5.0.9+, `reach.probe`) open and close a TCP
  connection to the enabled protocol ports of every other node. Control keeps
  the latest answer per vantage and node (migration `0039_reach_probes`) and
  `GET /api/v4/kernel/reach` answers each node's status: `ok`,
  `region_unreachable` (reachable from overseas only), `overseas_unreachable`,
  `unreachable` or `unknown`. The node list shows it in a 可达性 column.
