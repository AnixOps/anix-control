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

- `go_dev` is the only long-lived branch. Branch from it, open a PR back to it,
  and squash-merge. Do not push to `go_dev` directly and do not recreate
  `master` or `production`.
- The `go_dev` ruleset (`.github/BRANCH_PROTECTION.md`) requires a PR with
  0 approvals and these checks: Go Quality Gates, Go Lint Gate, Backend Tests,
  Frontend Build, Documentation Sync Check, Release Workflow Policy Check. No
  force pushes or deletions; administrators may bypass only by merging a PR.
- CI has two lanes (`config/scripts/classify_changes.py`, job "Classify
  Changes").
  - **Fast lane.** Pull requests run the required checks plus the heavy jobs
    whose paths changed:
    - PostgreSQL jobs for schema, storage and service code;
    - Docker and Kubernetes smokes for the module runtime, identity and
      deployment;
    - forward, Agent and package jobs for their areas.

    Documentation-only PRs skip the Go jobs.
  - **Full lane.** Every job, including the race detector and benchmarks.
    It runs on `go_dev` pushes, tags, the nightly schedule, manual runs,
    PRs labelled `ci:full`, and PRs that change the workflow or Go
    dependencies.
  - Skipped jobs count as passed for the required checks, so conditions stay
    job-level `if:`. Never use a workflow-level `paths` filter.

## Build And Test Cheat-Sheet

Always run Go with `GOWORK=off`; the module is not part of any `go.work`.

```bash
GOWORK=off go build ./... && GOWORK=off go vet ./...
GOWORK=off go test ./... -count=1 -p=1        # what Go Quality Gates runs
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
python3 config/scripts/check_release_version.py --self-test
GOWORK=off python3 config/scripts/check_plugin_only_routes.py      # 292 /api/v2 routes vs catalog and packages
GOWORK=off python3 -m unittest discover -s config/scripts -p '*_test.py'
for c in pluginhost packagebridge modulepki identity kernelidentity kernelsubscriber kernelsettings kernelorder kerneltelemetry agent; do bash sdk/api/$c/gen.sh; done  # needs protoc 29.2; then git diff must be empty
GOWORK=off go test ./internal/tests/protocompat                     # contracts may only grow
```

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
  landed on `go_dev` runs the `ci.yml` tag pipeline.
  - It produces signed packages for every package under `packages/`, the
    binaries, a signed GHCR image with SBOM and provenance, the source SBOM,
    checksums and the manifest.
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
- `scripts/install.sh` (systemd) is frozen: keep it working, add no features.
  It installs from GitHub Releases and resolves `releases/latest` when no
  version is pinned, so the latest release must always be a Control release.
  Control Center tags (`control-center-v*`) are never marked latest. Operator
  docs: `docs/DEPLOYMENT.md`, `docs/UPGRADE.md`,
  `docs/guide/release-installation.md`.

## Flux-panel Clone Guardrails

The `/admin/forward*` pages clone upstream
[flux-panel](https://github.com/bqlpfy/flux-panel).

- Read `docs/guide/flux-panel-clone.md` (workflow, status, gaps) and
  `docs/guide/flux-forward-contract.md` (endpoints, envelopes, DTOs) before
  touching forward, tunnel, user-tunnel, or speed-limit pages or APIs.
- Keep 1:1 parity: path, method, auth scope, request field names, the
  `code/msg/ts/data` envelope, and DTO field names and casing. Do not drop DTO
  fields the current page does not use. Flux user-scope endpoints need JWT user
  routes; do not move them under `/api/v2/admin/*` (admin mirrors are
  compatibility only).
- Keep `web/src/views/admin/Forward.vue` aligned with upstream `forward.tsx`:
  no runtime backend selectors, runtime job tables, installer, or inventory
  controls. Put those in `web/src/views/admin/System.vue` or operator docs.
- Do not call anything a "1:1 clone" while runtime side effects, diagnose
  paths, or quota/expiry/reset-flow gaps are undocumented.
- When forward, tunnel, or user-tunnel behavior changes, update
  `docs/guide/flux-panel-clone.md`, `docs/guide/flux-forward-contract.md`, and
  `docs/guide/api-reference.md` in the same PR; refresh
  `docs/control-boundary.md` if user-facing entry points change.
- Validate with `GOWORK=off go test ./internal/router ./internal/handler ./internal/service`
  and `cd web && npm run build`.

## Proprietary Runtime And Dual-Mode Rules

- The Flux clone is the public control plane. The execution plane is an
  internal differentiator: `internal/service/forward_nodex_client.go`,
  `internal/service/forward_panel_runtime_service.go`,
  `internal/service/forward_runtime_*.go`, `install.sh`, `panel_install.sh`,
  `config/deploy/ansible/`, and `GET /api/v2/admin/forward/runtime/jobs`.
- `forward_runtime.backend` in `config/config.yaml` is persisted as the system
  config key `forward.runtime_backend`:
  - `gost`: NodeX mode. Stateful; requires `forward_runtime.nodex.base_url`
    and `forward_runtime.nodex.token`. Entry/ingress-node language belongs only
    to this mode. `ForwardNode.api_token` is the relay gost API credential, not
    the NodeX token.
  - `nftables_ansible` (recommended local ansible mode): stateless; executor on
    the panel host plus SSH inventory and playbooks from
    `forward_runtime.nftables_ansible`. No ingress node.
  - `iptables_ansible`: legacy value, normalized to `nftables_ansible`.
  - `clean_agent`: clean-room pull agent (`docs/forward-clean-room/`).
  - optional `forward_runtime.nodex_mode` forces `gost` (true) or local ansible (false).
- SSH credentials come from the ansible inventory, never from `ForwardNode`
  fields. `/admin/forward/nodes` "online" means only `host:port` TCP
  reachability, not gost, NodeX, or SSH health.
- `Node` (`/admin/nodes`, proxy service) and `ForwardNode`
  (`/admin/forward/nodes`, forward execution) are different resources; never
  mix them in copy, validation, or docs.
- When the execution surface changes, update together:
  `docs/guide/forward-relay-onboarding.md` (source of truth for "relay really
  joined"), `docs/guide/nodex-internal-extension.md`,
  `docs/guide/forward-tunnel-runtime-ops.md`,
  `docs/guide/forward-tunnel-smoke-test.md`, and `docs/reference/runtime.md`.
- In PR descriptions, state whether a change affects the Flux-compatible
  control plane, NodeX runtime behavior, or ansible execution behavior.

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
- Its CI lives in `.github/workflows/control-center.yml` and `control-center-workers.yml` (path-filtered to `control-center/**`), and its releases in `control-center-release.yml`. Release tags are `control-center-v*`. Never let a Center release become the repository's latest release, because `scripts/install.sh` reads `releases/latest`.
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
  sweep start with `web/src/layouts/AdminLayout.vue`,
  `web/src/components/admin/ForwardSuiteNav.vue`, and the forward pages in
  `web/src/views/admin/` (`Forward.vue`, `Tunnel.vue`, `Limit.vue`, `ForwardNodes.vue`).
- 对话框关闭按钮统一使用 `×` 或 `✕`。
- In change notes, separate terminal display problems from real corruption.
