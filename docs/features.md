# Feature Status Register

Date: 2026-09-29

This document is the current feature status index for `anix-control`.
Use it together with `TODO.md`, `CHANGELOG.md`, and `docs/audit/test-gap.md`.
When any other planning note disagrees with this file, this file is the status
source of truth.

## Status Legend

| Status | Meaning |
|--------|---------|
| Implemented | Code, route or UI entry, and focused tests exist. Operational credentials may still be required. |
| Partial | A usable slice exists, but at least one required piece is missing, such as UI, runtime enforcement, provider integration, or end-to-end evidence. |
| Compatibility | Preserved for legacy clients or external protocols; not the preferred new surface. |
| Planned | Accepted backlog item with no complete implementation yet. |
| Deferred | Explicitly postponed because it is risky, broad, or outside the current release focus. |

## Update Rules

- Every user-facing, admin-facing, node-facing, runtime, API, or deployment feature commit must update this document when capability status changes.
- Do not mark a feature `Implemented` until code, tests, and the relevant API/UI/runtime entry exist.
- Keep `CHANGELOG.md` updated for delivered behavior, `TODO.md` updated for backlog state, and `docs/audit/test-gap.md` updated for test evidence.
- Preserve legacy and third-party protocol responses separately from panel API envelope work.
- If a feature is backend-only, mocked, or only configuration storage exists, mark it `Partial`.

## Product Feature Matrix

