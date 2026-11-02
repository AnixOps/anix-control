### Removed

- Forwarding routes run on the network model only. Control's validation
  refuses a hop on any other engine (`engine_not_enabled`) or a link on any
  other security (`link_unsupported`), and the planner only validates: the
  port and mark allocation, the hop wiring of `NodeForwardState` and the
  planner goldens (`contracts/forward/v1`) are gone, so a node's forward
  state carries vantage probes and no hops. `PlanRoute` answers violations
  only. A stored route still on a removed engine stays unplanned (flagged
  `gost_removed`) until it is edited onto the network model or deleted; its
  diagnosis is the enforcement step alone.
- The entry HA controller judges every entry on the network model (the
  device's status and the route's port forward); the hop-report rules and
  the `not_in_inventory` reason are gone. Route diagnosis runs the network
  model's steps; the hop-based steps and the Agent forward checks it
  called are no longer used.
- `forward.anixops_experimental` no longer reaches the kernel: the
  `kernelforward.Service` `AnixOps` and `Cluster` hooks were removed.

### Fixed

- Entry HA vantage probes use the route's listen port. They read the entry
  hop's port from the node's planned state, which network routes never had,
  so no vantage probe was ever planned for them.
