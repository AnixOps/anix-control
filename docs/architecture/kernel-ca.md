# Kernel CA and Agent PKI

Status: implemented (`internal/modulepki`, `internal/agentpki`,
`sdk/moduletls`).

The kernel runs one built-in certificate authority. It signs the client
certificates of AnixOps Agents and the kernel's own identity, and a second
root sealed with the same key signs forward link certificates. The
configuration keys keep their historical `module_runtime.` prefix
(`ca_kek`, `pki`, `cluster`, `cert_lifetime`), so existing installations
need no change.

## Kernel CA

- **Key and storage** (`internal/modulepki`).
  - The CA key is ECDSA P-256, sealed with AES-GCM under the root key
    (`module_runtime.ca_kek`, `ANIX_CONTROL_MODULE_RUNTIME_CA_KEK`, 32
    bytes as base64 or hex, adopted into `root.key`) and stored in
    `v4_kernel_service_ca`.
  - The CA certificate carries a name constraint: only `spiffe://anixops/...`
    URIs can chain to it.
  - The kernel creates the CA at startup (under the bootstrap lock) when
    `module_runtime.pki` is `builtin` and the key is set.
- **Rotation.** The CA keeps a `current` and a `next` CA. The trust bundle
  carries both, and issuing switches to `next` only after one full
  certificate lifetime; the kernel's maintenance loop promotes and retires
  CAs by itself.
  - Admin API: `POST /api/v4/kernel/modules/ca/rotate`.
  - CLI: `anix-control module ca rotate`; `anix-control module ca bundle`
    prints the trust bundle.
- **Identities.** Every leaf carries one URI SAN in the `anixops` trust
  domain, such as `spiffe://anixops/<cluster>/kernel`. The kernel ignores
  whatever subject or names a CSR asks for.
- **External CA (`module_runtime.pki: external`).** The kernel holds no CA
  key, so it issues no certificates and Agent enrollment is off.

## Agent PKI

Status: implemented in the kernel (`internal/agentpki`; A2-1 of the node
operations design, `node-ops-service.md` section 5.3). The anix-agent side
(enroll, store, renew, dial with mTLS) is AG-2.

The kernel CA above signs the client certificates of AnixOps Agents. Agents
get them from `anix.agent.v1.AgentEnrollment` on the node-facing gRPC
listener (`grpc.*`, port 50051).

- **Identity.** One URI SAN names the node:
  `spiffe://anixops/<cluster>/agent/proxy-<id>` for a `v5_node` row or
  `.../agent/forward-<id>` for a `v5_forward_node` row. Proxy and forward
  ids overlap, so the kind is part of the name. The kernel ignores the
  CSR's subject and SANs and issues the identity of the node the bootstrap
  credential authenticated.
- **Issuer and lifetime.** The kernel CA, with its `current`/`next`
  rotation. Agent certificates last 7 days (decision D9) and are renewed at
  two thirds of the lifetime with `AgentEnrollment.Renew`, authenticated by
  the current certificate. Retired CAs stay trusted for agent certificates
  until the last one they signed has expired.
- **Forward link CA (H28).** The same `module_runtime.ca_kek` also seals
  a second, separate root: the forward link CA
  (`v4_kernel_forward_link_ca`, `internal/agentpki/link.go`), created at
  startup with the kernel CA. It signs only the link certificates forward
  engines present to each other (`AgentEnrollment.IssueLinkCertificate`),
  never kernel or agent certificates, and the kernel CA never signs link
  certificates. `anix-control agent link-ca list|bundle|rotate`
  administers it (`forward-sdk.md` section 6.2).
- **Bootstrap** (`AgentEnrollment.Enroll`, without a client certificate):
  1. *New nodes* use a one-time enrollment credential (`anixagt_...`) bound
     to a node, at most 7 days (default 24 h), stored hashed in
     `v4_kernel_agent_enrollment`, in `enrollment_credential`. Node API keys
     and forward tokens no longer enroll.
  2. *Node groups* (5.0): `AgentEnrollment.Join` takes a group's join
     token (`anixjoin_...`), creates a node of the group or rejoins the
     machine's earlier node, and issues its certificate, enrollment method
     `join_token` (`internal/nodegroup`; `sdk/api/agent/v1/PROTOCOL.md`,
     "Joining a node group"). The token is stored as its SHA-256 and sealed
     with AES-256-GCM under a key derived (HKDF-SHA256, info
     `anixops node-group join token v1`) from `module_runtime.ca_kek`, so a
     super administrator can show the install command again.

  A one-time credential is consumed atomically: of two concurrent
  enrollments exactly one succeeds, and a failed issuance leaves it unused.
  Rejections share one answer.
