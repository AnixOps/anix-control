# Backend Review Remediation, 2026-11

Record of how the 2026-11 third-party backend review of anix-agent
(@eb69f96) and anix-control (@6320848) was handled: every finding, whether
it held on the latest main, what changed, the evidence, and what is left.
The rule for each pull request: motive, root cause, behaviour change,
consistency and security impact, error handling, tests, compatibility and
rollback, CI link.

## Method

1. Each finding was re-checked on the latest `main` before any change. A
   finding is a claim until the code shows it: all 18 held.
2. A regression test that fails on the old code comes first where the
   fault can be reproduced in a test; the fix follows.
3. Control pull requests merge on green required CI (standing owner
   authorization). Agent pull requests merge on the owner's word.
4. No database migration was needed for any fix in this campaign.

## Findings

| ID | Finding | Verdict on main | Pull request | Merge |
| -- | ------- | --------------- | ------------ | ----- |
| AG-001 | A traffic window spooled in parts lost the unspooled part (rolled back into a counter that was already reset) | held | [agent #179](https://github.com/AnixOps/anix-agent/pull/179) | awaiting owner |
| AG-002 | Core counter rollback contract: read-and-reset, then a rollback that the core could not honour | held | [agent #179](https://github.com/AnixOps/anix-agent/pull/179) | awaiting owner |
| AG-003 | Spool: a failed write dropped older batches; an undeletable file left the index and disk disagreeing | held | agent, next PR after #179 | queued |
| AG-004 | Hot reload removed the running node before checking the new TLS certificate | held | agent, next PR after #179 | queued |
| AG-005 | Core adapter holds its lock across management API calls | held | none: assessed below | — |
| CT-001 | Online IP sets never expired (`Expire` ignored sets, no LRU) | held | [#595](https://github.com/AnixOps/anix-control/pull/595) | 359f00b |
| CT-002 | Online lookups scanned the cache with a wildcard `KEYS` | held | [#595](https://github.com/AnixOps/anix-control/pull/595) | 359f00b |
| CT-003 | Agent stream checked revocation only on heartbeats; a silent stream was never checked or ended | held | [#597](https://github.com/AnixOps/anix-control/pull/597) | da79e59 |
| CT-004 | A node could report traffic for any user id | held | [#598](https://github.com/AnixOps/anix-control/pull/598) | b37c1a5 |
| CT-005 | A failed node read counted the batch at rate 1.0 and claimed its id | held | [#598](https://github.com/AnixOps/anix-control/pull/598) | b37c1a5 |
| CT-006 | Rate applied in float64; batch totals unchecked | held | [#599](https://github.com/AnixOps/anix-control/pull/599) | 1119312 |
| CT-007 | Accounting period of a late batch undefined | held (undocumented) | [#600](https://github.com/AnixOps/anix-control/pull/600) | 7148a17 |
| CT-008 | Online set replaced key by key, not atomically, and with no version | held | [#595](https://github.com/AnixOps/anix-control/pull/595) | 359f00b |
| CT-009 | A failed config apply was not retried until reconnect or the next change | held | [#599](https://github.com/AnixOps/anix-control/pull/599) | 1119312 |
| CT-010 | Revocation bounds written read-modify-write without a lock | held | [#600](https://github.com/AnixOps/anix-control/pull/600) | 7148a17 |
| ARC-001 | One synchronous send path per Agent stream | held (per node only) | none: assessed below | — |
| ARC-002 | Session ownership per process | held (documented limit) | none: assessed below | — |
| ARC-003 | Lease without fencing | held | none: assessed below | — |

## Decisions

- **CT-007, accounting time** (owner, 2026-11-24): traffic counts in the
  period in which Control applies the batch (arrival time), not the period
  its window ended in. A batch resent after a long outage lands in the
  current day and month, and in the user's current quota period.
  `window_end_unix_ms` only orders online sets.

## Assessed, not changed

### AG-005: kernel API calls under the core's lock

The Agent's core adapter holds its lock across management API calls
(request timeout 15 s), so a hung call delays other inbounds' user syncs by
up to 15 s.

Not changed, because:

- Every inbound is served by the same local management endpoint. A call
  that hangs for one inbound almost always hangs for all of them. Per-inbound
  locks would let more calls queue on the same stuck endpoint without
  letting any of them finish.
- The lock also orders configuration changes against one shared kernel
  state (inbounds, routing rules, egress and QoS, 19 lock sites). Splitting
  it means generation checks at each of those sites, which is a large,
  race-prone change for a ≤15 s delay.
- Each node controller already serializes its own calls. Traffic and online
  reads take the lock only to look up the inbound, not for the request.

Revisit when the core serves inbounds from more than one endpoint.

### ARC-001: one send path per Agent stream

`AgentControlConnection.send` holds a per-connection mutex around
`stream.Send`.

- The mutex is per connection. Control never sends to several nodes in one
  goroutine: the approval push starts one goroutine per connection, and the
  session listing only copies state. So a slow node delays only its own
  messages.
- On that node, a heartbeat acknowledgement can wait behind a large user
  snapshot page. gRPC flow control bounds the wait, and keepalive ends a
  dead peer.

Proposal, for when one stream carries enough data to matter: a writer
goroutine per session, with a bounded queue in which control messages
(heartbeat and report acknowledgements, cancellations) go ahead of bulk
pages, and a slow-consumer cutoff that ends the session. `sendMu` is not
removed: concurrent `Send` on one stream is not allowed.

### ARC-002: session ownership is per process

This is documented and enforced: Control runs as a single instance
(`docs/architecture/container-deployment.md`), and the Helm chart refuses
more than one replica (`replicaCount > 1`). Nothing to change until the HA
work starts; the direction for it is the "Agent gRPC streams" row of that
document's single-instance table: record stream ownership (node, instance)
in the database and forward admin requests to the owning instance. The
online sets moved to `internal/online` in this campaign (#595) are in the
same position: per process, and they need a shared store for HA.

### ARC-003: lease without fencing

`internal/lease` takes and renews leases in a PostgreSQL transaction. A
leader that fails to renew for 2/3 of the TTL stops its work, but stopping
is cooperative, and lease times come from the process clock.

With one Control instance, two holders can only overlap during a rolling
update. Proposal for HA:

- a generation column in the lease row, raised on each acquisition;
- the high-risk writers (certificate issuance, upgrade campaigns,
  automatic configuration) check it in the same transaction as their write;
- lease times taken from the database clock (`now()`).

This needs a schema migration, so it waits for the HA work.

## Residual gaps

- **Crash between the core read and the spool append** (AG-001/002). The
  core resets its counters when the Agent reads them; a process crash
  before the window reaches the spool loses that window (at most one report
  interval). Closing it needs cumulative counters in the core plus a
  checkpoint the Agent persists, a core interface change that is planned
  with the core, not in this campaign.
- **A user moved to another group before the batch arrives** (CT-004). The
  node no longer serves the user, so that traffic is refused and logged
  (`traffic_user_not_served`), not counted.
- **Database counter overflow** (CT-006). Control refuses values that do
  not fit in int64 before writing. A column that would overflow in the
  database itself (PostgreSQL 22003) is reported as a transient error, so
  the batch is resent until the spool's age bound drops it. Not reachable
  with int64 counters below 2^63 bytes.
- **`recordServerStatTx`** creates the day and month rows with
  `ON CONFLICT DO NOTHING`. Two concurrent first reports of one node in the
  same period could lose one increment. Reports of a node arrive on one
  stream in order, so this needs two overlapping sessions of one node.
- **Test system** (review section 08). Added in this campaign: regression
  tests per finding, a PostgreSQL concurrency test (CT-010), fault
  injection for spool writes and deletes (AG-003), node reads (CT-005) and
  core adds (AG-004), and month-crossing accounting (CT-007). Not built:
  crash-recovery tests across a process kill, soak tests, and a
  Control + Agent + core integration run; they belong with the core
  checkpoint work above.

## CI evidence

Control, CI/CD Pipeline runs on each pull request's final head (all
required checks green; links are the workflow runs):

- #595: [45312112238](https://github.com/AnixOps/anix-control/actions/runs/45312112238).
  Go Security Scans was red on that head and on `main` for a newly
  published advisory in a dependency (GO-2026-6278), fixed on `main` by #596.
- #597: [45314365193](https://github.com/AnixOps/anix-control/actions/runs/45314365193)
  (same unrelated advisory).
- #598: [45315821826](https://github.com/AnixOps/anix-control/actions/runs/45315821826).
- #599: [45317231969](https://github.com/AnixOps/anix-control/actions/runs/45317231969).
- #600: [45319237105](https://github.com/AnixOps/anix-control/actions/runs/45319237105).
  The PostgreSQL job (shard 1/4) ran
  `TestConcurrentRevocationsKeepTheHighestBounds`: PASS.

Agent #179: [45313339316](https://github.com/AnixOps/anix-agent/actions/runs/45313339316),
[45313339405](https://github.com/AnixOps/anix-agent/actions/runs/45313339405); required
checks green, govulncheck red for a newly published advisory in a
dependency (GO-2026-6974), to be fixed by a dependency update.
