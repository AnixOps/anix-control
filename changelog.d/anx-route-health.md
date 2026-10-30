### Added

- ANX routes get health, latency history, diagnosis and DNS entry failover.
  The entry's anixcore checks each target through the whole route and the
  Agent reports the result in the port forward's status
  (`PortForwardStatus.targets`). Control serves it as the route's health
  (`GET /routes/{id}/health`, now with `node_ref` on each item), folds it
  into the latency history, diagnoses the route from the network model
  (device plan, port forward, transports, egress, target health, a dial of
  the entry), and lets the DNS binding judge the entry device by the same
  facts. The route page reads its health table from the route's health.