- **Administration.** Both print the credential once; the kernel keeps its
  SHA-256.
  - Admin API: `POST /api/v4/kernel/agents/enrollment-tokens` with
    `{"node":"proxy-12","ttl_seconds":86400}` (`ttl_seconds` optional, at
    most 604800).
  - CLI: `anix-control agent token create -node proxy-12 [-ttl 24h]`.
  - Every credential issue and every enrollment writes an operation log
    entry (`v5_operation_log`, module `agent_pki`, actions
    `agent_enrollment_token_issue` and `agent_enroll`) without the
    credential.
- **Listener.** The listener keeps its public TLS certificate
  (`grpc.tls_cert_file`); agents verify it as before. With the agent PKI
  on, the handshake asks for a client certificate but never requires one;
  the Agent services and the v2board interceptors verify a presented
  certificate against the agent trust bundle, its identity and its
  revocation state. An invalid, expired, revoked or foreign certificate
  (the kernel's, another cluster's) is refused, never
  downgraded to the legacy credential.
  - The stream takes its node from the certificate, for proxy and forward
    nodes. Every envelope's `node_id` must name it, and `x-node-id`, when
    sent, too. Forward-node streams live in their own connection manager;
    operations for them arrive with KernelNodeOps (NO-6, NO-7).
  - On the v2board services only a proxy node's certificate is accepted,
    and every request's `node_id` must name the certificate's node.
  - A certificate revoked while a stream is open ends the stream at its
    next heartbeat.
- **Mode** `agent_control.mtls` (`ANIX_CONTROL_AGENT_CONTROL_MTLS`):

  | Mode | Without a client certificate |
  |---|---|
  | `off` | the legacy node credential authenticates; no client certificate is requested or accepted, and `Enroll` is unavailable |
  | `optional` | the legacy node credential authenticates, as before |
  | `preferred` (the 4.1 default) | the same, and the control stream answers it with the header `x-anix-auth-deprecated` (the legacy HTTP agent paths with `Deprecation`, `Sunset` and `Link`) |
  | `required` (default from 4.2) | the Agent services refuse the call (`agent_mtls_required`); `Enroll` accepts only enrollment credentials. The v2board services keep the legacy credential for third-party node software (the F3 decision) |

  `required` needs `grpc.tls_cert_file`/`grpc.tls_key_file` and the built-in
  CA; set explicitly, the kernel refuses to start without them. Left empty
  (the v4.2 default) it starts, refuses legacy agents and warns that none can
  enroll. The transition (decision
  H5, the transport inventory, the signals) is in
  [`node-ops-service.md`](node-ops-service.md) section 5.6.
- **Revocation.** Every certificate is recorded in
  `v4_kernel_agent_certificate` (serial, node kind and id, enrollment,
  issuer, `not_after`, `revoked_at`). Disabling a proxy node, replacing a
  forward node's token, disabling a forward node and deleting either node
  revoke the node's certificates and its unused enrollment credentials in
  the same transaction. The listener answers revocation from a cache of at
  most 30 s; revocations in the same process apply at once. Expired records
  are pruned hourly.
- **Requirements.** Agent enrollment needs only the built-in CA:
  `module_runtime.ca_kek` with `module_runtime.pki: builtin` (the default);
  `module_runtime.cluster` names the cluster in agent SPIFFE IDs. With
  `pki: external` the kernel
  holds no CA key, so agent enrollment is off (`optional` only; the other
  modes refuse to start). Without the CA, `Enroll` answers
  `FailedPrecondition`, client certificates are not requested, and
  `optional` behaves exactly as before. Client certificates need the
  listener's own TLS: behind a TLS-terminating proxy they do not reach
  Control.

## History

Until the 2026-11 slimming audit (PR 2d, `docs/audit/slim-2026-11.md`) this
document also described network modules: packages running as their own
containers that enrolled with the kernel CA (`ModulePKI`), bound to a kernel
module listener (`module_runtime.listen`, `:7443`) and served the host
protocol over mTLS. Production never enabled that runtime, and it was
removed: every package runs inside Control (`plugin-kernel-contract.md`).
The protobuf contract `sdk/api/modulepki/v1` is retired with it
(`config/scripts/check_proto_golden.py`, `RETIRED_PACKAGES`). The
`KernelPackageBridge.Bind`/`Heartbeat` RPCs stay in the append-only contract
golden file (`contracts/proto/descriptors.golden`), unserved, because their
package is still live.
