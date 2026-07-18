# V2 Full Package Ownership Design

## Decision

Control v4 will move every supported `/api/v2` business endpoint through a
signed package compatibility declaration and a supervised Control package
host. The existing fifteen required packages remain, and a sixteenth
Control-only package, `identity-platform`, is added for the identity and
platform-administration domain that has no coherent owner in the existing
catalog.

The former Task 6 order was invalid: Task 4 produced empty compatibility
route declarations and non-runnable manifest-stage hosts, while Tasks 7-12
were the first tasks that created real package routes. The corrected delivery
order freezes all ownership now, implements each package host and declaration
in waves, then enables the final all-route v2 cutover gate only after every
catalogued route is executable.

## Package Ownership

`identity-platform` owns login, registration, profile/session identity,
administrator user lifecycle, MFA, invitation and commission administration,
platform configuration, audit, and backup endpoints. The kernel retains only
authenticated-principal verification, package signature verification, and
generic routing/rollout duties.

The remaining endpoint families have one owner:

| Owner | V2 endpoint families |
| --- | --- |
| `identity-platform` | `/login`, `/register`, profile/dashboard, admin users, MFA, invite, system configuration/audit/backup |
| `machine-telemetry` | administrative dashboard, traffic aggregates/rankings, telemetry-facing system information |
| `subscription` | user subscription output, subscription groups/templates/formats/settings |
| `knowledge` | user and administrator knowledge routes |
| `ticket` | user and administrator ticket routes |
| `plan` | user and administrator plans, assignments, entitlement reads |
| `order` | user and administrator orders plus coupon validation and administration |
| `payment` | payment methods, status, creation, callbacks, gateway administration, and payment records |
| `notification` | notifications, templates, delivery logs, email settings, Telegram routes |
| `proxy-node` | node registration, node CRUD/credentials, node API, UniProxy compatibility, and node auth-key issuance |
| `protocol-runtime` | protocol templates/configuration and generic Agent task, monitor, and protocol-runtime operations |
| `wireguard` | WireGuard key and peer-specific routes |
| `forward` | forwarding, tunnels, forwarding agents, forwarding traffic ingestion, and forwarding diagnostics |
| `nftables-forward`, `gost-mesh`, `nat-egress` | their declared topology/runtime routes; no undeclared v2 business handler may remain in Control |

The ownership catalog is exhaustive at the method/path level. A route with a
path parameter has one normalized Gin path and one package route id. Ambiguous
or duplicate ownership is a build failure. The catalog records the legacy
envelope kind and the router middleware group in addition to package id and
package route id.

## Runtime Flow

The compatibility registry loads routes only from verified, installed,
generation-selected package artifacts. It rejects a declaration when the
package id/version, manifest digest, compatibility-routes digest, declared
method/path, envelope, or package route id is invalid.

`router.go` retains every existing public limiter, user limiter, JWT, admin,
node-key, signature, audit, and AppToken middleware group. Its business
handler becomes `compatv2.Gateway.Serve`. The gateway supplies the normalized
method/path, request bytes, path/query values, authenticated principal,
request identity, idempotency key, deadline, and selected route generation to
`pluginhost.Manager.Dispatch`.

The gateway translates only the package response into the documented legacy
v2 envelope. A disabled, unhealthy, incompatible, missing, or stale package
returns the documented package-unavailable/incompatible response and never
calls a legacy handler, service, model, or worker as fallback.

## Delivery And Reversibility

1. Add `identity-platform` to the signed package builder, release-stage
   contract, package matrix, and formal release evidence.
2. Commit a checked ownership catalog containing all supported v2 business
   method/path pairs, owner, package route id, envelope, and middleware group.
3. Build the generic compatibility registry/gateway and contract harness
   against signed fixture artifacts. Do not replace production registrations
   until the owning package is runnable.
4. Implement package declarations and hosts in ownership waves: identity and
   platform first, then knowledge/notification, ticket/plan, order/payment,
   subscription/proxy-node, forward, and Agent/runtime packages.
5. For each wave, migrate its route registrations only after its package
   migration, host health lease, signed declaration, exact response contract,
   disable behavior, and rollback path pass.
6. Enable the final static gate only when every catalog route resolves to a
   signed package declaration and every supported v2 business registration
   points at the generic compatibility gateway or generic Kernel handler.

Intermediate development commits may retain legacy registrations for route
families whose executable package has not been implemented. A v4 candidate,
canary, or release is blocked until the final catalog is complete; it may not
claim plugin-only v2 routing while any catalog route is direct legacy code.
Each package rollout retains the prior verified generation, so package disable
or host failure fails closed rather than reverting to coupled legacy code.

## Validation

- A source-derived route inventory must equal the checked ownership catalog;
  additions, removals, duplicate method/path pairs, or unowned business paths
  fail tests.
- Each package route has an exact legacy request/response envelope contract
  test, including path parameters, query values, principal projection,
  request identity, and idempotency keys where applicable.
- Disabled, unhealthy, stale-generation, unsigned, and mismatched-declaration
  tests prove fail-closed behavior without legacy fallback.
- Static route validation parses the actual router registration source and
  fails when a catalogued v2 business route binds a direct handler instead of
  `internal/compat/v2` or the generic Kernel surface.
- SQLite and PostgreSQL migration/rollback rehearsals run for every package
  wave. Formal release evidence includes all sixteen signed package manifests,
  artifact digests, SBOMs, and trust-root verification.

## Scope Boundaries

The compatibility gateway owns no package data and does not translate domain
decisions. Package hosts own domain behavior, package migrations, and
compatibility projections. Control retains platform primitives only: identity
projection verification, authorization middleware, package trust roots,
process supervision, generic generation/rollout records, and transport.
