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

## Plugin Platform

- [ ] Agent plugin dispatch and topology execution are off with no setting
  (`service.AgentPluginDispatchEnabled`) until M4's Agent release manifest
  brings the Agent plugins; the cross-repo E2E plugin scenarios skip until
  then.
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

## Implemented But Not Wired (keep; wire later)

These code paths exist and have unit tests, but nothing in the running server
calls them. Wire them (with tests and docs) or delete them deliberately.

- [ ] Automatic notifications: `NotifyUserExpire`, `NotifyTrafficLow`,
  `NotifyTicketReply`, `NotifyNodeOffline` and `Broadcast` in
  `internal/service/notification_service.go` are never called.
- [ ] `MetricsHandler.RecordRequest` (`internal/handler/metrics.go`) is never
  called, so the `/metrics` request counters stay at 0.
- [ ] `TelegramBotService.BindUser` (by e-mail; the bot's `/bind` uses
  `BindUserByToken`) and `Broadcast` (`internal/service/telegram_service.go`)
  have no callers; `SendNotification` is reached only through the unwired
  notifications above.
- [ ] `AdvancePackageCohort` and `RollbackPackageGeneration`
  (`internal/service/plugin_rollout.go`) are not wired to any route or
  worker. The migration-ledger half runs when a storage package's host starts
  (`internal/service/package_host_migration.go`).
- [ ] MFA brute-force protection: `MFAService.CheckBruteForce`
  (`internal/service/mfa_service.go`) is never consulted during
  verification, so repeated wrong codes are not throttled on the kernel's own
  path. (The identity module limits second-factor attempts separately.)
- [ ] `cmd/migrate` (XBoard MySQL dump importer) is undocumented;
  `docs/guide/legacy-migration.md` still says there is no one-command
  converter. Document its scope and limits, or remove it.

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
