# V4 Plugin-Only Stable Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship AnixOps Control v4.0.0 as a signed, external-process plugin platform with all fifteen required packages, stable supported /api/v2 contracts, and a zero-downtime reversible in-place upgrade path.

**Architecture:** Control keeps only platform-kernel duties and dispatches every package-owned business request to a supervised local Control Plugin Host over a Unix-domain gRPC socket. Package artifacts own their migrations, compatibility projections, routes, WebUI, and optional Agent entrypoints; the Agent remains a separately versioned repository and exposes only a tagged SDK contract to Control. Migration and traffic changes are additive, generation-fenced, idempotent, and progressed through 1/5/25/100 percent cohorts with a retained rollback generation.

**Tech Stack:** Go 1.25, gRPC/protobuf, Gin, GORM, SQLite, PostgreSQL, Ed25519 package signatures, existing github.com/AnixOps/anix-agent/sdk module, Python release scripts, Vite/TypeScript package WebUI bundles, GitHub Actions release evidence.

## Global Constraints

- Product release version is v4.0.0; Control module path stays github.com/AnixOps/anix-control/v4 and Agent module path stays github.com/AnixOps/anix-agent/v4.
- All fifteen required package ids are subscription, proxy-node, plan, order, payment, forward, ticket, notification, knowledge, machine-telemetry, nftables-forward, gost-mesh, nat-egress, wireguard, and protocol-runtime.
- A package is the only owner of its domain behavior, domain migrations, compatibility projection, background work, and WebUI. Kernel code may not import a package domain service or query a package-owned table.
- Supported /api/v2 path, method, authentication semantics, request fields, response envelope, and documented error codes remain stable. The handler must enter a package compatibility adapter and never fall back to coupled legacy domain code.
- Control packages are separately supervised local processes. They are not network listeners and receive only authenticated, authorized, deadline-bound requests over a permission-restricted Unix socket.
- Every external package artifact, manifest, WebUI bundle, migration digest, and runtime entrypoint is signed and verified with the configured official Ed25519 trust root before activation.
- Every mutation carries a stable request identity and idempotency key, is audited, and is fenced by the package route generation.
- Database work is additive until verified backup, validation, reverse migration, and the 72-hour canary window have passed. No hand edits to database rows, package files, or route state are part of an upgrade.
- Rollout is 1, 5, 25, then 100 percent of a configured cohort. Host failure, lease loss, validation mismatch, incompatible response, duplicate non-idempotent mutation, health failure, or configured error/latency breach halts expansion and returns the cohort to the previous verified generation.
- Both SQLite and PostgreSQL must pass clean bootstrap, v3-to-v4 upgrade, backup/restore, and full reverse-upgrade rehearsals.
- Agent changes are developed in ../anix-agent and published as a tagged github.com/AnixOps/anix-agent/sdk module. Control consumes the released SDK version; neither repository is a git submodule of the other.
- Do not create the formal v4.0.0 tag until all machine-readable evidence, a 72-hour production canary, explicit operator approval, and GitHub Support confirmation that PR #4's read-only ref and caches are purged are available.

---

## File Structure

- api/pluginhost/v1/control_host.proto: versioned Control-to-package host RPC wire contract.
- api/pluginhost/gen.sh: reproducible protobuf generation command for the Control host API.
- pkg/pluginhostsdk/: public package-side server interfaces and response validation shared by signed package binaries.
- internal/pluginhost/: kernel-side artifact inspection, Unix-socket process supervision, gRPC client, health leases, and generation-aware dispatch.
- internal/compat/v2/: declarative legacy-route matching and response-envelope translation with no domain database access.
- internal/model/plugin_rollout.go and internal/service/plugin_rollout.go: generic kernel-only migration-run, validation, backup-reference, and route-generation records.
- packages/<package-id>/: signed package source, manifest, package-local SQL migrations, compatibility route declarations, WebUI bundle, and optional Agent entrypoint.
- ../anix-agent/sdk/plugincontrol/: versioned SDK contract for Agent-target package operations and observed runtime reports.
- ../anix-agent/plugin/: Agent manifest validation and supervisor support for the new declared runtime entrypoints.
- config/scripts/: static plugin-only, package completeness, migration rehearsal, artifact, and release-evidence gates.
- docs/guide/: production upgrade, rollback, and operator evidence procedures.

### Task 1: Freeze the cross-repository package and Agent SDK boundary

**Files:**
- Create: ../anix-agent/sdk/plugincontrol/protocol.go
- Create: ../anix-agent/sdk/plugincontrol/protocol_test.go
- Create: ../anix-agent/sdk/plugincontrol/compatibility_test.go
- Modify: ../anix-agent/sdk/README.md
- Modify: ../anix-agent/sdk/scripts/check-breaking.sh
- Modify: go.mod
- Modify: go.sum
- Test: internal/grpc/agent_plugin_package_cross_repo_e2e_test.go

**Interfaces:**
- Produces: agentplugin.Operation, agentplugin.RuntimeStatus, and agentplugin.ValidateOperation for every Control-to-Agent package operation.
- Consumes: the existing anix.agent.v1 DesiredOperation stream and agent.OperationEnvelope fields.
- Version: release the SDK as v1.1.0 before Control changes its dependency from v1.0.0.

- [ ] **Step 1: Write the SDK contract tests**

~~~go
func TestValidateOperationRejectsGenerationRegression(t *testing.T) {
    operation := Operation{
        ID: "op-42", PackageID: "wireguard", PackageVersion: "4.0.0",
        Generation: 9, IdempotencyKey: "key-42", Kind: "configure",
    }
    require.NoError(t, ValidateOperation(operation))

    operation.Generation = 0
    require.ErrorContains(t, ValidateOperation(operation), "generation")
}
~~~

- [ ] **Step 2: Run the SDK test to verify it fails**

Run: cd ../anix-agent/sdk && GOWORK=off go test ./plugincontrol -run TestValidateOperationRejectsGenerationRegression -count=1
Expected: FAIL because package plugincontrol does not exist.

- [ ] **Step 3: Implement the versioned SDK contract**

~~~go
package plugincontrol

type Operation struct {
    ID             string
    PackageID      string
    PackageVersion string
    Generation     uint64
    IdempotencyKey string
    Kind           string
    ConfigJSON     []byte
}

type RuntimeStatus struct {
    PackageID      string
    PackageVersion string
    Generation     uint64
    Healthy        bool
    ObservedHash   string
    FailureCode    string
}

func ValidateOperation(operation Operation) error {
    if operation.ID == "" || operation.PackageID == "" || operation.PackageVersion == "" ||
        operation.Generation == 0 || operation.IdempotencyKey == "" || operation.Kind == "" {
        return errors.New("plugin operation has required empty fields")
    }
    if !json.Valid(operation.ConfigJSON) {
        return errors.New("plugin operation config is not valid JSON")
    }
    return nil
}
~~~

Document that v1.1.0 adds fields only and retains the v1.0.0 Agent Control protobuf. Update the breaking-change script to run the new package tests and tag the SDK release before changing Control's go.mod.

- [ ] **Step 4: Verify SDK compatibility and Control consumption**

Run: cd ../anix-agent/sdk && ./scripts/check-breaking.sh && GOWORK=off go test ./...
Expected: exit 0.

Run: cd ../../anix-control && GOWORK=off go get github.com/AnixOps/anix-agent/sdk@v1.1.0 && GOWORK=off go test ./internal/grpc -run TestAgentPluginPackageCrossRepo -count=1
Expected: exit 0 and the test reports the v1.1.0 package operation contract.

- [ ] **Step 5: Commit the independent repository changes**

~~~bash
cd ../anix-agent
git add sdk
git commit -m "feat(sdk): add v4 package operation contract"
git tag -a sdk/v1.1.0 -m "Anix Agent SDK v1.1.0"
git push origin dev_new sdk/v1.1.0

cd ../anix-control
git add go.mod go.sum internal/grpc/agent_plugin_package_cross_repo_e2e_test.go
git commit -m "build: consume agent sdk v1.1.0"
~~~

### Task 2: Define and generate the Control Plugin Host RPC

**Files:**
- Create: api/pluginhost/v1/control_host.proto
- Create: api/pluginhost/gen.sh
- Create: api/pluginhost/v1/control_host.pb.go
- Create: api/pluginhost/v1/control_host_grpc.pb.go
- Create: pkg/pluginhostsdk/server.go
- Create: pkg/pluginhostsdk/server_test.go
- Test: api/pluginhost/v1/control_host_contract_test.go

