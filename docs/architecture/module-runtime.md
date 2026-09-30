# Module Runtime: Network Modules

Status: DESIGN (2026-09-30). Contracts are defined; the implementation lands in
the PRs listed under [Delivery](#delivery). The plan of record is identity
first: see [`identity-service.md`](identity-service.md).

## Goal

Every core component, including login, runs as a module. Modules are
separately deployable network services and talk to the kernel and to each
other only through contracts:

- **AnixOps protocols** (gRPC/protobuf, guarded by
  `internal/tests/protocompat` and `contracts/proto/descriptors.golden`):
  - `api/pluginhost/v1` `ControlPackageHost` (kernel → module);
  - `api/packagebridge/v1` `KernelPackageBridge` (module → kernel);
  - `api/modulepki/v1` `ModulePKI`;
  - `api/identity/v1` `IdentityService`;
  - `api/kernelidentity/v1` `KernelIdentity`.
- **Public protocols:**
  - the v2 HTTP API (v2board compatible) behind the kernel gateway;
  - JWT (EdDSA) and JWKS for access tokens;
  - Prometheus metrics.

The kernel keeps a narrow set of jobs:
- package trust, signing and lifecycle;
- the HTTP gateway, which verifies tokens but does not issue them;
- the module PKI;
- audit, health and migration orchestration;
- subscriber and entitlement storage until those move into their own module.

## Runtimes

Each Control installation runs in one of two runtimes. The runtime is stored in
a new table, `v4_kernel_plugin_runtime`, because production never alters
existing tables. The same module binary supports both; `pkg/modulesdk` picks
the transport from the environment.

| | `local` (current) | `remote` (new) |
|---|---|---|
| Process | child of the kernel | own container or Kubernetes Deployment, any number of replicas |
| Kernel → module | gRPC over a private Unix socket | gRPC over TCP with mTLS; the kernel dials the addresses of bound instances |
| Module → kernel | socketpair inherited as FD 4 | gRPC over TCP with mTLS to the kernel module listener (`module_runtime.listen`, e.g. `:7443`) |
| Identity of the module | the socketpair the kernel created | client certificate URI SAN plus a `Bind` session token |
| Start / stop | the kernel execs and signals the process | the platform (Compose, Kubernetes) runs the process; the kernel binds, fences and drains |
| Integrity | the kernel executes only bytes it hashed | signed manifest pins the image digest (`control_runtime`); enforced by cosign and an admission policy, not by the kernel |
| Storage | PostgreSQL role or shared SQLite file | PostgreSQL only |

Existing packages keep the `local` runtime. `remote` is opt-in per installation.

## PKI

- **Built-in CA** (`internal/modulepki`).
  - The CA key is ECDSA P-256, sealed with AES-GCM under
    `ANIX_CONTROL_SERVICE_CA_KEK` and stored in `v4_kernel_service_ca`.
  - Rotation keeps a `current` and a `next` CA. The trust bundle carries
    both, and issuing switches to `next` only after one full certificate
    lifetime.
- **Certificates.**
  - Lifetime is 24 h. Modules renew at two thirds of the lifetime through
    `ModulePKI.Renew`, authenticated by the current certificate.
  - URI SAN: `spiffe://anixops/<cluster>/module/<package-id>` for a module and
    `spiffe://anixops/<cluster>/kernel` for the kernel. The kernel ignores
    whatever subject a CSR asks for.
- **Enrollment** (`ModulePKI.Enroll`, the only RPC allowed without a client
  certificate). Credentials are stored hashed in `v4_kernel_module_enrollment`
  and issued through `POST /api/v4/kernel/modules/enrollment-tokens` or
  `anix-control module token create`; both are audited.
  - Compose and VMs: a one-time token plus a persistent certificate volume.
  - Kubernetes: a revocable, package-bound bootstrap credential in a Secret,
    so a restarted pod can enroll again.
- **External CA (optional).** The kernel loads a trust bundle and its own
  certificate from files, for example from cert-manager. It then checks SANs
  only, and `Enroll`/`Renew` are disabled.

## Bind, sessions and fencing

1. **Bind.** A remote instance calls `KernelPackageBridge.Bind` with:
   - package id and version;
   - instance id (the pod name) and the address to dial;
   - its per-process lease id and its reported image digest.
2. **Checks.** The kernel requires that:
   - the SAN's package equals `package_id`;
   - the installation is enabled with the `remote` runtime;
   - the version equals the desired version.

   It then returns a session token and the current route generation.
3. **Later calls.** Every other bridge RPC carries the token as
   `x-anix-bridge-session`. The kernel accepts it only with the certificate
   fingerprint pinned at Bind, so a stolen token is useless elsewhere.
4. **Heartbeat.** Sent every 5 s; a session expires after 15 s without one. A
   heartbeat answer reports `fenced` and `draining`.

The bridge session becomes transport-independent (`GenerationSession`); the
socketpair is one transport of it.
- **Capabilities.** The per-request capabilities minted for `Dispatch` stay
  one-shot and kernel-held. They are stored per generation, so any bound
  instance of that generation can redeem them.
- **Generation change.** A new lifecycle generation invalidates every session
  of older generations (`PermissionDenied`). The existing database fence on
  `LifecycleGeneration` still applies to session-scoped RPCs.

## Lifecycle without signals

- **Install / update.** The kernel keeps routing to generation G until at
  least one G+1 instance has bound. It then runs the package migrations
  through one G+1 instance (the same ledger as local hosts) and switches
  routing to G+1.
- **Disable / stop.** Fence the generation, then `Drain` every instance.
- **Resume.** `ControlPackageHost.Resume` undoes a drain. Remote processes
  cannot be restarted by the kernel, so drains are reversible.
- **Watchdog.** Heartbeats plus a 10 s `Health` per instance. Unhealthy
  instances are ejected from routing; the platform restarts them.
- **WebSocket relays.** Relays to remote instances stop on events (instance
  fenced, heartbeat expired, stream error) plus gRPC keepalive, instead of the
  per-frame `Health` call that local hosts use.

## Storage

Remote modules require PostgreSQL. `LeaseStorage` credentials are cached per
generation, because every lease rotates the role password and several pods
share one role. The SDK leases again when PostgreSQL rejects the password
(`28P01`). `module_runtime.database_host` sets the database address modules
use when it differs from the kernel's.

## Trust boundary

For `local` hosts the kernel proves what runs: it executes only the entrypoint
bytes it verified. For `remote` modules it proves only who connects: the
client certificate and `Bind`. What runs is enforced outside the kernel:
- the signed manifest pins the OCI image digest in `control_runtime`;
- images are cosign-signed in CI;
- clusters enforce signatures with an admission policy (Kyverno or the
  sigstore policy-controller);
- NetworkPolicies allow module ingress only from the kernel, and module egress
  only to the kernel, PostgreSQL and DNS.

## Limits of this round

- The kernel stays single-replica. Sessions, capabilities, the revocation
  cache and agent streams are in kernel memory; see
  [`container-deployment.md`](container-deployment.md#still-single-instance-next-round-ha).
- Modules may run several replicas.
- Everything is additive: new RPCs, fields and tables only. v4.0.0 hosts and
  their strict request-metadata decoding keep working in the `local` runtime.

## Delivery

1. Contracts and this document.
2. `internal/modulepki`.
3. Module listener, `Bind`, `GenerationSession`.
4. Remote runtime manager.
5. `pkg/modulesdk`.
6. Module images, Compose and Helm, kind smoke test.

The identity work (7–13) is in [`identity-service.md`](identity-service.md).
