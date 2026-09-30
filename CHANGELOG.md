# Changelog

## Unreleased

### Security

- User access tokens are verified in one place, `internal/authn`, and can be
  revoked (N8). It covers the HTTP middleware, the admin monitor WebSocket
  and the gRPC interceptor.
  - **Pinning.** The algorithm is pinned to HS256, which closes the
    algorithm-confusion gap. `iss`, `exp` and `iat` are required.
  - **Session id.** New tokens carry a session id (`sid`); the login
    response is unchanged.
  - **Revocation tables.** New tables `v4_kernel_identity_revocation`
    (per user: `not_before`, `token_version`) and
    `v4_kernel_identity_session_revocation` (session denylist). They are
    cached in memory and reloaded every 5 s.
  - **Triggers.** Banning or deleting a user, or changing their password,
    email, admin flag or ban flag, revokes that user's existing tokens at
    once. Before, a banned user's token kept working until it expired (24 h
    by default). Updates that resend unchanged values revoke nothing.
  - `utils.ParseToken` is removed; use `authn.Default().Verify`.

### Removed

- The legacy source/Docker Compose installer in the root `install.sh`
  (`ANIX_CONTROL_LEGACY_SOURCE_INSTALL=1`). It built images on the target
  host against the release policy. `install.sh` now only forwards to the
  frozen systemd release installer and no longer carries version fields
  (`check_release_version.py` and AGENTS.md updated).
- Removed the legacy V2bX/Xray proxy-node test tooling, which was never part of
  a release build: `cmd/integration-test`, `cmd/configgen`, `cmd/report`,
  `cmd/verify`, `cmd/subtest`, the `internal/tests/integration` harness packages
  (`binary`, `clients`, `config`, `runner`, `echo`, `e2e`, `local`, `mock`), the
  `integration-test.yml` workflow, the `make test-integration` target,
  `config/examples/`, `tools/mock_gost_api.py`, and the unused
  `config/scripts/{setup_integration.go,generate-grpc.sh,coverage.sh,test-all.sh}`
  and `api/grpc/gen.ps1` helpers (use `api/grpc/gen.sh`). The package rollout
  tests in `internal/tests/integration` are kept.
- Removed Go code that no binary reaches: the unused `internal/websocket`
  subscription hub, the in-memory agent task store, the never-registered gRPC
  `ConfigSyncService` implementation (the `.proto` and generated code are
  unchanged), the in-process Control plugin executor registry and its
  GOST mesh / NAT egress / nftables forward executors (the production kernel
  never installed it), the `gost.Manager` rule-sync methods, and unused helpers
  in the cache, compat v2, gRPC, middleware, parser, utils and service layers.
  Unwired but implemented notification, invite commission, load balancer,
  backup cleanup and Telegram bot features are intentionally kept. Test-only
  helpers moved into test files, and backup tests now write to temporary
  directories instead of the source tree.
- `golang.org/x/net` is now an indirect dependency.

- Removed dead frontend code from `web/`: the unreferenced
  `views/admin/forward/ForwardCard.vue`; the System page's hidden runtime
  config editor state and save handler (`saveForwardRuntimeConfig`,
  `runtimeConfigPreview`, `applyDefaultRuntimeAnsibleConfig`,
  `runtimeSaving`, `runtimeValidationError`), which the template never
  rendered (runtime settings stay editable on the Local Runtime and NodeX
  Runtime pages); 21 API client helpers that only tests called (17 in
  `api/admin.js`, `getDashboard` and `getKnowledgeDetail` in `api/user.js`,
  `getKernelPluginReleaseArtifact` and the `diagnoseKernelTopology`
  compatibility wrapper in `api/kernel.js`); unused `menuRegistry`
  re-exports and admin extension exports from `extensions/runtime.js`; and
  the unused `PRODUCT_NAME` and repository URL constants.
- Removed 95 unused i18n keys from both `en` and `zh-CN`, plus the legacy
  literal-translation entries whose source strings no longer appear in the
  frontend, backend, packages, or agent (23 in `en`, 30 in `zh-CN`).
- Stripped UTF-8 byte order marks from nine frontend source files.

- Removed obsolete and personal repository files: the Windows-era `.claude/`
  directory (personal settings, memory, and `v2board.exe` build commands),
  `.superpowers/`, the agent plans/specs under `docs/superpowers/`, the
  2026-04 agent work plans (`docs/guide/forward-runtime-work-plan.md`,
  `docs/guide/flux-panel-workstream.md`), the retired 3.1-to-4.0 planning docs
  (`docs/FEATURE_ROADMAP.md`, `docs/architecture/upgrade-program.md`,
  `docs/architecture/plugin-platform-roadmap.md`), stale V2bX-era and one-off
  docs (`docs/V2BX_LOCAL_NODE_SETUP.md`, `docs/ANSIBLE_INTEGRATION_GUIDE.md`,
  `docs/arco-design-vue-setup.md`, `docs/guide/test-release-v2.0.2-test.1.md`,
  `docs/audit/admin-workbench-verification-2026-07-18.md`,
  `docs/coverage/grpc-coverage.html`), the personal-domain Nginx configs in
  `config/deploy/nginx/`, the unused `config/deploy/gost/` bundle, and the
  V2bX-era `scripts/check-health.sh` and `scripts/setup.sh` (which carried a
  hard-coded token). `docs/FEATURE_ROADMAP.md` is no longer accepted as
  documentation evidence by `config/scripts/check_docs_updated.sh`.

### Changed

- `anix-control module ...` commands log to stderr, so their JSON output on
  stdout can be piped (for example into `jq`).
- The AnixOps contracts and SDKs are now their own Go module,
  `github.com/AnixOps/anix-control/sdk`, in `sdk/`.
  - **Moves.** `api/{pluginhost,packagebridge,modulepki,identity,kernelidentity}`
    moved to `sdk/api/...`, and `pkg/{moduletls,pluginhostsdk,packagebridgesdk,modulesdk,packagestoresdk,v2compat}`
    moved to `sdk/...`. Import paths change accordingly; the wire formats do
    not.
  - **Consumers.** Other AnixOps services can depend on the contracts and
    SDKs, versioned by `sdk/vX.Y.Z` tags, without pulling in the kernel. The
    kernel uses the module through a local `replace`.
  - **Boundary gate.** `check_package_boundaries.sh` now fails when the SDK
    module depends on the kernel module. The SDK contract tests that use the
    kernel's real bridge moved to `internal/tests/bridgecontract`.
  - **CI and tooling.** CI builds, vets, tests (including race), lints,
    tidies and scans the SDK module. The Dockerfile copies `sdk/go.mod`
    before downloading modules.

- The generic Control package host and the identity-platform host now run
  on `pluginhostsdk.Router`. They have no native routes, so every request
  still passes through the bridge to its legacy handler.
- Package migration indexes are materialized per release.
  - Source `packages/*/migrations/index.json` files use the
    `__ANIXOPS_PACKAGE_VERSION__` token.
  - `build_package.py` writes the build version and a SHA-256 for every step
    script into the packaged index. It also rejects unknown index fields,
    source-provided digests and malformed step ids.
  - Packages built for any tag other than `v4.0.0` previously failed: the
    source index was pinned to `4.0.0` and had to equal the build version.
- The kernel now parses and verifies the migration index when it materializes
  a Control artifact. It checks the index digest, the package and version,
  every step id and path, and every step digest. The verified steps are
  exposed on the artifact reference so package migrations can be run through
  the ledger. v4.0.0 indexes without step digests stay valid.
- Manifest `capabilities` are validated by Control.
  - Names are lowercase and unique.
  - In the `kernel.` namespace only the listed forms are accepted:
    `kernel.observed-state`, `kernel.storage.v1`,
    `kernel.storage.adopt:<table>` and `kernel.view:kapi_<name>_v<N>`.
  - Kernel and identity tables cannot be adopted.
  - See `docs/architecture/plugin-kernel-contract.md`.

- Documentation now treats containers as the primary deployment.
  - README, `docs/README.md`, `docs/control-boundary.md`,
    `docs/features.md`, AGENTS.md and the UPGRADE alpha section point to the
    Compose and Helm paths first. The systemd installer is marked frozen in
    the release installation guide, and `scripts/install.sh` prints a notice.
  - New `docs/reference/environment-variables.md` lists all `ANIX_CONTROL_*`
    variables. It is generated from the configuration structure and kept
    current by `TestEnvironmentVariableReferenceIsCurrent`.
  - `TODO.md` now aims the production upgrade at containers and tracks the
    multi-replica (HA) work.

- Rewrote `docker-compose.prod.yml` for container-first production.
  - It runs only Control against an external PostgreSQL: a one-shot
    `migrate` service, then `control`.
  - The image is pinned by digest (`ANIX_CONTROL_IMAGE`), with no local
    build.
  - Settings come from `control.env` (`config/deploy/compose/control.env.example`).
    The JWT secret and database password come from `secrets/` files
    passed as `ANIX_CONTROL_*_FILE`.
  - Hardening: read-only root filesystem with an exec `/tmp` tmpfs,
    `cap_drop: ALL`, `no-new-privileges`.
  - Operations: `/readyz` healthcheck, 90 s stop grace period, rotated JSON
    logs; ports published on `127.0.0.1` by default.
  - Removed the bundled Redis (unused by the code), Postgres, Prometheus,
    Grafana and nginx services and their mounts of files that did not
    exist.
- `docker-compose.yml` is now a development stack: it builds the `source`
  target with a throwaway PostgreSQL and no longer bind-mounts `web/public`
  over the built frontend.
- The nginx and Prometheus configs moved to `config/deploy/examples/` as
  host-side examples for the published ports (`config/docker/` is gone).
- `.env.example` now documents the Compose variables (`ANIX_CONTROL_IMAGE`,
  bind addresses, ports) instead of settings the server never read.
- `docker-smoke` also runs the production Compose file against a separate
  PostgreSQL: `migrate` completes, `control` becomes healthy, stops cleanly.


- The container image was rebuilt for container-first deployments.
  - **Targets:** `source` (default) builds from the checkout. `release`, used
    only by CI, copies the exact release binary, frontend and signed
    `identity-platform` bootstrap package. A fresh database therefore
    bootstraps without extra files
    (`ANIX_CONTROL_PLUGINS_IDENTITY_BOOTSTRAP_PACKAGE_DIR` is preset).
  - **Base images:** pinned by digest. The Go and frontend stages
    cross-compile on the build platform.
  - **Runtime:** runs as uid 10001 (`anixops`, previously uid 1000
    `v2board`) under `tini`, with no config file, and works on a read-only
    root filesystem with a writable `/tmp`. `HEALTHCHECK` uses `/readyz`.
    Ansible state goes to `/tmp/.ansible`.
  - **Contents:** only the ansible playbooks and `ansible.cfg` are copied.
    `.dockerignore` keeps `config/deploy/ssh`, inventories, keys, `.env`
    files and `config/tls` out of the build context. The legacy `/app/v2board`
    symlink is gone.
- Release images are published to `ghcr.io/anixops/anix-control` instead of
  Docker Hub.
  - Platforms: `linux/amd64` and `linux/arm64`.
  - Tags: `X.Y.Z`, `X.Y` (stable only) and `sha-<commit>`.
  - SBOM and provenance attestations are attached, and the digest is signed
    with cosign keyless signing.
  - `docker-image.txt` records the digest, platforms and signature.
  - `check_release_workflow.sh` enforces these properties.
  - Pushes to `go_dev` publish `:edge` and `:sha-<commit>` images for
    pre-release testing.
- `Docker Build Smoke` now runs the image instead of only building it:
  `migrate` against PostgreSQL; a read-only, non-root container configured
  only by environment; `/readyz`, `/livez` and an unauthenticated `/api/v3`
  401; then a graceful `docker stop` that must exit 0.

