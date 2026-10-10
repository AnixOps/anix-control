# AGENTS.md: AnixOps Control Contributor And Agent Rules

These rules apply to humans and coding agents in this repository. Keep this
file short and enforceable; put explanations in `docs/`.

## Project Overview

- `anix-control` is the AnixOps Control plane: Go module
  `github.com/AnixOps/anix-control/v4` (gin + GORM on SQLite or PostgreSQL)
  with the Vue 3 + Vite admin/user frontend in `web/`. The Go toolchain is
  pinned in `go.mod` and in `ci.yml` (`GO_VERSION`).
- `sdk/` is a second Go module, `github.com/AnixOps/anix-control/sdk`: the
  AnixOps protocol contracts (`sdk/api/*`, including the Agent contract
  `sdk/api/agent/v1`) and the SDKs modules and other services build on. The
  kernel uses it through `replace => ./sdk`; it must
  never depend on the kernel module (`check_package_boundaries.sh`). Run its
  checks with `go -C sdk build ./...`, `go -C sdk vet ./...` and
  `go -C sdk test ./...`.
- `identity/` is the identity core module, `github.com/AnixOps/anix-control/identity`
  (signing keys, token issuing, OIDC discovery). It is product-neutral and
  may depend only on the SDK module (`check_package_boundaries.sh`); check it
  with `go -C identity build ./...`, `vet` and `test` like the SDK.
- v4.0.0 is the "plugin-only platform" release: signed plugin packages under
  `packages/*`, package host processes (`sdk/api/pluginhost`, `sdk/api/packagebridge`,
  `sdk/pluginhostsdk`, `sdk/packagebridgesdk`), and the kernel API under
  `/api/v3` (catalog, releases, installations, operations, deployments).
- Reality check: every `/api/v2` business route is registered through
  `registeredPackageRoute` in `internal/router/router.go`, goes through the
  package gateway to a package host process, and is bridged back into the
  legacy in-kernel gin handlers and services
  (`internal/identitybridge/identity_bridge.go`). The business domains have not
  moved out of the kernel yet (`docs/architecture/release-line-status.md`).
  Behavior changes to v2 routes are still made in `internal/handler/` and
  `internal/service/`; keep the route listed in
  `config/v2-package-route-catalog.json`
  (`config/scripts/check_v2_package_route_catalog.py`,
  `config/scripts/check_plugin_only_routes.py`). The extraction design and
  milestones are in `docs/architecture/package-extraction.md`.
- Outside the package gate, kernel handlers serve `/api/v1/server/UniProxy/*`,
  `/{subscribe_path}/:token` (default `/s/:token`), `/api/v1/client/subscribe`,
  and `/flow/upload` directly. Do not change their response shapes without
  compatibility tests.
- The node runtime is `anix-agent` (separate repository `AnixOps/anix-agent`).
  Control talks to agents over gRPC. The Agent contract (`anix.agent.v1`) is
  owned here, in `sdk/api/agent/v1`, `sdk/agentcontrol` and `sdk/plugincontrol`.
  Agents in the field run it, so changes must be additive
  (`internal/tests/protocompat`). Control imports no anix-agent module:
  `github.com/AnixOps/anix-agent/sdk` is frozen at v1.1.0
  (`config/scripts/check_agent_sdk_dependency.sh`).
  Cross-repository golden fixtures live in `contracts/`.
- `control-center/` is a separate app; see the Control Center section.
- Status sources: `docs/features.md` (feature register), `TODO.md` (open work),
  `CHANGELOG.md` (history). A route or UI existing is not proof a feature is
  complete.

## Repository Map

The single layout document is `docs/reference/repository-layout.md` (do not
duplicate it here); the docs landing page is `docs/README.md`.

## Branch And PR Workflow

- `go_dev` is the integration branch. Branch from it, open a PR back to it,
  and squash-merge. Do not push to `go_dev` directly and do not recreate
  `master` or `production`. The only other long-lived branches are the
  maintenance branches `release/vX.Y`, cut with the owner's approval and
  changed through PRs like `go_dev` (`docs/RELEASING.md`).
