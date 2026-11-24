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
4. No database migration was needed for the fixes of the findings
   AG-001 to CT-010. ARC-003 adds `0043_lease_generation` (additive).

## Findings

| ID | Finding | Verdict on main | Pull request | Merge |
| -- | ------- | --------------- | ------------ | ----- |
| AG-001 | A traffic window spooled in parts lost the unspooled part (rolled back into a counter that was already reset) | held | [agent #179](https://github.com/AnixOps/anix-agent/pull/179) | agent d359e2d |
| AG-002 | Core counter rollback contract: read-and-reset, then a rollback that the core could not honour | held | [agent #179](https://github.com/AnixOps/anix-agent/pull/179) | agent d359e2d |
| AG-003 | Spool: a failed write dropped older batches; an undeletable file left the index and disk disagreeing | held | [agent #180](https://github.com/AnixOps/anix-agent/pull/180) | agent 54e6d49 |
| AG-004 | Hot reload removed the running node before checking the new TLS certificate | held | [agent #180](https://github.com/AnixOps/anix-agent/pull/180) | agent 54e6d49 |
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
| ARC-001 | One synchronous send path per Agent stream | held (per node only) | [#603](https://github.com/AnixOps/anix-control/pull/603) | 52fa842 |
| ARC-002 | Session ownership per process | held (documented limit) | none: deferred to the HA work (owner) | — |
| ARC-003 | Lease without fencing | held | [#604](https://github.com/AnixOps/anix-control/pull/604) | 14862da |

## Decisions

- **CT-007, accounting time** (owner, 2026-11-24): traffic counts in the
  period in which Control applies the batch (arrival time), not the period
  its window ended in. A batch resent after a long outage lands in the
  current day and month, and in the user's current quota period.
  `window_end_unix_ms` only orders online sets.

- **Execute the proposals** (owner, 2026-11-24 16:37Z: "方案能不能直接开始执行"):
  ARC-001 and ARC-003 were implemented as proposed below.
- **ARC-002 waits for the HA work** (owner, 2026-11-24 16:41Z, decision
  card: "等多实例 HA 工作开始再做"). Control stays single-instance until then.
- **AG-005** keeps its assessment (no change): no proposal to execute.

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

`AgentControlConnection.send` held a per-connection mutex around
`stream.Send`.

- The mutex is per connection. Control never sends to several nodes in one
  goroutine: the approval push starts one goroutine per connection, and the
  session listing only copies state. So a slow node delays only its own
  messages.
- On that node, though, a write blocked by flow control (the Agent stopped
  reading while its TCP connection stays up, which keepalive does not
  detect) blocked every sender of the session, including the receive loop,
  which writes the heartbeat and report acknowledgements: nothing ended
  the session until the connection died.

Changed (2026-11-24, owner: "方案直接开始执行"), in
`internal/grpc/agent_control_writer.go`:

- one writer goroutine per session performs every `stream.Send`;
- acknowledgements (heartbeat, report, maintenance) are written before
  waiting bulk messages; operations stay in the bulk lane, so a
  cancellation never overtakes the operation it cancels;
- a write blocked for 2 minutes ends the session, and every waiting
  sender, the receive loop included, is released at once with that error;
- senders still wait for their write and get the stream's error, as
  before, so the callers' error handling is unchanged. The proposal's
  bounded queue is therefore the number of waiting senders (a handful per
  session); no message buffer was added. `sendMu` stays: it orders
  desired-state changes with their writes.

Tests: `TestAgentWriterWritesAcknowledgementsBeforeWaitingBulk`,
`TestAgentWriterEndsTheSessionOnAStuckWrite`,
`TestAgentWriterEndsTheSessionOnASendError`.

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

`internal/lease` took and renewed leases in a PostgreSQL transaction. A
leader that failed to renew for 2/3 of the TTL stopped its work, but
stopping was cooperative, and lease times came from each process's clock.
With one Control instance, two holders can only overlap during a rolling
update.

Changed (2026-11-24, owner: "方案直接开始执行"):

- migration `0043_lease_generation`: `v4_kernel_lease.generation`, raised
  each time the lease changes holder (not on renewals);
- leader work runs with a fence (lease, holder, generation) in its
  context; `lease.Check(tx)`, in the transaction of a write, reads the
  lease row (`FOR SHARE` on PostgreSQL, so a takeover waits for the write
  to commit) and returns `ErrLost` unless this process still holds it at
  that generation, unexpired by the database clock. Writes without a
  fence (administrator requests) pass;
- fenced writers: Agent upgrade campaign and node transitions
  (`agentupgrade.Worker.update`, `updateNode`), the reverse proxies' ACME
  certificate store (`reverseproxy.Issuer.store`), and the automatic
  Reality target switch (`realitytarget.AutoWorker.RunNode`). The Reality
  switch writes through the node service's own transaction, so its check
  runs just before the switch, not inside it;
- lease times come from the database clock (`now()` on PostgreSQL; on
  SQLite, one host's file, the process clock). How long a leader went
  without a renewal is measured on the process clock, which needs no
  database. A renewal that finds the lease taken back at a new generation
  restarts the work under it.

Tests: `TestTryAcquireRaisesTheGenerationOnATakeover`,
`TestCheckFencesTheWritesOfALostLeader`, `TestElectorWorkCarriesItsFence`,
`TestPostgresConcurrentAcquireGrantsOneHolder` (extended: `FOR SHARE` and
`now()` on PostgreSQL), `TestWorkerWritesNothingAfterItsLeaseIsTaken`
(fails without the check: the stale leader's write lands).

Release note: the next Control release carries `0043` with `0040`-`0042`;
back the database up before the upgrade (`docs/UPGRADE.md`).

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
- **`recordServerStatTx`** (corrected 2026-11-24). The first version of
  this report said two concurrent first reports of a node in one period
  could lose an increment through `ON CONFLICT DO NOTHING`. Checked again:
  `v5_stat_server` has no unique key on (server, type, record type,
  period), so the clause could only conflict on the generated id and never
  fired. Two concurrent first reports add two rows, and the one reader
  (`nodeDailyTraffic`) sums every row of a period: nothing is lost. The
  dead clause was removed and the behaviour commented (no schema change).
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

- #603 (ARC-001): [45440976073](https://github.com/AnixOps/anix-control/actions/runs/45440976073),
  every required check green on the first head (ccf7c7b).
- #604 (ARC-003, migration 0043): [45444915483](https://github.com/AnixOps/anix-control/actions/runs/45444915483)
  on the final head 03937ea, every required check green, the four
  Package Storage PostgreSQL shards included. Deviations on the first head
  (7a44ad9): Go Lint Gate failed on staticcheck QF1008 in
  `lease.DatabaseNow` (fixed in 03937ea); the CLA job failed on a GitHub
  API HTTP 502 while listing the commits and passed on its one re-run.

Agent (required checks green on the final heads; the owner approved each merge):

- #179: [45313339316](https://github.com/AnixOps/anix-agent/actions/runs/45313339316),
  [45313339405](https://github.com/AnixOps/anix-agent/actions/runs/45313339405).
  govulncheck was red on that head and on `main` for a newly published
  advisory in a dependency (GO-2026-6974).
- #180: [45325644744](https://github.com/AnixOps/anix-agent/actions/runs/45325644744)
  (go test + proto check, govulncheck),
  [45325644723](https://github.com/AnixOps/anix-agent/actions/runs/45325644723)
  (builds). #180 also updates go.opentelemetry.io/otel to v1.47.0, which
  fixes GO-2026-6974: govulncheck is green from it on.