- Recorded the production baseline and the local upgrade rehearsal from
  `v4.0.0-alpha.7` to `go_dev` on a copy of the 2026-09-29 production database
  (`docs/architecture/release-line-status.md`, `docs/RC-EVIDENCE-4.0.x.md`).
  Subscriptions (360/360) and UniProxy (45/45) were byte-identical, and all
  82 `/api/v2` GET routes returned the same status. `docs/UPGRADE.md` gains an
  alpha-to-4.0.x upgrade section: build identification, the five added kernel
  tables, the package install window, and network isolation for rehearsals.
  It also documents the empty-password DSN fix. `TODO.md` tracks the
  production upgrade, the random order of forward observability targets, and
  the missing switch for outbound background workers.

- `/api/v2` route resolution no longer re-reads and re-hashes every installed
  package artifact on each request. Successfully verified route declarations
  are cached per signed release (keyed by release id, artifact SHA-256, stored
  trust-root fingerprint and configured trust-root fingerprint; LRU-bounded to
  256 releases; failures are never cached), shared by the HTTP and WebSocket
  gateways. Installations, the official plugin row and trust-root activity are
  still read on every request (four small queries in total), so disabling a
  package, bumping its generation, or retiring its trust root takes effect on
  the next request. With 16 installed ~10 MiB packages on SQLite a resolution
  drops from about 197 ms to about 0.22 ms. `internal/compat/v2` joins the
  benchmark smoke CI job.

- The required "Go Quality Gates" CI job now runs the plugin-only `/api/v2`
  route gate (`check_plugin_only_routes.py`, which includes the route catalog
  check), the `config/scripts` Python unit tests, and generated-code drift
  checks for `api/pluginhost` and `api/packagebridge` on every pull request.
  Previously the route gate ran only on release tags and the Python tests never
  ran in CI.

- Made the `go_dev` CI pipeline green again. Go moves to `1.26.8` (go.mod
  toolchain, CI, SDK sync workflow, and a `golang:1.26-alpine` Docker builder),
  and `google.golang.org/grpc` moves to `v1.83.2` (with `golang.org/x/net`
  `v0.58.0`, `x/text` `v0.41.0`, `x/crypto` `v0.55.0`), which clears every
  govulncheck finding that reaches our code while keeping `go 1.25.0` as the
  module minimum. The web lockfile picks up fixed `postcss`, `nanoid` and
  `brace-expansion`, and Vitest moves to `^4.1.11`, so `npm audit` is clean.
- CI scanners are pinned (`golangci-lint` `v2.14.0`, `govulncheck` `v1.8.0`,
  `gosec` `v2.29.0`) so new upstream rules no longer turn `go_dev` red without a
  code change. The root pipeline drops the nonexistent `production` branch
  trigger, runs on every pull request again (its jobs become the required
  checks for `go_dev`), and cancels superseded pull-request runs.

- Rewrote the repository entry points: `AGENTS.md` is now a concise English
  rules file for the v4 kernel and package bridge, branch/PR workflow and
  required checks, build/test commands, the documentation sync gate, version
  bump surfaces, release policy, Flux-clone and runtime guardrails, the
  Control Center, and credential rules. The dated Flux clone status moved into
  `docs/guide/flux-panel-clone.md`. `ROADMAP.md` now points at the maintained
  roadmap documents, and `TODO.md` keeps only open items plus a new
  "Implemented but not wired" list and deployment hygiene gaps.
- `docs/reference/repository-layout.md` is the single repository layout
  document (the README copy was removed); `docs/intro/README.md` gained a
  "v4 architecture at a glance" section; `docs/README.md` and
  `docs/guide/README.md` index every current guide; docs no longer link to
  gitignored local files, deleted docs, `config/examples/`, or Windows paths.
- `.github/BRANCH_PROTECTION.md` documents the `go_dev` ruleset, and
  `.github/CODEOWNERS` is a single valid UTF-8 default rule.
- Repaired GBK-mojibake comments in `Makefile`, `config/config.prod.yaml`, and
  `docker-compose.prod.yml` (which also lost its UTF-8 BOM); the repaired
  `make help` text no longer has unterminated quotes. No configuration values
  changed.
- `.gitignore` now ignores agent-local state (`.claude/settings.local.json`,
  `.claude/projects/`, `.superpowers/`).
- Corrected documentation made stale by PR #7 and PR #8: `TODO.md` (removed
  the deleted `test-all.sh`/`coverage.sh` item; gost-mesh/nftables-forward
  Control validation must now be rebuilt; gost-mesh is QUIC/WSS with TUIC out
  of v1 scope; new "Later" section), the release-workflow row in
  `docs/features.md` (`v4.0.0` was published), the audit registers (current
  toolchain, commands, packages, workflows; removed `internal/websocket` hub),
  the RC roadmap/evidence notes, and the trust-root paragraph in
  `docs/architecture/plugin-kernel-contract.md` (Control keeps one active
  root and retires the others at startup).

- Imported the Control Center into `control-center/` as a single snapshot of the
  archived `AnixOps/Anixops-control-center` repository (`master` merged with
  `production`, without committed release binaries or assistant notes). It stays
  a separate Go module, `github.com/AnixOps/anix-control/control-center`, and
  its CI and release pipelines moved to `.github/workflows/control-center.yml`
  and `control-center-release.yml` (tags `control-center-v*`, never marked as
  the latest release). The Center web and Flutter clients dropped the AI, Web3
  and unrouted observability mock pages whose Workers endpoints are being
  removed. `go_dev` is now the only long-lived branch.
- The root CI pipeline now ignores changes limited to `control-center/**`, and
  its gosec and swag steps, the Makefile `swagger` target and the Docker build
  context exclude `control-center/`.
- Imported the Control Center Cloudflare Workers API into
  `control-center/workers/` as a single snapshot of the archived
  `AnixOps/Anixops-control-center-worker` repository, trimmed to the routes the
  Control Center clients use (platform probes, auth/MFA, users, nodes,
  node-groups, playbooks, tasks, schedules, notifications, dashboard, audit
  logs, SSH, plugins, agents, logs, backups, batch, SSE/WebSocket). Incidents,
  governance, webhooks, Kubernetes/load-balancer/mesh/autoscaling, AI/vector,
  Web3/IPFS and developer-mode routes were removed, and the unused `AI` binding
  was dropped. D1 migrations are unchanged. CI runs from
  `.github/workflows/control-center-workers.yml`; deployment stays on Cloudflare
  Workers Builds (root `control-center/workers`, branch `go_dev`).

### Added

- Deployment of network modules (N6).
  - **`Dockerfile.module`.** One image per package, built from the
    package's own host or the generic host. Distroless, uid 65532, about
    15 MB, remote mode by default.
    - `go_dev` publishes `ghcr.io/anixops/anix-module-identity-platform:edge`
      and `:sha-<commit>` for amd64 and arm64, signed with cosign.
  - **Compose.** The `docker-compose.modules.yml` overlay runs
    identity-platform as its own container next to Control; the guide is
    `config/deploy/compose/modules.md`.
  - **Helm (chart 0.2.0).**
    - `moduleRuntime` turns on Control's mTLS module listener: port 7443 on
      the Deployment and Service, and the CA key from the Secret.
    - `modules.<id>` adds one Deployment per module, with an optional
      NetworkPolicy.
    - Rendering fails without a CA key or with modules but no module
      runtime.
  - **CLI.** `anix-control module runtime list|set` selects a package's
    runtime (`local` or `remote`).
  - **CI smoke tests.** Both build the module image and log in through
    identity-platform running as a separate service:
    `modules_compose_smoke.sh` (production Compose files) and
    `modules_kind_smoke.sh` (the chart on kind, two replicas, NetworkPolicy
    on, pod replacement).
- Module SDK for network modules (N5).
  - **`pkg/modulesdk.Run`.** Runs a host as the kernel's local child process
    or, with `ANIX_MODULE_MODE=remote`, as a network module.
    - Enrolls from a credential file, or uses externally issued
      certificates.
    - Stores and renews its certificate (0600 files) and reuses it after a
      restart.
    - Serves the host protocol over mTLS, accepting only the kernel.
    - Binds, heartbeats and rebinds.
    - Answers `/livez` and `/readyz`.
    - Drains gracefully on SIGTERM.
    - The official generic and identity-platform hosts, and the storage test
      fixture, now start through it; local behaviour is unchanged.
  - **`packagebridgesdk.NetworkClient`.** The remote package bridge.
    - Session tokens travel on every call.
    - A call rejected for an unknown session binds again and retries once.
    - Concurrent rebinds collapse into one.
  - **`Resume`.** `pluginhostsdk.ResumablePackage` and `Router.Resume` let a
    drained network module serve again after it binds to a new generation.
  - **`packagestoresdk`.** PostgreSQL pools fetch the current storage lease
    before every new connection, so rotated role passwords heal without
    `28P01` failures.
  - **Trust bundle.** The kernel exports the module CA bundle for
    deployments: `GET /api/v4/kernel/modules/trust-bundle` and
    `anix-control module ca bundle`.

- Remote runtime for Control packages (N4).
  - **Selecting the runtime.** A package can run from network module
    instances instead of a local child process.
    - Set it with `PUT /api/v4/kernel/modules/runtimes/:plugin_id`
      (table `v4_kernel_plugin_runtime`).
    - Remote requires `module_runtime.enabled` and PostgreSQL.
    - A change applies at the package's next lifecycle operation.
  - **Instance pool.** Remote hosts live in the same host supervisor as
    local ones.
    - Calls go round-robin over the instances bound to the generation.
    - An instance that fails at the transport is ejected.
    - Health is cached for 2s, so per-frame WebSocket checks stay local.
    - Dispatch, WebSocket relays, ledger migrations and fencing use the same
      code as local hosts.
  - **Starting a remote generation.** It waits
    (`module_runtime.bind_timeout`, default 2m) for a bound, healthy
    instance.
    - A new version keeps the old generation serving until then.
    - The same version fences the old generation first so its instances
      rebind.
  - **Storage.** Replicas of one generation share one storage lease instead
    of rotating each other's password. `module_runtime.database_host` sets
    the database address in remote leases. Remote packages cannot lease
    SQLite storage.
  - **Startup order.** The module listener and remote runtime start before
    restart reconciliation, so remote packages never start locally by
    mistake.

- Kernel module listener and remote bridge sessions (N3).
  - **Listener.** With `module_runtime.enabled`, the kernel serves `ModulePKI`
    and the package bridge on an mTLS listener (`module_runtime.listen`,
    default `:7443`).
    - TLS 1.3 only.
    - Every RPC except `Enroll` needs a verified, unrevoked module
      certificate.
    - Revocations made by the kernel take effect on the next call.
  - **Bind and sessions.** Remote instances call `Bind` and get a session
    token for their package's current generation.
    - Later calls must carry the token from the same module identity.
    - Heartbeats keep a session for 15 s and report fencing and draining.
  - **Generation sessions.** Capabilities now live in a per-generation
    `GenerationSession`, so any instance of a generation can redeem them,
    once.
    - Local hosts keep their private socketpair on top of the same state.
    - Remote installations are admitted once the remote runtime manager
      lands; until then `Bind` answers `FailedPrecondition`.
  - **Maintenance.** The kernel promotes rotated module CAs and prunes expired
    certificate records every hour.

