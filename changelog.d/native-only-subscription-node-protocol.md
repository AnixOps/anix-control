### Removed

- The kernel's legacy handlers for 25 more v2 routes their packages already
  answered natively in production: subscription (20: groups, templates, a
  group's protocol links, plan and user groups, statistics, the protocol
  types and the user's subscription summary), proxy-node (2: node
  statistics and node logs) and protocol-runtime (3: protocol templates,
  diagnostic task history and detail), with the `SubscriptionService`
  methods only they used. The package hosts run these routes natively from
  the start; their compatibility tests compare with recorded answers.

### Changed

- `pluginhostsdk.RouterConfig.Flagged` names the routes of a `NativeOnly`
  host that still follow the kernel's route modes and may fall back to the
  legacy handler. Subscription keeps 2 such routes (a group's protocols,
  the protocol pool), proxy-node 3 (node list, detail, delete) and
  protocol-runtime 10 (node protocols, synchronization, Agent Control and
  the agent routes): they still run their legacy handler in production.
