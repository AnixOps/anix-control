# Feature Status Register

Date: 2026-07-09

This document is the current feature status index for `v2board_AnixOps`.
Use it together with `TODO.md`, `CHANGELOG.md`, and `docs/audit/test-gap.md`.
`docs/FEATURE_ROADMAP.md` contains older planning notes and design material; when the
two documents disagree, this file is the status source of truth.

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
| Platform | Health check | Implemented | `GET /health` | None known. |
| Platform | Prometheus-style metrics | Implemented | `GET /metrics` | Broaden metric coverage as new runtimes land. |
| Platform | Swagger/OpenAPI docs | Implemented | `/swagger/*any`, `docs/swagger.*` | Keep generated docs current with route changes. |
| Platform | Security headers, CORS, request IDs, recovery logging | Implemented | Gin middleware | Continue regression coverage around auth-sensitive routes. |
| Platform | Public/admin/user rate limit middleware | Implemented | Router groups | Tune production limits from real traffic. |
| Platform | Unified panel response envelope | Partial | Most panel APIs use `code/msg/ts/data` | Continue module-by-module cleanup until all panel APIs are covered. |
| Auth | Login and registration | Implemented | `POST /api/v2/login`, `POST /api/v2/register`, `web/src/views/Login.vue` | Keep frontend compatible with legacy and enveloped payloads. |
| Auth | Login and registration rate limiting | Implemented | `AuthHandler`, `auth_rate_limit` service | Tune limits from production signals. |
| Auth | Registration policy and invite requirement | Implemented | Config-driven registration policy | Add operator docs for production policy choices if needed. |
| Auth | JWT authenticated user/admin APIs | Implemented | `middleware.JWTAuth`, `middleware.AdminAuth` | Continue permission tests for new routes. |
| Auth | MFA setup and verification APIs | Partial | User MFA API and admin MFA config | Wire MFA into the primary login challenge flow before marking complete. |
| User | Profile, dashboard, subscription summary | Implemented | `/api/v2/user/profile`, `/dashboard`, `/subscription` | Continue UI regression coverage as payloads evolve. |
| User | User plan browsing | Implemented | `/api/v2/user/plan`, user Plans page, unified success/error envelopes | None known. |
| User | Order list, detail, and order creation | Implemented | `/api/v2/user/order*` | Keep amount/traffic boundary tests current. |
| User | Coupon validation | Implemented | `/api/v2/user/coupon/check` | None known. |
| User | Knowledge base browsing | Implemented | `/api/v2/user/knowledge*` | None known. |
| User | Ticket create/list/detail/reply/close | Implemented | `/api/v2/user/ticket*` | None known. |
| User | Invite codes, commission records, withdrawals | Implemented | `/api/v2/user/invite*`, admin Invite page | Add provider-specific payout integration only after policy review. |
| User | User notification inbox | Implemented | `/api/v2/user/notifications*`, unified success/error envelopes | Add broader event coverage as business events grow. |
| User | Telegram binding status and notification preference | Implemented | `/api/v2/user/telegram/*`, unified success/error envelopes | Bot token/webhook operations need operator credentials. |
| User | User-managed forwarding/tunnel entries | Partial | `/api/v2/forward/*`, `/api/v2/tunnel/user/tunnel` | Close remaining Flux parity and runtime enforcement gaps. |
| Admin | Dashboard and system info | Implemented | `/api/v2/admin/dashboard` success/database-error panel envelopes, `/system/info` | Add more operational health signals over time. |
| Admin | Hourly traffic and user ranking | Implemented | `/api/v2/admin/traffic/hourly`, `/traffic/user-ranking` | Watch high-volume query performance in production. |
| Admin | User management | Implemented | CRUD, ban/unban, traffic reset, subscribe reset with unified success/user-error envelopes | Continue authorization regression tests for new admin actions. |
| Admin | Plan management | Implemented | CRUD and assign with unified success/user-error envelopes | None known. |
| Admin | Order management | Implemented | list/detail/status/paid/cancel with unified success/user-error envelopes and localized admin error prompts | Payment provider callbacks remain separate. |
| Admin | Node management and protocol configuration | Implemented | CRUD, credentials, raw config, protocol templates, auth keys | Keep V2bX compatibility tests current. |
| Admin | Subscription groups, templates, preview, user/plan binding | Implemented | `/api/v2/admin/subscription/*`, unified group/template CRUD, protocol-binding, preview, and user/plan binding success/user-error envelopes | Keep public subscription compatibility separate. |
| Admin | Ticket management | Implemented | list/reply/close | None known. |
| Admin | Coupon management | Implemented | list/create/delete | Add update API only if product requires it. |
| Admin | Knowledge management | Implemented | list/create/update/delete | None known. |
| Admin | Payment gateway management and payment records | Implemented | gateway CRUD/toggle, stats, records, unified success/error envelopes | Provider-specific live payment creation is not complete for every provider. |
| Admin | Notification template/log/email config management | Implemented | `/api/v2/admin/notification/*`, unified success/error envelopes | Add more event emitters as needed. |
| Admin | Telegram bot management | Implemented | bot config, webhook, users, notify, broadcast, unified success/error envelopes | Requires live bot credentials for production. |
| Admin | MFA global config | Implemented | `/api/v2/admin/mfa/config` | Login enforcement remains incomplete. |
| Admin | System config and audit logs | Implemented | `/api/v2/admin/system/configs*`, `/audit-logs`, unified config/audit success/error envelopes | Keep sensitive config masking tests current. |
| Admin | Backup config, create/list/delete/restore, stats | Implemented | `/api/v2/admin/system/backup*`, unified backup success-error envelopes | Production backup storage and restore are operator-controlled. |
| Admin | Load balancer CRUD, stats, health check | Implemented | `/api/v2/admin/loadbalancers*`, unified CRUD/stats/health success-error envelopes | Add deeper runtime traffic integration if needed. |
| Admin | WebSocket monitor | Implemented | `/api/v2/admin/ws/monitor` | Continue origin/deadline/race coverage. |
| Admin | Agent diagnostics and command dispatch | Implemented | `/api/v2/admin/agent/*`, agent WebSocket | Keep command allowlists and audit coverage strict. |
| Node | Node registration and heartbeat | Implemented | `/api/v2/node/register`, `/api/v2/node/heartbeat` | Preserve node-client compatibility responses. |
| Node | UniProxy config, user list, traffic, online state | Implemented | `/api/v2/server/UniProxy/*` | Preserve V2bX compatibility and API key auth. |
| Node | UniProxy v1 compatibility | Compatibility | `/api/v1/server/UniProxy/*` | Do not remove without a migration window. |
| Node | Agent pull/WebSocket model for NAT-side nodes | Implemented | `/api/v2/agent/*`, `/api/v2/node/ws` | Keep lifecycle and command safety tests current. |
| Node | gRPC panel-node services | Partial | `api/grpc`, `internal/grpc` | Needs production startup/config documentation and broader integration evidence. |
| Subscription | Public subscription route | Implemented | `/{subscribe_path}/:token`, default `/s/:token` | Keep UA format detection tests current. |
| Subscription | Legacy V2Board subscribe route | Compatibility | `/api/v1/client/subscribe?token=` | Preserve for old clients. |
| Subscription | Multi-format subscription output | Implemented | V2Ray, Clash/Stash, Surge, Shadowrocket, Sing-box paths | Continue parser/formatter compatibility tests. |
| Payment | Public payment methods and status | Implemented | `/api/v2/payment/methods`, `/payment/status/:trade_no` | None known. |
| Payment | EPay callback plugin | Implemented | `/api/v2/payment/callback/epay` | Keep signature and amount verification tests. |
| Payment | X402 create/check/callback | Partial | `/payment/x402/*` | Current chain/provider behavior needs production integration evidence. |
| Payment | Stripe and PayPal webhooks | Partial | `/payment/stripe/webhook`, `/payment/paypal/webhook` | Checkout/order creation is still mocked/stubbed. |
| Payment | Alipay, WeChat, USDT live callbacks | Planned | Config structs and gateway type constants exist | Add callback implementations and tests before enabling. |
| Forwarding | Flux-compatible forward CRUD and ordering | Partial | `/api/v2/forward/*`, admin Forward page | Remaining Flux parity and runtime edge cases. |
| Forwarding | Tunnel CRUD and user assignment | Partial | `/api/v2/tunnel/*`, admin Tunnel/Users surfaces | Relation-backed flow counters and enforcement gaps remain. |
| Forwarding | Speed-limit management | Partial | `/api/v2/speed-limit/*`, admin Limit page | Runtime-side speed-limit enforcement is not fully proven. |
| Forwarding | Forward node and Ansible machine management | Implemented | `/api/v2/admin/forward/nodes*`, `/ansible-machines*` | Keep operator setup docs current. |
| Forwarding | Local, NodeX, and runtime diagnostics | Partial | `/forward/*/doctor`, runtime job/status APIs | Need more end-to-end runtime smoke evidence. |
| Forwarding | Traffic upload/report/snapshot | Implemented | `/flow/upload`, `/api/v2/internal/forward/traffic/*` | Keep app-token auth and stats tests current. |
| Forwarding | Observability targets, trend, topology, multi-ingress | Implemented | `/api/v2/admin/forward/observability/*` | Add more real runtime data sources if needed. |
| Forwarding | Complex load balancing, failover, chain orchestration | Deferred | None | Deferred by product scope for simplicity and auditability. |
| Operations | Deployment script and production deploy docs | Implemented | GitHub Actions release artifacts, guarded local `config/deploy/deploy_panel.sh`, guarded legacy `config/scripts/deploy.sh` and `config/scripts/pre-deploy.sh`, guarded V2bX Ansible rollout helper, CI release-build policy check, safe local artifact cleanup helper, `docs/DEPLOYMENT.md` | Keep local build guard, cleanup helper, and artifact deployment docs aligned. |
| Operations | SQLite to PostgreSQL migration docs and tooling | Implemented | migration commands/docs | Keep dry-run evidence for schema changes. |
| Operations | CI baseline | Implemented | GitHub Actions on Go 1.26.5 with current action runtimes for gofmt, vet, test, race, lint, gosec, govulncheck, Docker smoke | Watch runtime of full race testing. |
| Operations | Release workflow with artifacts, checksums, SBOM | Partial | GitHub Actions release workflow is the required release build source | Keep release notes and migration dry-run artifacts complete for tags. |
| Operations | Automated production deployment | Deferred | Operator-controlled manual deployment | Keep credentials and production rollout manual unless explicitly approved. |