- Built-in module PKI for network modules (N2).
  - **CA.** `internal/modulepki` is an ECDSA P-256 CA.
    - Its key is sealed under `module_runtime.ca_kek`.
    - A name constraint limits it to `spiffe://anixops/...` identities.
    - It is created at startup when `module_runtime.enabled` is set.
    - Rotation keeps the next and retired CAs trusted while their
      certificates are valid.
  - **Certificates.** Short-lived, 24h by default, with exactly one SPIFFE
    URI SAN; requested subjects and SANs are ignored. Renewal runs over mTLS.
  - **Enrollment credentials.** Hashed, one-time or reusable, consumed
    atomically. Revoking one revokes the certificates issued through it.
  - **Administration.** `POST/GET/DELETE /api/v4/kernel/modules/enrollment-tokens`,
    `POST /api/v4/kernel/modules/ca/rotate`, and the CLI
    `anix-control module token|ca ...`.
  - **External CA.** `module_runtime.pki: external` loads a trust bundle and
    kernel certificate (for example from cert-manager) and reloads them on
    change.
  - **`pkg/moduletls`** holds the SPIFFE identity rules and TLS 1.3
    configurations shared by the kernel and modules. Peers are verified by
    identity on every handshake, including resumed ones.
  - **New tables:** `v4_kernel_service_ca`, `v4_kernel_module_enrollment`,
    `v4_kernel_module_certificate`.
  - **New settings:** `module_runtime.*` (`ANIX_CONTROL_MODULE_RUNTIME_*`),
    off by default.

- Contracts for network modules and the identity service (N1).
  - **New protocols.**
    - `api/modulepki/v1` `ModulePKI`: `Enroll`, `Renew`, `GetTrustBundle`.
    - `api/identity/v1` `IdentityService`: `GetTokenKeys`,
      `BatchGetAccounts`, `ImportAccounts`, `ExportAccounts`.
    - `api/kernelidentity/v1` `KernelIdentity`: subscriber allocation,
      account projection, revocation, actor access, settings.
  - **New RPCs on existing protocols.** `Bind` and `Heartbeat` on
    `KernelPackageBridge`, and `Resume` on `ControlPackageHost`. Existing
    hosts answer them with `Unimplemented`, so v4.0.0 packages are
    unaffected.
  - **Compatibility gate.** `internal/tests/protocompat` checks every
    AnixOps protobuf contract against `contracts/proto/descriptors.golden`.
    Removing or renumbering a method, field or enum value fails; additions
    need an explicit `-update`.
  - **Token contract.** `contracts/identity/v1` fixes the EdDSA access-token
    claims and the tokens the kernel must reject.
  - **CI.** The protobuf drift check covers the new generators.
  - **Design documents.** `docs/architecture/module-runtime.md` and
    `docs/architecture/identity-service.md`. `package-extraction.md` no
    longer keeps identity in the kernel: login and credentials become the
    first network module.

- Extraction gates and a legacy/native comparison harness (M3).
  - **Extraction map.** `config/package-extraction.json` records every
    `/api/v2` route's extraction mode (`bridged`, `native-flagged`,
    `native`) and where its legacy handler lives. All 292 routes are
    `bridged`.
  - **Route gate.** `check_plugin_only_routes.py` enforces the map:
    - a `native` route binds the bare gateway and has no legacy handler left;
    - `native-flagged` and `native` routes need a package host that
      implements them;
    - the identity-platform exception, formerly hardcoded, is now the map's
      `identity-bridge` source, checked against `internal/identitybridge`.
  - **Inventory.** The route inventory reports each route's
    `legacy_handler`.
  - **Worker gate.** `check_plugin_only_workers.py` fails when `cmd/server`
    starts a new business worker and keeps the list of the seven legacy
    domain workers shrink-only.
  - **Boundary gate.** `check_package_boundaries.sh` fails when
    `packages/...` or `pkg/...` depend on `internal/`. Test imports need a
    reasoned allowlist entry.
  - **CI.** All three gates and their tests run in the required Go Quality
    Gates job.
  - **Harness.** `internal/tests/packagecompat` (`RunRead`, `RunWrite`) runs
    a route's legacy handler and its native implementation on identically
    seeded databases. It compares status codes, bodies normalized with
    `v2compat`, and, for writes, the database state. It uses SQLite, and
    also PostgreSQL schemas when `ANIX_TEST_POSTGRES_DSN` is set.

- Package migrations run through the migration ledger when a storage
  package's host starts (M3).
  - **Scope.** Applies to releases that declare `kernel.storage.v1`. After
    `plugin.install`, `enable`, `update` or `rollback` starts the host, the
    dispatcher runs the release's migration index before the operation
    succeeds.
  - **Ledger.** There is one run per lifecycle generation, with id
    `index.<index digest>`. Upgrades record an `operator-managed:` backup
    reference.
  - **Host side.** The host applies its embedded index through
    `packagestoresdk.IndexMigrator`, the new
    `pluginhostsdk.RouterConfig.IndexMigration` hook, and reports
    `packagestoresdk.StepsDigest`.
  - **Check.** The kernel requires that digest to match the verified index
    of the artifact.
  - **Failures and restarts.** A failed migration stops the host and fails
    the operation. A Control restart confirms the recorded run instead of
    migrating again.
  - **Other packages.** Packages without the capability, including all
    v4.0.0 packages and identity-platform, are unaffected.
  - **Test fixture.** `internal/tests/packagefixture/storagehost` is a real
    host binary used by the end-to-end tests on SQLite and PostgreSQL.

- Per-package database storage and the `LeaseStorage` bridge RPC (M3).
  - **Roles.** On PostgreSQL, `internal/packagestore` gives every package a
    login role `anix_pkg_<id>` (NOINHERIT, 4 connections, member of the
    privilege-less group `anix_packages`) and a kernel-owned schema
    `pkg_<id>` in which the role creates its own tables.
  - **Grants.** Grants come only from the signed manifest:
    `kernel.storage.adopt:<table>` grants DML on the table and its sequences,
    and `kernel.view:<view>` grants `SELECT`. Each lease replaces the
    previous grants, so a dropped capability is revoked.
  - **Leases.** Hosts call the new session-scoped RPC `LeaseStorage`.
    - It is fenced like `GetPackageConfig`.
    - It returns a connection string for the package role only. Each lease
      rotates the password through a SCRAM verifier computed by the kernel.
    - The kernel's database role needs `CREATEROLE`; without it leases fail
      with an explicit error.
    - SQLite shares the kernel's file with a `pkg_<id>_` table prefix and
      provides no isolation.
  - **Ledger.** The new kernel table `v4_kernel_package_storage` records each
    package's role, schema, grants and lease generation.
  - **View.** The kernel API view `kapi_user_directory_v1` exposes
    non-secret `v2_user` columns. It is created at startup; a failure is
    logged and does not stop Control.
  - **SDK.** `packagebridgesdk.Client.LeaseStorage` and
    `pkg/packagestoresdk` (`Open`, `SharedOpener`, `Store.Table`,
    `RunEmbeddedMigrations` with per-step digests and a `schema_migrations`
    state table).
  - **CI.** The new job `package-storage-postgres` runs the grant tests as a
    non-superuser `CREATEROLE` role. It also runs the PostgreSQL package
    rollout tests, which no job ran before.

- A per-route package router in the host SDK (M3).
  - **Router.** `pkg/pluginhostsdk.Router` polls `GetPackageConfig` every
    5 s and serves each route in `legacy`, `shadow` or `native` mode.
    - A failed poll keeps the last modes.
    - Routes without a native handler stay `legacy` and are reported as
      `mode_unsupported`.
    - `shadow` returns the legacy response, then compares the native one in
      the background with a timeout and a concurrency limit.
    - WebSocket routes always relay through the bridge.
  - **v2compat.** `pkg/v2compat` holds the v2 panel envelope
    (`PanelSuccess`, `PanelError`) and `NormalizeForCompare`. The kernel's
    panel helpers now use it, and their output is byte-identical.
  - **Health and metrics.** The supervisor calls `Health` on running hosts
    every 30 s and keeps the returned details. `/metrics` exports them as
    `anixops_package_config_status`, `anixops_package_route_mode` and the
    `anixops_package_native_*` and `anixops_package_shadow_*` counters.

- Package route modes and the `GetPackageConfig` package bridge RPC (M3).
  - **Route modes.** Control package configuration documents reserve a
    top-level `routes` key that maps v2 route ids to `legacy`, `shadow` or
    `native`.
  - **Validation.** The kernel checks the modes against the release's
    verified compatibility routes: `shadow` only on GET routes, and WebSocket
    routes stay `legacy`. It then removes the key before applying the
    package's own schema.
  - **Reading the config.** Hosts read their configuration with the new
    session-scoped bridge RPC `GetPackageConfig`.
    - It needs no per-request capability: it is authorized by the session's
      package id, version and lifecycle generation.
    - Disabled packages and stale versions or generations are fenced.
  - **SDK.** `packagebridgesdk.Client.GetPackageConfig` reports an older
    kernel as `ErrSessionOperationUnsupported`, which means every route stays
    `legacy`.

- A Helm chart for Kubernetes (`config/deploy/helm/anix-control`), single
  replica with an external PostgreSQL.
  - `Recreate` strategy. `replicaCount` other than 1 is rejected until
    multi-replica support lands.
  - A `migrate` init container. It is used instead of a Helm hook so it
    always sees the release's Secret and ConfigMap.
  - Probes: startup and liveness on `/livez`, readiness on `/readyz`.
  - Settings come from a ConfigMap of `ANIX_CONTROL_*` variables. Secrets,
    chart-created or `existingSecret`, are mounted as `_FILE` files. Config
    and secret checksums roll the pod.
  - Security: non-root uid 10001, read-only root filesystem, all
    capabilities dropped, `RuntimeDefault` seccomp, exec `emptyDir` `/tmp`.
  - Optional: gRPC Service for nodes, Ingress, `ServiceMonitor`, and an
    ansible inventory/SSH Secret.
  - CI: `Helm Chart Checks` (strict lint, kubeconform on several value sets,
    guard failures) and `Kubernetes Smoke`.
  - `.gitignore` now ignores only the root `/anix-control` binary. The bare
    `anix-control` pattern also matched any directory of that name, including
    the chart. The smoke installs the freshly
    built image into kind with an in-cluster PostgreSQL, probes it, upgrades
    it and uninstalls it.
- Database leases (`internal/lease`, table `v4_kernel_lease`) keep single-instance
  background work in one Control process per database. The following run only
  in the process holding the `control.singleton-workers` lease:
  - the forward runtime job executor and forward agent bridge worker;
  - the flow and monthly resets;
  - the gost and ansible stats workers;
  - the latency prober.

  The lease lasts 30 s and is renewed every 10 s. A leader that cannot renew
  stops its workers after 20 s, and a leader that shuts down releases the
  lease, so a rolling update or an accidental second process no longer
  double-runs jobs, resets or traffic accounting. `/metrics` reports
  `anixops_lease_leader`. `docs/architecture/container-deployment.md` records
  the container design and the remaining multi-replica blockers.

- `anix-control migrate` prepares the database, then exits: schema, `Ensure*`
  helpers, plugin trust root, identity package bootstrap, and default seeds.
  Use it as a one-shot Compose service or Kubernetes Job.
- Database preparation now runs under a PostgreSQL advisory lock, in both the
  server and `migrate`. Several processes starting at once run it one at a
  time instead of racing on DDL and seed rows.
- Production databases get their tables without `env: development`.
  - An empty database gets the full schema.
  - An existing database only gets the tables it lacks; existing tables,
    columns and indexes are never altered.
  - Previously production mode skipped table creation entirely, so a fresh
    production install needed `env: development` for its first start.
  - Verified on a copy of the production database: only the five
    `v4_kernel_package_*` tables were added.
- A configured `plugins.control_host_artifact_dir` is cleared of package
  copies left by earlier processes at startup.

- Container probes on both the API and UI servers:
  - `/livez`: the process answers.
  - `/readyz`: startup finished, not draining, and the database answers a
    ping.
  - `/health` keeps its response shape. The API server's copy now also
    returns `503 {"status":"draining"}` during shutdown, like the UI
    server's.
