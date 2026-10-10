# Anix Control 4.0.x Release Candidate Roadmap

Date: 2026-09-28

> Historical record. The release-stage, rehearsal, evidence and approval gates
> it cites were retired after `v4.0.0`; releases now follow
> [`RELEASING.md`](RELEASING.md). The handoff checklist below is not an open
> work list: open work is in [`../TODO.md`](../TODO.md).

## Goal

Deliver an auditable `4.0.x` release candidate in one focused week. The
release candidate is built on the current `v4` module and `go_dev` branch.
`anix-control` remains the product kernel; the Control Center is a
compatible frontend that calls the kernel API directly. Since 2026-09-29 the
Control Center lives in `control-center/` of this repository (imported from the
now-archived `Anixops-control-center` and `Anixops-control-center-worker`
repositories), and `go_dev` is the only long-lived branch.

The RC must close the plugin lifecycle loop, from catalog discovery through
installation, configuration, enable/disable, update, rollback, observed state,
and audit evidence. It must not be described as a production stable release:
topology execution, Supervisor execution, and dynamic data-plane switching stay
feature-gated until the operator canary gates are approved.

## Implementation Checkpoint

As of `2026-09-28`, the first implementation slice is present in both
repositories:

- `anix-control` exposes the existing signed `/api/v3` lifecycle contract and
  returns the complete dependency apply chain in operation response headers.
- The canonical Control administrator plugin page now includes the Center
  operation-history view, plugin-only operation filtering, cancellation, and
  stable lifecycle idempotency keys for transient retry paths.
- Its focused Web E2E now drives the native `/admin/plugins` page through a
  mocked signed catalog, filters out non-plugin operations, and exercises
  cancellation; the full Control browser suite passes `5` tests.
- The Control Center (then `Anixops-control-center`, now `control-center/`) has a `/plugins` route and sidebar entry, a separate
  authenticated `/api/v3` client, catalog/release/installation state, revisioned
  configuration editing, Control and Agent install/update/enable/disable/
  rollback controls, idempotency keys, and operation history refresh. The
  Control session uses `/api/v2/login` and its own token; the Workers login
  remains separate for legacy Center pages.
- The control-center frontend passes `31` test files / `384` tests and a
  production build with Tailwind processing. Its Chromium browser suite passes
  mobile navigation/layout, the plugin catalog,
  install, enable, disable, update, rollback, idempotency, operation-chain,
  and revisioned configuration flow, plus Agent-target installation-intent
  lifecycle coverage. With mocked Control responses, a browser test connects
  through Control login and MFA before loading the catalog; another confirms
  that a Control 401 preserves the Workers session. The API client has direct
  base URL, bearer, header, and
  401 mapping tests. A dedicated real-process Center gate also builds a
  signed temporary identity package, starts isolated Control, and drives the
  actual login -> `/api/v3/plugins` -> `/api/v3/plugin-installations` flow;
  its Workers session is synthetic by design. An opt-in local lifecycle variant
  also builds a formal `machine-telemetry` package from the pinned Agent
  checkout and drives Control-target disable/enable through the Center page;
  this remains local Control-process evidence. The Go repository full test run
  and release contract
  self-tests pass;
- The Control Center CI workflow (now `.github/workflows/control-center.yml`)
  has a dedicated `web-test` job that installs from
  `control-center/web/package-lock.json`, runs frontend tests and the production
  build, audits production dependencies, runs the Chromium suite, and uploads
  the Playwright report, test results, and production `dist` artifact even when
  a browser test fails. The job still needs a post-push Actions run before it
  is official evidence. Generated test databases were restored before the
  verification snapshot.
- The external Agent is pinned to commit
  `c459383955027ee00719fa2dac1a87813c88a511`; its four `linux/amd64` plugin
  binaries were built locally, the pinned GOST v3.2.6 runtime was checksum
  verified, and both cross-repository E2E tests passed.
- The four official package release gates passed locally, including
  reproducible unsigned builds, ephemeral Ed25519 signing, public-key
  verification, and tamper rejection. The nftables-forward and gost-mesh
  gates consumed the real Agent binaries; the machine-telemetry and nat-egress
  gates retain their CI fixture contract by design.
- A disposable local PostgreSQL 17 rehearsal passed the SQLite migration
  dry-run, schema/data dump, destructive reset, restore, and count/hash
  comparison. This is migration evidence, not a live Control/Agent canary.