**Interfaces:**
- Produces: pluginhostv1.ControlPackageHostClient with Dispatch, Migrate, Health, and Drain RPCs.
- Produces: pluginhostsdk.Package implementing Dispatch(context.Context, DispatchRequest), Migrate(context.Context, MigrationRequest), Health(context.Context), and Drain(context.Context).
- Consumes: a kernel-authenticated principal projection, route id, request id, idempotency key, deadline, and route generation.
- Does not expose: JWTs, password hashes, unrestricted database DSNs, or arbitrary network listener configuration.

- [ ] **Step 1: Write the protobuf descriptor contract test**

~~~go
func TestControlHostDescriptorContainsOnlyLocalDispatchRPCs(t *testing.T) {
    service := File_api_pluginhost_v1_control_host_proto.Services().ByName("ControlPackageHost")
    require.NotNil(t, service)
    require.NotNil(t, service.Methods().ByName("Dispatch"))
    require.NotNil(t, service.Methods().ByName("Migrate"))
    require.Nil(t, service.Methods().ByName("Listen"))
}
~~~

- [ ] **Step 2: Run the descriptor test to verify it fails**

Run: GOWORK=off go test ./api/pluginhost/v1 -run TestControlHostDescriptorContainsOnlyLocalDispatchRPCs -count=1
Expected: FAIL because the generated host API is absent.

- [ ] **Step 3: Add the RPC schema and package SDK**

~~~proto
syntax = "proto3";
package anix.pluginhost.v1;
option go_package = "github.com/AnixOps/anix-control/v4/api/pluginhost/v1;pluginhostv1";

service ControlPackageHost {
  rpc Dispatch(DispatchRequest) returns (DispatchResponse);
  rpc Migrate(MigrationRequest) returns (MigrationResponse);
  rpc Health(HealthRequest) returns (HealthResponse);
  rpc Drain(DrainRequest) returns (DrainResponse);
}

message DispatchRequest {
  string package_id = 1;
  string package_version = 2;
  uint64 route_generation = 3;
  string request_id = 4;
  string idempotency_key = 5;
  string route_id = 6;
  string method = 7;
  bytes request_body = 8;
  bytes principal_json = 9;
  int64 deadline_unix_millis = 10;
}
message DispatchResponse {
  uint32 status_code = 1;
  bytes response_body = 2;
  repeated Header headers = 3;
  string operation_id = 4;
  string failure_code = 5;
}
message Header { string name = 1; string value = 2; }
message MigrationRequest {
  string package_id = 1;
  string package_version = 2;
  string migration_id = 3;
  string checkpoint = 4;
  uint64 route_generation = 5;
}
message MigrationResponse {
  string checkpoint = 1;
  string validation_digest = 2;
  bool complete = 3;
  string failure_code = 4;
}
message HealthRequest { uint64 route_generation = 1; }
message HealthResponse { bool healthy = 1; string lease_id = 2; string details_json = 3; }
message DrainRequest { uint64 route_generation = 1; int64 deadline_unix_millis = 2; }
message DrainResponse { bool drained = 1; uint64 in_flight = 2; }
~~~

Generate code with api/pluginhost/gen.sh using protoc and protoc-gen-go versions pinned in tools.go. In pkg/pluginhostsdk/server.go reject a package id/version mismatch, expired deadline, zero generation, invalid JSON principal, response status outside 100..599, and response larger than the configured host response limit.

- [ ] **Step 4: Run generated-contract and SDK tests**

Run: ./api/pluginhost/gen.sh && GOWORK=off go test ./api/pluginhost/v1 ./pkg/pluginhostsdk -count=1
Expected: exit 0.

- [ ] **Step 5: Commit the host wire contract**

~~~bash
git add api/pluginhost pkg/pluginhostsdk tools.go
git commit -m "feat(plugin): add control package host rpc"
~~~

### Task 3: Replace in-process Control execution with supervised Unix-socket hosts

**Files:**
- Create: internal/pluginhost/artifact.go
- Create: internal/pluginhost/manager.go
- Create: internal/pluginhost/process.go
- Create: internal/pluginhost/client.go
- Create: internal/pluginhost/manager_test.go
- Create: internal/pluginhost/client_test.go
- Modify: internal/config/config.go
- Modify: config/config.yaml.example
- Modify: config/config.prod.yaml
- Modify: config/config.dev.yaml.example
- Modify: internal/handler/kernel.go
- Modify: internal/plugincontrol/registry.go
- Modify: internal/plugincontrol/operation_worker.go
- Modify: cmd/server/main.go
- Test: internal/handler/kernel_test.go

**Interfaces:**
- Produces: pluginhost.Manager.Dispatch(context.Context, DispatchInput) returning pluginhost.DispatchOutput.
- Produces: pluginhost.Manager.Start, Health, Drain, Stop, and Rollback for an installed Control package version.
- Consumes: a verified service.PluginControlRouteResolution and a verified immutable artifact path.
- Replaces: plugincontrol.Registry.ExecuteRoute and ExecuteLifecycle as live execution paths. Registry may remain only as a fixture adapter until its callers are removed.

- [ ] **Step 1: Write failing host isolation and fail-closed tests**

~~~go
func TestManagerRejectsDispatchWhenLeaseDoesNotMatchGeneration(t *testing.T) {
    manager := newTestManager(t, hostWithGeneration("knowledge", "4.0.0", 7))
    _, err := manager.Dispatch(context.Background(), DispatchInput{
        PackageID: "knowledge", Version: "4.0.0", Generation: 6, RouteID: "knowledge.article.list",
    })
    require.ErrorIs(t, err, ErrGenerationUnavailable)
}

func TestPluginRouteGatewayDoesNotUseRegistryFallback(t *testing.T) {
    response := performPackageRequest(t, "GET", "/api/v4/plugins/knowledge/articles")
    require.Equal(t, http.StatusBadGateway, response.Code)
    require.Contains(t, response.Body.String(), "plugin_host_unavailable")
}
~~~

- [ ] **Step 2: Run the tests to verify they fail**

Run: GOWORK=off go test ./internal/pluginhost ./internal/handler -run 'TestManagerRejectsDispatchWhenLeaseDoesNotMatchGeneration|TestPluginRouteGatewayDoesNotUseRegistryFallback' -count=1
Expected: FAIL because Manager and plugin_host_unavailable do not exist.

- [ ] **Step 3: Implement process lifecycle and kernel dispatch**

~~~go
type DispatchInput struct {
    PackageID      string
    Version        string
    Generation     uint64
    RequestID      string
    IdempotencyKey string
    RouteID        string
    Method         string
    Body           []byte
    PrincipalJSON  []byte
    Deadline       time.Time
}

type Manager interface {
    Start(context.Context, ArtifactRef, uint64) error
    Dispatch(context.Context, DispatchInput) (DispatchOutput, error)
    Health(context.Context, string, string, uint64) (HostHealth, error)
    Drain(context.Context, string, string, uint64, time.Time) error
    Rollback(context.Context, string, string, uint64) error
}
~~~

Start each package through exec.CommandContext with a private directory mode of 0700 and socket mode of 0600. The manager verifies artifact digest, entrypoint digest, manifest id/version, control target, and installed generation before start. Use grpc.DialContext with a Unix dialer; do not bind a TCP listener. Update KernelHandler to call Manager.Dispatch after ResolvePluginControlRoute, translate only typed host failures to public errors, and return plugin_host_unavailable or plugin_host_incompatible without calling the legacy handler or Registry.

- [ ] **Step 4: Run lifecycle, handler, and race tests**

Run: GOWORK=off go test -race ./internal/pluginhost ./internal/handler ./internal/plugincontrol -count=1
Expected: exit 0.

- [ ] **Step 5: Commit the supervised host cutover**

~~~bash
git add internal/pluginhost internal/config/config.go config/config.yaml.example config/config.prod.yaml config/config.dev.yaml.example internal/handler/kernel.go internal/plugincontrol/registry.go internal/plugincontrol/operation_worker.go cmd/server/main.go
git commit -m "feat(plugin): supervise control package hosts"
~~~

### Task 4: Extend signed manifests and materialize all fifteen package artifacts