- `server.shutdown_drain_delay`: time to keep serving after readiness starts
  failing on shutdown, so load balancers and Kubernetes endpoints stop routing
  first. The default is 0; the built-in container defaults use `5s`.
- `log.format: json`: one JSON object per line on stdout for standard log
  lines, slog records, HTTP access logs (path without query string) and GORM.
  `log.level` filters structured records; standard log lines are always
  written. `text` (the default) keeps the historical output.
- When the frontend directory is missing and cannot be created (for example on
  a read-only root filesystem), the server now logs a warning instead of
  failing to start.

- Configuration can now come from the environment, for container deployments.
  - Every scalar or list key has an `ANIX_CONTROL_<PATH>` variable, for
    example `ANIX_CONTROL_DATABASE_PASSWORD` or `ANIX_CONTROL_JWT_SECRET`.
  - Each variable also has a `_FILE` variant that reads the value from a
    mounted secret file.
  - Variables are applied on top of the config file.
  - Unknown `ANIX_CONTROL_*` names are logged.
  - `anix-control -print-env` lists every variable with its default.
- The server can start without a config file.
  - When neither `-config` nor `ANIX_CONTROL_CONFIG` is given and
    `config/config.yaml` does not exist, the server uses built-in,
    production-shaped defaults (`internal/config/defaults.yaml`).
  - A config file named explicitly must still exist.
  - In production, startup refuses an empty or template `jwt.secret`.
- New PostgreSQL connection settings:
  - `database.sslmode` (default `disable`) and `database.timezone` (default
    `Asia/Shanghai`), so managed databases that require TLS can be used;
  - `database.dsn`, a full connection string that is used verbatim.
- `env: production` now defaults `server.mode` to `release`; gin previously
  fell back to debug mode.

- Added `plugins.control_host_max_request_bytes` and
  `plugins.control_host_max_response_bytes` (default 1 MiB, maximum 64 MiB)
  to configure the package request and response body limits; see
  `docs/reference/configuration.md`. Hosts receive the response limit as
  `ANIX_CONTROL_HOST_MAX_RESPONSE_BYTES` (`pluginhostsdk.MaxResponseBytesFromEnvironment`),
  and gRPC message sizes on the kernel, bridge and host follow the limits.
  `pluginhostsdk.HostServerOptions()` is the new recommended host server
  option set.
- Added v2 gateway metrics to `/metrics`:
  `anixops_v2_gateway_requests_total{package,route,code_class}`,
  `anixops_v2_gateway_errors_total{package,route,code}` and the
  `anixops_v2_gateway_request_duration_seconds{package}` histogram, labelled by
  the declared package route id (never the raw path), plus the package host
  supervision counters from `Supervisor.Stats()`:
  `anixops_plugin_host_{starts,unexpected_exits,restarts,failures}_total{package}`
  and the `anixops_plugin_host_state{package,state}` gauge.
- Added `docs/architecture/package-extraction.md`, the design of record for
  moving business domains into packages: the current routing-only reality,
  the definition of done, the planned storage-lease / per-route-mode / typed
  kernel operation mechanism, the preserved constraints, invariants, domain
  data contracts, rollout and gate designs from the retired
  `docs/superpowers/` plans, the platform gaps, and milestones M0-M4 with a
  `knowledge` pilot. `ROADMAP.md`, `AGENTS.md`, `TODO.md`, and
  `docs/features.md` point to it.
- Added the operator runbook `docs/guide/release-root-rotation.md`, extracted
  from the retired root-rotation plan and checked against the current code
  (single active root, retired-root releases stop verifying, re-signed
  packages need a new version).

- Exposed `X-AnixOps-Operation-Chain` consistently for Control plugin
  installation upserts, configuration writes, and lifecycle actions, including
  dependency apply plans while excluding rollback operations.
- Added the compatible Control Center `/plugins` lifecycle slice: a direct
  authenticated `/api/v3` client, official catalog and release discovery,
  Control/Agent installation state, revisioned configuration editing,
  idempotent lifecycle actions, operation-chain display, health/failure
  summaries, and focused store/view/browser coverage. Its Control administrator
  session uses `/api/v2/login` and a separate token; the legacy Workers
  `/api/v1` client and login remain independent. The Center web build now
  processes Tailwind styles and exposes mobile navigation for the plugin page;
  transient lifecycle retries reuse their idempotency key, and unverified
  catalog entries cannot be installed from the UI. A dedicated Control Center
  browser gate now drives a temporary real Control process through Control
  login and authenticated catalog/installation reads.
- Merged the Center plugin operation-history experience into the canonical
  Control administrator plugin page. It now shows recent plugin operations,
  supports cancellation, and reuses lifecycle idempotency keys after transient
  Control failures.
- Established the formal product-stage contract from `3.1` through `4.0`,
  separate from the Go module `/v4` import path. The earlier `v3.1.0-alpha.2`
  candidate remains historical planning context; the active worktree follows
  the `4.0.x` RC roadmap and its signed package scope is tracked separately.
- Added an operational `/admin/access-groups` surface, server-side
  effective-access preview, and an identity-only group detail response.
- Added a real Control-to-Chromium signed WebUI E2E gate that proves package
  registration, artifact upload, enable, catalog/asset/menu/route loading, and
  durable disable revocation.
- Added a default-off Agent Supervisor canary to the node deployment flow,
  including secure gRPC endpoint checks, canonical Ed25519 public-key checks,
  and Ansible group-variable/template propagation.

### Fixed

- The web frontend depends on axios 1.20.0. axios up to 1.19.0 is affected
  by newly published advisories (prototype-pollution gadgets, ReDoS, HTTP/2
  proxy bypass and DoS), which made `npm audit` fail the Frontend Build
  check.

- Configuration changes to a Control package no longer fail. Its `plugin.configure`
  operation completes immediately, because hosts pull the configuration. Before
  this, the host lifecycle dispatcher rejected the operation as unsupported.

- `docker-compose.prod.yml` keeps verified package copies on a disk-backed
  `plugin-artifacts` volume (`/var/lib/anixops`, pre-created in the image for
  uid 10001) instead of the 512 MB RAM-backed `/tmp` tmpfs. With the sixteen
  official packages (~0.5 GB of copies) the tmpfs filled up and package hosts
  failed with "verified artifact reference is unavailable"; found by the
  container cutover rehearsal on a production database copy. The Helm chart's
  disk-backed `/tmp` limit is raised from 1 Gi to 2 Gi.

- `TestElectorStopsWorkWhenRenewalsFail` waits for the leader flag to clear
  instead of reading it the moment the work returns; it failed under the race
  detector.

- The edge and release image jobs pin `sigstore/cosign-installer@v4.1.2`:
  the action publishes no floating `v4` tag, so the jobs failed to start.

- Periodic traffic resets now run at most once per calendar day.
  - The forward flow reset and node monthly reset workers also run at every
    start, so restarting the server (or a rescheduled container) on a reset
    day zeroed user, tunnel and node traffic a second time.
  - Each run now claims the day in `v2_system_config`
    (`scheduler.forward_flow_reset.last_day`,
    `scheduler.node_monthly_reset.last_day`). The claim is made in the same
    transaction as the reset.
  - Expiry checks still run every time.
- Forward traffic snapshots now lock the forward row while advancing the
  traffic cursor. Two processes applying the same gost or ansible snapshot
  could both read the old cursor and double-count the delta; the in-process
  lock only covered one process.
- `OrderService.Complete` and node self-registration now really lock their
  rows. They used the GORM v1 `gorm:query_option` setting, which GORM v1.25
  ignores.
- The node request-signature nonce cache is now safe for concurrent requests:
  the map had no lock, and the check-then-mark sequence let two requests with
  the same nonce both pass.

- Legacy `/api/v2` handlers now see the original request `Host` and TLS state
  through the package bridge. The bridge rebuilt every request for the
  placeholder host `package-bridge`, so `GET /api/v2/forward-agent/install.sh`
  defaulted `PANEL_URL` to `http://package-bridge` and Telegram webhook
  registration without an explicit URL pointed at the same host. The address
  is kept in the kernel's bridge snapshot only; package hosts built with the
  `v4.0.0` SDK still receive the unchanged metadata. Found by the local
  upgrade rehearsal against `v4.0.0-alpha.7`.

- `/api/v2` error responses now keep the exact body the legacy handler wrote
  on `data` and `panel` package routes (status >= 400 with a JSON body). The
  gateway used to wrap `{"error": ...}`/`{"message": ...}` failure bodies as
  `{"data":{"error":...}}` on data routes and reject them as an invalid panel
  envelope (502) on panel routes, so 39 routes (agent/node admin, invite,
  WireGuard keypair, traffic reports) lost their error messages or turned
  client errors into 502. Found by the local upgrade rehearsal against
  `v4.0.0-alpha.7`.

- Fixed `/api/v2` package route resolution picking the first declared matching
  pattern instead of the most specific one. `GET /api/v2/admin/users/stats`,
  `/admin/orders/stats` and `/admin/nodes/stats` were dispatched to the
  `/:id` routes of the same package (400 or a wrong body). Resolution now
  follows gin's precedence (a static segment beats a parameter, left to
  right), within a package and across packages; only equally specific
  matches remain an ambiguity error. Found by the local upgrade rehearsal; a
  new test resolves every catalogued route through each package's real
  `compat/v2-routes.json` and checks the resolved route ID.
- Fixed a process crash (`fatal error: concurrent map writes`) in the forward
  background error logger. The latency prober logs probe failures from
  parallel goroutines, so when several probe targets were unreachable at the
  same time the unsynchronized rate-limit map aborted the whole server; this
  cannot be caught by `recover`. Found by the local upgrade rehearsal, where the
  production release (`v4.0.0-alpha.7`) crashed within a second of startup.
- A package response over the size limit is now reported as `502
  plugin_response_too_large` instead of `plugin_host_unavailable`. The package
  bridge, `pkg/pluginhostsdk` and the kernel now apply the same body-size rule
  (the SDK used to count the whole encoded response against 1 MiB, so a legacy
  body just under 1 MiB passed the bridge and then failed in the host); the
  bridge and SDK return `codes.ResourceExhausted`, and the kernel maps it to the
  new `pluginhost.ErrResponseTooLarge`.
- A panic in a legacy `/api/v2` handler reached through the package bridge, in
  a package bridge or node-facing gRPC handler, or in a package host no longer
  terminates the process. The bridge HTTP adapter now answers such a panic
  with an empty `500` (the same response as the kernel's gin recovery
  middleware; partial handler output is discarded), and the WebSocket adapter
  closes the relay with `1011`. The package bridge session and the kernel
  node gRPC server install recovery interceptors (new `internal/panicrecovery`)
  outermost and return `codes.Internal`. `pkg/pluginhostsdk` recovers panics
  in `Dispatch`, `Migrate`, `OpenWebSocket`, `Health` and `Drain` as
  `codes.Internal`, and its new `RecoveryServerOptions()` is used by the
  shared control host and identity-platform host. The kernel now reports a
  host's `codes.Internal` as `pluginhost.ErrPackageFailed` (still wrapped in
  `ErrHostUnavailable`, so the gateway response is unchanged). Recovered
  panics are logged with a stack trace bounded to 16 KiB.

- PostgreSQL connection strings are now built by `database.PostgresDSN`, which
  single-quotes and escapes every value. Before, an empty `database.password`
  made the driver read `password= dbname=x` as the password `dbname=x` and
  connect to the user's default database; empty hosts and values containing
  spaces, quotes or backslashes broke the same way. An empty host or a zero
  port now falls back to the driver default. `cmd/sqlite2postgres
  -target-config` uses the same builder. `github.com/jackc/pgx/v5` is now a
  direct dependency.
