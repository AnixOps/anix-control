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
  - `sdk/api/pluginhost/v1` `ControlPackageHost` (kernel → module);
  - `sdk/api/packagebridge/v1` `KernelPackageBridge` (module → kernel);
  - `sdk/api/modulepki/v1` `ModulePKI`;
  - `sdk/api/identity/v1` `IdentityService`;
  - `sdk/api/kernelidentity/v1` `KernelIdentity`.
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
existing tables. The same module binary supports both; `sdk/modulesdk` picks
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

Existing packages keep the `local` runtime. `remote` is opt-in per package:
- `PUT /api/v4/kernel/modules/runtimes/:plugin_id` with `{"runtime":"remote"}`
  selects it; `GET /api/v4/kernel/modules/runtimes` lists the choices.
- Remote requires `module_runtime.enabled` and PostgreSQL.
- The change applies at the package's next lifecycle operation.

Remote hosts live inside the same host `Supervisor` as local ones.
- **Instance pool.** Their client spreads calls over the instances bound to
  the generation:
  - calls go round-robin;
  - an instance whose call fails at the transport is ejected for 10 s;
  - health is cached for 2 s, so the per-frame WebSocket checks stay off
    the network.
- **Shared code.** Dispatch, WebSocket relays, migrations through the
  ledger, health polling and fencing use the same code for both
  runtimes.

## PKI

Status: implemented (`internal/modulepki`, `sdk/moduletls`); the listener
that serves `ModulePKI` arrives with `Bind`.

- **Built-in CA** (`internal/modulepki`).
  - The CA key is ECDSA P-256, sealed with AES-GCM under
    `module_runtime.ca_kek` (`ANIX_CONTROL_MODULE_RUNTIME_CA_KEK`, 32 bytes
    as base64 or hex) and stored in `v4_kernel_service_ca`.
  - The CA certificate carries a name constraint: only `spiffe://anixops/...`
    URIs can chain to it.
  - The kernel creates the CA at startup (under the bootstrap lock) when
    `module_runtime.enabled` is true and `module_runtime.pki` is `builtin`.
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
  certificate). Credentials (`anixenr_...`) are stored hashed in
  `v4_kernel_module_enrollment`.
  - One-time credentials last at most 7 days and are consumed atomically.
  - Reusable ones last at most 366 days.
  - Rejections share one answer, so credentials cannot be probed.
- **Administration.** Both interfaces print the credential once; the admin
  API routes are audited.
  - Admin API:
    - `POST /api/v4/kernel/modules/enrollment-tokens`
      (`{"package_id","ttl_seconds","reusable"}`);
    - `GET` lists enrollments;
    - `DELETE .../:id` revokes an enrollment;
    - `POST /api/v4/kernel/modules/ca/rotate`.
  - CLI:
    - `anix-control module token create -package <id> [-ttl 1h] [-reusable]`;
    - `anix-control module token list`;
    - `anix-control module token revoke <id>`;
    - `anix-control module ca rotate`.
- **Revocation.** Every issued certificate is recorded in
  `v4_kernel_module_certificate`. Revoking an enrollment revokes its
  certificates: they cannot renew, and the listener rejects their serials.
  - Compose and VMs: a one-time token plus a persistent certificate volume.
  - Kubernetes: a revocable, package-bound bootstrap credential in a Secret,
    so a restarted pod can enroll again.
- **External CA (optional, `module_runtime.pki: external`).** The kernel
  loads `trust_bundle_file`, `cert_file` and `key_file`, for example from
  cert-manager, and re-reads them when they change. It then checks SANs
  only, and `Enroll`/`Renew` are disabled.

## Bind, sessions and fencing

Status: implemented in the kernel.
- **`internal/moduleruntime`** is the listener, enabled by
  `module_runtime.enabled` and `module_runtime.listen` (default `:7443`). It
  requires a verified, unrevoked client certificate for every RPC except
  `Enroll`.
  - A revocation by this kernel applies to the next call.
  - Revocation state is cached for at most 30 s.
- **`packagebridge.ModuleBridge`** handles Bind and sessions.
- **`packagebridge.GenerationSession`** is the per-generation state.
- **Session token.** The token travels in `x-anix-bridge-session` as unpadded
  base64url.
- **Binder.** The host `Supervisor` is the binder. It admits an instance only
  to the starting or active remote generation of its package at the same
  version.

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
   `x-anix-bridge-session`. The kernel accepts it only from a client
   certificate with the same module identity, so a token stolen by another
   module is useless.
   - The pin is the identity, not one certificate, because instances renew
     their certificates while a session lives.
   - Rebinding the same instance id replaces its session.
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

- **Install / update.** `Start` registers the new generation for binding and
  waits (`module_runtime.bind_timeout`, default 2 m) until an instance has
  bound and answers `Health` with the lease it bound with. Only then does it
  retire the previous host.
  - **New version.** The previous generation keeps serving while the new
    pods start.
  - **Same version** (re-enable, restart reconciliation). The previous
    generation is fenced first, so its instances rebind to the new one
    within a heartbeat.
  - **Migrations.** Package migrations then run through one instance, with
    the same ledger as local hosts.
