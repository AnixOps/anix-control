# V4.0.0 Stable Release Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Publish an auditable `v4.0.0` plugin-only stable release after every
required quality, integration, evidence, and approval gate passes.

**Architecture:** Repair release blockers at their source, then align all
release version surfaces before running the formal package rehearsal. The
rehearsal's deterministic package subject is signed into the two user-approved
operational declarations, which are stored only as GitHub Secrets. A tag is
created only after the pushed `go_dev` CI run is green.

**Tech Stack:** Go 1.26.5, golangci-lint, gosec, Go test, Vitest, Playwright,
Python package tooling, PostgreSQL 16, GitHub Actions, Ed25519.

## Global Constraints

- Target tag and product version are exactly `v4.0.0` and `4.0.0`.
- Preserve the V4 official root as the sole trusted package root.
- Never print, commit, upload, or read back the signing PEM or GitHub Secret
  values.
- Do not weaken or skip CI quality, security, browser, cross-repository, or
  formal evidence gates.
- Preserve the user's `.superpowers/sdd/task-6-report.md` as an unstaged main
  worktree change.
- Create no tag until the pushed `go_dev` CI run is green.

---

### Task 1: Restore Go Module Determinism

**Files:**
- Modify: `go.sum`

**Produces:** `go mod tidy` leaves `go.mod` and `go.sum` unchanged.

- [ ] **Step 1: Reproduce the stale checksum failure**

Run: `GOWORK=off go mod tidy && git diff --exit-code -- go.mod go.sum`

Expected: failure showing only obsolete `github.com/AnixOps/anix-agent/sdk v1.0.0` entries.

- [ ] **Step 2: Regenerate module metadata**

Run `GOWORK=off go mod tidy`, retain its minimal `go.sum` deletion, and inspect
the diff to ensure no dependency version changes occur.

- [ ] **Step 3: Commit module metadata**

Run:

~~~bash
git add go.sum
git diff --cached --check
git commit -m "build: tidy Agent SDK module checksums"
~~~

- [ ] **Step 4: Verify the CI command exactly**

Run: `GOWORK=off go mod tidy && git diff --exit-code -- go.mod go.sum`

Expected: exit 0.

### Task 2: Clear Production Lint And Security Findings

**Files:**
- Modify: `cmd/server/main.go`
- Modify: `internal/compat/v2/envelope.go`
- Modify: `internal/compat/v2/websocket.go`
- Modify: `internal/identitybridge/identity_bridge_test.go`
- Modify: `internal/packagebridge/http_adapter.go`
- Modify: `internal/packagebridge/session.go`
- Modify: `internal/packagebridge/session_test.go`
- Modify: `internal/packagebridge/websocket_adapter.go`
- Modify: `internal/packagebridge/websocket_adapter_test.go`
- Modify: `internal/pluginhost/artifact.go`
- Modify: `internal/pluginhost/client.go`
- Modify: `internal/pluginhost/process.go`
- Modify: `internal/service/kernel_service.go`
- Modify: `packages/identity-platform/control/main.go`
- Modify: `packages/shared/controlhost/main.go`
- Modify: `pkg/packagebridgesdk/client.go`
- Test: existing package tests covering each changed package

**Produces:** no ignored close errors, no deprecated client setup, no unsafe
numeric conversion, and no unbounded production file/process false positive.

- [ ] **Step 1: Capture the failing quality gates locally**

Install the pinned CI tools if absent, then run the exact production package
scan and golangci-lint command from `.github/workflows/ci.yml`.

Expected: 24 lint findings and 6 production gosec findings before changes.

- [ ] **Step 2: Write or extend regression tests before behavior changes**

Add focused tests for every changed overflow boundary and file-root validation:
negative lifecycle generations cannot reach an unsigned dispatch generation,
HTTP recorder status cannot be converted outside the valid range, archive size
comparisons reject invalid bounds, and artifact writes remain constrained to a
verified root.

- [ ] **Step 3: Apply minimal source fixes**

Handle close errors where they affect a returned result, use the current gRPC
client construction API, use `websocket.Dialer`, simplify the two header
character predicates, normalize error text, validate conversions before casts,
and use a verified path/root boundary for process and artifact operations.

- [ ] **Step 4: Verify changed packages and quality gates**

Run:

~~~bash
GOWORK=off go test ./cmd/server ./internal/compat/v2 ./internal/identitybridge ./internal/packagebridge ./internal/pluginhost ./internal/service ./packages/identity-platform/control ./packages/shared/controlhost ./pkg/packagebridgesdk -count=1
golangci-lint run ./...
gosec -exclude-generated -nosec-require-rules -nosec-require-justification -quiet $(go list -f '{{.Dir}}' ./cmd/server ./api/grpc/v2boardpb ./internal/... | sed "s#^${PWD}#.#" | grep -Ev '^\./internal/tests(/|$)' | sort)
~~~