- `cmd/server` now shuts down in order and waits for its background work. A
  SIGINT/SIGTERM root context stops up to ten background workers (forward
  runtime, bridge, reset, stats and latency workers, the Control plugin
  lifecycle worker, the plugin operation dispatcher and the topology
  executor), and shutdown runs: `/health` draining, HTTP drain (30s), wait for
  workers (15s), gRPC stop (10s), Control plugin host shutdown with its own
  15s timeout, then cache and database close. A listener failure or plugin
  and dispatcher initialization error after startup no longer calls
  `log.Fatal`: it runs the same shutdown and exits with status 1. Plugin poll
  interval settings are validated before any listener starts. A second signal
  still forces an immediate exit.

- A package host that crashes or is killed no longer leaves its routes failing
  until the next lifecycle operation or reboot. The host supervisor
  (`internal/pluginhost`) logs the exit status and restarts the host at the
  same generation, version, and verified artifact after 1 s, 2 s, 4 s, ...
  (capped at 30 s); after 5 restarts within 5 minutes it marks the host failed
  and stops restarting until the next explicit lifecycle operation (enable,
  update, rollback, or boot reconciliation), which also resets the budget.
  While a host is exited, restarting, or failed, dispatch, health, migration,
  and WebSocket requests fail immediately with `ErrHostUnavailable` instead of
  dialing a dead socket. Host stdout and stderr are now written to the kernel
  log line by line as `[pkg:<id> v:<version> gen:<n> stderr] <line>` (lines
  capped at 8 KiB) instead of being discarded; `TZ` and `LANG` are passed to
  hosts when set in the kernel environment, and both bundled hosts embed
  `time/tzdata`. Stopping a host now sends `SIGTERM` to its process group and
  waits up to 2 s before `SIGKILL` (the bundled hosts stop gracefully on
  `SIGTERM`); on Linux a host also receives `SIGKILL` if the kernel dies.
  `Stop` and watchdog restarts no longer hold the supervisor lock that request
  dispatch uses, and `(*pluginhost.Supervisor).Stats()` exposes per-package
  start, unexpected-exit, restart, and failure counters.
- Made historical `v4.0.0-alpha.*` tags audit-only in release automation and
  made unconfigured product stages fail closed. Docker publication now waits
  for signed package publication.
- Removed the production WebUI runtime's compiled local-module fallback; a
  package WebUI must use its verified Control-served bundle URL.

## 4.0.0 - 2026-07-20

### Added

- Published the formal plugin-only Control release with the complete signed
  sixteen-package cohort, V2 package manifests, evidence verification, and
  release-bound canary and support approvals.
- Added a root-pinned cold-start bootstrap path for the signed
  `identity-platform` package, so a new package-only database can establish
  authenticated Control routes without an unauthenticated direct HTTP fallback.
- Added live signed-package browser coverage and cross-repository Agent package
  host coverage for package registration, lifecycle, WebUI delivery, bridge
  routing, and telemetry retrieval.

### Fixed

- Made SQLite package-artifact writes tolerate transient reader/writer
  contention and use WAL mode for file-backed databases.
- Bound Control-host bridge route IDs to safe deterministic identifiers while
  preserving the original validated request metadata for package handlers.
- Made the live signed-WebUI CI gate build the pinned Agent source for both
  supported Linux architectures, and scoped the full-repository security scan
  around its deliberately malformed static-analysis fixture.
- Replaced the transient third-party `protoc` setup Action with a checksum-
  verified official `protoc` 29.2 archive in the Go quality gate.
- Made release binaries compile across Linux, macOS, and Windows without
  weakening package-host isolation: unsupported non-Unix hosts now fail closed
  instead of falling back from the required Unix descriptor protections, while
  non-Linux Unix hosts apply an explicit checked close-on-exec descriptor flag.

## 4.0.0-alpha.7 - 2026-07-18

### Fixed

- Hardened G115 conversion boundaries for topology and plugin observations:
  Agent-reported revisions must fit the signed database range, and non-positive
  terminal operation revisions are rejected before promotion comparisons.

## 4.0.0-alpha.6 - 2026-07-18

### Added

- Added the signed `nftables-forward` 1.2.0 package contract and WebUI runtime
  status fields for live ruleset SHA-256, per-rule packets, bytes, and
  unhealthy/reconciling state.
- Added a bounded `PluginObservedState` heartbeat contract. The Agent
  Supervisor accepts only private observation files from enabled official
  packages declaring `kernel.observed-state`, then injects its own version,
  config hash, and revisions.
- Added Control persistence for authorized runtime observations with exact
  signed-release capability checks, timestamp bounds, monotonic ordering, and
  bounded/de-duplicated rule counters.
- Added topology health gates that bind terminal Agent state and runtime
  evidence to exact version, config hash, revision, health, and nftables rule
  IDs. Missing evidence gets a durable 90-second wait window; mismatches fail
  closed and rollback uses the same terminal-state checks.
- The Supervisor now checks the runtime health socket on every heartbeat and
  emits a Supervisor-owned unhealthy observation instead of retaining stale
  plugin evidence. Control treats observations older than two minutes as
  degraded in the nftables package WebUI.

### Fixed

- Stopped raw Agent operation JSON, runtime errors, and unknown plugin fields
  from being copied into public topology observed-state responses. Public
  `/api/v3/operations` responses now use a fixed non-secret projection.
- Made the Agent client preserve an unhealthy observation without a live
  fingerprint while rejecting invalid non-healthy counter payloads.
- Isolated malformed heartbeat observation entries so a bad or unauthorized
  plugin record cannot suppress later valid records, and made rollback validate
  the restored nftables configuration rather than the replacement config.

### Known Gaps

- This remains an isolated alpha canary. Production traffic takeover still
  requires staging restore evidence, a legacy fallback rehearsal, rollout
  records, and a successful 72-hour canary before explicit stable-release
  authorization.

## 4.0.0-alpha.5 - 2026-07-17

### Added

- Added read-only topology deployment preview and diagnosis with structured
  DAG, rollout dependency, node assignment, signed Agent release, artifact,
  dependency, config-schema, nftables runtime, port, address-family, MTU, and
  secret-reference checks. Deployment status now includes safe operation
  linkage and an event timeline without returning config or result payloads.
- Added the admin topology workflow for topology creation, immutable revision
  editing, server-side diagnosis, config-hash preview, canary rollout-group
  planning, apply/status polling, and guarded rollback. Unsaved revisions are
  fenced from preview and deployment actions.
- Added the signed `nftables-forward` 1.1.0 package contract with one canonical
  Control/Agent configuration schema, safe observation-only defaults, and
  `plugin.runtime-state` plus `plugin.cleanup` capabilities. The 1.0.0 Control
  executor remains registered for rollback compatibility.

### Fixed

- Replaced the incompatible 1.0 package configuration fields with the exact
  Agent runtime contract and added fail-closed Control semantic admission, so
  invalid nftables plans are rejected before dispatch.
- Pinned official package builds and cross-repository tests to the published
  Agent `v4.0.0-alpha.5` commit `72bbdff19f03768fd8bd9c720e236f93e898043f`, which journals the original
  nftables table, recovers after `SIGKILL` or Agent restart, and restores or
  removes owned state through the signed cleanup entrypoint.
- Cleared the deployment preflight static-analysis gate without changing
  runtime behavior.
- Fixed the frontend API proxy target for Control instances bound to a specific
  IPv4 or IPv6 address, while retaining loopback proxying for wildcard binds.
- Moved the required Agent gRPC bind ahead of plugin workers and HTTP listeners
  so an occupied node-control port fails startup before a partial Control
  instance becomes reachable.

### Known Gaps

- Topology execution remains disabled by default and this release is limited
  to isolated canary use. Live staging restore evidence, legacy fallback
  rehearsal, kernel-observed ruleset health, rollout records, and a fresh
  72-hour canary are still required before production traffic cutover or a
  stable 4.0 release.

## 4.0.0-alpha.4 - 2026-07-17

### Added

- Added the signed `machine-telemetry` 1.1.0 reference package end to end:
  Agent-local Unix RPC snapshots, gopsutil-backed CPU/memory/disk/network/
  process/uptime metrics, namespaced heartbeat transport, Control persistence,
  read-only API state, and the matching WebUI status view.
- Added signed-release capability admission for telemetry. Control accepts
  metrics only from an enabled assignment whose exact Agent release verifies
  against an active AnixOps trust root and declares `telemetry.read`.
- Added canary-aware topology planning, dependency checks, scoped rollback,
  and pure-removal revisions, including protection of the active revision until
  a full rollout completes.

### Fixed

- Added lifecycle admission fencing for the Agent Supervisor so close waits for
  active operations, rejects new work while closing, and can be retried after a
  deadline without losing cleanup state.
- Propagated operation deadlines and cancellation through queue, handler, and
  plugin locks; canonicalized Agent timeout observations and mapped them to
  Control `timed_out` state.
- Added cross-repository process E2E coverage from the signed Agent package
  through telemetry persistence and the Control plugin API/WebUI executor,
  pinned to Agent commit `676b5ad1c339a157075ae46028eee06540ffc26b`.
- Corrected the attached upgrade runbook to use the actual `anix-control-*`
  release assets and default systemd layout, and attached the exact signature
  verifier used by the Machine Telemetry package instructions.
- Cleared the blocking static-analysis gate for telemetry error aggregation and
  topology observed-state test setup without changing runtime behavior.

### Known Gaps

- This remains an opt-in alpha. Production traffic stays on the legacy path;
  topology data-plane cutover, Secret-ID materialization, live staging evidence,
  and the 72-hour canary are still required before stable 4.0 authorization.

## 4.0.0-alpha.3 - 2026-07-17

### Added

- Added explicit `permission_mode`, plugin permission, and restricted-plugin
  metadata to login, registration, and profile responses so the WebUI can
  enforce the same actor-scoped authorization contract as Control.

### Fixed

- Filtered the signed WebUI catalog, routes, menus, permissions, and bundle
  assets by the authenticated actor's `plugin_api` grants, with per-plugin
  legacy-admin compatibility and fail-closed regular-user behavior.
- Rejected unauthorized plugin routes before importing their modules, ensuring
  an actor with no matching route permission performs zero bundle fetches.
- Served plugin assets only for the active, enabled, version-matched verified
  installation and marked responses `private, no-store`, so disable, update,
  rollback, or permission revocation takes effect without a stale bundle cache.
- Quarantined an invalid signed WebUI extension at the plugin boundary so one
  malformed or tampered release cannot prevent valid authorized extensions or
  kernel pages from loading.
- Preserved the `/api/v2/user/profile` compatibility path when legacy tooling
  exercises a database before the optional Kernel tables are migrated: regular
  users receive empty authoritative plugin permissions and legacy admins keep
  pre-Kernel behavior, while real database failures still fail closed.
- Pinned cross-repository package and Agent process evidence to Agent commit
  `a8e6331e4c81274460e409720f8680649b7c2d17`, the matching alpha.3 source.

## 4.0.0-alpha.2 - 2026-07-17

### Fixed

- Retried the complete payment callback transaction on bounded SQLite
  writer-contention errors, preserving atomic payment-record, order, and
  gateway-stat updates without changing PostgreSQL or semantic error behavior.
- Tracked asynchronous notification work and drained it before shared SQLite
  test cleanup so notification logging cannot cross test boundaries and lock a
  later payment transaction.
- Required the real `nftables-forward` Agent binary to report plugin version
  `1.0.0` before package-contract or signed-release builds can proceed.
- Pinned package builds and cross-repository process gates to Agent hotfix
  commit `555be48faebf80e6c9d61cea10705583cf7c32f1`.
- Blocked release tags whose version does not match the Control runtime,
  frontend package/lockfile, configuration templates, generated Swagger,
  changelog, and current-preview documentation.
- Required both installer configuration templates to expose the exact release
  version, with negative tests for mismatched or format-escaped version fields.

## 4.0.0-alpha.1 - 2026-07-17

