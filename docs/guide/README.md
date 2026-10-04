# AnixOps Control Guide

This folder is the deep-dive layer of the documentation tree.

Start elsewhere first when possible:

- overview, v4 architecture, and boundary: [`../intro/README.md`](../intro/README.md)
- startup and config templates: [`../reference/README.md`](../reference/README.md)
- repository ownership: [`../reference/repository-layout.md`](../reference/repository-layout.md)
- runtime entrypoint: [`../reference/runtime.md`](../reference/runtime.md)

Use this folder when you need implementation detail, clone contracts, runtime operations, runbooks, or smoke-test checklists.

## Documents

### Flux-panel Clone

| Document | Purpose |
|------|------|
| [Flux-panel Clone Guide](flux-panel-clone.md) | Source-of-truth workflow, current clone status, remaining gaps, and next clone order for `flux-panel` 1:1 cloning work |
| [Flux Forward Contract](flux-forward-contract.md) | Concrete forward/tunnel endpoint mapping, DTO fields, auth scope and known gaps |

### Forwarding Runtime

| Document | Purpose |
|------|------|
| [Forward Relay Onboarding](forward-relay-onboarding.md) | Exact operator checklist for making a relay actually join the runtime |
| [Forward Runtime Operations](forward-tunnel-runtime-ops.md) | Runtime ownership, evidence chain, and what counts as a real relay attachment |
| [Forward/Tunnel Manual Smoke Tests](forward-tunnel-smoke-test.md) | Manual and real-machine proof steps for `gost`, `nftables_ansible`, and legacy `iptables_ansible` |
| [NodeX Internal Extension Boundary](nodex-internal-extension.md) | Where the internal NodeX and local ansible execution planes stop and the public Flux surface begins |

### Install, Upgrade, And Migration

| Document | Purpose |
|------|------|
| [Panel Release Installation](release-installation.md) | Tag-pinned GitHub Release installation, update, rollback, and native service operations without cloning/building on the host |
| [V4 Plugin-Only Upgrade](v4-plugin-only-upgrade.md) | Preconditions and steps for moving to the formal `v4.0.0` package-only release |
| [V4 Plugin-Only Rollback](v4-plugin-only-rollback.md) | Rollback triggers and restoring a previous verified package generation |
| [Release Root Rotation](release-root-rotation.md) | Rotating the official Ed25519 package signing root, its GitHub secrets, and re-signed package import |
| [Control In-Place Migration](control-migration.md) | Preflight, plan, and apply steps for migrating an existing AnixOps SQLite install in place |
| [Legacy Panel Migration](legacy-migration.md) | Supported upgrade paths, foreign-panel migration boundaries, coordinated node cutover, and rollback evidence |
| [Staging Rehearsal](staging-rehearsal.md) | Local Compose stack with synthetic data to rehearse route cutovers (legacy → shadow → native) batch by batch, read the report, sign off and roll back |

### Nodes, Subscriptions, And Clients

| Document | Purpose |
|------|------|
| [Node Management](node-management.md) | Node registration, heartbeat, protocol config and operations |
| [Installing The Agent With One Command](agent-onboarding.md) | Single-use install tokens, the signed `install.sh`, mirrors, re-runs and the legacy forward cleanup |
| [Subscription System](subscription-system.md) | Subscription groups, templates and formatting |
| [Client Compatibility](client-compatibility.md) | V2bX/XrayR and related compatibility notes |
| [Loon Subscriptions](loon-subscriptions.md) | User-Agent detection and native Loon node syntax output |
| [Loon WireGuard](loon-wireguard.md) | Loon-specific WireGuard endpoint compatibility handling |
| [Mihomo WireGuard](mihomo-wireguard.md) | Mihomo/Clash WireGuard output differences such as omitted `persistent-keepalive` |
| [API Reference](api-reference.md) | Existing project API overview |

### WireGuard

| Document | Purpose |
|------|------|
| [WireGuard Dual-Node Relay Plan](wireguard-relay.md) | P0 WireGuard access, domestic entry termination, GOST relay+QUIC default transport, WSS compatibility mode, and phased implementation plan |
| [WireGuard Entry Network Policy](wireguard-network-policy.md) | `relay.network_policy` active/standby uplink failover for WSS entry nodes |
| [WireGuard Peer Key Rotation](wireguard-key-rotation.md) | `cmd/wgrotate` client credential rotation without changing peer IDs or tunnel IPs |

## Recommended Reading Order

If the task is to continue cloning `flux-panel`:

1. Read [Flux-panel Clone Guide](flux-panel-clone.md), including its current status and gap sections.
2. Read [Flux Forward Contract](flux-forward-contract.md).
3. Inspect the upstream reference repository at <https://github.com/bqlpfy/flux-panel>.
4. Only then start changing routes, services, DTOs or pages.

If the task is not related to `flux-panel`, use the other topic-specific guides.

If the task is to validate or operate forwarding runtime:

1. Read [`../reference/runtime.md`](../reference/runtime.md).
2. Read [Forward Relay Onboarding](forward-relay-onboarding.md).
3. Read [Forward Runtime Operations](forward-tunnel-runtime-ops.md).
4. Run [Forward/Tunnel Manual Smoke Tests](forward-tunnel-smoke-test.md).
