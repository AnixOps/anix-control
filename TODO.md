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
  with a fixed address and confirmed subnets, the admission list
  `native_keys`): the `takeover` setting,
  forward nodes in the node picker, and turning the
  feature on once the node kernel runs the mesh. Also: the bridge to the
  old network during a migration (nodes of other implementations never
  join a managed network directly: they stay in the old network, bridged
  by this platform's nodes; the admission list `native_keys` keeps a
  node holding the region key out), the 「未启用前向保密」 display and the
  migration progress (R30); the result of a node's own region request back
  to the node (a refused `mesh.region_request` is only in the panel); a
  warning when two gateways report the same instance id; IPv6 (fc00::/7)
  subnets for full nodes and listing the subnets a full node discovers
  for confirmation. The members', devices' and full node credentials'
  addresses are kept unique by the region lock, not by the database: one
  address table with a (region_id, address) unique index across the
  three would make it a constraint. The PostgreSQL tests of
  `internal/meshregion` create their tables with AutoMigrate, not the
  migrations, and do not yet run enrollments or credentials against a
  range change. A partial unique index would hold a node or device to one
  pending region change in the database too (now a lock of its row). Low
  findings of the #634 re-review, for the next minor version: L-16
  UpdateDevice always writes device_key (an empty input clears a key an
  enrollment binds at the same time; write it only when it changes, on
  the old value); L-17 device names read their host group without a
  lock and UpdateDeviceName writes the whole row (lock the group row in
  the three paths, DeleteHostGroup locks the group before its names, use
  updateExisting); L-18 the rename test has no retry assertion or full
  node, gateway and standby rows are not locked in id order, and a
  request whose node was just moved answers 400 instead of 409. The node kernel has
  no copy of the name table yet: it is to embed
  `internal/meshregion/nametable-unicode-15.1.0.txt` and check the same
  SHA-256 as the panel's test. Admission list (#637 review, deferred to
  the next minor version): a size bound for `native_keys` agreed with the
  kernel and the Agent, and a cached list per region (L-2); an audit
  record and an overview count when a device reports another static key
  than its bound one (L-5); concurrent first reports in the PostgreSQL
  tests (L-7); the other members' push of a node's new key is started
  first but each push runs on its own, so arrival in that order is not
  guaranteed (N-1); a key bound by two devices at once, or by a device
  and a member's report at once, is not refused (N-2): the node
  channel's single-statement NOT EXISTS does not prevent it under
  PostgreSQL READ COMMITTED, nor does the device channel's check first;
  a unique index across members and devices (a static key table) or an
  advisory lock is needed, with a concurrent PostgreSQL test (#639
  review). The device list is also to mark a device whose bound key a
  member or another device has (#639 review). HARD
  PREREQUISITE before the feature is turned on anywhere but development
  (L-6): no node kernel may list the mesh capability before it parses
  `kind` and `native_keys`, or the Agent gates those fields by the
  kernel's capability (Agent #196 M1); the clients without an Agent must
  take the fields too, and the Agents must report their static keys.
  Older devices without an Agent refuse the new fields and never give a
  key.
  External bridge nodes (R36, spec section 15.12): the panel side is
  done for nodes with an Agent (0051_mesh_external_bridges). Still open:
  bridges on full nodes without an Agent (0.5 installer); intranet names
  for external devices (section 15.12 item 5); IPv6 allow entries once
  members have IPv6 addresses; members of other regions in an allow list
  (the external routes are advertised in the bridge's region only). The
  Agent gives `external_bridge` only to kernels listing
  `mesh_external_bridge` and passes the kernel status's `externalBridge`
  (allow list digest and applied time, spec #339) on as it is, which
  releases held addresses; a missing `externalBridge` is unknown and only
  an explicit null releases, and the Agent reports the alert
  `external_bridge_unsupported` when it withholds the section. Audit lows
  of #647 left for the next minor: every mesh change takes the global
  bridge lock and expands every allow list twice (skip both when there is
  no bridge, or when the change touches no address, key, host group or
  bridge); members' IPv6 /128 entries with IPv6 addresses; the Linux limit
  rests on the kernel capability (check the node's system if the panel
  learns it); tests for host group segment holds and concurrent changes
  on PostgreSQL.
  A report with `external_bridge_unsupported` counts as no bridge and
  releases holds (#650): a kernel downgraded from a version that bridged
  may leave its old firewall rules behind, which the deployment layer
  must clean up.
  Subnet requests take IPv4 networks only (IPv6 with the next minor).
  A device holding a region's resolver address (from before #641) is
  marked and moved only by an administrator ("Assign new address");
  moving it automatically is to be considered (#641 audit). The resolver
  address of an IPv6 range is not defined by the spec yet.
  The Agent must leave out `dns.resolver_address` for kernels that do not
  take it (as for the other sections), or those kernels reject the whole
  configuration; devices without an Agent take it from the release that
  supports it.
- [ ] Mesh device configuration, audit lows (#643, #645; next minor
  version): more tests; avoid scanning every device on each config read;
  the node-side warning in the Agent contract; the Swagger text of the
  device config `features` query; a report listing more than 64 features
  is cut by position, reject it or keep only known capability names.

- [ ] Region relays, deferred from the third R34 re-audit (requirements
  thread decision 2026-11-25): G6 check a group's relay addresses against
  `grpc.relay_sources` (resolving names) and warn; G7 cache download slots
  and the hourly miss bound per certificate identity rather than per
  address (two allowed hosts can still take every slot, nodes behind one
  NAT share 30 misses an hour). Next minor version.
- [ ] Region relays, sixth R34 re-audit (requirements thread decision
  2026-11-25, next minor version): P1 a generation number on the Agent's
  relay check so a result that Control's later route superseded is
  dropped; P2 a close() on the route that cancels a running check when the
  node identity stops or reloads; P3 check the relays in parallel, so two
  that connect but never answer do not use up the 30 s; P5 when the host
  cannot verify a cached zip strictly (OpenSSL older than 3: CentOS 7,
  RHEL 8, Ubuntu 20.04) download from the mirror instead of stopping; P6
  report a failed check to Control; P7 require the negotiated protocol to
  be h2; P9 apply a route's release caches while its relays are being
  checked or held back.
- [ ] Region relays, with the 0.4 region model (G3): Control checks that a
  node of the mainland region is in a group with relays and warns
  otherwise; the per-region fallback setting of the R33 assessment. Before
  Control ever sends `control_route` with `"direct": true` (the Agent obeys
  it unconditionally, anix-agent #193), it must refuse that for groups of
  the mainland region or groups that reach Control only through relays
  (fifth R34 re-audit N5).

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