### Added

- Added version-bound Control status executors for the signed
  `nftables-forward` and `nat-egress` WebUI packages, so real installed bundles
  resolve assignment, operation, revision, health, cleanup, and rollback state
  instead of failing with an unimplemented plugin route.
- Added an operational official-package center for release manifest/artifact
  import, installation intent, enable/disable/update/rollback actions,
  revisioned Schema or JSON configuration, operation polling/cancellation, and
  signed WebUI extension refresh with visible terminal errors.
- Added authenticated node-facing official package downloads and assignment
  reconciliation. Enabled Agent assignments now produce a durable,
  revision-ordered `install -> update(config) -> enable` chain, aggregate
  multiple roles fail-closed, and replay safely after Control reconnects or
  restarts without exposing package bytes to unassigned nodes.
- Added the node-plugin assignment matrix to the admin WebUI, including
  role-aware configuration, enable/disable/delete lifecycle actions, and
  operation/observed-state refresh for canary rollout.
- Added a real cross-repository signed Agent package process gate covering
  package download authorization, install/update/enable, immutable bundle
  files, socket health, disable cleanup, and terminal operation replay.
- Migrated the Control Go module to `github.com/AnixOps/anix-control/v4` while
  preserving the `/api/v2`, `v2_*`, and `v2board` compatibility namespaces.

### Fixed

- Fixed the nftables forward status topology formatter so the production lint
  gate accepts its whitespace and slash trimming logic.
- Hardened authenticated Agent node ID conversion and updated the signed-package
  process fixture to use current gRPC APIs with checked resource cleanup.
- Pinned package builds and cross-repository process gates to the verified Agent
  v4 commit `882024acfb1f125becec8138c3ade0173072ef71`.
- Fixed release-note generation to select the immutable changelog section for
  the current tag, including dated headings, instead of requiring an
  `Unreleased` section that no longer exists in a prepared release commit.

## 3.1.0-alpha.1 - 2026-07-17

### Added

- Added the authoritative 3.1-to-4.0 plugin-platform roadmap, defining signed
  service/WebUI packages, microkernel ownership, reproducible release gates,
  the 3.5 business-plugin migration, and the plugin-only 4.0 cutover criteria.
- Added signed WebUI extension metadata and catalog validation, dynamic
  namespaced admin route/menu registration with local-module identity checks,
  revisioned installation configuration guarded by release JSON Schema, and an
  opt-in durable Control-to-Agent lifecycle dispatcher with ACK/observed-state
  persistence.
- Added byte-identical Control/Agent manifest golden fixtures, Agent-side WebUI
  manifest decoding, Control-side dependency/conflict installation checks, and
  a bundled `machine-telemetry` reference WebUI module.
- Added signed `control_routes` validation and a fail-closed `/api/v3/plugins`
  backend gateway with verified installation checks, `plugin_api` access-group
  grants, and the opt-in version-exact `machine-telemetry` Control executor.
- Added the executable major-upgrade program with 3.1 promotion blockers,
  reproducible Control/Agent/package/browser/PostgreSQL gates, canary rules, and
  3.2-to-4.0 rollback boundaries.
- Added a repeatable destructive PostgreSQL restore rehearsal with schema/row
  evidence and a blocking PostgreSQL 16 CI job.
- Added a real cross-repository Control database/KernelOperationBridge to Agent
  process E2E gate, plus the Agent-side Supervisor and signed
  `machine-telemetry` process E2E.
- Added Chromium Playwright WebUI failure-isolation coverage and a CI browser
  gate for same-origin digest checks, route collision rejection, disabled
  plugins, and tampered bundles.
- Added deterministic `machine-telemetry` package release-contract checks with
  ephemeral Ed25519 signing, public-key-only verification, artifact binding,
  and tamper rejection; no private key is stored or uploaded.
- Added dependency-aware Control lifecycle plans with dependency-first
  execution, cancellation/restart recovery, reverse rollback, and focused
  durability coverage.
- Added feature-gated topology deployment fan-out with durable per-node steps,
  observed-state reconciliation, failure fencing, and reverse rollback. The
  executor remains disabled by default and requires Agent dispatch before it can
  run.
- Added production release-tag signing and upload wiring for the official
  `machine-telemetry` package, requiring the
  `ANIXOPS_PLUGIN_SIGNING_PRIVATE_KEY` GitHub secret and publishing package,
  manifest, signature, public-key, and checksum evidence.
- Added the `nftables-forward` reference package source with deterministic
  package builds, dependency-free WebUI smoke coverage, public-key-only
  signature verification, tamper rejection, and CI release-contract evidence.
- Added privileged Agent-side `nftables-forward` namespace acceptance proving
  TCP DNAT, UDP DNAT, plugin-created table rollback, and pre-existing nftables
  table snapshot restoration before production data-plane canary.
- Added release-tag signing and artifact verification wiring for the official
  `nftables-forward` package alongside `machine-telemetry`, pinning the Agent
  runtime source and publishing package, manifest, signature, public-key, and
  checksum evidence.
- Added 3.3 `gost-mesh` and `nat-egress` reference package sources with
  deterministic package builds, dependency-free WebUI modules, namespaced
  status routes, public-key-only signature verification, tamper rejection, and
  CI release-contract evidence.
- Added the real `nat-egress` Agent runtime with nftables masquerade, fwmark
  policy routing, interface-bound health probes, strict configuration and
  ownership checks, crash-safe state journaling, and privileged namespace
  acceptance for marked forwarding, wrong-mark isolation, NAT, and rollback.
  Production release signing, artifact verification, and the immutable Agent
  revision pin are wired and locally gate-verified.
- Added the real `gost-mesh` Agent runtime and signed auxiliary-runtime
  packaging for checksum-pinned GOST v3.2.6. The aggregate `tunnels[]` runtime
  supports QUIC and WSS with mandatory mutual TLS, source-policy routing,
  source-bound health probes, bounded restart, ownership journaling, and
  crash-safe cleanup. Privileged namespace acceptance proves TCP and UDP data
  paths, transport selection, wrong-SNI and untrusted-client rejection, health,
  and cleanup. TUIC is not advertised because the pinned GOST runtime does not
  implement it. Release signing and upload now require the real Agent binary
  and the archive- and binary-digest-pinned GOST executable. The package remains
  canary-only until Control Secret ID materialization and sustained rollout
  evidence are complete.
- Added Supervisor `plugin.runtime-state` and `plugin.cleanup` lifecycle
  contracts. Cleanup intent and version are persisted as `cleanup_pending`;
  failed target-version cleanup leaves the plugin disabled and blocks old-
  version restart, while automatic rollback starts the old version only after
  cleanup succeeds.
- Introduced the AnixOps Control / AnixOps Agent product identity, primary `anix-control` binaries, frontend archives and Docker images, stable/alpha/beta/RC release tag support, and a documented compatibility window with legacy `v2board-*` release aliases.
- Added a tag-pinned native release installer that downloads and verifies GitHub Actions-built panel/frontend assets without cloning or building on the target host, preserves configuration/data, and restores the previous application snapshot after a failed health check.
- Added detailed release installation and legacy migration guides covering fresh install, update, rollback, SQLite-to-PostgreSQL boundaries, foreign-panel migration limits, coordinated node rollout, and retained evidence.
- Added the first P0 WireGuard panel slice: `wireguard` node protocol template, stored per-user peer keypair/PSK/IP custody, native WireGuard `.conf` subscription output, sing-box 1.13-compatible WireGuard endpoint output, and targeted unit coverage.
- Added WireGuard UniProxy/gRPC runtime user fields so V2bX can receive panel-managed peer IP, public key, and preshared key for domestic entry termination.
- Updated P0 WireGuard status docs for the V2bX v2.3.3 runtime slice, which adds initial WireGuard peer online-state reporting from recent `wg show <iface> dump` handshakes.
- Extended the WireGuard relay runtime contract with entry/exit GOST TUN fields for V2bX entry policy routing and overseas exit NAT command application.
- Added a first admin WireGuard visual protocol form for CIDR, server keys, MTU, DNS, entry/exit GOST relay role, QUIC/WSS tunnel selection, one-click WSS compatibility mode, TUN addresses, routing table/priority, and exit NAT hints.
- Added deterministic `RELEASE_NOTES.md` generation to GitHub Release assets, with a CI self-test, release workflow policy guard, and artifact verification requirement.
- Added opt-in local deploy archive cleanup for stale ignored frontend/internal zip or tarball leftovers while keeping release builds GitHub Actions-only and preserving database backups.
- Attached `UPGRADE.md` to GitHub Release assets and guarded it with release workflow policy and artifact verification checks so tag releases include upgrade and rollback instructions.
- Added `docs/UPGRADE.md` with the GitHub Actions artifact verification, systemd/Docker upgrade, database migration, rollback, and post-upgrade evidence runbook.
- Added a root `README.md` that links the status registers, audit docs, deployment docs, local checks, compatibility surfaces, and GitHub Actions-only release policy.
- Added a safe local build artifact cleanup helper with CI self-tests so stale source-tree outputs can be removed without touching config, database, certificates, backups, or `web/node_modules`.
- Extended local cleanup coverage to frontend verification outputs including `web/public-check`, `web/coverage`, and `web/bundle-reports-check`, keeping local test/build artifacts out of the source tree.
- Added `docs/features.md` as the current feature status register for implemented, partial, planned, deferred, and compatibility surfaces, with an update rule for every feature-status-changing commit.

### Fixed

- Kept GOST Mesh status and IPv4 broadcast validation compatible with the
  repository's static-analysis and integer-safety release gates without
  changing the accepted configuration contract.
- Made `make run` create and use an isolated development configuration with
  loopback ports `19080` (API), `19000` (frontend), and `50052` (gRPC), avoiding
  collisions with an existing system `anix-control` service and its database.
- Made gRPC server lifecycle tests bind ephemeral loopback ports so `make run`
  can stay active while the full test suite runs.
- Made the PostgreSQL large-traffic regression derive its dashboard expectation
  from the local-day boundary, avoiding a false failure during the midnight
  hour without changing production aggregation semantics.
- Stabilized plugin runtime test gates by observing pre-handler gRPC stream
  rejection through the authoritative receive status, and by restoring Vitest
  mocked globals and timers after every frontend test.
- Silenced misleading frontend test-run stderr by mocking AdminLayout profile
  refreshes in its unit tests and escaping JSON examples in locale messages so
  vue-i18n no longer reports placeholder compilation errors during successful
  runs.
- Serialized Control package operations per installation, added target-level
  dependency/conflict transaction locks and lifecycle-generation idempotency,
  and made lease loss cancel the executor while automatic rollback remains
  bounded and lease-supervised.

