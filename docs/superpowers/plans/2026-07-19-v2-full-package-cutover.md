# V2 Full Package Cutover Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move every supported Control `/api/v2` business endpoint to a signed, supervised package host while preserving its public contract and adding `identity-platform` as the sixteenth required package.

**Architecture:** A checked, source-derived route ownership catalog is the sole design-time mapping of v2 method/path pairs to package route ids, envelopes, middleware groups, and transports. At runtime, `internal/compat/v2` resolves only the matching route declaration from the verified active package artifact and dispatches it through `pluginhost.Manager`; unary HTTP and WebSocket paths use separate generic transports. Domain packages are implemented and cut over in reversible ownership waves, and the final static gate refuses any catalogued v2 business route bound to a direct legacy handler.

**Tech Stack:** Go 1.25, Gin, GORM with SQLite/PostgreSQL, gRPC/protobuf over Unix sockets, Ed25519 signed package artifacts, Python release gates, Go AST route inventory tooling.

## Global Constraints

- Product release remains `v4.0.0`; Control module path remains `github.com/AnixOps/anix-control/v4`.
- The required v4 catalog contains the existing fifteen package ids plus the Control-only `identity-platform` package.
- `identity-platform` owns login, registration, profile/session identity, admin users, MFA, invitation/commission administration, platform configuration, audit, and backups.
- Every supported `/api/v2` business method/path has exactly one package owner, signed declaration, package route id, envelope, middleware group, and transport.
- `router.go` retains its existing middleware chain; final v2 business registrations call `compatv2.Gateway` or `compatv2.WebSocketGateway`, never a legacy handler/service/model/worker.
- Verified installed package artifacts are the only runtime source of compatibility declarations. Source catalog data is a build-time completeness and ownership gate, not a runtime fallback.
- Unary package responses preserve documented v2 status/body/header behavior through explicit envelope translators. Disabled, unsigned, stale, unhealthy, or incompatible packages fail closed.
- The existing v2 WebSocket endpoints use a versioned bidirectional Unix-gRPC package-host relay after normal HTTP middleware and verified route resolution; no legacy socket handler remains at final cutover.
- Each ownership wave is additive and generation-fenced. The final v4 candidate/release is blocked until every catalogued v2 route is cut over, its package migration/rollback test passes, and its signed artifact is included in formal evidence.
- SQLite and PostgreSQL compatibility/reverse-migration gates remain required. Missing `ANIX_TEST_POSTGRES_DSN` is an evidence gap, never a pass.

---

## File Structure

- `packages/identity-platform/`: signed Control package for identity and platform-admin behavior, migrations, compatibility routes, host, and WebUI.
- `config/v2-package-route-catalog.json`: complete checked v2 method/path ownership inventory.
- `config/scripts/v2_route_inventory.go`: Go AST reader that emits actual Gin v2 registrations and middleware groups from `internal/router/router.go`.
- `config/scripts/check_v2_package_route_catalog.py`: compares router inventory, catalog, signed route declarations, and final direct-handler policy.
- `internal/compat/v2/`: signed-declaration registry, unary gateway, envelope translators, and WebSocket relay.
- `api/pluginhost/v1/control_host.proto`: package-host unary and WebSocket relay protocol.
- `internal/service/kernel_service.go`: verified active-package compatibility-route resolution without legacy data fallback.

### Task 1: Expand the signed release catalog to sixteen packages

**Files:**
- Create: `packages/identity-platform/manifest.template.json`
- Create: `packages/identity-platform/migrations/index.json`
- Create: `packages/identity-platform/compat/v2-routes.json`
- Create: `packages/identity-platform/webui/index.mjs`
- Modify: `packages/shared/build_package.py`
- Modify: `packages/shared/tests/test_manifest_schema.py`
- Modify: `config/scripts/release-stage-contract.json`
- Modify: `config/scripts/check_release_stage.py`
- Create: `config/scripts/check_release_stage_test.py`

**Interfaces:**
- Produces: `PackageSpec("identity-platform", ("control",))` and a v4 release-stage contract with sixteen required signed package artifacts.
- Consumes: the existing v2 manifest, deterministic archive, formal-release trust-root, SBOM, and release-stage validation contracts.

- [ ] **Step 1: Write failing matrix and release-stage tests**