**Files:**
- Create: packages/shared/build_package.py
- Create: packages/shared/manifest_schema.json
- Create: packages/shared/tests/test_manifest_schema.py
- Create: packages/subscription/manifest.template.json
- Create: packages/proxy-node/manifest.template.json
- Create: packages/plan/manifest.template.json
- Create: packages/order/manifest.template.json
- Create: packages/payment/manifest.template.json
- Create: packages/forward/manifest.template.json
- Create: packages/ticket/manifest.template.json
- Create: packages/notification/manifest.template.json
- Create: packages/knowledge/manifest.template.json
- Create: packages/wireguard/manifest.template.json
- Create: packages/protocol-runtime/manifest.template.json
- Modify: packages/machine-telemetry/manifest.template.json
- Modify: packages/nftables-forward/manifest.template.json
- Modify: packages/gost-mesh/manifest.template.json
- Modify: packages/nat-egress/manifest.template.json
- Modify: internal/service/kernel_service.go
- Modify: ../anix-agent/plugin/supervisor.go
- Create: ../anix-agent/plugin/manifest_test.go
- Modify: config/scripts/release-stage-contract.json
- Test: config/scripts/check_release_stage.py

**Interfaces:**
- Produces: manifest api_version v2 fields control_entrypoint, agent_entrypoint, migrations, compatibility_routes, route_contract_digest, and runtime_api_version.
- Consumes: existing signature verification and package artifact storage functions.
- Package target map: Control only for subscription, plan, order, payment, ticket, notification, and knowledge; Control plus Agent for proxy-node, forward, machine-telemetry, nftables-forward, gost-mesh, nat-egress, wireguard, and protocol-runtime.

- [ ] **Step 1: Write failing package-completeness tests**

~~~python
def test_v4_required_packages_have_signed_host_metadata(repo_root):
    contract = load_contract(repo_root / "config/scripts/release-stage-contract.json")
    decision = validate_stage(repo_root, contract, "v4.0.0")
    assert decision.stage_id == "4.0"
    for requirement in decision.required_packages:
        manifest = load_manifest(repo_root / requirement["manifest"])
        assert manifest["api_version"] == "v2"
        assert manifest["control_entrypoint"]["path"]
        assert manifest["migrations"]["index"]
        assert manifest["route_contract_digest"]
~~~

- [ ] **Step 2: Run the release-stage check to verify it fails**

Run: python3 config/scripts/check_release_stage.py --tag v4.0.0
Expected: FAIL because one or more required package manifests or v2 host fields are missing.

- [ ] **Step 3: Define the common package manifest and build matrix**

~~~json
{
  "id": "knowledge",
  "version": "4.0.0",
  "api_version": "v2",
  "publisher": "AnixOps",
  "targets": ["control"],
  "control_entrypoint": {
    "path": "bin/control-host",
    "sha256": "GENERATED_AT_BUILD"
  },
  "migrations": {
    "index": "migrations/index.json",
    "sha256": "GENERATED_AT_BUILD"
  },
  "compatibility_routes": {
    "path": "compat/v2-routes.json",
    "sha256": "GENERATED_AT_BUILD"
  },
  "route_contract_digest": "GENERATED_AT_BUILD",
  "webui": {
    "bundle": {"path": "webui/index.mjs", "sha256": "GENERATED_AT_BUILD"},
    "menus": [{"id": "knowledge", "parent": "content", "route": "/plugins/knowledge"}],
    "routes": [{"id": "knowledge.index", "path": "/plugins/knowledge", "export": "mount"}]
  }
}
~~~

The shared builder must replace generated fields only after deterministic archive ordering, calculate all digests from the final bytes, sign the canonical manifest, and emit an SBOM. Refactor all four existing package builders to use it. Move Agent Manifest from plugin/supervisor.go into plugin/manifest.go, accept api_version v2 only when the declared Agent entrypoint and runtime_api_version match the SDK v1.1.0 contract, and retain api_version v1 validation for installed legacy packages during the migration window.

- [ ] **Step 4: Build, verify, and stage every package**

Run: python3 packages/shared/build_package.py --all --version 4.0.0 --out dist/packages
Expected: 15 signed .anxp artifacts and 15 SBOM files.

Run: python3 config/scripts/check_release_stage.py --tag v4.0.0 && GOWORK=off go test ./internal/service && cd ../anix-agent && GOEXPERIMENT=jsonv2 GOWORK=off go test ./plugin
Expected: exit 0.

- [ ] **Step 5: Commit manifests, package builders, and Agent validation**

~~~bash
git add packages internal/service/kernel_service.go config/scripts/release-stage-contract.json
git commit -m "feat(packages): materialize v4 package contract"

cd ../anix-agent
git add plugin
git commit -m "feat(plugin): validate v2 package manifests"
~~~

### Task 5: Add generic migration, validation, backup, and rollout generation records

**Files:**
- Create: internal/model/plugin_rollout.go
- Create: internal/model/plugin_rollout_test.go
- Create: internal/service/plugin_rollout.go
- Create: internal/service/plugin_rollout_test.go
- Create: internal/pluginhost/migration.go
- Create: internal/pluginhost/migration_test.go
- Modify: internal/model/kernel.go
- Modify: internal/service/kernel_service.go
- Modify: internal/pluginhost/manager.go
- Test: internal/tests/integration/plugin_rollout_test.go

**Interfaces:**
- Produces: model.PackageMigrationRun, model.PackageValidationResult, model.PackageRouteGeneration, and model.PackageBackupReference.
- Produces: service.BeginPackageMigration, RecordMigrationCheckpoint, RecordPackageValidation, AdvancePackageCohort, and RollbackPackageGeneration.
- Consumes: package-supplied opaque checkpoint and validation digest; the kernel never computes a domain mapping or queries a package-owned table.

- [ ] **Step 1: Write migration idempotency and rollback tests**

~~~go
func TestAdvancePackageCohortRequiresVerifiedPriorGeneration(t *testing.T) {
    generation := seedGeneration(t, "order", 4, 5, "validated")
    _, err := service.AdvancePackageCohort(db, generation.ID, 25)
    require.NoError(t, err)

    _, err = service.AdvancePackageCohort(db, generation.ID, 100)
    require.ErrorIs(t, err, service.ErrCohortTransition)
}

func TestRollbackRestoresPreviousVerifiedGeneration(t *testing.T) {
    current, previous := seedRollbackPair(t, "payment")
    restored, err := service.RollbackPackageGeneration(db, current.ID, "host_crash")
    require.NoError(t, err)
    require.Equal(t, previous.ID, restored.ID)
}
~~~

- [ ] **Step 2: Run the tests to verify they fail**

Run: GOWORK=off go test ./internal/model ./internal/service ./internal/pluginhost -run 'TestAdvancePackageCohortRequiresVerifiedPriorGeneration|TestRollbackRestoresPreviousVerifiedGeneration' -count=1
Expected: FAIL because package rollout models and services are absent.

- [ ] **Step 3: Implement generic, additive records**

~~~go
type PackageRouteGeneration struct {
    ID             uint
    PackageID      string
    Version        string
    Generation     uint64
    CohortPercent  uint8
    State          string
    PreviousID     *uint
    ActivatedAt    *time.Time
    RolledBackAt   *time.Time
}

func AdvancePackageCohort(db *gorm.DB, generationID uint, target uint8) (*model.PackageRouteGeneration, error) {
    if target != 1 && target != 5 && target != 25 && target != 100 {
        return nil, ErrCohortTransition
    }
    // Lock the generic generation row, require a validated predecessor,
    // persist the new cohort, and return the immutable new route generation.
}
~~~

Use v4_kernel_package_migration_run, v4_kernel_package_validation_result, v4_kernel_package_route_generation, and v4_kernel_package_backup_reference table names. Store migration checksum, package version, before/after schema version, opaque checkpoint, validation digest, backup reference, previous generation, and rollback reason. Have Manager.Migrate persist checkpoints after each host reply and refuse route activation until the host health lease matches the requested generation.

- [ ] **Step 4: Run SQLite, PostgreSQL, and interruption tests**

Run: GOWORK=off go test ./internal/model ./internal/service ./internal/pluginhost ./internal/tests/integration -run 'Test.*Package(Migration|Cohort|Rollback)' -count=1
Expected: exit 0 on SQLite.

Run: ANIX_TEST_POSTGRES_DSN="$ANIX_TEST_POSTGRES_DSN" GOWORK=off go test ./internal/tests/integration -run TestPostgresPackageMigrationRollback -count=1
Expected: exit 0 when the PostgreSQL test DSN is supplied.

