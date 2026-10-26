### Added

- 诊断 on every proxy node: the generic actions (service status, restart,
  log tail) also accept the service `anix-agent`, the node's Agent itself.
  `GET /api/v4/kernel/nodes/{id}/diagnostics` lists the services per node in
  `services`: `anix-agent` on a proxy node, plus `gost` where it forwards; a
  forward node keeps `gost`. The panel offers a service choice when a node
  runs more than one. Needs anix-agent 5.0.0-rc.6 or later, which advertises
  `agent.diagnostic` on every proxy node.