| Area | Feature | Status | Current Surface | Remaining Work |
|------|---------|--------|-----------------|----------------|
| Platform | Product editions (`app.edition`) | Implemented | `community` (default) or `commercial`; `config/editions.json` lists the commercial packages (`order`, `payment`, `affiliate`) and routes (`GET /api/v2/user/plan`); community answers them like an undeclared `/api/v2` route (`internal/router/edition.go`), the web app hides their menus and pages (`web/src/composables/useEdition.js`, public `GET /api/v4/public/config`), and `build_package.py --all` leaves the packages out of the community release; the administrator's invite codes (identity-platform) stay in community | Publish commercial package builds (`--edition commercial`) through a channel the owner chooses. |
| Platform | Health check | Implemented | `GET /health` | None known. |
| Platform | Prometheus-style metrics | Implemented | `GET /metrics` | Broaden metric coverage as new runtimes land. |
| Platform | Swagger/OpenAPI docs | Implemented | `/swagger/*any`, `docs/swagger.*` | Keep generated docs current with route changes. |
| Platform | Security headers, CORS, request IDs, recovery logging | Implemented | Gin middleware | Continue regression coverage around auth-sensitive routes. |
| Platform | Public/admin/user rate limit middleware | Implemented | Router groups | Tune production limits from real traffic. |
| Platform | Unified panel response envelope | Partial | Most panel APIs use `code/msg/ts/data` | Continue module-by-module cleanup until all panel APIs are covered. |
| Platform | Plugin microkernel and signed package catalog | Preview/Partial | `/api/v3`, `v3_kernel_*`, official manifest verification, immutable artifact repository, installation intent, canonical operation envelopes, opt-in durable Control/Agent lifecycle dispatch, lease/cancel/restart replay, dependency-aware graph execution/rollback, monotonic topology observed-state write-back, read-only topology diagnosis/config-hash previews, deployment operation timelines, feature-gated topology fan-out, cross-repository process E2E, full local Control/Agent process rehearsal, PostgreSQL restore rehearsal, release-tag package signing/upload workflow, deterministic official package contracts, and fail-closed backend plugin route admission | Keep execution flags off by default until staging restore smoke, rollout records, legacy fallback rehearsal, and operator canary approval pass. |
| Platform | `nftables-forward` official plugin runtime | Preview/Partial | Signed 1.2.0 package with one Control/Agent schema, safe `apply=false` defaults, semantic topology preflight, `plugin.runtime-state`/`kernel.observed-state`/`plugin.cleanup`, durable pre-apply table snapshots, signed cleanup/validate modes, heartbeat-persisted live kernel ruleset hash and per-rule counters, TCP/UDP namespace traffic, exact rollback, and `SIGKILL` plus Agent-restart recovery | Retain staged restore/fallback records, IPv4/IPv6 rollout evidence, and a sustained canary before production cutover. |
| Platform | `nat-egress` official plugin runtime | Preview/Partial | Real signed-package Agent entrypoint, nftables IPv4/IPv6 masquerade, fwmark policy routing, marked interface-bound health probes, `plugin.runtime-state`/`plugin.cleanup`, crash-safe ownership journal, fail-closed `cleanup_pending`, privileged namespace forwarding/NAT/rollback acceptance, immutable Agent revision pin, and production release-signing workflow wiring | Complete multi-node failure rollback, accounting, sustained canary, and operational IPv4/IPv6 rollout evidence. |
| Platform | `gost-mesh` official plugin runtime | Preview/Partial | Real aggregate `tunnels[]` Agent entrypoint; signed checksum-pinned GOST v3.2.6 auxiliary runtime; QUIC/WSS mutual TLS; source-policy routing and source-bound health probes; bounded restart; crash-safe ownership journal and cleanup; privileged namespace TCP/UDP, wrong-SNI, untrusted-client, health, transport, and cleanup acceptance; deterministic release signing/upload wiring | TUIC is not supported by v1. Complete Control Secret ID to Agent private-file materialization/renewal/deletion/audit, MTU and sustained loss/reconnect tests, composed NAT failure rollback, accounting, multi-node rollback, and sustained canary. Keep production execution disabled until then. |
| Platform | Package-driven pluggable WebUI | Preview/Partial | Signed WebUI metadata, actor-scoped verified catalog/assets, immutable same-origin bundle storage, browser SHA-256 verification, active enabled version binding with `private, no-store` revocation, dynamic namespaced menu/route registration, backend/frontend `plugin_api` permission enforcement, unauthorized-route rejection before bundle fetch, per-plugin invalid-extension quarantine, auth/profile permission metadata, happy-dom lifecycle tests, local-module identity checks, installation configuration API, fixture E2E, and a CI live-Control gate that bootstraps a signed `machine-telemetry` package through real registration/upload/enable/disable, catalog, asset, menu, route behavior, and the merged native `/admin/plugins` view | Retain production-like staging/canary evidence for grant/revoke, update, rollback, and malformed-extension isolation; keep package runtime feature-gated until canary approval exists. |
| Platform | Compatible Control Center plugin lifecycle | Preview/Partial | Canonical `anix-control` admin plugin page plus the compatible Control Center `/plugins` view in `control-center/web` (imported from the archived `Anixops-control-center` repository), authenticated `/api/v2` Control session, direct `/api/v3` client, official catalog/release/install discovery with unverified entries blocked from install, Control and Agent target state, desired/observed version, health and failure summary, revisioned JSON configuration, enable/disable/update/rollback controls, stable retry idempotency keys for Control actions, operation-chain status with target/version history, recent operation history, operation cancellation, passing Center frontend tests (23 files / 220 tests after the import dropped mock-only suites for removed AI/Web3/observability pages), production build, Chromium login/MFA/lifecycle and 401/403/409/501 error E2E, and local full-process signed Agent install/update/enable rehearsal | Add live Control/Agent staging evidence and signed package rollout records before RC acceptance; signed WebUI modules remain rendered by Control's own frontend. |
| Platform | Plugin-only business/runtime ownership | Planned | Every `/api/v2` business route is package-gated, but package hosts still bridge into the in-kernel legacy handlers; stage history in `docs/architecture/release-line-status.md` | Follow [`architecture/package-extraction.md`](architecture/package-extraction.md): harden the v4 host platform, move production to v4, add storage leases and per-route `legacy`/`shadow`/`native` modes, then extract domains starting with `knowledge`. |
| Auth | Login and registration | Implemented | `POST /api/v2/login`, `POST /api/v2/register`, `web/src/views/Login.vue` | Keep frontend compatible with legacy and enveloped payloads. |
| Auth | Login and registration rate limiting | Implemented | `AuthHandler`, `auth_rate_limit` service | Tune limits from production signals. |
| Auth | Registration policy and invite requirement | Implemented | Config-driven registration policy (`auth.registration.require_invite`); administrators generate the codes in every edition (Admin, "Invite code management") | Add operator docs for production policy choices if needed. |
| Auth | JWT authenticated user/admin APIs | Implemented | `middleware.JWTAuth`, `middleware.AdminAuth` | Continue permission tests for new routes. |
| Auth | MFA setup, verification, and login challenge | Partial | User MFA API, admin MFA config, `/api/v2/login`, `web/src/views/Login.vue` | User-enabled TOTP/backup MFA gates token issuance, and global `enforce_for_all`/`enforce_for_admin` policies now return no-token enrollment-required responses; user-facing self-service enrollment UI still needs implementation evidence. |
| User | Profile, dashboard, subscription summary | Implemented | `/api/v2/user/profile`, `/dashboard`, `/subscription` | Continue UI regression coverage as payloads evolve. |
| User | Subscription link self-service reset | Implemented | `POST /api/v2/user/subscription/reset` (identity-platform, group A, every edition), 订阅 page danger zone dialog; re-authentication with the password or a TOTP/recovery code, three attempts per user and hour, audited; rotates the token as the administrator's reset does | None known. |
| User | User plan browsing | Implemented (commercial edition) | `/api/v2/user/plan`, user Plans page, unified success/error envelopes; hidden in the community edition | None known. |
| User | Order list, detail, and order creation | Implemented | `/api/v2/user/order*`; the caller's own orders with the plan's `id` and `name`, no buyer row | Keep amount/traffic boundary tests current. |
| User | Coupon validation | Implemented (commercial edition) | `/api/v2/user/coupon/check`; hidden in the community edition | None known. |
| User | Knowledge base browsing | Implemented | `/api/v2/user/knowledge*` | None known. |
| User | Ticket create/list/detail/reply/close | Implemented | `/api/v2/user/ticket*` | None known. |
| User | Invite codes, commission records, withdrawals | Implemented (commercial edition) | `/api/v2/user/invite*`, admin Invite page, hidden in the community edition; a user holds at most `code_count` (default 5) unused codes, as in v2board | Add provider-specific payout integration only after policy review. |
| User | User notification inbox | Implemented | `/api/v2/user/notifications*`, unified success/error envelopes | Add broader event coverage as business events grow. |
| User | Telegram binding status and notification preference | Implemented | `/api/v2/user/telegram/*`, unified success/error envelopes | Bot token/webhook operations need operator credentials. |
| User | User-managed forwarding/tunnel entries | Partial | `/api/v2/forward/*`, `/api/v2/tunnel/user/tunnel` | Close remaining Flux parity and runtime enforcement gaps. |
| Admin | Dashboard and system info | Implemented | `/api/v2/admin/dashboard` success/database-error panel envelopes, `/system/info` | Add more operational health signals over time. |
| Admin | Admin API tokens (personal access tokens for automation) | Implemented (API and console) | `POST/GET/DELETE /api/v4/kernel/api-tokens`, `Authorization: Bearer anixadm_...` on `/api/v2/admin`, `/api/v3`, `/api/v4`; hashed storage, `read` and `admin` scopes, optional expiry, last use, per-use owner check, rate-limited refusals, audit; handler, middleware, service and router tests on SQLite and PostgreSQL. Console: Security → API tokens (table with scope, status, expiry and last use, a create form with re-authentication, the token shown once, revoke); Vitest, Playwright and visual baselines | Per-area scopes if wanted; the list rows carry only the owner's id, so a super administrator sees "Administrator #7" (an `owner_email` in the list would fix it). |
| Admin | Hourly traffic and user ranking | Implemented | `/api/v2/admin/traffic/hourly`, `/traffic/user-ranking` | Watch high-volume query performance in production. |
| Admin | User management | Implemented | CRUD, ban/unban, traffic reset, subscribe reset (users can also reset their own link) with unified success/user-error envelopes | Continue authorization regression tests for new admin actions. |
| Admin | Plan management / subscription templates | Implemented | CRUD and assign with unified success/user-error envelopes; the community edition calls plans subscription templates (订阅模板) and hides prices | None known. |
| Admin | Order management | Implemented (commercial edition) | list/detail/status/paid/cancel with unified success/user-error envelopes and localized admin error prompts; list and detail carry the plan's `id` and `name` and the buyer's `id` and `email` only | Payment provider callbacks remain separate. |
| Admin | Node management and protocol configuration | Implemented | CRUD, credentials, raw config, protocol templates, auth keys | Keep AnixOps Agent and legacy V2bX compatibility tests current. |
| Admin | Subscription groups, templates, preview, user/plan binding | Implemented | `/api/v2/admin/subscription/*`, unified group/template CRUD, protocol-binding, preview, and user/plan binding success/user-error envelopes | Keep public subscription compatibility separate. |
| Admin | Ticket management | Implemented | list/reply/close | None known. |
| Admin | Coupon management | Implemented (commercial edition) | list/create/delete | Add update API only if product requires it. |
| Admin | Invite code management (registration control) | Implemented (every edition) | `GET`/`POST /api/v2/admin/invite/codes`, `DELETE /api/v2/admin/invite/codes/:id` (identity-platform, bridged to `handler.InviteCodeAdminHandler`), admin "Invite codes" page (`/admin/invite-codes`): generate 1-50 codes with an optional expiry, list and filter (unused/used/expired), copy, revoke an unused code; codes belong to no user and attribute no referral | Native identity-platform handlers need a contract or adoption for `v2_invite_code`. |
| Admin | Knowledge management | Implemented | list/create/update/delete | None known. |
| Admin | Payment gateway management and payment records | Implemented (commercial edition) | gateway CRUD/toggle, stats, records, unified success/error envelopes | Provider-specific live payment creation is not complete for every provider. |
| Admin | Notification template/log/email config management | Implemented | `/api/v2/admin/notification/*`, unified success/error envelopes | Add more event emitters as needed. |
| Admin | Telegram bot management | Implemented | bot config, webhook, users, notify, broadcast, unified success/error envelopes, test message with a result class (`POST /api/v4/kernel/notifications/telegram/test`) | Requires live bot credentials for production. |
| Admin | MFA global config | Implemented | `/api/v2/admin/mfa/config` | Global policy now affects login; user-facing enrollment UI remains tracked under Auth. |
| Admin | System config and audit logs | Implemented | `/api/v2/admin/system/configs*`, `/audit-logs`, unified config/audit success/error envelopes | Keep sensitive config masking tests current. |
| Admin | Backup config, create/list/delete/restore, stats | Implemented | `/api/v2/admin/system/backup*`, unified backup success-error envelopes | Production backup storage and restore are operator-controlled. |
| Admin | Load balancer CRUD, stats, health check | Implemented | `/api/v2/admin/loadbalancers*`, unified CRUD/stats/health success-error envelopes | Add deeper runtime traffic integration if needed. |
| Admin | WebSocket monitor | Implemented | `/api/v2/admin/ws/monitor` | Continue origin/deadline/race coverage. |
| Admin | Agent diagnostics and command dispatch | Implemented | `/api/v2/admin/agent/*`, agent WebSocket | Keep command allowlists and audit coverage strict. |
| Node | Node registration and heartbeat | Implemented | `/api/v2/node/register`, `/api/v2/node/heartbeat` | Preserve node-client compatibility responses. |
| Node | UniProxy config, user list, traffic, online state | Implemented | `/api/v2/server/UniProxy/*` | Preserve AnixOps Agent and legacy V2bX compatibility plus API key auth. |
| Node | UniProxy v1 compatibility | Compatibility | `/api/v1/server/UniProxy/*` | Do not remove without a migration window. |
| Node | Agent pull/WebSocket model for NAT-side nodes | Implemented | `/api/v2/agent/*`, `/api/v2/node/ws` | Keep lifecycle and command safety tests current. |
| Node | Agent-first gRPC control stream | Preview/Partial | `anix.agent.v1`, bidirectional hello/heartbeat/desired-operation/ACK/observed-state stream with opt-in Agent config; `package-reports.v1` latest-only package reports (`systemd.services` kernel store and `kapi_package_report_v1`; read-only node-page Services section and `machine-telemetry` route `GET /api/v3/plugins/machine-telemetry/nodes/:id/services`, off per node until enabled; Agent collector pending); `maintenance.v1` maintenance events stored once each in the node log; certificate refusal codes (`agent_cert_*`) in `x-anix-error-code`; `server_capabilities` the intersection with the Agent's `Hello`; every path `agent_control.mtls: required` refuses has a stream equivalent (Control side; the Agent side is anix-agent AG-3 to AG-5) | The alpha foundation is not yet wired to every production task source. From v4.2 `agent_control.mtls` defaults to `required`, which refuses the legacy REST and WebSocket agent paths and API-key streams (`anix-control agents transports --check-required` is the upgrade gate); with `preferred` set they remain fallbacks. UniProxy and the v2board gRPC services stay open for third-party node software. |
| Subscription | Public subscription route | Implemented | `/{subscribe_path}/:token`, default `/s/:token` | Keep UA format detection tests current. |
| Subscription | Legacy V2Board subscribe route | Compatibility | `/api/v1/client/subscribe?token=` | Preserve for old clients. |
| Subscription | Multi-format subscription output | Implemented | V2Ray, Clash/Stash, Surge, Shadowrocket, Sing-box paths | Continue parser/formatter compatibility tests. |
| Payment | Public payment methods and status | Implemented | `/api/v2/payment/methods`, `/payment/status/:trade_no` | None known. |
| Payment | EPay callback plugin | Implemented | `/api/v2/payment/callback/epay` | Keep signature and amount verification tests. |
| Payment | X402 create/check/callback | Partial (commercial edition, internal play coin) | `/payment/x402/*`; `/payment/methods` reports x402 enabled, and `/payment/x402/create` accepts a payment, only with an enabled x402 payment configuration | Not for stable releases. Current chain/provider behavior needs production integration evidence. A callback pays only in the payment's token and at least its amount; the token amount is a placeholder conversion (total / 10^8), and a payment created without a token cannot be paid. |
| Payment | Stripe and PayPal webhooks | Partial | `/payment/stripe/webhook`, `/payment/paypal/webhook` | Checkout/order creation is still mocked/stubbed. |
| Payment | Alipay, WeChat, USDT live callbacks | Planned | Config structs and gateway type constants exist; service blocks enable/use until implementations exist | Add callback implementations and tests before enabling. |
| Forwarding | v4.2 routes (hops with per-hop engines), nodes, Ansible machines, statistics, observability, diagnosis | Implemented | `/api/v4/forward/*` (forward package), admin 转发 area | See `docs/forwarding/v4-api.md`. |
| Forwarding | Flux-compatible forward, rule, node and clean agent management | Removed (v4.2, F5d) | — | Old data is archived, not migrated (F5c). |
| Forwarding | Legacy forward and tunnel lists, tunnel create/delete, permissions assign/list, speed limits (no update) | API only, legacy (until F5c) | `/api/v2/forward/list`, `/api/v2/tunnel/*`, `/api/v2/speed-limit/*` | No UI since v4.2. |
| Forwarding | Local, NodeX, and runtime diagnostics | Partial | `/forward/*/doctor`, runtime job/status APIs | Need more end-to-end runtime smoke evidence. |
| Forwarding | Traffic upload/report/snapshot | Implemented | `/flow/upload`, `/api/v2/internal/forward/traffic/*` | Keep app-token auth and stats tests current. |
| Forwarding | Legacy multi-ingress comparison and statistics | API only, legacy (until F5c) | `/api/v2/admin/forward/observability/multi-ingress`, `/api/v2/admin/forward/stats` | The v4 observability is `/api/v4/forward/observability/*`. |
| Forwarding | Complex load balancing, failover, chain orchestration | Deferred | None | Deferred by product scope for simplicity and auditability. |
| WireGuard | WireGuard user access over dual-node relay | Partial/P0 | `ProtocolWireGuard`, `v2_wireguard_peer`, native `.conf` formatter, sing-box 1.13 endpoint output, UniProxy/gRPC runtime peer fields, relay TUN contract fields, secure WSS certificate configuration, admin WireGuard visual protocol form, server keypair endpoint, API validation, IPv4-only runtime guard, runtime health/self-healing, and V2bX GOST/TUN/tc runtime tests | Panel peer custody, subscription output, node runtime peer contract, admin configuration, API validation, dynamic speed-limit convergence, runtime health reporting, relay process restart, exit-role peer/subscription exclusion, and RC/tag-gated V2bX QUIC/WSS route acceptance jobs are implemented. `v2.5.0-rc.6` passed the namespace acceptance jobs; real cross-region relay and real-client import evidence are still required before marking production-complete. |
| WireGuard | WireGuard subscription output for Shadowrocket/Loon/v2rayN | Partial | Native WireGuard `.conf` output, link-only user-agent fallback to `.conf`, and sing-box 1.13 WireGuard endpoints are implemented and unit-tested | Verify Shadowrocket, Loon, and v2rayN import behavior against real client formats before marking fully implemented. |
| WireGuard | GOST relay+QUIC tunnel backend | Partial/P0 | Panel node config carries `tunnel_type=quic`, relay TUN defaults, V2bX entry/exit runtime, process supervision, health reporting, and command tests | This is the default entry-to-exit transport; real two-machine relay-path evidence is still required. |
| WireGuard | WSS compatibility tunnel mode | Partial | `tunnel_type=wss`/`relay.wss_compat` contract, entry SNI/CA and exit certificate/key validation, admin WSS certificate form, V2bX verified command selection, and RC/tag namespace acceptance | `v2.5.0-rc.6` passed the namespace acceptance job. Production certificate rotation evidence and real cross-region compatibility evidence remain; WSS is compatibility mode, not the default. |
| Operations | Container deployment (Docker Compose, Kubernetes) | Implemented, production cutover pending | Signed multi-arch GHCR image built from release artifacts (SBOM, provenance, cosign), config-less start with `ANIX_CONTROL_*` variables and `_FILE` secrets, `/livez`/`/readyz`, drain delay, JSON logs, advisory-locked `migrate`, lease-guarded singleton workers, `docker-compose.prod.yml` with external PostgreSQL, single-replica Helm chart; CI runs the image, the Compose file and the chart in kind | Multiple replicas are blocked on per-replica package hosts, agent stream ownership and shared limits (`docs/architecture/container-deployment.md`); record the production cutover. |
| Operations | Deployment, upgrade, and production docs | Implemented | GitHub Actions release artifacts, guarded local `config/deploy/deploy_panel.sh`, guarded legacy `config/scripts/deploy.sh` and `config/scripts/pre-deploy.sh`, guarded V2bX Ansible rollout helper, CI release-build policy check, safe local artifact cleanup helper with opt-in deploy archive cleanup, `docs/DEPLOYMENT.md`, `docs/UPGRADE.md` | Keep local build guard, cleanup helper, artifact deployment docs, and upgrade rollback docs aligned. |
| Operations | SQLite to PostgreSQL migration docs and tooling | Implemented | migration commands/docs | Keep dry-run evidence for schema changes. |
| Operations | CI baseline | Implemented | GitHub Actions on Go 1.26.9 with current action runtimes for gofmt, vet, test, race, lint, gosec, govulncheck, Docker and Compose smoke, Helm chart checks, kind Kubernetes smoke, release-build policy, documentation sync, and release workflow policy checks | Watch runtime of full race testing. |
| Operations | Release workflow with artifacts, checksums, SBOM | Implemented | GitHub Actions is the required release build source; CI accepts stable/alpha/beta/RC tags and publishes `anix-control-*` assets, Docker metadata, migration evidence, manifests, checksums, SBOM, runbooks, and the signed sixteen-package cohort. `v4.0.0` was published through it on 2026-07-20 | Each new tag needs its own commit-bound run, canary/support approvals, and evidence (`docs/ROADMAP-4.0.x-RC.md`); signing-root changes follow `docs/guide/release-root-rotation.md`. |
| Operations | Automated production deployment | Deferred | Operator-controlled manual deployment | Keep credentials and production rollout manual unless explicitly approved. |