- Enforced global MFA `enforce_for_all` and `enforce_for_admin` login enrollment policies by returning no-token enrollment-required responses for covered users who have not enabled MFA, with handler coverage for all-users, admin-only, and regular-user bypass prevention paths plus a Login page enrollment-required prompt.
- Enforced user-enabled TOTP/backup MFA during login before issuing JWTs, counting invalid MFA codes in the login rate limiter and adding backend handler coverage plus a frontend two-step MFA challenge flow.
- Blocked Alipay, WeChat, and USDT payment gateways from being enabled or used for new payment records until their live callback or confirmation implementations and tests exist, including historical enabled-row filtering.
- Made notification async sends return a completion signal and updated the user-ID copy regression test to wait for the send path, removing a timing race with shared service-test database cleanup.
- Fixed the integration workflow coverage upload by generating `coverage.out` during integration unit tests before uploading the artifact.
- Refreshed GitHub Actions workflow dependencies to current action major versions so CI/release jobs no longer depend on Node.js 20 action runtimes.
- Fixed the GitHub Actions CI baseline by pinning Go setup to `1.26.5` for stdlib vulnerability scanning and adding focused gRPC NodeLog/server-context tests so the gRPC coverage gate remains above 80%.
- Added a CI release-build policy check that fails deployment scripts containing unguarded local `go build` or frontend build commands, including the V2bX Ansible rollout helper.
- Guarded the legacy `config/scripts/deploy.sh` and `config/scripts/pre-deploy.sh` entrypoints behind `ALLOW_LOCAL_BUILD=1` and added CI self-tests so old local deploy paths cannot build by default.
- Guarded the local source-tree deploy script behind `ALLOW_LOCAL_BUILD=1`, documented GitHub Actions as the only release build source, and updated the generated release runbook to deploy release artifacts instead of building on the host.
- Upgraded vulnerable Go dependencies reported by `govulncheck`: `google.golang.org/grpc` to `v1.79.3`, `github.com/jackc/pgx/v5` to `v5.9.2`, and `github.com/quic-go/quic-go` to `v0.59.1`.
- Cleared production runtime `gosec` findings by range-checking sing-box integer conversion, tightening generated runtime file/directory permissions, and documenting reviewed config-path and subscriber credential JSON outputs.
- Replaced the weak default bootstrap admin password with a generated `crypto/rand` password when `admin.password` is empty.
- Replaced load balancer random and weighted node selection with `crypto/rand` and explicit random-source error handling.
- Hardened `cmd/verify` local E2E verification by requiring `V2BOARD_VERIFY_TOKEN`, restricting panel URLs to loopback hosts, allowlisting local Xray binary names, and writing temporary Xray configs with private permissions.
- Hardened `cmd/subtest` subscription test tooling by validating local YAML config paths and writing generated sample configs with private permissions.
- Hardened `cmd/configgen` by checking generated config write errors and writing integration client configs with private permissions.
- Hardened `cmd/report` by checking report generation and E2E cleanup errors, writing reports/configs with private permissions, validating local Xray binary names, and adding a local echo server header timeout.
- Hardened the integration mock server by checking JSON response encoding failures and adding HTTP header read timeouts.
- Hardened integration echo helpers by checking close/write/deadline/copy errors and documenting the intentional local echo response behavior.
- Hardened the integration local environment by checking process/probe cleanup errors, restricting reviewed local server binary names, and writing generated configs with private permissions.
- Hardened the shared test database helper by returning close failures through `CloseWithError` while keeping the legacy close wrapper observable.
- Hardened the integration runner by reporting config-directory setup failures, using private generated-file permissions, and logging generated-config cleanup failures.
- Hardened the integration binary manager by checking download/cache cleanup errors, bounding archive extraction, using private cache permissions, and scoping local file access through `os.Root`.
- Hardened the integration clients by checking shutdown/probe cleanup errors, allowlisting local client binaries before subprocess launch, and scoping manual binary lookup through `os.Root`.
- Hardened the migration dump parser by opening operator-provided dump files through `os.Root` and adding plain/gzip dump parser coverage.
- Hardened `cmd/sqlite2postgres` by logging database close failures, returning row-close failures, and covering table copy behavior.
- Hardened `cmd/integration-test` JSON-output tests by checking pipe close/copy errors and clearing package-local lint findings.
- Hardened `cmd/report` tests by checking listener/response cleanup and validating the echo server's assigned-port ping path.
- Hardened integration echo server tests by checking response/server cleanup errors and clearing package-local lint findings.
- Hardened integration local environment tests by checking echo shutdown, subprocess cleanup, and HTTP response close errors while clearing package-local lint findings.
- Hardened integration mock server tests by checking response close, JSON decode, server shutdown, and JSON fixture encoding errors while clearing package-local lint findings.
- Hardened integration binary manager tests and download URL selection by checking mock response writes, removing ineffectual assignments, and clearing package-local lint findings.
- Hardened the integration runner by propagating client stop failures into test results and preserving result duration updates while clearing package-local lint findings.
- Cleared `cmd/verify` lint findings by normalizing local Xray lookup error text.
- Cleared WebSocket Origin utility lint findings by normalizing scheme mapping through a tagged switch.
- Hardened database package tests by checking database close paths, isolating SQLite path tests under temporary directories, and clearing package-local lint findings.
- Hardened the integration setup script by logging database close failures and clearing package-local lint findings.
- Hardened middleware tests by checking node table migration and database cleanup errors while clearing package-local lint findings.
- Hardened integration client tests and HTTP probe cleanup by checking manager registration errors and logging deferred response body close failures while clearing package-local lint findings.
- Hardened command entrypoint shutdown paths by logging `cmd/migrate` and `cmd/server` database close failures while clearing package-local lint findings.
- Hardened router, smoke, and root integration tests by checking schema migration, database cleanup, response close, and port parsing failures while clearing package-local lint findings.
- Hardened websocket tests by checking client close failures and removed an unused client mutex while clearing package-local lint findings.
- Hardened GOST client and manager tests by returning response close failures, checking mock response writes, and removing an unused manager mutex while clearing package-local lint findings.
- Hardened cache tests by checking cache mutation/read errors and correcting concurrent access coverage to read typed values while clearing package-local lint findings.
- Hardened gRPC tests by checking database lifecycle, server serve, client connection close, and stream close errors while clearing package-local lint findings.
- Hardened E2E tests by checking JSON response decoding, database cleanup, and subscription group association errors while clearing package-local lint findings.
- Hardened integration E2E tests by checking echo shutdown, port probe, process signal/kill, and HTTP response body close errors while clearing package-local lint findings.
- Hardened handler package cleanup and tests by checking WebSocket, request/response body, Telegram update, database setup, and JSON decoding errors while clearing package-local lint findings.
- Hardened service small-file cleanup by checking migrations, benchmark setup, response/archive close paths, removing stale private helpers and fields, and clearing service staticcheck/unused findings outside the consolidated `service_test.go` suite.
- Hardened the consolidated service test suite by checking fixture creation, registration, order setup, auth-key generation, and cleanup errors, clearing the final Go lint baseline.
- Hardened forward node health checks, tag parsing, API token generation, and random node selection with explicit error handling and context-aware dialing.
- Enabled SMTP certificate verification for notification email delivery and made asynchronous notification send failures observable.
- Returned explicit MFA backup-code JSON parse errors instead of silently treating corrupt data as no remaining backup codes.
- Made dashboard and user subscription statistics cache writes/deletes explicit, with refresh paths returning cache failures and read paths logging non-fatal cache write errors.
- Made legacy server node config generation reject malformed JSON settings and return online-status cache update failures instead of silently producing partial state.
- Made Telegram broadcast, API request encoding, and admin ID parsing errors explicit instead of silently treating failures as success or empty configuration.
- Hardened local backup creation and restore with private backup-directory permissions, zip path traversal checks, non-regular entry rejection, and archive decompression size limits.
- Restricted local forward runtime command execution to `ansible-playbook` or absolute `ansible-playbook` paths before spawning runtime jobs.
- Replaced subscription template node ID MD5 hashing with SHA-256 and documented the remaining SS2022 MD5 path as a reviewed legacy compatibility requirement.
- Made WebSocket response and monitor streams handle JSON, deadline, write, close, and backpressure failures explicitly; also replaced UniProxy ETag MD5 hashing with SHA-256.
- Restricted WebSocket browser origins through a shared same-host/CORS allowlist policy and added agent WebSocket read/write deadlines.
- Cleared production-package `G104` findings outside `internal/service` by checking model JSON decode failures, Gost delete/update errors, server startup writes, and verification command I/O/process cleanup.
- Added range-checked gRPC protobuf integer conversions for node IDs, ports, TLS flags, and user device limits, and made traffic/online heartbeat update failures observable.
- Added gRPC bidirectional stream cancellation coverage, including online stream cancellation and status-stream connection cleanup.
- Added background worker cancellation/drain coverage for shared delayed cycles, the Gost stats idle loop, and runtime executor claimed-job cleanup.
- Added clean-agent bridge worker cancellation and retry coverage, including canceled NodeX translation draining to a failed runtime job and retry upsert of an existing bridge mapping.
- Made memory cache init/close lifecycle idempotent by closing and waiting for stale cleanup goroutines before replacing or restarting the cache.
- Isolated the shared service test SQLite database under a per-process temporary directory and close it before cleanup.
- Fixed node update cache invalidation to delete `node:<id>` keys using decimal node IDs instead of rune conversion.
- Added compatibility for the legacy `/api/v1/client/subscribe?token=` subscription endpoint.
- Added service and admin HTTP coverage for oversized traffic ranking requests, verifying the 200/1000 row caps used by `/admin/traffic/user-ranking`.
- Hardened EPay callbacks with constant-time signature comparison, signed amount parsing, and order amount verification before marking payments paid.
- Added a registry guard test so every plugin payment callback gateway must provide valid and tampered-signature coverage.
- Added mocked PayPal webhook verification tests covering remote signature success and rejection without external network calls.
- Made panel forward pause/resume idempotent while runtime jobs are pending or running, preventing duplicate runtime job enqueueing and stale status overwrites from repeated or bulk actions.
- Added a partial unique database guard for pending/running forward runtime jobs and a schema repair step that collapses historical duplicates before creating the guard.
- Filtered bulk forward pause/resume actions in the admin UI so only forwards that actually need the requested state change are submitted.
- Normalized Ansible Machines admin responses to the panel `code/msg/ts/data` envelope and added handler coverage for success and error responses.
- Normalized Forward Node management responses to the panel `code/msg/ts/data` envelope and expanded handler coverage for invalid ID, missing body, not-found, scope rejection, toggle, and sync-stats paths.
- Normalized Forward Rule management responses to the panel `code/msg/ts/data` envelope and expanded handler coverage for list, create, get, update, delete, toggle, missing body, invalid ID, and not-found paths.
- Normalized Forward stats, user-rule, and connection-test responses to the panel `code/msg/ts/data` envelope while preserving connection-test diagnostics under `data.success=false`.
- Expanded Forward observability response-envelope coverage for targets, trend, topology, and multi-ingress endpoints, with frontend API mapping tests for all observability calls.
- Expanded Forward internal traffic report/snapshot handler coverage for panel `code/msg/ts/data` success, binding-error, and service-error responses.
- Normalized admin traffic hourly and user-ranking responses to the panel `code/msg/ts/data` envelope while keeping the TrafficHourly page compatible with legacy and enveloped payloads.
- Normalized the admin dashboard success/database-error response to the panel `code/msg/ts/data` envelope while keeping the Dashboard page compatible with legacy and enveloped payloads.
- Normalized admin user stats success/database-error responses to the panel `code/msg/ts/data` envelope while keeping the Users page compatible with legacy and enveloped payloads.
- Normalized admin order stats success/database-error responses to the panel `code/msg/ts/data` envelope while keeping the Orders page compatible with legacy and enveloped payloads.
- Stabilized async notification service coverage by waiting for the final send status before asserting copied user IDs.
- Normalized admin node stats responses to the panel `code/msg/ts/data` envelope while keeping the Nodes page compatible with legacy and enveloped payloads.
- Normalized admin system info responses to the panel `code/msg/ts/data` envelope while keeping the AdminLayout version display compatible with legacy and enveloped payloads.
- Normalized admin invite stats responses to the panel `code/msg/ts/data` envelope while keeping the Invite page compatible with legacy and enveloped payloads.
- Normalized admin payment stats responses to the panel `code/msg/ts/data` envelope while keeping the Payment page compatible with legacy and enveloped payloads.
- Normalized admin subscription stats responses to the panel `code/msg/ts/data` envelope while keeping the Subscriptions page compatible with legacy and enveloped payloads.
- Normalized admin system backup stats success/user-error responses to the panel `code/msg/ts/data` envelope while keeping the System backup view compatible with legacy, enveloped, and `code=-1` payloads.
- Normalized admin load balancer stats success/user-error responses to the panel `code/msg/ts/data` envelope and added frontend API mapping coverage for the stats route.
- Normalized admin system backup config success/user-error responses to the panel `code/msg/ts/data` envelope while keeping sensitive-field masking and the System backup view compatible with legacy, enveloped, and `code=-1` payloads.
- Normalized admin system backup list/create/delete/restore success/user-error responses to the panel `code/msg/ts/data` envelope while keeping the System backup list compatible with legacy, enveloped, and `code=-1` payloads.
- Normalized admin invite config responses to the panel `code/msg/ts/data` envelope while keeping the Invite page compatible with legacy and enveloped payloads.
- Normalized user invite info/code/commission/withdrawal responses and admin invite config update/withdrawal responses to the panel `code/msg/ts/data` envelope while keeping the Invite page compatible with legacy, enveloped, and nested payloads.
- Normalized admin subscription settings responses to the panel `code/msg/ts/data` envelope while keeping System and Users subscription-link flows compatible with legacy and enveloped payloads.
- Normalized admin payment gateway list responses to the panel `code/msg/ts/data` envelope while keeping the Payment page compatible with legacy and enveloped payloads.
- Normalized admin payment gateway create/update/delete/toggle and payment-record list success/user-error responses to the panel `code/msg/ts/data` envelope while keeping the Payment page compatible with legacy, enveloped, and `code=-1` payloads.
- Normalized user payment channels/create/status/records success/user-error responses to the panel `code/msg/ts/data` envelope while preserving payment callback plain-text compatibility.
- Updated Auth and admin traffic E2E tests to assert the panel response envelope after login/register and traffic stats response normalization.
- Normalized legacy X402 and fiat payment create/check success and user-error responses to the panel `code/msg/ts/data` envelope while preserving X402, Stripe, and PayPal callback/webhook compatibility responses.
- Normalized user/admin MFA success and user-error responses to the panel `code/msg/ts/data` envelope while keeping the Admin MFA page compatible with legacy, enveloped, and error config payloads.
- Normalized user/admin notification success and user-error responses to the panel `code/msg/ts/data` envelope while keeping the Admin Notifications page compatible with legacy, enveloped, and `code=-1` templates, logs, email config, and mutation payloads.
- Normalized admin/user Telegram panel API success and user-error responses to the panel `code/msg/ts/data` envelope while preserving the public Telegram webhook `status=ok` compatibility response and keeping the Admin Telegram page compatible with legacy, enveloped, and `code=-1` payloads.
- Normalized admin subscription group CRUD success/user-error responses to the panel `code/msg/ts/data` envelope, made missing group deletes return a user error, and moved group delete cleanup into a transaction while keeping the Subscriptions page compatible with legacy, enveloped, and `code=-1` group payloads; template, protocol, and preview success payloads remain covered separately.
- Normalized admin subscription template CRUD success/user-error responses to the panel `code/msg/ts/data` envelope, made missing template update/delete operations return user errors, and kept the Subscriptions page compatible with legacy, enveloped, and `code=-1` template payloads.
- Normalized admin subscription protocol binding/read success-user-error responses to the panel `code/msg/ts/data` envelope, made missing protocol IDs fail instead of being silently ignored, and kept the Subscriptions page compatible with legacy, enveloped, and `code=-1` protocol payloads.
- Normalized admin subscription preview success/user-error responses to the panel `code/msg/ts/data` envelope, accepted the frontend `group_ids` request alias, and allowed preview to use the authenticated admin context when `user_id` is omitted.
- Normalized admin subscription user/plan group binding success-user-error responses to the panel `code/msg/ts/data` envelope, made missing users/plans/groups and missing relations fail explicitly, and kept Plans subscription-group flows compatible with enveloped errors.
- Normalized admin node management CRUD, protocol, log, raw-config, and auth-key success responses to the panel `code/msg/ts/data` envelope while keeping the Nodes page compatible with legacy and enveloped payloads.
- Normalized admin Agent list, task result, task history, monitor read, task creation, and execute-command success responses to the panel `code/msg/ts/data` envelope while keeping the Agent page compatible with legacy, enveloped, and nested payloads.
- Normalized admin user management CRUD, ban/unban, traffic reset, and subscribe-reset success and user-error responses to the panel `code/msg/ts/data` envelope, made missing user mutations fail explicitly instead of silently updating zero rows, and kept the Users page compatible with legacy, enveloped, nested, and `code=-1` mutation payloads.
- Normalized admin plan management list/detail/create/update/delete/assign success and user-error responses to the panel `code/msg/ts/data` envelope, made missing plan/user operations fail explicitly instead of silently updating or inserting, and kept the Plans page compatible with legacy, enveloped, nested, and `code=-1` payloads.
- Normalized admin order management list/detail/status/paid/cancel success and user-error responses to the panel `code/msg/ts/data` envelope, made missing orders and missing related plans fail explicitly instead of silently updating zero rows, and kept the Orders page compatible with legacy, enveloped, nested, localized, and `code=-1` payloads.
- Normalized user registration success/error and order-save success/error responses to the panel `code/msg/ts/data` envelope while preserving the existing token and order payloads under `data`.
- Normalized user login success/error responses to the panel `code/msg/ts/data` envelope while keeping the Login page compatible with enveloped token and error payloads.
- Normalized user order list and detail success/error responses to the panel `code/msg/ts/data` envelope while keeping the Orders page compatible with legacy and enveloped payloads.
- Normalized user subscription info success/error responses to the panel `code/msg/ts/data` envelope while keeping the Subscribe page compatible with legacy and enveloped payloads.
- Normalized user profile success/error responses to the panel `code/msg/ts/data` envelope while keeping the user store compatible with the enveloped profile payload.
- Normalized user dashboard success/error responses to the panel `code/msg/ts/data` envelope while preserving the subscription payload under `data.subscription`.
- Normalized user plan list success/database-error responses to the panel `code/msg/ts/data` envelope while keeping the Plans page compatible with legacy, enveloped, and `code=-1` payloads.
- Normalized user coupon-check success and business-error responses to the panel `code/msg/ts/data` envelope while keeping the Plans page compatible with legacy and enveloped coupon payloads.
- Normalized user knowledge list and detail success/error responses to the panel `code/msg/ts/data` envelope while keeping the Knowledge page compatible with legacy and enveloped article lists.
- Normalized user ticket list, create, detail, reply, and close success/error responses to the panel `code/msg/ts/data` envelope while keeping the Tickets page compatible with legacy and enveloped ticket payloads.
- Normalized public payment methods and payment-status responses to the panel `code/msg/ts/data` envelope with handler coverage for key payload fields and missing payment records.
- Normalized admin ticket list, reply, and close success/error responses to the panel `code/msg/ts/data` envelope while keeping the Tickets admin page compatible with legacy and enveloped payloads.
- Normalized admin coupon list, create, and delete success/error responses to the panel `code/msg/ts/data` envelope while keeping the Coupons admin page compatible with legacy and enveloped payloads.
- Normalized admin knowledge list, create, update, and delete success/error responses to the panel `code/msg/ts/data` envelope, preserved partial-update sort/visibility fields when omitted, and restored missing Knowledge admin page locale strings.
- Normalized admin system audit-log success and database-error responses to the panel `code/msg/ts/data` envelope while keeping the System audit page compatible with legacy, enveloped, and `code=-1` payloads.
- Normalized admin system config CRUD success and user-error responses to the panel `code/msg/ts/data` envelope while retaining sensitive-value masking and keeping the System runtime/config views compatible with legacy, enveloped, and `code=-1` payloads.
- Normalized admin load balancer CRUD and health-check success/user-error responses to the panel `code/msg/ts/data` envelope while keeping the System load balancer view compatible with legacy, enveloped, and `code=-1` payloads.