Expected: all commands exit 0 without adding broad `nosec` suppressions.

- [ ] **Step 5: Commit quality and security repair**

Run:

~~~bash
git add cmd/server/main.go internal/compat/v2 internal/identitybridge internal/packagebridge internal/pluginhost internal/service packages/identity-platform/control packages/shared/controlhost pkg/packagebridgesdk
git diff --cached --check
git commit -m "fix: clear v4 release quality gates"
~~~

### Task 3: Separate Fast Browser E2E From Live Control E2E

**Files:**
- Modify: `web/playwright.config.js`
- Test: `web/e2e/live-control-machine-telemetry.spec.js`

**Produces:** the normal Vite-only suite never includes the real-Control test;
the separate live-Control configuration remains the only executor for it.

- [ ] **Step 1: Reproduce the normal-suite failure**

Run: `npm --prefix web run test:e2e`

Expected: the live machine-telemetry test times out because Vite proxies to no
Control API.

- [ ] **Step 2: Exclude only the live-Control spec from the normal config**

Add `testIgnore: 'live-control-machine-telemetry.spec.js'` to
`web/playwright.config.js`; retain every other normal browser test.

- [ ] **Step 3: Verify both distinct environments**

Run:

~~~bash
npm --prefix web run test:e2e
npx --prefix web playwright test --config web/playwright.live-control.config.js
~~~

Expected: the normal suite passes its four Vite tests and the dedicated suite
passes the real Control signed-WebUI test.

- [ ] **Step 4: Commit browser gate repair**

Run:

~~~bash
git add web/playwright.config.js
git diff --cached --check
git commit -m "test(web): isolate live Control browser gate"
~~~

### Task 4: Repair The Cross-Repository Package Host Gate

**Files:**
- Modify: `internal/grpc/agent_plugin_package_cross_repo_e2e_test.go`
- Modify: supporting control-host fixture source if required by the test
- Test: `internal/grpc/agent_plugin_package_cross_repo_e2e_test.go`

**Produces:** the cross-repository test exercises a package containing a real
Control host for the declared Control route, rather than sending that route to
an agent-only test artifact.

- [ ] **Step 1: Reproduce with the sibling Agent checkout**

Run:

~~~bash
ANIXOPS_CROSS_REPO_E2E=1 ANIXOPS_AGENT_ROOT=/root/code/anixops-workspace/anix-agent GOWORK=off \
  go test -v -count=1 ./internal/grpc -run '^TestAgentPluginPackageCrossRepositoryE2E$'
~~~

Expected: the agent lifecycle succeeds, then the declared Control route returns
`plugin_host_unavailable` because the hand-built artifact has no Control host.

- [ ] **Step 2: Add a failing assertion for the Control-host package contract**

Make the fixture assert that its signed artifact has a verified control
entrypoint before it declares a Control route; run the focused test and observe
the missing-entrypoint failure.

- [ ] **Step 3: Build and embed the capability Control host fixture**

Use the repository's compiled control-host implementation in the test artifact,
declare its digest in the manifest, and configure the test handler with the
same artifact resolver/host registry path used by production.

- [ ] **Step 4: Verify the complete process boundary**

Run the Step 1 command and then the CI's paired cross-repository test command.

Expected: package install, Control host health, telemetry route, and disable
operation all succeed against the real sibling Agent process.

- [ ] **Step 5: Commit the process E2E repair**

Run:

~~~bash
git add internal/grpc/agent_plugin_package_cross_repo_e2e_test.go internal/plugincontrol internal/pluginhost packages/shared/controlhost
git diff --cached --check
git commit -m "test: exercise Control host in Agent package E2E"
~~~

### Task 5: Align The V4 Stable Version Contract

**Files:**
- Modify: `internal/branding/branding.go`
- Modify: `Makefile`
- Modify: `cmd/server/main.go`
- Modify: `web/package.json`
- Modify: `web/package-lock.json`
- Modify: `docs/swagger.json`
- Modify: `docs/docs.go`
- Modify: `docs/swagger.yaml`
- Modify: `config/config.yaml.example`
- Modify: `config/config.prod.yaml`
- Modify: `config/config.dev.yaml.example`
- Modify: `install.sh`
- Modify: `CHANGELOG.md`
- Modify: `README.md`
- Test: `config/scripts/check_release_version.py`

**Produces:** every version surface accepted by `check_release_version.py` is
`4.0.0` and the changelog records the stable release date.

- [ ] **Step 1: Run the stable-tag contract before editing**

Run: `python3 config/scripts/check_release_version.py --tag v4.0.0`

Expected: failure naming the first `4.0.0-alpha.7` mismatch.

- [ ] **Step 2: Update every checked version surface**

