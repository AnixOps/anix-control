# Anix Control 4.0.x RC Evidence

Date: 2026-09-28

This record is a local verification snapshot for the `4.0.x` RC roadmap. It is
not a production approval and does not replace the signed CI or staging
records required by [`ROADMAP-4.0.x-RC.md`](ROADMAP-4.0.x-RC.md).

Update 2026-09-29: the Control Center rows below were recorded in the separate
`Anixops-control-center` repository. It has since been imported into
`control-center/` (and its Workers API into `control-center/workers/`); its CI
now runs from `.github/workflows/control-center.yml`, and its web suite is 23
files / 220 tests because mock-only suites for the removed AI/Web3/observability
pages were dropped. The rows are kept unchanged as the historical snapshot.
PR #7 (`57c62541`: `go_dev` CI green, Go toolchain 1.26.8) and PR #8
(`8dfc7b70`: dead code and legacy V2bX/Xray test tooling removed) merged after
this snapshot, so the Go rows must be re-run on the evidence commit. PRs #11
to #19 (plugin platform hardening, M0/M1 in
[`architecture/package-extraction.md`](architecture/package-extraction.md))
merged on 2026-09-29; see the production-copy rehearsal below.

## Passing Locally

| Area | Command or evidence | Result |
|------|---------------------|--------|
| Go formatting | `gofmt -l` over tracked Go files | Passed |
| Go modules | `GOTOOLCHAIN=auto go mod tidy -diff` | Passed with no diff |
| Go static analysis | `GOTOOLCHAIN=auto go vet ./...` | Passed |
| Go tests | `GOTOOLCHAIN=auto go test ./... -count=1 -p=1` | Passed |
| Go race | `GOTOOLCHAIN=auto go test -race ./... -count=1 -p=1` | Passed |
| SQLite to PostgreSQL migration | `go run ./cmd/sqlite2postgres -dry-run` against local PostgreSQL 17 | Passed; 3-table fixture inspected with no target writes |
| PostgreSQL restore rehearsal | `scripts/postgres_restore_rehearsal.sh` against disposable local PostgreSQL 17 | Passed; schema/data dump, reset, restore, counts and hashes matched |
| Release scripts | `python3 -m unittest discover -s config/scripts -p '*_test.py'` | 49 tests passed |
| Package contracts | Official package self-tests, package tests, and four release gates | Passed; 53 Python tests; reproducible signed/unsigned contracts and tamper rejection passed |
| Real Agent package inputs | Pinned Agent `c459383955027ee00719fa2dac1a87813c88a511`; four `linux/amd64` binaries; GOST v3.2.6 SHA-256 `a2aea24efb4597b5f57b35b8e1bbcc59f439b80723854d4371f6828b46682ffb` | Versions and runtime checksum passed; nftables-forward and gost-mesh release gates consumed real binaries |
| Cross-repository Agent E2E | `ANIXOPS_CROSS_REPO_E2E=1 ... go test -v -count=1 ./internal/grpc -run '^Test(KernelOperationBridgeCrossRepositoryAgentProcess|AgentPluginPackageCrossRepositoryE2E)$'` | Passed; 2 tests, 19.536s |
| Agent failure/recovery contracts | Focused `internal/grpc`, `internal/service`, and `internal/handler` lifecycle tests | Passed; deadline timeout, transport/Agent rejection, cancellation delivery failure, Control restart replay, bounded retry recovery, install/update/enable chain, disable, and chain-build rollback cases |
| Full local Control/Agent process rehearsal | Temporary full `cmd/server` with gRPC dispatcher + pinned Agent fixture + signed `machine-telemetry` package | Passed; real process completed persisted `plugin.install`, `plugin.update`, and `plugin.enable` operations; all three succeeded and Agent installation became healthy |
| Full package bundle rehearsal | `packages/shared/build_package.py --all --formal-release` with pinned Agent binaries and GOST v3.2.6 for `linux/amd64` and `linux/arm64`, followed by `--verify-release` | Passed; 16 `.anxp` artifacts, 16 manifests/signatures, and 16 SPDX SBOMs verified with an ephemeral local Ed25519 root; this is not the protected official signing bundle |
| Package WebUI | Machine Telemetry and GOST Mesh smoke scripts | Passed |
| Live Control signed WebUI and native Plugin Center | `ANIXOPS_AGENT_ROOT=... npx playwright test --config playwright.live-control.config.js` | Passed; 2 Chromium tests used a real Control binary with signed identity/machine packages, covered WebUI fetch/disable/revocation, and opened the merged native `/admin/plugins` page against the same real process. The native page assertion is local process evidence, not Agent staging. |
| Release workflow | `bash config/scripts/check_release_workflow.sh` | Passed |
| Release stage | `python3 config/scripts/check_release_stage.py --tag v4.0.0` | Accepted; sixteen-package scope |
| Release self-tests | Stage, workflow, rehearsal, evidence, docs-sync, and deploy guards | Passed |
| Control Center frontend | `npm ci`, `npm test -- --run` | 31 files / 384 tests passed, including separate Control login, MFA, session isolation, stable retry idempotency, unverified-catalog handling, operation history, and direct kernel API client coverage |
| Control Center build | `npm run build` | Passed |
| Control Center browser flow | `npm run test:e2e -- --reporter=line` | 19 executable tests passed; 7 pre-existing credentialed tests skipped; mocked Control login/MFA-to-catalog, separate 401 session recovery, mobile navigation/layout, Control and Agent plugin install/enable/configure/update/rollback/disable, and mocked disabled-execution 409/403/501 error paths passed in Chromium |
| Control Center real Control gate | `ANIXOPS_REAL_CONTROL_WEB_PORT=3013 ANIXOPS_REAL_CONTROL_API_PORT=38083 npm run test:e2e:real-control -- --reporter=line` | Passed; 1 Chromium test built a signed temporary `identity-platform` package, started an isolated real Control process, completed `/api/v2/login`, and used the returned Control JWT for `/api/v3/plugins` and `/api/v3/plugin-installations`, asserting healthy enabled `identity-platform` 4.0.0. Workers auth was synthetic localStorage by design; the default gate is read-only after bootstrap. |
| Control Center local lifecycle gate | `ANIXOPS_AGENT_ROOT=... ANIXOPS_REAL_CONTROL_LIFECYCLE=1 npm run test:e2e:real-control -- --reporter=line` | Passed; 1 Chromium test built a signed formal `machine-telemetry` package with the pinned Agent checkout, installed it into the isolated Control process, and used the Center page to disable and re-enable the Control target, verifying disabled and healthy/succeeded states through the real `/api/v3` API. This is local Control-process evidence, not Agent staging. |
| Canonical Control plugin page | `npm test -- --run src/__tests__/Plugins.test.js` | Passed; 17 tests cover target-scoped lifecycle actions, revisioned configuration, official-only installation, plugin-only recent operation history, cancellation, polling, operation-chain display, and stable retry idempotency keys. |
| Control frontend | `npm test -- --run` | Passed; 56 files / 346 tests, including the canonical plugin page and shared operation timeline. |
| Control frontend build | `npm run build` | Passed; production Vite build completed with the plugin history integration. |
| Canonical Control browser flow | `npm run test:e2e -- --reporter=line` | Passed; 5 Chromium tests, including the native `/admin/plugins` operation-history and cancellation flow, signed WebUI isolation, and narrow deployment layout. |
| Production frontend dependencies | `npm audit --audit-level=high --omit=dev` | 0 production vulnerabilities |
| Control Center CI definition | `.github/workflows/ci.yml` `web-test` job | Added with always-uploaded Playwright/test-results/dist artifacts; it will become official evidence on the next post-push Actions run |

