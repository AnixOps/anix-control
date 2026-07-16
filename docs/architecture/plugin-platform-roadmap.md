# AnixOps Plugin Platform Roadmap

Date: 2026-07-17

This document is the authoritative migration sequence from the coupled 2.x/3.0
application to the package-driven AnixOps 4.0 platform. Release numbers are
capability gates, not calendar milestones. A release cannot advance because a
route, table, or mock page exists; its exit evidence must pass.

The operational checklist, current blockers, reproducible commands, canary
rules, and rollback stop conditions are maintained in
[`upgrade-program.md`](upgrade-program.md).

## End State

AnixOps 4.0 is a microkernel with officially signed packages and a pluggable
administration UI, following the useful parts of the OpenWrt package plus LuCI
application model:

- the Control kernel owns identity, RBAC, audit, secrets, package trust,
  configuration storage, durable operations, Agent sessions, and extension
  routing;
- an installable service package owns its domain models, API handlers,
  migrations, tasks, capabilities, health contract, and optional Agent
  runtime;
- an optional WebUI companion is version-bound to the service package and
  contributes menus, routes, pages, permissions, and configuration schemas;
- installing, enabling, disabling, updating, or rolling back a package changes
  its API and WebUI contribution as one audited operation;
- a physical node has plugin-owned service assignments instead of one global
  node type;
- `/api/v2` remains a compatibility adapter through 3.5 and is removed from
  the 4.0 kernel.

The frontend is not a security boundary. A WebUI extension may only call
backend operations permitted by kernel RBAC. The first-party-only trust policy
remains in force until process isolation and a third-party review model are
explicitly designed.

## Package Model

One logical plugin release may contain these signed targets:

| Target | Responsibility |
|--------|----------------|
| `control` | Domain API, migrations, tasks, configuration and health. |
| `agent` | Node-side runtime managed by the Agent Supervisor. |
| `webui` | Admin/user pages, menu and route descriptors, and schema-driven forms. |

The release manifest binds all target versions and SHA-256 digests. A WebUI
bundle must be served from an immutable same-origin, content-addressed path.
Remote URLs, protocol-relative URLs, path traversal, inline script, and a
bundle whose digest does not match the signed release are rejected.

The kernel exposes an extension catalog derived from verified releases and
enabled installations. It never accepts an independent client-supplied menu or
route record. Disabling a plugin removes its routes, tasks, and UI contribution
but retains its data. Data deletion requires a separate audited purge command.

Package lifecycle states are:

```text
available -> staged -> installed -> enabled -> disabled
                    \-> failed       \-> updating -> rollback
```

Every transition records package/version, desired and observed revision,
operation ID, actor, result, deadline, and rollback target. Repeated delivery
must result in one side effect.

## Kernel And Extension Contracts

The non-removable kernel provides only:

- authentication, administrator RBAC, audit and secret references;
- package repository trust, signatures, dependencies, conflicts and versions;
- backend route and WebUI extension registration;
- namespaced configuration read/write with JSON Schema validation;
- durable operation dispatch, cancellation, timeout and observed state;
- service scopes, access groups, grants and node assignments;
- topology revisions, staged deployment and rollback;
- migrations and the temporary `/api/v2` compatibility gateway.

Plugin API paths and permission names are namespaced by plugin ID. Menu and
route names are also namespaced and cannot replace kernel routes. A bad or
unavailable extension must fail closed without breaking login, package
management, recovery, audit, or rollback pages.

## Version Gates

### 3.1.0 - Package And WebUI Foundation

Deliver:

- one canonical manifest and operation-envelope contract in Control and Agent;
- an official package catalog, signed release registration, installation state
  machine, dependency/conflict checks, and trust-root fingerprinting;
- a verified extension catalog plus dynamic menu/route registration;
- same-origin immutable WebUI assets, digest verification and failure isolation;
- durable lifecycle dispatch and observed-state write-back;
- `machine-telemetry` as the reference end-to-end package;
- production `/api/v2` and all data-plane behavior unchanged.

Exit evidence now includes cross-repository golden fixtures; tamper and
permission tests; install/enable/disable/update/rollback tests; Control and
Agent restart/replay tests; frontend plugin failure isolation; PostgreSQL
migration and restore rehearsal; the real Control-to-Agent process gate; and
the deterministic package release contract. Supervisor and dynamic packages
remain feature-gated until dependency-aware graph execution, topology
fan-out, staging validation, and production signing/upload evidence exist.

### 3.2.0 - Declarative Runtime And Dedicated Forwarding

Deliver `topology.plan/apply/status/diagnose/rollback`, deployment fan-out,
canary groups, and the signed `nftables-forward` package. The first production
data-plane candidate is domestic dedicated-line TCP and UDP forwarding; Agent
does not carry business traffic.

Exit evidence: network-namespace TCP/UDP tests, IPv4/IPv6 validation, atomic
nftables snapshots, partial-node failure rollback, Control/Agent double restart
recovery, and a staged 1/5/25/100 percent rollout with a legacy fallback.

### 3.3.0 - Tunnel Mesh And NAT Egress

Deliver `gost-mesh` and `nat-egress` packages for WSS, TUIC, and QUIC paths from
standard domestic entries to overseas NAT exits.

Exit evidence: certificate, MTU, UDP/QUIC, route-loop, reconnect, accounting,
exit-failure and multi-node rollback tests plus sustained canary evidence.

### 3.4.0 - WireGuard And Protocol Composition

Deliver `wireguard` and `protocol-runtime` packages, including WireGuard entry
to GOST/NAT and WireGuard entry to protocol-adapter/NAT topologies. Existing
coupled WireGuard code remains the fallback until equivalent plugin evidence
exists.

Exit evidence: peer provisioning and revocation, key rotation, MTU, policy
routing, speed and device limits, supported client imports, traffic accounting,
upgrade interruption, and composed-topology rollback tests.

### 3.5.0 - Business Plugin Migration

This release replaces the former plan to call business separation 4.0. Move
subscription, proxy-node management, plans/orders/payments, forwarding,
tickets, notifications and content into official packages. Each migration uses
shadow reads, dual-write or change capture where required, reconciliation, and
an independently reversible ownership switch. `/api/v2` becomes an adapter to
plugin services rather than a direct legacy implementation.

Exit evidence: old/new contract comparison, row and monetary reconciliation,
subscription byte comparison, traffic-counter isolation, payment idempotency,
disable/re-enable data retention, and rollback under production-like load.

### 4.0.0 - Plugin-Only Platform

Remove coupled business handlers, direct legacy workers, global node-type
semantics, and obsolete tables only after every installed default package has
passed the 3.5 observation window. The kernel boots with no business package,
then installs the default official package profile through the same package
manager used by operators.

Exit evidence: clean-database bootstrap; upgrade from the final 3.5 release;
backup/restore; install/uninstall/reinstall of every default package; no kernel
imports of plugin domain packages; no production request reaching legacy
handlers; and a tested full rollback before the destructive migration is
approved.

## Reproducibility And Promotion

Every candidate records the source commit, toolchain and lock files, generated
contract versions, migration checksums, trust-root key ID, package and bundle
digests, signatures, test reports, canary observations, and rollback evidence.
Promotion always follows development, staging, single-node canary, rollout
group, then full deployment. Failed health or reconciliation stops expansion
and rolls changed nodes back before another revision may be promoted.
