# TODO

Date: 2026-09-29

This list holds open work only. Completed items live in
[`CHANGELOG.md`](CHANGELOG.md) (and in Git history of this file); feature state
lives in [`docs/features.md`](docs/features.md). Do not mark an item done
without code, tests, and verification evidence where applicable, and remove it
from this file in the same PR that records it in `CHANGELOG.md`.

## P0: 4.0.x Release Candidate Handoff

Source: [`docs/ROADMAP-4.0.x-RC.md`](docs/ROADMAP-4.0.x-RC.md) release handoff
checklist.

- [ ] Land the reviewed RC changes on `go_dev` through PRs so that a single
  `go_dev` commit SHA (including `control-center/`) is the evidence identity.
- [ ] Run the release workflow for that SHA and retain the signed package,
  manifest, checksum, SBOM, migration, restore, and browser artifacts.
- [ ] Get an official post-push GitHub Actions run of the Control Center
  `web-test` job; local runs are not official evidence.
- [ ] Run the isolated Control/Agent staging canary with the pinned Agent
  revision. Record Agent install, observed-state write-back, restart recovery,
  timeout, health failure, duplicate request, and reverse rollback outcomes.
- [ ] Attach legacy API, subscription, node heartbeat, WebSocket, and any
  supported QUIC/WSS, WireGuard, or GOST Mesh smoke records.
- [ ] Obtain the documented operator approval, then publish the RC tag and
  release bundle. Keep execution and topology feature flags disabled until
  that approval is recorded.

## P0: Plugin Platform

- [ ] Keep Supervisor and dynamic plugin execution feature-gated until staging
  restore smoke, rollout records, legacy fallback rehearsal, and operator
  canary approval pass.
- [ ] Implement the real Agent runtime and namespace traffic evidence for
  `gost-mesh` WSS/TUIC/QUIC before the 3.3 canary.
- [ ] Move the legacy business domains out of the kernel. Every `/api/v2`
  business route is registered through `registeredPackageRoute`
  (`internal/router/router.go`) and served by a package host, but the hosts
  still bridge back into the in-kernel gin handlers and services
  (`internal/identitybridge/identity_bridge.go`); see
  `docs/architecture/release-line-status.md`.
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

## P2: Forwarding (Flux Clone)

Status and ordering live in
[`docs/guide/flux-panel-clone.md`](docs/guide/flux-panel-clone.md).

- [ ] Replace the `v2_forward` backfill with native runtime writes into `ForwardUserTunnel.inFlow/outFlow`.
- [ ] Propagate and enforce speed-limit rules on the runtime side, not only in the resource/API/UI.
- [ ] Close the user disable-state parity gap with Flux `ResetFlowAsync` / `FlowController`.
- [ ] Finish exact `user.tsx` / `limit.tsx` layout and interaction parity.

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
- [ ] The package cohort/rollback engine in
  `internal/service/plugin_rollout.go` (`BeginPackageMigration`,
  `AdvancePackageCohort`, `RollbackPackageGeneration`, ...) is not wired to
  any route or worker.
- [ ] MFA brute-force protection: failed MFA attempts are recorded, but
  `MFAService.CheckBruteForce` (`internal/service/mfa_service.go`) is never
  consulted during verification, so repeated wrong codes are not throttled.
- [ ] `cmd/migrate` (XBoard MySQL dump importer) is undocumented;
  `docs/guide/legacy-migration.md` still says there is no one-command
  converter. Document its scope and limits, or remove it.
- [ ] Control-side configuration validation for the `gost-mesh` and
  `nftables-forward` packages has not been reconnected since `7ccd4781`.

## Deployment And Repository Hygiene

- [ ] `docker-compose.prod.yml` mounts files that do not exist in the
  repository: `config/docker/postgres/init.sql`,
  `config/docker/grafana/provisioning`, and `config/docker/nginx/ssl`. Docker
  creates empty directories in their place. Add the assets or drop the mounts.
- [ ] The `Dockerfile` copies all of `config/deploy/` into the image
  (`COPY --from=builder /app/config/deploy ./config/deploy`), including node
  deployment playbooks and local deploy helpers the runtime does not need.
  Narrow it to the runtime assets (`config/deploy/ansible/`).
- [ ] `config/config.prod.yaml`, `config/config.yaml.example`, and the
  configuration that `install.sh` generates still write a
  `forward_runtime.iptables_ansible` block, but the config loader
  (`internal/config/config.go`) only parses `forward_runtime.nftables_ansible`,
  so that block is ignored. Move the templates to `nftables_ansible`.
- [ ] Runtime diagnostics (`internal/service/forward_panel_runtime_diagnostics.go`)
  and the web locale strings point operators at a nonexistent
  `docs/forward-runtime-relay-onboarding.md`; the guide is
  `docs/guide/forward-relay-onboarding.md`.
- [ ] Swagger annotations in `internal/handler/` (for example `invite.go`,
  `telegram.go`, `agent.go`) contain GBK-mojibake Chinese that propagates into
  `docs/swagger.json`, `docs/swagger.yaml`, and `docs/docs.go`. Fix the
  annotations and regenerate with `make swagger`.
- [ ] `config/scripts/test-all.sh` has mojibake comments and
  `config/scripts/coverage.sh` is not valid UTF-8.
- [ ] Remove hard-coded `/home/dev/...` defaults from
  `config/deploy/deploy_panel.sh`, `config/deploy/ansible/nodes/`,
  `scripts/deploy_wireguard_gost_quic.sh`, and the agent deploy defaults in
  `web/src/views/admin/Nodes.vue`.