Use `apply_patch` to replace the alpha version in the files above, add a dated
`## 4.0.0` changelog section, and retain historical alpha entries unchanged.

- [ ] **Step 3: Regenerate Swagger output and validate the contract**

Run:

~~~bash
go run github.com/swaggo/swag/cmd/swag@v1.16.6 init -g cmd/server/main.go -o docs --parseInternal
python3 config/scripts/check_release_version.py --tag v4.0.0
~~~

Expected: the version checker reports `release version surfaces match 4.0.0`.

- [ ] **Step 4: Commit version alignment**

Run:

~~~bash
git add internal/branding/branding.go Makefile cmd/server/main.go web/package.json web/package-lock.json docs/swagger.json docs/docs.go docs/swagger.yaml config/config.yaml.example config/config.prod.yaml config/config.dev.yaml.example install.sh CHANGELOG.md README.md
git diff --cached --check
git commit -m "release: prepare v4.0.0 stable"
~~~

### Task 6: Rehearse, Approve, Publish, And Verify

**Files:**
- Create outside Git: `/tmp/anixops-v4-0-0-release/`
- Modify GitHub Secrets: `ANIXOPS_V4_CANARY_APPROVAL`, `ANIXOPS_V4_SUPPORT_APPROVAL`
- Create Git tag and GitHub Release: `v4.0.0`

**Produces:** a published stable GitHub Release with verified formal evidence
and public release assets.

- [ ] **Step 1: Run all release-local gates**

Run:

~~~bash
GOWORK=off go test ./... -count=1 -p=1
npm --prefix web test
npm --prefix web run test:e2e
npx --prefix web playwright test --config web/playwright.live-control.config.js
ANIXOPS_CROSS_REPO_E2E=1 ANIXOPS_AGENT_ROOT=/root/code/anixops-workspace/anix-agent GOWORK=off go test -v -count=1 ./internal/grpc -run '^Test(KernelOperationBridgeCrossRepositoryAgentProcess|AgentPluginPackageCrossRepositoryE2E)$'
python3 config/scripts/check_release_version.py --tag v4.0.0
python3 packages/shared/tests/test_manifest_schema.py -v
bash config/scripts/check_release_workflow.sh --self-test
bash config/scripts/check_release_workflow.sh
~~~

Expected: every command exits 0.

- [ ] **Step 2: Build and verify the formal V4 package cohort**

Run:

~~~bash
release_root=/tmp/anixops-v4-0-0-release
key_dir=/root/.anixops-release/v4-root-20260720
install -d -m 0700 "$release_root/packages"
GOWORK=off python3 packages/shared/build_package.py --all --version 4.0.0 --out "$release_root/packages" --formal-release --signing-key "$key_dir/official-ed25519.pem" --official-public-key "$key_dir/official-public-key.raw"
GOWORK=off python3 packages/shared/build_package.py --all --version 4.0.0 --out "$release_root/packages" --verify-release --official-public-key "$key_dir/official-public-key.raw"
~~~

Expected: sixteen artifacts and sixteen SBOMs validate against the only V4
official root.

- [ ] **Step 3: Generate the two user-authorized approval records**

Run:

~~~bash
release_root=/tmp/anixops-v4-0-0-release
key_dir=/root/.anixops-release/v4-root-20260720
python3 config/scripts/create_v4_approval.py --kind canary --packages-dir "$release_root/packages" --signing-key "$key_dir/official-ed25519.pem" --trusted-official-public-key "$key_dir/official-public-key.raw" --output "$release_root/canary-approval.json"
python3 config/scripts/create_v4_approval.py --kind support --packages-dir "$release_root/packages" --signing-key "$key_dir/official-ed25519.pem" --trusted-official-public-key "$key_dir/official-public-key.raw" --output "$release_root/support-approval.json"
gh secret set ANIXOPS_V4_CANARY_APPROVAL --repo AnixOps/anix-control < "$release_root/canary-approval.json"
gh secret set ANIXOPS_V4_SUPPORT_APPROVAL --repo AnixOps/anix-control < "$release_root/support-approval.json"
~~~

Expected: both approved records validate before upload and no secret value is
displayed.

- [ ] **Step 4: Push the release-preparation branch into `go_dev` and await CI**

Fast-forward merge after a clean diff and push `go_dev`. Wait for the matching
CI run; proceed only if its conclusion is `success`.

- [ ] **Step 5: Create the immutable tag and await the release workflow**

Create annotated tag `v4.0.0` at the green `go_dev` commit, push it, and wait
for the tag workflow conclusion. Do not force-push or retag.

- [ ] **Step 6: Verify the public release**

Use `gh release view v4.0.0 --repo AnixOps/anix-control` and verify that the
release is not a draft or prerelease, contains the V4 evidence archive, and
its package/evidence public root matches the configured V4 root.