```python
def test_v4_stage_requires_identity_platform_and_sixteen_artifacts(repo_root, tmp_path):
    decision = validate_stage(repo_root, load_contract(repo_root / "config/scripts/release-stage-contract.json"), "v4.0.0")
    assert "identity-platform" in decision.package_ids
    assert len(decision.package_ids) == 16

    output = tmp_path / "packages"
    result = build(repo_root, "--all", "--version", "4.0.0", "--out", str(output))
    assert result.returncode == 0
    assert len(list(output.glob("*.anxp"))) == 16
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `python3 -m unittest packages/shared/tests/test_manifest_schema.py config/scripts/check_release_stage_test.py`

Expected: FAIL because `identity-platform` is absent from the package matrix and v4 stage.

- [ ] **Step 3: Add the package and formal-release matrix entry**

```python
PACKAGE_SPECS = (
    PackageSpec("identity-platform", ("control",)),
    PackageSpec("subscription", ("control",)),
    # Existing fifteen entries remain in their release order.
)
```

The identity template uses `api_version: "v2"`, `targets: ["control"]`, a
canonical `bin/control-host`, `migrations/index.json`,
`compat/v2-routes.json`, and `webui/index.mjs`. The v4 release contract adds
an `identity-platform` required package with the same artifact, SBOM, and
formal-root checks used by every other package.

- [ ] **Step 4: Verify local and formal package output**

Run: `python3 packages/shared/build_package.py --all --version 4.0.0 --out /tmp/anixops-v2-catalog-build`

Expected: `built 16 signed package artifacts and 16 SBOM files`.

Run: `python3 config/scripts/check_release_stage.py --tag v4.0.0 && python3 -m unittest packages/shared/tests/test_manifest_schema.py config/scripts/check_release_stage_test.py`

Expected: exit 0.

- [ ] **Step 5: Commit the package-matrix change**

```bash
git add packages/identity-platform packages/shared/build_package.py packages/shared/tests/test_manifest_schema.py config/scripts/release-stage-contract.json config/scripts/check_release_stage.py config/scripts/check_release_stage_test.py
git commit -m "feat(packages): add identity platform package"
```

### Task 2: Freeze the complete v2 route ownership catalog

**Files:**
- Create: `config/v2-package-route-catalog.json`
- Create: `config/scripts/v2_route_inventory.go`
- Create: `config/scripts/check_v2_package_route_catalog.py`
- Create: `config/scripts/check_v2_package_route_catalog_test.py`
- Create: `config/scripts/testdata/v2-route-catalog-duplicate.json`
- Create: `config/scripts/testdata/v2-route-catalog-unowned.json`
- Modify: `internal/router/router_test.go`

**Interfaces:**
- Produces: one record per supported v2 route:

```json
{
  "method": "POST",
  "path": "/api/v2/login",
  "owner": "identity-platform",
  "route_id": "identity.auth.login",
  "envelope": "data",
  "middleware_group": "public",
  "transport": "http"
}
```

- Consumes: AST-derived Gin groups and registration calls from `internal/router/router.go`.

- [ ] **Step 1: Write failing catalog-completeness tests**

```python
class RouteCatalogTest(unittest.TestCase):
    def test_catalog_equals_router_v2_inventory(self):
        inventory = parse_router_v2_routes(REPO_ROOT / "internal/router/router.go")
        catalog = load_catalog(REPO_ROOT / "config/v2-package-route-catalog.json")
        self.assertEqual({(row.method, row.path) for row in catalog}, {(row.method, row.path) for row in inventory})

    def test_catalog_rejects_duplicate_owner_and_unowned_route(self):
        with self.assertRaisesRegex(CatalogError, "duplicate method/path"):
            load_catalog(FIXTURES / "v2-route-catalog-duplicate.json")
        with self.assertRaisesRegex(CatalogError, "unowned v2 route"):
            validate_catalog(FIXTURES / "v2-route-catalog-unowned.json")
