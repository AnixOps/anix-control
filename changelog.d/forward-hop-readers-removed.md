### Removed

- The last readers of the forward planner's allocations: route
  subscriptions offer an entry on the route's listen port, and route
  health comes from the network model only (a route left on a removed
  engine has none). The `v4_kernel_forward_allocation` table stays in the
  schema but nothing reads or writes it.
- The kernel's forward check client (`NodeChecker.Check`,
  `kernelnodeops.ForwardChecks.Check`): route diagnosis on the network
  model never runs the Agent's hop checks. Diagnosis still shows whether
  each node's Agent is connected. The administrator's node diagnostics keep
  the forward actions of `agent.diagnostic`.