### Documentation

- Updated WireGuard P0 status docs to reflect the implemented panel peer/subscription slice while keeping full V2bX runtime, WSS compatibility UI, traffic/limit behavior, migration evidence, and GitHub Actions relay verification marked incomplete.
- Documented the P0 WireGuard dual-node relay plan, including WireGuard user access, domestic entry termination, default GOST relay+QUIC transport, overseas exit NAT, and WSS compatibility mode.
- Added the initial repository, concurrency, and performance audit baselines plus root `ROADMAP.md`, root `TODO.md`, and `docs/manual-intervention.md`.
- Documented the production root deployment command, Go/Node/npm prerequisites, deploy script self-test, and common recovery hints.
- Added a SQLite-to-PostgreSQL migration runbook covering dry run, import, verification evidence, and rollback.
- Added the traffic stats operations runbook covering `v2_server_log` indexes, query bounds, and retention maintenance.
- Added forwarding design, API, security, and compatibility baselines under `docs/forwarding/`, documenting the current runtime boundaries, Flux-shaped route contract, security controls, and remaining clone/runtime gaps.

### CI/CD

- Added `config/scripts/generate_release_notes.py` with a CI self-test so tag releases attach deterministic `RELEASE_NOTES.md` generated from the current `CHANGELOG.md` instead of relying only on GitHub's generated notes.
- Added `config/scripts/verify_release_artifacts.py` with a CI self-test and a release-job verification step so tag releases fail before publishing if required artifacts are missing or `RELEASE_MANIFEST.json`/`SHA256SUMS.txt` disagree with the release directory.
- Moved release manifest generation into `config/scripts/generate_release_manifest.py` with a CI self-test so release artifact metadata generation is directly validated instead of living only as inline workflow code.
- Added a machine-readable `RELEASE_MANIFEST.json` to GitHub Release assets with tag, commit, run metadata, CI build-source marker, manual-deployment flag, artifact sizes, and artifact SHA-256 hashes; the release workflow policy guard now fails if the manifest is removed.
- Attached SQLite-to-PostgreSQL migration dry-run output as `migration-dry-run.txt` in release artifacts, included it in release checksums, and extended the release workflow policy guard to prevent removing that evidence.
- Added a release workflow policy CI gate with self-tests so tag-gated release jobs keep their quality/security/race/test prerequisites, multi-platform binary matrix, Docker metadata, frontend archives, checksums, SBOM, operator deployment runbook, and generated release notes.
- Added a documentation sync CI gate with self-tests so implementation, frontend, deployment, workflow, or config changes must include maintained status documentation such as `CHANGELOG.md`, `TODO.md`, `docs/features.md`, audit docs, forwarding docs, guide docs, reference docs, or manual intervention notes.
- Added Go quality gates for `go mod tidy`, `gofmt`, `go vet`, full `go test ./...`, race testing, benchmark smoke testing, `govulncheck`, blocking production runtime `gosec`, non-blocking full-repository `golangci-lint` reports, and Docker build smoke testing.
- Promoted full-repository `golangci-lint` from report-only to a blocking CI gate after clearing the baseline.
- Updated the former full-repository `gosec` report to exclude generated code after all non-generated findings were cleared.
- Promoted the generated-file-excluded full-repository `gosec` report to a blocking CI gate while preserving the JSON artifact upload.
- Added `.golangci.yml` to keep frontend dependency trees out of Go lint reports.
- Added a SQLite-to-PostgreSQL migration dry-run CI gate and made release/backend builds depend on it.
- Added realistic service benchmarks for hourly traffic and user-ranking queries so benchmark smoke covers stats hot paths.
- Added forwarding runtime job benchmarks for admin listing filters and clean-agent heartbeat claiming so the service benchmark smoke covers runtime queue hot paths.
- Added a frontend bundle size report script and CI artifact that tracks total JS/CSS output plus heavy admin/runtime chunks such as Forward, Nodes, System, NodeX, LocalRuntime, echarts, and G6.
- Replaced the placeholder production deploy job with a release-attached `OPERATOR_DEPLOYMENT.md` manual deployment runbook.
- Added SPDX JSON SBOM generation to release assets using `anchore/sbom-action`.