- [ ] **Step 5: Commit generic rollout orchestration**

~~~bash
git add internal/model/plugin_rollout.go internal/model/plugin_rollout_test.go internal/service/plugin_rollout.go internal/service/plugin_rollout_test.go internal/pluginhost/migration.go internal/pluginhost/migration_test.go internal/model/kernel.go internal/service/kernel_service.go internal/pluginhost/manager.go internal/tests/integration/plugin_rollout_test.go
git commit -m "feat(kernel): add package migration rollout generations"
~~~

### Task 6: Route every supported v2 business endpoint through package compatibility adapters

**Files:**
- Create: internal/compat/v2/registry.go
- Create: internal/compat/v2/gateway.go
- Create: internal/compat/v2/envelope.go
- Create: internal/compat/v2/registry_test.go
- Create: internal/compat/v2/gateway_test.go
- Create: internal/compat/v2/contract_test.go
- Modify: internal/router/router.go
- Modify: internal/handler/kernel.go
- Create: config/scripts/check_plugin_only_routes.py
- Create: config/scripts/check_plugin_only_routes_test.py
- Test: internal/router/router_test.go

**Interfaces:**
- Produces: compatv2.Registry.Resolve(method, path) and compatv2.Gateway.Serve.
- Consumes: signed package compatibility_routes declarations and pluginhost.Manager.Dispatch.
- Prohibits: direct calls from a supported v2 business route to a legacy handler, model query, service, or worker.

- [ ] **Step 1: Write response-compatibility and no-fallback tests**

~~~go
func TestV2KnowledgeListUsesPackageEnvelope(t *testing.T) {
    response := requestV2(t, "GET", "/api/v2/user/knowledge")
    require.Equal(t, http.StatusOK, response.Code)
    require.JSONEq(t, `{"data":{"items":[]}}`, response.Body.String())
    require.Equal(t, "knowledge.article.list", testHost(t).LastRouteID())
}

func TestV2GatewayFailsClosedWhenPackageIsDisabled(t *testing.T) {
    disablePackage(t, "knowledge")
    response := requestV2(t, "GET", "/api/v2/user/knowledge")
    require.Equal(t, http.StatusServiceUnavailable, response.Code)
    require.NotContains(t, response.Body.String(), "legacy")
}
~~~

- [ ] **Step 2: Run the v2 tests to verify they fail**

Run: GOWORK=off go test ./internal/compat/v2 ./internal/router -run 'TestV2KnowledgeListUsesPackageEnvelope|TestV2GatewayFailsClosedWhenPackageIsDisabled' -count=1
Expected: FAIL because the compatibility gateway is absent.

- [ ] **Step 3: Implement declarative route lookup and envelope translation**

~~~go
type Route struct {
    Method       string
    LegacyPath   string
    PackageID    string
    PackageRoute string
    Envelope     string
}

type Gateway struct {
    Registry   Registry
    Dispatcher Dispatcher
}

func (g Gateway) Serve(c *gin.Context) {
    route, ok := g.Registry.Resolve(c.Request.Method, c.FullPath())
    if !ok {
        writeV2Error(c, http.StatusNotFound, "not_found")
        return
    }
    output, err := g.Dispatcher.Dispatch(c.Request.Context(), DispatchInput{
        PackageID: route.PackageID, RouteID: route.PackageRoute,
        Method: c.Request.Method, Body: readBody(c), PrincipalJSON: principalJSON(c),
        RequestID: requestID(c), IdempotencyKey: idempotencyKey(c),
    })
    writeV2Envelope(c, route.Envelope, output, err)
}
~~~

Read mappings only from the verified installed package version. Preserve each legacy middleware chain and endpoint shape in router.go, then replace direct business handler registrations with Gateway.Serve. The static script must parse router registration source and fail if a supported /api/v2 business path references an identifier outside internal/compat/v2 or the generic Kernel handler.

- [ ] **Step 4: Run contract and static gates**

Run: GOWORK=off go test ./internal/compat/v2 ./internal/router ./internal/tests/e2e -count=1
Expected: exit 0.

Run: python3 config/scripts/check_plugin_only_routes.py
Expected: plugin-only route gate passed.

- [ ] **Step 5: Commit the compatibility gateway**

~~~bash
git add internal/compat/v2 internal/router/router.go internal/handler/kernel.go config/scripts/check_plugin_only_routes.py config/scripts/check_plugin_only_routes_test.py
git commit -m "feat(api): route v2 business APIs through packages"
~~~

### Task 7: Move knowledge and notification into read-mostly packages

**Files:**
- Create: packages/knowledge/control/main.go
- Create: packages/knowledge/control/service.go
- Create: packages/knowledge/migrations/001_initial.sql
- Create: packages/knowledge/compat/v2-routes.json
- Create: packages/knowledge/tests/contract_test.go
- Create: packages/notification/control/main.go
- Create: packages/notification/control/service.go
- Create: packages/notification/migrations/001_initial.sql
- Create: packages/notification/compat/v2-routes.json
- Create: packages/notification/tests/contract_test.go
- Modify: internal/tests/e2e/admin_test.go
- Modify: internal/service/telegram_service.go
- Test: internal/tests/e2e/knowledge_notification_plugin_test.go

**Interfaces:**
- Produces: knowledge.article.list, knowledge.article.read, knowledge.admin.write, notification.notice.list, notification.delivery.enqueue, and notification.delivery.retry package routes.
- Package tables: v4_knowledge_article, v4_knowledge_category, v4_notification_notice, v4_notification_delivery, and v4_notification_outbox.
- Consumes: principal projection, package secret lease for outbound delivery, and generic durable operation/route generation services.

- [ ] **Step 1: Write package contract tests**

~~~go
func TestNotificationRetryIsIdempotent(t *testing.T) {
    first := invokePackage(t, "notification", "notification.delivery.retry", `{"delivery_id":"d-1"}`, "retry-d-1")
    second := invokePackage(t, "notification", "notification.delivery.retry", `{"delivery_id":"d-1"}`, "retry-d-1")
    require.Equal(t, first.Body, second.Body)
    require.Equal(t, 1, countRows(t, "v4_notification_outbox"))
}
~~~

- [ ] **Step 2: Run the package tests to verify they fail**

Run: GOWORK=off go test ./packages/knowledge/tests ./packages/notification/tests ./internal/tests/e2e -run 'Test.*(Knowledge|Notification)' -count=1
Expected: FAIL because the package hosts and migrations are absent.

- [ ] **Step 3: Implement package-owned migrations, projections, and routes**