The Control Center development dependency audit still reports five advisories
through the current Vite/Vitest and happy-dom major line. Clearing those
requires a deliberate toolchain upgrade; `npm audit fix --force` was not used
as part of this lifecycle slice.

The Go tests and race tests modify a tracked SQLite fixture during execution;
the fixture was restored to its empty `HEAD` state before this evidence
snapshot. Generated frontend output is ignored and is not part of the source
change.

## Verified Official CI Baseline

The published `v4.0.0` GitHub Actions run `29711408468` is independently
verifiable from the release assets. It is bound to commit
`349ef1c487f02a3988ffffc101da1db70a712ec5` and contains the protected official
root, sixteen signed package artifacts, manifests, SBOMs, release checksums,
SQLite/PostgreSQL rehearsal records, V2/WebSocket compatibility transcripts,
and signed canary/support approval records. The local checks used to verify the
downloaded assets were:

```text
python3 config/scripts/verify_release_artifacts.py ...
python3 config/scripts/verify_v4_evidence.py --archive v4-release-evidence.tar.gz ...
```

The official root matched `config/config.prod.yaml`. This is release evidence
for the tagged baseline; the current worktree changes are uncommitted and must
be included in a later CI run before they can be called an official release.
The `signed-plugin-releases` workflow artifact contained 81 package files; the
published release package set and the evidence bundle matched those files
byte-for-byte, and `build_package.py --verify-release` passed against the
official root. The evidence archive SHA-256 is
`fa686ab389f019c519958206dc8a3159cb45a9a52899a5639d36b1cc479f1bde`.

