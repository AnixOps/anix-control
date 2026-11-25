# TODO

Date: 2026-11-08

This list holds open work only. Direction and release plans live in
[`ROADMAP.md`](ROADMAP.md); completed items live in
[`CHANGELOG.md`](CHANGELOG.md) (and in Git history of this file); feature state
lives in [`docs/features.md`](docs/features.md). Do not mark an item done
without code, tests, and verification evidence where applicable, and remove it
from this file in the same PR that records it in `CHANGELOG.md`.

## 5.0.x Line

- [ ] Decide when nodes stop accepting the legacy UUID-derived Shadowsocks
  2022 user key, and how nodes and their relay child nodes move to random
  server keys (`docs/manual-intervention.md`). Random user keys exist since
  5.0.2; server keys are still derived.

- [ ] Intranet mesh (组网, `docs/design/mesh-regions`), after the first
  step (regions, fixed addresses, boundary nodes, configuration and status;
  gated feature `mesh`) and the second (device pool, host groups and
  names, the name list, region change approval, settings, enrollment
  credentials and devices without an Agent, full nodes without an Agent
  with a fixed address and confirmed subnets): per-member
  admission in the kernel instead of the region key, the `takeover` setting,
  forward nodes in the node picker, and turning the
  feature on once the node kernel runs the mesh. Also: the bridge to the
  old network during a migration, the 「未启用前向保密」 display and the
  migration progress (R30); the result of a node's own region request back
  to the node (a refused `mesh.region_request` is only in the panel); a
  warning when two gateways report the same instance id; IPv6 (fc00::/7)
  subnets for full nodes and listing the subnets a full node discovers
  for confirmation. The members', devices' and full node credentials'
  addresses are kept unique by the region lock, not by the database: one
  address table with a (region_id, address) unique index across the
  three would make it a constraint. The PostgreSQL tests of
  `internal/meshregion` create their tables with AutoMigrate, not the
  migrations.

- [ ] Region relays, deferred from the third R34 re-audit (requirements
  thread decision 2026-11-25): G6 check a group's relay addresses against
  `grpc.relay_sources` (resolving names) and warn; G7 cache download slots
  and the hourly miss bound per certificate identity rather than per
  address (two allowed hosts can still take every slot, nodes behind one
  NAT share 30 misses an hour). Next minor version.
- [ ] Region relays, with the 0.4 region model (G3): Control checks that a
  node of the mainland region is in a group with relays and warns
  otherwise; the per-region fallback setting of the R33 assessment.

## Plugin Platform

- [ ] Move the legacy business domains out of the kernel. Every `/api/v2`
  business route goes through the package gateway (`registeredPackageRoute`,
  `internal/router/router.go`), but most still fall back to the in-kernel
  handlers, services and tables: of the 132 routes in
  `config/package-extraction.json`, 14 are `native`, 102 `native-flagged`,
  8 `bridged` (`internal/identitybridge`) and 8 `kernel-owned`. Design and
  milestones: `docs/architecture/package-extraction.md`.
- [ ] `/{subscribe_path}/:token` (default `/s/:token`) is still served
  directly by a kernel handler, outside the `/api/v2` package gate. Decide
  whether to package-gate it.

## Cross-Border Standby And Canaries (paused, not urgent)

Code and playbooks are ready; the operator does these when there is time
(owner decision 2026-11-09: no reminder).

- [ ] Verify on one pair that the Agent passes the `egress` member's relay
  `settings` and `listen` through, then drill both standby tiers
  (`docs/guide/cross-border-standby.md`).
- [ ] Tier 2: an own domain (not the ICP-filed one) and the nginx front on
  each cross-border exit node.
- [ ] Buy canary exits (same provider/ASN as the real exits) and one domestic
  probe machine; deploy AnixOps-ansible `canary-egress` / `canary-probe`.
- [ ] Turn on entry pools (`PUT /api/v4/kernel/entry-pools/policy`) and give
  busy nodes per-shard and quarantine entries (`docs/guide/entry-pools.md`).
- [ ] Bump networkcore in anx so anx-node has VLESS egress relays.

## Deployment And Repository Hygiene

- [ ] Remove the hard-coded `/home/dev/...` Node.js paths from
  `config/deploy/deploy_panel.sh`.
- [ ] Background workers that contact real infrastructure (entry DNS
  updates, reverse-proxy ACME, Agent upgrade campaigns, Reality target
  rechecks, remote category-list fetches, alert digests;
  `cmd/server/singleton_workers.go`) cannot be switched off by
  configuration, so rehearsals on production copies need network namespace
  isolation. Add a config switch for outbound background work, and update the
  worker list in `docs/UPGRADE.md` ("Rehearse On A Copy First"), which still
  names the removed flux forward workers.

## Later (Not This Phase)

- Multiple Control replicas: production runs a single primary with a cold
  standby, and the Helm chart refuses `replicaCount > 1`. The blockers
  (per-replica package hosts, agent stream ownership, operation recovery,
  shared rate limits and online state, object-storage backups) are listed in
  [`docs/architecture/container-deployment.md`](docs/architecture/container-deployment.md#still-single-instance-next-round-ha);
  ROADMAP.md defers them.