- The real Control signed-WebUI gate passed with a temporary production build,
  signed identity and machine-telemetry packages, upload/install, Chromium
  extension fetch, durable disable, post-disable asset revocation, and a
  second browser scenario that opens the merged native `/admin/plugins` page
  against the same real Control process. This is local process evidence, not
  Agent staging.
- A full local process rehearsal passed with the temporary `cmd/server`, its
  gRPC dispatcher, the pinned Agent fixture, and a signed machine-telemetry
  package: persisted Agent `plugin.install`, `plugin.update`, and
  `plugin.enable` operations all succeeded and the installation became
  healthy. This remains local process evidence, not a staging canary.
- A full package-bundle rehearsal passed with the pinned Agent binaries and
  GOST v3.2.6 for both release platforms: sixteen `.anxp` artifacts plus
  manifests, signatures, and SPDX SBOMs were built and verified with a
  temporary local Ed25519 root. The tagged `v4.0.0` baseline also has a
  separately verified protected official bundle in GitHub Actions; a new run
  is required to bind the current uncommitted worktree changes to that root.
- The official baseline's 81 workflow package files match the published
  release/evidence package files byte-for-byte, and the independent release
  verifiers pass against the configured official root.
- The release-stage test now supplies isolated executable fixtures to the
  package builder, preserving the builder's requirement for explicit Agent and
  runtime inputs while keeping the sixteen-package cohort assertion.

This checkpoint combines local reproducible evidence with the verified tagged
baseline. It does not claim that the current uncommitted worktree changes have
passed a new staging canary or official release run.

## Delivery Rules

- Keep the Go module path and existing `/api/v1` and `/api/v2` compatibility
  surfaces unchanged.
- Use the existing authenticated `/api/v3` kernel API. Do not add a BFF for the
  control center.
- Use the CI-pinned Agent/V2bX revision. The Agent repository is an external
  build input, not a third local product repository for this milestone.
- Treat signed manifests, immutable artifacts, migration output, rollback
  output, staging records, and CI logs as release evidence.
- Keep production execution flags off by default. A passing unit test or local
  browser test is not a production canary.
- Preserve legacy subscriptions, node heartbeats, UniProxy routes, and
  WebSocket behavior while the kernel surface is extended.

## Seven-Day Sequence

### Days 1-2: API and Release Baseline

- Freeze the RC plugin cohort, Agent revision, official public key, manifest
  digests, migration version, and rollback target.
- Verify the existing kernel routes:
  `GET /api/v3/plugins`, `GET /api/v3/plugin-releases`,
  `GET /api/v3/plugin-installations`, `PUT /api/v3/plugin-installations`,
  `POST /api/v3/plugin-installations/:id/actions`, and the installation config
  `GET`/`PUT` routes.
- Keep JWT and admin authorization on every management request.
- Require idempotency keys for lifecycle actions and preserve operation ID and
  operation-chain response headers.
- Add or tighten contract tests for permissions, signatures, artifact digests,
  revision conflicts, disabled execution, and idempotent retries.

### Days 3-4: Control Center Plugin Lifecycle

- Add a separate Kernel Plugin API client in the control center while retaining
  its existing `/api/v1` client for legacy pages.
- Read the Control API base URL from `VITE_KERNEL_API_URL`; the legacy Workers
  client keeps `VITE_API_URL`. Authenticate against Control `/api/v2/login`,
  attach only its JWT to `/api/v3`, and make a Control 401 require Control
  reconnection without ending the Workers session.
- Implement one user flow: login, list official plugins and installations,
  inspect a plugin, edit revisioned configuration, install or update, enable,
  disable, rollback, and inspect operation state.
- Show target, desired/observed version, revision, health, failure summary,
  operation ID, and disabled/unauthorized states.
- Keep signed WebUI module rendering in Control's verified same-origin loader.
  The Control Center reads the extension catalog but does not execute its
  modules; extension rendering in the Center needs a separate origin, digest,
  permission, and revocation integration before it can be accepted.

### Day 5: Staging and Failure Evidence

- Run an isolated Control/Agent staging pair using the CI-pinned Agent input.
- Exercise signed package installation, configuration, enablement, observed
  state write-back, restart recovery, revision conflict, duplicate request,
  timeout, cancellation, health failure, and reverse rollback.
- Confirm legacy `/api/v1`, `/api/v2`, subscription, node heartbeat, and
  WebSocket smoke paths remain usable.
- Record QUIC/WSS, WireGuard, and GOST Mesh results when the staging nodes
  support them, but do not call cross-region traffic production evidence.