~~~sql
CREATE TABLE v4_knowledge_article (
    id BIGINT PRIMARY KEY,
    category_id BIGINT NOT NULL,
    title TEXT NOT NULL,
    body TEXT NOT NULL,
    visibility TEXT NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

CREATE TABLE v4_notification_outbox (
    id TEXT PRIMARY KEY,
    idempotency_key TEXT NOT NULL UNIQUE,
    payload_json TEXT NOT NULL,
    state TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL
);
~~~

Knowledge migration copies legacy article/category rows into package tables, stores row-count and content-hash checkpoints, and exposes the documented public and admin article envelopes through the adapter. Notification migration imports notices and delivery history, writes delivery state and the package outbox in one transaction, and leases outbound credentials only for the declared notification capability. Remove Telegram domain decisions from Control after the package route handles them.

- [ ] **Step 4: Run migration, contracts, and rollback tests**

Run: GOWORK=off go test ./packages/knowledge/tests ./packages/notification/tests ./internal/compat/v2 ./internal/tests/e2e -count=1
Expected: exit 0.

Run: GOWORK=off go test ./internal/tests/integration -run TestKnowledgeNotificationReverseMigration -count=1
Expected: exit 0.

- [ ] **Step 5: Commit the read-mostly package wave**

~~~bash
git add packages/knowledge packages/notification internal/tests/e2e/admin_test.go internal/service/telegram_service.go internal/tests/e2e/knowledge_notification_plugin_test.go
git commit -m "feat(packages): move knowledge and notifications to hosts"
~~~

### Task 8: Move ticket and plan into package-owned support and catalog hosts

**Files:**
- Create: packages/ticket/control/main.go
- Create: packages/ticket/control/service.go
- Create: packages/ticket/migrations/001_initial.sql
- Create: packages/ticket/compat/v2-routes.json
- Create: packages/ticket/tests/contract_test.go
- Create: packages/plan/control/main.go
- Create: packages/plan/control/service.go
- Create: packages/plan/migrations/001_initial.sql
- Create: packages/plan/compat/v2-routes.json
- Create: packages/plan/tests/contract_test.go
- Modify: internal/handler/handler.go
- Modify: internal/service/service.go
- Test: internal/tests/e2e/ticket_plan_plugin_test.go

**Interfaces:**
- Produces: ticket.list, ticket.message.create, ticket.status.transition, plan.catalog.list, plan.assignment.read, and plan.entitlement.read.
- Package tables: v4_ticket_ticket, v4_ticket_message, v4_plan_catalog, v4_plan_assignment, and v4_plan_quota.
- Consumes: user and access-group identities through the platform identity contract, never through direct user-table ownership.

- [ ] **Step 1: Write support-state and entitlement tests**

~~~go
func TestTicketStatusTransitionIsGenerationFenced(t *testing.T) {
    result := invokeAtGeneration(t, "ticket", 12, "ticket.status.transition", `{"ticket_id":7,"to":"closed"}`)
    require.Equal(t, http.StatusOK, result.Status)

    stale := invokeAtGeneration(t, "ticket", 11, "ticket.status.transition", `{"ticket_id":7,"to":"open"}`)
    require.Equal(t, http.StatusConflict, stale.Status)
}
~~~

- [ ] **Step 2: Run the tests to verify they fail**

Run: GOWORK=off go test ./packages/ticket/tests ./packages/plan/tests ./internal/tests/e2e -run 'Test.*(Ticket|Plan)' -count=1
Expected: FAIL because ticket and plan host routes are absent.

- [ ] **Step 3: Implement the package contracts**

~~~go
type EntitlementReader interface {
    ReadEntitlement(context.Context, Identity, uint64) (Entitlement, error)
}

type TicketService interface {
    Transition(context.Context, Identity, uint64, string, string) (Ticket, error)
}
~~~

Ticket package persists every message and state transition in package tables and emits an immutable package audit event. Plan package owns catalog, eligibility, assignments, and quota definitions; it publishes a versioned entitlement response for subscription, proxy-node, order, and forward hosts. Replace direct ticket and plan service calls in legacy handlers with package mappings.

- [ ] **Step 4: Run package, adapter, and migration tests**

Run: GOWORK=off go test ./packages/ticket/tests ./packages/plan/tests ./internal/compat/v2 ./internal/tests/e2e -count=1
Expected: exit 0.

- [ ] **Step 5: Commit ticket and plan extraction**

~~~bash
git add packages/ticket packages/plan internal/handler/handler.go internal/service/service.go internal/tests/e2e/ticket_plan_plugin_test.go
git commit -m "feat(packages): move ticket and plan domains"
~~~

### Task 9: Move order and payment into an atomic commercial package wave

**Files:**
- Create: packages/order/control/main.go
- Create: packages/order/control/service.go
- Create: packages/order/migrations/001_initial.sql
- Create: packages/order/compat/v2-routes.json
- Create: packages/order/tests/contract_test.go
- Create: packages/payment/control/main.go
- Create: packages/payment/control/service.go
- Create: packages/payment/migrations/001_initial.sql
- Create: packages/payment/compat/v2-routes.json
- Create: packages/payment/tests/callback_contract_test.go
- Modify: internal/payment/registry.go
- Modify: internal/handler/payment_gateway_callback.go
- Test: internal/handler/payment_gateway_callback_test.go

**Interfaces:**
- Produces: order.create, order.apply_coupon, order.transition, order.request_entitlement, payment.initiate, payment.callback, and payment.reconcile.
- Package tables: v4_order_order, v4_order_promotion, v4_order_coupon_redemption, v4_payment_record, v4_payment_callback, and v4_payment_outbox.
- Consumes: plan.EntitlementReader and a one-time package secret lease for the selected payment gateway.

- [ ] **Step 1: Write duplicate-callback and coupon tests**

~~~go
func TestPaymentCallbackIsExactlyOnce(t *testing.T) {
    payload := signedGatewayPayload(t, "gateway-event-9")
    first := invokePackage(t, "payment", "payment.callback", payload, "gateway-event-9")
    second := invokePackage(t, "payment", "payment.callback", payload, "gateway-event-9")
    require.Equal(t, http.StatusOK, first.Status)
    require.Equal(t, first.Body, second.Body)
    require.Equal(t, 1, countRows(t, "v4_payment_callback"))
}
~~~

- [ ] **Step 2: Run the commercial tests to verify they fail**

Run: GOWORK=off go test ./packages/order/tests ./packages/payment/tests ./internal/handler -run 'Test.*(Payment|Coupon)' -count=1
Expected: FAIL because commercial package hosts are absent.

- [ ] **Step 3: Implement atomic commercial state handling**

~~~go
type EntitlementRequest struct {
    OrderID        string
    UserID         uint64
    PlanID         uint64
    IdempotencyKey string
}

func (s *Service) SettleCallback(ctx context.Context, event GatewayEvent) (Receipt, error) {
    // Insert the callback receipt under its gateway event id, transition one
    // order state, and enqueue one entitlement request in one transaction.
}
~~~

Order owns promotions and coupons. Payment stores the verified gateway callback receipt before any transition, and reports the stable public callback envelope through the v2 adapter. Entitlement requests are durable package messages and use the plan package's versioned contract. Do not retain payment gateway registration or callback state transitions in Control after cutover.

- [ ] **Step 4: Run SQLite/PostgreSQL and reverse-migration tests**

Run: GOWORK=off go test ./packages/order/tests ./packages/payment/tests ./internal/tests/integration -run 'Test.*(Order|Payment)' -count=1
Expected: exit 0.

Run: ANIX_TEST_POSTGRES_DSN="$ANIX_TEST_POSTGRES_DSN" GOWORK=off go test ./internal/tests/integration -run TestPostgresCommercialReverseMigration -count=1
Expected: exit 0 when the PostgreSQL test DSN is supplied.

- [ ] **Step 5: Commit the commercial wave**

~~~bash
git add packages/order packages/payment internal/payment/registry.go internal/handler/payment_gateway_callback.go internal/handler/payment_gateway_callback_test.go
git commit -m "feat(packages): move orders and payments"
~~~

### Task 10: Move subscription and proxy-node contracts into package hosts

**Files:**
- Create: packages/subscription/control/main.go
- Create: packages/subscription/control/service.go
- Create: packages/subscription/migrations/001_initial.sql
- Create: packages/subscription/compat/v2-routes.json
- Create: packages/subscription/tests/output_contract_test.go
- Create: packages/proxy-node/control/main.go
- Create: packages/proxy-node/control/service.go
- Create: packages/proxy-node/agent/main.go
- Create: packages/proxy-node/migrations/001_initial.sql
- Create: packages/proxy-node/compat/v2-routes.json
- Create: packages/proxy-node/tests/node_contract_test.go
- Modify: internal/grpc/node_protocol_selection.go
- Modify: internal/service/subscription_settings.go
- Test: internal/tests/e2e/subscribe_test.go

**Interfaces:**
- Produces: subscription.render, subscription.usage.read, proxy-node.register, proxy-node.config.deliver, proxy-node.user.deliver, and proxy-node.traffic.report.
- Package tables: v4_subscription_group, v4_subscription_template, v4_subscription_usage, v4_proxy_node, v4_proxy_node_config, v4_proxy_node_user, and v4_proxy_node_usage.
- Consumes: plan.EntitlementReader and agentplugin.Operation / RuntimeStatus for node-side state.

- [ ] **Step 1: Write byte-stable subscription and node delivery tests**

~~~go
func TestSubscriptionOutputMatchesLegacyFixture(t *testing.T) {
    want := readFixture(t, "testdata/v3_subscription_base64.txt")
    got := fetchV2Subscription(t, "fixture-token")
    require.Equal(t, want, got)
}

func TestProxyNodeDeliveryUsesAgentPackageOperation(t *testing.T) {
    operation := waitForAgentOperation(t, "proxy-node")
    require.Equal(t, "configure", operation.Kind)
    require.Equal(t, uint64(1), operation.Generation)
}
~~~

- [ ] **Step 2: Run the tests to verify they fail**

Run: GOWORK=off go test ./packages/subscription/tests ./packages/proxy-node/tests ./internal/tests/e2e ./internal/grpc -run 'Test.*(Subscription|ProxyNode)' -count=1
Expected: FAIL because subscription and proxy-node package hosts are absent.

- [ ] **Step 3: Implement public output and Agent delivery contracts**

~~~go
type SubscriptionRenderer interface {
    Render(context.Context, Identity, string) ([]byte, string, error)
}

type NodeDelivery struct {
    NodeID         uint64
    ConfigVersion  string
    Generation     uint64
    Operation      agentplugin.Operation
}
~~~

Subscription copies legacy template and usage rows into package tables, validates row and rendered-output hashes, and preserves each supported response content type and cache header. Proxy-node owns registration/config/user/traffic semantics and turns node delivery into a versioned Agent package operation. The kernel gRPC layer remains transport-only and does not decide node protocols, users, quotas, or subscription content.

- [ ] **Step 4: Run cross-repository and public-contract tests**

Run: GOWORK=off go test ./packages/subscription/tests ./packages/proxy-node/tests ./internal/grpc ./internal/tests/e2e -count=1
Expected: exit 0.

Run: cd ../anix-agent && GOEXPERIMENT=jsonv2 GOWORK=off go test ./node ./plugin -run TestProxyNodePackage -count=1
Expected: exit 0.

- [ ] **Step 5: Commit subscription and proxy-node extraction**

~~~bash
git add packages/subscription packages/proxy-node internal/grpc/node_protocol_selection.go internal/service/subscription_settings.go internal/tests/e2e/subscribe_test.go
git commit -m "feat(packages): move subscriptions and proxy nodes"
~~~

### Task 11: Move forwarding control-plane behavior into the forward package

**Files:**
- Create: packages/forward/control/main.go
- Create: packages/forward/control/service.go
- Create: packages/forward/agent/main.go
- Create: packages/forward/migrations/001_initial.sql
- Create: packages/forward/compat/v2-routes.json
- Create: packages/forward/tests/forward_contract_test.go
- Modify: internal/handler/forward_panel_handler.go
- Modify: internal/service/forward_panel_service.go
- Modify: internal/service/forward_agent_bridge_worker.go
- Modify: internal/router/router.go
- Modify: docs/guide/flux-panel-clone.md
- Modify: docs/guide/flux-forward-contract.md
- Modify: docs/guide/forward-tunnel-runtime-ops.md
- Modify: docs/guide/forward-relay-onboarding.md
- Test: internal/handler/forward_panel_router_smoke_test.go

**Interfaces:**
- Produces: forward.rule.create, forward.rule.update, forward.tunnel.assign, forward.observation.read, and forward.agent.apply.
- Package tables: v4_forward_rule, v4_forward_tunnel, v4_forward_assignment, v4_forward_observation, and v4_forward_outbox.
- Consumes: proxy-node configuration delivery and agentplugin.Operation for Agent execution.

- [ ] **Step 1: Write route, authority, and assignment rollback tests**

~~~go
func TestForwardAssignmentRollsBackOnAgentReject(t *testing.T) {
    result := invokePackage(t, "forward", "forward.tunnel.assign", `{"rule_id":3,"relay_id":8}`, "assign-3-8")
    require.Equal(t, http.StatusAccepted, result.Status)

    reportAgentFailure(t, result.OperationID, "invalid_runtime_config")
    assignment := getForwardAssignment(t, 3)
    require.Equal(t, "rolled_back", assignment.State)
}
~~~

- [ ] **Step 2: Run the forward tests to verify they fail**

Run: GOWORK=off go test ./packages/forward/tests ./internal/handler ./internal/service ./internal/router -run 'Test.*Forward' -count=1
Expected: FAIL because forward package routes and migrations are absent.

- [ ] **Step 3: Implement the package ownership boundary**

~~~go
type ForwardOperation struct {
    RuleID         uint64
    RelayID        uint64
    Generation     uint64
    IdempotencyKey string
}

func (s *Service) Assign(ctx context.Context, identity Identity, input ForwardOperation) (OperationRef, error) {
    // Write the package assignment and durable Agent operation atomically.
}
~~~

Migrate rules, tunnels, relay assignments, limits, and observability into the package namespace. Preserve documented forwarding envelopes through compat/v2. Move recurring forwarding workers behind package entrypoints and remove forwarding domain decisions from the kernel. Update all four required forwarding guides with the exact package lifecycle, Agent acknowledgement, generation rollback, and retained public contract.

- [ ] **Step 4: Run forwarding, Agent bridge, and documentation checks**

Run: GOWORK=off go test ./packages/forward/tests ./internal/handler ./internal/service ./internal/router -count=1
Expected: exit 0.

Run: grep -q "forward package" docs/guide/flux-forward-contract.md
Expected: exit 0.

- [ ] **Step 5: Commit forwarding extraction and required docs**

~~~bash
git add packages/forward internal/handler/forward_panel_handler.go internal/service/forward_panel_service.go internal/service/forward_agent_bridge_worker.go internal/router/router.go docs/guide/flux-panel-clone.md docs/guide/flux-forward-contract.md docs/guide/forward-tunnel-runtime-ops.md docs/guide/forward-relay-onboarding.md
git commit -m "feat(packages): move forwarding control plane"
~~~

### Task 12: Complete Agent-target runtime packages and coordinated host reports

**Files:**
- Create: packages/wireguard/control/main.go
- Create: packages/wireguard/agent/main.go
- Create: packages/wireguard/migrations/001_initial.sql
- Create: packages/wireguard/tests/runtime_contract_test.go
- Create: packages/protocol-runtime/control/main.go
- Create: packages/protocol-runtime/agent/main.go
- Create: packages/protocol-runtime/migrations/001_initial.sql
- Create: packages/protocol-runtime/tests/runtime_contract_test.go
- Modify: packages/machine-telemetry/manifest.template.json
- Modify: packages/nftables-forward/manifest.template.json
- Modify: packages/gost-mesh/manifest.template.json
- Modify: packages/nat-egress/manifest.template.json
- Create: ../anix-agent/plugin/wireguard/runtime.go
- Create: ../anix-agent/plugin/protocolruntime/runtime.go
- Create: ../anix-agent/plugin/runtime_report.go
- Modify: ../anix-agent/node/agent_control.go
- Modify: internal/grpc/kernel_operation_bridge.go
- Test: internal/grpc/kernel_operation_bridge_cross_repo_e2e_test.go

**Interfaces:**
- Produces: Agent packages wireguard and protocol-runtime, plus v2 manifests and runtime reports for all six Agent-target runtime packages.
- Consumes: agentplugin.Operation and emits agentplugin.RuntimeStatus with package id, version, generation, health, observed hash, and failure code.
- Runtime package ids: machine-telemetry, nftables-forward, gost-mesh, nat-egress, wireguard, protocol-runtime.

- [ ] **Step 1: Write cross-repository runtime report tests**

~~~go
func TestAgentRuntimeReportIsBoundToOperationGeneration(t *testing.T) {
    report := agentplugin.RuntimeStatus{
        PackageID: "wireguard", PackageVersion: "4.0.0",
        Generation: 3, Healthy: true, ObservedHash: validHash,
    }
    require.NoError(t, bridge.AcceptRuntimeStatus(context.Background(), report))

    report.Generation = 2
    require.ErrorIs(t, bridge.AcceptRuntimeStatus(context.Background(), report), ErrStaleRuntimeStatus)
}
~~~

- [ ] **Step 2: Run the cross-repository tests to verify they fail**

Run: GOWORK=off go test ./internal/grpc -run TestAgentRuntimeReportIsBoundToOperationGeneration -count=1
Expected: FAIL because RuntimeStatus delivery is absent.

Run: cd ../anix-agent && GOEXPERIMENT=jsonv2 GOWORK=off go test ./plugin ./node -run TestWireGuardPackageRuntime -count=1
Expected: FAIL because the WireGuard package runtime is absent.

- [ ] **Step 3: Implement runtime package execution and observed-state reporting**

~~~go
func (s *Supervisor) ReportRuntimeStatus(ctx context.Context, status agentplugin.RuntimeStatus) error {
    if err := agentplugin.ValidateRuntimeStatus(status); err != nil {
        return err
    }
    return s.reporter.Report(ctx, status)
}
~~~

WireGuard package owns peer lifecycle and generated configuration; protocol-runtime owns protocol composition and runtime-adapter selection. The four existing packages retain their runtime semantics but gain v2 entrypoint, generation, and report declarations. Control's operation bridge persists only generic observed status and never interprets package-specific runtime configuration.

- [ ] **Step 4: Run all Agent runtime and Control bridge tests**

Run: cd ../anix-agent && GOEXPERIMENT=jsonv2 GOWORK=off go test ./plugin ./node ./core/wireguard -count=1
Expected: exit 0.

Run: cd ../anix-control && GOWORK=off go test ./internal/grpc ./packages/wireguard/tests ./packages/protocol-runtime/tests -count=1
Expected: exit 0.

- [ ] **Step 5: Commit coordinated runtime changes**

~~~bash
cd ../anix-agent
git add plugin node
git commit -m "feat(plugin): report v4 runtime package status"

cd ../anix-control
git add packages/wireguard packages/protocol-runtime packages/machine-telemetry packages/nftables-forward packages/gost-mesh packages/nat-egress internal/grpc/kernel_operation_bridge.go
git commit -m "feat(packages): add wireguard and protocol runtime"
~~~

### Task 13: Deliver package-owned, restrained administrator WebUI surfaces

**Files:**
- Create: packages/knowledge/webui/index.mjs
- Create: packages/notification/webui/index.mjs
- Create: packages/ticket/webui/index.mjs
- Create: packages/plan/webui/index.mjs
- Create: packages/order/webui/index.mjs
- Create: packages/payment/webui/index.mjs
- Create: packages/subscription/webui/index.mjs
- Create: packages/proxy-node/webui/index.mjs
- Create: packages/forward/webui/index.mjs
- Create: packages/wireguard/webui/index.mjs
- Create: packages/protocol-runtime/webui/index.mjs
- Modify: web/src/layouts/AdminLayout.vue
- Modify: web/src/router/index.ts
- Modify: web/src/views/admin/PluginCenter.vue
- Create: web/src/views/admin/PluginCenter.spec.ts
- Test: web/src/views/admin/PluginCenter.spec.ts

**Interfaces:**
- Produces: signed package WebUI route declarations and mount exports referenced by each manifest.
- Consumes: the kernel extension catalog and package route contracts; admin shell owns navigation chrome but not package business state.
- Presentation rule: compact operational layout, no nested cards, no marketing hero, no decorative gradients, stable toolbars, icon controls with tooltips, and Chinese text encoded as UTF-8.

- [ ] **Step 1: Write plugin-center behavior tests**

~~~ts
it("shows package health and a single primary lifecycle action", async () => {
  render(PluginCenter, { packages: [{ id: "forward", state: "healthy", enabled: true }] })
  expect(screen.getByText("forward")).toBeVisible()
  expect(screen.getByRole("button", { name: "停用" })).toBeVisible()
  expect(screen.queryByText("安装说明")).not.toBeInTheDocument()
})
~~~

- [ ] **Step 2: Run the frontend test to verify it fails**

Run: cd web && npm test -- PluginCenter.spec.ts --runInBand
Expected: FAIL because the package-owned view state is absent.

- [ ] **Step 3: Implement extension mounting and concise package controls**

~~~ts
export type PackageSummary = {
  id: string
  version: string
  state: "disabled" | "healthy" | "degraded" | "rolling_back"
  generation: number
  cohortPercent: 1 | 5 | 25 | 100
}

export function primaryAction(pkg: PackageSummary): "启用" | "停用" | "回滚" {
  if (pkg.state === "rolling_back" || pkg.state === "degraded") return "回滚"
  return pkg.state === "disabled" ? "启用" : "停用"
}
~~~

Load menu and route metadata only from verified extension catalog entries. Plugin Center shows one compact package table with health, active version, cohort, generation, and a primary lifecycle action; details open as a modal or package route, not nested cards. Each business screen is a signed package bundle and calls its v4 package routes. Keep AdminLayout as the generic host shell and remove hard-coded business navigation entries when their package bundle is enabled.

- [ ] **Step 4: Run frontend build and package bundle smoke tests**

Run: cd web && npm run test -- --runInBand && npm run build
Expected: exit 0.

Run: cd .. && python3 packages/shared/build_package.py --all --version 4.0.0 --out dist/packages && node packages/knowledge/tests/webui_smoke.mjs
Expected: exit 0.

- [ ] **Step 5: Commit WebUI package surfaces**

~~~bash
git add packages/*/webui web/src/layouts/AdminLayout.vue web/src/router/index.ts web/src/views/admin/PluginCenter.vue web/src/views/admin/PluginCenter.spec.ts
git commit -m "feat(web): mount concise package administration views"
~~~

### Task 14: Remove coupled domain execution and enforce plugin-only static gates

**Files:**
- Modify: internal/plugincontrol/registry.go
- Modify: internal/plugincontrol/operation_worker.go
- Modify: cmd/server/main.go
- Modify: internal/router/router.go
- Modify: internal/handler/handler.go
- Modify: internal/service/service.go
- Create: config/scripts/check_plugin_only_workers.py
- Create: config/scripts/check_plugin_only_workers_test.py
- Modify: config/scripts/check_release_stage.py
- Modify: config/scripts/verify_release_artifacts.py
- Create: internal/router/plugin_only_static_test.go
- Test: config/scripts/check_plugin_only_routes_test.py

**Interfaces:**
- Produces: release gates that fail on a required missing package, direct legacy route registration, legacy domain worker startup, unsigned artifact, absent migration evidence, or unverified v4 package version.
- Removes: live in-process Executor use and kernel-owned domain worker startup after package cutover.
- Retains: kernel identity/auth/authorization/package policy/audit/lifecycle/migration orchestration/health only.

- [ ] **Step 1: Write static-gate regression tests**

~~~python
def test_worker_gate_rejects_legacy_forward_worker(tmp_path):
    source = tmp_path / "main.go"
    source.write_text("go service.StartForwardWorker(db)\n", encoding="utf-8")
    result = run_gate(tmp_path)
    assert result.returncode != 0
    assert "legacy domain worker" in result.stderr
~~~

- [ ] **Step 2: Run static-gate tests to verify they fail**

Run: python3 config/scripts/check_plugin_only_workers_test.py && GOWORK=off go test ./internal/router -run TestPluginOnlyStaticGate -count=1
Expected: FAIL because the worker gate and test are absent.

- [ ] **Step 3: Remove coupled routes/workers and implement gates**

~~~python
FORBIDDEN_WORKER_MARKERS = (
    "StartForwardWorker",
    "StartPaymentWorker",
    "StartSubscriptionWorker",
    "StartTicketWorker",
)

def verify_worker_sources(repo_root):
    for path in source_files(repo_root / "cmd", repo_root / "internal"):
        if any(marker in path.read_text(encoding="utf-8") for marker in FORBIDDEN_WORKER_MARKERS):
            raise GateError(f"legacy domain worker in {path}")
~~~

Delete the final in-process route and lifecycle calls after each equivalent package has reached 100 percent and passed reverse migration. Keep historical type definitions only where needed to read old data during the 72-hour rollback window; no live request path or worker may invoke them. Make release-stage validation require the current plugin-only route and worker gates, signed all-package artifact manifest, SBOM, and package migration evidence.

- [ ] **Step 4: Run full static, unit, and race checks**

Run: python3 config/scripts/check_plugin_only_routes.py && python3 config/scripts/check_plugin_only_workers.py && python3 config/scripts/check_release_stage.py --tag v4.0.0
Expected: exit 0.

Run: GOWORK=off go test -race ./internal/router ./internal/handler ./internal/plugincontrol ./internal/pluginhost ./internal/compat/v2 -count=1
Expected: exit 0.

- [ ] **Step 5: Commit plugin-only enforcement**

~~~bash
git add internal/plugincontrol internal/router internal/handler/handler.go internal/service/service.go cmd/server/main.go config/scripts
git commit -m "refactor(kernel): enforce plugin-only execution"
~~~

### Task 15: Rehearse in-place upgrade, reverse upgrade, and fault rollback

**Files:**
- Create: internal/tests/integration/v3_to_v4_rehearsal_test.go
- Create: internal/tests/integration/v4_to_v3_reverse_test.go
- Create: internal/tests/integration/plugin_fault_injection_test.go
- Create: config/scripts/run_v4_rehearsal.sh
- Create: config/scripts/verify_v4_evidence.py
- Create: docs/guide/v4-plugin-only-upgrade.md
- Create: docs/guide/v4-plugin-only-rollback.md
- Modify: docs/guide/control-migration.md
- Modify: docs/guide/release-installation.md
- Test: cmd/integration-test/main_test.go

**Interfaces:**
- Produces: a JSON evidence bundle containing package artifact digest, migration run/checkpoint/validation digest, route generation, backup reference, fault outcome, and reverse-upgrade result for SQLite and PostgreSQL.
- Consumes: a v3 database fixture, signed package output, host manager, Agent package test node, and a configured test PostgreSQL DSN.
- Required faults: host crash, deadline expiry, duplicate mutation, interrupted migration, Agent disconnect, failed package update, and failed rollback.

- [ ] **Step 1: Write the full rehearsal assertion**

~~~go
func TestV3ToV4AndBackKeepsSupportedV2Contracts(t *testing.T) {
    environment := startV3Fixture(t)
    evidence := upgradeToV4(t, environment)
    assertAllPackagesAtGeneration(t, evidence, 1)
    assertV2ContractFixtures(t, environment)

    rollbackToV3(t, environment)
    assertV3Readable(t, environment)
}
~~~

- [ ] **Step 2: Run the rehearsal test to verify it fails**

Run: GOWORK=off go test ./internal/tests/integration -run TestV3ToV4AndBackKeepsSupportedV2Contracts -count=1
Expected: FAIL because v4 upgrade orchestration and evidence collection are absent.

- [ ] **Step 3: Implement deterministic rehearsals and operator guides**

~~~bash
#!/usr/bin/env bash
set -euo pipefail
TAG=v4.0.0
python3 config/scripts/check_release_stage.py --tag "$TAG"
GOWORK=off go test ./internal/tests/integration -run 'TestV3ToV4|TestV4ToV3|TestPluginFault' -count=1
python3 config/scripts/verify_v4_evidence.py --input artifacts/v4-rehearsal-evidence.json
~~~

The upgrade guide must require a verified backup before package installation, install packages disabled, run additive migrations, validate package-supplied hashes, route 1 percent, drain and switch generations, and retain the prior artifacts/projections. The rollback guide must identify automatic triggers, route-generation restoration, package version restoration, reverse migrations, and exact evidence collection. No guide may instruct manual database or package-file edits.

- [ ] **Step 4: Run SQLite, PostgreSQL, and fault-injection rehearsals**

Run: ./config/scripts/run_v4_rehearsal.sh
Expected: exit 0 and artifacts/v4-rehearsal-evidence.json validates.

Run: ANIX_TEST_POSTGRES_DSN="$ANIX_TEST_POSTGRES_DSN" ./config/scripts/run_v4_rehearsal.sh
Expected: exit 0 and evidence includes postgres.

- [ ] **Step 5: Commit rehearsal automation and guides**

~~~bash
git add internal/tests/integration config/scripts/run_v4_rehearsal.sh config/scripts/verify_v4_evidence.py docs/guide/v4-plugin-only-upgrade.md docs/guide/v4-plugin-only-rollback.md docs/guide/control-migration.md docs/guide/release-installation.md
git commit -m "test(release): rehearse v4 in-place rollback"
~~~

### Task 16: Align formal version surfaces and publish only after release gates

**Files:**
- Modify: internal/branding/branding.go
- Modify: Makefile
- Modify: cmd/server/main.go
- Modify: web/package.json
- Modify: web/package-lock.json
- Modify: docs/swagger.json
- Modify: docs/docs.go
- Modify: docs/swagger.yaml
- Modify: config/config.yaml.example
- Modify: config/config.prod.yaml
- Modify: config/config.dev.yaml.example
- Modify: install.sh
- Modify: CHANGELOG.md
- Modify: README.md
- Modify: config/scripts/check_release_version.py
- Modify: config/scripts/generate_release_manifest.py
- Modify: config/scripts/generate_release_notes.py
- Create: config/scripts/render_v4_release_evidence.py
- Create: config/scripts/render_v4_release_evidence_test.py
- Modify: .github/workflows/release.yml
- Test: config/scripts/check_release_version.py

**Interfaces:**
- Produces: one v4.0.0 release manifest tying the actual Control commit, Agent SDK v1.1.0, all fifteen package versions/digests/signatures/SBOMs, frontend assets, Docker image, binaries, and evidence bundle together. The evidence JSON is generated into the release artifact directory and is never a tracked placeholder file.
- Consumes: completed Task 15 evidence and a 72-hour canary record at 1/5/25/100 cohorts.
- External prerequisite: GitHub Support must confirm PR #4's read-only ref and caches are purged before release wording says history is fully scrubbed.

- [ ] **Step 1: Write release-gate tests**

~~~python
def test_v4_release_requires_all_evidence(tmp_path):
    manifest = minimal_release_manifest("4.0.0")
    manifest["canary"]["hours"] = 71
    error = validate_v4_evidence(manifest)
    assert "72-hour canary" in str(error)
~~~

- [ ] **Step 2: Run version and evidence tests to verify they fail**

Run: python3 config/scripts/check_release_version.py --tag v4.0.0
Expected: FAIL because source versions and evidence are not final.

- [ ] **Step 3: Set versions and render release evidence from real inputs**

~~~python
def render_release_evidence(inputs: ReleaseInputs) -> dict:
    return {
        "release": inputs.tag,
        "control_commit": inputs.control_commit,
        "agent_sdk": "v1.1.0",
        "packages": inputs.packages,
        "canary": inputs.canary,
        "rehearsals": inputs.rehearsals,
        "history_hygiene": inputs.github_support,
    }
~~~

The renderer accepts the current git commit, signed package manifest, canary record, rehearsal bundle, and GitHub Support confirmation as required inputs and fails on a missing value. Extend the release workflow to call package build/sign/SBOM verification, route/worker gates, SDK compatibility tests, rehearsal evidence validation, release version validation, evidence rendering, and artifact verification before it can create a tag or GitHub Release.

- [ ] **Step 4: Run the complete release candidate gate**

Run: python3 config/scripts/render_v4_release_evidence.py --tag v4.0.0 --control-commit "$(git rev-parse HEAD)" --package-manifest dist/release/package-manifest-v4.0.0.json --canary-evidence artifacts/v4-canary.json --rehearsal-evidence artifacts/v4-rehearsal-evidence.json --github-support-evidence artifacts/github-support-pr4.json --out dist/release/v4.0.0-evidence.json && python3 config/scripts/check_release_version.py --tag v4.0.0 && python3 config/scripts/check_release_stage.py --tag v4.0.0 && python3 config/scripts/check_plugin_only_routes.py && python3 config/scripts/check_plugin_only_workers.py && python3 config/scripts/verify_v4_evidence.py --input dist/release/v4.0.0-evidence.json && ./config/scripts/check_release_workflow.sh
Expected: exit 0.

Run: GOWORK=off go test ./... && cd web && npm run build
Expected: exit 0.

- [ ] **Step 5: Create the formal release only after the gates and operator approval**

~~~bash
git add internal/branding/branding.go Makefile cmd/server/main.go web/package.json web/package-lock.json docs config install.sh CHANGELOG.md README.md .github/workflows/release.yml
git commit -m "release: v4.0.0 plugin-only stable"
git tag -a v4.0.0 -m "AnixOps Control v4.0.0"
git push origin go_dev v4.0.0
gh release create v4.0.0 --verify-tag --title "AnixOps Control v4.0.0" --notes-file dist/release-notes-v4.0.0.md dist/release/*
~~~

Do not execute this step until the evidence document contains a real release commit, a real 72-hour canary, explicit operator approval, and GitHub Support's purge confirmation.

## Self-Review

- Spec coverage: Tasks 1-3 implement the cross-repository SDK, external Control host, and fail-closed lifecycle. Task 4 requires every signed package artifact. Tasks 5-6 implement additive migrations, generation rollouts, and /api/v2 adapters. Tasks 7-12 cover all fifteen package owners. Task 13 covers package-owned admin UI. Tasks 14-16 cover legacy removal, SQLite/PostgreSQL rehearsals, 72-hour canary evidence, version surfaces, and formal release controls.
- Placeholder scan: the prohibited-marker scan found no unresolved planning markers, generic validation instruction, or unbounded test instruction. Release evidence is rendered only from real workflow inputs and is not committed as a template.
- Type consistency: pluginhost.DispatchInput is the single Control host dispatch input; agentplugin.Operation and agentplugin.RuntimeStatus are the single Control/Agent package operation boundary; PackageRouteGeneration is the single rollout fence; compatv2.Gateway is the only supported /api/v2 business dispatch path.