- The `go_dev` ruleset (`.github/BRANCH_PROTECTION.md`) requires a PR with
  0 approvals and these checks: Go Quality Gates, Go Lint Gate, Backend Tests,
  Frontend Build, Documentation Sync Check, Release Workflow Policy Check,
  Frontend Visual Regression. No
  force pushes or deletions; administrators may bypass only by merging a PR.
  The intended `release/**` ruleset has the same rules.
- CI has two lanes (`config/scripts/classify_changes.py`, job "Classify
  Changes").
  - **Fast lane.** Pull requests run the required checks plus the heavy jobs
    whose paths changed:
    - PostgreSQL jobs for schema, storage and service code;
    - Docker and Kubernetes smokes for the module runtime, identity and
      deployment;
    - forward, Agent and package jobs for their areas (the `forward` class
      also covers the forward SDK, for the "Forward Netns E2E" job).

    Documentation-only PRs skip the Go jobs.
  - **Full lane.** Every job, including benchmarks.
    It runs on `go_dev` and `release/**` pushes, tags, the nightly
    schedule, manual runs, PRs labelled `ci:full`, and PRs that change the Go
    dependencies, the classifier or `.github/actions/`.
  - **Workflow changes.** A PR that changes `ci.yml` runs the full lane,
    unless the change stays inside the bodies of jobs gated on one class
    (`if: ${{ needs.changes.outputs.<class> == 'true' }}`) or of always-run
    jobs, with their `if:`, `needs:` and `outputs:` unchanged. Then it runs
    those classes plus the classes of the jobs that need the changed ones.
    Changes to the header, to Classify Changes or the tag gate, adding or
    removing a job, and edits to release, edge or schedule-only jobs still
    run the full lane.
  - **Job graph.**
    - "Go Quality Gates" runs the static gates: protobuf and `go mod tidy`
      drift, the route, worker and package-boundary gates, the script unit
      tests, gofmt and `go vet`. It does not run the tests.
    - "Backend Tests (1/3)" to "(3/3)" are the single full Go test run.
      `config/scripts/plan_test_shards.py` splits the packages by measured
      time, and each shard runs its packages four at a time (`-p=4`), except
      the timing-sensitive `SERIAL_PACKAGES` (`internal/kernelnodeops`),
      which run alone afterwards. Each
      shard takes coverage of its share of `./internal/...` (without
      `internal/tests/integration` and `e2e`) and runs its share of the
      other root-module packages. Shard 1 also runs `sdk`, `identity` and
      the Swagger gate.
    - The required "Backend Tests" job aggregates the shards. It fails unless
      every shard passed and every package ran exactly once. It then merges
      the coverage profiles (`plan_test_shards.py --merge-coverage`) and
      writes the coverage report.
    - "Package Storage PostgreSQL (1/4)" to "(4/4)" split the PostgreSQL
      package tests by measured time, each shard with its own PostgreSQL.
      Inside a shard they run one at a time (`-p 1`): package schemas and
      roles are named after package ids, so two test binaries on one
      database would collide. The shard's PostgreSQL runs without
      `fsync`, `synchronous_commit` and `full_page_writes`: the harnesses
      commit tens of thousands of single statements, so a slow runner disk
      doubled the run time. No test crashes that database.
    - When a test package gets much slower or faster, update its seconds in
      `WEIGHTS` in `plan_test_shards.py` (`--summary` prints the split).
    - "Build Smoke Images" builds the Control image and the identity-platform
      module image once, with a buildx `type=gha` layer cache per image.
      "Docker Build Smoke" and "Kubernetes Smoke" load its archives and
      start without waiting for the Go jobs.
    - "Smoke Tests" and "E2E Tests" start at once as well.
    - The edge image publish still waits for the Go quality and security
      gates, Backend Tests, the frontend and the Docker smoke.
  - **Race detector** (`go test -race`, about 28 min serially). It runs only
    in the nightly schedule and manual runs, with a 40-minute timeout, and no
    release job waits for it. Check the latest nightly run before tagging.
  - Skipped jobs count as passed for the required checks, so conditions stay
    job-level `if:`. Never use a workflow-level `paths` filter.

## Build And Test Cheat-Sheet

Always run Go with `GOWORK=off`; the module is not part of any `go.work`.

```bash
GOWORK=off go build ./... && GOWORK=off go vet ./...
GOWORK=off go test ./... -count=1 -p=1        # the full suite; CI runs it in Backend Tests (plus sdk, identity)
GOWORK=off go test ./cmd/server -count=1      # reads Dockerfile, ci.yml, deploy_panel.sh
make run              # isolated dev instance: API 127.0.0.1:19080, UI :19000, gRPC :50052
make build            # frontend (web/public) + backend (build/anix-control)
make docker-build     # container image, Dockerfile "source" target (anix-control:dev)
docker compose up -d --build   # development stack with a throwaway PostgreSQL
make test             # unit tests (./internal/..., -race)
make test-grpc        # internal/grpc with coverage
make test-e2e         # internal/tests/e2e
make lint             # golangci-lint (Go Lint Gate)
make swagger          # regenerate docs/docs.go, docs/swagger.{json,yaml}
make grpc-gen         # regenerate api/grpc/v2boardpb (pinned protoc, see tools.go)
make help             # list all targets
```

Frontend (`web/`, Node.js 22):

```bash
cd web && npm ci && npm test && npm run build
npm audit --audit-level=moderate
npm run test:e2e                              # Playwright; needs chromium
```

Repository gates you can run locally:

```bash
bash config/scripts/check_docs_updated.sh --base origin/go_dev --head HEAD
bash config/scripts/check_docs_updated.sh --self-test
bash config/scripts/check_release_workflow.sh
python3 config/scripts/check_mojibake.py                            # GBK mojibake, U+FFFD, private-use, BOM
python3 config/scripts/check_release_version.py --self-test
GOWORK=off python3 config/scripts/check_plugin_only_routes.py      # 243 /api/v2 routes vs catalog and packages
GOWORK=off python3 -m unittest discover -s config/scripts -p '*_test.py'
for c in pluginhost packagebridge modulepki identity kernelidentity kernelsubscriber kernelsettings kernelorder kerneltelemetry kernelnodeops forward agent; do bash sdk/api/$c/gen.sh; done  # needs protoc 29.2; then git diff must be empty
GOWORK=off go test ./internal/tests/protocompat                     # contracts may only grow
python3 config/scripts/check_proto_golden.py --base origin/go_dev   # the golden file only grows (no draft package left)
```

Every contract in `contracts/proto/descriptors.golden` is additions-only,
`anixops.forward.v1` included since F3a served it: never remove,
renumber or retype a field, message, RPC or enum value, and never edit a
golden line by hand (`check_proto_golden.py`, in the Documentation Sync
Check job, compares with the base revision).

The route gate, the release-script unit tests, and the generated-code drift
checks for `api/grpc` and every `sdk/api/*` contract all run in the
required "Go Quality Gates" job on every PR.

## Documentation Sync Gate

`config/scripts/check_docs_updated.sh` (CI job "Documentation Sync Check")
fails a diff that touches implementation, CI, deployment, or config surfaces
(`.github/workflows/*`, `api/*`, `cmd/*`, `internal/*`, `web/src/*` and web
build config, `go.mod`/`go.sum`, `Dockerfile`, `docker-compose*.yml`,
`config/deploy/*`, `config/scripts/*`,
`config/config.yaml.example`, `config/config.prod.yaml`) unless the same diff
updates one of `README.md`, `CHANGELOG.md`, `TODO.md`, `ROADMAP.md`,
`docs/README.md`, `docs/DEPLOYMENT.md`, `docs/features.md`,
`docs/manual-intervention.md`, or `docs/{audit,forwarding,guide,reference}/*.md`.
Default: add a `## Unreleased` entry in `CHANGELOG.md` and update the doc the
change affects.

## Version Bumps

A release tag `vX.Y.Z[-alpha|-beta|-rc.N]` must match every surface checked by
`config/scripts/check_release_version.py --tag vX.Y.Z`:

- `internal/branding/branding.go` (`DefaultVersion`)
- `Makefile` (`VERSION :=`)
- `cmd/server/main.go` (`// @version`)
- `web/package.json` and `web/package-lock.json` (`version` and `packages[""].version`)
- `docs/swagger.json`, `docs/swagger.yaml`, `docs/docs.go` (regenerate with `make swagger`)
- `config/config.yaml.example`, `config/config.prod.yaml`, `config/config.dev.yaml.example` (`app.version`)
- `CHANGELOG.md` (first `## X.Y.Z - YYYY-MM-DD` heading)
- `README.md` (the `- Current release:` line)

## Release Policy

- Releases are built only by GitHub Actions: a `v*.*.*` tag on a commit that
  landed through a PR on `go_dev` or on a `release/vX.Y` branch runs the
  `ci.yml` tag pipeline. A release branch's version bump is merged back to
  `go_dev` in a follow-up PR (`docs/RELEASING.md`).
  - It produces signed packages for every package under `packages/`, the
    binaries, a signed GHCR image with SBOM and provenance, the source SBOM,
    checksums and the manifest.
  - The release page ships the packages as one signed
    `anix-control-packages-<version>.tar.gz` (+ `.sig`), plus the
    `identity-platform` trio that `scripts/install.sh` downloads by name; one
    frontend archive (`.tar.gz`). `docs/RELEASING.md` lists every asset.
  - The release body is the tag's CHANGELOG section.
- Cut a release with `config/scripts/prepare_release.py <version>`, a pull
  request, then the tag (`docs/RELEASING.md`). There are no release-stage,
  rehearsal or approval gates.
- Never build release artifacts locally or on a production host; local deploy
  scripts are guarded by `config/deploy/check_release_build_policy.sh`.
- If you change release steps in `ci.yml`, update
  `config/scripts/check_release_workflow.sh` in the same PR.
- Containers are the primary deployment. The release `docker` job builds the
  `Dockerfile` `release` target from the release binaries, frontend and signed
  identity package, and publishes `ghcr.io/anixops/anix-control` (linux/amd64
  and linux/arm64, SBOM, provenance, cosign keyless signature, digest in
  `docker-image.txt`); `check_release_workflow.sh` enforces this. Pushes to
  `go_dev` publish `:edge`. Deployment assets: `docker-compose.prod.yml` and
  `config/deploy/compose/` (external PostgreSQL), `config/deploy/helm/anix-control`
  (single replica), `docker-compose.yml` (development). Container design and
  multi-replica blockers: `docs/architecture/container-deployment.md`.
- Configuration for containers is `ANIX_CONTROL_*` environment variables
  (`docs/reference/environment-variables.md`, generated and checked by
  `TestEnvironmentVariableReferenceIsCurrent`). A new config key changes that
  file: regenerate it in the same PR.
- `scripts/install.sh` (systemd) installs the release tag it is given:
  `--version vX.Y.Z` is required, and `install.sh` and `panel_install.sh`
  fetch the installer at that tag. It never reads `releases/latest` or a
  moving branch, so tagging a release cannot change what an unpinned command
  installs. Containers stay the primary deployment. Control Center tags
  (`control-center-v*`) are never marked latest. Operator docs:
  `docs/DEPLOYMENT.md`, `docs/UPGRADE.md`, `docs/guide/release-installation.md`.

## Forwarding (v4.2)

Forwarding is the v4.2 model in `docs/architecture/forward-sdk.md` (the
design and the owner's decisions H11–H28) with the API in
`docs/forwarding/v4-api.md`. Flux compatibility was dropped in v4.2 (F5d):
do not reintroduce flux-panel routes, DTOs, envelopes or page layouts, and
do not cite the archived flux documents (`docs/archive/`) as requirements.

- **The model.** A route is an ordered chain of hops (entry, optional
  relays, exit, targets), and every hop picks its own engine: `NFTABLES`
  (kernel DNAT, unencrypted, for trusted private links such as IEPL/IPLC),
  `GOST` (gost v3: TLS, WSS, QUIC, gRPC, multiplexing, for the public
  internet) or `ANIXOPS` (the AnixOps relay protocol; the slot exists, the
  engine ships in v4.3). Limits (bandwidth, quota, connections, expiry) sit
  on the entry hop; traffic is counted per hop and per direction. The
  planner is a pure function: routes and the node inventory in, one
  `NodeForwardState` per node with sticky port and mark allocations and a
  per-node generation out. Do not add per-engine special cases outside the
  planner and the drivers.
- **SDK first.** A forwarding change starts in `sdk/` and flows outwards:
  the contract (`sdk/api/forward/v1`, `anixops.forward.v1`), the model and
  validation (`sdk/forward/model`, `sdk/forward/validate`, one rule set for
  Control, the planner and the Agent), the planner (`sdk/forward/planner`,
  golden fixtures in `contracts/forward/v1`), the drivers
  (`sdk/forward/driver/*`, conformance suite and netns end-to-end tests in
  `sdk/forward/e2e`), then the kernel's forwarding state
  (`internal/kernelforward`, `ForwardControl`), the forward package's v4 API
  (`packages/forward/v4api`, `/api/v4/forward/*`), the operator CLI
  (`anix-control forward`) and the UI (`web/src/views/admin/forward/`).
  Other products import `sdk/forward` and call `ForwardControl`; nothing in
  `sdk/` may import the kernel (`check_package_boundaries.sh`).
- **Everything goes through Control.** Routes, nodes, settings and
  diagnoses are changed only through `ForwardControl` (the v4 API, the CLI
  or a package), where they are authorized and audited. A node never
  accepts routes from anywhere else: the Agent applies the desired state
  Control pushes on the Agent Control stream (`sdk/forward/wire`) and
  reports back; Ansible is only the fallback for hosts without an Agent and
  ships the same nftables artifacts. There is no standalone forwarding CLI
  or node-local configuration. Do not dial nodes from request handlers: a
  node's health, applied generation and hop errors come from its Agent's
  report, and its traffic from the counters the nodes push.
- **Contract freeze (`forward.v1`).** `anixops.forward.v1` has been binding
  since F3a: additions only. Never remove, renumber or retype a field,
  message, RPC or enum value, never edit a line of
  `contracts/proto/descriptors.golden` by hand, and keep
  `internal/tests/protocompat` and `config/scripts/check_proto_golden.py`
  green (its `DRAFT_PACKAGES` stays empty). The `forward.v1` Hello attribute
  and the `anixops.nodeconfig/v2` member are wire contracts with deployed
  Agents: change them only additively, with both sides' checks in
  `sdk/forward/wire`. Regenerate with `bash sdk/api/forward/gen.sh` and
  update the planner goldens only for an intended plan change, saying why in
  the PR.
- **Privileges and ownership (H13).** A driver touches only objects it owns
  and marks: the nftables driver the `inet anixops_fwd` table, its tc
  handles and its sysctl drop-in; the gost driver the `anixops-gost` unit
  and its files. A foreign object is never changed (`ErrNotOwned`,
  `ErrConflict`). The Agent runs as a dedicated user with ambient
  `CAP_NET_ADMIN` and `CAP_NET_BIND_SERVICE` in the systemd sandbox of
  forward-sdk.md section 14; gost runs as its own user with
  `CAP_NET_BIND_SERVICE` only; root is only for the installer and the
  signed-artifact updater unit. Uninstall and the v4.2 cleanup remove
  exactly what we installed plus the named legacy tables.
- **Link certificates (H28).** Node-to-node encryption uses per-node link
  certificates from a dedicated forward link CA, a separate root that signs
  nothing else (DNS name = the node's identity name such as `forward-41`,
  URI = its SPIFFE identity, serverAuth and clientAuth, 7 days, renewed
  with the Agent certificate). gost holds only the link certificate and the
  key the Agent generates for it, never the Agent's Control key; a link
  certificate never authenticates to Control. The desired state names
  identities, never keys or other secrets, and no v4 answer carries a node
  credential.
- **Editions (H23).** Routes, hops, the nftables, gost and AnixOps engines,
  limits, counters, load balancing, two-level failover, the circuit
  breaker, latency, diagnosis, entry HA via DNS, one-command onboarding and
  staged upgrades are in both editions. User self-service forwarding,
  forwarding plans, auto-renewal, billing multipliers and resellers are
  commercial (v4.3 and later); their API prefixes
  (`/api/v4/forward/self/`, `/plans/`, `/multipliers/`) are reserved in
  `config/editions.json` and must not appear in the community edition.
- **Security defaults.** Targets are `PUBLIC_ONLY` unless an administrator
  route says `ALLOW_PRIVATE` (never loopback, never on a user's route);
  Control checks at save time and the Agent re-checks every DNS answer.
  Relay and exit listeners admit only `ingress_sources`; the planner never
  allocates reserved ports (SSH, the Agent's, the node's list).
- **The legacy runtime is frozen.** The v4.1 flux tables (`v2_forward`,
  `v2_forward_tunnel`, `v2_forward_user_tunnel`, `v2_speed_limit`,
  `v2_forward_rule`, `v2_forward_runtime_job`, ...), their GORM models, the
  NodeX, local Ansible and clean agent runtimes and the remaining forward v2
  routes (all `kernel-owned`, served by the kernel) exist only until the
  legacy cleanup (F5c) archives the data and drops the tables (IRREVERSIBLE,
  gate H15). Add no feature, route, page or column to them, do not migrate
  their data into the new model, and never let the forward package adopt or
  read a flux table again (its manifest declares only `kernel.forward.v1`). `Node` (`/admin/nodes`, proxy
  service) and the forwarding inventory (`/admin/forward/inventory`) are
  different resources; never mix them in copy, validation or docs.
- **Docs to update together.** A forwarding change updates
  `docs/architecture/forward-sdk.md` (the design and its status),
  `docs/forwarding/v4-api.md` (API changes), `docs/guide/forwarding.md` (the
  pages) and `docs/UPGRADE.md` (operator-visible changes) in the same PR;
  Agent-side behaviour also updates `sdk/api/agent/v1/PROTOCOL.md` (the
  data plane, the forward diagnostic checks and the forward link
  certificates). In the PR description, say whether the change affects the
  `forward.v1` contract, the planner's output (goldens), a driver, or the
  legacy runtime.
- **Validate** with `GOWORK=off go -C sdk test ./forward/... ./api/forward/...`,
  `GOWORK=off go test ./internal/kernelforward ./packages/forward/...
  ./internal/tests/protocompat`,
  `python3 config/scripts/check_proto_golden.py --base origin/go_dev`, and
  for UI changes `cd web && npm test && npm run build`.

## Operator Experience And Root Hygiene

- Operators must not assemble flags, env keys, or troubleshooting steps from
  scattered notes: cover install, upgrade, the control-vs-execution model, and
  troubleshooting with copyable commands and smoke-test checklists. Do not add
  UI buttons for automation that does not exist.
- When startup behavior changes, update `docs/control-boundary.md`,
  `docs/README.md`, `docs/reference/quickstart.md`,
  `docs/reference/startup-config.md`, `docs/reference/configuration.md`, and
  `docs/reference/runtime.md` together. When Docker or env behavior changes,
  check both compose files and those docs in the same PR.
- Keep the root NodeX-like: one short `README.md`, one docs landing page, the
  layout only in `docs/reference/repository-layout.md`, docs layered as
  `docs/intro/`, `docs/reference/`, `docs/guide/`. `control-center/` is the one
  self-contained app directory at the root. Deployment assets live in
  `config/deploy/`; no top-level `deploy/`.
- Never commit logs, scratch output, sample YAML at the root, personal paths,
  personal domains or hostnames, or tokens. Agent-local state
  (`.claude/settings.local.json`, `.claude/projects/`, `.superpowers/`) and
  agent plans/specs stay out of the repository.

## Control Center (`control-center/`)

- `control-center/` holds the Control Center, imported on 2026-09-29 from the archived `AnixOps/Anixops-control-center` repository. `control-center/workers/` holds its Cloudflare Workers API, imported from the archived `AnixOps/Anixops-control-center-worker` repository. Do not reopen or push to the archived repositories.
- It is a separate Go module (`github.com/AnixOps/anix-control/control-center`, Go 1.24). Run its checks from inside the directory: `cd control-center && go test ./...`; `cd control-center/web && npm test -- --run && npm run build`; `cd control-center/mobile && flutter test`; `cd control-center/workers && npm run typecheck && npm test`.
- Its CI lives in `.github/workflows/control-center.yml` and `control-center-workers.yml` (path-filtered to `control-center/**`), and its releases in `control-center-release.yml`. Release tags are `control-center-v*`. Never let a Center release become the repository's latest release: the Releases page and the image's `latest` tag follow it, and the latest release must be a Control release.
- Keep `-exclude-dir=control-center` on gosec, `--exclude control-center` on swag (CI and the Makefile `swagger` target), and `control-center/` in `.dockerignore`.
- The Workers API deploys through Cloudflare Workers Builds (root `control-center/workers`, branch `go_dev`). A merge to `go_dev` that touches `control-center/workers/**` deploys `api.anixops.com`.
- `control-center/workers/migrations/` is applied to a production D1 database. Only append new migrations; never edit or delete existing ones.
- The Center's plugin page talks to Control through `/api/v2/login` and `/api/v3`. Its other pages use the Workers `/api/v1` session. Do not add a second Control API contract for it.

## Credentials And Privileged Operations

- All panel, admin, API, and deployment tokens are user-owned: do not infer,
  scrape, decode, read from protected config, or generate replacements unless
  the user provides the token or asks for that exact local test.
- Deployments, service restarts, writes under system paths, and anything else
  needing `sudo`/root are left to the user through the provided scripts; no
  interactive sudo workarounds.
- Before tool-dependent work (database, Go, frontend builds), check the tool
  exists; if it is missing, tell the user what to install or run instead of
  inventing a workaround.

## UTF-8 And Chinese Copy

- 所有源码和文档必须使用 UTF-8（无 BOM），不要保存为 GBK/ANSI。
- Before "fixing" garbled text, check the real bytes, for example with
  Python `Path(...).read_text(encoding='utf-8')` and
  `line.encode('unicode_escape')`; a terminal can mis-render valid UTF-8.
- Real corruption signals: private-use characters (`U+E000`-`U+F8FF`),
  `U+FFFD`, GBK-mojibake runs such as `鐢熶骇` or `缂栬瘧`, and `?` where a
  trailing byte was lost (this often swallows a closing quote or parenthesis).
- Re-check UTF-8 after editing Chinese copy in `web/src/views/`,
  `web/src/components/`, `web/src/layouts/`, `internal/handler/` (Swagger
  annotations flow into `docs/swagger.*`), `docs/`, and `AGENTS.md`. For a UI
  sweep start with `web/src/layouts/AdminLayout.vue`, the locale modules in
  `web/src/locales/`, and the forwarding pages in `web/src/views/admin/forward/`.
- 对话框关闭按钮统一使用 `×` 或 `✕`。
- In change notes, separate terminal display problems from real corruption.
- `python3 config/scripts/check_mojibake.py` (CI "Documentation Sync Check")
  rejects these signals in tracked files; intentional examples go in its
  `ALLOWLIST`.