```

- [ ] **Step 2: Run the catalog tests to verify they fail**

Run: `python3 config/scripts/check_v2_package_route_catalog_test.py`

Expected: FAIL because no catalog or AST inventory parser exists.

- [ ] **Step 3: Implement AST inventory and explicit ownership records**

`v2_route_inventory.go` uses `go/parser` and `go/ast` to follow local Gin
`Group`, `Use`, and verb registration calls, retaining normalized full path,
HTTP method, handler identifier, and middleware group. It must identify all
three WebSocket registrations as `transport: "websocket"`.

The checked catalog contains every emitted method/path exactly once. Ownership
uses the design mapping: identity/platform, telemetry, subscription,
knowledge, ticket, plan, order, payment, notification, proxy-node,
protocol-runtime, wireguard, forward, and declared topology packages. The
catalog maps `POST /api/v2/login` to `identity.auth.login`,
`GET /api/v2/user/knowledge` to `knowledge.article.list`, and
`POST /api/v2/payment/callback/:type` to `payment.callback`; it may not use
prefix-only wildcard ownership.

- [ ] **Step 4: Verify inventory and catalog are exact**

Run: `go run ./config/scripts/v2_route_inventory.go --router internal/router/router.go --format json | python3 config/scripts/check_v2_package_route_catalog.py --catalog config/v2-package-route-catalog.json --inventory -`

Expected: `v2 package route catalog passed` and one record for every supported v2 registration.

- [ ] **Step 5: Commit the catalog**

```bash
git add config/v2-package-route-catalog.json config/scripts/v2_route_inventory.go config/scripts/check_v2_package_route_catalog.py config/scripts/check_v2_package_route_catalog_test.py config/scripts/testdata internal/router/router_test.go
git commit -m "feat(api): define v2 package route ownership"
```

### Task 3: Extend the package-host protocol for WebSocket relaying

**Files:**
- Modify: `api/pluginhost/v1/control_host.proto`
- Modify: `api/pluginhost/gen.sh`
- Modify: `api/pluginhost/generator_test.go`
- Modify: `api/pluginhost/v1/control_host.pb.go`
- Modify: `api/pluginhost/v1/control_host_grpc.pb.go`
- Modify: `pkg/pluginhostsdk/server.go`
- Modify: `pkg/pluginhostsdk/server_test.go`
- Modify: `internal/pluginhost/client.go`
- Create: `internal/pluginhost/websocket.go`
- Create: `internal/pluginhost/websocket_test.go`

**Interfaces:**
- Produces: `rpc OpenWebSocket(stream WebSocketFrame) returns (stream WebSocketFrame)`.
- Produces: an opening frame with package identity, version, generation, route id, principal JSON, request id, idempotency key, and deadline; data and close frames carry opaque bytes/code/reason.

- [ ] **Step 1: Write failing protocol and relay tests**

```go
func TestWebSocketRelaySendsVerifiedOpenBeforeData(t *testing.T) {
    relay := newRelayForHost(t, "machine-telemetry", 9)
    stream := relay.Open(testOpen("machine-telemetry", 9, "telemetry.monitor.ws"))
    require.Equal(t, FrameOpen, stream.Recv().Kind)
    stream.Send(dataFrame([]byte("ping")))
    require.Equal(t, []byte("ping"), stream.RecvFromHost().Data)
}