## Not Implemented Or Not Complete

These items must not be described as production-complete until the listed gaps are closed.

| Feature | Status | Why It Is Not Complete |
|---------|--------|------------------------|
| MFA self-service forced enrollment UI | Partial | Backend login now blocks JWT issuance for global `enforce_for_all`/`enforce_for_admin` policies when a covered user has not enabled MFA, but the user-facing enrollment page/flow is not complete. |
| Alipay live payment callback | Planned | Config model exists, but no provider callback implementation and tests; enable/use is blocked at service level. |
| WeChat Pay live payment callback | Planned | Config model exists, but no provider callback implementation and tests; enable/use is blocked at service level. |
| USDT live payment confirmation workflow | Planned | Config model exists, but no blockchain confirmation implementation and tests; enable/use is blocked at service level. |
| Stripe live checkout creation | Partial | Webhook verification exists; payment creation currently uses a mocked checkout URL. |
| PayPal live order creation | Partial | Webhook verification exists; order creation currently uses a mocked approval URL. |
| Full Flux Panel clone parity | Partial | Compatibility API and major UI surfaces exist, but relation-backed counters, runtime enforcement, and several parity gaps remain. |
| Runtime-side speed-limit enforcement | Partial | Management APIs/UI exist; enforcement still needs proof in runtime paths. |
| Agent-first gRPC production control plane | Preview/Partial | The bidirectional stream and bounded built-in operations exist, but it is opt-in, not every production task source dispatches through it, and full restart/replay durability still needs rollout evidence. |
| Package-driven WebUI execution | Preview/Partial | Installable immutable bundles, browser digest verification, actor-scoped catalog/asset and route/menu permission enforcement, zero-fetch rejection for unauthorized routes, active-installation asset revocation, per-plugin quarantine, auth/profile permission metadata, lifecycle component evidence, fixture Chromium failure-isolation E2E, and an isolated real-Control signed-package browser gate exist; production-like staging, sustained canary, and stable release authorization remain pending. |
| 3.3 tunnel mesh and NAT egress | Preview/Partial | Real `gost-mesh` QUIC/WSS mutual-TLS and `nat-egress` Agent runtimes, deterministic signed package inputs, policy-routing/NAT and tunnel namespace acceptance, health, crash-safe cleanup, and release workflow wiring exist. TUIC is outside `gost-mesh` v1. Secret ID materialization, composed failure rollback, accounting, multi-node rollback, and sustained canary evidence remain. |
| Plugin-only 4.0 kernel | Planned | v4.0.0 is plugin-only at the routing level only: legacy handlers, workers, global node semantics, and business table ownership remain in the kernel. Extraction design and milestones: `docs/architecture/package-extraction.md`. |
| P0 WireGuard dual-node relay | Partial/P0 | Panel peer custody, native/sing-box endpoint subscription output, UniProxy/gRPC peer runtime fields, relay TUN contract fields, secure WSS certificate contract, admin visual/keypair workflow, hardened IPv4-only protocol validation, V2bX entry interface, traffic delta parsing, peer online-state reporting, GOST TUN command planning, entry policy routing, exit NAT, tc shaping, dynamic-limit sync, process supervision, runtime health reporting, and release-gated GitHub Actions QUIC/WSS namespace route acceptance jobs exist. `v2.5.0-rc.6` accepted both transports; real client compatibility and cross-region evidence remain. |
| Automatic node deployment | Planned | Agent/Ansible surfaces exist, but one-click safe production provisioning is not complete. |
| Complex forwarding failover/load orchestration | Deferred | Deliberately postponed to keep forwarding minimal and auditable. |
| Anonymous or unaudited forwarding | Deferred | Explicitly out of scope for security and compliance reasons. |