### Day 6: Automated Evidence and Documentation

- Keep Go full tests, race, vet, formatting, module tidy, frontend tests,
  frontend build, and audit checks green.
- Run package self-tests, manifest negative fixtures, signature verification,
  WebUI smoke, release gates, migration dry-run, backup/restore, and rollback
  checks.
- Update `docs/features.md`, `TODO.md`, `CHANGELOG.md`, upgrade instructions,
  rollback instructions, RC notes, release manifest, checksum, and SBOM.
- Attach staging canary records and failure/rollback records to the RC evidence
  bundle.

### Day 7: RC Acceptance

The RC is accepted only when:

- the `go_dev` tree, including `control-center/`, is clean and the required
  checks are reproducible;
- Control and control-center lifecycle tests pass;
- signed package, manifest, artifact, checksum, and SBOM records agree;
- migration, restore, and rollback evidence is present;
- staging canary records are complete;
- production topology execution, Supervisor execution, and dynamic plugin
  execution remain explicitly gated.

## Interfaces and Compatibility

The control center calls these kernel surfaces directly:

- `GET /api/v3/plugins`
- `GET /api/v3/extensions`
- `GET /api/v3/plugin-installations`
- `PUT /api/v3/plugin-installations`
- `POST /api/v3/plugin-installations/:id/actions`
- `GET/PUT /api/v3/plugin-installations/:id/config`

Configuration writes send `config` and optional `expected_revision`. Lifecycle
Control lifecycle actions send `action`, an optional `target_version`, and a
unique `idempotency_key`; Agent-target intent updates use the kernel's
installation upsert contract and do not claim the same explicit request key.
The frontend displays the fixed public installation state
and generic operation errors; it never displays raw operation payloads.
Configuration is shown only to the authenticated administrator as the stored
JSON document. Any future package with `secret_fields` must add a dedicated
secret reference/redaction contract before it is exposed here.

## Test and Acceptance Matrix

### Control

- Kernel route, permission, trust-root, signature, artifact, and configuration
  contract tests.
- Installation lifecycle, idempotency, lease, restart recovery, timeout,
  cancellation, observed-state, and rollback tests.
- Legacy API, subscription, node heartbeat, and WebSocket smoke tests.
- Full Go test, race, vet, formatting, module tidy, package self-tests, and
  release-script tests.

### Control Center

- API client and error mapping tests.
- Plugin list, detail, configuration, revision conflict, and lifecycle state
  tests.
- Unauthorized, disabled-execution (mocked 409), 401, 403, 409, and 501 tests.
- Browser flow: login -> list -> configure -> enable -> inspect -> rollback.

## Explicit Non-Goals for This RC

- No removal or incompatible rewrite of `/api/v1` or `/api/v2`.
- No new backend adapter service for the control center.
- No claim of a 72-hour production canary or stable production approval.
- No production traffic takeover by WireGuard, GOST Mesh, NAT egress, or
  topology execution without the documented operator approval gates.

## Release Handoff Checklist

The implementation and local evidence are ready for handoff. Complete these
steps in order before calling the RC an official final build:

- [x] Keep the current `go_dev` and `master` worktrees reproducible and
  document the local verification snapshot.
- [ ] Land the reviewed changes on `go_dev` through PRs (`go_dev` is PR-only;
  see `.github/BRANCH_PROTECTION.md`). Since the Control Center now lives in
  `control-center/`, a single `go_dev` commit SHA becomes the evidence
  identity.
- [ ] Run the release workflow for those SHAs and retain the signed package,
  manifest, checksum, SBOM, migration, restore, and browser artifacts.
- [ ] Run the isolated Control/Agent staging canary with the pinned Agent
  revision. Record Agent install, observed-state write-back, restart recovery,
  timeout, health failure, duplicate request, and reverse rollback outcomes.
- [ ] Attach legacy API, subscription, node heartbeat, WebSocket, and any
  supported QUIC/WSS, WireGuard, or GOST Mesh smoke records.
- [ ] Obtain the documented operator approval, then publish the RC tag and
  release bundle. Keep execution and topology feature flags disabled until
  that approval is recorded.

The evidence commit and external approval remain explicit review decisions.
Longer-term development after the RC (moving business domains into packages)
is planned in
[`architecture/package-extraction.md`](architecture/package-extraction.md).

## Completion Evidence

The milestone is complete only when the repository contains the implementation,
tests, updated status documents, reproducible CI evidence, staging records, and
the signed RC artifact bundle described above. A green local test run alone is
insufficient.
