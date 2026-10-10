# TODO

Date: 2026-10-09

This list holds open work only. Completed items live in
[`CHANGELOG.md`](CHANGELOG.md) (and in Git history of this file); feature state
lives in [`docs/features.md`](docs/features.md). Do not mark an item done
without code, tests, and verification evidence where applicable, and remove it
from this file in the same PR that records it in `CHANGELOG.md`.

## P0: Production Upgrade (alpha build to the current release, onto containers)

Baseline and rehearsal:
[`docs/architecture/release-line-status.md`](docs/architecture/release-line-status.md#production-baseline-and-upgrade-rehearsal);
procedure: [`docs/UPGRADE.md`](docs/UPGRADE.md#upgrading-from-a-v31-or-v40-alpha-build).
The target is the container deployment (`docker-compose.prod.yml`, external
PostgreSQL), not the systemd installer.

- [ ] Production ran a `v4.0.0-alpha.6`/`alpha.7` build when it was last
  inspected (2026-09-29, `docs/architecture/release-line-status.md`). That
  build can crash with `concurrent map writes` when forward latency probes
  fail concurrently; the fix (PR #14) ships from `v4.1.0-rc.1`. Until it is
  upgraded, set `forward_runtime.latency.concurrency: 1` in the production
  config.
- [ ] Move production onto containers with a CI-built release (`v4.1.0` or
  later: the fixes the rehearsal needed, PRs #11 to #19, and the container
  work, #21 to #29 with the signed GHCR image, first shipped in
  `v4.1.0-rc.1`; the published `v4.0.0` has none of them). Back up, point
  `control.env` at the existing PostgreSQL (on the host:
  `host.docker.internal`, with `listen_addresses`/`pg_hba.conf` allowing the
  Docker bridge), run `docker compose -f docker-compose.prod.yml up -d` with
  the release digest (the image imports the identity package), install the
  other fourteen packages of the community archive, and run the smoke checks.
  Stop the old service first; never run both against one database. Read the
  4.1.0 and v4.2 sections of `docs/UPGRADE.md` first (native route defaults,
  `agent_control.mtls: required`, the signing-root change).
- [ ] Make the `ghcr.io/anixops/anix-control` package public (or grant pull
  access) after the first image push; GHCR creates it private. Still open: an
  anonymous pull-token request for it is refused as of this update.
- [ ] Optional before cutover: install Docker on the rehearsal host and repeat
  the upgrade rehearsal with the Compose file against a production copy.
- [ ] Correct the stale `app.version: 2.0.1` in the production config during
  that upgrade.

## P0: Plugin Platform

- [ ] Keep Supervisor and dynamic plugin execution feature-gated
  (`plugins.dispatch_enabled` and `plugins.topology_execution_enabled` are off
  by default) until staging restore smoke, rollout records, a legacy fallback
  rehearsal and the operator's own canary exist. This is evidence for the
  operator to collect; no release gate requires it (`docs/RELEASING.md`).
- [ ] `gost-mesh` canary evidence. The QUIC/WSS Agent runtime and namespace
  traffic acceptance exist; TUIC is out of v1 scope
  (`packages/gost-mesh/README.md`). Still needed: Control Secret ID to Agent
  private-file materialization/renewal/deletion/audit, MTU and sustained
  loss/reconnect tests, composed NAT failure rollback, accounting, multi-node
  rollback, and a sustained canary.
- [ ] Move the legacy business domains out of the kernel. Every `/api/v2`
  business route is registered through `registeredPackageRoute`
  (`internal/router/router.go`) and served by a package host. Of the 243
  routes, 177 are `native-flagged` and 130 of them run natively by default
  (`config/package-extraction.json`, `config/package-route-defaults.json`);
  13 are still `bridged` back into the in-kernel gin handlers and services
  (`internal/identitybridge/identity_bridge.go` for identity-platform) and 53
  are `kernel-owned`. The legacy handlers, services, workers and tables stay
  in the kernel, as the `legacy` fallback and as the owner of the tables.
  Design, platform gaps, and milestones M0-M4:
  `docs/architecture/package-extraction.md`. The M3 infrastructure, the
  network module runtime and the identity module are in place (storage
  leases, route modes, ledger-run migrations, the extraction map and gates,
  `internal/tests/packagecompat`, `docs/architecture/module-runtime.md`,
  `docs/architecture/identity-service.md`). Next: the kernel contracts the 13
  bridged routes wait for, then each extracted domain's workers and table
  ownership.
- [ ] Known gap: routes outside the `/api/v2` package gate are still served
  directly by kernel handlers: `/api/v1/server/UniProxy/*`,
  `/{subscribe_path}/:token` (default `/s/:token`), `/api/v1/client/subscribe`,
  and `/flow/upload`. Decide per route whether to package-gate it or keep it as
  a documented compatibility surface.

## P0: Audit Deliverables

- [ ] Keep all audit files current as fixes land.
- [ ] Keep `docs/features.md` current as new features or feature-status changes land.

## P0: WireGuard Dual-Node Entry/Exit Support

Target path: WireGuard access -> domestic entry termination -> GOST relay+QUIC
-> overseas exit NAT. WSS is a one-click compatibility mode, not the default
tunnel mode.

- [ ] Add guided entry/exit node selection plus operator migration and rollback evidence.
- [ ] Verify and, where needed, specialize subscription output for Shadowrocket, Loon, and v2rayN beyond the native `.conf`/sing-box outputs now covered by tests.
- [ ] Verify Agent runtime application for both domestic entry termination and overseas exit NAT configuration on real entry/exit machines.
- [ ] Add integration, compatibility, traffic-accounting, limit, and migration tests.

## P1: Security And Error Handling

- [ ] Add callback implementations and tests before enabling Alipay, WeChat, or USDT payment callbacks.

## P3: API And UI Consistency

- [ ] Normalize API error envelopes module by module.
- [ ] Add handler tests before changing response shapes.
- [ ] Keep frontend build/test/audit green for admin and user workflows.

## Implemented But Not Wired (keep; wire later)

These code paths exist and have unit tests, but nothing in the running server
calls them. Keep the code; wire it (with tests and docs) or delete it
deliberately in a later PR.

- [ ] Automatic notifications: `NotifyUserExpire`, `NotifyTrafficLow`,
  `NotifyTicketReply`, `NotifyOrderPaid`, `NotifyNodeOffline`, and `Broadcast`
  in `internal/service/notification_service.go` are never called.
- [ ] Invite commission accrual: `CalculateCommission` and `AddCommission` in
  `internal/service/invite_service.go` are never called.
- [ ] `LoadBalancerService.SelectNode` strategies
  (`internal/service/loadbalancer_service.go`) are unused.
- [ ] Backup retention: `CleanupOldBackups`
  (`internal/service/system_service.go`) is never scheduled, although
  `retention_days` is configurable from the admin system settings.
- [ ] `MetricsHandler.RecordRequest` (`internal/handler/metrics.go`) is never
  called, so the `/metrics` request counters stay at 0.
- [ ] `TelegramBotService.BindUser` and `Broadcast`
  (`internal/service/telegram_service.go`) have no callers;
  `SendNotification` is reached only through the unwired notification paths
  above.
- [ ] The cohort and rollback half of `internal/service/plugin_rollout.go`
  (`AdvancePackageCohort`, `RollbackPackageGeneration`) is not wired to any
  route or worker. The migration-ledger half (`BeginPackageMigration`,
  `MigratePackageHost`, `RecordPackageValidation`) runs when a storage
  package's host starts (`internal/service/package_host_migration.go`).
- [ ] MFA brute-force protection: failed MFA attempts are recorded, but
  `MFAService.CheckBruteForce` (`internal/service/mfa_service.go`) is never
  consulted during verification, so repeated wrong codes are not throttled on
  the kernel's own path. (The identity module limits second-factor attempts
  per account, 4.2.0-rc.3; that is separate code.)
- [ ] `cmd/migrate` (XBoard MySQL dump importer) is undocumented;
  `docs/guide/legacy-migration.md` still says there is no one-command
  converter. Document its scope and limits, or remove it.
- [ ] Control-side configuration validation for the `gost-mesh` and
  `nftables-forward` packages (and the in-process status/lifecycle executors
  for those two and `nat-egress`) no longer exists: nothing called it after
  `7ccd4781`, and PR #8 (`8dfc7b70`) deleted
  `internal/plugincontrol/{gost_mesh,gost_mesh_validation,nftables_forward,nat_egress}.go`.
  Rebuild it in the package hosts, or restore it from history before
  `8dfc7b70` and wire it, before these packages take production traffic.

## Deployment And Repository Hygiene

- [ ] `config/config.prod.yaml`, `config/config.yaml.example`, and the
  configuration that `install.sh` generates still write a
  `forward_runtime.iptables_ansible` block, but the config loader
  (`internal/config/config.go`) only parses `forward_runtime.nftables_ansible`,
  so that block is ignored. Move the templates to `nftables_ansible`.
- [ ] Runtime diagnostics (`internal/service/forward_panel_runtime_diagnostics.go`)
  and the web locale strings point operators at a nonexistent
  `docs/forward-runtime-relay-onboarding.md`; the guide is
  `docs/guide/forward-relay-onboarding.md`.
- [ ] Remove hard-coded `/home/dev/...` defaults from
  `config/deploy/deploy_panel.sh`, `config/deploy/ansible/nodes/`,
  `scripts/deploy_wireguard_gost_quic.sh`, and the agent deploy defaults in
  `web/src/views/admin/nodes/useNodeDeploy.js`.

- [ ] Background workers that contact real infrastructure (forward runtime
  job executor, forward agent bridge, gost/ansible stats, flow reset, latency
  prober) cannot be switched off by configuration. Staging and upgrade
  rehearsals on production copies need network namespace isolation
  (`docs/UPGRADE.md`). Add a config switch for outbound background work.

## P1: Multiple Control Replicas (HA)

Design and blocker list:
[`docs/architecture/container-deployment.md`](docs/architecture/container-deployment.md#still-single-instance-next-round-ha).
Singleton workers are already lease-guarded; the Helm chart refuses
`replicaCount > 1` until these land:

- [ ] Start enabled package hosts on every replica (reconcile per process),
  keeping lifecycle operations durable and single-claimed.
- [ ] Record agent gRPC stream and agent WebSocket ownership per replica and
  forward admin actions to the owning replica.
- [ ] Record the dispatching replica on node operations; `recoverOnce` must only
  recover operations of replicas whose heartbeat expired.
- [ ] Share online-user state (UniProxy alive lists, device limits), login and
  registration rate limits, and per-IP limiters across replicas.
  identity-platform's login, registration and subscription-reset counters are
  already in its shared `throttle` table; the kernel's legacy handlers and its
  per-IP limiters are not.
- [ ] Move backups to object storage with `pg_dump`.
- [ ] Then allow `replicaCount > 1` with a RollingUpdate strategy and a
  PodDisruptionBudget, and add a multi-replica kind smoke test.

## P2: Package Artifact Footprint

- [ ] Control package hosts materialize the whole signed `.anxp` (each bundles
  Agent binaries for several architectures, up to ~50 MB) because the host
  re-verifies the package digest; the official set needs ~0.5 GB of disk per
  Control process. Materialize only the verified control entrypoint (verify the
  package once, then pin the entrypoint digest) to cut this to a few MB each.

## Later (Not This Phase)

Recorded so they are not lost; schedule after the package-extraction
milestones in `docs/architecture/package-extraction.md`.

- [ ] Control Center convergence: the Worker/D1 users and nodes and Control's
  users and nodes are two separate systems, several Center pages are still
  mocks, and Center's own Go server is an unwired shell.