- **Disable / stop.** Fence the generation, then `Drain` every instance.
- **Resume.** `ControlPackageHost.Resume` undoes a drain. Remote processes
  cannot be restarted by the kernel, so drains are reversible.
- **Watchdog.** Heartbeats plus a 10 s `Health` per instance. Unhealthy
  instances are ejected from routing; the platform restarts them.
- **WebSocket relays.** Relays to remote instances stop on events (instance
  fenced, heartbeat expired, stream error) plus gRPC keepalive, instead of the
  per-frame `Health` call that local hosts use.

## Storage

Remote modules require PostgreSQL; a remote lease on SQLite is refused.
- **Shared lease.** The kernel caches one `LeaseStorage` lease per package
  generation and grants. Every lease rotates the role password, and the
  replicas of a generation share one role, so a new replica reuses the
  cached lease instead of invalidating the others.
- **Re-leasing.** The SDK leases again when PostgreSQL rejects the password
  (`28P01`).
- **Database address.** `module_runtime.database_host` (`host` or
  `host:port`) is the database address in remote leases when it differs
  from the kernel's.

## Running a module

`sdk/modulesdk.Run` starts a host in either runtime. The official hosts
(`packages/shared/controlhost`, `packages/identity-platform/control`) use it:
- **Local.** Without `ANIX_MODULE_MODE`, the host expects the environment the
  kernel gives a child process.
- **Remote.** With `ANIX_MODULE_MODE=remote` it runs as a network module:
  - it enrolls, or loads its stored certificate, and renews it at the renew
    time; after expiry it enrolls again;
  - it serves `ControlPackageHost` over mTLS and accepts only the kernel's
    identity;
  - it binds, heartbeats and binds again after a fence or a kernel restart,
    and resumes a drained package when it binds;
  - it answers `/livez` and `/readyz` (bound and not shutting down);
  - on SIGTERM it fails readiness, stops heartbeating, keeps serving for
    the shutdown grace, then stops gracefully.

| Variable | Default | Meaning |
|----------|---------|---------|
| `ANIX_MODULE_MODE` | (local) | `remote` for a network module |
| `ANIX_MODULE_KERNEL_ADDR` | required | kernel module listener, `host:port` |
| `ANIX_MODULE_CLUSTER` | `default` | SPIFFE cluster |
| `ANIX_MODULE_LISTEN_ADDR` | `:7000` | host protocol listener |
| `ANIX_MODULE_ADVERTISE_ADDR` | instance id and listen port | address the kernel dials (use the pod IP) |
| `ANIX_MODULE_INSTANCE_ID` | host name | unique per process (pod name) |
| `ANIX_MODULE_CERT_DIR` | `/var/lib/anix-module/certs` | stored key, certificate and trust bundle (0600) |
| `ANIX_MODULE_TRUST_BUNDLE_FILE` | required with the built-in PKI | kernel CA bundle: `anix-control module ca bundle` or `GET /api/v4/kernel/modules/trust-bundle` |
| `ANIX_MODULE_ENROLL_CREDENTIAL_FILE` | | enrollment credential from `anix-control module token create` |
| `ANIX_MODULE_CERT_FILE`, `ANIX_MODULE_KEY_FILE` | | externally issued certificate (cert-manager); enables the external mode |
| `ANIX_MODULE_HEALTH_ADDR` | `:8081` | `/livez` and `/readyz` |
| `ANIX_MODULE_SHUTDOWN_GRACE` | `20s` | serving time after SIGTERM |
| `ANIX_MODULE_IMAGE_DIGEST` | | reported at Bind (advisory) |

**Storage in remote modules.** New connections fetch the current storage
lease from the kernel before they connect, so a pool picks up a rotated role
password instead of failing.

## SDK module

The contracts and SDKs form their own Go module,
`github.com/AnixOps/anix-control/sdk`, in `sdk/`:

- **Contracts:** `sdk/api/{pluginhost,packagebridge,modulepki,identity,kernelidentity}/v1`.
- **SDKs:** `sdk/moduletls`, `sdk/pluginhostsdk`, `sdk/packagebridgesdk`,
  `sdk/modulesdk`, `sdk/packagestoresdk`, `sdk/v2compat`.

**Independence from the kernel.** The module has no dependency on the kernel
module: Go forbids it from importing the kernel's `internal/` packages, and
`check_package_boundaries.sh` also rejects any dependency on
`github.com/AnixOps/anix-control/v4`. Tests that exercise the SDK against the
kernel's real bridge live in the kernel module (`internal/tests/bridgecontract`).

**Consumers.** Other AnixOps services depend only on this module, never on
the kernel:
- It is versioned by tags of the form `sdk/vX.Y.Z` and fetched with
  `go get github.com/AnixOps/anix-control/sdk@vX.Y.Z` (set `GOPRIVATE` while
  the repository is private).
- Wire compatibility of the contracts is guarded by
  `internal/tests/protocompat`.
- If a second consumer or a separate release cadence makes it worthwhile,
  the directory can move to its own repository unchanged.

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
5. `sdk/modulesdk`.
6. Module images, Compose and Helm, kind smoke test.

The identity work (7–13) is in [`identity-service.md`](identity-service.md).