## Not Implemented Or Not Complete

These items must not be described as production-complete until the listed gaps are closed.

| Feature | Status | Why It Is Not Complete |
|---------|--------|------------------------|
| MFA login challenge enforcement | Partial | Setup/verify APIs exist, but login does not yet require the second factor. |
| Alipay live payment callback | Planned | Config model exists, but no provider callback implementation and tests. |
| WeChat Pay live payment callback | Planned | Config model exists, but no provider callback implementation and tests. |
| USDT live payment confirmation workflow | Planned | Config model exists, but no blockchain confirmation implementation and tests. |
| Stripe live checkout creation | Partial | Webhook verification exists; payment creation currently uses a mocked checkout URL. |
| PayPal live order creation | Partial | Webhook verification exists; order creation currently uses a mocked approval URL. |
| Full Flux Panel clone parity | Partial | Compatibility API and major UI surfaces exist, but relation-backed counters, runtime enforcement, and several parity gaps remain. |
| Runtime-side speed-limit enforcement | Partial | Management APIs/UI exist; enforcement still needs proof in runtime paths. |
| gRPC production node transport | Partial | Protobuf/server/client code exists; production startup, config, and integration rollout are not complete. |
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
- `docs/forwarding/api.md`: forwarding API contract.
- `docs/forwarding/security.md`: forwarding security constraints.
- `docs/guide/flux-panel-workstream.md`: Flux clone status and parity requirements.
- `docs/DEPLOYMENT.md`: deployment command and prerequisites.