## Compatibility Surfaces

Do not normalize these responses blindly, because external clients depend on them.

| Surface | Purpose |
|---------|---------|
| `/api/v1/client/subscribe?token=` | Legacy V2Board subscription URL. |
| `/{subscribe_path}/:token` | Public subscription downloads with client-specific formats and headers. |
| `/api/v1/server/UniProxy/*` | Legacy V2bX/UniProxy node communication. |
| `/api/v2/server/UniProxy/*` | Current node communication protocol. |
| `/api/v2/node/register`, `/api/v2/node/heartbeat` | Node registration and heartbeat client protocol. |
| `/api/v2/telegram/webhook` | Telegram webhook callback. |
| Payment callbacks/webhooks | Provider callback protocols with provider-specific response expectations. |
| Forward agent and internal flow upload endpoints | Runtime/agent protocol surfaces. |

## Related Documents

- `TODO.md`: concrete backlog and done state.
- `CHANGELOG.md`: delivered changes by commit slice.
- `docs/audit/test-gap.md`: verification evidence and remaining test gaps.
- `docs/forwarding/design.md`: forwarding module design.
- `docs/forwarding/v4-api.md`: forwarding API contract (`/api/v4/forward/*`).
- `docs/forwarding/security.md`: forwarding security constraints.
- `docs/guide/wireguard-relay.md`: P0 WireGuard dual-node relay plan.
- `docs/archive/`: the archived Flux clone guide and flux v2 forwarding contract (removed in v4.2, F5d).
- `docs/DEPLOYMENT.md`: deployment command and prerequisites.