func TestWebSocketRelayRejectsLeaseLoss(t *testing.T) {
    relay := newRelayForHost(t, "proxy-node", 11)
    relay.LoseLease()
    require.ErrorIs(t, relay.Open(testOpen("proxy-node", 11, "proxy.node.ws")), ErrHostUnavailable)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOWORK=off go test ./api/pluginhost ./pkg/pluginhostsdk ./internal/pluginhost -run 'TestWebSocketRelay' -count=1`

Expected: FAIL because the streaming RPC and relay do not exist.

- [ ] **Step 3: Add a versioned streaming RPC and safe client relay**

```proto
rpc OpenWebSocket(stream WebSocketFrame) returns (stream WebSocketFrame);

message WebSocketFrame {
  oneof value {
    WebSocketOpen open = 1;
    bytes data = 2;
    WebSocketClose close = 3;
  }
}
```

The generated code remains reproducible through `api/pluginhost/gen.sh`.
`Supervisor.OpenWebSocket` resolves an exact active host generation before
opening the gRPC stream, sends exactly one open frame first, and closes both
sides on health loss, drain, deadline, invalid frame order, or RPC failure.

- [ ] **Step 4: Verify generated contract and race safety**

Run: `./api/pluginhost/gen.sh && git diff --exit-code -- api/pluginhost/v1/control_host.pb.go api/pluginhost/v1/control_host_grpc.pb.go`

Expected: exit 0.

Run: `GOWORK=off go test -race ./api/pluginhost ./pkg/pluginhostsdk ./internal/pluginhost -count=1`

Expected: exit 0.

- [ ] **Step 5: Commit the relay protocol**

```bash
git add api/pluginhost pkg/pluginhostsdk internal/pluginhost
git commit -m "feat(plugin): relay package websocket routes"
```

### Task 4: Build the verified v2 compatibility registry and gateways

**Files:**
- Create: `internal/compat/v2/registry.go`
- Create: `internal/compat/v2/gateway.go`
- Create: `internal/compat/v2/websocket.go`
- Create: `internal/compat/v2/envelope.go`
- Create: `internal/compat/v2/registry_test.go`
- Create: `internal/compat/v2/gateway_test.go`
- Create: `internal/compat/v2/websocket_test.go`
- Create: `internal/compat/v2/contract_test.go`
- Modify: `internal/service/kernel_service.go`
- Modify: `internal/handler/kernel.go`

**Interfaces:**
- Produces: `Registry.Resolve(method, normalizedPath) (Route, bool)`, `Gateway.Serve(*gin.Context)`, and `WebSocketGateway.Serve(*gin.Context)`.
- Consumes: a verified active package release and its signed `compat/v2-routes.json`; `pluginhost.Manager` and `pluginhost.Supervisor` respectively.

- [ ] **Step 1: Retain and extend the current RED tests**

```go
func TestV2GatewayFailsClosedWhenPackageIsDisabled(t *testing.T) {
    response := requestV2(t, http.MethodGet, "/api/v2/user/knowledge")
    require.Equal(t, http.StatusServiceUnavailable, response.Code)
    require.NotContains(t, response.Body.String(), "legacy")
}

func TestWebSocketGatewayDoesNotInvokeLegacyHandler(t *testing.T) {
    result := openV2Socket(t, "/api/v2/admin/ws/monitor", disablePackage(t, "machine-telemetry"))
    require.Equal(t, http.StatusServiceUnavailable, result.StatusCode)
    require.Equal(t, 0, legacyMonitorHandlerCalls(t))
}
```

- [ ] **Step 2: Run gateway tests to verify they fail**

Run: `GOWORK=off go test ./internal/compat/v2 ./internal/router -run 'TestV2KnowledgeListUsesPackageEnvelope|TestV2GatewayFailsClosedWhenPackageIsDisabled|TestWebSocketGatewayDoesNotInvokeLegacyHandler' -count=1`

Expected: FAIL because the registry and gateways are absent.

- [ ] **Step 3: Implement signed route resolution and envelope translation**

```go
type Route struct {
    Method, LegacyPath, PackageID, Version, PackageRoute, Envelope string
    Generation uint64
    Transport  string
}

type RouteSource interface {
    ResolveV2Route(context.Context, string, string) (Route, error)
}
```

`ResolveV2Route` verifies release signature, active installation, selected
generation, artifact declaration digest, exact method/path, owner, envelope,
and transport before returning a route. `Gateway.Serve` propagates body,
principal projection, request id, idempotency key, deadline, query/path
metadata, and generation to the host. `WebSocketGateway.Serve` performs the
upgrade only after the same resolution and relays the socket through Task 3.
No registry method may synthesize a source-catalog route at runtime.

- [ ] **Step 4: Verify package fixture contracts**

Run: `GOWORK=off go test ./internal/compat/v2 ./internal/router ./internal/handler -count=1`

Expected: exit 0 with signed fixture routes, exact envelopes, disabled-package
503 responses, and no legacy handler calls.

- [ ] **Step 5: Commit the generic compatibility surface**

```bash
git add internal/compat/v2 internal/service/kernel_service.go internal/handler/kernel.go
git commit -m "feat(api): add signed v2 compatibility gateway"
```

### Task 5: Implement and cut over identity-platform

**Files:**
- Create: `packages/identity-platform/control/main.go`
- Create: `packages/identity-platform/control/service.go`
- Create: `packages/identity-platform/control/service_test.go`
- Create: `packages/identity-platform/migrations/001_identity_platform.sql`
- Modify: `packages/identity-platform/compat/v2-routes.json`
- Modify: `internal/router/router.go`
- Modify: `internal/router/router_test.go`
- Create: `internal/tests/e2e/identity_platform_plugin_test.go`

**Interfaces:**
- Produces: identity package routes for authentication, profile, admin users,
  MFA, invite, platform config/audit/backup; every route has an exact v2
  envelope declaration.
- Consumes: the generic identity projection, authorization middleware, route
  generation, and compatibility gateway.

- [ ] **Step 1: Write identity route, disable, and reverse-migration tests**

```go
func TestV2LoginUsesIdentityPlatformHost(t *testing.T) {
    response := requestV2(t, http.MethodPost, "/api/v2/login", jsonBody(`{"email":"u@example.test","password":"secret"}`))
    require.Equal(t, http.StatusOK, response.Code)
    require.Equal(t, "identity.auth.login", identityHost(t).LastRouteID())
}

func TestIdentityPlatformRollbackRestoresPriorVerifiedGeneration(t *testing.T) {
    restored := rollbackIdentityPlatform(t)
    require.Equal(t, priorIdentityGeneration(t), restored.Generation)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOWORK=off go test ./packages/identity-platform/control ./internal/compat/v2 ./internal/tests/e2e -run 'Test.*Identity|TestV2LoginUsesIdentityPlatformHost' -count=1`

Expected: FAIL because the identity host and declarations are absent.

- [ ] **Step 3: Implement package-owned identity/platform state and declarations**

The migration creates package-owned identity/session/MFA/invite/platform-config
and package audit/backup projection tables. It imports legacy state under a
checkpointed migration, exposes legacy v2 envelopes from its host, and lists
every catalogued identity-platform method/path in `compat/v2-routes.json`.
Router registrations for those catalog entries become generic gateway calls
without changing their existing middleware group.

- [ ] **Step 4: Verify SQLite/PostgreSQL and compatibility contracts**

Run: `GOWORK=off go test ./packages/identity-platform/control ./internal/compat/v2 ./internal/tests/e2e -count=1`

Expected: exit 0.

Run: `ANIX_TEST_POSTGRES_DSN="$ANIX_TEST_POSTGRES_DSN" GOWORK=off go test ./internal/tests/integration -run TestIdentityPlatformReverseMigration -count=1`

Expected: exit 0 when the PostgreSQL DSN is supplied.

- [ ] **Step 5: Commit the first real v2 cutover wave**

```bash
git add packages/identity-platform internal/router/router.go internal/router/router_test.go internal/tests/e2e/identity_platform_plugin_test.go
git commit -m "feat(packages): move identity platform to host"
```

### Task 6: Deliver knowledge, notification, ticket, and plan route waves

**Files:**
- Modify: `packages/knowledge/compat/v2-routes.json`
- Modify: `packages/notification/compat/v2-routes.json`
- Modify: `packages/ticket/compat/v2-routes.json`
- Modify: `packages/plan/compat/v2-routes.json`
- Create or modify: each package's `control/`, `migrations/`, and contract tests
- Modify: `internal/router/router.go`
- Modify: `internal/tests/e2e/knowledge_notification_plugin_test.go`
- Modify: `internal/tests/e2e/ticket_plan_plugin_test.go`

**Interfaces:**
- Produces: signed declarations and runnable host routes for every catalogued
  knowledge, notification, ticket, and plan endpoint.

- [ ] **Step 1: Write failing wave contracts**

```go
func TestV2KnowledgeAndNotificationRoutesUseHosts(t *testing.T) {
    require.Equal(t, "knowledge.article.list", dispatchV2(t, http.MethodGet, "/api/v2/user/knowledge").RouteID)
    require.Equal(t, "notification.notice.list", dispatchV2(t, http.MethodGet, "/api/v2/user/notifications").RouteID)
}

func TestV2TicketAndPlanRoutesUseHosts(t *testing.T) {
    require.Equal(t, "ticket.list", dispatchV2(t, http.MethodGet, "/api/v2/user/ticket").RouteID)
    require.Equal(t, "plan.catalog.list", dispatchV2(t, http.MethodGet, "/api/v2/user/plan").RouteID)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOWORK=off go test ./packages/knowledge ./packages/notification ./packages/ticket ./packages/plan ./internal/compat/v2 ./internal/tests/e2e -run 'TestV2(Knowledge|Ticket|Plan)|Test.*Notification' -count=1`

Expected: FAIL before host declarations and migrations are installed.

- [ ] **Step 3: Implement the four package declarations, hosts, and router cutovers**

Each declaration contains only its catalogued exact routes and corresponding
envelope kinds. Each host owns its migration/projection and idempotency state;
the router changes only the registrations in those catalog rows. Existing
Task 7 and Task 8 data/contract requirements remain mandatory, including
notification outbox exactly-once behavior and generation-fenced ticket state.

- [ ] **Step 4: Verify wave contracts and reverse migration**

Run: `GOWORK=off go test ./packages/knowledge ./packages/notification ./packages/ticket ./packages/plan ./internal/compat/v2 ./internal/tests/e2e -count=1`

Expected: exit 0.

Run: `GOWORK=off go test ./internal/tests/integration -run 'Test(KnowledgeNotification|TicketPlan)ReverseMigration' -count=1`

Expected: exit 0.

- [ ] **Step 5: Commit the read/catalog cutover wave**

```bash
git add packages/knowledge packages/notification packages/ticket packages/plan internal/router/router.go internal/tests/e2e internal/tests/integration
git commit -m "feat(packages): cut over content support and plan v2 routes"
```

### Task 7: Deliver order, payment, and subscription route waves

**Files:**
- Modify: `packages/order/compat/v2-routes.json`
- Modify: `packages/payment/compat/v2-routes.json`
- Modify: `packages/subscription/compat/v2-routes.json`
- Create or modify: each package's `control/`, `migrations/`, and contract tests
- Modify: `internal/router/router.go`
- Modify: `internal/handler/payment_gateway_callback.go`
- Modify: `internal/tests/e2e/subscribe_test.go`

**Interfaces:**
- Produces: exact v2 order/coupon/payment/subscription declarations and host routes, including public callbacks and byte-stable subscription output.

- [ ] **Step 1: Write failing commercial and subscription contracts**

```go
func TestV2PaymentCallbackUsesPackageHostExactlyOnce(t *testing.T) {
    first := dispatchV2(t, http.MethodPost, "/api/v2/payment/callback/stripe")
    second := dispatchV2(t, http.MethodPost, "/api/v2/payment/callback/stripe")
    require.Equal(t, "payment.callback", first.RouteID)
    require.Equal(t, first.Body, second.Body)
}

func TestV2SubscriptionMatchesLegacyFixtureThroughHost(t *testing.T) {
    require.Equal(t, legacySubscriptionFixture(t), fetchV2SubscriptionThroughPackage(t))
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOWORK=off go test ./packages/order ./packages/payment ./packages/subscription ./internal/compat/v2 ./internal/tests/e2e -run 'Test.*(Payment|Coupon|Subscription)' -count=1`

Expected: FAIL before signed routes and hosts exist.

- [ ] **Step 3: Implement commercial/subscription declarations and cutovers**

Payment callbacks persist their package receipt before a transition; order
owns coupon/order data; subscription owns output/groups/templates. The
gateway preserves public limiter/callback behavior and subscription bytes.
No direct legacy payment, order, coupon, or subscription business handler is
left for a catalogued route.

- [ ] **Step 4: Verify contracts, SQLite, and PostgreSQL when configured**

Run: `GOWORK=off go test ./packages/order ./packages/payment ./packages/subscription ./internal/compat/v2 ./internal/tests/e2e -count=1`

Expected: exit 0.

Run: `ANIX_TEST_POSTGRES_DSN="$ANIX_TEST_POSTGRES_DSN" GOWORK=off go test ./internal/tests/integration -run 'Test(PostgresCommercialReverseMigration|PostgresSubscriptionReverseMigration)' -count=1`

Expected: exit 0 when the PostgreSQL DSN is supplied.

- [ ] **Step 5: Commit commercial/subscription cutover**

```bash
git add packages/order packages/payment packages/subscription internal/router/router.go internal/handler/payment_gateway_callback.go internal/tests
git commit -m "feat(packages): cut over commercial and subscription v2 routes"
```

### Task 8: Deliver node, runtime, WireGuard, forwarding, and topology route waves

**Files:**
- Modify: `packages/proxy-node/compat/v2-routes.json`
- Modify: `packages/protocol-runtime/compat/v2-routes.json`
- Modify: `packages/wireguard/compat/v2-routes.json`
- Modify: `packages/forward/compat/v2-routes.json`
- Modify: `packages/machine-telemetry/compat/v2-routes.json`
- Modify: `packages/nftables-forward/compat/v2-routes.json`
- Modify: `packages/gost-mesh/compat/v2-routes.json`
- Modify: `packages/nat-egress/compat/v2-routes.json`
- Create or modify: each package's control/agent host, migrations, and tests
- Modify: `internal/router/router.go`
- Modify: `internal/grpc/node_protocol_selection.go`
- Modify: `internal/tests/e2e/subscribe_test.go`

**Interfaces:**
- Produces: signed declarations for node registration/API/UniProxy, Agent task and monitor routes, WireGuard, forwarding/tunnel, telemetry, and topology runtime paths, including the three WebSocket paths.

- [ ] **Step 1: Write failing node, forwarding, and socket contracts**

```go
func TestV2NodeAndUniProxyUseProxyNodeHost(t *testing.T) {
    require.Equal(t, "proxy.node.register", dispatchV2(t, http.MethodPost, "/api/v2/node/register").RouteID)
    require.Equal(t, "proxy.uniproxy.config", dispatchV2(t, http.MethodGet, "/api/v2/server/UniProxy/config").RouteID)
}

func TestV2MonitorSocketUsesTelemetryHost(t *testing.T) {
    socket := openV2Socket(t, "/api/v2/admin/ws/monitor")
    require.Equal(t, "telemetry.monitor.ws", socket.HostRouteID())
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOWORK=off go test ./packages/proxy-node ./packages/protocol-runtime ./packages/wireguard ./packages/forward ./packages/machine-telemetry ./internal/compat/v2 ./internal/tests/e2e -run 'TestV2(Node|Monitor|Forward|UniProxy)' -count=1`

Expected: FAIL before runtime hosts and socket declarations exist.

- [ ] **Step 3: Implement declarations, Agent contract calls, and cutovers**

Proxy-node owns node protocol delivery and UniProxy compatibility; protocol-runtime
owns generic Agent task/monitor and protocol configuration; telemetry owns
monitor socket/dashboard data; forward owns forwarding/tunnel/forward-agent
paths; WireGuard and topology packages own their exact declared runtime paths.
The socket routes use Task 3 relay and package drain/lease behavior. Legacy
node/Agent/forward/WebSocket handlers are removed only for catalogued routes
whose package tests and migrations pass.

- [ ] **Step 4: Verify runtime and socket behavior**

Run: `GOWORK=off go test -race ./packages/proxy-node ./packages/protocol-runtime ./packages/wireguard ./packages/forward ./packages/machine-telemetry ./internal/compat/v2 ./internal/pluginhost ./internal/tests/e2e -count=1`

Expected: exit 0.

Run: `cd ../anix-agent && GOEXPERIMENT=jsonv2 GOWORK=off go test ./...`

Expected: exit 0 from the Agent repository when run there; route operations use SDK v1.1.0.

- [ ] **Step 5: Commit runtime/topology cutover**

```bash
git add packages internal/router/router.go internal/grpc/node_protocol_selection.go internal/compat/v2 internal/tests
git commit -m "feat(packages): cut over node runtime and forwarding v2 routes"
```

### Task 9: Enable final all-route v2 cutover and static enforcement

**Files:**
- Modify: `internal/router/router.go`
- Modify: `internal/router/router_test.go`
- Create: `config/scripts/check_plugin_only_routes.py`
- Create: `config/scripts/check_plugin_only_routes_test.py`
- Modify: `config/scripts/check_release_stage.py`
- Modify: `config/scripts/check_release_stage_test.py`
- Modify: `docs/guide/v4-plugin-only-upgrade.md`
- Modify: `docs/guide/v4-plugin-only-rollback.md`

**Interfaces:**
- Produces: a router where every catalogued v2 business registration is a generic compatibility gateway or WebSocket gateway call and `check_plugin_only_routes.py` is a release gate.

- [ ] **Step 1: Write failing final-cutover tests**

```python
def test_plugin_only_route_gate_rejects_direct_catalogued_handler(tmp_path):
    router = tmp_path / "router.go"
    router.write_text('auth.GET("/user/knowledge", knowledgeHandler.GetArticles)')
    assert run_gate(router).returncode != 0
    assert "direct legacy handler" in run_gate(router).stderr
```

```go
func TestAllCataloguedV2RoutesUsePackageGateway(t *testing.T) {
    for _, route := range loadV2RouteCatalog(t) {
        registration := registrationFor(t, route.Method, route.Path)
        require.Contains(t, registration.Handler, "compatv2")
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `python3 config/scripts/check_plugin_only_routes_test.py && GOWORK=off go test ./internal/router -run TestAllCataloguedV2RoutesUsePackageGateway -count=1`

Expected: FAIL until every final direct registration is replaced.

- [ ] **Step 3: Switch remaining registrations and enforce the gate**

The Python entrypoint invokes the Go AST inventory, compares it with the
checked catalog and signed package declarations, and rejects a catalogued
direct handler, missing route declaration, declaration owner mismatch,
missing host, or legacy WebSocket binding. It treats generic Kernel handlers
as allowed only when the catalog marks the route `transport: "kernel"`.

- [ ] **Step 4: Run final v2 and release-stage gates**

Run: `python3 config/scripts/check_v2_package_route_catalog.py && python3 config/scripts/check_plugin_only_routes.py && python3 config/scripts/check_release_stage.py --tag v4.0.0`

Expected: all three gates exit 0 and report sixteen required package artifacts.

Run: `GOWORK=off go test ./internal/compat/v2 ./internal/router ./internal/handler ./internal/tests/e2e -count=1`

Expected: exit 0 with no direct legacy v2 business fallback.

- [ ] **Step 5: Commit final v2 enforcement**

```bash
git add internal/router/router.go internal/router/router_test.go config/scripts docs/guide
git commit -m "feat(api): enforce package-only v2 routes"
```

### Task 10: Carry sixteen-package evidence through rehearsals and release gates

**Files:**
- Modify: `config/scripts/run_v4_rehearsal.sh`
- Modify: `config/scripts/verify_v4_evidence.py`
- Modify: `config/scripts/render_v4_release_evidence.py`
- Modify: `config/scripts/render_v4_release_evidence_test.py`
- Modify: `.github/workflows/release.yml`
- Modify: `docs/guide/release-installation.md`

**Interfaces:**
- Produces: formal evidence that names all sixteen package manifests, artifacts, SBOMs, official-root verification, v2 catalog gate output, socket relay tests, and SQLite/PostgreSQL rehearsal results.

- [ ] **Step 1: Write failing sixteen-package evidence tests**

```python
def test_release_evidence_requires_identity_platform_and_v2_route_gate(tmp_path):
    evidence = minimal_release_evidence(package_count=15)
    with pytest.raises(EvidenceError, match="identity-platform"):
        validate_v4_evidence(evidence)
    evidence = minimal_release_evidence(package_count=16)
    evidence["v2_route_gate"] = {"passed": False}
    with pytest.raises(EvidenceError, match="v2 route gate"):
        validate_v4_evidence(evidence)
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `python3 config/scripts/render_v4_release_evidence_test.py`

Expected: FAIL until release evidence validates the sixteenth package and v2 gate.

- [ ] **Step 3: Require complete route and package evidence**

The rehearsal calls the signed package formal-release verifier for all sixteen
artifacts, `check_v2_package_route_catalog.py`,
`check_plugin_only_routes.py`, unary/WebSocket compatibility tests, and both
database rehearsals. The release workflow refuses a tag when any artifact,
catalogue, trust-root, canary, PostgreSQL, or support-evidence input is
missing.

- [ ] **Step 4: Verify evidence gates**

Run: `python3 config/scripts/run_v4_rehearsal.sh && python3 config/scripts/verify_v4_evidence.py --input artifacts/v4-rehearsal-evidence.json`

Expected: exit 0 only when all local evidence is present; PostgreSQL requires
an explicitly supplied DSN.

- [ ] **Step 5: Commit evidence changes**

```bash
git add config/scripts .github/workflows/release.yml docs/guide/release-installation.md
git commit -m "test(release): require complete v2 package cutover evidence"
```

## Plan Self-Review

- Spec coverage: Tasks 1-2 add the sixteenth package and exhaustive ownership
  catalog; Tasks 3-4 provide the runtime transport and signed registry;
  Tasks 5-8 implement all owner waves; Task 9 enforces final no-fallback
  routing; Task 10 carries the sixteen-package and v2 gates into release.
- Placeholder scan: every task names paths, interfaces, RED/GREEN commands,
  and commit boundaries. The catalog is generated from router source but is a
  checked explicit data file rather than an inferred runtime mapping.
- Type consistency: unary dispatch uses `pluginhost.DispatchInput`; socket
  dispatch uses the Task 3 `WebSocketFrame` stream; route source resolution
  returns a generation-fenced `compatv2.Route`; final router entries select a
  gateway based on catalog transport.