## Production-Copy Upgrade Rehearsal

Recorded 2026-09-29. Old side: the published `v4.0.0-alpha.7` binary, which
matches the production schema. New side: `go_dev` `b905a4a8`,
with the sixteen signed `v4.0.0` packages installed through `/api/v3`
(`identity-platform` through `identity_bootstrap_package_dir`). Both sides ran
against copies of the 2026-09-29 production PostgreSQL backup, with
credentials replaced by invalid values. Both servers and the test client ran
inside one `unshare -n` network namespace with only `lo`, with PostgreSQL
reached over its Unix socket.

| Area | Evidence | Result |
|------|----------|--------|
| Schema upgrade | Table, column and index diff between the two copies after the new server started in `env: production` | Passed; only the five `v4_kernel_package_*` tables and their indexes were added |
| Package install | 16 signed `v4.0.0` packages on the upgraded copy | Passed; all 16 healthy |
| Subscriptions | 18 users x `/s/:token` and `/api/v1/client/subscribe` x every `?type=`, byte comparison | 360/360 identical |
| UniProxy v1 | `config`, `user`, `alivelist` for every node and protocol, byte comparison | 45/45 identical |
| `/api/v2` GET routes | Every catalogued GET route that needs no path parameter, admin and user JWTs, status and body (without `ts`) | 82/82 status equal; 77/82 bodies equal (3 `cached_at` timestamps, 2 per-instance local-ansible paths) |
| Latency | p50/p99 for subscribe, UniProxy user, `/api/v2/user/info`, `/api/v2/admin/users` | New side within noise of the old side (p50 0.23-4.18 ms) |
| Host supervision | `SIGKILL` of the `identity-platform` host process | Passed; watchdog restarted it after 1 s, the next admin request returned 200 |
| Network isolation | `ss -tanp` at the end of the run; blocked-egress log lines | Passed; no non-loopback sockets; only the forward latency prober attempted egress |

Summary and defects found: [`architecture/release-line-status.md`](architecture/release-line-status.md#production-baseline-and-upgrade-rehearsal).
The harness and its output stay on the operator machine, because they contain
production data. This is local evidence, not a staging canary or production
approval.

## Not Run Locally

The following evidence remains a CI/staging gate and must not be inferred from
the passing local checks:

- a Control Center browser lifecycle mutation run against a live Control/Agent
  staging pair (the real local gate above proves login and read-only catalog /
  installation state; its signed identity package does not claim Agent staging
  or lifecycle mutation evidence);
- live Control/Agent staging with the CI-pinned external Agent revision
  (the local cross-repository and full-process rehearsals above are not a
  staging canary);
- Agent-target signed artifact install, observed-state write-back, restart
  recovery, timeout, health failure, and reverse rollback records;
- legacy API, subscription, node heartbeat, and WebSocket staging smoke;
- a new official CI evidence bundle bound to the current worktree changes
  (the tagged baseline has signed canary/support records, but those records do
  not attest to uncommitted changes);
- stable-production authorization.

The release workflow already contains the migration, restore, package signing,
cross-repository Agent, and live-Control browser jobs. The next execution should
attach their artifacts to the RC record before a release tag is considered.

## Release Handoff

The local implementation is ready for a reviewed handoff. The next evidence
identity must be the commit SHA produced after the current changes are approved
and pushed. That workflow run must replace the tagged-baseline references above
for the current RC, then add the staging canary and operator approval records.
Until those records exist, this document remains a verification snapshot and
the feature status remains `Preview/Partial`.

## Compatibility Invariants

- The Control Center retains its existing `/api/v1` client for legacy pages.
- Plugin lifecycle requests use the authenticated kernel `/api/v3` contract
  directly; no adapter or BFF was added. The Control `/api/v2/login` token is
  separate from the Workers `/api/v1/auth/login` token; one cannot authenticate
  the other's API.
- The Center reads `/api/v3/extensions` but does not render signed WebUI modules.
  Control's own same-origin frontend remains the verified module loader.
- Control execution, Agent dispatch, topology execution, and dynamic data-plane
  switching remain default-off until the external gates above are complete.
