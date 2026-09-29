# Repository Layout

This page is the single source of truth for what belongs where in
`anix-control`. The root [`README.md`](../../README.md) and
[`AGENTS.md`](../../AGENTS.md) link here instead of repeating the layout.

## Root Principles

The root should stay comparable to NodeX:

- short README
- source directories
- deployment entrypoints
- contributor/agent instructions

It should not become a dumping ground for:

- generated logs
- temporary smoke output
- ad-hoc YAML examples
- scratch notes, agent plans, or agent-local state

## Root Ownership

| Path | Purpose |
|------|------|
| `cmd/` | Go entrypoints: `server` (the Control binary), `migrate` (XBoard MySQL importer), `sqlite2postgres` (database migration and dry run), `wgrotate` (WireGuard peer key rotation) |
| `internal/` | backend implementation: `router`, `handler`, `service`, `model`, `middleware`, `database`, `config`, `grpc`, `agentws`, `websocket`, `payment`, `parser`, plugin kernel (`plugincontrol`, `pluginhost`, `packagebridge`, `identitybridge`), `panicrecovery` (gRPC panic recovery interceptors), and `tests/` (`e2e`, `smoke`, `integration`, `testutil`) |
| `api/` | wire contracts and generated code: `grpc/` (legacy panel-node `v2board.proto` / `v2boardpb`), `pluginhost/v1` (kernel to package host), `packagebridge/v1` (package host back into kernel bridge operations); regenerate with each directory's `gen.sh` |
| `pkg/` | importable SDKs for package host processes: `pluginhostsdk` (serve the host protocol) and `packagebridgesdk` (call kernel bridge operations) |
| `packages/` | signed plugin package sources: one directory per official package (`identity-platform`, `subscription`, `plan`, `order`, `payment`, `ticket`, `knowledge`, `notification`, `proxy-node`, `forward`, `machine-telemetry`, `nftables-forward`, `gost-mesh`, `nat-egress`, `wireguard`, `protocol-runtime`) plus `shared/` (package builder, manifest schema, shared control host) |
| `contracts/` | cross-repository golden fixtures shared with `anix-agent`: `plugin/v1` manifests and `agent/v1` operation envelopes |
| `web/` | Vue 3 + Vite admin/user frontend: `src/`, Vitest tests in `src/__tests__/`, Playwright specs in `e2e/`, helper scripts in `scripts/` |
| `control-center/` | Control Center app imported from the archived `Anixops-control-center` repository: its own Go module (`github.com/AnixOps/anix-control/control-center`), Vue web, Flutter client, Helm/deploy/monitoring assets, and the Cloudflare Workers API under `control-center/workers/`; built by `.github/workflows/control-center*.yml` |
| `config/` | configuration templates (`config.yaml.example`, `config.dev.yaml.example`, `config.prod.yaml`), the `/api/v2` package route catalog (`v2-package-route-catalog.json`), `deploy/`, `docker/`, and `scripts/` (see below) |
| `scripts/` | operator-facing scripts: release installer `install.sh`, `install-agent.sh`, `manage.sh`, SQLite/PostgreSQL migration and restore rehearsal, plugin release signing, WireGuard helpers, legacy panel upgrade |
| `docs/` | documentation tree (see below) plus generated Swagger output (`docs.go`, `swagger.json`, `swagger.yaml`) |
| `.github/` | CI (`workflows/ci.yml`), SDK sync, Control Center workflows, `CODEOWNERS`, and the branch ruleset description in `BRANCH_PROTECTION.md` |
| `install.sh` / `panel_install.sh` | one-click Docker/bootstrap installer for the internal forward runtime (`panel_install.sh` is a wrapper) |
| `Dockerfile` / `docker-compose.yml` / `docker-compose.prod.yml` | container build and startup |
| `Makefile` | local build, run, test, lint, swagger, and gRPC generation targets |
| `tools.go` | pins the protoc Go plugins and records the expected `protoc` versions |
| `README.md` | canonical repository entrypoint |
| `AGENTS.md` | contributor/agent rules |
| `ROADMAP.md` / `TODO.md` / `CHANGELOG.md` | roadmap pointer, open backlog, and change history |

## `scripts/` Versus `config/scripts/`

- `scripts/` holds scripts an operator runs on a host (install, manage,
  migrate, sign, rehearse).
- `config/scripts/` holds repository gates and release tooling that CI runs:
  documentation sync (`check_docs_updated.sh`), release workflow policy
  (`check_release_workflow.sh`), release version surfaces
  (`check_release_version.py`), release stage contract, `/api/v2` package route
  catalog and plugin-only route checks, Agent SDK dependency check, release
  manifest/notes generators, artifact and V4 evidence verifiers, and the
  guarded legacy `deploy.sh`/`pre-deploy.sh` entrypoints.

## `config/deploy/` And `config/docker/`

- `config/deploy/ansible/`: bundled ansible inventory examples and forward
  apply/remove/stats playbooks (nftables and legacy iptables) for the local
  ansible runtime; `nodes/` holds node deployment playbooks.
- `config/deploy/ssh/`: mount point for the ansible SSH key (contents ignored).
- `config/deploy/grpc_tls/`: certbot helper for the gRPC listener.
- `config/deploy/deploy_panel.sh`, `clean_local_build_artifacts.sh`,
  `check_release_build_policy.sh`: guarded local deploy, artifact cleanup, and
  the release-build policy gate.
- `config/docker/`: Nginx and Prometheus configuration mounted by
  `docker-compose.prod.yml`.

The Docker image copies all of `config/deploy/` into `/app/config/deploy`. Do
not reintroduce a top-level `deploy/` directory.

## Documentation Layout

| Path | Purpose |
|------|------|
| `docs/README.md` | documentation landing page |
| `docs/intro/` | repository role, v4 architecture at a glance, runtime-mode semantics |
| `docs/reference/` | startup, configuration, runtime, migrations, and this layout page |
| `docs/guide/` | implementation deep dives, Flux clone contract, runtime operations, release installation, upgrade/rollback runbooks |
| `docs/architecture/` | plugin kernel contract and release-line status |
| `docs/audit/` | security, concurrency, performance, test-gap, and repository audit registers |
| `docs/forwarding/` | forwarding module design, API, security, and compatibility |
| `docs/forward-clean-room/` | clean-room forward agent spec and provenance |
| `docs/*.md` | feature register (`features.md`), deployment, upgrade, RC roadmap/evidence, Control Center merge record, brand migration, manual intervention |

## Generated Artifacts

Generated artifacts should go to dedicated locations or stay ignored:

- logs: `logs/`
- test reports: `test-reports/`
- frontend output: `web/public/`, `web/coverage/`, `web/playwright-report/`
- local temporary output: ignored `tmp_*`, `.codex_*`, and similar scratch files
- agent-local state: `.claude/settings.local.json`, `.claude/projects/`, `.superpowers/` (ignored)

If you create new tooling that emits artifacts, do not write them into the root
by default. Use `bash config/deploy/clean_local_build_artifacts.sh --dry-run`
to find leftovers.

## Related Docs

- [../control-boundary.md](../control-boundary.md)
- [startup-config.md](startup-config.md)
- [../intro/README.md](../intro/README.md)
- [../guide/README.md](../guide/README.md)
