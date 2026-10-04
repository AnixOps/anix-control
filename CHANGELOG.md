# Changelog

## Unreleased

### Added

- **v4.2 forwarding admin UI** (F5b, H16 approved 2026-10-04 with D1–D15).
  New pages in the core app, lazy-loaded and shown only when the forward
  package serves the v4 API: 概览 (`/admin/forward/overview`), 路由
  (`/admin/forward/routes`, with the full-page editor and the route
  detail) and 节点 (`/admin/forward/inventory`). The flux-clone pages stay
  as 转发（旧版） and 转发节点（旧版） until F5d.
  - Route list with client-side status, engine, node and label filters,
    24-hour entry traffic from one `GET /stats`, bulk pause, resume and
    delete (per-route calls, four at a time, each with its own
    `Idempotency-Key`, retry and undo), and JSON import and export.
  - Route editor: hop chain builder (keyboard reordering, link and engine
    options explained, link fix actions), targets, policy, limits, labels,
    violations mapped to their fields, and a plan preview 1 s after typing
    stops (in-flight previews cancelled). One `Idempotency-Key` per save
    attempt is reused on retry; `409 revision_conflict` keeps the form and
    offers a diff and reapply.
  - Route detail: per-hop health, breakers and ports, a node dot scoped to
    the route, hourly traffic, node states, configuration, and 诊断, which
    shows `POST /routes/{id}/diagnose` by stage.
  - Node inventory and node page: health, hosted hops, engines, forwarding
    settings (`PUT /nodes/{ref}/settings`), the Agent install sheet,
    disable and delete. Proxy nodes show 「代理节点」, and the proxy node page
    offers 加入转发清单….
  - The anixops engine is offered only with the experimental flag, which
    Control does not serve yet (F6), so it stays hidden except on routes
    that use it.
  - Operator guide: `docs/guide/forwarding.md`. The dev-only mockups are
    removed; their data is the e2e fixture.
- **Forward v4 API: `can_delete` and quiet previews** (F5b D7, D13).
  `GET /api/v4/forward/routes`, `/nodes` and `/ansible-machines` answer
  `can_delete` (the super administrator rule, passed by the kernel as the
  principal's `super_admin`). `POST /routes/preview` is no longer written
  to the audit log. The extension catalog (`GET /api/v3/extensions`) lists a
  package's `control_routes` to actors who may call them.

- **Forward route diagnosis** (F3c, forward-sdk.md section 7.6).
  `ForwardControl.DiagnoseRoute` now answers instead of `UNIMPLEMENTED`;
  `POST /api/v4/forward/routes/{id}/diagnose` (administrators, audited as
  `forward/diagnose`) and `anix-control forward routes diagnose <id>` serve
  it.
  - Stages: Control's records (each node's desired against reported
    generation, applied, hop errors; upstream health and circuit breakers
    from the latest reports), then node probes through the
    `agent.diagnostic` operation on the Agent Control stream for nodes whose
    Agent advertises `agent.diagnostic` and `diag.v1` (`forward.listen`,
    `forward.port_conflict` including foreign nat-table rules,
    `forward.connect` to the next hop, delivery from the last hop to the
    targets, and `forward.udp_probe` for UDP routes, where no reply is
    inconclusive), then dials from Control to the entries and public
    targets for the nodes that cannot probe. Each step carries its stage,
    vantage, target, verdict (`OK`, `FAILED`, `INCONCLUSIVE`, `SKIPPED`) and
    a stable code.
  - Public targets only: Control never dials a private target, and a last
    hop is asked to dial a private target only on an administrator's
    `TARGET_POLICY_ALLOW_PRIVATE` route.
  - 15 s by default (`timeout_ms`, at most 25 s). A diagnosis of the same
    route from the last 10 seconds is answered again (`cached`), and at most
    4 run at once per Control process (`429 rate_limited`).
  - Contract additions only: `ProbeKind` `CONFIG`, `HEALTH`,
    `PORT_CONFLICT`; `ProbeStatus` and `DiagnoseVantage`;
    `ProbeResult.status` and `code`; `DiagnoseStep.vantage`, `target` and
    `protocol`; `DiagnoseRouteResponse.route_id`, times, `cached` and
    `nodes` (`DiagnoseNode`).
  - The forward checks are refused by the administrator diagnostic routes
    and the KernelNodeOps `agent.diagnostic` kind. The Agent side, which
    must run them, is described in `sdk/api/agent/v1/PROTOCOL.md`,
    "Forward diagnostic checks"; until an Agent advertises `diag.v1` its
    node steps are `SKIPPED` (`node_vantage_unavailable`).

- **Staged Agent upgrades from Control** (onboarding O4, owner decision H19;
  `docs/architecture/forward-sdk.md` section 9, `PROTOCOL.md` "Agent
  upgrades").
  - A campaign pushes a signed Agent release (from
    `agent_install.artifact_dir`, both signatures and `SHA256SUMS` verified
    with `plugins.official_public_key`) to every enabled node whose Agent was
    seen on the Agent Control stream, in batches of 5%, 25% and 100% of the
    nodes (canaries first, by node name hash), each lasting at least
    30 minutes. A batch in which more than 5% of the offered nodes fail —
    refused, failed to apply, or no reconnect with the new version in
    `Hello` within 10 minutes — is rolled back automatically: the campaign
    stops and the batch's upgraded nodes are told to reinstate their
    previous release. Nodes whose Agent cannot be upgraded, or that stay
    offline for their batch, are skipped and do not count.
  - New capability `upgrade.v1` (offered by the intersection rule to proxy
    and forward nodes) and the `agent.upgrade` operation with the payload
    `anixops.agent-upgrade/v1` (`sdk/agentcontrol.UpgradeRequest`), sent
    only on sessions that negotiated it and never by packages. No protobuf
    change.
  - `POST /api/v4/kernel/agents/upgrades` (super administrators, audited),
    `GET` and `GET /:id`, `POST /:id/pause`, `/:id/resume`, `/:id/abort`
    (`rollback`); `anix-control agent upgrade start|status|pause|resume|abort`;
    the campaign's batches on the Agent transports page.
  - New protected tables `v4_kernel_agent_upgrade_campaign` and
    `v4_kernel_agent_upgrade_node`.
  - `install.sh` writes the privileged updater
    (`anixops-agent-updater.path` and the root oneshot
    `anixops-agent-updater.service`, which runs the installed
    `anix-agent upgrade apply`) and removes it on uninstall. The Agent side
    (`upgrade.v1`, `upgrade apply`) follows in anix-agent.

- **Agent installer preflight, offline bundles and uninstall** (onboarding
  O2 and O3, `docs/guide/agent-onboarding.md`).
  - Preflight: before changing anything, `install.sh` checks systemd (240+),
    the kernel (5.10+) and nftables (0.9.7+) on forward nodes, `tc`,
    conntrack, polkit (0.106+), firewalld, ufw and a Docker `FORWARD DROP`
    policy, IPv6 SLAAC interfaces, an optional `--port-range`, free disk
    space, Control's https address and gRPC target with verified TLS, and the
    clock against Control's `Date` header (warns past 30 s, fails past
    5 minutes). Each failure prints a fix; `--skip-preflight` installs
    anyway. `--accept-ra` writes `accept_ra = 2` for SLAAC interfaces to
    the sysctl drop-in.
  - `--offline <bundle>` installs from a signed tar.gz without downloads.
    `anix-control agent offline-bundle -arch amd64|arm64 -o <file>` writes it
    from `agent_install.artifact_dir` (the Agent zip and `.sig`,
    `SHA256SUMS` and `SHA256SUMS.sig`, `agent.env`, `install.sh`); the script
    requires every signature.
  - `install.sh uninstall [--purge]` removes the Agent and gost units, the
    polkit rule and the binaries and keeps the identity; `--purge` also
    removes the state, configuration, sysctl drop-in, users, and the
    `inet anixops_fwd` table and `af00:` tc qdiscs only when they carry the
    drivers' marks.

### Changed

- **WARNING: `agent_control.mtls` now defaults to `required` (v4.2, owner
  decision H5). Legacy API-key Agents are refused on the AnixOps Agent
  channels unless you set `agent_control.mtls: preferred`.** Before
  upgrading, run `anix-control agents transports --check-required`; it must
  exit 0 ([UPGRADE](docs/UPGRADE.md#agent-transports-v42-requires-enrolled-agents)).
  - An empty `agent_control.mtls` (`ANIX_CONTROL_AGENT_CONTROL_MTLS`) is
    `required`; `defaults.yaml`, `config.yaml.example` and
    `config.prod.yaml` leave it empty. A config file that sets
    `mtls: "preferred"`, as the 4.1 template did, keeps `preferred`.
    `preferred`, `optional` and `off` stay selectable.
  - `required` refuses the API key on the Agent Control stream and the API
    key bootstrap of `Enroll`, `/api/v2/agent/*`, `/api/v2/node/*`,
    `/api/v2/forward/agent/rules` and the clean agent endpoints (403 /
    `Unauthenticated`, code `agent_mtls_required`). UniProxy, the v2board
    gRPC services and the plugin release download stay open.
  - Startup: an explicit `required` still refuses to start without the gRPC
    listener, its TLS and the built-in CA. The default `required` starts
    without them and logs `WARNING: Agent transports: ...`: no Agent can
    connect until they are set.
  - Readiness: `GET /api/v4/kernel/agents/transports` adds
    `summary.ready_for_required`, `summary.required_reasons` and
    `summary.required_blockers` (enabled nodes on a legacy channel, and
    enabled nodes that never enrolled). `anix-control agents transports
    --check-required [--json]` lists them and exits with status 3 while
    there is one (0 when none, 2 on errors). Under `required` the startup
    log warns with the counts (legacy nodes, those seen within the last 7
    days, never enrolled) and up to ten node names, and starts anyway.
  - The staging rehearsal runs the new default (`STAGING_AGENT_CONTROL_MTLS`
    selects another mode); the v2 package router tests that drive legacy
    agent paths set `preferred` explicitly.
- CI builds the Agent from anix-agent `1b155dee` (was `c459383`), which
  accepts the machine-telemetry `systemd_services` setting and collects the
  systemd services report (anix-agent #5). Older Agents refuse that setting,
  so the services panel needs this Agent.
- **Release branches and the v4.2 upgrade order** (owner decisions of
  2026-10-04; documentation and CI triggers only).
  - A release may now be cut from a maintenance branch `release/vX.Y` started
    at an earlier release's tag or commit: the version bump PR targets the
    branch, the tag goes on the branch's commit, and a follow-up PR merges
    the bump and CHANGELOG section back to `go_dev`. Patch releases of that
    line use the same branch (`docs/RELEASING.md`). `ci.yml` now runs on pull
    requests to and pushes on `release/**`, with the same required checks;
    the intended `release/**` ruleset is in `.github/BRANCH_PROTECTION.md`.
    v4.1.0 is to be cut this way, from the v4.1.0-rc.6 commit.
  - The v4.2 forward upgrade order (`docs/architecture/forward-sdk.md`
    section 10, `docs/UPGRADE.md`): first the new Agent, which removes the
    legacy forward runtime locally on install; then every forward node
    switches to it; then Control v4.2 with `agent_control.mtls: required`.
    The upgrade (F5c) then verifies the nodes are clean, cleans NodeX hosts
    through NodeX's API and Ansible hosts over SSH, and drops the tables
    after H15. Of the 53 bridged forward routes, F5d deletes the 30 flux
    routes, F5a rewrites the 19 node management routes as
    `/api/v4/forward/*`, and the 4 clean agent routes retire with the switch.
  - M3-4 and M3-5 are cancelled (`docs/architecture/node-ops-service.md`
    section 7), and anix-agent releases follow Control's version numbers,
    with Control's CI pinning the same Agent commit (H25).


### Added

- **AnixOps relay transport design (H22, approved 2026-10-04).**
  `docs/architecture/anixops-protocol.md` specifies the secure transport
  between forward nodes (TLS 1.3 mutual authentication with the H28 link
  certificates and identity pinning, framing and multiplexing, TLS, QUIC
  and trusted-link plaintext carriers, the `anixops` driver and its own
  unit, the v4.2 experimental flag `forward.anixops_experimental`), with
  questions P1–P10, all decided by the owner on 2026-10-04 (separate
  `anixops-relay.service`; plaintext only on administrator routes between
  `link=iepl|iplc` nodes; nftables RAW handover in v4.3). H23 is recorded
  as decided (the protocol in both editions). Camouflage stays reserved for
  the owner. Documentation only.
- **Forward v4 API and command line (F5a, owner decision H23).** The forward
  package serves `/api/v4/forward/*` on the kernel's `ForwardControl`
  (`kernel.forward.v1`), as its manifest control route
  `/api/v4/plugins/forward/*` (`packages/forward/v4api`). Reference:
  `docs/forwarding/v4-api.md`.
  - **Routes.** List, get, create, preview, update at a revision, delete,
    pause, resume, statistics and health. A refusal carries violations with
    the stable codes of `sdk/forward/validate`, and `Idempotency-Key` makes
    a retried write apply once.
  - **Nodes.** The node inventory with forwarding settings (port range,
    reserved ports, addresses, labels) and the node view (desired state,
    latest report, hop errors, health).
  - **The 19 v2 node management routes, rewritten.** Forward node and
    Ansible machine CRUD and toggle, and the observability targets,
    topology and trend.
  - **Statistics.** Traffic totals and hourly series from the ledger.
  - **Not served.** No answer carries a node credential. The v2 routes stay
    until F5d.
  - **Contract.** `ForwardControl` gains, additions only, `ListNodes`,
    `GetNode`, `SetNodeSettings`, `CreateForwardNode`, `UpdateForwardNode`,
    `DeleteForwardNode`, `GetTraffic` and `enforced` on route answers; the
    validate code `node_in_use` is new. The module listener forwards the
    new methods.
  - **Kernel checks.** Every `DELETE` needs a super administrator. Writes
    are audited as module `forward`.
  - **Editions.** `config/editions.json` `commercial_api_prefixes` reserves
    `/api/v4/forward/self/`, `/plans/` and `/multipliers/` for the
    commercial edition (v4.3). The community edition answers them as
    unknown routes.
  - **Command line.** `anix-control forward routes list|get|create -f|delete
    --yes|pause|resume`, `forward nodes list|set` and `forward stats` run
    beside `forward reset-node` and audit as `system/cli`.
- **Forward link certificates, Control side (owner decision H28).** A
  dedicated forward link CA issues each node whose Agent negotiated
  `forward.v1` the certificate its forward engines (gost) present to each
  other on encrypted links, so gost never holds the Agent's Control key.
  Additions only to `anix.agent.v1` (`agent_enrollment.proto`);
  `contracts/proto/descriptors.golden` only gains lines.
  - **CA.** A self-signed ECDSA P-256 root, separate from the module,
    kernel and Agent CA and name-constrained to `spiffe://anixops` URIs,
    created at startup wherever the built-in CA runs, its key sealed with
    `module_runtime.ca_kek` under additional data of its own. New table
    `v4_kernel_forward_link_ca`. Rotation mirrors the module CA's with the
    link lifetime as overlap: `anix-control agent link-ca rotate` adds the
    next CA to the link trust bundle at once, it signs 7 days later, and the
    retired CA stays trusted 7 more days (`agent link-ca list|bundle`).
  - **Issuance.** `AgentEnrollment.IssueLinkCertificate`
    (`IssueLinkCertificateRequest`: `csr_der = 1`;
    `IssueLinkCertificateResponse`: `certificate = 1`) and
    `GetLinkTrustBundle` (`GetLinkTrustBundleResponse.trust_bundle_der = 1`);
    `LinkCertificate`: `certificate_der = 1`, `trust_bundle_der = 2`,
    `spiffe_id = 3`, `node = 4`, `dns_name = 5`, `serial = 6`,
    `not_after_unix = 7`, `renew_after_unix = 8`. The certificate's CN and
    only DNS name are the node's identity name (`forward-7`, the planner's
    default `server_name`), its only URI the node's SPIFFE ID (the state's
    `peer_identity`), with serverAuth and clientAuth, for 7 days, renewed at
    two thirds by asking again with a new key. Only an Agent client
    certificate authenticates the call, only for an enabled node whose
    Agent negotiated `forward.v1`; the CSR may name only the node's DNS name
    and SPIFFE ID and must not reuse the Agent's key. New codes
    `link_cert_not_negotiated`, `link_cert_request_invalid`,
    `link_cert_unavailable` (`sdk/agentcontrol`).
  - **Records and revocation.** Every link certificate is recorded in the
    new table `v4_kernel_forward_link_certificate` (serial, node, the Agent
    certificate that asked, issuer, `not_after`, `revoked_at`) and revoked
    with the node's Agent credentials (disable, credential replacement,
    deletion, `RetireNode`); expired records are pruned with the Agent's.
  - **Agent contract.** `PROTOCOL.md` "Forward link certificates" specifies
    the Agent's side (F3b): a separate key, the files under
    `/var/lib/anixops-gost/tls` (group `anixops-gost`, 0640), keeping the
    bundle current and reloading gost. `docs/UPGRADE.md`, "Forward Link
    Certificates"; `docs/architecture/forward-sdk.md` sections 6.2, 14 and 16.
- **One-command node onboarding (O1, H18;
  `docs/guide/agent-onboarding.md`, `docs/architecture/forward-sdk.md`
  section 9).**
  - `POST /api/v4/kernel/agents/install-tokens` (super administrators):
    a single-use AgentPKI enrollment token (`anixagt_`, stored as SHA-256,
    audited) bound to a proxy or forward node, 1 hour by default and at most
    7 days, with the install command for each mirror (`control`, `cn`,
    `github`). Node-group tokens are not supported yet.
  - The node's 部署 section and the forward node page have
    **复制安装命令**: a sheet that issues the token and shows each command
    with a copy button.
  - `GET /install.sh` serves the Agent installer (public, rate limited),
    byte for byte the new release asset `agent-install.sh`, which the release
    job signs with the official package root (`agent-install.sh.sig`); the
    image serves the signature at `/install.sh.sig` when it verifies.
    `GET /install/agent.env` gives the script the Agent release, the gRPC
    target, the mirrors and, with `agent_install.artifact_dir`, the digests
    of the release Control serves at `/install/agent/<tag>/<asset>`.
  - The installer (`curl -fsSL https://<control>/install.sh | sudo bash -s --
    --control ... --node ... --token ...`) checks the platform (amd64/arm64,
    systemd), verifies the release's SHA-256 from Control or GitHub (and its
    signature when published), runs the Agent as the unprivileged
    `anixops-agent` with ambient `CAP_NET_ADMIN`/`CAP_NET_BIND_SERVICE` and a
    systemd sandbox, installs `anixops-gost.service` and its polkit rule,
    writes the token to a 0600 credential file the Agent removes, removes the
    legacy forward runtime (`inet v2b_forward`, `ip v2b_forward`,
    `ip anixops_forward`, the clean agent's `v2forward-agent`) and reports it,
    and waits until the Agent has enrolled. Re-running it upgrades in place
    and keeps the identity; `--reset` enrolls again. `--offline`, `--group`
    and `uninstall` are refused with a reason (later work).
  - New `agent_install` settings (`public_url`, `grpc_target`,
    `agent_version`, `artifact_dir`, `cn_mirror_url`, `signature_file`);
    `app.subscribe_path` may no longer be `install`, `install.sh` or start
    with `install/`.
  - CI runs shellcheck and fake-root tests of the installer
    (`scripts/tests/test_agent_install.sh`).
  - End to end it needs the next anix-agent release, which accepts the
    credential-only configuration the script writes (no node API key, no
    proxy cores, `forward-<id>` nodes); today's Agent starts and refuses it.

- **The last Agent contract gaps before `required` (owner approval of
  2026-10-04: additions only to `anix.agent.v1`).** Older Agents are
  unaffected; `contracts/proto/descriptors.golden` only gains lines.
  - **Alive list on the stream** (`alive.v1`, new capability).
    `ControlToAgent.alive_list = 17` (`AliveList`: `revision = 1`,
    `last_page = 2`, `entries = 3` of `UserAlive` {`user_id = 1`,
    `alive_count = 2`}, `computed_at_unix_ms = 4`) carries what UniProxy
    `alivelist` answers, each user's online device count across all nodes,
    after `HelloAck` and whenever it changes (checked once a minute; pages
    of at most 10 000 users). Proxy nodes only, offered only to an Agent that
    lists it.
  - **Plugin downloads by client certificate** (`artifacts.v1`, new
    capability). The new service `AgentArtifacts` (`artifacts.proto`:
    `GetPluginManifest`, `DownloadPluginArtifact` streaming 1 MiB
    `PluginArtifactChunk`s; `PluginReleaseAddress`, `PluginRelease`) serves
    an enrolled Agent the releases assigned to its node with the HTTP
    download's authorization, content addresses and verified bytes, so the
    Agent verifies them unchanged. Offered only on a certificate-
    authenticated proxy session; at most 2 downloads per node at once.
    The node API key download stays for Agents not yet enrolled.
  - **Codes in acknowledgements.** `ReportAck.error_code = 4` and
    `retry_after_ms = 5`, `MaintenanceEventResult.error_code = 4` and
    `retry_after_ms = 5`, `ConfigStatus.error_code = 5` (set by the Agent;
    kept as `v4_kernel_node_config_status.reported_error_code`). Every
    refusal carries its code (`report_*`, `maintenance_*`; the Agent's
    `config_*`), a maintenance event Control cannot store now carries
    `maintenance_unavailable` with a 30 s hint, and a report batch it cannot
    record now is answered with `report_unavailable` and a 15 s hint, only
    to an Agent that lists `reports.v1` with `transient_ack: "v1"` (others
    still get no answer, since they drop a batch on any `ReportAck`). A
    payload sent without its capability ends the stream with
    `agent_capability_not_negotiated` in the trailer. `sdk/agentcontrol`
    names every code.
  - **Legacy diagnostic routes reach stream Agents.** `POST
    /admin/agent/tasks` and `/admin/agent/execute` send the task as the
    `agent.diagnostic` operation on the stream when the node has no agent
    WebSocket and its Agent advertises the operation.

- **The `agent_control.mtls: required` prerequisites on Control (A2-6b).**
  An Agent that negotiates the data plane now needs no legacy HTTP or
  WebSocket path, so v4.2 can make `required` the default once the Agent
  release uses the stream (anix-agent AG-3 to AG-5). Additions only to
  `anix.agent.v1`; older Agents are unaffected.
  - **Maintenance events on the stream** (`maintenance.v1`, new
    capability). `AgentToControl.maintenance_events = 19`
    (`MaintenanceEvents`: `version`, `events_json`) carries the Agent's
    durable maintenance outbox (`anixops.maintenance/v1`, at most 50 events
    of 16 KiB), which until now went only to the agent WebSocket, where
    Control never answered it. Control stores each event once per node and
    event id as a node log entry of source `maintenance` and answers every
    batch with `ControlToAgent.maintenance_ack = 16` (`MaintenanceAck` of
    `MaintenanceEventResult`: `event_id`, `persisted`, `error`), one result
    per event in order: persisted, refused for good, or (database failure)
    neither, to be resent. Offered to proxy nodes. Metric
    `anixops_agent_maintenance_events_total{result}`. `sdk/agentcontrol`
    has the schema, its bounds and `ParseMaintenanceEvent`.
  - **Heartbeat and runtime health.** `NodeStatus` (`reports.v1`) is the
    stream equivalent of `POST /api/v2/node/heartbeat` and
    `/api/v2/node/runtime-health`, with their side effects together, and
    now also records the session's transport (`mtls-stream`) in the
    transport inventory as the legacy requests did. `PROTOCOL.md` maps every
    path `required` refuses to its stream equivalent.
  - **Certificate refusal codes.** Every refusal of an agent certificate
    carries `x-anix-error-code`, as `agent_mtls_required` did:
    `agent_cert_revoked`, `agent_cert_expired`, `agent_cert_invalid`,
    `agent_cert_wrong_cluster`, `agent_cert_wrong_node`, and
    `agent_enrollment_rejected` for `Enroll`; on the stream (at connection
    and at the heartbeat that ends a revoked or expired session),
    `Renew`, `GetTrustBundle` and the v2board services. Constants in
    `sdk/agentcontrol` (`ErrorCode*`).
  - **The capability offer is an intersection.**
    `HelloAck.server_capabilities` lists a data-plane capability only when
    the Agent's `Hello` lists it; Control used to list `users.v1` to every
    proxy node. No Agent behaves differently: a capability was in use only
    when both sides listed it.
  - **Session identity.** `AgentControlSnapshot`
    (`GET /admin/nodes/:id/agent-control`) and a new `session` per node in
    `GET /api/v4/kernel/agents/transports` show a live stream session's
    authentication (`mtls` or `api-key`), its certificate's serial, expiry
    and SAN, and its negotiated capabilities, and the Agent's own health
    metrics from its latest heartbeat (`agent_metrics`:
    `agent_control_*`, `agent_identity_*`, `agent_dataplane_*`), which
    Control used to drop.
  - `TestAgentStreamUnderRequiredNeedsNoLegacyPath` walks an Agent enrolled
    under `required` through configuration, users, heartbeats, status and
    runtime health, traffic, logs, maintenance events and a diagnostic task
    on the stream alone.

- **Package reports on the Agent Control stream** (systemd services panel
  1/7). The Agent contract gains `PackageReport` (`plugin_id`, `kind`,
  `version`, `payload_json`, `observed_at_unix_ms`) as
  `AgentToControl.package_report = 18`, sent with the new capability
  `package-reports.v1`, which Control offers to proxy nodes. Additions only:
  v4.0.0 Agents and hosts are unaffected. The latest report per node,
  plugin and kind wins; there is no acknowledgement or spool.
  - Control accepts a report only when the node's assigned release of the
    plugin is a signed official Agent release declaring the kind's
    capability (`systemd.services`: `telemetry.systemd.read`, which
    `machine-telemetry` will declare from 4.1), and stores the payload as the
    kind's sanitizer re-encodes it in the new table
    `v4_kernel_package_report_state` (latest only, no history; stale after
    25 minutes). Packages read their own rows through the kernel API view
    `kapi_package_report_v1` (on PostgreSQL a security-barrier view scoped
    to the reading package's role).
  - `sdk/telemetry/systemdreport` is the `systemd.services` schema and
    sanitizer the Agent and Control share. Per `.service` unit only the
    name, ActiveState, SubState, the 10-minute average and peak CPU and the
    current and peak memory; `Description` and `ExecStart` are refused,
    `user@*` and `run-*` units dropped, at most 512 units with names of at
    most 256 bytes (owner decision H24).
  - Metrics: `anixops_agent_package_reports_total{result}` and
    `anixops_agent_package_reports_refused_total{reason}`.
  - Design and privacy notes: `docs/architecture/package-reports.md`.
- **Read-only systemd services panel** (systemd services panel 5–6/7).
  - `machine-telemetry` declares `telemetry.systemd.read`, the permission
    `machine-telemetry.services.view` and the control route
    `GET /api/v3/plugins/machine-telemetry/nodes/:id/services` (admin only).
    The package answers it itself from `kapi_package_report_v1`: the node's
    units (name, ActiveState, SubState, 10-minute average and peak CPU,
    current and peak memory), a summary `{total, failed, active, inactive}`,
    `observed_at`, `window_seconds`, `supported` / `unsupported_reason`,
    `stale` (older than 25 minutes) and the node's `enabled` setting.
  - Collection is off on every node until enabled: the package configuration
    gains `systemd_services.nodes.<node_id>` with `enabled` and optional
    `include` / `exclude` globs (`path.Match` syntax). Control pushes the
    document to the package's nodes with `plugin.configure`; the Agent
    collector (anix-agent, panel 3–4/7) reads its own node's entry
    (`sdk/telemetry/systemdreport.ParseConfig`). Control validates the globs
    when the configuration of a release declaring `telemetry.systemd.read`
    is saved.
  - New kernel API view `kapi_plugin_configuration_v1`: each installation's
    configuration document with its package, target and desired version
    (on PostgreSQL scoped to the reading package's role), so a package can
    read its own settings.
  - Node page: a 服务 / Services section, shown when the node's
    machine-telemetry release declares the capability, with filters by name
    and state, sortable columns, the totals line
    “总计 N | 失败 N | 每 10 分钟更新一次”, a stale banner, the unsupported
    reason, and a per-node switch with the include / exclude patterns. No
    start, stop or restart. The Machine Telemetry WebUI links each node to it.

- Forward SDK F1c (`docs/architecture/forward-sdk.md` section 5):
  `sdk/forward/planner`, the pure, deterministic planner. `Plan` validates
  every route with `sdk/forward/validate` and renders them into one
  `NodeForwardState` per node; `PlanRoute` is the single-route plan of
  `ForwardControl.PlanRoute`; `Stamp` sets `state_hash` (SHA-256 of the
  deterministic encoding of the node's sorted hops) and bumps a node's
  `generation` only when that hash changes.
  - Ports: the entry's nodes share one port; previous allocations stick
    across re-plans while legal; explicit ports already held are refused;
    the rest get the lowest free port of the node's range outside reserved
    and taken ports (grace period). Connection marks 1..4095 per node,
    sticky too.
  - Wiring: next-hop nodes in `node_refs` order (their failover priority)
    at `dial_address` or the first address, with the link's server name
    and the pinned Agent identity on encrypted links; targets on the last
    hop; ingress sources and peers from the previous hop; limits on the
    entry only; `DIRECT_MODE_PREFERRED`; paused routes kept with `paused`
    set.
  - Contract (draft golden policy, `anixops.forward.v1` only):
    `PortAllocation.mark = 5`, so stored allocations carry the sticky mark
    (`planner.AllocationsFromProto` reads them back); the `NodeHop.mark`
    comment says it is per route, hop and node, unique on the node,
    1..4095 within the driver's mask; `Route.paused` and `NodeHop.paused`
    say a paused route stays rendered (ports, marks, counters, quota kept)
    with its traffic dropped.
  - New violation codes, listed with validate's: `port_in_use`,
    `port_exhausted`, `no_port_range`, `mark_exhausted`, `no_address`. Any
    violation refuses the whole plan.
  - The two F1a plan fixtures in `contracts/forward/v1` are now produced by
    the planner byte for byte (canonical, re-indented protojson; zero
    values such as `"priority": 0` are omitted, and the single-hop fixture
    gains a warning that its node's IPv6 clients have no upstream), with
    `-update` to regenerate. New goldens: UDP over WSS to IPv6 targets,
    two entry nodes behind an entry hostname with failover, a sticky
    re-plan after adding a target, port exhaustion, and a multi-route plan
    with generations. Property tests cover idempotent re-plans, collision
    free and in-range allocations, freed ports and generation bumps.
- Forward SDK F1b (`docs/architecture/forward-sdk.md`): `sdk/forward/model`,
  Go domain types for the draft `anixops.forward.v1` contract (routes, hops,
  targets, policy, limits, counters, node inventory) with lossless
  conversion to and from the protobuf messages and the defaults in one place
  (health checks every 5 s with a 2 s timeout, circuit breaker after 3
  failures for 30 s: proposed, owner decision H21 still open); and
  `sdk/forward/validate`, the route rules Control, the planner and the Agent
  share, answering violations with a field path, a stable code and a
  message. The draft contract's `Violation` gains `code` (field 3) so the
  code reaches UIs and API clients. The draft fixtures in `contracts/forward/v1` now run through it:
  the two plans validate and the five negative routes fail at the documented
  fields (the last negative case's entry port moved into its node's range so
  it breaks only the rule it documents). Nothing serves forwarding with them
  yet.

- Forward SDK F2a (`docs/architecture/forward-sdk.md` sections 6.0 and 13):
  `sdk/forward/driver`, the interface every forwarding engine driver
  implements (`Engine`, `Capabilities`, `Render`, `Apply`, `Observe`,
  `SetUpstreams` for failover without a full apply, `Remove`), its shared
  types (`Artifact` with a SHA-256 digest that excludes the generation,
  `ApplyResult`, `Observation`, `HopError`/`RenderError`), typed errors
  (`ErrUnsupported`, `ErrConflict`, `ErrNotOwned`, `ErrStaleGeneration`,
  ...) and a `Registry` keyed by engine that splits a node state by engine;
  `sdk/forward/driver/fake`, an in-memory driver with a simulated host,
  traffic, health and fault injection; and
  `sdk/forward/driver/conformance`, the suite (`conformance.Run(t,
  factory)`) the nftables and gost drivers will have to pass: data-driven
  state cases gated on capabilities and 33 scenarios covering determinism,
  idempotency, generations, restart and repair, ownership of foreign
  objects, counter epochs, failover, removal, contexts and concurrency. The
  fake passes it; mutant drivers prove the suite catches each violation.
  Nothing uses the drivers yet.

- Forward SDK F2b (`docs/architecture/forward-sdk.md` section 6.1):
  `sdk/forward/driver/nftables`, the nftables driver's `Render`. It turns a
  node's nftables hops into one deterministic `nft -f` transaction that
  touches only `table inet anixops_fwd` (owned through its comment):
  objects declared, rules and balancing elements flushed and re-added, so
  counters, quotas and connection counts keep their values. Covered: DNAT
  for TCP, UDP or both over IPv4, IPv6 and dual stack with masquerade;
  round robin, random, IP hash, least connections (weighted random until
  re-weighting) and failover through fixed-size slot maps that failover can
  rewrite without touching rules; named counters per hop and direction,
  named quotas, `ct count` limits, the hop's connection mark inside a
  configurable mask (default `0x0fff0000`), packet marks for tc, MSS
  clamping, admission of relay and exit sources, and paused hops (kept,
  traffic dropped). Hops it cannot run are rejected one by one with the
  others still rendered; names are derived only from checked route ids and
  every address is a checked literal. Goldens with their input states are
  in `contracts/forward/v1/nft` (`-update` rewrites them), including every
  nftables node state of the planner goldens and a planned paused route, checked with
  `nft -c` where nft is available (CI installs it), and `FuzzRender` keeps
  any input inside the script grammar. `Capabilities` is static until host
  probing, and `Apply`, `Observe`, `SetUpstreams` and `Remove` answer
  `ErrUnsupported` until F2c. The conformance suite's generated hops now
  use the planner's mark semantics (an index the driver shifts into its
  mask). Nothing uses the driver yet.

- Forward SDK F2c (`docs/architecture/forward-sdk.md` section 6.1): the
  nftables driver runs on a host. `nftables.Probe` checks nft, the
  `CAP_NET_ADMIN` the Agent needs and every kernel feature (with `nft -c`
  inside the driver's own table name, never committed), fills the
  configuration (`Capabilities` reports it, unavailable with a reason) and
  warns about another table's forward chain that drops by default
  (Docker). `Apply` refuses a foreign `inet anixops_fwd` table before
  anything runs (`ErrNotOwned`), checks generations, compares the host with
  the artifact (recorded digest, a fingerprint seal of the table, the tc
  objects), so applying what runs changes nothing and a damaged table is
  repaired, refuses foreign DNAT rules on its listen ports (`ErrConflict`)
  and runs one `nft -f` transaction that also deletes the objects of
  removed hops (their last counters go to a hook) and records the state in
  a set of the driver's own table, so a restarted Agent observes it.
  `Observe` reads counters per direction with a counter epoch that ends
  only when a counter is re-created, `SetUpstreams` rewrites the balancing
  map elements for failover and weights and keeps the rotation on the host,
  and `Remove` deletes only what the driver owns. Bandwidth limits are tc
  HTB classes per hop and direction on the configured egress interfaces
  under a driver-owned root qdisc (`af00:`), added before the nft
  transaction and removed again when it fails; a foreign root qdisc is
  `ErrConflict`. Render adds a manifest of comment lines to the script
  (goldens rewritten). The whole conformance suite passes on a real kernel
  in throwaway network namespaces, with traffic, damage, faults, conflicts
  and impostors; CI runs it with the tc, quota and probe tests under sudo
  in Backend Tests shard 1, and replay tests check the exact command
  sequences without privileges. Minimum nft 0.9.7 and Linux 5.10; tested
  with nft 1.0.9 and 1.1.3. Nothing uses the driver yet.

- Forward SDK F2d (`docs/architecture/forward-sdk.md` section 13): the
  multi-namespace end-to-end suite, `sdk/forward/e2e`. Each test builds
  client, entry, relay, exit and target network namespaces joined by veth
  pairs (IPv4 and IPv6, forwarding on inside the node namespaces only) and
  drives them as Control and the Agents will: model routes, validation
  against an inventory of the namespace nodes with their probed
  capabilities, the planner and its generations, then Render and Apply by
  each node's nftables driver in its namespace. Echo servers (the test
  binary re-executed in the target namespaces) answer real TCP and UDP
  traffic: one hop over IPv4 and IPv6 with counters matching the bytes
  moved per direction (UDP exactly), two and three hops with every hop
  counting and admitting only the previous node, each balance strategy
  over three targets (round robin in turn and by weight, random and least
  connections in proportion, IP hash stable per client address), failover
  through `SetUpstreams` driven by a simulated health loop when the primary
  dies, quota exhaustion, the connection limit, a tc HTB bandwidth limit
  within 0.5 to 1.5 times the rate, pause and unpause with counters kept,
  an Agent restart observing the same state with a no-op re-apply, and
  Remove leaving foreign tables and qdiscs alone. A failed test writes the
  namespaces' state to `ANIXOPS_FORWARD_E2E_LOGDIR`; leftovers of a dead
  run are removed before the next. It needs root and
  `ANIXOPS_FORWARD_E2E=1`. CI: the new "Forward Netns E2E" job runs it
  under sudo on the hosted runner (H14) on pull requests in the `forward`
  change class, which now also covers `sdk/forward`, `sdk/api/forward` and
  `contracts/forward`, nightly and on manual runs; it is not a required
  check until it has been green for two weeks. The SDK module now requires
  `golang.org/x/sys` directly (it was indirect), for `setns`.

- Forward F3a (`docs/architecture/forward-sdk.md` section 8): Control serves
  `anixops.forward.v1`. The contract is binding from here on: additions
  only (`Violation.route_id = 4` added, so a refusal names the route it
  belongs to), and the new gate `config/scripts/check_proto_golden.py`
  (Documentation Sync Check job) fails a pull request whose
  `contracts/proto/descriptors.golden` loses or rewrites a line of the base
  revision; no package is in the draft policy any more.
  - **Kernel forwarding state** (`internal/kernelforward`), in nine new
    protected tables (`v4_kernel_forward_route`, `_allocation`, `_node`,
    `_node_state`, `_node_report`, `_counter`, `_traffic`, `_request`,
    `_plan`): routes, their sticky port and mark allocations, the node
    inventory (proxy and forward nodes whose Agent negotiated `forward.v1`
    or that have forwarding settings; default port range 30000-39999, SSH
    and the node's own ports reserved), `planner.Plan` on every change under
    one lock row, and per-node generations (`planner.Stamp`). Any violation
    refuses the whole plan: a route write is refused, an inventory change
    keeps every node's state and records why (`PlanStatus`, the
    `anixops_forward_plan_refused` gauge). A deleted route's ports stay held
    for 10 minutes.
  - **`ForwardControl`** for official packages declaring the new kernel
    capability `kernel.forward.v1`, on local bridge sessions and the module
    listener, authorized on every call: route writes with request ids and
    revisions, refusals as `INVALID_ARGUMENT` or `FAILED_PRECONDITION` with
    the response and its violations in the status details, `PlanRoute`,
    `GetRouteStats` and `GetRouteHealth`. `DiagnoseRoute` answers
    `UNIMPLEMENTED` until F3c.
  - **Agent Control stream.** An Agent lists `forward.v1` with its
    `NodeCapabilities` in the attribute `node_capabilities`
    (`sdk/forward/wire` encodes and checks it); Control serves it with
    `config.v1` to proxy and forward nodes. Such a node's configuration is
    `anixops.nodeconfig/v2`, the v1 document plus `forward`, its
    `NodeForwardState`, and every generation change pushes it at once;
    Agents without `forward.v1` keep v1 and get no forwarding. Forward nodes
    are offered `package-reports.v1` with it and asked for a 60-second
    heartbeat (decided by default, section 8.4).
  - **Reports and the traffic ledger.** The `NodeForwardReport` arrives as
    a `PackageReport` (`forward`, `forward.report`, `v1`), accepted only on a
    `forward.v1` session (else refused `unnegotiated`), checked for the
    stream's node, kept as the node's latest, and its counters metered:
    per route, hop, node and counter epoch the largest values reported, and
    their raw growth per hour and direction. A reset or re-created hop adds
    its new epoch in full, a decrease adds nothing, an older report is
    dropped; no multiplier is applied. Gauges for nodes behind their
    desired generation, the largest lag and hop errors.
  - **Control-authoritative limits** (section 5.3, decided by default): a
    route whose entry traffic reaches `quota_bytes`, or whose expiry passed,
    is planned paused without changing the stored route; raising the quota
    lifts it. Rendering each entry's local remainder is deferred.
  - H21 is decided: health checks every 5 s with a 2 s timeout, the breaker
    opens after 3 failures for 30 s, least-connections re-weights every 10 s
    (`sdk/forward/model/defaults.go`).

- Forward SDK F4a (`docs/architecture/forward-sdk.md` section 6.2): the
  gost driver, `sdk/forward/driver/gost`, renders and runs a node's gost
  hops with the gost v3 release the Agent pins (3.2.6, MIT, owner decision
  H20; the SDK does not import gost). Render writes one JSON configuration:
  a service per listener (RAW `tcp`/`udp` forwarding; `relay` over `tls`,
  `mtls`, `wss`, `mwss`, `quic`, `grpc` or `mtcp` for the other links, with
  mutual TLS from the node's link certificate), forwarder nodes or a relay
  chain to the upstreams with the balance strategy as gost's selector
  (weighted round robin and IP hash by entry repetition) and the circuit
  breaker as its fail filter, an admission whitelist of the ingress sources
  (deny-all for a paused hop), traffic and connection limiters, and metrics
  on a unix socket under a path that names the configuration; a top-level
  manifest gost ignores carries what Apply needs. Unsupported hops (a byte
  quota, target names, `ANIXOPS` links, a hop mixing RAW and relayed
  upstreams) are rejected alone. `Apply` writes the configuration
  atomically to the driver's directory with its state file (ownership mark,
  generation, digest), refuses ports that foreign sockets hold
  (`ErrConflict`, read with `ss`) and foreign directories or units
  (`ErrNotOwned`), reloads gost with SIGHUP or starts it and waits until
  gost serves the new configuration, else restores the previous one; an
  established TCP connection and the counter epoch survive the reload.
  gost runs as `anixops-gost.service` (`gost.UnitFile`: its own user with
  `CAP_NET_BIND_SERVICE` only and a systemd sandbox) managed with
  systemctl, or as a child process (`ProcessSupervisor`). `Observe` reports
  the applied identity and rotation with counters at 0 and `SetUpstreams`
  answers `ErrUnsupported` until F4b. Goldens in `contracts/forward/v1/gost`
  (planner outputs over TLS, WSS, QUIC, gRPC and mux, UDP, failover, and the
  unit file). The conformance suite passes on a simulated host and, under
  sudo in Backend Tests shard 1, against the pinned gost (downloaded and
  checked by SHA-256 in CI) in network namespaces, with every golden loaded
  and TCP and UDP through a mutual-TLS relay. Owner decisions recorded: H20
  (the pinned gost and its unit) and H28 (forward link certificates from a
  dedicated forward link CA, built later in Control and F3b; until then
  encrypted gost links cannot be set up on real nodes). Nothing uses the
  driver yet.

- Forward SDK F4b and L1 (`docs/architecture/forward-sdk.md` sections 6.2
  and 7.1): the gost driver talks to gost's web API, on `api.sock` in its
  runtime directory without authentication (the socket's 0770
  `anixops-gost` owner and group permissions, from the unit's `UMask` and
  `RuntimeDirectoryMode`, are its only key). Render now gives every gost hop
  a top-level gost hop with its upstreams (named by the service's forwarder
  or its chain), an admission on every hop and statistics on every service,
  and names the metrics path after the configuration's structure only;
  goldens regenerated. `Apply` takes changes of these hot objects
  (upstreams, weights, strategy, sources, pause, limits' values, quota)
  through the API without re-creating any service, so listeners,
  established connections, UDP sessions, mux carriers and counters stay; a
  structural change still reloads gost (every hop then starts a new counter
  epoch), and a refused API change restores the previous configuration.
  `SetUpstreams` replaces the running hop with the selection and records it
  in the hop's metadata, so failover and re-weighting need no apply and
  survive an Agent restart; a changing apply restores every upstream.
  `Observe` reads every service's connections and bytes from `GET /config`
  (bounded, never per client as gost's Prometheus labels are), packets 0,
  with a counter epoch that ends exactly when gost re-creates the service.
  The soft quota (`Config.SoftQuota`, on by default, so the gost driver now
  reports the `quota` capability and validation accepts a byte quota on a
  gost entry): `EnforceQuotas`, the new optional `driver.QuotaEnforcer` the
  Agent calls after every Observe and Apply, closes a hop's admission once
  its current-epoch bytes reach `quota_bytes` and opens it when the quota is
  raised. A paused or quota-closed gost hop refuses new connections while
  established ones run until they close (gost cannot close accepted
  connections). L1, `sdk/forward/leastconn`: `Reweighter` sets each
  `LEAST_CONN` hop's weights to its rendered weight divided by live
  connections plus one, every 10 s, over the upstreams the health loop
  keeps; sources are the gost driver's `ActiveConns` (established sockets
  per upstream) and the fake driver; nftables needs a conntrack source from
  the Agent (F3b). The conformance suite now passes against gost 3.2.6 in
  network namespaces with no scenario skipped and traffic through the hops,
  and the Forward Netns E2E job runs a gost entry (exact payload counters,
  failover) with the pinned gost.

- Forward SDK F4c (`docs/architecture/forward-sdk.md` sections 6.2 and
  13): the gost driver's `Apply` makes structural changes through gost's
  web API, object by object (`POST`/`DELETE /config/services|chains|...`,
  `PUT` for hot objects): adding, changing or removing a route creates,
  re-creates or deletes only that route's services, chains, admissions and
  limiters, so every other route's listeners, established connections, UDP
  sessions, mux carriers and statistics stay (before, any route change
  reloaded gost and re-created every service on the node). A reload remains
  the fallback when the web API does not answer, gost does not run the
  recorded configuration or the log, API or metrics address change. A call
  gost refuses is rolled back through the web API, restarting gost only
  when that fails too. Counter epochs end per hop: a sequence number in
  `state.json` per hop whose services were deleted or created, and
  `WithRetiredCounters` gets exactly those hops' last counters; the state
  file also records the metrics path gost loaded last. gost's relay handler
  and connector now run with `nodelay`, so protocols whose server speaks
  first (SSH, SMTP, databases) work over encrypted and multiplexed links
  (they hung before; gost goldens regenerated); a client's TCP half-close
  arrives as a full close over such links. Conformance: new scenario
  `apply-leaves-unrelated-hops` (gost, nftables and the fake pass; a fake
  mutant proves it). The Forward Netns E2E job runs mixed-engine chains:
  an nftables entry, a gost relay and two gost exits over RAW and
  mutual-TLS mux links (TCP, UDP, per-hop counters, failover at the gost
  relay, and another route added and removed on the relay without
  disturbing a held connection or UDP session), and a gost entry before an
  nftables exit.

### Fixed

- **gost link certificate renewals start a new counter epoch.** A
  supervisor reload of gost after the Agent renewed the link certificate
  (H28) re-created every service without the driver recording it, so
  Control saw the counters fall within one `counter_epoch`, lost that
  observation interval, and the last counters went unreported. The gost
  driver has a new entry point, `(*gost.Driver).ReloadCredentials(ctx)`,
  which the Agent calls after every renewal instead (`PROTOCOL.md`
  "Forward link certificates", step 5). gost 3.2.6 has no certificate
  hot-reload: it reads a certificate when it creates the service or hop
  that uses it. So, serialised with Apply, the driver checks that the
  files load, re-creates through gost's web API only the services with an
  encrypted listener (TLS, WSS, gRPC) and replaces the hops with encrypted
  dialers (keeping a `SetUpstreams` selection), waits until every listener
  is back, records the re-created services' hops in `state.json` so their
  epoch advances (the F4c per-hop sequence), and hands their last counters
  to `WithRetiredCounters`. Other hops keep their epochs; established
  connections run on with the old certificate. A node with a mux (mtls,
  mwss) or QUIC listener restarts gost instead (measured: a re-created mux
  listener strands its peers' carriers, so new connections through them
  hang; a QUIC listener keeps its port while its connections live): every
  hop starts a new epoch and hands its counters over. A real-gost test
  (`TestNetnsReloadCredentials`) covers each link type.
- **gost no longer strands mux and QUIC carriers on structural changes.**
  Deleting a gost mux (`mtcp`, `mtls`, `mwss`) service through the web API,
  or reloading gost with SIGHUP, closed only its listening socket: the
  carriers it had accepted stayed up, the relay kept opening streams on
  them that nothing accepted, and new connections through the relay hung
  until the carrier closed (over 40 s, without end, measured); a QUIC
  listener could not be created again while its connections held the port.
  This hit an Apply that changed or removed such an exit hop and the reload
  fallback on any node with a mux listener. Apply now restarts gost
  instead whenever it would delete or re-create a running mux or QUIC
  service (adding one still goes through the web API), and its fallback
  restarts rather than reloads a gost whose loaded configuration has a mux
  or QUIC listener: the restart closes every carrier and peers dial new
  ones (over mux new connections pass within about 0.1 s; over QUIC at the
  peer's 30 s idle timeout), at the cost of that node's established
  connections and counter epochs (`Loads` counted, every hop's counters
  retired). `ReloadCredentials` shares the rule's predicate (unchanged
  behaviour: `mtcp` holds no certificate, so it never touches one). A
  running gost with no recorded configuration is restarted rather than
  reloaded, since what it runs is unknown. Both ends of mux and QUIC links now render their
  keepalives explicitly (10 s interval, 30 s timeout; gost's defaults), so
  a carrier whose peer vanished without a close is dropped; the `links`
  and planner goldens in `contracts/forward/v1/gost` change accordingly. A
  real-gost test (`TestNetnsMuxRestart`, about 6 s of CI) covers the web
  API path and the fallback.
- **Forwarding recovers after a Control database reset or restore.** Per-node
  forward generations restarted below the ones the Agents held, and Agents
  ignore an older generation forever (the drivers answer
  `ErrStaleGeneration`). A node's `NodeForwardReport` carries the generation
  and `state_hash` its Agent holds: when that is ahead of the stored state
  (higher, or equal with another hash), Control moves the node's generation
  to the reported one (same hash) or one above it, keeping its hops, pushes
  the node, and every later plan stamps above it. Recoveries are logged and
  counted (`anixops_forward_generation_recoveries_total{reason}`). The new
  operator command `anix-control forward reset-node <node_ref>` forces a
  generation above both (audit log `system/cli`). Agents need no change
  (`forward-sdk.md` section 8.2, PROTOCOL.md "Forwarding").
- The Agent installer turns on IP forwarding on forward nodes (and on proxy
  nodes with `--forward`): it writes `/etc/sysctl.d/90-anixops-forward.conf`
  (`net.ipv4.ip_forward = 1`, `net.ipv6.conf.all.forwarding = 1`), applies
  it with `sysctl -e -p` and reports it in its summary. Without it nftables
  forwarding dropped every packet on a host that did not forward already.
  The drop-in comes from `anix-agent forward sysctl-dropin` when the Agent
  has it (anix-agent #13).
- The Agent unit the installer writes has `RuntimeDirectory=anixops-agent`
  (0750): plugin sockets live in `/run/anixops-agent`, which
  `ProtectSystem=strict` otherwise keeps read-only. A host switching from
  anix-agent's root install (its unit without `User=anixops-agent`, or
  `/var/lib/anix-agent` or `/var/lib/anixops/plugins`) now runs
  `anix-agent migrate-paths --chown anixops-agent` before the new unit
  starts, so the sandboxed Agent keeps its identity and state; a failed
  copy stops the installer before the Agent starts.
- UniProxy `alivelist` answered an empty list with the built-in memory
  cache (its key pattern matched nothing there), so device limits counted
  only each node's own connections. It now counts the online sets of every
  node, through the reader `alive.v1` shares.
- The live Control WebUI E2E gate defaults to ports 24175 and 28080 instead
  of 34175 and 38080. The old ports sat in Linux's ephemeral range, so an
  outgoing connection left open by an earlier CI step could hold one and fail
  the gate with `EADDRINUSE` (seen twice on go_dev).

## 4.1.0-rc.6 - 2026-10-03

### Changed

- The `go_dev` ruleset now also requires the `Frontend Visual Regression`
  check (26 green runs since it was added; owner decision H9). It skips on
  changes that cannot affect the UI, which counts as passing.

- **Behaviour change: the rehearsed packages default to native routes**
  (decision H8). The 151 v2 routes of the 15 packages that passed the
  staging rehearsal (H7: `knowledge`, `ticket`, `notification`, `platform`,
  `machine-telemetry`, `protocol-runtime`, `plan`, `order`, `payment`,
  `affiliate`, `subscription`, `forward`, `proxy-node`, `gost-mesh`,
  `wireguard`) run their packages' native handlers when the installation
  stores no mode for them and the installed package is at least
  `4.1.0-rc.5`, the rehearsed release; older packages (signed 4.0.0 ones
  included) stay `legacy`. The set is explicit data,
  `config/package-route-defaults.json`, with a gate test against the R5
  record: a route that becomes native-flagged later does not default to
  native until a release adds it. A stored mode always wins. **Kill switch:**
  `package_routes.default_mode: legacy`
  (`ANIX_CONTROL_PACKAGE_ROUTES_DEFAULT_MODE`, default `rehearsed`) keeps
  every unstored route legacy, exactly the 4.0 behaviour; the startup log
  states the policy and how many routes default to native. **Rollback:**
  `anix-control routes rollback --package <id>` and `routes set --mode
  legacy` now act on the effective mode: defaulted routes get an explicit
  stored `legacy` with revision rows and an audit entry, and whole-package
  `set` compares against the effective mode. Package hosts receive the
  resolved modes through GetPackageConfig. `routes list` has a `SOURCE`
  column, the route-mode API returns `source` per route (`stored`,
  `default`, `kill-switch`, `package-too-old`, `identity-authority`,
  `unset`) and `defaults` per package, and the admin page shows
  “原生（默认）” / “Native (default)”. See `docs/UPGRADE.md`, "Upgrading To
  4.1.0: Packages Now Default To Native Routes".

- CI's frontend dependency audit (`npm run audit:check`,
  `web/scripts/check-npm-audit.mjs`) still fails on any advisory of moderate
  severity or above in production dependencies. In development dependencies
  it accepts only advisories listed in `web/audit-allowlist.json`, each with
  a reason and an expiry date; an expired or stale entry fails the check.
  The first entry is GHSA-vfj7-8cjw-p6xm (braces, no fixed version, reached
  only through histoire and stylelint), waived until 2026-11-02 with the
  owner's approval.

- `agent_control.mtls` defaults to `preferred` instead of `optional`
  (decision H5): legacy API-key agents keep working and now get the
  deprecation signals. `preferred` no longer needs gRPC TLS and the
  built-in CA to start (only `required` does, together with
  `grpc.enabled`); without them agents cannot enroll, which the startup log
  says. Set `optional` to keep the release candidates' silent behaviour.


- `agent_control.mtls` defaults to `preferred` instead of `optional`
  (decision H5): legacy API-key agents keep working and now get the
  deprecation signals. `preferred` no longer needs gRPC TLS and the
  built-in CA to start (only `required` does, together with
  `grpc.enabled`); without them agents cannot enroll, which the startup log
  says. Set `optional` to keep the release candidates' silent behaviour.
- The admin console's first visit downloads less: the admin messages are
  split into `admin` (shell, navigation, ⌘K, dashboard) and `adminPages`
  (every other admin page and the forward suite), which the router loads
  before the first of those pages opens, or when the browser is idle. The
  admin dashboard's first visit went from 239.8 KB to 210.2 KB gzip
  (258.0 KB to 228.4 KB with axios), against unchanged budgets of 250 KB
  and 269 KB.
- The package parity tests (`internal/tests/*compat`, harness
  `internal/tests/packagecompat`) run about twice as fast on PostgreSQL:
  the cases of one test share a migrated database per side instead of
  creating, migrating and dropping two schemas per case. Each case still
  starts empty: what the previous case added is dropped, rows are deleted
  and id sequences restart, a dropped foreign key is added back, and any
  other schema change gets a newly migrated database. Locally,
  `ordercompat` went from 27.1 s to 14.7 s and `paymentcompat` from
  29.6 s to 13.9 s with PostgreSQL (in CI's PostgreSQL shards 48.9 s to
  24.1 s and 52.6 s to 22.7 s; their shard weights are updated); every
  compat package passes unchanged.
- The CSS classes plugin WebUI bundles render (`.btn`, `.btn-secondary`,
  `.table-container`, `.data-table`, `.empty-state`) are documented as a
  stable contract ("WebUI CSS Classes" in
  `docs/architecture/plugin-kernel-contract.md`): Control must not remove
  or rename them. A web test fails when one loses its rule in
  `web/src/style.css`.

### Added

- **Staging rehearsal of route cutovers** (`scripts/staging/`, runbook
  `docs/guide/staging-rehearsal.md`), for the batch sign-offs of v4.1.0
  (H6, H7). Synthetic data only (H4); nothing reads a backup or a production
  database, and the containers run on an internal network with no route out.
  - `scripts/staging/rehearse.sh up` builds Control and every package
    (commercial edition, signed with a throwaway key) from the checkout,
    starts PostgreSQL and Control with Compose (no image is built), installs
    and enables the packages through `/api/v3`, seeds a deterministic data
    set (`STAGING_SEED`: administrators, staff, members with plans, banned,
    expired and empty members, orders in every status, payments, coupons,
    invitations and commissions, tickets, multi-language knowledge articles,
    notifications, audit logs, nodes, subscription groups, forwards, traffic;
    TEST-NET addresses and example.com only) and keeps it as a template.
  - `rehearse.sh --batch N` switches the batch's `native-flagged` read routes
    to `shadow` as the super administrator, replays read traffic as members,
    administrators and anonymous callers (pagination, filters, empty
    results, not found, invalid input, permission errors) until every route
    has `--min-requests` (200) compared requests and `--min-shadow-duration`
    (2h) in shadow, runs the batch's packagecompat suites, replays the write
    routes on two throwaway twins started from copies of the seeded
    database (one legacy, one native) and compares their answers and tables
    row by row (ignored columns listed per table), and writes
    `reports/batch-N/report.md` and `report.json` with one PASS or FAIL.
    `--smoke` lowers the thresholds to 20 requests and 2 minutes. It never
    switches the rehearsal instance to native: the report prints the
    `rehearse.sh routes set … --mode native` commands for after the sign-off;
    `--rollback` returns the batch to legacy.
  - Batches: 1 knowledge + ticket; 2 notification + platform +
    machine-telemetry + protocol-runtime; 3 plan + order + payment +
    affiliate; 4 subscription + forward + proxy-node + gost-mesh + wireguard.
    identity-platform has its own cutover.
  - CI: the job "Staging Rehearsal Smoke" runs batch 1 with `--smoke` in the
    full lane only; it is not a required check.
- Route modes have their own tooling. Until now a package's routes moved
  between `legacy`, `shadow` and `native` only by editing the reserved
  `routes` key of the package configuration. Now:
  - the admin API `GET /api/v4/kernel/route-modes` lists every v2 Control
    package route with its configured and effective mode, the modes it may
    switch to (from `config/package-extraction.json`) and the running
    host's native and shadow counters; `POST /api/v4/kernel/route-modes`
    switches named routes or a whole package, `POST
    /api/v4/kernel/route-modes/rollback` returns a whole package to legacy
    in one change, and `GET /api/v4/kernel/route-modes/revisions` lists the
    history;
  - the CLI `anix-control routes list|set|rollback|history` does the same
    from the server;
  - the admin page 插件中心 → 路由模式 (`/admin/plugins/route-modes`).

  Only a super administrator may switch: an administrator who is not staff
  and not banned, checked against the database on each request. Switching
  anything to `native` needs an explicit confirmation and a reason
  (`confirm: true` and `reason`, or `--yes` and `--reason`); a rollback
  needs neither. The existing rules still hold: `shadow` only for GET
  routes, `native` only for `native-flagged` routes, `kernel-owned` and
  WebSocket routes do not switch, and identity group A moves only with the
  identity cutover and rollback. Every switch writes an audit log entry and
  one row per route to the new table `v4_kernel_route_mode_revision`, in the
  same transaction as the configuration revision; package hosts apply it at
  their next configuration poll, as before. See "Switching Route Modes" in
  `docs/UPGRADE.md`.
- **Design: Forward SDK for v4.2** (`docs/architecture/forward-sdk.md`),
  with a draft contract `sdk/api/forward/v1` (`anixops.forward.v1`) and
  draft planner fixtures in `contracts/forward/v1`, approved by the owner
  (H11 to H14). Nothing changes in behaviour: nothing serves the contract and no
  planner or driver exists yet.
  - **Model.** A route is a chain of hops (entry, relays, exit) to one or
    more targets; each hop picks its engine: nftables for trusted links,
    gost v3 for the public internet, the AnixOps protocol in v4.3. A pure
    planner turns routes into one desired state per node, allocates ports,
    wires the chain and puts bandwidth, quota, connection and expiry limits
    on the entry.
  - **Drivers.** The Agent applies the state through drivers that own their
    objects: an `inet anixops_fwd` table with per-direction counters,
    numgen/jhash maps, named quotas, `ct count` and tc HTB; an Agent-managed
    gost that replaces NodeX; Ansible only as a fallback for hosts without
    an Agent.
  - **Failover.** Round robin, random, IP hash, least connections and
    failover at the exit and target levels, a circuit breaker, entry HA via
    DDNS, per-hop latency probes and end-to-end diagnosis, all running on
    the nodes while Control is down.
  - **Also covered:** the Control and Agent transport (`config.v1` and
    `PackageReport`), nyanpass-style one-command node onboarding, the v4.2
    upgrade (archive the flux forwarding data, clean the nodes, drop the
    tables; irreversible, H15), metering, editions, testing, security, the
    PR plan, and the open questions H11 to H14 and H18 to H23 with
    recommendations.
  - The draft contract joins the CI generated-code check and the proto
    golden file under the draft golden policy (its own lines may change
    until the first change that serves it), and `internal/tests/protocompat`
    checks the fixtures parse as the draft contract and are consistent.

- The agent transport transition (A2-6, `docs/architecture/node-ops-service.md`
  section 5.6):
  - `agent_control.mtls` takes four modes: `off` (new: no client
    certificates are requested or accepted, a rollback switch), `optional`,
    `preferred` and `required`. `required` now also refuses the legacy
    AnixOps-agent HTTP and WebSocket paths (`/api/v2/agent/*`,
    `/api/v2/node/*`, `/api/v2/forward/agent/rules`, the clean agent
    endpoints) with HTTP 403 `{"code":"agent_mtls_required"}`, and the
    stream and `Enroll` add the trailer `x-anix-error-code:
    agent_mtls_required`. UniProxy and the v2board gRPC services stay open
    to third-party node software in every mode.
  - Deprecation signals in `preferred`: `Deprecation: true`, a `Link` to the
    upgrade guide and, with the new `agent_control.legacy_sunset`, `Sunset`
    on the legacy agent paths (and the WebSocket handshake);
    `x-anix-auth-deprecation-link` and `x-anix-auth-sunset` next to
    `x-anix-auth-deprecated` on the control stream, in its header and
    trailer.
  - Metrics `anixops_agent_legacy_requests_total{path}`,
    `anixops_agent_legacy_refused_total{path}` and
    `anixops_agent_mtls_mode{mode}`, and a startup log line with the
    effective mode and whether agents can enroll.
  - The transport inventory: `GET /api/v4/kernel/agents/transports`
    (admin), `anix-control agents transports [--json] [--legacy-only]` and
    the admin page NodeX Agents → Agent 连接方式 (`/admin/agent/transports`)
    list each proxy and forward node with the transport it was last seen on
    (`mtls-stream`, `apikey-stream`, `http-legacy`, `websocket`,
    `clean-agent`, `uniproxy`, `v2board-grpc`), its agent version, its
    newest valid certificate and when it was last seen, with a warning on
    legacy nodes. Sightings are kept in memory and in the new table
    `v4_kernel_agent_transport`, written at most once a minute per node and
    transport.

- Shadow mismatches can be investigated. Until now a shadow run that answered
  differently from legacy only incremented
  `anixops_package_shadow_mismatches_total`. Package hosts built with this
  SDK now keep a sanitized sample of each mismatch (route, method, path
  template, legacy and native status, request id, and a structural diff of
  the two JSON answers) and report the latest ones in their Health details;
  the kernel sanitizes them again and stores them in the new table
  `v4_kernel_shadow_mismatch_sample`, for 7 days and at most 100 per route.
  `GET /api/v4/kernel/route-modes/mismatches` lists them, the route-mode
  list adds each route's mismatch rate, last mismatch and stored samples,
  and the admin page 路由模式 shows the rate and opens the samples of a
  route. Any administrator may read them. Secrets are masked before a
  sample leaves the host and again in the kernel: tokens, passwords,
  hashes, keys, UUIDs, signatures, cookies, subscription URLs, JWTs and long
  hex or base64 strings become `***`, e-mail addresses keep their first
  character and domain, IPv4 addresses two octets and IPv6 addresses two
  hextets; request bodies are never stored. Hosts built with the 4.0.0 SDK
  keep working and report no samples. See "Reading Shadow Mismatches" in
  `docs/UPGRADE.md`.

### Deprecated

- Legacy AnixOps Agent authentication: the node API key on the Agent
  Control stream, the agent and node WebSocket, `/api/v2/agent/*`,
  `/api/v2/node/*`, `/api/v2/forward/agent/rules` and the clean agent
  endpoints. **v4.2 makes `agent_control.mtls: required` the default:
  before upgrading to v4.2, every node must run an Agent that has enrolled
  (mTLS); legacy API-key agents will be refused.** Third-party node
  software on UniProxy or the v2board gRPC services is not affected. Run
  `anix-control agents transports --legacy-only` as the checklist; see
  "Agent Transports: Preparing For v4.2" in `docs/UPGRADE.md`.

### Fixed

- SQLite deployments no longer fail writes with "database is locked" under
  concurrent load. A transaction that read before it wrote could not
  upgrade to a writer once another connection had committed after its read
  began, and SQLite failed it at once instead of waiting out
  `busy_timeout`. Control and package storage now open SQLite with
  `_txlock=immediate`, so every transaction waits for the write lock at
  `BEGIN`.
- On PostgreSQL, Agents and subscriber watchers no longer miss subscriber
  changes. Change log ids are taken at insert but become visible at commit,
  so a later id could commit first; a consumer then moved its cursor past
  the earlier row and never read it. Writers of the change log now take a
  transaction-scoped advisory lock, so they commit in id order.
- Moving a node to another group now records subscriber changes. A node's
  users follow its group, but nothing was written to the change log, so an
  Agent kept serving the old group's users and only picked up the new
  group's one by one as they changed, or at its next resync. Updating a
  node's `group_id` now records a change for each active user of the old
  and the new group (every active user when either is "all").
- KernelNodeOps: an executor whose operation passes its deadline now
  always sees `context.DeadlineExceeded`. The dispatcher's deadline sweep
  could cancel the executor's context just before the context's own
  deadline fired, handing it `context.Canceled` instead. The dispatcher's
  own statements now also finish when it stops instead of being
  interrupted, which avoided a data race in the SQLite driver.
- Flaky tests made deterministic: `internal/kernelnodeops`
  (`TestDeadlinesEndStartedOperationsTimedOut`, and the TARGET_GONE steps
  of `TestDiagnoseForwardDialsPublicTargetsOnly` and `TestDiagnoseTunnel`,
  which could run the diagnosis before the target was deleted) and
  `internal/tests/nodeopsagent` (the scripted agent announced an operation
  before acknowledging it, so a test's terminal report could overtake the
  acknowledgement and Control closed the stream).
- The 流量转发管理 page header reads fully in Chinese: its description,
  runtime badge and runtime summary no longer mix in English ("runtime",
  "tunnel", "Port Forward", "Local Runtime"); product names such as NodeX,
  gost, nftables and Ansible stay as they are. English is unchanged.

## 4.1.0-rc.5 - 2026-10-02

4.1.0-rc.5 is the fifth 4.1.0 release candidate and completes the UI
redesign: the page bodies that 4.1.0-rc.4 left as they were now use the
AnixOps Design templates (sign-in, the user pages, every admin list, the node
and forward pages, settings, the dashboard, 流量与监控 and 部署编排), the sign-in
page downloads less than half as much as before (278 → 115 KB gzip), axe finds
no serious or critical issue on the key screens in either theme, and a visual
regression baseline guards them. It also makes the nftables forward path
count traffic correctly and carry IPv6, lets users reset their own
subscription link, and repairs four garbled API error messages.
`docs/UPGRADE.md`, "Upgrading From 4.1.0-rc.4 To 4.1.0-rc.5", has the
checklist.

**For users:** a new sign-in card with six code boxes for two-step
verification; new 概览, 订阅, 帮助中心, 工单 and 账户 pages; the subscription link
has a scannable QR code and one-click import for the common clients, and
users can **reset their own link** (订阅 → 重置链接…, with their password or a
two-step code; the old `/s/<token>` link stops working, the proxy UUID is
kept). "备用码" are now "恢复码" (recovery codes).

**For admins:** every list is one table (search, filters and page kept in the
URL, bulk actions, cards on phones); nodes, NodeX forward nodes and Ansible
machines get their own pages (`/admin/nodes/:id`,
`/admin/forward/nodes/:id`, `/admin/forward/ansible-machines/:id`); the
forward suite's lists use the same table, with 上移 / 下移 next to the drag
handle. Several pages merged into sectioned pages: 系统设置
(`/admin/system/:section`), 安全 (`/admin/security/:section`), 通知
(`/admin/notifications/:channel`) and 流量与监控 (`/admin/monitor/:section`).
The old paths `/admin/mfa`, `/admin/access-groups`, `/admin/telegram`,
`/admin/traffic-hourly` and `/admin/forward/observability` redirect, so
bookmarks keep working. Settings pages ask before leaving with unsaved
changes, and a node's API key is read only when 凭据 asks for it. Users no
longer file a ticket to have their link reset.

**For operators:** move the `identity-platform` installation to 4.1.0-rc.5:
it serves the new reset route (identity group A; installations already cut
over serve it natively with no further action). On the `nftables_ansible`
forward path the table becomes `inet v2b_forward` (was `ip v2b_forward`) as
each forward is next applied, relays need Linux 5.2+ and nft 0.9.1+, IPv6
forwarding is turned on only where a forward has an IPv6 target, a tunnel's
listen address is now honoured, and **traffic numbers jump to correct
values**, so quotas are used up faster than before. The frontend is split
into new chunks and per-language message files, so a proxy or CDN that caches
`index.html` must be purged. The four repaired error messages (`缺少 API Key`,
`API Key 无效`, `节点已被禁用`, `权限不足`) change text; anything that matched the
garbled strings must match the readable ones. CI got faster without dropping
a test or renaming a required check.

**What did not change:** the database schema (no migration), configuration
keys and environment variables, `config/editions.json`, the existing API
routes and their answers (one route is new; the v2 catalog has 296 routes),
the Agent contract, the gost / NodeX and clean-agent forward paths, and the
forward suite's flows and Flux-compatible API.

### Added

- **Users reset their own subscription link** (owner decision 2026-10-02):
  `POST /api/v2/user/subscription/reset`
  (`identity.user.subscription.reset.post`), in every edition.
  - Rotates exactly what the administrator's reset rotates: a new
    subscription token, the proxy UUID kept, recorded in the subscriber
    request ledger (`identity.user_reset_subscribe:<user>:<digest>`), so a
    retry with the same `Idempotency-Key` applies once. The answer is the
    new token in the v2 panel envelope.
  - Re-authenticated, rate-limited and audited (see Security).
  - identity-platform owns it as a group A route (it checks credentials,
    which identity owns once authoritative): native handler, identity
    bridge handler, parity tests on SQLite and PostgreSQL. Catalog: 296
    routes (174 native-flagged, 91 bridged, 31 kernel-owned).
  - 订阅 page: 「重置链接…」 opens a dialog for the password, or the code
    boxes / a recovery code, then shows the new link and its QR code and
    reloads the page (toast 订阅链接已重置，请在所有设备重新导入). The
    reset-request ticket is gone. A profile request still in flight can no
    longer put the old link back on the page after a reset
    (`web/src/stores/user.js`); the user-portal e2e test holds the profile
    answer until the reset is done, so it covers that race on every run.
  - While identity is authoritative, a group A route the stored route
    modes do not name resolves to native (`service.ResolvePackageRouteModes`,
    for the host's configuration and the group A check), so installations
    cut over before this route joined group A serve it natively with no
    operator action; before the cutover a missing route stays legacy.

### Changed

- **Sign-in and user pages redesigned (UI redesign phase U5)**
  (`web/src/views/Login.vue`, `views/user/*`, `views/Account.vue`;
  `docs/reference/frontend-design.md` "Sign-in and user pages"). Same
  endpoints and routes; page state (search, category, article, ticket, new
  ticket) lives in the query.
  - 登录: one centred card on a brand-tinted backdrop, the decorative stats
    gone. Two-factor authentication is a second step with six code boxes
    (paste, autofill, auto-advance, Backspace) that submits on the sixth
    digit; 使用恢复码 appears only when the account has recovery codes. An
    account the administrator requires to use two-factor authentication but
    that has none gets steps to follow instead of a bare error. No
    "忘记密码" link (there is no reset endpoint); registration and the invite
    code follow the public config.
  - 概览: greeting, a hero card with the remaining traffic in a brand-gradient
    ring, status, expiry and usage, 复制订阅链接 and 导入到客户端, the first help
    articles and the latest tickets; plans and orders in the commercial
    edition.
  - 订阅: the link with a real, scannable QR code, one-click import for
    Clash Verge, Shadowrocket, sing-box, Stash, Surge, Quantumult X and Loon
    (v2rayN copies its link), every other format with copy and preview, and
    a danger zone that resets the link (see Added).
  - 帮助中心: search (`/`), category cards, articles at reading width with a
    table of contents and previous / next instead of a dialog; Markdown-style
    bodies render as elements, never HTML.
  - 工单: list and conversation side by side, full width on phones; new
    tickets in a Sheet. 套餐 / 订单: store-style plan cards with a period
    switch and checkout, orders with a details Sheet.
  - 账户: two-factor setup with a QR code and the code boxes; "备用码" are
    now "恢复码" / recovery codes, with a download. The admin MFA policy
    page says 恢复码 / recovery codes too.
  - Every page has a skeleton after 300 ms, an empty state, and an error
    state with 重试 and 复制错误详情. Page titles follow the navigation
    (概览, 订阅, 帮助中心, 工单, 套餐, 订单).
  - New components `UiOtpField` and `UiQrCode` (the `uqr` 0.1.3 encoder, MIT,
    3.8 KB gzip in its own chunk, loaded on first use). Badges and unselected
    segmented-control items now reach 4.5:1 on every background they sit on.
  - Bundle: the login page grows by about 12 KB gzip (form and code fields,
    new strings), the user pages by 10–18 KB.

- **Admin list pages on one template (UI redesign phase U6)**
  (`web/src/ui/UiDataTable.vue`, `views/admin/*`;
  `docs/reference/frontend-design.md` "List pages"). Same endpoints,
  request fields, permission and edition checks.
  - New `UiDataTable`: column definitions; sorting and pagination on the
    client or the server (`UiPagination`); hidden columns and row height
    remembered per table; row selection with a bulk bar that floats up from
    the bottom; a "…" row menu (`UiMenu`); rows that open with a click or
    Enter (↑/↓ move); error with 重试 / 复制错误详情, skeleton rows after
    300 ms, empty and "no results" states; a header that sticks under the
    top bar; one card per row on phones instead of sideways scrolling. Also
    `UiSearchField` (`/` focuses it), `UiFilterChips`, `UiErrorState` (the
    user pages' `LoadError` now wraps it) and `UiUsageBar`.
  - Migrated: 用户 (status chips with counts, 流量用尽 for the loaded page,
    usage bars, a detail Sheet, bulk 封禁 / 解封 with 撤销), 工单 (an inbox:
    queue and ticket side by side, quick replies), 帮助中心内容 (Markdown
    editor with a live preview that renders elements, never HTML), 插件中心
    (card grid and details Sheet), NodeX Agents, Ansible 机器 (visuals only),
    邀请码 (bulk copy and revoke), 访问组 (the group in a Sheet), and in the
    commercial edition 订单, 优惠券, 套餐, 支付 and 邀请返佣. Page titles
    follow the navigation (用户, 工单, 插件中心, 支付, 邀请返佣…).
  - Bulk actions without a bulk endpoint (ban users, revoke invite codes)
    call the existing per-item endpoint for each selected row.
  - `web/scripts/data-table-pages.mjs` lists the migrated pages; ESLint
    (`vue/no-restricted-html-elements`) and
    `src/__tests__/dataTableGuard.test.js` reject a bare `<table>` in them.
  - Library buttons, checkboxes, switches and chips no longer grow to 40 px
    on phones through the legacy global `button` rule; selected rows, the
    segmented tabs and the operation timeline badges reach 4.5:1.
  - Bundle: `UiDataTable` and its parts are a shared chunk of about 10 KB
    gzip; each migrated page changes by −0.6 to +1.4 KB gzip; the locale
    files grow by about 2 KB each.

- **Nodes: a list and a node page (UI redesign phase U7)**
  (`web/src/views/admin/Nodes.vue`, `NodeDetail.vue`, `views/admin/nodes/`;
  `docs/reference/frontend-design.md` "Node pages"). Same endpoints, request
  bodies, permission and edition checks; one new read of an existing route,
  `GET /admin/nodes/:id` (`getNode`), for the node page.
  - 节点 list on the list template: name and tags, address, protocol count,
    status (with runtime health and 本月超限), Agent version, load (CPU and
    memory of the last report, online nodes only), last heartbeat as a
    relative time; ID, parent, total traffic and monthly quota in the column
    settings. Server search and status chips (在线 / 离线 / 已停用 / 待激活,
    the API's `status`) and the page are in the URL. Headers and dates no
    longer wrap at 1440 px; phones get cards with status and heartbeat.
  - A row opens the new node page `/admin/nodes/:id` (`?section=`): back
    link, name, status, 编辑 and 同步并重载, then 概览 (health, traffic,
    settings) / 协议 / 凭据 / 部署 / 日志 / 危险区.
  - The seven dialogs became sheets and sections: add / edit node and the
    protocol editor are sheets (inline field errors); the protocol list and
    the logs are sections with `UiDataTable`; the registration key and the
    parent-node Ansible helper are sheets on the list and part of 部署; the
    template picker stays a small dialog.
  - 凭据 reads the node's API key only when asked (each read is audited as a
    reveal) and shows it masked with reveal and copy; the shared secret is
    still never shown. 危险区 disables or enables a node (the edit body with
    status 3 / 0) and deletes it after the name is typed.
  - The 3,272-line page is now 21 files, none over 480 lines; `admin.nodes`
    strings moved to `locales/modules/*/adminNodes.js`.

- **转发节点 on the list and detail templates (UI redesign phase U7)**
  (`web/src/views/admin/ForwardNodes.vue`, `AnsibleMachines.vue`,
  `NodeX.vue`, `LocalRuntime.vue`, `views/admin/forward-nodes/`;
  `docs/guide/flux-panel-clone.md`). Same endpoints, inventory scopes,
  system config keys, request fields and confirmations. Execution plane
  only: the Flux control plane (转发, 隧道, 限速) is untouched.
  - The four pages behind the sidebar item 转发节点 stay separate (NodeX and
    local Ansible are separate runtime paths with separate endpoints, see
    AGENTS.md) but share the header 转发节点 and a run-mode switch: NodeX
    拓扑 · Ansible 机器 · 本地运行时 · NodeX 运行时, and a link to NodeX
    Agents. The forward suite navigation (a control-plane control) no longer
    shows above these pages; the sidebar item 转发 leads back.
  - NodeX 拓扑: the node cards became a `UiDataTable` with server pages,
    type and reachability chips, the stats as a summary row and the
    actions in the row menu; the legacy rules are a second table with the
    user ID filter. Node, rule and connection-test forms use the Ui fields.
  - New detail pages `/admin/forward/nodes/:id` and
    `/admin/forward/ansible-machines/:id` (a row opens them): 概览 (status,
    traffic, last check result, sync, connection test), 配置 (the stored
    fields, the token only as 已设置) and 危险操作 (enable / disable, delete
    after typing the name); the section is kept in `?tab=`.
  - NodeX 运行时 and 本地运行时: grouped settings with a switch and Ui
    fields, the probes as grouped lists, copyable troubleshooting commands,
    the Doctor output and the latest jobs as a table.
  - `ForwardNodesI18n.vue` is gone: since the i18n change of April 2026
    `ForwardNodes.vue` only wrapped it; the page now lives in
    `ForwardNodes.vue` again. The NodeX node form no longer keeps the
    metrics port of the node edited before when it adds a new one.
  - Bundle (JS and CSS a route loads beyond the shell, gzip): NodeX 拓扑
    37.1 → 30.6 KB (no longer pulls the whole `@/ui` barrel), Ansible 机器
    17.3 → 20.8 KB, 本地运行时 7.5 → 28.1 KB and NodeX 运行时 5.8 → 23.9 KB
    (they now load the shared `UiDataTable`, section, switch and state
    chunks); the detail pages load 15.4 KB (node) and 11.1 KB (machine).

- **Forward suite redesigned (UI redesign phase U7, forward part)**
  (`web/src/views/admin/Forward.vue` + `views/admin/forward/`, `Tunnel.vue`,
  `LimitI18n.vue`, `ForwardWizard.vue`, `components/admin/ForwardSuiteNav.vue`;
  `docs/guide/flux-panel-clone.md`, `docs/reference/frontend-design.md`
  "Forward suite and the wizard template"). Flux-panel clone: visuals and
  interaction components only. Same endpoints, request bodies (including the
  order payload), fields, permission checks and flows; no runtime backend
  selector, runtime job table, installer or inventory control.
  - Sub-navigation: 流量转发 / 隧道 / 限速 are a segmented control, with
    快速配置向导 before it and NodeX 拓扑 / 更多 after it (same links and
    routes).
  - 流量转发: the direct view is a `UiDataTable` (search, tunnel filter,
    status chips, selection with a bulk bar for 恢复 / 暂停 / 导出 / 删除, the
    service switch in the row); the grouped view (user → tunnel) stays,
    behind a 直连 / 分组 switch. 编辑 / 诊断 / 删除 and the new 上移 / 下移 (the
    keyboard and touch alternative to the drag handle, same
    `forward/update-order` payload) are in the row menu; 导入 / 导出 in the
    page's "…" menu. The editor is a Sheet; the diagnosis is a timeline in
    a Sheet built from the existing `results[]`; a failed first load shows
    重试. `Forward.vue` went from 3 425 to about 1 900 lines (page-local
    table, grouped view, dialogs, and the pure helpers in `forwardModel.js`).
  - 隧道 and 限速: `UiDataTable` lists with editor Sheets (the tunnel
    diagnosis as a timeline), error states with 重试.
  - 快速配置向导: the wizard template, steps on the left, the form on the
    right, 上一步 / 下一步 at the bottom; the five steps, fields and API calls
    are unchanged; a generated node token is a copy field.
  - New `components/admin/forward/DiagnosisTimeline.vue`. Forward, Tunnel and
    LimitI18n join the `UiDataTable` guard list.
  - The rules table's 状态 column is one line: the service switch and one
    badge with the worst state (异常 / 同步失败 > 执行中 / 待下发 > 暂停 >
    已应用 > 正常); the other state and the runtime message are a tooltip on
    the badge (also in its accessible name) and a truncated second line on
    phone cards. Rows are 52–60 px again.
  - `UiDataTable`: the bulk bar stays under the overlays' layer and fades out
    while a modal Dialog or Sheet is open (`ui/composables/useModalOpen.js`).
  - `UiRadioGroup`: the radio no longer stretches to 40 / 44 px on phones
    through the legacy global `button` rule.

- **Settings, subscription groups, notifications and security (UI redesign
  phase U7)** (`web/src/views/admin/{System,Subscriptions,SubscriptionGroup,Notifications,Security,MFA}.vue`
  and their folders; `docs/reference/frontend-design.md` "Settings pages").
  Same endpoints, request fields, permission and edition checks.
  - 系统设置 on the settings template: a section list on the left (a list,
    then the section with a back link, on phones) and the section in the
    path, `/admin/system/:section` (通用, 转发运行时, 备份, 负载均衡,
    审计日志, 关于; `/admin/system` shows 通用). Each section loads its own
    data. Forms validate as you type; a 保存 / 放弃 bar floats up while
    they have unsaved changes, and leaving the section or the page asks
    first (`useUnsavedChanges`). The audit log (filters and page in the
    query), configuration keys, backups, load balancers and runtime jobs are
    `UiDataTable`s; load failures are error states with 重试 instead of empty
    lists. 关于 lists the same version and build rows as the account menu's
    关于 (`aboutRows` in `utils/systemInfo.js`). The runtime workbench (backend
    status, doctor, jobs, operator commands) stays here, not on the forward
    pages.
  - 订阅分组: the groups in a `UiDataTable` with their usage; a group page,
    `/admin/subscriptions/:id/:section`, with segmented tabs for 概览, 节点模板, 节点协议 (picker
    dialog), 成员 (counts: the API has no member list) and 订阅输出 (server
    preview in the new `UiCodeBlock`, copy and download).
  - 通知: e-mail (SMTP settings with the save bar and 测试发送), Telegram (bot
    with the save bar, webhook, send to one user or everyone, linked users,
    commands), templates and the send log in one page with segmented tabs,
    `/admin/notifications/:channel`. `/admin/telegram` redirects.
  - 安全: the MFA policy (now switches, ranges and the save bar) and the
    access groups as sections, `/admin/security/:section`; `/admin/mfa` and
    `/admin/access-groups` redirect. There is no third section because the
    API has no administrator tokens.
  - The sidebar's 系统 group is 系统设置, 安全, 通知; the command palette
    lists every section under its page, so 访问组 or 审计日志 are still
    found by name. The breadcrumb no longer repeats a group named like the
    page (用户 › 用户) and names the section (系统 › 系统设置 › 备份).
  - `UiGroupedListRow`: a row with only a control, and stacked fields on
    phones, take the full width.
  - `UiDataTable` columns take `nowrap`, `truncate` (ellipsis, full text in
    the cell's title), `minWidth` and `maxWidth`. The audit log keeps short
    columns on one line, labels known actions, modules and results
    (更新, 删除, 成功…), truncates the operator and clamps 内容 to two lines.
  - The subscription group page's section tabs are segmented, like the other
    detail pages.

- **Dashboard, 流量与监控 and 部署编排 (UI redesign phase U8)**
  (`web/src/ui/UiChart.vue`, `UiMetricCard.vue`,
  `web/src/views/admin/Dashboard.vue`, `Monitor.vue`, `Deployments.vue`,
  `views/admin/{dashboard,monitor,deployments}/`;
  `docs/reference/frontend-design.md` "Charts" and "Dashboards and
  monitoring (U8)"). Same endpoints, request bodies, permission and edition
  checks; no backend change.
  - `UiChart`: the one ECharts wrapper. Tree-shaken (`echarts/core` with
    line and bar series, grid, tooltip, legend, canvas), loaded on first
    use into the `echarts` chunk (about 180 KB gzip, was 374 KB), never in
    the entry chunk. The AnixOps Design theme comes from the tokens and
    follows light / dark (pure black) live; it resizes with its box; it has
    loading (delayed chart skeleton), empty and error (重试) states; the plot
    is `role="img"` with a summary, and 以表格查看 shows the data as a table
    that screen readers always read. `UiMetricCard`: label, big number,
    trend with a word, detail line and an optional token-coloured sparkline.
    Histoire stories and unit tests for both; `UiSkeleton` gains `chart`.
  - 仪表盘 on the dashboard template (plan §7.4): 用户 / 在线节点 / 今日流量 /
    待处理工单 cards (each opens its page), the 24-hour traffic chart, then
    需要处理 (offline nodes, tickets waiting for a reply, stalled traffic
    reports; pending orders in the commercial edition) and 最近操作 (the
    audit log). Each block loads, fails and retries on its own; 刷新 asks
    the dashboard API for fresh numbers.
  - 流量与监控 (`/admin/monitor/:section`) merges 实时监控, 小时流量 and
    转发可观测性: 实时节点 (the monitor WebSocket, now a `UiDataTable`), 用户流量
    (all users or one), 节点延迟 (the prober's node targets) and 转发
    (topology graph, ingress comparison, runtime jobs). The section is in
    the path and the time range (1 小时 / 24 小时 / 7 天 / 30 天) in `?range=`
    for the sections whose API takes one. `/admin/traffic-hourly` and
    `/admin/forward/observability` redirect; the menu has one 流量与监控
    entry and ⌘K lists its sections.
  - 部署编排: the page is split into page-local panels and
    `useDeploymentCenter.js`; topologies and node roles on `UiDataTable`;
    the node-role editor (`AssignmentDrawer`) is a `UiSheet`; the G6
    topology (`TopologyGraph.vue`, shared with 流量与监控) follows the theme
    live; the operation timeline shared with 插件中心 is a timeline list
    with state words instead of a bare table.

- **Performance budget and legacy CSS cleanup (UI redesign phase U9)**
  (`web/vite.config.js`, `web/src/i18n.js`, `web/src/locales/`,
  `web/src/utils/request.js`, `web/src/style.css`,
  `web/scripts/check-bundle-budget.mjs`, `web/bundle-budget.json`;
  `docs/reference/frontend-design.md` "Bundle" and "Styles"). No API,
  permission or edition change.
  - First-visit gzip size (JS and CSS, zh-CN): sign-in 278 → 115 KB, user
    home 289 → 168 KB, admin dashboard 307 → 237 KB (with its messages);
    axios (18 KB) now loads with the first request.
  - Locale messages are split into a core group and an admin group per
    language. Only the active language loads (en no longer fetches zh-CN
    as a fallback; `localeParity.test.js` keeps the keys equal); the admin
    group loads when an administrator is signed in.
  - The single `api` chunk is gone: API modules go with the routes that use
    them; Vue's `@vue/*` packages join `vue-vendor`; Reka UI is shared per
    route instead of one 52 KB `ui-vendor` chunk; the extension runtime
    loads with the first admin page; vue-i18n drops its legacy API.
  - `npm run bundle:budget` sums what the first visit to the sign-in page,
    the user home and the admin dashboard downloads, and checks every lazy
    chunk, against `web/bundle-budget.json` (sign-in 120 KB, the plan §13
    budget; admin shell and dashboard 250 KB; route chunks 80 KB). CI runs
    it after the build, and now runs ESLint and stylelint too.
  - `style.css` loses the pre-redesign variable aliases (`--primary-color`,
    `--text-color`, ...), the unused utility and page classes (`.card`,
    `.tabs`, `.grid-*`, `.page-toolbar`, ...), the phone rule that set every
    button to 40 / 44 px and the 14 px phone root size. What remains is the
    element baseline and the classes signed plugin WebUI bundles render
    (`.btn`, `.data-table`, `.empty-state`, ...). The stylelint rules are
    errors now and reject the removed variables.

- **Mobile polish, accessibility to zero, visual regression baseline and
  page follow-ups (UI redesign phase U9, polish)** (`web/src/ui/`,
  `web/src/views/`, `web/src/components/`, `web/e2e/`,
  `web/playwright.visual.config.js`, `.github/workflows/ci.yml` job
  `frontend-visual`, `.github/workflows/frontend-visual-baselines.yml`;
  `docs/reference/frontend-design.md` "Accessibility and visual regression
  tests (U9)"). Same endpoints, request bodies, permission and edition
  checks; no backend change.
  - Every admin and user route swept at 390 and 360 px with touch
    emulation, light and dark: no horizontal scroll; every control is a
    44 px touch target on coarse pointers (an `::after` hit area where the
    look stays: buttons, close buttons, switches, segmented tabs and
    controls, locale options, data-table card titles, back links, suite
    and mode navs, dashboard and help links); medium fields are 44 px tall
    there; tab rows that do not fit fade on the clipped edge and keep the
    active tab in view (subscription group tabs at 390); the forward
    wizard footer clears the home indicator.
  - Accessibility: axe finds no serious or critical issue on 23 admin and
    user screens in both themes at 1440 and 390 px, and the moderate ones
    found are fixed (the admin sidebar is a labelled complementary
    landmark; search fields, table pagination and the traffic summary
    carry their own names; heading levels on the forward node page). Charts
    and topology graphs turn their animation off under
    `prefers-reduced-motion`. `web/e2e/a11y.spec.js` (`@axe-core/playwright`,
    a dev dependency) runs in the Frontend Build job and also checks the
    skip link, landmarks, one `h1` and focus return after a dialog.
  - Visual regression: 28 full-page screenshots of 12 key screens with the
    API mocked, a fixed clock, UTC, English and no animation, compared in
    the official Playwright image by the new Frontend Visual Regression job;
    `npm run test:visual` / `test:visual:update` run the same image locally
    with docker, and the Frontend Visual Baselines workflow regenerates them
    without docker.
  - Admin lists keep search, filter chips and page in the URL query (plan
    §9): users, orders, tickets, plugins, agents, coupons and invite codes,
    as nodes already did (`useListQuery`).
  - 部署编排 is the page title in both locale layers; its tab bar is a
    segmented `UiTabs`; the topology workspace's pickers are `UiSelect`; the
    node config and deploy previews use `UiCodeBlock` (new `copyLabel`,
    `copyDisabled`); the NodeX and local runtime pages use the settings
    save / discard bar with the leave prompt.
  - 转发 says when the runtime is nftables / Ansible that Primary / Backup
    and Hash use only the first target and speed limits are not enforced.
  - Safe areas: `viewport-fit=cover`, and the admin top bar and drawer, the
    user bar and tab bar, sheets, the wizard footer and `body` (landscape)
    pad themselves with `env(safe-area-inset-*)`; checked on an emulated
    iPhone 13 with a notch. The skip link is a 44 px target.
  - Known follow-ups: an open action menu sits outside the landmarks (axe
    "region", moderate); the plugin drawer's target tabs move to `UiTabs`;
    the topology workspace's text inputs move to `UiTextField`.

- **CI: shorter critical path** (`.github/workflows/ci.yml`,
  `config/scripts/classify_changes.py`, `config/scripts/plan_test_shards.py`;
  AGENTS.md "Job graph"). Not user-facing. No test is dropped and the
  required check names are unchanged.
  - The Go tests run once, in "Backend Tests (1/3)" to "(3/3)", balanced by
    measured time (`-p=4`; `internal/kernelnodeops` runs alone afterwards);
    "Backend Tests" stays the required check, fails unless every package
    ran exactly once, and merges the coverage. "Go Quality Gates" keeps only
    the static gates. The PostgreSQL package tests run in four shards, each
    with its own PostgreSQL.
  - "Build Smoke Images" builds the Control and identity-platform images
    once (buildx GitHub Actions cache) for "Docker Build Smoke" and
    "Kubernetes Smoke", which no longer rebuild or wait for each other or
    for the Go jobs (a 24-minute serial chain before). "Smoke Tests" and
    "E2E Tests" start at once. The edge publish keeps its gates, now named
    explicitly.
  - The Go race detector (about 28 minutes) runs only nightly and on manual
    runs, with a 40-minute timeout, and no release job waits for it
    (`check_release_workflow.sh` updated).
  - A pull request that changes `ci.yml` only inside class-gated jobs runs
    those classes instead of the full lane; other `.github/` files no
    longer force the full lane, `.github/actions/` still does.

### Fixed

- **CI: the live Control WebUI E2E login retries on 429** (`web/e2e/support/live-control-machine-telemetry.mjs`). The public route limiter is per IP and shared with the browser under test, so a burst of page requests could fail the login; it now waits for `Retry-After` and retries within its deadline. Not user-facing.

- **nftables forwards count traffic in both directions, support IPv6, and
  move to an `inet` table** (`nftables_ansible` backend;
  `config/deploy/ansible/playbooks/forward_*_nftables.yml`, new
  `playbooks/files/v2b_forward_nft.sh`, `internal/service/forward_nftables_plan.go`,
  `forward_ansible_stats_worker.go`; upgrade notes in `docs/UPGRADE.md`).
  **Upgrade-impacting:** traffic numbers, and with them quota use, rise to
  the real values.
  - Traffic was counted on the NAT chain, which sees only the first packet
    of each connection, and download was always 0. A `forward` hook chain
    now counts every packet into two named counters per forward and
    protocol: conntrack original direction as upload, reply direction as
    download (the gost path's `u`/`d`, with the same ratio and one-way /
    two-way billing). The counters survive re-apply and pause; the stats
    playbook prints both, and the worker keeps them on their own traffic
    cursor (`nftables_ansible:ct`) so the switch from the old counter is not
    read as a reset.
  - The table is `inet v2b_forward` (was `ip v2b_forward`). IPv6 targets
    (`[v6]:port`) and IPv6 listen addresses work, with IPv6 masquerade;
    target lists with both families are balanced per family. IPv6
    forwarding is enabled only when a forward has an IPv6 target. A forward's
    first apply after the upgrade removes it from the old table in the same
    nft transaction; the old table goes with its last forward. Pause and
    delete clean both tables.
  - Each apply is one atomic `nft -f` transaction, and the panel renders the
    ruleset (validated targets and interface names). `round`/`rand` over
    several targets did not load before (a `vmap` cannot hold `dnat`); they
    now jump to one chain per target. Changing a tunnel's protocol or a
    forward's targets no longer leaves the old protocol's or targets' rules
    behind.
  - The stats playbook printed its lines only as task stdout, which the
    default Ansible callback does not show; a debug task prints them now,
    and the worker reads both forms.
  - Documented: `fifo` (主备) and `hash` use only the first target on this
    path, and speed limits are not enforced on it
    (`docs/guide/forward-tunnel-runtime-ops.md`).
- GBK mojibake (UTF-8 once decoded as GBK and saved again) is repaired in
  `internal/handler/{agent,invite,telegram}.go`, `internal/middleware`,
  `internal/database` and `internal/service/service_test.go`, with the line
  breaks and separators the lost bytes had swallowed. Four API error
  messages read correctly again (`缺少 API Key`, `API Key 无效`,
  `节点已被禁用`, `权限不足`). Regenerated Swagger: Chinese summaries and
  tags are readable, and the agent heartbeat and WebSocket, admin execute and
  Telegram user-notify operations get the tags their merged annotations had
  lost. `docs/DEPLOYMENT.md` loses its BOM. A new gate,
  `config/scripts/check_mojibake.py` (CI "Documentation Sync
  Check", so it runs on every pull request) rejects GBK mojibake runs,
  `U+FFFD`, private-use characters, BOMs and non-UTF-8 text in tracked files.
- Flux clone docs (`docs/guide/flux-forward-contract.md`,
  `docs/guide/flux-panel-clone.md`) no longer say forward create, update,
  delete, pause and resume are "mainly DB-layer": each runs
  `syncForwardRuntime` on the selected backend.
- The legacy literal translator (`web/src/utils/legacyI18n.js`) wrote the
  first text it saw back over later updates, so labels that change in place
  (a copy button turning into "已复制", a form switching to registration)
  snapped back. It now treats any value it did not write as the new source.
- Vue Router no longer warns about the unnamed empty-path child of the
  `admin` route (now `admin-index`).

### Security

- **Self-service subscription reset re-authenticates.**
  `POST /api/v2/user/subscription/reset` asks for the current password, or
  with two-step verification on, a TOTP or recovery code. Three attempts
  that check a credential per user and hour, then `Retry-After`. It is
  audited like the administrator's reset, as `user` / `reset_subscribe`,
  without the request body (it holds the credential).

## 4.1.0-rc.4 - 2026-10-02

4.1.0-rc.4 is the fourth 4.1.0 release candidate and the **UI redesign
preview**: the web app adopts AnixOps Design v1.0.2 (new colours, type, brand
mark, favicon and app icons, a pure-black dark mode), gets new admin and user
shells (frosted sidebar regrouped by object, a `⌘K` / `Ctrl+K` command
palette, an account page, 404 and 无权限 pages), and replaces every browser
`alert`/`confirm`/`prompt` with in-app toasts and dialogs. **For users:**
language and theme (now with "跟随系统") live in the account menu, and the user
bar becomes a bottom tab bar on phones. **For admins:** the sidebar groups are
概览, 用户, 网络, 扩展, 系统 (and 商业 in the commercial edition); the version
line moved to 账户菜单 → 关于; reversible actions (ban, gateway on/off, removing
a group or member) happen at once with 撤销 for 5 s instead of an "are you
sure?"; deleting a node, NodeX forward node or Ansible machine asks you to type
its name; a Telegram broadcast now asks first. Administrators can also
**generate and revoke invite codes in every edition** (用户 → 邀请码); the new
routes are served by `identity-platform`, so **install this release's
`identity-platform` package** with Control. **What did not change:** page
bodies (the fields, tables and flows inside each page), the existing API
and page routes (only the three invite-code routes are new), the database
schema, the forward suite's flows and `config/editions.json`. The new
assets have new hashed names, so a proxy or CDN that caches `index.html`
aggressively must be purged. `docs/UPGRADE.md`, "Upgrading From 4.1.0-rc.3 To
4.1.0-rc.4", has the checklist.

### Changed

- **The web UI adopts AnixOps Design v1.0.2** (UI redesign phase U1,
  `docs/reference/frontend-design.md`). Colours, type, radii, shadows,
  favicon and app icons change.
  - **Vendored design system.** `web/src/design/` holds `tokens.css`, the
    mark, wordmarks, favicon and PWA icons, and self-hosted Inter (Latin
    subset, `font-display: swap`), copied unmodified from
    `AnixOps/AnixOps-design` at `v1.0.2`. `npm run design:sync -- --tag
    <tag>` updates them; `npm run design:check` (Frontend Build CI job,
    offline) compares them with the SHA-256 manifest. The mark and wordmark
    are not MIT: all rights reserved, AnixOps products only
    (`web/src/design/LICENSE-BRAND.md`).
  - **Theme.** Light by default, follows `prefers-color-scheme` until the
    user picks a theme, and `data-theme` overrides it; dark is pure black
    with `#1C1C1E` cards. The accent moves from `#0064FA` to `#4F5BE8`
    (dark `#818CF8`, filled buttons `#5B63E6`).
  - **Legacy bridge.** Every pre-redesign variable (`--primary-color`,
    `--surface-color`, `--text-*`, the admin sidebar set, ...) is now an
    alias of a token, and the global `.btn`, `.card`, `.data-table`,
    `.tabs` and form classes are restyled through tokens: pill buttons,
    10/14/20 px radii, hairline separators.
  - **Brand.** The admin sidebar, user header and login page show the
    AnixOps mark lockup ("AnixOps" + "Control") instead of the text and
    icon placeholders. The browser tab, home-screen icon and web app
    manifest use the AnixOps icons; `theme-color` follows the theme.
  - **Charts.** The hourly traffic and observability charts and the
    topology graph take their colours from the chart tokens and re-theme
    when the theme changes.
- **New navigation: admin and user shells rebuilt** (UI redesign phase U3,
  `docs/reference/frontend-design.md` "App shell").
  - **Admin sidebar** regrouped by object: 概览, 用户, 网络 (节点, 转发,
    转发节点, NodeX Agents), 扩展, 系统, and 商业 in the commercial edition
    only. Light frosted material (dark in dark mode), the selected page a
    rounded fill with an accent icon, ↑/↓ between links, and a toggle that
    collapses it to an icon rail (remembered). Below 834 px it is a modal
    drawer. Everything it shows comes from one menu config
    (`web/src/navigation/menu.js`) that merges the built-in items, the
    edition, permissions and plugin menus (`services` and `operations` under
    扩展, `system` under 系统).
  - **Top bar**: breadcrumb, and a search button that opens the new command
    palette (`⌘K` / `Ctrl+K`): jump to any page the admin can see, open the
    existing create flows (添加节点, 添加用户, 转发快速向导), switch appearance
    or language, and find users by email. The ticking clock and the fixed
    subtitle are gone; admin content keeps to 1280 px (1440 px for wide
    table pages).
  - **Where things moved.** Language and theme are in the account menu
    (avatar at the bottom of the sidebar; in the top bar on phones), with a
    new "跟随系统" (system) choice; the sidebar's second language switcher and
    the header theme toggle are gone. The version and build line moved from
    the sidebar to 账户菜单 → 关于.
  - **Forward suite**: its sub-navigation (快速配置向导, 流量转发, 隧道, 限速,
    NodeX 拓扑, 更多) now appears once, as a segmented strip at the top of
    every `/admin/forward*` page, instead of both in the sidebar and inside
    four pages. Routes and the seven legacy redirects are unchanged.
  - **User shell**: a 48 px frosted bar with centred links (概览, 订阅, 帮助中心,
    工单, 账户) and the account menu; below 834 px a bottom tab bar with
    safe-area padding replaces the side drawer and the text icons.
  - **New pages**: 账户 (`/user/account`, `/admin/account`: profile,
    two-factor authentication on the existing `/user/mfa/*` endpoints,
    language, appearance), 404 for unknown paths, and 无权限 when a signed-in
    user who is not an administrator opens an admin page (it used to
    redirect silently to the user dashboard). `/admin` and `/user` open
    their dashboards.
  - Pages fade in (240 ms, 8 px rise; fade only under reduced motion); going
    back restores the list's scroll position. The shells load after sign-in,
    so the login page stays at 112 KB gzip.
- **Browser pop-ups replaced by in-app toasts and dialogs** (UI redesign
  phase U4, `docs/reference/frontend-design.md` "Feedback and dialogs in
  pages"). No admin or user page opens the browser's `alert`, `confirm` or
  `prompt` any more, and every hand-rolled pop-up window is the shared
  dialog or side sheet: same fields, buttons and API calls.
  - **Results** appear as a toast at the bottom (success disappears after
    3 s; errors stay until closed). Errors in a form stay in the open
    dialog, next to the field or above its buttons, instead of a pop-up
    that closed the form's context.
  - **Undo instead of "are you sure?"** where the action has a real
    inverse: banning or unbanning a user, enabling or disabling a payment
    gateway, deleting the Telegram webhook, removing a group from a plan,
    removing a member or plan from an access group. The action happens at
    once and the toast offers 撤销 for 5 s.
  - **Confirmations** for irreversible actions name the object and say what
    happens, with a red button named after the action (「删除节点」, not
    「确定」); a failure is shown inside the confirmation. Deleting a node, a
    NodeX forward node or an Ansible machine now asks you to type its name.
    Sending a Telegram broadcast now asks first.
  - **Dialogs** trap focus, close with Esc and return focus to the button
    that opened them, are named for screen readers, follow dark mode, and
    form dialogs fill the screen on phones. Details open as a side sheet (a
    bottom sheet on phones): a user's 30-day traffic, order and
    payment-record details, tickets, a node's protocols, plugin details and
    deployment assignments.
  - **Forward suite** (转发, 隧道, 限速, NodeX 转发节点, Ansible 机器): visual
    change only; flows, including the second confirmation before a forward
    force delete, are unchanged. Their result banners became toasts, so a
    message is no longer hidden behind an open dialog.
- **Administrators manage invite codes in every edition** (owner decision
  2026-10-01; `internal/handler/invite_codes.go`,
  `internal/service/invite_code_admin.go`, `docs/UPGRADE.md`). Invite codes
  are registration control, so the community edition keeps them; the
  commission, withdrawals, invite statistics and configuration stay with
  the commercial `affiliate` package. Install this release's
  `identity-platform` package: it serves the new routes.
  - **Codes made here belong to no user**: they admit a registration and
    attribute no referral, and do not count toward a user's `code_count`
    limit. The user's own generation (affiliate), its limit and its
    advisory lock are unchanged.
  - **Bridged.** The identity-platform host relays the new routes to the
    kernel's handler (`bridgedRoutes`), like every route whose table only
    Control reads: `v2_invite_code` is consumed by registration inside
    Control. The v2 catalog grows to 295 routes (91 bridged, 173
    native-flagged, 31 kernel-owned); `config/editions.json` is unchanged.
- **Frontend lint.** `no-alert` and the legacy `.modal*` classes are ESLint
  errors, and `npm test` fails on a native dialog or `.modal-overlay`; the
  global `.modal*` CSS and `useModalFocus` are removed. `npm run
  lint:styles` (stylelint) still only warns about colour literals and font
  sizes or radii off the token scale.

### Added

- **Admin invite-code routes and page**, owned by `identity-platform`
  (which owns registration and ships in every release):
  - `GET /api/v2/admin/invite/codes` (every code, newest first, `status` =
    `unused`, `used` or `expired`, paged), `POST /api/v2/admin/invite/codes`
    (`count` 1-50, optional `expire_days`: empty uses the configured
    `code_expire_days`, `0` never expires) and
    `DELETE /api/v2/admin/invite/codes/:id` (revokes an unused code; a used
    code is kept as the record of who registered with it). Panel envelope;
    errors are `code: -1` answers.
  - A "邀请码 / Invite codes" admin page (`/admin/invite-codes`, under 用户
    in both editions) generates, lists, filters, copies and revokes codes
    and says whether registration requires one; in commercial it links the
    Invite Rewards page, which is unchanged.
- **Web component library** (UI redesign phase U2, `web/src/ui/`,
  `docs/reference/frontend-design.md` "Components"), used by the new shells
  and dialogs; page bodies migrate to it in later phases.
  - Built on Reka UI 2.10.5 (headless; pinned) with our own scoped CSS on
    AnixOps Design tokens only: Button, IconButton, Field, TextField,
    Textarea, PasswordField, NumberField, Select, Combobox, Switch,
    Checkbox, RadioGroup, SegmentedControl, Tabs, Dialog, ConfirmDialog
    (typed-name variant for destructive actions), Sheet (right drawer,
    bottom sheet below 834 px), Toast, Badge and StatusDot (one status
    map), EmptyState, Skeleton, CopyField, PageHeader, Card, Section and
    GroupedList. Keyboard operable, announced correctly, token focus
    ring, reduced motion, light and dark, 390 px.
  - Composables: `useToast()` (success 3 s, errors persistent, undo 5 s,
    one live region, F8), `useConfirm()` (a promise instead of
    `confirm()`), `useFormat()` (bytes, rates, durations, money, numbers,
    dates, relative times on Intl and the current locale; `formatBytes`
    matches the page copies, `{ precision: 1 }` for hero numbers),
    `useDelayedLoading()` (the 300 ms rule).
  - Built-in strings in `zh-CN` and `en` under `ui.*`.
  - Histoire 1.0.0-beta.1 documents every component (`npm run story:dev`,
    `npm run story:build`), with the vendored tokens and a dark-mode
    toggle. `package.json` overrides Histoire's `vite` peer range (^7) to
    the project's Vite 8.
  - `reka-ui` and its helpers build into their own `ui-vendor` chunk.

### Fixed

- Web app, found while moving pages to the shared dialogs (U4): the forward
  import hint and the notification template hint lost text to i18n
  placeholders; the payment gateway config example logged errors;
  Subscriptions' protocol-pool checkboxes did nothing and its icon buttons
  had no names; a failed subscription cache refresh reported success; the
  plan group picker could not be used from the keyboard; several dialog
  labels were not tied to their fields.
- Component library: a dialog stays within the screen width on phones; a
  scrolling dialog body with read-only content can be scrolled from the
  keyboard; the select placeholder and the `danger-soft` button meet 4.5:1
  contrast.
- The affiliate invite-code parity test no longer fails when a 30-day expiry
  crosses a daylight-saving change in the machine's local zone: it compares
  expiries with the same calendar arithmetic as the handlers
  (`time.Now().AddDate`). Test code only.

## 4.1.0-rc.3 - 2026-10-01

4.1.0-rc.3 is the third 4.1.0 release candidate. **Two changes need action
before upgrading.** Control now runs as the **community edition** by
default: an install that uses payments, orders, coupons, plan purchase or
the invite commission must set `app.edition: commercial`
(`ANIX_CONTROL_APP_EDITION=commercial`) first, or those routes, including
payment callbacks, answer `404`; the release no longer ships the
`affiliate`, `order` and `payment` packages, so a commercial install keeps
the versions it has or builds them with
`packages/shared/build_package.py --all --edition commercial` until a
commercial channel exists. And forwarding headers are now trusted only
from `server.trusted_proxies` (default loopback only): list every reverse
proxy that is not on the same host, or links come out as `http://` or an
internal address and logs show the proxy's IP. The release page shrinks
from 104 assets to 18, with the packages in one signed archive;
`scripts/install.sh` is unaffected. Phase P3 of the node credential split
(finalize and unsplit) ships, off by default: upgrading changes no table's
phase. `docs/UPGRADE.md`, "Upgrading From 4.1.0-rc.2 To 4.1.0-rc.3", has
the checklist.

### Security

- **Forwarding headers are trusted only from configured reverse proxies**
  (`internal/requestorigin`, `docs/UPGRADE.md`, `docs/DEPLOYMENT.md`
  section 6.1). Control took the request's scheme and host from
  `X-Forwarded-Proto`/`X-Forwarded-Host` sent by any client, and built links
  from them.
  - **Affected.** Every deployment a client can reach directly, not only
    through its proxy: a published port, a LoadBalancer or NodePort
    Service, a peer in the same network. Up to 4.1.0-rc.2.
  - **What an attacker could do.** Make Control emit links to a host of
    their choosing: the clean agent install script's panel URL (when
    `forward_runtime.clean_agent.public_url` is unset), subscription links
    (when no subscription domain is set), the default Telegram webhook URL,
    and the `request_scheme`/`request_host` package hosts receive. A shared
    cache or a victim's request carrying the header then hands out a script
    that downloads from the attacker's panel. HSTS could also be forced on a
    plain-HTTP origin. The client IP behind rate limits, login throttling and
    audit logs could be set with `X-Forwarded-For` from any private address
    (the old default list), and from any client on routes bridged to legacy
    handlers, whose engine trusted every proxy.
  - **Now.** `X-Forwarded-Proto`, `X-Forwarded-Host`, `X-Forwarded-For` and
    `X-Real-IP` count only when the TCP peer is in `server.trusted_proxies`
    (`ANIX_CONTROL_SERVER_TRUSTED_PROXIES`), for the links and for gin's
    `ClientIP()` alike. Otherwise the connection decides: https over TLS,
    the `Host` header, the peer address. The forwarded host must be a valid
    `host[:port]` and the scheme `http` or `https`; the last value (the
    nearest proxy's) is used. A configured public address still wins:
    `forward_runtime.clean_agent.public_url`, `app.subscribe_domains`, a
    webhook `url`. The kernel resolves the origin once and passes no
    forwarding header to package hosts or bridged handlers; the notification
    package no longer reads `X-Forwarded-Proto`. The UI server's `/api` proxy
    replaces a client's forwarding headers with what it resolved, and sends
    the public host as `Host`.
  - **Supersedes** two 4.1.0-rc.2 entries (Added, the native
    notification and forward-agent routes): the native
    `POST /api/v2/admin/telegram/webhook` no longer honours
    `X-Forwarded-Proto` itself, and `GET /api/v2/forward-agent/install.sh`
    falls back to the origin resolved as above, not to any request's
    scheme and host.
  - **Default** `127.0.0.1/32,::1/128` (was `127.0.0.1` plus `10.0.0.0/8`,
    `172.16.0.0/12` and `192.168.0.0/16`); an empty list trusts none; an
    invalid entry stops startup. The Compose example keeps the Docker bridge
    range for a proxy on the host; the Helm chart keeps the private ranges
    for the ingress controller, to be narrowed to its pod CIDR.
  - **Operators must** add a reverse proxy that is not on the same host (or
    container) to `server.trusted_proxies` before upgrading, keeping
    loopback; otherwise links come out as `http://` or an internal address
    and logs show the proxy's IP. Also set the public addresses above.

### Changed

- **Community edition by default (`app.edition`,
  `ANIX_CONTROL_APP_EDITION`). BREAKING for installs that sell plans.**
  Control now ships as the `community` edition; `commercial` restores every
  commercial feature exactly as in 4.1.0-rc.2. **An install that uses
  payments, orders, coupons or the invite commission must set
  `app.edition: commercial` before upgrading** (`docs/UPGRADE.md`).
  - **Hidden in community.** Every `/api/v2` route of the `order`,
    `payment` (all gateways, x402, Stripe, PayPal, epay, USDT, and their
    callbacks and webhooks) and `affiliate` packages, plus the user's plan
    list (`GET /api/v2/user/plan`), answers `404`
    `{"error":{"code":"package_route_not_found",...}}`, the same body as an
    `/api/v2` path that does not exist. One table lists them
    (`config/editions.json`), and one filter in the router applies it
    (`internal/router/edition.go`, `internal/edition`).
  - **Plans become free subscription templates (订阅模板).** Administrators
    still create, edit and assign them, and they keep mapping users to
    subscription groups; the admin page hides prices and calls them
    subscription templates (the administrator's user list too: its plan
    column reads "Subscription template"), and a stored price is sent back
    unchanged.
  - **Web app.** The user menu drops Plans and Orders; the admin menu drops
    Orders, Coupons, Invite and Payment, the user balance field, the
    revenue and order cards on the dashboard, and the extension menus of
    the hidden packages; their routes redirect to the dashboard. One
    composable decides (`web/src/composables/useEdition.js`), fed by the
    new public `GET /api/v4/public/config` (edition, hidden packages,
    registration settings). Registration shows the invite-code field only
    when `auth.registration.require_invite` is on.
  - **Release packages.** `packages/shared/build_package.py --all` builds
    the community set by default (15 packages, without `affiliate`,
    `order` and `payment`); `--edition commercial` builds all 18. The
    release builds the community set, so the packages archive below holds
    15 packages and **commercial packages are no longer published**: build
    them with `--edition commercial` until a commercial channel exists.
  - **x402 is never enabled by default.** `GET /api/v2/payment/methods`
    reported x402 as enabled when no payment method was configured; it is
    now enabled only by an enabled x402 payment configuration, in either
    edition, and `POST /api/v2/payment/x402/create` refuses with
    `gateway is disabled` (the disabled-gateway answer) without one.
- **The release page ships 18 assets instead of 104**
  (`.github/workflows/ci.yml`, `packages/shared/build_package.py`,
  `docs/RELEASING.md`, `docs/UPGRADE.md`).
  - **Packages: one signed archive.** `anix-control-packages-<version>.tar.gz`
    holds every released package's `.anxp`, `.manifest.json`,
    `.manifest.sig` and `.sbom.spdx.json` (the community set, see above),
    plus one `official-public-key.pem`.
    `anix-control-packages-<version>.tar.gz.sig` is the Base64 Ed25519
    signature of the archive, made with the package signing key by the same
    `openssl` step as each manifest signature. `RELEASE_MANIFEST.json` lists
    every package under `packages` (id, version, `.anxp` size and SHA-256,
    manifest SHA-256), and `SHA256SUMS.txt` covers the archive and its
    signature. The release job verifies the signature and every package in
    the archive (`build_package.py --verify-release-archive`).
  - **Getting a package now:** download the archive, its `.sig`,
    `official-public-key.raw` and `SHA256SUMS.txt`, check the signature
    against the pinned root with `openssl pkeyutl -verify -rawin`, then
    `tar -xzf anix-control-packages-<version>.tar.gz --strip-components=1
    --wildcards '*/<plugin-id>-<version>.*'`. The exact commands are in
    `docs/UPGRADE.md`, "Getting A Package From The Release".
  - **Removed assets:** the 18 per-package `.public-key.pem` files (all the
    same official key), the per-package `.anxp`, manifest, signature and SBOM
    assets, and `anix-control-frontend.zip` (nothing consumed it;
    `anix-control-frontend.tar.gz` stays).
  - **Kept:** the six binaries, `anix-control-frontend.tar.gz`,
    `SHA256SUMS.txt`, `RELEASE_MANIFEST.json`, `RELEASE_NOTES.md`,
    `docker-image.txt`, `official-public-key.raw`, the source SBOM, and the
    `identity-platform-<version>` `.anxp`, `.manifest.json` and
    `.manifest.sig`: the frozen `scripts/install.sh` downloads that trio by
    name, so it keeps working unchanged.

### Added

- **Node credential split, phase P3: finalize and unsplit (NO-9)**
  (`internal/nodesecrets`, `docs/architecture/node-ops-service.md`
  section 4.3, `docs/UPGRADE.md`). It completes phases P1 (dual-write and
  backfill) and P2 (dual-read) of 4.1.0-rc.2. Off by default: nothing runs
  finalize, and every table stays in its phase after the upgrade.
  Production finalize needs the owner's approval and a staging rehearsal
  (D6).
  - **`anix-control node-secrets finalize -confirm <table|all>`** needs
    phase `dual_read` and a matching `verify` at most an hour old. It
    rewrites the legacy secret columns in batches, one transaction each:
    `!moved:<row id>` in every credential column (unique per row, so no
    unique index collides; a row already holding another row's tombstone
    stops the batch), an empty key hash, and the masked document in every
    JSON column. It resumes when run again, and is audited.
  - **After finalize** the readers read the new tables only and never fall
    back; the writers' `Sync` writes tombstones; `verify` compares by
    presence. A binary reading the legacy columns fails closed.
  - **`unsplit -confirm`** writes every secret back byte for byte (the
    original JSON documents are kept for it), returns the table to
    `dual_read`, drops the views that wait for finalize, and verifies. It
    refuses a table a package adopted unless `-adopted-ok`.
  - **Conditional adoption and views.** A manifest may declare
    `kernel.storage.adopt:` for `v2_node`, `v2_node_protocol` and
    `v2_forward_node`; the lease honours it only once the table is
    finalized. The views `kapi_node_public_v1`,
    `kapi_node_protocol_public_v1`, `kapi_node_credential_status_v1`,
    `kapi_registration_key_v1`, `kapi_forward_clean_agent_v1` and
    `kapi_wireguard_peer_v1` are created only after finalize and show no
    moved column. `GetCapabilities.tables` answers each table's phase.
  - **The last readers moved.** The forward node inventory asks whether a
    node has a token through the split (a live credential row since
    `dual_read`); the default registration key from the environment, the
    diagnosis scrubber and the forward node update's token-change check
    read through it too.
  - **Static gate** `config/scripts/check_moved_columns.sh` (CI): no kernel
    read of a moved credential column outside `internal/nodesecrets`, but
    for reasoned exceptions.

### Fixed

- Tests: the node gRPC listener's binding tests read a refused stream's status from `Recv` when `Send` returns `io.EOF`, instead of failing intermittently with `Unknown`.
- Tests: the bridge contract tests' SQLite databases use `_txlock=immediate` and are closed, waiting for every connection, before their temporary directory is removed, instead of failing intermittently with "directory not empty". Both had stopped the 4.1.0-rc.2 release run.

## 4.1.0-rc.2 - 2026-10-01

4.1.0-rc.2 is the second 4.1.0 release candidate. It brings thirteen
security fixes, mostly in how nodes and agents authenticate and in what
administrator answers reveal, and lands the kernel side of node
operations: the binding KernelNodeOps contract with its executors (NO-1 to
NO-8), the node credential split (phases P1 and P2), sealed secret handles
at the v2 gateway, Agent PKI, and configuration, users and reports on the
Agent Control stream (A2-1 to A2-5). The new features are opt-in and
additive on the wire; new tables are created at start and no existing
table is altered. Before upgrading, give every caller of the node gRPC
listener a node key or `grpc.api_token`, update API clients of the
administrator's user and order lists and of masked secrets, and review
users' legacy forward rules, forward targets and clean agent bindings, and
paid payments with a pending order. After upgrading, disable again any node
an agent had re-enabled, set the API port of forward nodes without one,
then run `anix-control node-secrets backfill` and `verify`.
`docs/UPGRADE.md`, "Upgrading From 4.1.0-rc.1 To 4.1.0-rc.2", has the
checklist. Upgrade the forward package before plan, and affiliate before
identity-platform.

### Security

- The node gRPC listener no longer lets in callers without a node key when
  `grpc.api_token` is empty, the default. It accepted any `authorization`
  header, so anyone who reached port 50051 could read every node's
  configuration and protocol keys (`NodeService.GetConfig`), every user's
  UUID (`UserService.GetUsers`), and report traffic, status, online users and
  logs for any node. See `docs/UPGRADE.md`.
  - **Affected.** Every build up to 4.1.0-rc.1 that ran the node gRPC
    listener (`grpc.enabled`: on, bound to 127.0.0.1, in
    `config.yaml.example`; off in `config.prod.yaml` and the Helm chart)
    where untrusted callers could reach port 50051. With `grpc.api_token`
    empty, the default, any caller was let in; whatever the token, any node
    key acted for every node and a disabled node's key still worked. Every
    transport's heartbeat re-enabled a disabled node in every deployment.
  - Without credentials only `HealthService` and `NodeService/Register`
    answer. A node authenticates with its API key (`x-api-key` and
    `x-node-id`); `grpc.api_token`, when set, is the administrator's token.
  - A node key now acts only for its node. `GetConfig`, `ReportStatus`,
    `StatusStream`, `ReportLogs`, `GetUsers`, `ReportTraffic`,
    `ReportOnline`, `TrafficStream` and `OnlineStream` compare the request's
    `node_id` with the authenticated node and answer `PermissionDenied` (a
    stream ends) when they differ; one node's key read another node's
    configuration and users. The interceptors record the authenticated node
    in the call's context for the handlers. `grpc.api_token` acts for any
    node.
  - The listener's JWT path accepts only an administrator's JWT. The server
    never configures its secret, so it is unused; a user's login JWT would
    have read node configurations.
  - A disabled node's key is refused (`PermissionDenied`), as the HTTP node
    API and the Agent control stream refuse it; a status stream open when
    the node is disabled ends at its next report.
  - Heartbeats no longer re-enable a disabled node. gRPC config fetches and
    reports, UniProxy polling, the agent WebSocket and the agent HTTP routes
    set the node online, which undid an administrator's disable while its
    agent ran. They still record `last_check_at` and set a pending or offline
    node online, and the administrator's node list shows a disabled node as
    disabled.
- UniProxy over HTTP refuses a disabled node. A node an administrator
  disabled kept polling its configuration and users over
  `/api/v1/server/UniProxy/*` (and the `/api/v2` alias), so disabling a
  node did not stop it serving. `NodeAuth` now answers 403
  (`{"error":"node disabled"}`) before the heartbeat, as the HTTP node API
  and the gRPC listener do.
  - **Affected.** Every build up to 4.1.0-rc.1 with nodes that poll UniProxy
    (V2bX, XrayR): a disabled node kept its configuration and users.
- SQL logs no longer contain the values bound to statements. With
  `database.log_level: info`, the example configuration's level, GORM logged
  every statement with its values: node API keys and secrets, tokens,
  password hashes and subscription UUIDs. Failed statements (and, at
  `warn`, slow ones) were logged with their values at the other levels too.
  Statements are now logged with their placeholders
  (`ParameterizedQueries`).
  - **Affected.** Every build up to 4.1.0-rc.1 whose `database.log_level` was
    `info` (as in `config.yaml.example`) or `warn`; at `error`, the
    `config.prod.yaml` level, failed statements were still logged with
    their values. Logs kept from those builds may hold these secrets.
- The x402 callback marks a payment paid only when it pays the payment's
  token and at least its amount. `POST /api/v2/payment/x402/callback`
  verified the confirmation service's signature but never compared the
  signed `amount` and `token` with the payment, so a confirmed transfer of
  any amount in any token paid the order.
  - **Affected.** Every build up to 4.1.0-rc.1 with an x402 gateway enabled,
    on the kernel's callback and, from #96, the payment package's native one.
  - The amount is compared in the unit `POST /api/v2/payment/x402/create`
    asks for: whole tokens (not base units such as wei), the record's
    `actual_amount`, shown with eight decimals. More is accepted.
  - The token must be the payment's (the record's `currency`, any case) and,
    when the x402 gateway sets `accept_tokens`, one of those.
  - The check runs in the transaction that marks the payment paid. A refused
    callback changes nothing and is answered `200`
    `{"status":"ok","message":"payment not applied: ..."}`, like the other
    callbacks that change nothing, so it is not redelivered.
  - Known gaps: a payment created without a `token` is stored with the
    column default currency `CNY` while its amount is a token amount, so no
    callback pays it; and the token amount is still the create route's
    placeholder conversion (order total in cents / 10^8), not an exchange
    rate.
- **Invite codes are capped.** A user holds at most `code_count` unused
  invite codes, as v2board limits them (`invite_gen_limit`).
  `POST /api/v2/user/invite/generate` created codes without limit, and the
  invite configuration's `code_count` was stored but unused.
  - **Affected.** Every build up to 4.1.0-rc.1: any user could create codes
    without limit.
  - The limit is read from the stored configuration on each request; a
    missing configuration or a `code_count` of `0` means 5, v2board's
    default.
  - Used codes, expired codes, other users' codes and public codes are not
    counted. Concurrent requests of one user are counted one after another.
  - At the limit the answer is v2board's: `500`
    `{"error":"The maximum number of creations has been reached"}`, and no
    code is created.
  - No other path creates a user's codes; an administrator's generation,
    if one is added, is not limited (`InviteService.GenerateInviteCode`).
- Only administrators create or change legacy forward rules.
  `POST /api/v2/user/forward/rules` let any user create a rule on any relay
  and exit node, to any target, with speed, traffic and expiry limits of
  their own choosing. A user now gets the panel error "only administrators
  can create or change legacy forward rules; forward through your tunnels
  instead", and nothing is stored. A legacy rule names its nodes, and no user
  entitlement covers them: a tunnel permission (`v2_forward_user_tunnel`)
  grants a tunnel, and a rule is neither counted in the permission's quotas
  nor paused when the permission ends. Users forward through the tunnels they
  are granted (`POST /api/v2/forward/create`);
  `GET /api/v2/user/forward/rules` still lists their rules, read-only. The
  administrator's `/api/v2/admin/forward/rules` routes are unchanged. See
  `docs/UPGRADE.md`.
  - **Affected.** Every build up to 4.1.0-rc.1 with forward nodes: any
    logged-in user could create such a rule.
- A user's forward targets must be public. `POST /api/v2/forward/create`
  and `POST /api/v2/forward/update` let a user point a forward at any
  target, and the tunnel's node connects to it: the node's loopback
  services, the private network behind it or its cloud metadata service.
  For a user, every target must now be a public address, or a name every
  address of which is public, with the classification the user's diagnosis
  already used: loopback, private (RFC 1918 and ULA), link-local,
  unspecified, multicast, carrier-grade NAT and other special-purpose
  addresses are refused, and so are `localhost` and its aliases, numeric
  IPv4 forms such as `127.1`, and names that do not resolve. The answer is
  the panel error `不能转发到内网或本机地址: <target>` (or
  `无法解析目标地址: <target>`), and nothing changes. Names are resolved on
  Control when the forward is written; DNS can answer differently later, on
  the node, so this is not complete protection against DNS rebinding.
  Administrators' forwards are not checked, and tunnels are administrator
  routes. A user's diagnosis now also refuses the loopback names and numeric
  forms without asking DNS. See `docs/UPGRADE.md`.
  - **Affected.** Every build up to 4.1.0-rc.1 whose users hold tunnel
    permissions.
- A clean agent's token is bound to its node.
  `POST /api/v2/forward-agent/register` set the agent's node to the body's
  `nodeId`, so the token of any clean agent could register under any node id
  and then claim that node's pending runtime jobs on its heartbeat, whose
  payloads carry the node's API token. A token is now bound to the node it
  was issued for or, for a token issued without one, to the node of its
  first registration that names one. A registration naming another node is
  `403` (`agent is bound to another node`) and changes nothing; one without
  `nodeId` keeps the binding. The binding uses the existing `node_id`
  column, so there is no migration. See `docs/UPGRADE.md`.
  - **Affected.** Every build up to 4.1.0-rc.1 with clean agents
    (`forward_runtime.backend: clean_agent`): the holder of any clean agent
    token could take another node's jobs and its API token.
  - Issuing a token now names its node. `POST /api/v2/admin/forward/agents`
    without a `nodeId`, with `0` or with an id that is not a forward node is
    refused with the panel error `nodeId is required: a clean agent token is
    issued for one forward node` or `forward node not found`, and issues
    nothing; the token is bound to the node at issue. Tokens issued earlier
    without a node keep working and bind on their first registration.
- **Node secrets are masked in administrator answers.** They no longer
  show in clear; they read `********`, the placeholder of system
  configuration and payment gateway secrets.
  - **Affected.** Every build up to 4.1.0-rc.1: administrator answers carried
    these secrets in clear, so anything that stored or relayed an answer
    held them.
  - **Node protocols.** The node list and detail,
    `GET /api/v2/admin/nodes/:id/protocols`, the answer of
    `POST /api/v2/admin/nodes/:id/protocols`, and a subscription group's
    protocols and the protocol pool
    (`GET /api/v2/admin/subscription/groups/:id/protocols`,
    `GET /api/v2/admin/subscription/protocols/available`) showed whole
    `v2_node_protocol` rows: Reality and TLS private keys, WireGuard server
    private keys, Shadowsocks server keys, Hysteria2 obfuscation and auth
    passwords, and tokens in a custom configuration. A setting whose name
    marks a secret (`*_key` except public keys and key file paths,
    `password`, `psk`, `auth`, `token`, `secret`, `credential`, `seed`) now
    reads `********` in `settings`, `tls_settings`, `transport_settings`,
    `reality_settings` and `custom_config`. Public keys and Reality's
    `short_id` are still shown. A node's raw configuration (`raw_config` in
    the node answers and `GET /api/v2/admin/nodes/:id/raw-config`) is masked
    the same way.
  - **Saving back.** A protocol update, a raw configuration update and a
    node update that send `********` keep the stored secret, and a new value
    replaces it, so the protocol editor keeps working. A new protocol has
    nothing stored, so `********` in it is stored empty.
  - **Registration keys.** `GET /api/v2/admin/auth-keys` showed every node
    registration key. Keys now read `********`. `POST /api/v2/admin/auth-keys`
    still answers the new key, once, and the node page's Auth Key dialog can
    now generate one.
  - **Forward node tokens.** `GET /api/v2/admin/forward/nodes[/:id]`, the
    answer of `PUT /api/v2/admin/forward/nodes/:id`, and the relay and exit
    nodes in the admin forward rule answers showed every node's `api_token`,
    which authenticates the node's agent. It now reads `********`.
    `POST /api/v2/admin/forward/nodes` answers it once, and the forward node
    page and the setup wizard show a generated token after creating the
    node. An update that sends an empty token or `********` keeps the stored
    token.
  - **Proxy node credentials.** A proxy node's `api_key` and `secret` were
    already left out of the node answers.
    `GET /api/v2/admin/nodes/:id/credentials` still answers them, for the
    deployment helper and Ansible, and the audit log now records every read
    of it, as action `reveal`.

  Nodes and agents are not affected: UniProxy, the gRPC node service and
  the forward agent routes read the stored values. None of these routes has
  a native package handler. See `docs/UPGRADE.md`.
- The SMTP password is no longer answered in clear to administrators.
  `GET /api/v2/admin/notification/email/config` (legacy and native) answered
  it as stored, and the system configuration list, single-key read and
  update answer showed it inside the `notification.email.config` value: the
  key's name does not mark it secret. The password now reads `********` when
  one is set (`""` when none is) in all of these answers; the rest of the
  value is shown as stored.
  - **Affected.** Every build up to 4.1.0-rc.1 with an SMTP password stored.
  - Saving keeps it: an e-mail configuration update with the placeholder (or,
    as before, a blank password) and a system configuration update whose
    value carries `"password":"********"` keep the stored password, and a
    new value replaces it. KernelSettings applies the same rule to a
    `mail` namespace write, and the notification package sends the
    placeholder when an update keeps the password.
  - The test e-mail still uses the stored password: the notification package
    keeps `kernel.settings.mail.secrets.v1`; only answers to administrators
    are masked.
  - The e-mail settings page starts the password field empty with
    "Password stored; leave blank to keep it". See `docs/UPGRADE.md`.
- The single-key system configuration answer no longer returns secrets.
  `GET /api/v2/admin/system/configs/:key` answered a sensitive value in
  clear, such as the NodeX token (`forward.runtime.nodex.token`), the SMTP
  password, or any key whose name marks a token, secret, password or key.
  It now masks it as the list already did: `value` and `display_value` read
  `********` when a value is stored (`""` when none is), with `sensitive`
  and `has_value`. `PUT /api/v2/admin/system/configs/:key` answers masked
  too, as before.
  - **Affected.** Every build up to 4.1.0-rc.1 with a sensitive system
    configuration value stored.
  - Saving `********` back keeps the stored value, as before; a new value
    replaces it. Saving `********` for a secret that is not stored is now
    refused (`value is required`) instead of storing the placeholder.
  - The NodeX page and the runtime workbench keep a stored token: the field
    shows `********`, saving keeps it, and the commands they show use
    `<FORWARD_API_TOKEN>` instead of the token. The forward setup wizard
    works as before. The routes stay bridged to the kernel; there is no
    native handler. See `docs/UPGRADE.md`.
- **The gost API connection test runs in the kernel.**
  `POST /api/v2/admin/forward/test-connection` (gost-mesh, native-flagged)
  dialled the gost API from the package host with the token the
  administrator typed. The route is now listed in
  `config/node-secret-fields.json` (target kind `dial`): the gateway seals
  the token into a handle, the package's native handler submits
  `TestForwardBackend` (`diagnose.forward_backend`, capability
  `kernel.nodeops.diagnose.v1`) with its request binding, and the kernel
  dials with its own gost client. The package never sees the token, the
  ledger holds neither token nor handle, and the answer is byte for byte
  the legacy one (`gostmeshcompat`, through the real gateway, sealer,
  bridge session and router). A request that is not an administrator's
  dials public addresses only; a shadow run submits nothing; a host
  without KernelNodeOps keeps the route legacy.
  - **Affected.** Every build up to 4.1.0-rc.1: the gost-mesh package host
    received the token in the request body, and in `native` mode dialled
    with it.
- **Forward runtime job payloads and NodeX requests carry no stored
  token.** See the NO-7 entry under Added: new job rows hold no node token,
  old rows are scrubbed at start, readers scrub meanwhile, and a forward
  node's token is presented only at its pinned endpoint.
  - **Affected.** Every build up to 4.1.0-rc.1 with the gost (NodeX)
    backend or a job backend (local Ansible, clean agent): the job table, the
    administrator's job list and a clean agent's claim carried the ingress
    node's API token.

### Changed

- **Extraction map at 4.1.0-rc.2.** Of the 292 `/api/v2` routes, 173 are
  `native-flagged`, 88 `bridged` and 31 `kernel-owned`
  (`config/package-extraction.json`). The counts quoted in the entries
  below are those at each change.
- **Extraction map: `kernel-owned` routes.** `config/package-extraction.json`
  has a fourth mode, `kernel-owned`, for routes that stay in the kernel by
  design; `bridged` now means only "not yet". Each `kernel-owned` row
  carries a one-line `reason`. Nothing changes at runtime: these routes are
  registered and relayed as before. 16 routes are `kernel-owned`, 112
  `bridged` and 164 `native-flagged`:
  - platform: the generic system configuration routes (no package gets a
    settings grant over every namespace) and backup creation, deletion and
    restore (archives on the kernel's disk);
  - machine-telemetry: the system information and the monitoring WebSocket;
  - protocol-runtime: the agent channel (registration, heartbeat, task poll,
    result, monitor and the WebSocket);
  - proxy-node: the node agent WebSocket.

  `check_plugin_only_routes.py` accepts a `reason` only on `kernel-owned`
  rows and now checks the package hosts too: a `bridged` or `kernel-owned`
  route must be in its host's `bridgedRoutes` and named nowhere else in its
  package, so it has no native handler, and a `native-flagged` or `native`
  route must not be in `bridgedRoutes`.
  `docs/architecture/package-extraction.md` has a refreshed status: route
  counts per package and mode, what unblocks each group of `bridged`
  routes, the kernel contracts (KernelIdentity, KernelSubscriber,
  KernelSettings, KernelOrder), the protected tables and the `kapi_*`
  views. Text that earlier merges duplicated in its section 3.4 is removed.
- **Extraction map: 15 node routes are `kernel-owned`.** The owner accepted
  decisions D3 and D4 of `docs/architecture/node-ops-service.md`, and the
  routes its section 6 keeps in the kernel are now `kernel-owned` in
  `config/package-extraction.json`, each with its reason. Nothing changes
  at runtime: they are registered and relayed as before. 31 routes are
  `kernel-owned`, 88 `bridged` and 173 `native-flagged`.
  - forward (11): the runtime status and doctor (the kernel's own
    executors, until the runtime moves to agents, A5); flow upload, report
    and snapshot (one kernel transaction, as the callers send no batch id);
    clean agent registration, heartbeat and report, and the agents' rule
    list (the agent channel, removed in 5.0).
  - proxy-node (4): the node credentials display (no contract call reveals
    a stored secret); node registration, heartbeat and runtime health (the
    node channel, removed in 5.0).

  The package hosts list them in `bridgedRoutes` as before, now commented as
  kernel-owned. `docs/architecture/package-extraction.md` updates its counts
  and drops the open-decision row from its section 3.2 blockers; 76 routes
  wait on KernelNodeOps.
- **The speed-limit routes moved to forward; four run natively.** The five
  `/api/v2/speed-limit/*` routes are Flux forward limits, whose rows name
  forward tunnels; they move from plan to forward (`forward.speed_limit.*`)
  with the same paths and answers.
  - Creation, the list, the deletion of a limit no permission names and
    the tunnels a limit may name have native handlers on forward's adopted
    `v2_speed_limit`, `v2_forward_tunnel` and `v2_forward_user_tunnel` and
    `kapi_forward_runtime_settings_v1`. A limit runs nothing until a
    permission names it, so none of them reaches a node.
  - The update stays bridged: it re-applies the forwards of every
    permission that names the limit on their nodes, which waits for
    KernelNodeOps.
  - `internal/tests/forwardcompat` proves byte parity and the same rows on
    SQLite and PostgreSQL (43 cases each).
  - **Transition.** Old plan releases still declare the routes under their
    `plan.speed_limit.*` ids. The kernel keeps those ids callable from plan
    (the moved-route operations now cover every previous owner, not only
    identity-platform) and prefers forward while both declare a route.
    Upgrade forward before plan. plan now has 7 routes, all native-flagged.
  - Extraction map: 170 `native-flagged`, 106 `bridged`, 16 `kernel-owned`.
- **The user's invite codes moved to affiliate and run natively.**
  `GET /api/v2/user/invite` and `POST /api/v2/user/invite/generate` move
  from identity-platform to the affiliate package
  (`affiliate.user.invite.get`, `affiliate.user.invite.generate.post`);
  paths and answers are unchanged.
  - affiliate adopts `v2_invite_code` (not a protected table: a code holds
    no credential of an existing account) and reads the new grant
    `kapi_order_billing_v1` for the paying users a user invited. The
    statistics, the commission total and the commission balance come from
    the tables and views the package already had.
  - **Generation lock.** For the cap under Security, the kernel counted a
    user's unused codes under a row lock on the user's `v2_user` row, which
    no package may take. The
    kernel and the package now both count and create under a PostgreSQL
    advisory lock keyed by the user (`pg_advisory_xact_lock`, class
    `0x696e7663`, the user id masked to 31 bits); advisory locks need no
    grant. SQLite still runs one writer at a time, and a generation that
    read before another's write is retried, on both sides.
  - `internal/tests/affiliatecompat` proves byte parity and the same codes
    and configuration on SQLite and PostgreSQL (19 cases each), and that
    the kernel's and the package's generations, run concurrently on one
    database, never pass the limit. A leased package role waits for the
    kernel's lock.
  - **Transition.** The kernel keeps the old `identity.*` ids callable
    (`internal/compat/v2/moved_routes.go`) and prefers affiliate while both
    packages declare the routes. Upgrade affiliate before identity-platform.
    identity-platform now has 22 routes, all native-flagged.
  - Extraction map: 166 `native-flagged`, 110 `bridged`, 16 `kernel-owned`.
- The Agent contract (`anix.agent.v1`) now lives in Control's SDK module,
  `github.com/AnixOps/anix-control/sdk` (plan step A0). Control no longer
  requires `github.com/AnixOps/anix-agent/sdk`, which is frozen at v1.1.0.
  - **Moved as of v1.1.0.** The proto, generated code and `PROTOCOL.md` are
    in `sdk/api/agent/v1` (Go package `agentv1pb`, generated by
    `sdk/api/agent/gen.sh`). The helpers are `sdk/agentcontrol` and
    `sdk/plugincontrol`.
  - **Wire compatible.** Only `option go_package` changed; the registered file
    name stays `api/grpc/agent/v1/agent.proto`. `agent_descriptor_test.go`
    compares the serialized descriptor, with `go_package` cleared, against
    v1.1.0's. The contract is in the protobuf compatibility golden and the
    generated-code drift check.
  - **Gates.** `check_agent_sdk_dependency.sh` now rejects any anix-agent
    module in Control or the SDK.
  - **SDK Sync.** The workflow reads anix-agent's `go.mod`. While anix-agent
    requires its own SDK, the workflow checks that SDK's descriptor against
    Control's. Once anix-agent requires `anix-control/sdk`, it builds and
    tests anix-agent against this checkout's SDK through a `go.work` replace.
  - **License.** The moved files were MPL-2.0 in anix-agent; their sole
    author relicensed them under this repository's MIT license.
- **Breaking for v2 API clients: the administrator's user list no longer
  shows subscription tokens.** `GET /api/v2/admin/users` answered every
  listed user's whole `v2_user` row, subscription token and proxy UUID
  included, with the plan's whole row. Each user in `data.list` now
  carries only the account, the subscription summary and its plan as
  `{id, name}`, as the order answers do.
  - **Removed fields:** `token`, `uuid`, `invite_user_id`, `telegram_id`,
    `discount`, `commission_type`, `commission_rate`, `remark_content`,
    `last_login_at` and `updated_at`.
  - **Slimmed:** `plan` is `{id, name}`; the rest of the plan row is gone.
    A user without a plan, or whose plan no longer exists, has no `plan`,
    as before.
  - **Kept:** `id`, `email`, `balance`, `commission_balance`,
    `device_limit`, `speed_limit`, `flowResetTime`, `transfer_enable`, `u`,
    `d`, `plan_id`, `group_id`, `expired_at`, `banned`, `is_admin`,
    `is_staff` and `created_at`.
  - `GET /api/v2/admin/users/:id` still answers one user's whole row, token
    included. The administrator's page now reads the token from it to copy
    a subscription link and fills the edit form from it (the remark is no
    longer in the list). `docs/UPGRADE.md` lists the change.
  - Users created in the same second keep a stable order
    (`created_at DESC, id DESC`); pages could repeat or skip them.
- **Breaking for v2 API clients: the order list and detail answers no
  longer embed the buyer's and the plan's rows.**
  `GET /api/v2/admin/orders`, `GET /api/v2/admin/orders/:id`,
  `GET /api/v2/user/order` and `GET /api/v2/user/order/:id` embedded the
  buyer's whole `v2_user` row, with its subscription token and proxy UUID,
  and the whole `v2_plan` row. Each order now carries its own fields, `plan`
  as `{id, name}` and, for administrators only, `user` as `{id, email}`; a
  plan or buyer that no longer exists is left out, as before.
  - The bundled frontend reads only those fields (`plan.name`, and
    `user.email` on the administrator's page) and needs no change; its order
    page tests now use the slim answers, an order without a plan or buyer
    included. `docs/UPGRADE.md` lists every field that disappears and where
    to read it instead.
  - The user's order list clamps `page_size` as the administrator's does
    (1 to 100, default 20 for 0 or less): a negative size listed every order
    and 0 none.
  - A user's order detail looks the order up by id and owner, and a request
    without a user names no one; another user's order stays "not found".
- The kernel's legacy subscription membership routes go through the same
  engine functions (`internal/subscriber`) as the contract:
  - `POST /api/v2/admin/subscription/users/:user_id/groups`;
  - `DELETE /api/v2/admin/subscription/users/:user_id/groups/:group_id`;
  - `DELETE /api/v2/admin/subscription/groups/:id`.

  They now append change-log rows, which they never did before. They also
  record their request ids (`subscription.grant:`, `subscription.revoke:`,
  `subscription.delete_group:`, derived from the request's
  `Idempotency-Key`, else its request id). A retry therefore applies once
  whichever side serves it, and a retried removal answers success rather
  than "用户订阅分组不存在".
- The kernel's backup and invite services reload the configuration they
  keep in memory when it changes in another handler instance or through
  KernelSettings. Before, a copy loaded once stayed until restart: for
  example, invite code expiry kept using the configuration it first read.
- The platform package reads the backup configuration through
  KernelSettings and no longer adopts `v2_backup_config`. It could read the
  S3 access and secret keys from the adopted row and write the row
  directly. `GET /api/v2/admin/system/backup/config` and the answer of its
  `PUT` now read namespace `backup` without its secrets, so the S3 keys
  reach the package masked, exactly as the kernel's handler shows them; the
  package's grants are `kernel.storage.adopt:v2_backup_record`,
  `kernel.view:kapi_system_audit_log_v1` and
  `kernel.settings.backup.read.v1`/`write.v1`, and on PostgreSQL its role
  loses its privileges on `v2_backup_config`. A backup read through
  KernelSettings now creates the default row when there is none, as the
  kernel's handler does, and answers the row's `id`, `created_at` and
  `updated_at` as read-only keys. A host without the contract keeps both
  routes legacy.
- A paid payment callback marks its order paid and completes it only while
  the order is pending. A payment for an order that was cancelled,
  completed, or paid by another payment is recorded, and the order is left
  unchanged; before, it was marked paid and completed again (its plan was
  already granted once per order). The kernel's callbacks now complete
  orders through `service.CompleteOrderPaymentTx`, which records each paid
  payment's outcome in `v4_kernel_subscriber_request` under
  `payment:<trade_no>`, and a repeat of a paid payment applies it to its
  order again, changing nothing unless the order was left pending. See
  `docs/UPGRADE.md`.

### Added

- **Design: node operations, the node credential split and Agent A2**
  (`docs/architecture/node-ops-service.md`), with a draft contract
  `sdk/api/kernelnodeops/v1` (`anixops.kernelnodeops.v1`). When it merged
  (#101) nothing changed in behaviour; the entries below implement it, and
  NO-1 made the contract binding.
  - **KernelNodeOps.** Packages would request typed, idempotent node
    operations: apply a forward, sync a node, check endpoints, run an agent
    diagnostic, issue a credential. The kernel holds the credentials and
    agent connections and answers receipts and results. The design covers
    the request-id ledger, polling and a watch stream, a capability per
    operation family, generation fencing, and sealed secret handles, so a
    package never sees a token or private key.
  - **Node credential split.** Credentials and secrets would move out of
    `v2_node`, `v2_authorized_key`, `v2_forward_node`,
    `v2_forward_clean_agent`, `v2_node_protocol` and `v2_wireguard_peer`
    into new protected tables, in the phases dual-write, backfill,
    dual-read and finalize, without altering any existing table.
    proxy-node, protocol-runtime and forward could then adopt the
    credential-free tables.
  - **Agent A2.** One mTLS Agent Control stream for configuration, users,
    traffic and logs, with agent certificates from the module PKI
    (`spiffe://anixops/<cluster>/agent/<node>`). The REST, WebSocket and
    v2board gRPC transports would stay for one major version.
  - **Route plan.** The 83 routes waiting on node operations and the 7
    waiting on the agent channel decision: 75 to go native and 15 to be
    marked kernel-owned. The document also gives the PR sequence (Control
    and anix-agent), the risks, the test strategy, and the decisions the
    owner must make.
  - **The draft contract.** It was unreleased and could change until the
    first kernel change that serves it (NO-1, next). It is in the proto
    golden file and in the CI generated-code check.
- **KernelNodeOps contract engine (NO-1)** (`internal/kernelnodeops`,
  `docs/architecture/node-ops-service.md` section 3). The kernel now serves
  `anixops.kernelnodeops.v1` on local bridge sessions and the mTLS module
  listener, and accepts its five capabilities
  `kernel.nodeops.{forward,nodeconfig,diagnose,agents,credentials}.v1`
  (official packages only, checked on every call against the host's
  generation). **The contract is binding**: it is no longer a draft and
  changes by additions only.
  - **Executors.** NO-1 executed no kind; NO-5 to NO-8 below add the
    executors. A kind without one answers `UNIMPLEMENTED` to
    `SubmitOperation` and records nothing, so a retry after an upgrade
    applies. `GetCapabilities` lists the kinds a kernel executes.
  - **The ledger.** `v4_kernel_node_operation` (unique `request_id`), its
    targets `v4_kernel_node_operation_target` and the event log
    `v4_kernel_node_operation_event`. These are new protected tables; no
    existing table changes. A repeat of a request id answers the first
    receipt; the same id with another operation, or from another package,
    is `FAILED_PRECONDITION`. A missing target is `NOT_FOUND` and records
    nothing.
  - **States.** Operations move pending, dispatching, running, then
    succeeded, failed, cancelled, timed out or superseded, and a terminal
    state never changes. A result that arrives after the deadline is kept as
    evidence. Operations are polled (`GetOperation`, `ListOperations`) or
    watched from a cursor (`WatchOperations`, `RESYNC` for a cursor the
    7-day log no longer holds). `CancelOperation` stops pending and running
    operations.
  - **Fencing and quotas.** A fenced generation is refused. The kernel runs
    one operation per resource at a time, and a newer level-triggered
    operation supersedes a pending one. Each kind takes only its node kinds.
    A package may have 256 operations that have not ended, and a node 32.
    Fan-outs count their children.
  - **No secrets.** Results, errors and evidence are scrubbed before they
    are stored: every credential the operation used, and every value at an
    `IsNodeSecretKey` key. Sealed handles are not part of the digest. A
    secret document with a secret in clear is refused.
  - **Administrators** list the ledger read-only at
    `GET /api/v4/kernel/node-operations` (filters, cursor paging).
  - Ended operations are kept 90 days and events 7 days; the kernel's
    singleton worker runs the dispatcher and prunes hourly.
  - Contract tests on SQLite and PostgreSQL, and SDK bridge contract tests
    over the local bridge and the module listener.
- **Node credential split, phase P1: dual-write** (the KernelNodeOps design,
  `docs/architecture/node-ops-service.md`, section 4; NO-2). Node
  credentials and protocol secrets are now also kept in new protected
  tables, as the first step towards packages adopting the credential-free
  node tables. No existing table is altered, and nothing reads the new
  tables until an operator moves a table to `dual_read` (phase P2, below).
  - New tables, created at start: `v4_kernel_node_credential` (node API
    keys and shared secrets, registration keys, forward node tokens with the
    `host:api_port` they are pinned to, clean agent tokens; one current
    version per credential, replaced versions retired with their value
    cleared), `v4_kernel_protocol_secret` (the secrets inside protocol
    settings and raw configurations by JSON pointer, WireGuard peer keys)
    and `v4_kernel_node_secret_split` (each table's phase, `dual_write`, and
    the backfill and verify outcomes). Values are in clear, like the legacy
    columns (decision D5). The tables are protected: `service.protectedTables`
    names them, so no manifest can adopt them.
  - `internal/nodesecrets` is their one writer. Every kernel writer of a
    moved column calls it in the transaction of its legacy write, and it
    derives the new rows from the legacy rows just written, so a failure
    leaves neither: node creation, update (raw configuration) and deletion,
    registration, registration key creation, use, deletion and the
    `NODE_DEFAULT_AUTH_KEY` seed, forward node creation, update and
    deletion, clean agent tokens and revocation, protocol creation, update
    and deletion, WireGuard peer creation, rotation (`wgrotate`) and
    deletion (with their protocol, node or user), and the XBoard import
    (`cmd/migrate`).
  - New commands `anix-control node-secrets backfill` (idempotent, in
    batches by id, resumable), `verify` (compares SHA-256 digests over every
    secret of both forms in one snapshot; exits 3 on a difference and names
    the differing secrets by subject or JSON pointer, never by value) and
    `status`. The phase stays `dual_write` until `phase` moves it; see
    `docs/UPGRADE.md`.
  - `service.IsNodeSecretKey` is now `nodesecrets.IsSecretKey`, so the
    administrator's masks and the split place secrets by one rule; a test
    proves the stored positions restore every masked document.
  - Tests on SQLite and PostgreSQL (`internal/nodesecrets`,
    `internal/tests/nodesecretsplit`, both on the CI PostgreSQL line):
    backfill then verify with equal digests, detected mismatches,
    dual-write from each writer, a failed split write rolling back the
    legacy write, tombstones against the unique indexes, idempotent
    re-runs, and the legacy readers authenticating every node afterwards.
- **Node credential split, phase P2: dual-read** (the KernelNodeOps
  design, `docs/architecture/node-ops-service.md`, section 4.3; NO-3).
  Every kernel reader of a moved column now reads through
  `internal/nodesecrets`, in its table's phase. Nothing changes until an
  operator moves a table, and the way back is one command.
  - **The readers.**
    - Node API key checks: `NodeAuth`, `NodeAPIKeyAuth`,
      `NodeAPIKeyHeaderAuth`, the gRPC interceptor, the Agent Control
      stream, and the agent WebSocket and HTTP routes.
    - The `SignatureAuth` shared secret, registration keys, forward node
      tokens (agent checks, the gost manager, NodeX payloads) and clean agent
      tokens.
    - Protocol secrets in `BuildNodeProtocolConfig` and the subscription
      renderer, raw configurations in UniProxy, WireGuard peer keys, and
      `GET /admin/nodes/:id/credentials`.
    - The agent enrollment bootstrap (a node API key or a forward token) and
      the forward node update's token-change check.
  - **Phases.** In `dual_write` the readers read the legacy columns as
    before and never the new tables. In `dual_read` they read the new
    tables.
    - A missing or differing row falls back to the legacy column. It counts
      `anixops_node_secrets_fallback_total{table,kind,reason}` on `/metrics`
      and logs once per subject, never a value.
    - JSON columns are rewritten only where the legacy document holds the
      placeholder, so answers stay byte for byte while both forms agree.
    - Each process caches the phases for 5 seconds.
  - **New command `anix-control node-secrets phase [-by <name>] <table|all>
    dual_read|dual_write`.**
    - `dual_read` needs the table's latest `verify` to have matched, with no
      mismatch and within the last hour; `dual_write` (rollback) is always
      allowed.
    - `all` moves every table or none.
    - Each change writes a `v2_operation_log` entry (module `node_secrets`)
      in the same transaction.
  - **Validate on build, report-only (decision D7).** Before a node's
    configuration is built from a protocol or a raw configuration, its
    secrets are checked: no placeholder or tombstone, a valid WireGuard
    server key pair, a 32-byte Reality private key, a Shadowsocks 2022
    server key of its cipher's length.
    - A failing row counts `anixops_node_secrets_invalid_total{table,type,
      reason}` and logs the node, protocol and field. It is still built.
    - New command `anix-control node-secrets validate` scans every row and
      exits 3 on a finding.
  - **Tombstone guard.** No reader accepts a tombstone (`!moved:<id>`) or
    the placeholder (`********`) as a node key, registration key, forward
    node token or clean agent token, in any phase. That includes the
    plain-key fallback for node rows without a hash, which would have
    accepted a finalized row's tombstone. A request signed with a
    placeholder secret is refused.
  - **Tests on SQLite and PostgreSQL.**
    - Every reader in each phase, through the real middleware, handlers and
      services. With only the new tables holding the secrets, every reader
      still works in `dual_read` and none in `dual_write`.
    - Fallback when a row is missing or differs, with the metric and one log
      line.
    - The phase gate, rollback and audit; validation reporting without
      excluding; the tombstone guard.
    - An older binary's legacy-only reads still authenticate every node
      after `dual_read`.
    - An agent enrolls in `dual_read` with a key whose legacy column is a
      tombstone; a missing new row falls back and counts.
- **Sealed secret handles (NO-4)** (`internal/sealedsecrets`,
  `docs/architecture/node-ops-service.md` section 3.7). A package host no
  longer reads a node secret an administrator types, or one shown once in
  an answer, on the routes that carry one.
  - **The field list.** New kernel-owned table
    `config/node-secret-fields.json`, embedded in the kernel binary. It
    lists 11 routes with their request and answer fields: node creation and
    update, raw configuration and its validation, protocol creation and
    update, forward node creation and update, registration keys and clean
    agents. The route gate checks every route id against
    `config/package-extraction.json`.
  - **Requests.** The v2 gateway replaces every secret in a listed route's
    body with an opaque handle (`anix-sealed:v1:` and 43 random base64url
    characters) before the host reads it:
    - each listed field;
    - every value under a key `IsNodeSecretKey` marks, inside listed
      documents, inline or as JSON strings, and anywhere else in the body.

    Member names match whatever their case or underscores. The placeholder
    `********` and empty values pass, so keep-on-save works. Every other
    byte stays as sent. The bridge capability keeps the original body, so
    the legacy handler, in every route mode, reads the request as sent.
  - **Handles.** Each handle is bound to its request (the package
    generation, request id and route), its target (the path parameter's
    resource, or the one the request creates, bound at first use) and its
    field. A handle is single-use and lives in kernel memory only until its
    request ends. Handles never appear in logs, metric labels, the
    KernelNodeOps ledger or error texts.
  - **KernelNodeOps request bindings are verified.** NO-1 accepted them
    unchecked. `SubmitOperation` now checks a binding on the session the
    call arrived on, against a live, unconsumed dispatch to the calling
    host; any other is `PERMISSION_DENIED`, and nothing is recorded.
    - Executors resolve handles with `Submission.Unseal`, and mint the
      handles of secrets they generate with `Run.Reveal`.
    - The ledger stores results without handles, and refuses a request id,
      reason or operation that holds one. Handles are answered only to the
      submitting call.
  - **Answers.** The gateway expands a handle only at a listed answer field,
    under its name, in the answer to the request it was minted for, once.
    Any other handle of a request in flight in an answer or its headers is
    refused (502 `sealed_secret_refused`).
  - **Fail closed.** A request that cannot be sealed is served by the
    kernel's legacy handler without the package host
    (`Supervisor.DispatchLegacy`), or refused with 503
    `sealed_secret_unavailable` when the route has none. That covers a body
    that is not JSON, a secret field that is not a string, a target that is
    not an id, a sealed body over the limit, or a missing field list. New
    metric `anixops_v2_gateway_sealed_secrets_total`.
  - **Shadow mode.** The SDK router gives native handlers the request
    binding (`NativeRequest.Binding`) and shadow runs none. The shadow and
    parity comparisons (`v2compat.EqualForCompare`, `packagecompat`) mask
    handles on both sides.
  - No route changes mode, and answers are byte-identical. Tests cover:
    - every listed route and field, the placeholder and empty values;
    - refusals of another request's, route's, target's or field's handle;
    - expansion in the bound answer only, the legacy fallback and the
      shadow comparison;
    - a walk of `IsNodeSecretKey` keys through every listed route, end to
      end over a real bridge session (`internal/tests/sealedhandles`).
- **Credential, secret and retirement operations (NO-5)**
  (`internal/kernelnodeops`, `docs/architecture/node-ops-service.md`
  section 3.11). The kernel executes the credentials family
  (`IssueCredential`, `RevokeCredential`, `IssueRegistrationKey`,
  `RevokeRegistrationKey`, `IssueCleanAgent`), the nodeconfig family's
  `RetireNode`, `RetireProtocol` and `PutSecretDocument`, and the
  `ValidateNodeConfig` RPC; `GetCapabilities` lists the eight kinds.
  - **Secrets in, secrets out.** A secret an administrator types reaches an
    executor as a sealed handle, resolved for the bound request only and
    kept in kernel memory until the request ends. A secret shown once (a
    node's key and secret, a forward node token, a registration key, a
    clean agent token, an `anixagt_` enrollment credential) is minted as a
    handle inside the transaction that stores it: when the answer cannot
    show it, nothing is issued. Issuing outside an administrator's request
    is refused. Results and the ledger never hold a value or a handle.
  - **One implementation.** The legacy node, protocol, raw configuration,
    registration key, clean agent and forward node deletion routes run the
    same functions (`internal/service/node_credential_ops.go`), with every
    `nodesecrets.Sync` and every agent certificate revocation in the same
    transaction as before. Their answers are pinned byte for byte
    (`TestNodeCredentialAnswers`) and unchanged.
  - **Cascades (D11).** `RetireNode` deletes a node's protocols with their
    WireGuard peers, subscription group links and secrets, its credentials
    and raw configuration secrets, and revokes its agent certificates, in
    one transaction, as the legacy deletion does; `RetireProtocol` likewise.
    A retirement of what is gone succeeds with nothing counted; a failure
    rolls the cascade back. The node or protocol row stays the package's.
  - **Validation.** `ValidateNodeConfig` runs the routes' validators; a
    handle or the placeholder at a secret position counts as present.
  - **Not revoked in place.** `RevokeCredential` revokes a clean agent's
    token or a node's agent enrollments; a node's key, secret or token is
    rotated (`replace`) or retired with the node.
  - `nodesecrets.Retire` removes the split rows of legacy rows their owner
    is about to delete; `nodesecrets.ReplacePositions` is exported.
- KernelNodeOps executes the node configuration and agent kinds
  (`docs/architecture/node-ops-service.md` sections 3.11 and 5.5, NO-6):
  - `SyncNode` (`node.sync`) rebuilds a node's desired configuration from
    its rows into the new protected table `v4_kernel_node_desired_config`
    (one row per node kind and id: a monotonic revision, the SHA-256 of
    the canonical document, the document), drops the kernel's node cache,
    and pushes `node.reload` to an agent on the Agent Control stream when
    the configuration changed or the sync is forced (a `config.v1` agent gets
    a `ConfigSnapshot` instead, A2-3 below); a node on the legacy
    transports keeps the stored configuration for its next pull
    (`LEGACY_PULL`). `NodeSyncResult` gains `config_revision` (field 8).
  - `AgentControlOperation` (`agent.operation`) sends `agent.ping`,
    `node.reload` or `users.reload` on the stream and ends with the agent's
    terminal state; `RunAgentDiagnostic` (`agent.diagnostic`) sends a
    whitelisted diagnostic task as an `agent.diagnostic` operation to an
    agent that advertises it, else on the node's WebSocket with the legacy
    fallback, and completes the task row from the agent's report.
  - `ListAgentSessions`, `GetAgentSession` and `GetAgentMonitor` answer
    the live sessions of both transports and both node kinds with the
    identity each authenticated by (SPIFFE ID or `api-key`), never a
    credential; what an agent reported is scrubbed.
  - `GetCapabilities` lists the three kinds. The dispatcher takes node
    kinds: `internal/grpc.AgentStreams` routes proxy and forward nodes to
    their own Agent Control managers (`internal/agentstreams`).
  - The legacy routes `POST /admin/nodes/:id/sync`,
    `POST /admin/nodes/:id/agent-control/operations`,
    `POST /admin/agent/tasks` and `POST /admin/agent/execute` run on the
    same functions and answer the same bytes; the sync route now writes
    the desired configuration row too.
  - `internal/tests/fakeagent`: a scripted Agent Control client against
    the real listener (Hello, acknowledgements, observed states, refusals,
    replays, a replaced session), and `internal/tests/nodeopsagent`, the
    executors' tests on SQLite and PostgreSQL.
- **KernelNodeOps forward operations (NO-7)** (`internal/kernelnodeops`,
  `docs/architecture/node-ops-service.md` sections 3.8, 3.11 and 6.1). The
  kernel executes the forward family's `ApplyForward`, `ApplyTunnel`,
  `SyncForwardBackend` and `ApplyLegacyRule`, and `GetCapabilities` lists
  them. No forward route switches to native; M3-4 and M3-5 do that.
  - **One implementation.** The executors run the legacy routes' code over
    the executors the kernel has: NodeX for the gost backend and the legacy
    rules, the local Ansible job executor and the clean agent job queue.
    The legacy routes call the same functions
    (`PanelForwardService.ApplyForwardRuntime`,
    `ForwardRuleService.ApplyRuntime`), and their answers and what they
    send NodeX are unchanged byte for byte (`TestForwardOperationsAnswers`,
    written before the move).
  - **Operations name resources.** `forward.apply` reads the forward and
    its tunnel when it runs, applies the action through the backend in
    force for the forward and records the forward's runtime columns; the
    status column stays the package's. On the gost backend NodeX answers
    in the operation; on a job backend the operation is accepted when the
    job is queued (its id in the receipt) and ends with the job.
    `FORCE_DELETE` succeeds whatever the node answered; a forward gone
    since the submission is `TARGET_GONE`.
  - **Fan-outs.** `forward.tunnel` creates one `forward.apply UPDATE` per
    active forward of the tunnel. `forward.sync_backend` takes its target
    from the new `SyncForwardBackend.backend` field (the backend in force
    when empty), records the target on each forward and creates one
    `forward.apply SYNC` per forward.
  - **Legacy rules.** `forward.legacy_rule` pushes the row through NodeX
    with both nodes' tokens resolved at send time.
  - **Job payloads without tokens.** A `v2_forward_runtime_job` payload no
    longer holds the ingress node's token: the kernel resolves it when it
    sends a request to NodeX. A start-up pass in the singleton worker
    process scrubs the rows written before this release, and every reader
    (a clean agent's claim, the administrator's job list) serves old rows
    scrubbed meanwhile. See `docs/UPGRADE.md`.
  - **Endpoint pinning (D12).** A forward node's token is presented only at
    the endpoint it is pinned to, `host:api_port` as the kernel's own
    forward node writers recorded it; any other address, and a node
    without an API port, is `ENDPOINT_UNCONFIRMED`, with nothing sent. The
    administrator's forward node update moves the pin. A node not yet
    backfilled is unpinned and counted in
    `anixops_node_secrets_pin_total{reason}`. `node-secrets status` now
    prints its tables under `tables` and, under
    `forward_nodes_without_api_port`, the forward nodes whose token is
    pinned to no endpoint, by id and name with their count.
  - **Contract additions.** `SyncForwardBackend.backend`,
    `TestForwardBackend` (operation 35) and `ForwardBackendTestResult`
    (result 34). The proto golden file grows by 9 elements.
  - Tests on SQLite and PostgreSQL: each executor against a fake NodeX, a
    fake `ansible-playbook`, and a clean agent claiming and reporting jobs
    (success, failure, timeout, cancellation, a fenced generation,
    supersession on one forward), the fan-outs, the pin rule, the payload
    scrub, a secret walk and a bridge contract round trip.
- **KernelNodeOps diagnoses (NO-8)** (`internal/kernelnodeops`,
  `docs/architecture/node-ops-service.md` section 6). The kernel executes the
  diagnose family's `CheckEndpoints`, `CollectNodeStats`, `DiagnoseForward`
  and `DiagnoseTunnel`, and `GetCapabilities` lists them. No route switches
  to native; M3 does that.
  - **One implementation.** The executors run the legacy routes' code from
    Control (`internal/service/forward_diagnosis.go`): the forward node and
    Ansible machine checks, the node statistics (a gost node's metrics, an
    Ansible machine's counters), and the forward and tunnel diagnoses. The
    routes' answers are unchanged byte for byte
    (`TestForwardDiagnosisAnswers`, written before the move). The legacy
    forward and tunnel diagnoses now probe their targets concurrently, at
    most 8 at a time, and stop when the request ends.
  - **Results.** Typed and scrubbed: the nodes' tokens and every value at a
    secret key are masked. Failed probes are part of the result, not a
    failed operation. A node, forward or tunnel deleted since the submission
    is `TARGET_GONE`. Deadlines and cancellation stop the dials, and a
    stopped endpoint check records nothing.
  - **No private targets for a package.** `DiagnoseForward` checks every
    target as a user's: public addresses only, as the #84 guard does. When
    NO-8 merged the kernel could not tell an administrator's request from a
    user's; NO-4 now verifies request bindings, but `DiagnoseForward` still
    checks every target as a user's. The forward connection test (under
    Security) is the operation that tells an administrator's request apart.
  - **Vantage (D10).** Control by default, the node when the agent of every
    node concerned advertises `diag.v1`. A2-5 records `diag.v1` but no
    diagnostic is sent to agents yet, so the kernel still dials from Control
    and says so in the result.
  - **Contract additions.** `VantageReport` (in `EndpointCheck` and
    `DiagnosisResult`), `ServiceTraffic`, and `NodeStatsResult.services` and
    `current_connections`. The proto golden file grows by 11 elements.
  - Tests on SQLite and PostgreSQL: each executor against a fake network and
    a fake gost metrics endpoint (success, partial failure, timeout,
    cancellation, the private-target refusals, a secret walk), and a bridge
    contract round trip.
- **Agent stream data-plane contract** (`sdk/api/agent/v1/PROTOCOL.md`, "Data
  plane"). `anix.agent.v1` gains the payloads that will carry each node's
  configuration, users and reports on the Agent Control stream instead of
  UniProxy, the v2board gRPC services and the WebSocket. This entry is the
  contract (A2-2); the kernel sends and accepts the payloads from A2-3,
  A2-4 and A2-5, below.
  - Control → Agent: `ControlToAgent.config` (`ConfigSnapshot`), `users`
    (`UserDelta` of `NodeUser`) and `report_ack` (`ReportAck`), fields 13 to
    15. Agent → Control: `AgentToControl.config_status` (`ConfigStatus`),
    `traffic` (`TrafficReport` of `UserTraffic` and `OnlineUser`), `logs`
    (`LogBatch` of `LogEntry`) and `status` (`NodeStatus`), fields 14 to 17.
    `Hello` gains `config_revision` (6) and `users_cursor` (7), and
    `HelloAck` gains `server_capabilities` (5).
  - Negotiation: each side sends a payload only when the other advertised
    its capability, `config.v1`, `users.v1` or `reports.v1` (name `config`,
    `users` or `reports`, version `v1`). `sdk/agentcontrol` names them and
    adds `Negotiated`.
  - At A2-2 the kernel advertised no `server_capabilities`; A2-3 to A2-5
    advertise `config.v1`, `users.v1` and `reports.v1`. An Agent that sends
    `config_status`, `traffic`, `logs` or `status` without the capability
    negotiated gets `InvalidArgument`, naming the capability, and the stream
    ends, as with a kernel built before these payloads existed. Agents built
    against the v1.1.0 SDK see no change.
  - Additive only: `agent_descriptor_test.go` now requires the descriptor to
    be a superset of v1.1.0's (every message, field, enum value and method
    unchanged) instead of equal to it apart from `go_package`, and tests
    that a removed, renamed or renumbered field fails. The proto golden file
    grows by 55 elements. The manual SDK Sync workflow runs the renamed
    `TestDescriptorExtendsExternalAgentSDK`.
- **Agent PKI: mTLS client certificates for AnixOps Agents (A2-1)**
  (`docs/architecture/module-runtime.md`, "Agent PKI"). The module CA now
  also signs agent certificates whose one URI SAN names the node,
  `spiffe://anixops/<cluster>/agent/proxy-<id>` or `.../forward-<id>`. They
  last 7 days and renew at two thirds; the kernel ignores the CSR's subject.
  - New service **`anix.agent.v1.AgentEnrollment`** (`Enroll`, `Renew`,
    `GetTrustBundle`) in its own file, `agent_enrollment.proto`, so
    `agent.proto`'s descriptor stays the v1.1.0 one. It is served on the
    agent listener (port 50051). SDK helpers for agent identities and the
    metadata keys are in `sdk/agentcontrol`.
  - Bootstraps: the node credential an agent already has (`x-node-id` with
    `x-api-key`, or a forward node's token with `x-node-kind: forward`), a
    node bound by a registration key the same way, or a one-time
    `anixagt_...` enrollment credential (at most 7 days, stored hashed) from
    `POST /api/v4/kernel/agents/enrollment-tokens` or
    `anix-control agent token create -node proxy-12`. Each credential issue
    and each enrollment is written to the operation log.
  - The listener verifies a client certificate when one is presented and
    takes the node from it, for proxy and forward nodes; forward nodes can
    now open the control stream. Envelope `node_id`s and v2board request
    `node_id`s must name the certificate's node.
  - New key `agent_control.mtls`: `optional` (default; legacy agents work
    unchanged), `preferred` (a legacy control stream is answered with
    `x-anix-auth-deprecated`) or `required` (certificates only on the Agent
    services).
  - Agent enrollment needs only the built-in CA: `module_runtime.ca_kek`
    with `pki: builtin`. `module_runtime.enabled` and the `:7443` module
    listener are not needed; the kernel now creates the CA, and runs its
    rotation maintenance, whenever `ca_kek` is set. With `pki: external`
    agent enrollment is off. See `docs/UPGRADE.md`.
  - New tables `v4_kernel_agent_enrollment` and
    `v4_kernel_agent_certificate`, protected from package adoption.
    The streams these certificates open carry the data plane of A2-3 to A2-5,
    below.
    Disabling or deleting a node and replacing a forward node's token revoke
    its certificates; the listener refuses revoked serials through a cache
    of at most 30 s, and an open stream ends at its next heartbeat.
- **Configuration push on the Agent Control stream (A2-3, `config.v1`)**
  (`sdk/api/agent/v1/PROTOCOL.md`, "Data plane";
  `docs/architecture/node-ops-service.md` section 5.5). When an agent's
  `Hello` lists `config.v1`, the kernel lists it in
  `HelloAck.server_capabilities`, for proxy and forward nodes, and sends the
  node's desired configuration (`v4_kernel_node_desired_config`) as a
  `ConfigSnapshot`: after the `HelloAck` when `Hello.config_revision` is not
  the desired revision (none, older, or from another database); when
  `node.sync` stores or forces a configuration, in place of `node.reload`;
  and when a rebuild from the node's rows, once a minute per session, moves
  the revision. A session is never sent a revision older than one it was
  sent. Agents without `config.v1`, and every agent in the field, get
  nothing new and keep receiving `node.reload`.
  - `ConfigStatus` from the agent is accepted once `config.v1` is
    negotiated (before, and without it, it is still `InvalidArgument`) and
    recorded in the new protected table `v4_kernel_node_config_status`: the
    last report with the kernel's verdict (`applied`, `failed`, `stale` for
    an older revision, `mismatch` for another hash), and the applied
    revision and hash, which only a status naming the desired revision and
    hash moves.
  - `node.sync` on a `config.v1` agent ends on the agent's `ConfigStatus`
    for the pushed revision and hash: `SUCCEEDED` when applied, `FAILED`
    (`BACKEND_FAILED`) with the agent's error otherwise; a stale or
    mismatched status does not end it. The operation runs at the
    configuration revision (`node_revision`), and the result's `ack` is the
    status. An agent that reconnects meanwhile is sent the snapshot again
    and its answer on the new session ends the operation. A sync that is not
    forced also pushes when the agent has not applied the stored
    configuration. `POST /admin/nodes/:id/sync` answers such an agent with
    the snapshot's `config_revision` and `config_hash`.
  - The snapshot carries what the legacy pulls give the node and no other
    secret. The proxy document gains `legacy_pull`: the UniProxy answer
    for no node type and for each type the node serves, built by
    `service.BuildUniProxyNodeConfig`, which the UniProxy handler now calls;
    each protocol's `config` is the v2board `GetConfig` source. The
    document's `raw_config` is read through the node credential split, as
    UniProxy reads it. The new revision lands once, at the next rebuild.
  - `/metrics` adds `anixops_agent_config_snapshots_sent_total{trigger}`
    (`hello`, `sync`, `refresh`),
    `anixops_agent_config_statuses_total{result}` and
    `anixops_agent_config_lagging_nodes`.
  - Tests: `internal/tests/nodeopsagent` on SQLite and PostgreSQL
    (negotiation, the `Hello` reconcile, `node.sync` ending on
    `ConfigStatus`, a reconnect mid-operation, parity with UniProxy and
    v2board `GetConfig`, a walk of every secret-named key), and the listener
    in `internal/grpc`.
- User deltas on the Agent Control stream (A2-4, `users.v1`). Control now
  lists `users.v1` in `HelloAck.server_capabilities` for proxy nodes and,
  to an Agent whose `Hello` lists it too, sends `UserDelta` payloads from
  the subscriber change log (`v4_kernel_subscriber_change`): on connect the
  changes after `Hello.users_cursor`, or a paged full resync when the
  cursor is 0, ahead of the log or older than its 7-day retention (also
  when rows are pruned mid-session); then a delta per batch of changes as
  the log advances, bounded to 500 users or changes a message. A node gets
  the same users as from the legacy pulls (UniProxy `user`, v2board
  `GetUsers`): the three now share `service.ActiveUsersForNodeQuery`. A
  `NodeUser` carries the id, uuid, limits and WireGuard peer fields only,
  never the e-mail, password hash or subscription token. Agents without
  `users.v1` get nothing new. `/metrics` adds
  `anixops_agent_user_deltas_sent_total{kind}`,
  `anixops_agent_user_resyncs_total{reason}`,
  `anixops_agent_users_sessions` and `anixops_agent_users_cursor_lag`.
  See `sdk/api/agent/v1/PROTOCOL.md`, "Data plane".
- **Reports on the Agent Control stream (A2-5)** (`sdk/api/agent/v1/PROTOCOL.md`,
  "Reports"). When an agent's `Hello` lists `reports.v1`, the kernel
  advertises it back in `HelloAck.server_capabilities` and accepts
  `TrafficReport`, `LogBatch` and `NodeStatus` on the stream, for the
  stream's node (a proxy node; forward-node reports join with A5). Agents
  without `reports.v1`, and every agent in the field, keep the legacy paths
  and today's `InvalidArgument` for the payloads.
  - Traffic is counted through the transaction the legacy `ReportTraffic`
    and the UniProxy push use (`v2_server_log`, the node's counters, the
    server stats and the subscriber ledger), with the node's rate, so a byte
    counts once whichever path carried it. Online IPs replace the node's
    alive set, the one `ReportOnline` feeds, and `online_users`. Logs go
    into `v2_node_log` as `ReportLogs` records them, with the runtime health
    a WireGuard entry carries. A `NodeStatus` writes the heartbeat, system
    and runtime-health columns `ReportStatus` writes.
  - A `TrafficReport` or `LogBatch` is applied at most once per node and
    batch id. The new table `v4_kernel_agent_report_batch` (protected from
    package adoption, pruned after 7 days) is claimed in the transaction that
    applies the batch. `ReportAck` answers each: `applied: true` when this
    delivery recorded it; `applied: false` without an error for a batch a
    committed delivery recorded before, which is not counted again;
    `applied: false` with an error for a batch refused for good (no batch
    id, or one over 128 bytes; a missing `user_id`; bytes beyond the
    counter range; `fields_json` that is not JSON; a node that no longer
    exists). A batch the kernel cannot record for now (the database failed)
    gets no acknowledgement and the stream stays open, so the agent's spool
    resends it. `NodeStatus` is never acknowledged.
  - `diag.v1` in the `Hello` is recorded on the session
    (`AgentControlSnapshot.diagnostics`, with `server_capabilities`), for the
    diagnosis vantage of NO-8. No diagnostic is sent yet.
- **KernelSettings contract** (`sdk/api/kernelsettings/v1`,
  `docs/architecture/settings-service.md`). Official packages read and write
  system settings per namespace instead of the protected `v2_system_config`
  and the backup configuration row. It is served on local bridge sessions
  and the module listener.
  - The kernel maps each key to at most one namespace: `mail`
    (`notification.email.*`), `invite` (`invite.*`), `nodex`
    (`forward.runtime.nodex.*`), `forward-runtime` (the runtime backend and
    Ansible keys) and `backup` (the backup configuration fields). No call
    reaches any other key.
  - Each namespace has `kernel.settings.<namespace>.read.v1`, `.write.v1`
    and `.secrets.v1` capabilities, for official packages only. Without
    `secrets`, a secret reads as `********`. The `notification.email.config`
    value holds the SMTP password, so it counts as a secret. A write that
    sends the placeholder, or asks to keep a value, keeps the stored secret.
  - Writes run in the kernel, in one transaction with the same audit
    entries the legacy handlers record and with the new request ledger
    `v4_kernel_settings_request` (a new table, kept 90 days). Before the
    call returns, the kernel reloads the backup and invite configuration it
    keeps in memory.
- **Seven more routes native** (151 of 292 now `native-flagged`), over
  KernelSettings. Byte parity holds on SQLite and PostgreSQL against the
  real server in process; the tests also compare the settings rows, the
  audit rows and the kernel's in-memory copies:
  - notification: the e-mail configuration GET and PUT, and the test send
    (15 cases against a test SMTP server). The GET answered the SMTP
    password, as before; the masking under Security now reads it as
    `********` on both sides.
  - affiliate: the invite configuration update (33 cases). All 8 affiliate
    routes are now native.
  - gost-mesh: the NodeX runtime status and diagnosis (21 cases each,
    against test NodeX servers). All 3 gost-mesh routes are now native. The
    NodeX probe now leaves from the package host.
  - platform: the backup configuration update (14 cases).

  The generic system configuration routes (list, get, put and delete) stay
  bridged. They reach every key, and a grant over every key would hold
  every secret. See `settings-service.md`.
- **Subscription group membership in `KernelSubscriber`.** The contract
  gains `GrantSubscriptionGroup`, `RevokeSubscriptionGroup` and
  `RemoveSubscriptionGroupMembers`, under the new capability
  `kernel.subscriber.groups.v1`
  (`docs/architecture/subscriber-service.md`). Only calls and messages are
  added, so v4.0.0 hosts are unaffected; the proto golden file grows.
  - A grant creates a user's `v2_user_subscription_group` row, or sets the
    given expiry, traffic and renewal price of an existing one. A
    revocation deletes one row, and `RemoveSubscriptionGroupMembers` every
    row of a group.
  - Every call is idempotent by request id through
    `v4_kernel_subscriber_request`. A missing subscriber, group or
    membership is `NotFound` and records nothing.
  - A change that alters which groups an active subscriber holds, or until
    when, appends a change-log row, so `WatchSubscriberChanges` streams the
    new `subscription_group_ids`. A change to an inactive subscriber, or to
    a membership's traffic or renewal price only, appends nothing.
- **Subscription module: membership routes.** `packages/subscription` now
  serves 20 of its 25 routes natively and declares
  `kernel.subscriber.groups.v1`. Through `KernelSubscriber`, it:
  - grants a user a group;
  - takes it away;
  - deletes a group: its members first, then its templates, plan links and
    node protocol links with the group.

  `internal/tests/subscriptioncompat` runs the real `KernelSubscriber`
  server in process. It proves byte parity, the same memberships, request
  ledger and change log on SQLite and PostgreSQL (173 cases each). 147 of
  292 routes are now `native-flagged`.
- **Order completion contract `KernelOrder`.** A new service,
  `KernelOrder.CompleteOrderPayment`, under the new capability
  `kernel.order.complete.v1` (`docs/architecture/order-service.md`). Only a
  new proto package (`anixops.kernelorder.v1`) is added, so v4.0.0 hosts
  are unaffected; the proto golden file grows.
  - The caller names a paid payment record. In one transaction the kernel
    re-checks that it pays its order (the record's user, a pending order,
    the amount covering its total), marks the order paid and completes it
    with request id `order:<id>`, the payment callbacks' id.
  - It is idempotent per trade number (`payment:<trade_no>` in the request
    ledger). A record that is not paid or names another order is refused
    and nothing is recorded.
  - It is served on local package bridge sessions and the module listener,
    to official packages only, authorized on every call.
- **Payment module: the provider callbacks.** `packages/payment` now serves
  all 20 of its routes natively and declares `kernel.order.complete.v1`:
  `POST /api/v2/payment/callback/:type` (EPay), the x402 callback, and the
  Stripe and PayPal webhooks. Without a bridge connection they stay legacy.
  - The provider signature checks are ported as is, with the gateways'
    secrets from the adopted `v2_payment_gateway`. The PayPal webhook calls
    PayPal's API from the payment host.
  - A paid callback records the payment and the gateway statistics in the
    module's tables, commits, then asks `KernelOrder` to complete the
    order. A repeat of a paid payment asks again, so a failure between the
    steps converges on the provider's next delivery, and no order is paid
    without a paid payment. If the kernel cannot be reached the callback
    fails with 502, so the provider delivers it again.
  - `internal/tests/paymentcompat` runs the real `KernelOrder` server in
    process. It proves byte parity and the same payment records, gateway
    statistics, orders, subscribers, request ledger and change log on SQLite
    and PostgreSQL (78 callback cases each), with PayPal's API faked on
    both sides. 151 of 292 routes are now `native-flagged`.
- **Order module: native order lists and details.** `packages/order` now
  serves all 13 of its routes natively (148 of 292 v2 routes are
  `native-flagged`). The administrator's and user's order lists and details
  (`order.admin.orders.get`, `order.admin.orders.id.get`,
  `order.user.order.get`, `order.user.order.id.get`) were bridged because
  their answers embedded the buyer's `v2_user` row; with the slim answers
  they read only kernel views.
  - The new kernel view `kapi_plan_name_v1` shows a plan's `id` and `name`
    and nothing else of `v2_plan`; the package declares
    `kernel.view:kapi_plan_name_v1`. The buyer's e-mail comes from
    `kapi_user_directory_v1`.
  - A user's list and detail are the caller's own orders, with the owner in
    the query.
  - `internal/tests/ordercompat` proves byte parity on SQLite and PostgreSQL
    for 55 more cases per backend: every list filter and paging edge, other
    users' orders, deleted plans and buyers, and invalid ids. A mutation
    check (owner filter, page clamp, ordering, e-mail match, plan and buyer
    names) fails the parity tests. The seeded payment time is now taken once,
    so both sides of a case see the same answer.
- **Identity module: the administrator's user directory.** identity-platform
  serves the user list and statistics natively (`identity.admin.users.get`,
  `identity.admin.users.stats.get`; 153 of 292 routes are now
  `native-flagged`).
  - One query on the package's own storage joins identity's accounts with
    Control's views `kapi_user_directory_v1`,
    `kapi_subscriber_entitlement_v1` and `kapi_plan_name_v1`
    (`native.UserDirectory`). It filters by e-mail, plan and status,
    orders, pages and counts; "active" is not banned in identity and not
    expired in Control. The total and the page are read in one
    repeatable-read transaction. The package now declares
    `kernel.view:kapi_subscriber_entitlement_v1` and
    `kernel.view:kapi_plan_name_v1`.
  - Both routes read identity's accounts, so they join
    `service.IdentityAccountReadRoutes`: they leave legacy mode only while
    identity is authoritative, and a rollback returns them to legacy.
  - `internal/tests/identitycompat` proves byte parity on SQLite and
    PostgreSQL (47 more cases per backend, 164 in all), and that the
    answers follow identity's account rather than Control's projection. A
    PostgreSQL test runs the search as the package's own role. A mutation
    check of the native search (filters, statuses, ordering, paging, page
    sizes, account source, plan names, counts) fails the tests.
  - The design and the rejected alternatives (a kernel-side search over the
    projection, a two-phase query) are in
    `docs/architecture/identity-service.md`.
- **The administrator dashboard and the user's subscription summary run
  natively, from the kernel's caches** (`docs/architecture/kernel-caches.md`).
  The kernel answers both routes from a cache in its memory, with the time
  it built the answer (`cached_at`), so they had stayed bridged. The kernel
  now keeps both caches and modules read them through typed contract
  methods, so legacy and native answers are the same entry. No invalidation
  event is needed, because no module holds a copy.
  - New contract **KernelTelemetry** (`sdk/api/kerneltelemetry/v1`,
    `anixops.kerneltelemetry.v1`). `GetDashboard` answers the dashboard
    snapshot the kernel caches for 60 seconds; `refresh` rebuilds it. The
    online users cross only as a count. Capability
    `kernel.telemetry.dashboard.v1`, held by machine-telemetry.
  - New method **`KernelSubscriber.GetSubscriptionSummary`**, family
    `kernel.subscriber.summary.v1`, held by subscription. It answers one
    subscriber's summary, which the kernel caches for 30 seconds, with the
    subscription link settings, and no token or UUID.
  - Both are served on local bridge sessions and on the module listener,
    for official packages only, and authorized on every call. They add only
    new methods, messages and fields: the proto golden file grows, and
    `kerneltelemetry` joins the CI generated-code check.
  - `GET /api/v2/admin/dashboard` (machine-telemetry) and
    `GET /api/v2/user/subscription` (subscription) are `native-flagged`,
    which makes 166 of 292. A host without a bridge connection keeps them
    legacy.
  - Parity runs on SQLite and PostgreSQL against the real kernel servers:
    15 dashboard cases and 21 summary cases. They compare the answers byte
    for byte, including a cached entry's `cached_at`, and the cache entry
    each side leaves.
- **Package hosts receive the request's scheme and host; the Telegram
  webhook is set natively.**
  - The kernel sends the original request's scheme (`https` when the
    connection was TLS, else `http`) and host (the `Host` header, only when
    it is a plain `host[:port]`) to package hosts as the new
    `DispatchRequest` and `WebSocketOpen` fields `request_scheme` and
    `request_host`; the SDK shows them as `RequestMetadata.Scheme` and
    `Host`. These are the values the kernel's bridge already gave the legacy
    handlers; no forwarding header is trusted beyond what those handlers
    read. They are protobuf fields, not request metadata JSON keys: hosts
    built with the v4.0.0 SDK decode that JSON strictly and skip unknown
    protobuf fields, so they keep working and no metadata version is needed.
    The SDK refuses a scheme other than `http` or `https` and a host that is
    not an authority.
  - `pluginhostsdk.ErrNativeUnavailable`: a native handler that cannot
    answer a request as the legacy handler would (for example, an older
    kernel sent no request address) returns it, and the router answers from
    the legacy handler; in shadow mode the comparison is skipped.
  - `POST /api/v2/admin/telegram/webhook` runs natively in the notification
    package: without a `url` it points the webhook at the request's
    `/api/v2/telegram/webhook`, with `X-Forwarded-Proto` honoured as the
    kernel's handler does. `internal/tests/notificationcompat` proves byte
    parity, the same bot row and the same Bot API calls on SQLite and
    PostgreSQL (16 cases each). The parity harness sends each case's host
    and TLS state to both sides.
  - `GET /api/v2/forward-agent/install.sh` stays bridged: its panel URL is
    Control's `forward_runtime.clean_agent.public_url` when set, process
    configuration no package can read, and only otherwise the request's
    scheme and host.
  - Extraction map: 171 `native-flagged`, 105 `bridged`, 16 `kernel-owned`.

### Fixed

- The Windows server binaries build again (#119). The sealed secret handles
  (#112) used `pluginhost.ErrLegacyUnavailable`, which only the Unix build
  defined; the non-Unix stub now defines it too.
- The legacy user dashboard no longer shows the subscription link settings.
  `GET /api/v2/user/subscription` wrote `subscribe_path` and
  `subscribe_domains` into the user's cached summary, and
  `GET /api/v2/user/dashboard` answers that same entry. For up to 30
  seconds after a summary read, the dashboard therefore showed them, and
  concurrent requests wrote the entry while others encoded it. The summary
  now answers a copy. identity's native dashboard never showed them.
- Network modules can change subscription group memberships. The module
  listener did not forward `KernelSubscriber.GrantSubscriptionGroup`,
  `RevokeSubscriptionGroup` or `RemoveSubscriptionGroupMembers`. A
  subscription module running as a network module got `Unimplemented` for
  the native membership routes, where a local host was served. A test now
  checks that the listener forwards every method of every contract.
- E-mail and invite configuration writes record an audit entry. `PUT
  /api/v2/admin/notification/email/config` and `PUT
  /api/v2/admin/invite/config` (legacy and native) wrote
  `v2_system_config` without a `v2_operation_log` entry, unlike every other
  system write. They now record the system configuration entry for the key
  they write (`notification.email.config`, `invite.frontend.config`):
  module `system`, `create` or `update`, target `system_config`, and
  content naming the key, its group and type, whether it has a value and
  whether the stored SMTP password was kept, and, for the e-mail
  configuration, `"masked_fields":["password"]` with
  `masked_fields_with_value` saying whether a password is set; never a
  value. The legacy handlers and KernelSettings (namespaces `mail` and
  `invite`) build it with the same function, so both write the same row.
- Audit entries written through the package bridge name the user. A legacy
  system handler the bridge relays to gets the actor's id only, so its
  `v2_operation_log` rows (system and backup configuration, backups, and
  now the e-mail and invite configuration) and the KernelSettings rows had
  an empty username. Both now record the user's e-mail, looked up by id.
- The administrator's order list filtered by `email` always failed: the
  joined `v2_user` made `created_at` ambiguous in the ordering. The filter is
  now a subquery, and orders created in the same second keep a stable order
  (`created_at DESC, id DESC`).
- **A paid payment no longer leaves its order pending.** The payment module
  records a paid callback, then asks `KernelOrder` to complete the order. If
  that second step failed and the provider never delivered the callback
  again, the payment stayed paid and the order pending until an
  administrator stepped in. A kernel worker now runs the same step
  (`CompleteOrderPaymentTx`, request id `payment:<trade_no>`) for such
  records (`docs/architecture/order-service.md`).
  - It runs at start and every five minutes in every Control process. It
    reads paid records whose order is pending and which have no outcome yet,
    paid more than two minutes ago (so it does not race the live callback)
    and within the request ledger's 90 days, in batches of 100 and at most
    10 batches a run.
  - The checks are the callback's: the record's user, a pending order, the
    amount covering its total. A record that fails them is refused, logged
    and recorded, and the order is left unchanged; it is not read again.
  - A payment applies once: a second run, a callback's repeat, or two
    processes at once change nothing more. See `docs/UPGRADE.md`.

## 4.1.0-rc.1 - 2026-10-01

### Security

- The agent HTTP routes authenticate the node. `POST /api/v2/agent/heartbeat`,
  `GET /api/v2/agent/tasks`, `POST /api/v2/agent/result` and
  `POST /api/v2/agent/monitor` took a node id from the body or query and
  checked nothing, so anyone could:
  - mark any proxy or forward node online;
  - take a node's queued diagnostic and forward bridge tasks, which the real
    agent then never received;
  - complete any task with a forged result: a forward runtime job and its
    forward's runtime state, or a diagnostic result shown to administrators
    (and create such results for any task id);
  - store monitoring data for any node id in the kernel's memory.

  They now require the credentials the agent WebSocket takes (`X-Node-ID` and
  `X-API-Key`, or the `node_id` and `api_key` or `token` query), checked in
  the kernel before the package gateway (`AgentHandler.RequireAgentNode`); the
  credentials never reach the package host, which gets the verified node.
  A node reports and polls only for itself: a body naming another node is
  `403`, and a result for another node's task is `404` and changes nothing.
  A heartbeat or monitoring report marks the node online in the table that
  authenticated it; without a live connection a proxy node's report used to
  mark the forward node with the same id online. See `docs/UPGRADE.md`.
- An administrator's agent task goes out only as a diagnostic task.
  `POST /api/v2/admin/agent/tasks` checked the action and params against the
  diagnostic whitelist but sent the body's `type` to the agent as given;
  `POST /api/v2/admin/agent/execute` already fixed it to `diagnostic`. Any
  other type is now refused with `400`.
- Node and node protocol writes save only their own columns.
  - **Nested objects.** `POST /api/v2/admin/nodes` and
    `POST /api/v2/admin/nodes/:id/protocols` bound the body to the kernel
    model, and GORM saved the associations nested in it.
    - A node's `protocols` moved other nodes' protocols to the new node.
    - A protocol's `node` created a node without an API key and moved the
      protocol to it.
    - `subscription_groups` created groups and linked them.

    These nested objects are now ignored, the answer no longer echoes
    them, and a body's `id` no longer picks the new row's id.
  - **Spellings.** `PUT /api/v2/admin/nodes/:id` refused `id`, `api_key`,
    `api_key_hash` and `secret`, and
    `PUT /api/v2/admin/nodes/:id/protocols/:protocol_id` refused `id` and
    `node_id`. GORM also accepts the field names (`APIKey`, `NodeID`), and
    SQLite any case (`API_KEY`). An administrator's body could replace or
    empty a node's credentials, renumber a node or protocol, or move a
    protocol to another node. Every spelling is now refused.
  - **Checks.** The check of a node's parent ran only for `parent_id` with
    a number, so `ParentID` or `"parent_id": "<id>"` could make a node its
    own parent or a cycle. A protocol update was checked with the value of
    `type` while `Type` was written, so an invalid WireGuard protocol could
    be stored. Both checks now apply to every spelling. A parent that is not
    a whole number, or two keys for one column, now fail the update.
- Forward routes no longer give users node credentials, rules or a view of
  Control's network.
  - **Node tokens.** `GET /api/v2/user/forward/rules` showed the caller's
    rules with their relay and exit nodes, API tokens included. A forward
    node's token authenticates its agent, and any user can create a rule on
    any node (`POST /api/v2/user/forward/rules`), so any user could act as
    any forward node. The answer now shows the nodes with an empty
    `api_token`. Give the nodes new tokens; see `docs/UPGRADE.md`.
  - **Agent rules.** `GET /api/v2/forward/agent/rules` is a public route
    and answered anyone with the rules of any node: every user's listen
    ports and targets. It now answers only the forward node it names, with
    that node's token (`X-API-Key`, `api_key` or `token`); a proxy node's
    key is refused.
  - **Diagnosis.** `POST /api/v2/forward/diagnose` connects from Control to
    the forward's targets, which the user chooses, so a user could probe
    Control's loopback, private and metadata addresses and read which ports
    were open. For a user, every address a target resolves to must now be
    public, and the probe connects to the address it checked; an
    administrator's diagnosis is unchanged.
- An empty token no longer authenticates an agent. The agent WebSocket and
  REST authentication (`verifyForwardNodeToken`) accepted an empty token for
  a node whose API key and hash were empty, and for a forward node without a
  token, so anyone could act as such a node: fetch its users and report
  traffic. Tokens are now compared in constant time, and an empty token is
  refused. A node without a key must be given one; see `docs/UPGRADE.md`.
- Subscription group and template writes save only their own columns.
  `POST` and `PUT /api/v2/admin/subscription/groups[/:id]` and
  `POST /api/v2/admin/subscription/groups/:id/templates` bound the body to
  the kernel model, and GORM saved the associations nested in it.
  - A group's `protocols` upserted `v2_node_protocol` rows, and the nodes
    nested in them into `v2_node`, without the node routes' checks and with
    an empty API key, and linked them to the group. Linking protocols has
    its own route, `POST .../groups/:id/protocols`.
  - A group's `templates` created templates or moved existing ones from
    other groups.
  - A template's `group` created a group, or moved the new template into
    another group than the one in the path.

  These nested objects are now ignored, and the answer no longer echoes
  them. `PUT /api/v2/admin/subscription/templates/:id` dropped `id`,
  `created_at` and `updated_at` from the update, but GORM also accepts the
  field names (`ID`, `CreatedAt`) and SQLite any case (`Id`), which changed
  a template's id or creation time; every spelling is now dropped.
- Kernel API views that filter rows are PostgreSQL security barriers.
  `kapi_system_audit_log_v1` shows only the `system` rows of
  `v2_operation_log`, but a package could define a cheap function, which the
  planner may run before the view's filter, and see every module's audit
  rows. Such views are now created `WITH (security_barrier)`, and an existing
  one is altered to be a barrier at startup.
- The PayPal webhook marks a payment paid only for a completed capture of
  the record's amount and currency. It treated `CHECKOUT.ORDER.APPROVED`
  as paid, although an approved checkout has collected nothing until it is
  captured, and it did not compare the captured amount.
  `CHECKOUT.ORDER.COMPLETED` is acknowledged without effect; its captures
  arrive as `PAYMENT.CAPTURE.COMPLETED`.
- Commission withdrawals can no longer overdraw or be refunded twice.
  - **Overdraw.** `POST /api/v2/user/invite/withdraw` checked the commission
    balance before its transaction and then subtracted the amount
    unconditionally, so concurrent requests could take the balance below
    zero. The withdrawal and its debit are now one transaction, and the debit
    is taken under the subscriber's row lock and refused below zero
    (`subscriber.AdjustBalanceTx`, ledger id `affiliate.withdraw:<id>`).
  - **Fractions.** The commission balance is a whole number of cents, but a
    withdrawal took any amount from 1. On PostgreSQL the driver truncated the
    amount bound to `commission_balance - ?`, so a withdrawal of 1.99 debited
    1 and its approval paid out 1.99; SQLite stored a fraction in the
    integer column. A fractional amount is now
    `amount must be a whole number`.
  - **Double processing.** `POST /api/v2/admin/invite/withdrawals/:id/process`
    read the withdrawal, then saved the decision unconditionally, so two
    concurrent decisions both applied and a rejection refunded twice. A
    decision now applies only while the withdrawal is pending; the other is
    `withdrawal already processed`.
  - **Refunds.** A rejection's refund was a separate write whose failure was
    only logged, leaving the withdrawal rejected and the amount lost. The
    refund is now part of the decision (ledger id
    `affiliate.withdraw.refund:<id>`): if it fails, the answer is
    `failed to process withdrawal` and the withdrawal stays pending. A
    pending withdrawal stored with a fraction is refunded its whole part,
    which is what PostgreSQL debited; one whose user no longer exists is
    rejected without a refund, as before.
  - **Upgrade note.** Pending withdrawals with a fractional `amount` were
    debited only their whole part on PostgreSQL. Review them
    (`GET /api/v2/admin/invite/withdrawals?status=pending`) before approving
    them.
  - **Minimum.** The handler serving users kept the configuration it first
    read, so an administrator's new minimum withdrawal applied only after a
    restart. Each withdrawal now reads it.
- A payment can no longer activate an order it does not pay.
  `POST /api/v2/user/payment/create` took any `order_id` with any amount the
  gateway allowed, and a paid callback marked that order paid and assigned
  its plan: a user could pay the gateway minimum for any plan, or attach
  another user's order.
  - **Creation.** With an `order_id`, the order must be the caller's, still
    pending, and the amount its total to the cent. Otherwise the answer is
    `订单不存在`, `订单已支付或已取消` or `支付金额与订单金额不符`.
    `/api/v2/payment/x402/create` and `/api/v2/payment/fiat/create` also
    answer `订单不存在` for another user's order; their amounts already
    came from the order.
  - **Callbacks.** A paid callback for a record created before this fix that
    names another user's order, or pays less than its total, records the
    payment but leaves the order unpaid and its plan unassigned. Such a
    payment is logged for an administrator.
- Users see only their own payment records. `GET /api/v2/payment/x402/check/:id`
  (by trade number or numeric id) and `GET /api/v2/user/payment/status/:trade_no`
  answered any user's record; another user's record is now `支付记录不存在`.
  The public `GET /api/v2/payment/status/:trade_no` is unchanged.
- Administrators' payment gateway responses no longer carry secrets. The
  gateway list, create and update answers showed the whole `config`, with
  EPay's `key`, Stripe's `secret_key` and `webhook_secret`, PayPal's
  `client_secret` and similar values. These now read `********`.
  - A configuration saved back with `********` keeps the stored secret, so
    the admin page's edit form works unchanged.
  - A configuration that is not a JSON object is shown as `********` whole.
- The administrator audit trail no longer records credentials. The audit
  middleware stored the raw body of every administrator write request in
  `v2_audit_log.request_body` (up to 4 KiB) and logged its first 512 bytes.
  That included user passwords, payment gateway keys (also inside the
  gateway's JSON `config` string), SMTP and S3 credentials, and bot tokens.
  - **Redaction.** Bodies are now redacted before they are logged or stored.
    Any field whose name contains `password`, `secret`, `token`, `key`,
    `private`, `credential` or `uuid` becomes `[REDACTED]`, except public
    names such as `public_key` and `key_id`. So does the `value` of a
    sensitive system setting, sent as `{"key","value"}` or to
    `PUT /admin/system/configs/:key`.
  - **Non-JSON bodies.** A body that is not JSON, or is larger than 64 KiB, is
    recorded only by its size.
  - **Old rows.** Existing rows are not rewritten; `docs/UPGRADE.md` shows how
    to clear them.
- User access tokens are verified in one place, `internal/authn`, and can be
  revoked (N8). It covers the HTTP middleware, the admin monitor WebSocket
  and the gRPC interceptor.
  - **Pinning.** The algorithm is pinned to HS256, which closes the
    algorithm-confusion gap. `iss`, `exp` and `iat` are required.
  - **Session id.** New tokens carry a session id (`sid`); the login
    response is unchanged.
  - **Revocation tables.** New tables `v4_kernel_identity_revocation`
    (per user: `not_before`, `token_version`) and
    `v4_kernel_identity_session_revocation` (session denylist). They are
    cached in memory and reloaded every 5 s.
  - **Triggers.** Banning or deleting a user, or changing their password,
    email, admin flag or ban flag, revokes that user's existing tokens at
    once. Before, a banned user's token kept working until it expired (24 h
    by default). Updates that resend unchanged values revoke nothing.
  - `utils.ParseToken` is removed; use `authn.Default().Verify`.
- Administrator routes no longer run a path id as SQL. The notification
  template update and delete and the withdrawal processing passed the raw
  `:id` to GORM, which runs a non-numeric id as an inline condition (for
  example `0 OR 1=1`, which matches every row). They now parse the id:
  - an invalid template id answers "template not found" on update and
    "invalid template id" on delete, where delete used to leak the database
    error;
  - an invalid withdrawal id answers 404 "withdrawal not found", as before.

### Removed

- The legacy source/Docker Compose installer in the root `install.sh`
  (`ANIX_CONTROL_LEGACY_SOURCE_INSTALL=1`). It built images on the target
  host against the release policy. `install.sh` now only forwards to the
  frozen systemd release installer and no longer carries version fields
  (`check_release_version.py` and AGENTS.md updated).
- Removed the legacy V2bX/Xray proxy-node test tooling, which was never part of
  a release build: `cmd/integration-test`, `cmd/configgen`, `cmd/report`,
  `cmd/verify`, `cmd/subtest`, the `internal/tests/integration` harness packages
  (`binary`, `clients`, `config`, `runner`, `echo`, `e2e`, `local`, `mock`), the
  `integration-test.yml` workflow, the `make test-integration` target,
  `config/examples/`, `tools/mock_gost_api.py`, and the unused
  `config/scripts/{setup_integration.go,generate-grpc.sh,coverage.sh,test-all.sh}`
  and `api/grpc/gen.ps1` helpers (use `api/grpc/gen.sh`). The package rollout
  tests in `internal/tests/integration` are kept.
- Removed Go code that no binary reaches: the unused `internal/websocket`
  subscription hub, the in-memory agent task store, the never-registered gRPC
  `ConfigSyncService` implementation (the `.proto` and generated code are
  unchanged), the in-process Control plugin executor registry and its
  GOST mesh / NAT egress / nftables forward executors (the production kernel
  never installed it), the `gost.Manager` rule-sync methods, and unused helpers
  in the cache, compat v2, gRPC, middleware, parser, utils and service layers.
  Unwired but implemented notification, invite commission, load balancer,
  backup cleanup and Telegram bot features are intentionally kept. Test-only
  helpers moved into test files, and backup tests now write to temporary
  directories instead of the source tree.
- `golang.org/x/net` is now an indirect dependency.

- Removed dead frontend code from `web/`: the unreferenced
  `views/admin/forward/ForwardCard.vue`; the System page's hidden runtime
  config editor state and save handler (`saveForwardRuntimeConfig`,
  `runtimeConfigPreview`, `applyDefaultRuntimeAnsibleConfig`,
  `runtimeSaving`, `runtimeValidationError`), which the template never
  rendered (runtime settings stay editable on the Local Runtime and NodeX
  Runtime pages); 21 API client helpers that only tests called (17 in
  `api/admin.js`, `getDashboard` and `getKnowledgeDetail` in `api/user.js`,
  `getKernelPluginReleaseArtifact` and the `diagnoseKernelTopology`
  compatibility wrapper in `api/kernel.js`); unused `menuRegistry`
  re-exports and admin extension exports from `extensions/runtime.js`; and
  the unused `PRODUCT_NAME` and repository URL constants.
- Removed 95 unused i18n keys from both `en` and `zh-CN`, plus the legacy
  literal-translation entries whose source strings no longer appear in the
  frontend, backend, packages, or agent (23 in `en`, 30 in `zh-CN`).
- Stripped UTF-8 byte order marks from nine frontend source files.

- Removed obsolete and personal repository files: the Windows-era `.claude/`
  directory (personal settings, memory, and `v2board.exe` build commands),
  `.superpowers/`, the agent plans/specs under `docs/superpowers/`, the
  2026-04 agent work plans (`docs/guide/forward-runtime-work-plan.md`,
  `docs/guide/flux-panel-workstream.md`), the retired 3.1-to-4.0 planning docs
  (`docs/FEATURE_ROADMAP.md`, `docs/architecture/upgrade-program.md`,
  `docs/architecture/plugin-platform-roadmap.md`), stale V2bX-era and one-off
  docs (`docs/V2BX_LOCAL_NODE_SETUP.md`, `docs/ANSIBLE_INTEGRATION_GUIDE.md`,
  `docs/arco-design-vue-setup.md`, `docs/guide/test-release-v2.0.2-test.1.md`,
  `docs/audit/admin-workbench-verification-2026-07-18.md`,
  `docs/coverage/grpc-coverage.html`), the personal-domain Nginx configs in
  `config/deploy/nginx/`, the unused `config/deploy/gost/` bundle, and the
  V2bX-era `scripts/check-health.sh` and `scripts/setup.sh` (which carried a
  hard-coded token). `docs/FEATURE_ROADMAP.md` is no longer accepted as
  documentation evidence by `config/scripts/check_docs_updated.sh`.

### Changed

- CI has a fast lane for pull requests and a full lane for everything else
  (`config/scripts/classify_changes.py`).
  - **Fast lane.** The required checks, plus the heavy jobs whose paths
    changed.
    - Docker and Kubernetes smokes, the PostgreSQL jobs, and the forward,
      Agent and package jobs each run only for their areas.
    - The race detector and benchmarks run only in the full lane.
    - Backend tests no longer wait for the quality gates.
    - Documentation-only PRs skip the Go jobs.
  - **Full lane.** Runs on `go_dev` pushes, tags, a nightly schedule, manual
    runs, the `ci:full` label, and workflow or Go dependency changes.

- Releases follow an individual-developer flow (`docs/RELEASING.md`).
  - **Cutting a release.** `config/scripts/prepare_release.py <version>`
    sets every declared version and dates the CHANGELOG. Merge that through
    a pull request, then push the `vX.Y.Z` tag.
  - **Package set.** The tag pipeline signs every package under `packages/`
    at the tag version: 18 today, including `platform` and `affiliate`. The
    old per-stage package list pinned releases to 4.0 and 16 packages;
    v4.1.0 can now ship.
  - **Release body.** The GitHub Release body is the tag's CHANGELOG
    section. Suffixed tags are prereleases and never become the latest
    release.
  - **Removed gates.** The release-stage contract, the public rehearsal, the
    signed evidence bundle, and the canary and support approvals are gone,
    together with their scripts. So are the `OPERATOR_DEPLOYMENT.md`,
    migration dry-run and legacy signature-verifier release assets.
    `v4.0.0` keeps its evidence bundle, and its guides still describe it.
  - **Kept.** Package signing with the protected root (checked against the
    shipped trust root), the cosign-signed multi-architecture image with SBOM
    and provenance, the source SBOM, checksums, the release manifest and the
    full test suite before release.
  - **Policy check.** `check_release_workflow.sh` shrinks from 997 to about
    380 lines of essentials. Its self-test now mutates the real workflow.
- **A successful payment now activates the plan at once.** The payment
  callback completes the paid order through the entitlement engine; before,
  an administrator had to complete paid orders.
  - If the activation fails (for example, the plan was deleted), the payment
    stays recorded, the order stays paid for an administrator, and the
    failure is logged.

- 21 v2 routes moved out of identity-platform (N7). Paths and responses are
  unchanged; the routes stay bridged to the same kernel handlers.
  - **New packages.** `platform` takes system configuration, audit logs and
    backup (12 routes, `platform.admin.system.*`). `affiliate` takes
    commissions, withdrawals and invite statistics and configuration (8
    routes, `affiliate.*.invite.*`).
  - **forward.** `POST /api/v2/user/reset` (Flux reset flow) moves to
    `forward` as `forward.user.reset.post`.
  - identity-platform keeps 24 routes: login, register, profile, dashboard,
    user administration, MFA, and invite codes. The builder, route catalog,
    extraction map and route gates know the two new package ids.
  - **Transition.** Old identity-platform releases keep working: the kernel
    still accepts their old route ids for the moved routes and runs the new
    owner's handler. While both packages declare a route, the new owner
    serves it (`internal/compat/v2/moved_routes.go`). Install `platform` and
    `affiliate`, and upgrade `forward`, before upgrading identity-platform.
- `anix-control module ...` commands log to stderr, so their JSON output on
  stdout can be piped (for example into `jq`).
- The AnixOps contracts and SDKs are now their own Go module,
  `github.com/AnixOps/anix-control/sdk`, in `sdk/`.
  - **Moves.** `api/{pluginhost,packagebridge,modulepki,identity,kernelidentity}`
    moved to `sdk/api/...`, and `pkg/{moduletls,pluginhostsdk,packagebridgesdk,modulesdk,packagestoresdk,v2compat}`
    moved to `sdk/...`. Import paths change accordingly; the wire formats do
    not.
  - **Consumers.** Other AnixOps services can depend on the contracts and
    SDKs, versioned by `sdk/vX.Y.Z` tags, without pulling in the kernel. The
    kernel uses the module through a local `replace`.
  - **Boundary gate.** `check_package_boundaries.sh` now fails when the SDK
    module depends on the kernel module. The SDK contract tests that use the
    kernel's real bridge moved to `internal/tests/bridgecontract`.
  - **CI and tooling.** CI builds, vets, tests (including race), lints,
    tidies and scans the SDK module. The Dockerfile copies `sdk/go.mod`
    before downloading modules.

- The generic Control package host and the identity-platform host now run
  on `pluginhostsdk.Router`. They have no native routes, so every request
  still passes through the bridge to its legacy handler.
- Package migration indexes are materialized per release.
  - Source `packages/*/migrations/index.json` files use the
    `__ANIXOPS_PACKAGE_VERSION__` token.
  - `build_package.py` writes the build version and a SHA-256 for every step
    script into the packaged index. It also rejects unknown index fields,
    source-provided digests and malformed step ids.
  - Packages built for any tag other than `v4.0.0` previously failed: the
    source index was pinned to `4.0.0` and had to equal the build version.
- The kernel now parses and verifies the migration index when it materializes
  a Control artifact. It checks the index digest, the package and version,
  every step id and path, and every step digest. The verified steps are
  exposed on the artifact reference so package migrations can be run through
  the ledger. v4.0.0 indexes without step digests stay valid.
- Manifest `capabilities` are validated by Control.
  - Names are lowercase and unique.
  - In the `kernel.` namespace only the listed forms are accepted:
    `kernel.observed-state`, `kernel.storage.v1`,
    `kernel.storage.adopt:<table>` and `kernel.view:kapi_<name>_v<N>`.
  - Kernel and identity tables cannot be adopted.
  - See `docs/architecture/plugin-kernel-contract.md`.

- Documentation now treats containers as the primary deployment.
  - README, `docs/README.md`, `docs/control-boundary.md`,
    `docs/features.md`, AGENTS.md and the UPGRADE alpha section point to the
    Compose and Helm paths first. The systemd installer is marked frozen in
    the release installation guide, and `scripts/install.sh` prints a notice.
  - New `docs/reference/environment-variables.md` lists all `ANIX_CONTROL_*`
    variables. It is generated from the configuration structure and kept
    current by `TestEnvironmentVariableReferenceIsCurrent`.
  - `TODO.md` now aims the production upgrade at containers and tracks the
    multi-replica (HA) work.

- Rewrote `docker-compose.prod.yml` for container-first production.
  - It runs only Control against an external PostgreSQL: a one-shot
    `migrate` service, then `control`.
  - The image is pinned by digest (`ANIX_CONTROL_IMAGE`), with no local
    build.
  - Settings come from `control.env` (`config/deploy/compose/control.env.example`).
    The JWT secret and database password come from `secrets/` files
    passed as `ANIX_CONTROL_*_FILE`.
  - Hardening: read-only root filesystem with an exec `/tmp` tmpfs,
    `cap_drop: ALL`, `no-new-privileges`.
  - Operations: `/readyz` healthcheck, 90 s stop grace period, rotated JSON
    logs; ports published on `127.0.0.1` by default.
  - Removed the bundled Redis (unused by the code), Postgres, Prometheus,
    Grafana and nginx services and their mounts of files that did not
    exist.
- `docker-compose.yml` is now a development stack: it builds the `source`
  target with a throwaway PostgreSQL and no longer bind-mounts `web/public`
  over the built frontend.
- The nginx and Prometheus configs moved to `config/deploy/examples/` as
  host-side examples for the published ports (`config/docker/` is gone).
- `.env.example` now documents the Compose variables (`ANIX_CONTROL_IMAGE`,
  bind addresses, ports) instead of settings the server never read.
- `docker-smoke` also runs the production Compose file against a separate
  PostgreSQL: `migrate` completes, `control` becomes healthy, stops cleanly.

- The container image was rebuilt for container-first deployments.
  - **Targets:** `source` (default) builds from the checkout. `release`, used
    only by CI, copies the exact release binary, frontend and signed
    `identity-platform` bootstrap package. A fresh database therefore
    bootstraps without extra files
    (`ANIX_CONTROL_PLUGINS_IDENTITY_BOOTSTRAP_PACKAGE_DIR` is preset).
  - **Base images:** pinned by digest. The Go and frontend stages
    cross-compile on the build platform.
  - **Runtime:** runs as uid 10001 (`anixops`, previously uid 1000
    `v2board`) under `tini`, with no config file, and works on a read-only
    root filesystem with a writable `/tmp`. `HEALTHCHECK` uses `/readyz`.
    Ansible state goes to `/tmp/.ansible`.
  - **Contents:** only the ansible playbooks and `ansible.cfg` are copied.
    `.dockerignore` keeps `config/deploy/ssh`, inventories, keys, `.env`
    files and `config/tls` out of the build context. The legacy `/app/v2board`
    symlink is gone.
- Release images are published to `ghcr.io/anixops/anix-control` instead of
  Docker Hub.
  - Platforms: `linux/amd64` and `linux/arm64`.
  - Tags: `X.Y.Z`, `X.Y` (stable only) and `sha-<commit>`.
  - SBOM and provenance attestations are attached, and the digest is signed
    with cosign keyless signing.
  - `docker-image.txt` records the digest, platforms and signature.
  - `check_release_workflow.sh` enforces these properties.
  - Pushes to `go_dev` publish `:edge` and `:sha-<commit>` images for
    pre-release testing.
- `Docker Build Smoke` now runs the image instead of only building it:
  `migrate` against PostgreSQL; a read-only, non-root container configured
  only by environment; `/readyz`, `/livez` and an unauthenticated `/api/v3`
  401; then a graceful `docker stop` that must exit 0.

- Recorded the production baseline and the local upgrade rehearsal from
  `v4.0.0-alpha.7` to `go_dev` on a copy of the 2026-09-29 production database
  (`docs/architecture/release-line-status.md`, `docs/RC-EVIDENCE-4.0.x.md`).
  Subscriptions (360/360) and UniProxy (45/45) were byte-identical, and all
  82 `/api/v2` GET routes returned the same status. `docs/UPGRADE.md` gains an
  alpha-to-4.0.x upgrade section: build identification, the five added kernel
  tables, the package install window, and network isolation for rehearsals.
  It also documents the empty-password DSN fix. `TODO.md` tracks the
  production upgrade, the random order of forward observability targets, and
  the missing switch for outbound background workers.

- `/api/v2` route resolution no longer re-reads and re-hashes every installed
  package artifact on each request. Successfully verified route declarations
  are cached per signed release (keyed by release id, artifact SHA-256, stored
  trust-root fingerprint and configured trust-root fingerprint; LRU-bounded to
  256 releases; failures are never cached), shared by the HTTP and WebSocket
  gateways. Installations, the official plugin row and trust-root activity are
  still read on every request (four small queries in total), so disabling a
  package, bumping its generation, or retiring its trust root takes effect on
  the next request. With 16 installed ~10 MiB packages on SQLite a resolution
  drops from about 197 ms to about 0.22 ms. `internal/compat/v2` joins the
  benchmark smoke CI job.

- The required "Go Quality Gates" CI job now runs the plugin-only `/api/v2`
  route gate (`check_plugin_only_routes.py`, which includes the route catalog
  check), the `config/scripts` Python unit tests, and generated-code drift
  checks for `api/pluginhost` and `api/packagebridge` on every pull request.
  Previously the route gate ran only on release tags and the Python tests never
  ran in CI.

- Made the `go_dev` CI pipeline green again. Go moves to `1.26.8` (go.mod
  toolchain, CI, SDK sync workflow, and a `golang:1.26-alpine` Docker builder),
  and `google.golang.org/grpc` moves to `v1.83.2` (with `golang.org/x/net`
  `v0.58.0`, `x/text` `v0.41.0`, `x/crypto` `v0.55.0`), which clears every
  govulncheck finding that reaches our code while keeping `go 1.25.0` as the
  module minimum. The web lockfile picks up fixed `postcss`, `nanoid` and
  `brace-expansion`, and Vitest moves to `^4.1.11`, so `npm audit` is clean.
- CI scanners are pinned (`golangci-lint` `v2.14.0`, `govulncheck` `v1.8.0`,
  `gosec` `v2.29.0`) so new upstream rules no longer turn `go_dev` red without a
  code change. The root pipeline drops the nonexistent `production` branch
  trigger, runs on every pull request again (its jobs become the required
  checks for `go_dev`), and cancels superseded pull-request runs.

- Rewrote the repository entry points: `AGENTS.md` is now a concise English
  rules file for the v4 kernel and package bridge, branch/PR workflow and
  required checks, build/test commands, the documentation sync gate, version
  bump surfaces, release policy, Flux-clone and runtime guardrails, the
  Control Center, and credential rules. The dated Flux clone status moved into
  `docs/guide/flux-panel-clone.md`. `ROADMAP.md` now points at the maintained
  roadmap documents, and `TODO.md` keeps only open items plus a new
  "Implemented but not wired" list and deployment hygiene gaps.
- `docs/reference/repository-layout.md` is the single repository layout
  document (the README copy was removed); `docs/intro/README.md` gained a
  "v4 architecture at a glance" section; `docs/README.md` and
  `docs/guide/README.md` index every current guide; docs no longer link to
  gitignored local files, deleted docs, `config/examples/`, or Windows paths.
- `.github/BRANCH_PROTECTION.md` documents the `go_dev` ruleset, and
  `.github/CODEOWNERS` is a single valid UTF-8 default rule.
- Repaired GBK-mojibake comments in `Makefile`, `config/config.prod.yaml`, and
  `docker-compose.prod.yml` (which also lost its UTF-8 BOM); the repaired
  `make help` text no longer has unterminated quotes. No configuration values
  changed.
- `.gitignore` now ignores agent-local state (`.claude/settings.local.json`,
  `.claude/projects/`, `.superpowers/`).
- Corrected documentation made stale by PR #7 and PR #8: `TODO.md` (removed
  the deleted `test-all.sh`/`coverage.sh` item; gost-mesh/nftables-forward
  Control validation must now be rebuilt; gost-mesh is QUIC/WSS with TUIC out
  of v1 scope; new "Later" section), the release-workflow row in
  `docs/features.md` (`v4.0.0` was published), the audit registers (current
  toolchain, commands, packages, workflows; removed `internal/websocket` hub),
  the RC roadmap/evidence notes, and the trust-root paragraph in
  `docs/architecture/plugin-kernel-contract.md` (Control keeps one active
  root and retires the others at startup).

- Imported the Control Center into `control-center/` as a single snapshot of the
  archived `AnixOps/Anixops-control-center` repository (`master` merged with
  `production`, without committed release binaries or assistant notes). It stays
  a separate Go module, `github.com/AnixOps/anix-control/control-center`, and
  its CI and release pipelines moved to `.github/workflows/control-center.yml`
  and `control-center-release.yml` (tags `control-center-v*`, never marked as
  the latest release). The Center web and Flutter clients dropped the AI, Web3
  and unrouted observability mock pages whose Workers endpoints are being
  removed. `go_dev` is now the only long-lived branch.
- The root CI pipeline now ignores changes limited to `control-center/**`, and
  its gosec and swag steps, the Makefile `swagger` target and the Docker build
  context exclude `control-center/`.
- Imported the Control Center Cloudflare Workers API into
  `control-center/workers/` as a single snapshot of the archived
  `AnixOps/Anixops-control-center-worker` repository, trimmed to the routes the
  Control Center clients use (platform probes, auth/MFA, users, nodes,
  node-groups, playbooks, tasks, schedules, notifications, dashboard, audit
  logs, SSH, plugins, agents, logs, backups, batch, SSE/WebSocket). Incidents,
  governance, webhooks, Kubernetes/load-balancer/mesh/autoscaling, AI/vector,
  Web3/IPFS and developer-mode routes were removed, and the unused `AI` binding
  was dropped. D1 migrations are unchanged. CI runs from
  `.github/workflows/control-center-workers.yml`; deployment stays on Cloudflare
  Workers Builds (root `control-center/workers`, branch `go_dev`).

### Added

- **Subscriber contract (F2).** `KernelSubscriber`
  (`sdk/api/kernelsubscriber/v1`, design in
  `docs/architecture/subscriber-service.md`) is the kernel's contract for
  shared subscriber state: entitlements, traffic counters, subscription
  credentials, balances, and a directory with a change feed for node user
  lists.
  - Domain modules will call it instead of writing `v2_user`, per capability
    family (`kernel.subscriber.{entitlements,traffic,credentials,balance,directory}.v1`).
  - Writes are idempotent by request id.
  - The contract is registered in the proto compatibility and drift checks.
  - **Entitlement engine (F2a).** `internal/subscriber.ApplyEntitlementTx` is
    now the one writer of plan activations, with the request ledger
    `v4_kernel_subscriber_request`. Order completion and administrator plan
    assignment use it; their results are unchanged.
  - **Traffic ledger (F2b).** `RecordTrafficTx` and `ResetTrafficTx` are now
    the only writers of subscriber traffic counters: node reports, the traffic
    service, forward flow and manual resets.
    - Updates run in user-id order, which removes a deadlock risk between
      concurrent node reports.
    - A batch id makes a report apply once.
  - **Subscriber directory and change log (F2c).**
    - `subscriber.Active` defines the subscribers a node serves; UniProxy and
      v2board gRPC user lists use it.
    - `v4_kernel_subscriber_change` records every change that can alter a
      node's user list: entitlements, ban, uuid, traffic exhaustion or reset,
      creation and deletion. It is pruned after 7 days.
    - New kernel view `kapi_subscriber_entitlement_v1` exposes entitlements
      and counters, without token or uuid.
  - **Contract served (F2d).** `KernelSubscriber` is served on local package
    bridge sessions and on the mTLS module listener to official packages.
    Each method family is authorized by its own signed capability:
    `kernel.subscriber.{entitlements,traffic,credentials,balance,directory}.v1`.
    - Writes are idempotent by request id.
    - Directory reads carry no token.
    - `WatchSubscriberChanges` streams each change with the subscriber's
      current state, and answers `RESYNC` for pruned cursors.
- **Knowledge pilot: the first domain module on an adopted table.**
  - `packages/knowledge` now has its own host with native handlers for all 6
    routes, on the kernel's `v2_knowledge` table adopted in place
    (`kernel.storage.v1`, `kernel.storage.adopt:v2_knowledge`).
  - `internal/tests/knowledgecompat` proves byte parity with the legacy
    handlers on SQLite and PostgreSQL. The PostgreSQL run is part of the
    `package-storage-postgres` job.
  - Legacy and native share the table, so each route's mode switches freely
    through the installation's `routes` configuration.
  - `config/package-extraction.json` now marks the knowledge routes and
    identity's 15 group A routes `native-flagged`.
- **Ticket module on adopted tables.** `packages/ticket` serves its 8
  routes natively on `v2_ticket` and `v2_ticket_message`, adopted in place.
  `internal/tests/ticketcompat` proves byte parity on SQLite and PostgreSQL,
  and the PostgreSQL run is part of CI.
- **Notification module on adopted tables.** `packages/notification` serves
  19 of its 24 routes natively on the `v2_notification_template`,
  `v2_notification_log`, `v2_telegram_bot` and `v2_telegram_user` tables,
  adopted in place, and reads members' e-mail addresses through
  `kapi_user_directory_v1`. The Telegram Bot API calls those routes make are
  ported into the package. `internal/tests/notificationcompat` proves byte
  parity on SQLite and PostgreSQL, and the PostgreSQL run is part of CI. The
  e-mail configuration and test send, setting the webhook, and the public
  Telegram webhook stay bridged.
- **Platform module on adopted tables.** `packages/platform` serves 4 of its
  12 routes natively: the backup configuration, list and statistics on
  `v2_backup_config` and `v2_backup_record`, adopted in place, and the system
  audit log through the new kernel view `kapi_system_audit_log_v1` (the
  `v2_operation_log` rows of module `system`; read-only).
  `internal/tests/platformcompat` proves byte parity on SQLite and
  PostgreSQL, and the PostgreSQL run is part of CI. The system configuration
  routes, updating the backup configuration, and creating, deleting and
  restoring backups stay bridged. `EnsureKernelAPIViews` now leaves out a
  view whose source table does not exist yet.
- **Plan module, the first on KernelSubscriber.** `packages/plan` has its own
  host and serves 7 of its 12 routes natively: the admin plan CRUD, the
  assignment and the user plan list.
  - It runs on `v2_plan` and `v2_event`, adopted in place.
  - An administrator's assignment calls `KernelSubscriber.ApplyEntitlement`
    over the bridge connection (`kernel.subscriber.entitlements.v1`). The
    package never writes `v2_user`.
  - Assignments are now idempotent on the legacy and native paths alike. The
    request id is `plan.assign:<plan>:<user>:<digest>`, a digest of the
    request's `Idempotency-Key` (else its request id) and the expiry, and it
    is recorded in `v4_kernel_subscriber_request`. A retried request applies
    once, and a new request is a new grant, as before.
  - `internal/tests/plancompat` proves byte parity on SQLite and PostgreSQL,
    and the same `v2_user` rows, request ledger and change log, against the
    real KernelSubscriber server. The PostgreSQL run is part of CI.
  - The five `/api/v2/speed-limit/*` routes stay bridged: they are forward
    limits on forward tunnels and join the forward package later.
  - The parity harness can send request headers (`Case.RequestHeaders`).
- **Order module.** `packages/order` has its own host and serves 9 of its 13
  routes natively: the coupon routes, order statistics, status changes,
  cancellation, "mark paid" and the user's order creation.
  - It runs on `v2_order` and `v2_coupon`, adopted in place. Plans are read
    through the new kernel views `kapi_plan_catalog_v1` and
    `kapi_plan_subscription_group_v1`, the buyer's current plan through
    `kapi_user_directory_v1`.
  - "Mark paid" completes the order through
    `KernelSubscriber.ApplyEntitlement` (`kernel.subscriber.entitlements.v1`)
    with the kernel's request id `order:<order id>`, so a plan is granted once
    whichever side completes the order. The package never writes `v2_user`.
    Completion pays no commission and sends no notification, as in the
    kernel.
  - The four order list and detail routes stay bridged: their answers embed
    the buyer's `v2_user` row with its subscription token and UUID.
  - `internal/tests/ordercompat` proves byte parity on SQLite and
    PostgreSQL, and the same orders, coupons, subscribers, request ledger and
    change log, against the real KernelSubscriber server. The PostgreSQL run
    is part of CI, and a change under `packages/*/native` now runs the
    PostgreSQL jobs in the pull request lane.
  - `EnsureKernelAPIViews` leaves out a view whose source table does not
    exist.
- **Payment module.** `packages/payment` has its own host and serves 16 of
  its 20 routes natively: gateway administration, payment records and
  statistics, the user's channels, payments and their status, the method
  list, and x402 and fiat payment creation.
  - It runs on `v2_payment_gateway`, `v2_payment_record` and `v2_payment`,
    adopted in place. As the gateways' owner it holds their secrets; its
    administrator answers redact them as the kernel's do.
  - Orders are read through the new kernel view `kapi_order_billing_v1`
    (buyer, total, status) only. No native route changes an order or a
    subscriber, and none calls a payment provider: the fiat routes are
    stubs, as in the kernel.
  - The four callback routes stay bridged. A paid callback marks the payment,
    the gateway statistics and the order paid and completes the order in one
    kernel transaction, with no contract yet for the order's part.
  - `internal/tests/paymentcompat` proves byte parity on SQLite and
    PostgreSQL, and the same payment records, gateways and orders. The
    PostgreSQL run is part of CI.
- **Affiliate module.** `packages/affiliate` has its own host and serves 7
  of its 8 routes natively: the user's commissions, withdrawals and
  withdrawal request, and the administrator's withdrawal list and
  decisions, invite statistics and configuration.
  - It runs on `v2_commission_record`, `v2_commission_withdraw` and
    `v2_invite_config`, adopted in place.
  - Other domains are read through kernel views: the new
    `kapi_user_referral_v1` (who invited whom),
    `kapi_subscriber_entitlement_v1` (the commission balance) and the new
    `kapi_affiliate_settings_v1`, which shows the one non-sensitive
    `v2_system_config` row holding the frontend settings
    (`invite.frontend.config`). A view that filters rows is a
    `security_barrier` view on PostgreSQL, so a package's own functions
    cannot see the rows it hides.
  - Withdrawals debit and rejections refund the commission balance through
    `KernelSubscriber.AdjustBalance` (`kernel.subscriber.balance.v1`), with
    the ledger ids the kernel's handlers use, so a balance changes once
    whichever side serves the request. The package never writes `v2_user`.
    The contract already covered the commission balance and is unchanged.
  - A native withdrawal is recorded as a reservation (status `-1`) until its
    debit applies, and a rejection claims the withdrawal before refunding
    it, so no failure pays out or refunds an undebited amount.
  - Updating the configuration stays bridged: it writes `v2_system_config`,
    which no package may adopt and no contract writes.
  - `internal/tests/affiliatecompat` proves byte parity on SQLite and
    PostgreSQL (80 cases each), and the same withdrawals, balances, request
    ledger and configuration, against the real KernelSubscriber server. The
    PostgreSQL run is part of CI. The parity harness now tells a path its
    route does not match from a 404 the handler answers.
- **Forward module.** `packages/forward` has its own host and serves 17 of
  its 80 routes natively: the forward lists and display order, the tunnel
  list, creation and the deletion of an unused tunnel, the tunnels a
  forward may use, granting and listing tunnel permissions, a forward's
  ingress latencies, the node statistics, the user's legacy rules and the
  administrator's `POST /api/v2/user/reset`.
  - It runs on `v2_forward`, `v2_forward_tunnel`, `v2_forward_user_tunnel`,
    `v2_speed_limit`, `v2_forward_rule` and `v2_forward_latency_bucket`,
    adopted in place. None of its native routes changes what a node runs.
  - Forward nodes are read through the new `kapi_forward_node_v1` (every
    column but `api_token`), the runtime backend through the new
    `kapi_forward_runtime_settings_v1` (three keys of `v2_system_config`, a
    `security_barrier` view), users through `kapi_user_directory_v1` and
    their speed limit through `kapi_subscriber_entitlement_v1`.
  - `v2_forward_node`, `v2_forward_clean_agent` and `v2_forward_runtime_job`
    are now protected kernel tables that no package may adopt: they hold
    the forward nodes' API tokens, the clean agents' tokens, and clean agent
    jobs whose payloads carry a node's token.
  - The subscriber traffic reset (`POST /api/v2/user/reset`, type 1) goes
    through `KernelSubscriber.ResetTraffic` (`kernel.subscriber.traffic.v1`).
    Its request id is `forward.reset_traffic:<user>:<digest>`, a digest of
    the request's `Idempotency-Key` (else its request id); the legacy
    handler now derives and records the same id, so a retry applies once
    whichever side serves it.
  - The other 63 routes stay bridged, with the reason in the host's route
    map: forward nodes and Ansible machines (node tokens, the kernel's gost
    client cache, probes), every change that applies forwards or legacy
    rules on nodes, diagnoses, runtime status and jobs, the observability
    routes that read `v2_node`, clean agents, and flow accounting.
  - `internal/tests/forwardcompat` proves byte parity on SQLite and
    PostgreSQL (167 cases each) and the same forwards, tunnels,
    permissions, counters, request ledger and change log, against the real
    KernelSubscriber server. The PostgreSQL run is part of CI.
- **Identity routes outside group A.** identity-platform serves 5 of the 9
  natively: the user's profile and dashboard, and the administrator's user
  detail, traffic reset and subscription reset.
  - **Account reads.** The profile, dashboard and user detail take the
    account (email, administrator, staff and ban flags) from identity's
    store. Identity's store is current only while identity is
    authoritative, so Control lets them leave legacy mode (shadow or native)
    only then; the identity rollback returns them to legacy with group A.
    They switch on their own: group A's rule and the cutover are unchanged.
  - **Subscriber fields.** Plan, traffic and expiry, and the subscription
    token and proxy uuid that the profile and the detail show, come from
    `KernelIdentity.GetSubscriber` for that one user, never from a view.
    `GetSubscriber` now includes the subscriber's plan, as the v2 detail
    does. The native dashboard is computed on every request; the kernel's
    was cached for 30 seconds.
  - **Resets.** The traffic and subscription resets go through
    `KernelSubscriber.ResetTraffic` and `ResetCredentials`; identity-platform
    now declares `kernel.subscriber.traffic.v1` and
    `kernel.subscriber.credentials.v1`. They change only the subscriber, so
    they switch at any time.
  - **Idempotent resets.** A reset's request id is
    `identity.reset_traffic:<user>:<digest>` or
    `identity.reset_subscribe:<user>:<digest>`, a digest of the request's
    `Idempotency-Key` (else its request id). The kernel's legacy handlers
    derive the same id and record it in the subscriber request ledger, so a
    retry applies once whichever side serves it; a retried subscription
    reset answers the token the first one issued.
  - **Contract.** `KernelSubscriber.ResetCredentials` and
    `AdjustEntitlement` answer `NotFound` for a subscriber that does not
    exist, instead of recording an empty change.
  - **Still bridged.** The administrator's user list and statistics: their
    one query mixes identity's ban flag with the subscriber's expiry, and
    the list shows every user's subscription token. The user's invite code
    list and generation: affiliate data that Control keeps.
  - `internal/tests/identitycompat` proves byte parity and the same Control
    state (counters, tokens, request ledger, change log, revocations) on
    SQLite and PostgreSQL, and that the reads answer identity's account
    rather than Control's projection.
- **Subscription module.** `packages/subscription` has its own host and
  serves 17 of its 25 routes natively: subscription groups and templates,
  a group's node protocol links, a plan's groups, a user's groups, the
  group statistics and the format and protocol lists.
  - It runs on `v2_subscription_group`, `v2_subscription_template`,
    `v2_plan_subscription_group` and `v2_subscription_group_node_protocols`,
    adopted in place. The kernel's subscription renderer and order
    completion keep reading them; the module's writes leave the same rows.
  - Other domains are read through kernel views: `kapi_plan_catalog_v1`,
    `kapi_subscriber_entitlement_v1` and the new
    `kapi_user_subscription_group_v1` (a subscriber's groups and their
    expiry), `kapi_node_protocol_v1` (a protocol's node) and
    `kapi_node_heartbeat_v1` (a node's last report). None shows a token,
    UUID, e-mail address, key or protocol settings.
  - Eight routes stay bridged: deleting a group and granting or removing a
    user's group write `v2_user_subscription_group`, which only the kernel
    writes and `KernelSubscriber` cannot edit yet (an extension is proposed
    in `docs/architecture/subscriber-service.md`); a group's protocols and
    the protocol pool answer node rows with keys; the preview renders a
    user's subscription; the link settings read process configuration; the
    user's summary comes from the kernel's cache.
  - `internal/tests/subscriptioncompat` proves byte parity on SQLite and
    PostgreSQL (104 cases each) and the same groups, templates and links,
    and that the bound types mirror the kernel model. The PostgreSQL run is
    part of CI.
- **Proxy node module.** `packages/proxy-node` has its own host and serves
  7 of its 31 routes natively: the load balancer list, detail, creation,
  update and deletion, the node statistics and a node's runtime logs.
  - It runs on `v2_load_balancer` and `v2_node_log`, adopted in place. The
    node statistics, and whether a node exists, come from the new kernel
    view `kapi_node_status_v1` (each node's id, status, last check and
    traffic counters, no credentials).
  - `v2_node` and `v2_authorized_key` are now protected kernel tables that
    no package may adopt. `v2_node` holds each node's API key, key hash and
    secret, which the kernel's node authentication checks, and
    `v2_authorized_key` the registration keys that mint them; a package
    that could read or write them could act as any node and read every
    subscriber's proxy credentials from UniProxy.
  - The other 24 routes stay bridged, with the reason in the host's route
    map: node CRUD, credentials and raw configuration (node credentials,
    and protocols with Reality private keys in the answers), configuration
    validation (the kernel's WireGuard validator), authorization keys,
    registration, heartbeat and runtime health, the agent WebSocket,
    UniProxy, and the load balancer statistics and health check (forward
    nodes).
  - `internal/tests/proxynodecompat` proves byte parity on SQLite and
    PostgreSQL (56 cases each) and the same load balancer rows. The
    PostgreSQL run is part of CI.
- **Protocol runtime module.** `packages/protocol-runtime` has its own host
  and serves 3 of its 20 routes natively: the protocol templates and the
  administrator's diagnostic task history and detail.
  - It runs on `v2_agent_diagnostic_task`, adopted in place. The agents'
    HTTP poll now checks a pending task against the diagnostic whitelist
    again before handing it out, so a task written outside the kernel's
    checks fails instead.
  - `v2_node_protocol` (Reality and WireGuard server private keys, custom
    configurations, the rows the kernel builds node configurations from
    without validating them again) and `v2_wireguard_peer` (users' WireGuard
    keys) are now protected kernel tables that no package may adopt. The
    protocol routes stay bridged.
  - The other 17 routes stay bridged, with the reason in the host's route
    map: node protocols, Agent Control (the kernel's gRPC control streams),
    the administrator's agent list, monitoring and tasks (the agents' live
    connections in the kernel's memory), and the agent routes (node
    credentials, node status, forward bridge tasks).
  - `internal/tests/protocolruntimecompat` proves byte parity on SQLite and
    PostgreSQL (22 cases each). The PostgreSQL run is part of CI.
- **Machine telemetry module.** `packages/machine-telemetry` has its own host
  and serves 2 of its 5 routes natively: the administrator's hourly traffic
  series and user traffic ranking.
  - It adopts no table: the traffic log comes from the new kernel view
    `kapi_traffic_log_v1` (`user_id`, `u`, `d`, `rate` and `log_at` of
    `v2_server_log`), e-mail addresses from `kapi_user_directory_v1`.
  - The dashboard (users, orders and the online set in the kernel's cache,
    cached itself), the system information (the kernel binary's build
    metadata) and the monitoring WebSocket stay bridged.
  - `internal/tests/machinetelemetrycompat` proves byte parity on SQLite and
    PostgreSQL (42 cases each). The PostgreSQL run is part of CI.
- **Gost mesh module.** `packages/gost-mesh` has its own host and serves 1
  of its 3 routes natively: the administrator's gost API connection test,
  which reads no table. The NodeX runtime status and diagnosis stay bridged:
  they use the NodeX token in the protected `v2_system_config`.
  `internal/tests/gostmeshcompat` proves byte parity (23 cases) against test
  gost APIs that answer, refuse, fail, answer what the client cannot decode,
  or do not listen. The PostgreSQL run is part of CI.
- **WireGuard module.** `packages/wireguard` has its own host and serves its
  one route natively: the administrator's WireGuard server keypair, which
  reads and stores nothing. `internal/tests/wireguardcompat` proves the same
  answer with the random keys masked, and that both sides answer a fresh
  X25519 pair. The PostgreSQL run is part of CI.

- The kernel side of the identity module, `KernelIdentity` (N9). It is served
  on the local package bridge and on the module listener.
  - **Capability.** A new `kernel.identity.v1` capability. Only the current
    generation of an official package whose signed release declares it may
    call, checked on every call; identity-platform declares it.
  - **Subscribers.** `CreateSubscriber` is idempotent on the account UUID and
    allocates the `v2_user` row, whose password never matches.
    `UpdateSubscriber` takes the v2 admin entitlement fields, and a new plan
    fills group, traffic and limits as the admin API does. `DeleteSubscriber`
    also revokes the user's tokens.
  - **Projection.** `ApplyAccountProjection` is versioned and monotonic. A
    ban or demotion revokes at once. Password and TOTP are mirrored back
    until finalize.
  - **Other calls.** `PublishRevocation` feeds the revocation store.
    `ResolveActorAccess` returns the v2 permission fields.
    `GetIdentitySettings` returns the registration policy, rate limits, admin
    MFA configuration, token lifetime and authority state.
  - **Tables.** New tables `v4_kernel_identity_account` (account links and
    projection versions) and `v4_kernel_identity_authority` (authority state
    and import checkpoint).
  - **SDK.** `packagebridgesdk.Client.Conn()` lets a host call further kernel
    contracts over its bridge connection.
  - The v2 admin user update now shares its entitlement logic
    (`service.SubscriberEntitlements`) with `UpdateSubscriber`.
- Deployment of network modules (N6).
  - **`Dockerfile.module`.** One image per package, built from the
    package's own host or the generic host. Distroless, uid 65532, about
    15 MB, remote mode by default.
    - `go_dev` publishes `ghcr.io/anixops/anix-module-identity-platform:edge`
      and `:sha-<commit>` for amd64 and arm64, signed with cosign.
  - **Compose.** The `docker-compose.modules.yml` overlay runs
    identity-platform as its own container next to Control; the guide is
    `config/deploy/compose/modules.md`.
  - **Helm (chart 0.2.0).**
    - `moduleRuntime` turns on Control's mTLS module listener: port 7443 on
      the Deployment and Service, and the CA key from the Secret.
    - `modules.<id>` adds one Deployment per module, with an optional
      NetworkPolicy.
    - Rendering fails without a CA key or with modules but no module
      runtime.
  - **CLI.** `anix-control module runtime list|set` selects a package's
    runtime (`local` or `remote`).
  - **CI smoke tests.** Both build the module image and log in through
    identity-platform running as a separate service:
    `modules_compose_smoke.sh` (production Compose files) and
    `modules_kind_smoke.sh` (the chart on kind, two replicas, NetworkPolicy
    on, pod replacement).
- Identity token contract and identity core module (N10a).
  - **`sdk/identitytoken`.** Verifies identity access tokens for any AnixOps
    service.
    - It checks the EdDSA signature against a published, unrevoked `kid`,
      and requires the issuer, audience and the required claims (`sub` equal
      to `user_id`, `sid`, `tv`, `jti`), with 60 s of leeway.
    - It includes the JWKS types.
    - Tests run the `contracts/identity/v1` golden and negative fixtures.
  - **`identity/`.** A new Go module, `github.com/AnixOps/anix-control/identity`:
    the product-neutral identity core, which depends only on the SDK
    (boundary gate).
    - **Signing keys:** Ed25519 keys sealed under a KEK, with a
      NEXT → ACTIVE → RETIRED rotation policy and revocation.
    - **Tokens:** per-audience token issuing and a JWKS rendering.
    - **Discovery:** the OpenID discovery document.
  - CI and the local gates tidy, vet, test, race-test, lint, scan and
    vulnerability-check the new module.
- Identity tokens are revoked by token version.
  - **What changed.** A session-ending change identity projects (ban,
    demotion, password or email change) now carries the account's new token
    version (`ApplyAccountProjectionRequest.token_version`, new). The kernel
    ends identity tokens with a lower `tv` claim. Before, it ended every token
    issued up to that second, so a user who logged in again within the same
    second got a dead token.
  - **Kernel tokens.** HS256 tokens carry no `tv` and are still ended by time.
    A token-version revocation no longer ends every future HS256 token of the
    user, which mattered after a rollback.
  - **Guard.** A version that did not rise above the last one projected falls
    back to time, so a revocation cannot be missed.
  - **Schema.** New columns `v4_kernel_identity_revocation.identity_not_before`
    and `v4_kernel_identity_account.token_version`.
  - **Acceptance.** The identity acceptance logs a demoted administrator in
    again at once.
- Identity acceptance on Compose and kind (N13).
  - **Shared flow.** The cutover acceptance is now
    `config/scripts/identity_cutover_acceptance.sh`: import, cutover, native
    login, ban and demotion revoking within 5 seconds, logout, rollback,
    second cutover and finalize.
  - **Kind.** The kind modules smoke runs it against two identity replicas.
  - **Compose.** The Compose modules smoke runs it, then stops the identity
    module: new logins fail while issued identity tokens still verify from
    the keys Control persisted.
- Identity cutover, rollback and finalize (N12b, `internal/identitycutover`).
  Each is recorded in the new table `v4_kernel_identity_cutover`.
  - **Cutover.** `POST /api/v4/kernel/identity/cutover` runs in the
    background (`GET /api/v4/kernel/identity` follows it).
    - It catches identity up with a delta import, then pauses group A: the
      v2 gateway answers 503 with `Retry-After` and lets dispatched requests
      finish.
    - It imports the last changes, then switches the authority to
      `identity` and group A to native in one transaction.
    - It waits until the identity host serves group A natively, and
      switches both back if the host does not within 30 seconds.
  - **Rollback.** `POST /api/v4/kernel/identity/rollback` returns group A to
    the legacy handlers until finalize.
  - **Finalize.** `POST /api/v4/kernel/identity/finalize`, a day after the
    cutover or with `{"force": true}`:
    - makes legacy passwords unusable and empties `v2_user_mfa`;
    - stops the mirror;
    - makes Control refuse HS256 tokens, also after restarts.
  - **Consistency.** Configuration writes that would make group A partly
    native, native before the cutover, or legacy while identity is
    authoritative are refused.
  - **Complete deltas.** Delta imports now also carry legacy MFA changes
    (disabling MFA touches the user). Every import deletes the identity
    accounts of deleted subscribers (new
    `ImportAccountsRequest.deleted_user_id`).
  - **Bootstrap.** No default administrator is created once identity is
    authoritative.
  - **Acceptance.** The Compose modules smoke now drives the whole move
    against the remote identity module:
    - import, cutover and an EdDSA login;
    - a ban that revokes within 5 seconds, and a logout that ends one
      session;
    - a rollback, after which a natively created user logs in through the
      legacy handler;
    - a second cutover and finalize, after which HS256 tokens are refused.
  - **Runbook.** `docs/UPGRADE.md` gains "Moving Logins To The Identity
    Module" (prerequisites, cutover, rollback window, finalize).
  - **Package migrations** gain the `__PKG_NAME_PREFIX__` token everywhere
    identity's migrations are applied in tests.
- Rollback-safe native identity routes, and logout (N12a).
  - **Legacy mirror.** Until finalize, every native change the legacy routes
    read is mirrored into `v2_user` and `v2_user_mfa`, so switching group A
    back to legacy keeps logins working:
    - the password hash of new registrations and admin-created users, through
      the new `CreateSubscriber.legacy_mirror` field (applied with the
      subscriber, revoking nothing);
    - TOTP setup, enable and disable.
  - **Rollback test.** `internal/tests/identitycompat` registers and enables
    MFA natively, then logs in through the legacy handler.
  - **Logout.** `POST /api/v4/identity/logout` ends the calling session (its
    `sid`) at once and until the token would have expired. Other sessions of
    the user continue. HS256 and EdDSA tokens alike.
- Native user administration in identity-platform (N11c): create, update,
  ban, unban and delete, serving once their mode is native.
  - **Projection.** Identity changes the account and projects email, admin,
    staff and ban flags to Control, which revokes sessions exactly as v2
    does.
  - **Password changes** also mirror the hash and MFA state to the legacy
    columns.
  - **Entitlements** go through `UpdateSubscriber`.
  - **Contract.** New `KernelIdentity.GetSubscriber` returns the v2 user
    object for admin answers. `CreateSubscriber` gains `is_admin` and
    `is_staff`. Control keeps emails as identity sends them: registration
    lowercases, v2 administration stores them as given.
  - **Parity.** `internal/tests/identitycompat` compares 27 more cases on
    SQLite and PostgreSQL, the responses and Control's resulting
    subscribers, revocations and MFA rows. Generated uuid, token and times
    are masked.
- Native MFA routes in identity-platform (N11b): the six user MFA routes
  (status, TOTP setup and enable, disable, verify, backup code regeneration)
  and the admin MFA configuration. Like login, they serve once their mode is
  native.
  - **Storage.** TOTP secrets are sealed and backup codes kept as keyed
    hashes in identity (`identity/account` MFA operations).
  - **Admin configuration.** It lives in identity's settings document with
    the v2 defaults, value coercions and normalization; login applies it too.
  - **Parity.** `internal/tests/identitycompat` compares 26 more cases with
    the v2 handlers on SQLite and PostgreSQL. Masked: generated secrets and
    codes, and `last_used`, which identity keeps to the second.
- Native login and registration in identity-platform (N11a). They serve once
  their route mode is native, which the identity cutover sets; until then
  both routes stay legacy.
  - **Parity.** Responses equal the v2 handlers byte for byte, except the
    token: messages, key order, `Retry-After`, the MFA challenge and
    enrollment answers, and the permission fields.
    `internal/tests/identitycompat` checks 31 cases on SQLite and
    PostgreSQL, the latter in the CI PostgreSQL job.
  - **Login.** Identity checks accounts, bcrypt passwords, bans and MFA
    (TOTP with one step of skew; backup codes consumed on use). Expiry is
    read from `kapi_user_directory_v1`. The token is EdDSA for
    `aud=anix-control`.
  - **Registration.** It creates the subscriber through `CreateSubscriber`.
    Its new `invite_code` field is validated and consumed with the
    subscriber in one kernel transaction, with the v2 messages.
  - **Identity core.** It gains `identity/throttle` (attempt limits shared by
    replicas) and `identity/settings`. The account store gains lookup,
    creation, MFA verification and login attempts (migration `004_login`).
  - identity-platform also declares `kernel.view:kapi_user_directory_v1`.
  - **Harness.** `internal/tests/packagecompat` can mask values that differ
    by design, require headers, send warm-up requests, and passes the client
    IP and user agent to native routes.
- Kernel-led account import into the identity module (N10c).
  - **Identity side.** The core gains `identity/account`, which stores
    accounts, MFA and import runs in identity's own tables (migration
    `003_accounts`).
    - TOTP seeds are sealed and backup codes kept as KEK-keyed hashes
      (`identity/secretbox`).
    - Versions rise only on real changes, so a repeated batch changes
      nothing.
    - identity-platform serves `ImportAccounts`, `BatchGetAccounts` and
      `ExportAccounts` when a KEK is configured.
  - **Kernel side.** `internal/identityimport` copies the `v2_user` identity
    columns and `v2_user_mfa` in checkpointed batches.
    - An interrupted import resumes. A delta re-sends what changed since the
      previous import started.
    - Legacy users are linked in `v4_kernel_identity_account` under
      deterministic account UUIDs.
    - The authority state becomes `importing`. Imports are refused once
      identity is authoritative.
    - Invite codes stay with Control.
  - **Admin API.** `GET /api/v4/kernel/identity` (state and progress) and
    `POST /api/v4/kernel/identity/import` (`{"delta": bool}`, one at a time).
  - **Contract.** New, additive fields: `ImportedMFA.last_used_unix` and
    `last_method`.
  - **Package migrations.** New `__PKG_NAME_PREFIX__` token (and
    `Store.NamePrefix`, `Store.ExpandScript` in `packagestoresdk`) for index
    and constraint names, which PostgreSQL cannot schema-qualify. It expands
    to nothing on PostgreSQL and to the table prefix on SQLite. A PostgreSQL
    test applies identity-platform's migrations in a real package schema.
- Control verifies identity tokens (N10b, kernel side).
  - **Key refresh.** `internal/identitykeys` pulls the identity module's
    token keys with `IdentityService.GetTokenKeys`. It refreshes every 5
    minutes, and at once (rate limited) when a token names an unknown `kid`.
    - Keys are persisted in the new table `v4_kernel_identity_token_key`, so
      tokens keep verifying while identity is down.
    - Refused: keys with another issuer or audience, and malformed keys.
    - Keys identity no longer lists are dropped; revoked keys are kept so
      their tokens fail.
  - **Verification.** `internal/authn` accepts EdDSA tokens (issuer
    `anixops-identity`, audience `anix-control`) beside the kernel's HS256
    tokens, for the HTTP middleware, the monitor WebSocket and the gRPC
    interceptor. The same revocation store checks both kinds (token version,
    session, not-before).
  - **JWKS.** `GET /api/v4/identity/jwks.json` (public, cached 5 minutes)
    publishes the accepted keys.
  - **Host connection.** `pluginhost.Supervisor.PackageConn` reaches a
    running package host, local or remote, for contracts it serves beside
    the host protocol.
- Identity signing keys in the identity module (N10b, host side).
  - **Storage.** identity-platform now declares `kernel.storage.v1`. Its
    embedded migration index adds `002_signing_keys`, which the kernel ledger
    runs when the host starts.
  - **Keys.** The host keeps its Ed25519 signing keys in its own storage
    (`identity/keystore`).
    - They are sealed under `ANIX_IDENTITY_KEK` or `ANIX_IDENTITY_KEK_FILE`.
    - Rotation runs in the host every minute. It takes a PostgreSQL advisory
      lock, so replicas never race.
    - Without a KEK the host publishes no keys and signs nothing.
  - **Service.** The host serves `IdentityService.GetTokenKeys` next to the
    host protocol: live keys with their states and ends, issuer
    `anixops-identity`, audience `anix-control`. `modulesdk` gains a
    `ServiceRegistrant` hook for this.
  - **Local hosts.** A new `identity.kek` setting (`ANIX_CONTROL_IDENTITY_KEK`,
    secret) is passed only to the local identity-platform host, through the
    supervisor's per-package environment.
  - **Deployment.**
    - Compose: `docker-compose.modules.yml` requires a new `identity_kek`
      secret.
    - Helm: `modules.<id>.extraEnv` (chart 0.2.1) carries it, for example
      from a Secret.
    - Both module smokes set it.
  - The kernel module now requires the identity module (`replace => ./identity`).
  - **Upgrade note:** from this version identity-platform needs package
    storage. On PostgreSQL the kernel's database role must be allowed to
    create the package role (`CREATEROLE`, as for other storage packages);
    without it the identity host fails to start after the upgrade.
- Module SDK for network modules (N5).
  - **`pkg/modulesdk.Run`.** Runs a host as the kernel's local child process
    or, with `ANIX_MODULE_MODE=remote`, as a network module.
    - Enrolls from a credential file, or uses externally issued
      certificates.
    - Stores and renews its certificate (0600 files) and reuses it after a
      restart.
    - Serves the host protocol over mTLS, accepting only the kernel.
    - Binds, heartbeats and rebinds.
    - Answers `/livez` and `/readyz`.
    - Drains gracefully on SIGTERM.
    - The official generic and identity-platform hosts, and the storage test
      fixture, now start through it; local behaviour is unchanged.
  - **`packagebridgesdk.NetworkClient`.** The remote package bridge.
    - Session tokens travel on every call.
    - A call rejected for an unknown session binds again and retries once.
    - Concurrent rebinds collapse into one.
  - **`Resume`.** `pluginhostsdk.ResumablePackage` and `Router.Resume` let a
    drained network module serve again after it binds to a new generation.
  - **`packagestoresdk`.** PostgreSQL pools fetch the current storage lease
    before every new connection, so rotated role passwords heal without
    `28P01` failures.
  - **Trust bundle.** The kernel exports the module CA bundle for
    deployments: `GET /api/v4/kernel/modules/trust-bundle` and
    `anix-control module ca bundle`.

- Remote runtime for Control packages (N4).
  - **Selecting the runtime.** A package can run from network module
    instances instead of a local child process.
    - Set it with `PUT /api/v4/kernel/modules/runtimes/:plugin_id`
      (table `v4_kernel_plugin_runtime`).
    - Remote requires `module_runtime.enabled` and PostgreSQL.
    - A change applies at the package's next lifecycle operation.
  - **Instance pool.** Remote hosts live in the same host supervisor as
    local ones.
    - Calls go round-robin over the instances bound to the generation.
    - An instance that fails at the transport is ejected.
    - Health is cached for 2s, so per-frame WebSocket checks stay local.
    - Dispatch, WebSocket relays, ledger migrations and fencing use the same
      code as local hosts.
  - **Starting a remote generation.** It waits
    (`module_runtime.bind_timeout`, default 2m) for a bound, healthy
    instance.
    - A new version keeps the old generation serving until then.
    - The same version fences the old generation first so its instances
      rebind.
  - **Storage.** Replicas of one generation share one storage lease instead
    of rotating each other's password. `module_runtime.database_host` sets
    the database address in remote leases. Remote packages cannot lease
    SQLite storage.
  - **Startup order.** The module listener and remote runtime start before
    restart reconciliation, so remote packages never start locally by
    mistake.

- Kernel module listener and remote bridge sessions (N3).
  - **Listener.** With `module_runtime.enabled`, the kernel serves `ModulePKI`
    and the package bridge on an mTLS listener (`module_runtime.listen`,
    default `:7443`).
    - TLS 1.3 only.
    - Every RPC except `Enroll` needs a verified, unrevoked module
      certificate.
    - Revocations made by the kernel take effect on the next call.
  - **Bind and sessions.** Remote instances call `Bind` and get a session
    token for their package's current generation.
    - Later calls must carry the token from the same module identity.
    - Heartbeats keep a session for 15 s and report fencing and draining.
  - **Generation sessions.** Capabilities now live in a per-generation
    `GenerationSession`, so any instance of a generation can redeem them,
    once.
    - Local hosts keep their private socketpair on top of the same state.
    - Remote installations are admitted once the remote runtime manager
      lands; until then `Bind` answers `FailedPrecondition`.
  - **Maintenance.** The kernel promotes rotated module CAs and prunes expired
    certificate records every hour.

- Built-in module PKI for network modules (N2).
  - **CA.** `internal/modulepki` is an ECDSA P-256 CA.
    - Its key is sealed under `module_runtime.ca_kek`.
    - A name constraint limits it to `spiffe://anixops/...` identities.
    - It is created at startup when `module_runtime.enabled` is set.
    - Rotation keeps the next and retired CAs trusted while their
      certificates are valid.
  - **Certificates.** Short-lived, 24h by default, with exactly one SPIFFE
    URI SAN; requested subjects and SANs are ignored. Renewal runs over mTLS.
  - **Enrollment credentials.** Hashed, one-time or reusable, consumed
    atomically. Revoking one revokes the certificates issued through it.
  - **Administration.** `POST/GET/DELETE /api/v4/kernel/modules/enrollment-tokens`,
    `POST /api/v4/kernel/modules/ca/rotate`, and the CLI
    `anix-control module token|ca ...`.
  - **External CA.** `module_runtime.pki: external` loads a trust bundle and
    kernel certificate (for example from cert-manager) and reloads them on
    change.
  - **`pkg/moduletls`** holds the SPIFFE identity rules and TLS 1.3
    configurations shared by the kernel and modules. Peers are verified by
    identity on every handshake, including resumed ones.
  - **New tables:** `v4_kernel_service_ca`, `v4_kernel_module_enrollment`,
    `v4_kernel_module_certificate`.
  - **New settings:** `module_runtime.*` (`ANIX_CONTROL_MODULE_RUNTIME_*`),
    off by default.

- Contracts for network modules and the identity service (N1).
  - **New protocols.**
    - `api/modulepki/v1` `ModulePKI`: `Enroll`, `Renew`, `GetTrustBundle`.
    - `api/identity/v1` `IdentityService`: `GetTokenKeys`,
      `BatchGetAccounts`, `ImportAccounts`, `ExportAccounts`.
    - `api/kernelidentity/v1` `KernelIdentity`: subscriber allocation,
      account projection, revocation, actor access, settings.
  - **New RPCs on existing protocols.** `Bind` and `Heartbeat` on
    `KernelPackageBridge`, and `Resume` on `ControlPackageHost`. Existing
    hosts answer them with `Unimplemented`, so v4.0.0 packages are
    unaffected.
  - **Compatibility gate.** `internal/tests/protocompat` checks every
    AnixOps protobuf contract against `contracts/proto/descriptors.golden`.
    Removing or renumbering a method, field or enum value fails; additions
    need an explicit `-update`.
  - **Token contract.** `contracts/identity/v1` fixes the EdDSA access-token
    claims and the tokens the kernel must reject.
  - **CI.** The protobuf drift check covers the new generators.
  - **Design documents.** `docs/architecture/module-runtime.md` and
    `docs/architecture/identity-service.md`. `package-extraction.md` no
    longer keeps identity in the kernel: login and credentials become the
    first network module.

- Extraction gates and a legacy/native comparison harness (M3).
  - **Extraction map.** `config/package-extraction.json` records every
    `/api/v2` route's extraction mode (`bridged`, `native-flagged`,
    `native`) and where its legacy handler lives. All 292 routes are
    `bridged`.
  - **Route gate.** `check_plugin_only_routes.py` enforces the map:
    - a `native` route binds the bare gateway and has no legacy handler left;
    - `native-flagged` and `native` routes need a package host that
      implements them;
    - the identity-platform exception, formerly hardcoded, is now the map's
      `identity-bridge` source, checked against `internal/identitybridge`.
  - **Inventory.** The route inventory reports each route's
    `legacy_handler`.
  - **Worker gate.** `check_plugin_only_workers.py` fails when `cmd/server`
    starts a new business worker and keeps the list of the seven legacy
    domain workers shrink-only.
  - **Boundary gate.** `check_package_boundaries.sh` fails when
    `packages/...` or `pkg/...` depend on `internal/`. Test imports need a
    reasoned allowlist entry.
  - **CI.** All three gates and their tests run in the required Go Quality
    Gates job.
  - **Harness.** `internal/tests/packagecompat` (`RunRead`, `RunWrite`) runs
    a route's legacy handler and its native implementation on identically
    seeded databases. It compares status codes, bodies normalized with
    `v2compat`, and, for writes, the database state. It uses SQLite, and
    also PostgreSQL schemas when `ANIX_TEST_POSTGRES_DSN` is set.

- Package migrations run through the migration ledger when a storage
  package's host starts (M3).
  - **Scope.** Applies to releases that declare `kernel.storage.v1`. After
    `plugin.install`, `enable`, `update` or `rollback` starts the host, the
    dispatcher runs the release's migration index before the operation
    succeeds.
  - **Ledger.** There is one run per lifecycle generation, with id
    `index.<index digest>`. Upgrades record an `operator-managed:` backup
    reference.
  - **Host side.** The host applies its embedded index through
    `packagestoresdk.IndexMigrator`, the new
    `pluginhostsdk.RouterConfig.IndexMigration` hook, and reports
    `packagestoresdk.StepsDigest`.
  - **Check.** The kernel requires that digest to match the verified index
    of the artifact.
  - **Failures and restarts.** A failed migration stops the host and fails
    the operation. A Control restart confirms the recorded run instead of
    migrating again.
  - **Other packages.** Packages without the capability, including all
    v4.0.0 packages and identity-platform, are unaffected.
  - **Test fixture.** `internal/tests/packagefixture/storagehost` is a real
    host binary used by the end-to-end tests on SQLite and PostgreSQL.

- Per-package database storage and the `LeaseStorage` bridge RPC (M3).
  - **Roles.** On PostgreSQL, `internal/packagestore` gives every package a
    login role `anix_pkg_<id>` (NOINHERIT, 4 connections, member of the
    privilege-less group `anix_packages`) and a kernel-owned schema
    `pkg_<id>` in which the role creates its own tables.
  - **Grants.** Grants come only from the signed manifest:
    `kernel.storage.adopt:<table>` grants DML on the table and its sequences,
    and `kernel.view:<view>` grants `SELECT`. Each lease replaces the
    previous grants, so a dropped capability is revoked.
  - **Leases.** Hosts call the new session-scoped RPC `LeaseStorage`.
    - It is fenced like `GetPackageConfig`.
    - It returns a connection string for the package role only. Each lease
      rotates the password through a SCRAM verifier computed by the kernel.
    - The kernel's database role needs `CREATEROLE`; without it leases fail
      with an explicit error.
    - SQLite shares the kernel's file with a `pkg_<id>_` table prefix and
      provides no isolation.
  - **Ledger.** The new kernel table `v4_kernel_package_storage` records each
    package's role, schema, grants and lease generation.
  - **View.** The kernel API view `kapi_user_directory_v1` exposes
    non-secret `v2_user` columns. It is created at startup; a failure is
    logged and does not stop Control.
  - **SDK.** `packagebridgesdk.Client.LeaseStorage` and
    `pkg/packagestoresdk` (`Open`, `SharedOpener`, `Store.Table`,
    `RunEmbeddedMigrations` with per-step digests and a `schema_migrations`
    state table).
  - **CI.** The new job `package-storage-postgres` runs the grant tests as a
    non-superuser `CREATEROLE` role. It also runs the PostgreSQL package
    rollout tests, which no job ran before.

- A per-route package router in the host SDK (M3).
  - **Router.** `pkg/pluginhostsdk.Router` polls `GetPackageConfig` every
    5 s and serves each route in `legacy`, `shadow` or `native` mode.
    - A failed poll keeps the last modes.
    - Routes without a native handler stay `legacy` and are reported as
      `mode_unsupported`.
    - `shadow` returns the legacy response, then compares the native one in
      the background with a timeout and a concurrency limit.
    - WebSocket routes always relay through the bridge.
  - **v2compat.** `pkg/v2compat` holds the v2 panel envelope
    (`PanelSuccess`, `PanelError`) and `NormalizeForCompare`. The kernel's
    panel helpers now use it, and their output is byte-identical.
  - **Health and metrics.** The supervisor calls `Health` on running hosts
    every 30 s and keeps the returned details. `/metrics` exports them as
    `anixops_package_config_status`, `anixops_package_route_mode` and the
    `anixops_package_native_*` and `anixops_package_shadow_*` counters.

- Package route modes and the `GetPackageConfig` package bridge RPC (M3).
  - **Route modes.** Control package configuration documents reserve a
    top-level `routes` key that maps v2 route ids to `legacy`, `shadow` or
    `native`.
  - **Validation.** The kernel checks the modes against the release's
    verified compatibility routes: `shadow` only on GET routes, and WebSocket
    routes stay `legacy`. It then removes the key before applying the
    package's own schema.
  - **Reading the config.** Hosts read their configuration with the new
    session-scoped bridge RPC `GetPackageConfig`.
    - It needs no per-request capability: it is authorized by the session's
      package id, version and lifecycle generation.
    - Disabled packages and stale versions or generations are fenced.
  - **SDK.** `packagebridgesdk.Client.GetPackageConfig` reports an older
    kernel as `ErrSessionOperationUnsupported`, which means every route stays
    `legacy`.

- A Helm chart for Kubernetes (`config/deploy/helm/anix-control`), single
  replica with an external PostgreSQL.
  - `Recreate` strategy. `replicaCount` other than 1 is rejected until
    multi-replica support lands.
  - A `migrate` init container. It is used instead of a Helm hook so it
    always sees the release's Secret and ConfigMap.
  - Probes: startup and liveness on `/livez`, readiness on `/readyz`.
  - Settings come from a ConfigMap of `ANIX_CONTROL_*` variables. Secrets,
    chart-created or `existingSecret`, are mounted as `_FILE` files. Config
    and secret checksums roll the pod.
  - Security: non-root uid 10001, read-only root filesystem, all
    capabilities dropped, `RuntimeDefault` seccomp, exec `emptyDir` `/tmp`.
  - Optional: gRPC Service for nodes, Ingress, `ServiceMonitor`, and an
    ansible inventory/SSH Secret.
  - CI: `Helm Chart Checks` (strict lint, kubeconform on several value sets,
    guard failures) and `Kubernetes Smoke`.
  - `.gitignore` now ignores only the root `/anix-control` binary. The bare
    `anix-control` pattern also matched any directory of that name, including
    the chart. The smoke installs the freshly
    built image into kind with an in-cluster PostgreSQL, probes it, upgrades
    it and uninstalls it.
- Database leases (`internal/lease`, table `v4_kernel_lease`) keep single-instance
  background work in one Control process per database. The following run only
  in the process holding the `control.singleton-workers` lease:
  - the forward runtime job executor and forward agent bridge worker;
  - the flow and monthly resets;
  - the gost and ansible stats workers;
  - the latency prober.

  The lease lasts 30 s and is renewed every 10 s. A leader that cannot renew
  stops its workers after 20 s, and a leader that shuts down releases the
  lease, so a rolling update or an accidental second process no longer
  double-runs jobs, resets or traffic accounting. `/metrics` reports
  `anixops_lease_leader`. `docs/architecture/container-deployment.md` records
  the container design and the remaining multi-replica blockers.

- `anix-control migrate` prepares the database, then exits: schema, `Ensure*`
  helpers, plugin trust root, identity package bootstrap, and default seeds.
  Use it as a one-shot Compose service or Kubernetes Job.
- Database preparation now runs under a PostgreSQL advisory lock, in both the
  server and `migrate`. Several processes starting at once run it one at a
  time instead of racing on DDL and seed rows.
- Production databases get their tables without `env: development`.
  - An empty database gets the full schema.
  - An existing database only gets the tables it lacks; existing tables,
    columns and indexes are never altered.
  - Previously production mode skipped table creation entirely, so a fresh
    production install needed `env: development` for its first start.
  - Verified on a copy of the production database: only the five
    `v4_kernel_package_*` tables were added.
- A configured `plugins.control_host_artifact_dir` is cleared of package
  copies left by earlier processes at startup.

- Container probes on both the API and UI servers:
  - `/livez`: the process answers.
  - `/readyz`: startup finished, not draining, and the database answers a
    ping.
  - `/health` keeps its response shape. The API server's copy now also
    returns `503 {"status":"draining"}` during shutdown, like the UI
    server's.
- `server.shutdown_drain_delay`: time to keep serving after readiness starts
  failing on shutdown, so load balancers and Kubernetes endpoints stop routing
  first. The default is 0; the built-in container defaults use `5s`.
- `log.format: json`: one JSON object per line on stdout for standard log
  lines, slog records, HTTP access logs (path without query string) and GORM.
  `log.level` filters structured records; standard log lines are always
  written. `text` (the default) keeps the historical output.
- When the frontend directory is missing and cannot be created (for example on
  a read-only root filesystem), the server now logs a warning instead of
  failing to start.

- Configuration can now come from the environment, for container deployments.
  - Every scalar or list key has an `ANIX_CONTROL_<PATH>` variable, for
    example `ANIX_CONTROL_DATABASE_PASSWORD` or `ANIX_CONTROL_JWT_SECRET`.
  - Each variable also has a `_FILE` variant that reads the value from a
    mounted secret file.
  - Variables are applied on top of the config file.
  - Unknown `ANIX_CONTROL_*` names are logged.
  - `anix-control -print-env` lists every variable with its default.
- The server can start without a config file.
  - When neither `-config` nor `ANIX_CONTROL_CONFIG` is given and
    `config/config.yaml` does not exist, the server uses built-in,
    production-shaped defaults (`internal/config/defaults.yaml`).
  - A config file named explicitly must still exist.
  - In production, startup refuses an empty or template `jwt.secret`.
- New PostgreSQL connection settings:
  - `database.sslmode` (default `disable`) and `database.timezone` (default
    `Asia/Shanghai`), so managed databases that require TLS can be used;
  - `database.dsn`, a full connection string that is used verbatim.
- `env: production` now defaults `server.mode` to `release`; gin previously
  fell back to debug mode.

- Added `plugins.control_host_max_request_bytes` and
  `plugins.control_host_max_response_bytes` (default 1 MiB, maximum 64 MiB)
  to configure the package request and response body limits; see
  `docs/reference/configuration.md`. Hosts receive the response limit as
  `ANIX_CONTROL_HOST_MAX_RESPONSE_BYTES` (`pluginhostsdk.MaxResponseBytesFromEnvironment`),
  and gRPC message sizes on the kernel, bridge and host follow the limits.
  `pluginhostsdk.HostServerOptions()` is the new recommended host server
  option set.
- Added v2 gateway metrics to `/metrics`:
  `anixops_v2_gateway_requests_total{package,route,code_class}`,
  `anixops_v2_gateway_errors_total{package,route,code}` and the
  `anixops_v2_gateway_request_duration_seconds{package}` histogram, labelled by
  the declared package route id (never the raw path), plus the package host
  supervision counters from `Supervisor.Stats()`:
  `anixops_plugin_host_{starts,unexpected_exits,restarts,failures}_total{package}`
  and the `anixops_plugin_host_state{package,state}` gauge.
- Added `docs/architecture/package-extraction.md`, the design of record for
  moving business domains into packages: the current routing-only reality,
  the definition of done, the planned storage-lease / per-route-mode / typed
  kernel operation mechanism, the preserved constraints, invariants, domain
  data contracts, rollout and gate designs from the retired
  `docs/superpowers/` plans, the platform gaps, and milestones M0-M4 with a
  `knowledge` pilot. `ROADMAP.md`, `AGENTS.md`, `TODO.md`, and
  `docs/features.md` point to it.
- Added the operator runbook `docs/guide/release-root-rotation.md`, extracted
  from the retired root-rotation plan and checked against the current code
  (single active root, retired-root releases stop verifying, re-signed
  packages need a new version).

- Exposed `X-AnixOps-Operation-Chain` consistently for Control plugin
  installation upserts, configuration writes, and lifecycle actions, including
  dependency apply plans while excluding rollback operations.
- Added the compatible Control Center `/plugins` lifecycle slice: a direct
  authenticated `/api/v3` client, official catalog and release discovery,
  Control/Agent installation state, revisioned configuration editing,
  idempotent lifecycle actions, operation-chain display, health/failure
  summaries, and focused store/view/browser coverage. Its Control administrator
  session uses `/api/v2/login` and a separate token; the legacy Workers
  `/api/v1` client and login remain independent. The Center web build now
  processes Tailwind styles and exposes mobile navigation for the plugin page;
  transient lifecycle retries reuse their idempotency key, and unverified
  catalog entries cannot be installed from the UI. A dedicated Control Center
  browser gate now drives a temporary real Control process through Control
  login and authenticated catalog/installation reads.
- Merged the Center plugin operation-history experience into the canonical
  Control administrator plugin page. It now shows recent plugin operations,
  supports cancellation, and reuses lifecycle idempotency keys after transient
  Control failures.
- Established the formal product-stage contract from `3.1` through `4.0`,
  separate from the Go module `/v4` import path. The earlier `v3.1.0-alpha.2`
  candidate remains historical planning context; the active worktree follows
  the `4.0.x` RC roadmap and its signed package scope is tracked separately.
- Added an operational `/admin/access-groups` surface, server-side
  effective-access preview, and an identity-only group detail response.
- Added a real Control-to-Chromium signed WebUI E2E gate that proves package
  registration, artifact upload, enable, catalog/asset/menu/route loading, and
  durable disable revocation.
- Added a default-off Agent Supervisor canary to the node deployment flow,
  including secure gRPC endpoint checks, canonical Ed25519 public-key checks,
  and Ansible group-variable/template propagation.

### Fixed

- Moving a forward to another tunnel now moves it.
  `POST /api/v2/forward/update` and `POST /api/v2/admin/forward/update`
  saved the forward with the tunnel it was loaded with, and GORM set
  `tunnel_id` back to that tunnel. The answer and the row kept the old
  tunnel, and the forward was applied there again, while its port bindings
  moved to the new tunnel's node; the old node's port was then free for
  another forward. The forward is now saved without its associations.
- Deleting a node protocol, or a node, removes the protocols' subscription
  group links. On PostgreSQL deleting a linked protocol or its node failed on
  the foreign key; on SQLite the links were left behind.
- Deleting a subscription group removes its node protocol links. On
  PostgreSQL deleting a group with links failed on the foreign key; on SQLite
  the links were left behind.
- `kapi_subscriber_entitlement_v1` is created again. It named no source
  table, so the check that skips a view whose source table does not exist
  left it out. A test now requires every view to name its source.
- The subscriber request ledger (`v4_kernel_subscriber_request`) is now pruned
  after 90 days, as the subscriber contract documents, by the same hourly
  worker as the change log. Before, it was never pruned.
- CI's fast lane runs the PostgreSQL parity tests when a legacy handler
  (`internal/handler`), a package's native code (`packages/*/native`),
  `internal/kernelsubscriber` or `sdk/v2compat` changes. Before, those
  changes ran the parity tests on SQLite only.
- Network modules no longer crash-loop when they start while Control is
  unreachable (for example during a rollout restart). `modulesdk` retries
  enrollment with backoff while the kernel is unavailable; a refused
  credential still fails at once.
- The module smoke scripts no longer fail with SIGPIPE (exit 141) when a log
  check matches early: `grep -q` now reads captured output instead of a pipe.
- The web frontend depends on axios 1.20.0. axios up to 1.19.0 is affected
  by newly published advisories (prototype-pollution gadgets, ReDoS, HTTP/2
  proxy bypass and DoS), which made `npm audit` fail the Frontend Build
  check.

- Configuration changes to a Control package no longer fail. Its `plugin.configure`
  operation completes immediately, because hosts pull the configuration. Before
  this, the host lifecycle dispatcher rejected the operation as unsupported.

- `docker-compose.prod.yml` keeps verified package copies on a disk-backed
  `plugin-artifacts` volume (`/var/lib/anixops`, pre-created in the image for
  uid 10001) instead of the 512 MB RAM-backed `/tmp` tmpfs. With the sixteen
  official packages (~0.5 GB of copies) the tmpfs filled up and package hosts
  failed with "verified artifact reference is unavailable"; found by the
  container cutover rehearsal on a production database copy. The Helm chart's
  disk-backed `/tmp` limit is raised from 1 Gi to 2 Gi.

- `TestElectorStopsWorkWhenRenewalsFail` waits for the leader flag to clear
  instead of reading it the moment the work returns; it failed under the race
  detector.

- The edge and release image jobs pin `sigstore/cosign-installer@v4.1.2`:
  the action publishes no floating `v4` tag, so the jobs failed to start.

- Periodic traffic resets now run at most once per calendar day.
  - The forward flow reset and node monthly reset workers also run at every
    start, so restarting the server (or a rescheduled container) on a reset
    day zeroed user, tunnel and node traffic a second time.
  - Each run now claims the day in `v2_system_config`
    (`scheduler.forward_flow_reset.last_day`,
    `scheduler.node_monthly_reset.last_day`). The claim is made in the same
    transaction as the reset.
  - Expiry checks still run every time.
- Forward traffic snapshots now lock the forward row while advancing the
  traffic cursor. Two processes applying the same gost or ansible snapshot
  could both read the old cursor and double-count the delta; the in-process
  lock only covered one process.
- `OrderService.Complete` and node self-registration now really lock their
  rows. They used the GORM v1 `gorm:query_option` setting, which GORM v1.25
  ignores.
- The node request-signature nonce cache is now safe for concurrent requests:
  the map had no lock, and the check-then-mark sequence let two requests with
  the same nonce both pass.

- Legacy `/api/v2` handlers now see the original request `Host` and TLS state
  through the package bridge. The bridge rebuilt every request for the
  placeholder host `package-bridge`, so `GET /api/v2/forward-agent/install.sh`
  defaulted `PANEL_URL` to `http://package-bridge` and Telegram webhook
  registration without an explicit URL pointed at the same host. The address
  is kept in the kernel's bridge snapshot only; package hosts built with the
  `v4.0.0` SDK still receive the unchanged metadata. Found by the local
  upgrade rehearsal against `v4.0.0-alpha.7`.

- `/api/v2` error responses now keep the exact body the legacy handler wrote
  on `data` and `panel` package routes (status >= 400 with a JSON body). The
  gateway used to wrap `{"error": ...}`/`{"message": ...}` failure bodies as
  `{"data":{"error":...}}` on data routes and reject them as an invalid panel
  envelope (502) on panel routes, so 39 routes (agent/node admin, invite,
  WireGuard keypair, traffic reports) lost their error messages or turned
  client errors into 502. Found by the local upgrade rehearsal against
  `v4.0.0-alpha.7`.

- Fixed `/api/v2` package route resolution picking the first declared matching
  pattern instead of the most specific one. `GET /api/v2/admin/users/stats`,
  `/admin/orders/stats` and `/admin/nodes/stats` were dispatched to the
  `/:id` routes of the same package (400 or a wrong body). Resolution now
  follows gin's precedence (a static segment beats a parameter, left to
  right), within a package and across packages; only equally specific
  matches remain an ambiguity error. Found by the local upgrade rehearsal; a
  new test resolves every catalogued route through each package's real
  `compat/v2-routes.json` and checks the resolved route ID.
- Fixed a process crash (`fatal error: concurrent map writes`) in the forward
  background error logger. The latency prober logs probe failures from
  parallel goroutines, so when several probe targets were unreachable at the
  same time the unsynchronized rate-limit map aborted the whole server; this
  cannot be caught by `recover`. Found by the local upgrade rehearsal, where the
  production release (`v4.0.0-alpha.7`) crashed within a second of startup.
- A package response over the size limit is now reported as `502
  plugin_response_too_large` instead of `plugin_host_unavailable`. The package
  bridge, `pkg/pluginhostsdk` and the kernel now apply the same body-size rule
  (the SDK used to count the whole encoded response against 1 MiB, so a legacy
  body just under 1 MiB passed the bridge and then failed in the host); the
  bridge and SDK return `codes.ResourceExhausted`, and the kernel maps it to the
  new `pluginhost.ErrResponseTooLarge`.
- A panic in a legacy `/api/v2` handler reached through the package bridge, in
  a package bridge or node-facing gRPC handler, or in a package host no longer
  terminates the process. The bridge HTTP adapter now answers such a panic
  with an empty `500` (the same response as the kernel's gin recovery
  middleware; partial handler output is discarded), and the WebSocket adapter
  closes the relay with `1011`. The package bridge session and the kernel
  node gRPC server install recovery interceptors (new `internal/panicrecovery`)
  outermost and return `codes.Internal`. `pkg/pluginhostsdk` recovers panics
  in `Dispatch`, `Migrate`, `OpenWebSocket`, `Health` and `Drain` as
  `codes.Internal`, and its new `RecoveryServerOptions()` is used by the
  shared control host and identity-platform host. The kernel now reports a
  host's `codes.Internal` as `pluginhost.ErrPackageFailed` (still wrapped in
  `ErrHostUnavailable`, so the gateway response is unchanged). Recovered
  panics are logged with a stack trace bounded to 16 KiB.

- PostgreSQL connection strings are now built by `database.PostgresDSN`, which
  single-quotes and escapes every value. Before, an empty `database.password`
  made the driver read `password= dbname=x` as the password `dbname=x` and
  connect to the user's default database; empty hosts and values containing
  spaces, quotes or backslashes broke the same way. An empty host or a zero
  port now falls back to the driver default. `cmd/sqlite2postgres
  -target-config` uses the same builder. `github.com/jackc/pgx/v5` is now a
  direct dependency.
- `cmd/server` now shuts down in order and waits for its background work. A
  SIGINT/SIGTERM root context stops up to ten background workers (forward
  runtime, bridge, reset, stats and latency workers, the Control plugin
  lifecycle worker, the plugin operation dispatcher and the topology
  executor), and shutdown runs: `/health` draining, HTTP drain (30s), wait for
  workers (15s), gRPC stop (10s), Control plugin host shutdown with its own
  15s timeout, then cache and database close. A listener failure or plugin
  and dispatcher initialization error after startup no longer calls
  `log.Fatal`: it runs the same shutdown and exits with status 1. Plugin poll
  interval settings are validated before any listener starts. A second signal
  still forces an immediate exit.

- A package host that crashes or is killed no longer leaves its routes failing
  until the next lifecycle operation or reboot. The host supervisor
  (`internal/pluginhost`) logs the exit status and restarts the host at the
  same generation, version, and verified artifact after 1 s, 2 s, 4 s, ...
  (capped at 30 s); after 5 restarts within 5 minutes it marks the host failed
  and stops restarting until the next explicit lifecycle operation (enable,
  update, rollback, or boot reconciliation), which also resets the budget.
  While a host is exited, restarting, or failed, dispatch, health, migration,
  and WebSocket requests fail immediately with `ErrHostUnavailable` instead of
  dialing a dead socket. Host stdout and stderr are now written to the kernel
  log line by line as `[pkg:<id> v:<version> gen:<n> stderr] <line>` (lines
  capped at 8 KiB) instead of being discarded; `TZ` and `LANG` are passed to
  hosts when set in the kernel environment, and both bundled hosts embed
  `time/tzdata`. Stopping a host now sends `SIGTERM` to its process group and
  waits up to 2 s before `SIGKILL` (the bundled hosts stop gracefully on
  `SIGTERM`); on Linux a host also receives `SIGKILL` if the kernel dies.
  `Stop` and watchdog restarts no longer hold the supervisor lock that request
  dispatch uses, and `(*pluginhost.Supervisor).Stats()` exposes per-package
  start, unexpected-exit, restart, and failure counters.
- Made historical `v4.0.0-alpha.*` tags audit-only in release automation and
  made unconfigured product stages fail closed. Docker publication now waits
  for signed package publication.
- Removed the production WebUI runtime's compiled local-module fallback; a
  package WebUI must use its verified Control-served bundle URL.

## 4.0.0 - 2026-07-20

### Added

- Published the formal plugin-only Control release with the complete signed
  sixteen-package cohort, V2 package manifests, evidence verification, and
  release-bound canary and support approvals.
- Added a root-pinned cold-start bootstrap path for the signed
  `identity-platform` package, so a new package-only database can establish
  authenticated Control routes without an unauthenticated direct HTTP fallback.
- Added live signed-package browser coverage and cross-repository Agent package
  host coverage for package registration, lifecycle, WebUI delivery, bridge
  routing, and telemetry retrieval.

### Fixed

- Made SQLite package-artifact writes tolerate transient reader/writer
  contention and use WAL mode for file-backed databases.
- Bound Control-host bridge route IDs to safe deterministic identifiers while
  preserving the original validated request metadata for package handlers.
- Made the live signed-WebUI CI gate build the pinned Agent source for both
  supported Linux architectures, and scoped the full-repository security scan
  around its deliberately malformed static-analysis fixture.
- Replaced the transient third-party `protoc` setup Action with a checksum-
  verified official `protoc` 29.2 archive in the Go quality gate.
- Made release binaries compile across Linux, macOS, and Windows without
  weakening package-host isolation: unsupported non-Unix hosts now fail closed
  instead of falling back from the required Unix descriptor protections, while
  non-Linux Unix hosts apply an explicit checked close-on-exec descriptor flag.

## 4.0.0-alpha.7 - 2026-07-18

### Fixed

- Hardened G115 conversion boundaries for topology and plugin observations:
  Agent-reported revisions must fit the signed database range, and non-positive
  terminal operation revisions are rejected before promotion comparisons.

## 4.0.0-alpha.6 - 2026-07-18

### Added

- Added the signed `nftables-forward` 1.2.0 package contract and WebUI runtime
  status fields for live ruleset SHA-256, per-rule packets, bytes, and
  unhealthy/reconciling state.
- Added a bounded `PluginObservedState` heartbeat contract. The Agent
  Supervisor accepts only private observation files from enabled official
  packages declaring `kernel.observed-state`, then injects its own version,
  config hash, and revisions.
- Added Control persistence for authorized runtime observations with exact
  signed-release capability checks, timestamp bounds, monotonic ordering, and
  bounded/de-duplicated rule counters.
- Added topology health gates that bind terminal Agent state and runtime
  evidence to exact version, config hash, revision, health, and nftables rule
  IDs. Missing evidence gets a durable 90-second wait window; mismatches fail
  closed and rollback uses the same terminal-state checks.
- The Supervisor now checks the runtime health socket on every heartbeat and
  emits a Supervisor-owned unhealthy observation instead of retaining stale
  plugin evidence. Control treats observations older than two minutes as
  degraded in the nftables package WebUI.

### Fixed

- Stopped raw Agent operation JSON, runtime errors, and unknown plugin fields
  from being copied into public topology observed-state responses. Public
  `/api/v3/operations` responses now use a fixed non-secret projection.
- Made the Agent client preserve an unhealthy observation without a live
  fingerprint while rejecting invalid non-healthy counter payloads.
- Isolated malformed heartbeat observation entries so a bad or unauthorized
  plugin record cannot suppress later valid records, and made rollback validate
  the restored nftables configuration rather than the replacement config.

### Known Gaps

- This remains an isolated alpha canary. Production traffic takeover still
  requires staging restore evidence, a legacy fallback rehearsal, rollout
  records, and a successful 72-hour canary before explicit stable-release
  authorization.

## 4.0.0-alpha.5 - 2026-07-17

### Added

- Added read-only topology deployment preview and diagnosis with structured
  DAG, rollout dependency, node assignment, signed Agent release, artifact,
  dependency, config-schema, nftables runtime, port, address-family, MTU, and
  secret-reference checks. Deployment status now includes safe operation
  linkage and an event timeline without returning config or result payloads.
- Added the admin topology workflow for topology creation, immutable revision
  editing, server-side diagnosis, config-hash preview, canary rollout-group
  planning, apply/status polling, and guarded rollback. Unsaved revisions are
  fenced from preview and deployment actions.
- Added the signed `nftables-forward` 1.1.0 package contract with one canonical
  Control/Agent configuration schema, safe observation-only defaults, and
  `plugin.runtime-state` plus `plugin.cleanup` capabilities. The 1.0.0 Control
  executor remains registered for rollback compatibility.

### Fixed

- Replaced the incompatible 1.0 package configuration fields with the exact
  Agent runtime contract and added fail-closed Control semantic admission, so
  invalid nftables plans are rejected before dispatch.
- Pinned official package builds and cross-repository tests to the published
  Agent `v4.0.0-alpha.5` commit `72bbdff19f03768fd8bd9c720e236f93e898043f`, which journals the original
  nftables table, recovers after `SIGKILL` or Agent restart, and restores or
  removes owned state through the signed cleanup entrypoint.
- Cleared the deployment preflight static-analysis gate without changing
  runtime behavior.
- Fixed the frontend API proxy target for Control instances bound to a specific
  IPv4 or IPv6 address, while retaining loopback proxying for wildcard binds.
- Moved the required Agent gRPC bind ahead of plugin workers and HTTP listeners
  so an occupied node-control port fails startup before a partial Control
  instance becomes reachable.

### Known Gaps

- Topology execution remains disabled by default and this release is limited
  to isolated canary use. Live staging restore evidence, legacy fallback
  rehearsal, kernel-observed ruleset health, rollout records, and a fresh
  72-hour canary are still required before production traffic cutover or a
  stable 4.0 release.

## 4.0.0-alpha.4 - 2026-07-17

### Added

- Added the signed `machine-telemetry` 1.1.0 reference package end to end:
  Agent-local Unix RPC snapshots, gopsutil-backed CPU/memory/disk/network/
  process/uptime metrics, namespaced heartbeat transport, Control persistence,
  read-only API state, and the matching WebUI status view.
- Added signed-release capability admission for telemetry. Control accepts
  metrics only from an enabled assignment whose exact Agent release verifies
  against an active AnixOps trust root and declares `telemetry.read`.
- Added canary-aware topology planning, dependency checks, scoped rollback,
  and pure-removal revisions, including protection of the active revision until
  a full rollout completes.

### Fixed

- Added lifecycle admission fencing for the Agent Supervisor so close waits for
  active operations, rejects new work while closing, and can be retried after a
  deadline without losing cleanup state.
- Propagated operation deadlines and cancellation through queue, handler, and
  plugin locks; canonicalized Agent timeout observations and mapped them to
  Control `timed_out` state.
- Added cross-repository process E2E coverage from the signed Agent package
  through telemetry persistence and the Control plugin API/WebUI executor,
  pinned to Agent commit `676b5ad1c339a157075ae46028eee06540ffc26b`.
- Corrected the attached upgrade runbook to use the actual `anix-control-*`
  release assets and default systemd layout, and attached the exact signature
  verifier used by the Machine Telemetry package instructions.
- Cleared the blocking static-analysis gate for telemetry error aggregation and
  topology observed-state test setup without changing runtime behavior.

### Known Gaps

- This remains an opt-in alpha. Production traffic stays on the legacy path;
  topology data-plane cutover, Secret-ID materialization, live staging evidence,
  and the 72-hour canary are still required before stable 4.0 authorization.

## 4.0.0-alpha.3 - 2026-07-17

### Added

- Added explicit `permission_mode`, plugin permission, and restricted-plugin
  metadata to login, registration, and profile responses so the WebUI can
  enforce the same actor-scoped authorization contract as Control.

### Fixed

- Filtered the signed WebUI catalog, routes, menus, permissions, and bundle
  assets by the authenticated actor's `plugin_api` grants, with per-plugin
  legacy-admin compatibility and fail-closed regular-user behavior.
- Rejected unauthorized plugin routes before importing their modules, ensuring
  an actor with no matching route permission performs zero bundle fetches.
- Served plugin assets only for the active, enabled, version-matched verified
  installation and marked responses `private, no-store`, so disable, update,
  rollback, or permission revocation takes effect without a stale bundle cache.
- Quarantined an invalid signed WebUI extension at the plugin boundary so one
  malformed or tampered release cannot prevent valid authorized extensions or
  kernel pages from loading.
- Preserved the `/api/v2/user/profile` compatibility path when legacy tooling
  exercises a database before the optional Kernel tables are migrated: regular
  users receive empty authoritative plugin permissions and legacy admins keep
  pre-Kernel behavior, while real database failures still fail closed.
- Pinned cross-repository package and Agent process evidence to Agent commit
  `a8e6331e4c81274460e409720f8680649b7c2d17`, the matching alpha.3 source.

## 4.0.0-alpha.2 - 2026-07-17

### Fixed

- Retried the complete payment callback transaction on bounded SQLite
  writer-contention errors, preserving atomic payment-record, order, and
  gateway-stat updates without changing PostgreSQL or semantic error behavior.
- Tracked asynchronous notification work and drained it before shared SQLite
  test cleanup so notification logging cannot cross test boundaries and lock a
  later payment transaction.
- Required the real `nftables-forward` Agent binary to report plugin version
  `1.0.0` before package-contract or signed-release builds can proceed.
- Pinned package builds and cross-repository process gates to Agent hotfix
  commit `555be48faebf80e6c9d61cea10705583cf7c32f1`.
- Blocked release tags whose version does not match the Control runtime,
  frontend package/lockfile, configuration templates, generated Swagger,
  changelog, and current-preview documentation.
- Required both installer configuration templates to expose the exact release
  version, with negative tests for mismatched or format-escaped version fields.

## 4.0.0-alpha.1 - 2026-07-17

### Added

- Added version-bound Control status executors for the signed
  `nftables-forward` and `nat-egress` WebUI packages, so real installed bundles
  resolve assignment, operation, revision, health, cleanup, and rollback state
  instead of failing with an unimplemented plugin route.
- Added an operational official-package center for release manifest/artifact
  import, installation intent, enable/disable/update/rollback actions,
  revisioned Schema or JSON configuration, operation polling/cancellation, and
  signed WebUI extension refresh with visible terminal errors.
- Added authenticated node-facing official package downloads and assignment
  reconciliation. Enabled Agent assignments now produce a durable,
  revision-ordered `install -> update(config) -> enable` chain, aggregate
  multiple roles fail-closed, and replay safely after Control reconnects or
  restarts without exposing package bytes to unassigned nodes.
- Added the node-plugin assignment matrix to the admin WebUI, including
  role-aware configuration, enable/disable/delete lifecycle actions, and
  operation/observed-state refresh for canary rollout.
- Added a real cross-repository signed Agent package process gate covering
  package download authorization, install/update/enable, immutable bundle
  files, socket health, disable cleanup, and terminal operation replay.
- Migrated the Control Go module to `github.com/AnixOps/anix-control/v4` while
  preserving the `/api/v2`, `v2_*`, and `v2board` compatibility namespaces.

### Fixed

- Fixed the nftables forward status topology formatter so the production lint
  gate accepts its whitespace and slash trimming logic.
- Hardened authenticated Agent node ID conversion and updated the signed-package
  process fixture to use current gRPC APIs with checked resource cleanup.
- Pinned package builds and cross-repository process gates to the verified Agent
  v4 commit `882024acfb1f125becec8138c3ade0173072ef71`.
- Fixed release-note generation to select the immutable changelog section for
  the current tag, including dated headings, instead of requiring an
  `Unreleased` section that no longer exists in a prepared release commit.

## 3.1.0-alpha.1 - 2026-07-17

### Added

- Added the authoritative 3.1-to-4.0 plugin-platform roadmap, defining signed
  service/WebUI packages, microkernel ownership, reproducible release gates,
  the 3.5 business-plugin migration, and the plugin-only 4.0 cutover criteria.
- Added signed WebUI extension metadata and catalog validation, dynamic
  namespaced admin route/menu registration with local-module identity checks,
  revisioned installation configuration guarded by release JSON Schema, and an
  opt-in durable Control-to-Agent lifecycle dispatcher with ACK/observed-state
  persistence.
- Added byte-identical Control/Agent manifest golden fixtures, Agent-side WebUI
  manifest decoding, Control-side dependency/conflict installation checks, and
  a bundled `machine-telemetry` reference WebUI module.
- Added signed `control_routes` validation and a fail-closed `/api/v3/plugins`
  backend gateway with verified installation checks, `plugin_api` access-group
  grants, and the opt-in version-exact `machine-telemetry` Control executor.
- Added the executable major-upgrade program with 3.1 promotion blockers,
  reproducible Control/Agent/package/browser/PostgreSQL gates, canary rules, and
  3.2-to-4.0 rollback boundaries.
- Added a repeatable destructive PostgreSQL restore rehearsal with schema/row
  evidence and a blocking PostgreSQL 16 CI job.
- Added a real cross-repository Control database/KernelOperationBridge to Agent
  process E2E gate, plus the Agent-side Supervisor and signed
  `machine-telemetry` process E2E.
- Added Chromium Playwright WebUI failure-isolation coverage and a CI browser
  gate for same-origin digest checks, route collision rejection, disabled
  plugins, and tampered bundles.
- Added deterministic `machine-telemetry` package release-contract checks with
  ephemeral Ed25519 signing, public-key-only verification, artifact binding,
  and tamper rejection; no private key is stored or uploaded.
- Added dependency-aware Control lifecycle plans with dependency-first
  execution, cancellation/restart recovery, reverse rollback, and focused
  durability coverage.
- Added feature-gated topology deployment fan-out with durable per-node steps,
  observed-state reconciliation, failure fencing, and reverse rollback. The
  executor remains disabled by default and requires Agent dispatch before it can
  run.
- Added production release-tag signing and upload wiring for the official
  `machine-telemetry` package, requiring the
  `ANIXOPS_PLUGIN_SIGNING_PRIVATE_KEY` GitHub secret and publishing package,
  manifest, signature, public-key, and checksum evidence.
- Added the `nftables-forward` reference package source with deterministic
  package builds, dependency-free WebUI smoke coverage, public-key-only
  signature verification, tamper rejection, and CI release-contract evidence.
- Added privileged Agent-side `nftables-forward` namespace acceptance proving
  TCP DNAT, UDP DNAT, plugin-created table rollback, and pre-existing nftables
  table snapshot restoration before production data-plane canary.
- Added release-tag signing and artifact verification wiring for the official
  `nftables-forward` package alongside `machine-telemetry`, pinning the Agent
  runtime source and publishing package, manifest, signature, public-key, and
  checksum evidence.
- Added 3.3 `gost-mesh` and `nat-egress` reference package sources with
  deterministic package builds, dependency-free WebUI modules, namespaced
  status routes, public-key-only signature verification, tamper rejection, and
  CI release-contract evidence.
- Added the real `nat-egress` Agent runtime with nftables masquerade, fwmark
  policy routing, interface-bound health probes, strict configuration and
  ownership checks, crash-safe state journaling, and privileged namespace
  acceptance for marked forwarding, wrong-mark isolation, NAT, and rollback.
  Production release signing, artifact verification, and the immutable Agent
  revision pin are wired and locally gate-verified.
- Added the real `gost-mesh` Agent runtime and signed auxiliary-runtime
  packaging for checksum-pinned GOST v3.2.6. The aggregate `tunnels[]` runtime
  supports QUIC and WSS with mandatory mutual TLS, source-policy routing,
  source-bound health probes, bounded restart, ownership journaling, and
  crash-safe cleanup. Privileged namespace acceptance proves TCP and UDP data
  paths, transport selection, wrong-SNI and untrusted-client rejection, health,
  and cleanup. TUIC is not advertised because the pinned GOST runtime does not
  implement it. Release signing and upload now require the real Agent binary
  and the archive- and binary-digest-pinned GOST executable. The package remains
  canary-only until Control Secret ID materialization and sustained rollout
  evidence are complete.
- Added Supervisor `plugin.runtime-state` and `plugin.cleanup` lifecycle
  contracts. Cleanup intent and version are persisted as `cleanup_pending`;
  failed target-version cleanup leaves the plugin disabled and blocks old-
  version restart, while automatic rollback starts the old version only after
  cleanup succeeds.
- Introduced the AnixOps Control / AnixOps Agent product identity, primary `anix-control` binaries, frontend archives and Docker images, stable/alpha/beta/RC release tag support, and a documented compatibility window with legacy `v2board-*` release aliases.
- Added a tag-pinned native release installer that downloads and verifies GitHub Actions-built panel/frontend assets without cloning or building on the target host, preserves configuration/data, and restores the previous application snapshot after a failed health check.
- Added detailed release installation and legacy migration guides covering fresh install, update, rollback, SQLite-to-PostgreSQL boundaries, foreign-panel migration limits, coordinated node rollout, and retained evidence.
- Added the first P0 WireGuard panel slice: `wireguard` node protocol template, stored per-user peer keypair/PSK/IP custody, native WireGuard `.conf` subscription output, sing-box 1.13-compatible WireGuard endpoint output, and targeted unit coverage.
- Added WireGuard UniProxy/gRPC runtime user fields so V2bX can receive panel-managed peer IP, public key, and preshared key for domestic entry termination.
- Updated P0 WireGuard status docs for the V2bX v2.3.3 runtime slice, which adds initial WireGuard peer online-state reporting from recent `wg show <iface> dump` handshakes.
- Extended the WireGuard relay runtime contract with entry/exit GOST TUN fields for V2bX entry policy routing and overseas exit NAT command application.
- Added a first admin WireGuard visual protocol form for CIDR, server keys, MTU, DNS, entry/exit GOST relay role, QUIC/WSS tunnel selection, one-click WSS compatibility mode, TUN addresses, routing table/priority, and exit NAT hints.
- Added deterministic `RELEASE_NOTES.md` generation to GitHub Release assets, with a CI self-test, release workflow policy guard, and artifact verification requirement.
- Added opt-in local deploy archive cleanup for stale ignored frontend/internal zip or tarball leftovers while keeping release builds GitHub Actions-only and preserving database backups.
- Attached `UPGRADE.md` to GitHub Release assets and guarded it with release workflow policy and artifact verification checks so tag releases include upgrade and rollback instructions.
- Added `docs/UPGRADE.md` with the GitHub Actions artifact verification, systemd/Docker upgrade, database migration, rollback, and post-upgrade evidence runbook.
- Added a root `README.md` that links the status registers, audit docs, deployment docs, local checks, compatibility surfaces, and GitHub Actions-only release policy.
- Added a safe local build artifact cleanup helper with CI self-tests so stale source-tree outputs can be removed without touching config, database, certificates, backups, or `web/node_modules`.
- Extended local cleanup coverage to frontend verification outputs including `web/public-check`, `web/coverage`, and `web/bundle-reports-check`, keeping local test/build artifacts out of the source tree.
- Added `docs/features.md` as the current feature status register for implemented, partial, planned, deferred, and compatibility surfaces, with an update rule for every feature-status-changing commit.

### Fixed

- Kept GOST Mesh status and IPv4 broadcast validation compatible with the
  repository's static-analysis and integer-safety release gates without
  changing the accepted configuration contract.
- Made `make run` create and use an isolated development configuration with
  loopback ports `19080` (API), `19000` (frontend), and `50052` (gRPC), avoiding
  collisions with an existing system `anix-control` service and its database.
- Made gRPC server lifecycle tests bind ephemeral loopback ports so `make run`
  can stay active while the full test suite runs.
- Made the PostgreSQL large-traffic regression derive its dashboard expectation
  from the local-day boundary, avoiding a false failure during the midnight
  hour without changing production aggregation semantics.
- Stabilized plugin runtime test gates by observing pre-handler gRPC stream
  rejection through the authoritative receive status, and by restoring Vitest
  mocked globals and timers after every frontend test.
- Silenced misleading frontend test-run stderr by mocking AdminLayout profile
  refreshes in its unit tests and escaping JSON examples in locale messages so
  vue-i18n no longer reports placeholder compilation errors during successful
  runs.
- Serialized Control package operations per installation, added target-level
  dependency/conflict transaction locks and lifecycle-generation idempotency,
  and made lease loss cancel the executor while automatic rollback remains
  bounded and lease-supervised.

- Enforced global MFA `enforce_for_all` and `enforce_for_admin` login enrollment policies by returning no-token enrollment-required responses for covered users who have not enabled MFA, with handler coverage for all-users, admin-only, and regular-user bypass prevention paths plus a Login page enrollment-required prompt.
- Enforced user-enabled TOTP/backup MFA during login before issuing JWTs, counting invalid MFA codes in the login rate limiter and adding backend handler coverage plus a frontend two-step MFA challenge flow.
- Blocked Alipay, WeChat, and USDT payment gateways from being enabled or used for new payment records until their live callback or confirmation implementations and tests exist, including historical enabled-row filtering.
- Made notification async sends return a completion signal and updated the user-ID copy regression test to wait for the send path, removing a timing race with shared service-test database cleanup.
- Fixed the integration workflow coverage upload by generating `coverage.out` during integration unit tests before uploading the artifact.
- Refreshed GitHub Actions workflow dependencies to current action major versions so CI/release jobs no longer depend on Node.js 20 action runtimes.
- Fixed the GitHub Actions CI baseline by pinning Go setup to `1.26.5` for stdlib vulnerability scanning and adding focused gRPC NodeLog/server-context tests so the gRPC coverage gate remains above 80%.
- Added a CI release-build policy check that fails deployment scripts containing unguarded local `go build` or frontend build commands, including the V2bX Ansible rollout helper.
- Guarded the legacy `config/scripts/deploy.sh` and `config/scripts/pre-deploy.sh` entrypoints behind `ALLOW_LOCAL_BUILD=1` and added CI self-tests so old local deploy paths cannot build by default.
- Guarded the local source-tree deploy script behind `ALLOW_LOCAL_BUILD=1`, documented GitHub Actions as the only release build source, and updated the generated release runbook to deploy release artifacts instead of building on the host.
- Upgraded vulnerable Go dependencies reported by `govulncheck`: `google.golang.org/grpc` to `v1.79.3`, `github.com/jackc/pgx/v5` to `v5.9.2`, and `github.com/quic-go/quic-go` to `v0.59.1`.
- Cleared production runtime `gosec` findings by range-checking sing-box integer conversion, tightening generated runtime file/directory permissions, and documenting reviewed config-path and subscriber credential JSON outputs.
- Replaced the weak default bootstrap admin password with a generated `crypto/rand` password when `admin.password` is empty.
- Replaced load balancer random and weighted node selection with `crypto/rand` and explicit random-source error handling.
- Hardened `cmd/verify` local E2E verification by requiring `V2BOARD_VERIFY_TOKEN`, restricting panel URLs to loopback hosts, allowlisting local Xray binary names, and writing temporary Xray configs with private permissions.
- Hardened `cmd/subtest` subscription test tooling by validating local YAML config paths and writing generated sample configs with private permissions.
- Hardened `cmd/configgen` by checking generated config write errors and writing integration client configs with private permissions.
- Hardened `cmd/report` by checking report generation and E2E cleanup errors, writing reports/configs with private permissions, validating local Xray binary names, and adding a local echo server header timeout.
- Hardened the integration mock server by checking JSON response encoding failures and adding HTTP header read timeouts.
- Hardened integration echo helpers by checking close/write/deadline/copy errors and documenting the intentional local echo response behavior.
- Hardened the integration local environment by checking process/probe cleanup errors, restricting reviewed local server binary names, and writing generated configs with private permissions.
- Hardened the shared test database helper by returning close failures through `CloseWithError` while keeping the legacy close wrapper observable.
- Hardened the integration runner by reporting config-directory setup failures, using private generated-file permissions, and logging generated-config cleanup failures.
- Hardened the integration binary manager by checking download/cache cleanup errors, bounding archive extraction, using private cache permissions, and scoping local file access through `os.Root`.
- Hardened the integration clients by checking shutdown/probe cleanup errors, allowlisting local client binaries before subprocess launch, and scoping manual binary lookup through `os.Root`.
- Hardened the migration dump parser by opening operator-provided dump files through `os.Root` and adding plain/gzip dump parser coverage.
- Hardened `cmd/sqlite2postgres` by logging database close failures, returning row-close failures, and covering table copy behavior.
- Hardened `cmd/integration-test` JSON-output tests by checking pipe close/copy errors and clearing package-local lint findings.
- Hardened `cmd/report` tests by checking listener/response cleanup and validating the echo server's assigned-port ping path.
- Hardened integration echo server tests by checking response/server cleanup errors and clearing package-local lint findings.
- Hardened integration local environment tests by checking echo shutdown, subprocess cleanup, and HTTP response close errors while clearing package-local lint findings.
- Hardened integration mock server tests by checking response close, JSON decode, server shutdown, and JSON fixture encoding errors while clearing package-local lint findings.
- Hardened integration binary manager tests and download URL selection by checking mock response writes, removing ineffectual assignments, and clearing package-local lint findings.
- Hardened the integration runner by propagating client stop failures into test results and preserving result duration updates while clearing package-local lint findings.
- Cleared `cmd/verify` lint findings by normalizing local Xray lookup error text.
- Cleared WebSocket Origin utility lint findings by normalizing scheme mapping through a tagged switch.
- Hardened database package tests by checking database close paths, isolating SQLite path tests under temporary directories, and clearing package-local lint findings.
- Hardened the integration setup script by logging database close failures and clearing package-local lint findings.
- Hardened middleware tests by checking node table migration and database cleanup errors while clearing package-local lint findings.
- Hardened integration client tests and HTTP probe cleanup by checking manager registration errors and logging deferred response body close failures while clearing package-local lint findings.
- Hardened command entrypoint shutdown paths by logging `cmd/migrate` and `cmd/server` database close failures while clearing package-local lint findings.
- Hardened router, smoke, and root integration tests by checking schema migration, database cleanup, response close, and port parsing failures while clearing package-local lint findings.
- Hardened websocket tests by checking client close failures and removed an unused client mutex while clearing package-local lint findings.
- Hardened GOST client and manager tests by returning response close failures, checking mock response writes, and removing an unused manager mutex while clearing package-local lint findings.
- Hardened cache tests by checking cache mutation/read errors and correcting concurrent access coverage to read typed values while clearing package-local lint findings.
- Hardened gRPC tests by checking database lifecycle, server serve, client connection close, and stream close errors while clearing package-local lint findings.
- Hardened E2E tests by checking JSON response decoding, database cleanup, and subscription group association errors while clearing package-local lint findings.
- Hardened integration E2E tests by checking echo shutdown, port probe, process signal/kill, and HTTP response body close errors while clearing package-local lint findings.
- Hardened handler package cleanup and tests by checking WebSocket, request/response body, Telegram update, database setup, and JSON decoding errors while clearing package-local lint findings.
- Hardened service small-file cleanup by checking migrations, benchmark setup, response/archive close paths, removing stale private helpers and fields, and clearing service staticcheck/unused findings outside the consolidated `service_test.go` suite.
- Hardened the consolidated service test suite by checking fixture creation, registration, order setup, auth-key generation, and cleanup errors, clearing the final Go lint baseline.
- Hardened forward node health checks, tag parsing, API token generation, and random node selection with explicit error handling and context-aware dialing.
- Enabled SMTP certificate verification for notification email delivery and made asynchronous notification send failures observable.
- Returned explicit MFA backup-code JSON parse errors instead of silently treating corrupt data as no remaining backup codes.
- Made dashboard and user subscription statistics cache writes/deletes explicit, with refresh paths returning cache failures and read paths logging non-fatal cache write errors.
- Made legacy server node config generation reject malformed JSON settings and return online-status cache update failures instead of silently producing partial state.
- Made Telegram broadcast, API request encoding, and admin ID parsing errors explicit instead of silently treating failures as success or empty configuration.
- Hardened local backup creation and restore with private backup-directory permissions, zip path traversal checks, non-regular entry rejection, and archive decompression size limits.
- Restricted local forward runtime command execution to `ansible-playbook` or absolute `ansible-playbook` paths before spawning runtime jobs.
- Replaced subscription template node ID MD5 hashing with SHA-256 and documented the remaining SS2022 MD5 path as a reviewed legacy compatibility requirement.
- Made WebSocket response and monitor streams handle JSON, deadline, write, close, and backpressure failures explicitly; also replaced UniProxy ETag MD5 hashing with SHA-256.
- Restricted WebSocket browser origins through a shared same-host/CORS allowlist policy and added agent WebSocket read/write deadlines.
- Cleared production-package `G104` findings outside `internal/service` by checking model JSON decode failures, Gost delete/update errors, server startup writes, and verification command I/O/process cleanup.
- Added range-checked gRPC protobuf integer conversions for node IDs, ports, TLS flags, and user device limits, and made traffic/online heartbeat update failures observable.
- Added gRPC bidirectional stream cancellation coverage, including online stream cancellation and status-stream connection cleanup.
- Added background worker cancellation/drain coverage for shared delayed cycles, the Gost stats idle loop, and runtime executor claimed-job cleanup.
- Added clean-agent bridge worker cancellation and retry coverage, including canceled NodeX translation draining to a failed runtime job and retry upsert of an existing bridge mapping.
- Made memory cache init/close lifecycle idempotent by closing and waiting for stale cleanup goroutines before replacing or restarting the cache.
- Isolated the shared service test SQLite database under a per-process temporary directory and close it before cleanup.
- Fixed node update cache invalidation to delete `node:<id>` keys using decimal node IDs instead of rune conversion.
- Added compatibility for the legacy `/api/v1/client/subscribe?token=` subscription endpoint.
- Added service and admin HTTP coverage for oversized traffic ranking requests, verifying the 200/1000 row caps used by `/admin/traffic/user-ranking`.
- Hardened EPay callbacks with constant-time signature comparison, signed amount parsing, and order amount verification before marking payments paid.
- Added a registry guard test so every plugin payment callback gateway must provide valid and tampered-signature coverage.
- Added mocked PayPal webhook verification tests covering remote signature success and rejection without external network calls.
- Made panel forward pause/resume idempotent while runtime jobs are pending or running, preventing duplicate runtime job enqueueing and stale status overwrites from repeated or bulk actions.
- Added a partial unique database guard for pending/running forward runtime jobs and a schema repair step that collapses historical duplicates before creating the guard.
- Filtered bulk forward pause/resume actions in the admin UI so only forwards that actually need the requested state change are submitted.
- Normalized Ansible Machines admin responses to the panel `code/msg/ts/data` envelope and added handler coverage for success and error responses.
- Normalized Forward Node management responses to the panel `code/msg/ts/data` envelope and expanded handler coverage for invalid ID, missing body, not-found, scope rejection, toggle, and sync-stats paths.
- Normalized Forward Rule management responses to the panel `code/msg/ts/data` envelope and expanded handler coverage for list, create, get, update, delete, toggle, missing body, invalid ID, and not-found paths.
- Normalized Forward stats, user-rule, and connection-test responses to the panel `code/msg/ts/data` envelope while preserving connection-test diagnostics under `data.success=false`.
- Expanded Forward observability response-envelope coverage for targets, trend, topology, and multi-ingress endpoints, with frontend API mapping tests for all observability calls.
- Expanded Forward internal traffic report/snapshot handler coverage for panel `code/msg/ts/data` success, binding-error, and service-error responses.
- Normalized admin traffic hourly and user-ranking responses to the panel `code/msg/ts/data` envelope while keeping the TrafficHourly page compatible with legacy and enveloped payloads.
- Normalized the admin dashboard success/database-error response to the panel `code/msg/ts/data` envelope while keeping the Dashboard page compatible with legacy and enveloped payloads.
- Normalized admin user stats success/database-error responses to the panel `code/msg/ts/data` envelope while keeping the Users page compatible with legacy and enveloped payloads.
- Normalized admin order stats success/database-error responses to the panel `code/msg/ts/data` envelope while keeping the Orders page compatible with legacy and enveloped payloads.
- Stabilized async notification service coverage by waiting for the final send status before asserting copied user IDs.
- Normalized admin node stats responses to the panel `code/msg/ts/data` envelope while keeping the Nodes page compatible with legacy and enveloped payloads.
- Normalized admin system info responses to the panel `code/msg/ts/data` envelope while keeping the AdminLayout version display compatible with legacy and enveloped payloads.
- Normalized admin invite stats responses to the panel `code/msg/ts/data` envelope while keeping the Invite page compatible with legacy and enveloped payloads.
- Normalized admin payment stats responses to the panel `code/msg/ts/data` envelope while keeping the Payment page compatible with legacy and enveloped payloads.
- Normalized admin subscription stats responses to the panel `code/msg/ts/data` envelope while keeping the Subscriptions page compatible with legacy and enveloped payloads.
- Normalized admin system backup stats success/user-error responses to the panel `code/msg/ts/data` envelope while keeping the System backup view compatible with legacy, enveloped, and `code=-1` payloads.
- Normalized admin load balancer stats success/user-error responses to the panel `code/msg/ts/data` envelope and added frontend API mapping coverage for the stats route.
- Normalized admin system backup config success/user-error responses to the panel `code/msg/ts/data` envelope while keeping sensitive-field masking and the System backup view compatible with legacy, enveloped, and `code=-1` payloads.
- Normalized admin system backup list/create/delete/restore success/user-error responses to the panel `code/msg/ts/data` envelope while keeping the System backup list compatible with legacy, enveloped, and `code=-1` payloads.
- Normalized admin invite config responses to the panel `code/msg/ts/data` envelope while keeping the Invite page compatible with legacy and enveloped payloads.
- Normalized user invite info/code/commission/withdrawal responses and admin invite config update/withdrawal responses to the panel `code/msg/ts/data` envelope while keeping the Invite page compatible with legacy, enveloped, and nested payloads.
- Normalized admin subscription settings responses to the panel `code/msg/ts/data` envelope while keeping System and Users subscription-link flows compatible with legacy and enveloped payloads.
- Normalized admin payment gateway list responses to the panel `code/msg/ts/data` envelope while keeping the Payment page compatible with legacy and enveloped payloads.
- Normalized admin payment gateway create/update/delete/toggle and payment-record list success/user-error responses to the panel `code/msg/ts/data` envelope while keeping the Payment page compatible with legacy, enveloped, and `code=-1` payloads.
- Normalized user payment channels/create/status/records success/user-error responses to the panel `code/msg/ts/data` envelope while preserving payment callback plain-text compatibility.
- Updated Auth and admin traffic E2E tests to assert the panel response envelope after login/register and traffic stats response normalization.
- Normalized legacy X402 and fiat payment create/check success and user-error responses to the panel `code/msg/ts/data` envelope while preserving X402, Stripe, and PayPal callback/webhook compatibility responses.
- Normalized user/admin MFA success and user-error responses to the panel `code/msg/ts/data` envelope while keeping the Admin MFA page compatible with legacy, enveloped, and error config payloads.
- Normalized user/admin notification success and user-error responses to the panel `code/msg/ts/data` envelope while keeping the Admin Notifications page compatible with legacy, enveloped, and `code=-1` templates, logs, email config, and mutation payloads.
- Normalized admin/user Telegram panel API success and user-error responses to the panel `code/msg/ts/data` envelope while preserving the public Telegram webhook `status=ok` compatibility response and keeping the Admin Telegram page compatible with legacy, enveloped, and `code=-1` payloads.
- Normalized admin subscription group CRUD success/user-error responses to the panel `code/msg/ts/data` envelope, made missing group deletes return a user error, and moved group delete cleanup into a transaction while keeping the Subscriptions page compatible with legacy, enveloped, and `code=-1` group payloads; template, protocol, and preview success payloads remain covered separately.
- Normalized admin subscription template CRUD success/user-error responses to the panel `code/msg/ts/data` envelope, made missing template update/delete operations return user errors, and kept the Subscriptions page compatible with legacy, enveloped, and `code=-1` template payloads.
- Normalized admin subscription protocol binding/read success-user-error responses to the panel `code/msg/ts/data` envelope, made missing protocol IDs fail instead of being silently ignored, and kept the Subscriptions page compatible with legacy, enveloped, and `code=-1` protocol payloads.
- Normalized admin subscription preview success/user-error responses to the panel `code/msg/ts/data` envelope, accepted the frontend `group_ids` request alias, and allowed preview to use the authenticated admin context when `user_id` is omitted.
- Normalized admin subscription user/plan group binding success-user-error responses to the panel `code/msg/ts/data` envelope, made missing users/plans/groups and missing relations fail explicitly, and kept Plans subscription-group flows compatible with enveloped errors.
- Normalized admin node management CRUD, protocol, log, raw-config, and auth-key success responses to the panel `code/msg/ts/data` envelope while keeping the Nodes page compatible with legacy and enveloped payloads.
- Normalized admin Agent list, task result, task history, monitor read, task creation, and execute-command success responses to the panel `code/msg/ts/data` envelope while keeping the Agent page compatible with legacy, enveloped, and nested payloads.
- Normalized admin user management CRUD, ban/unban, traffic reset, and subscribe-reset success and user-error responses to the panel `code/msg/ts/data` envelope, made missing user mutations fail explicitly instead of silently updating zero rows, and kept the Users page compatible with legacy, enveloped, nested, and `code=-1` mutation payloads.
- Normalized admin plan management list/detail/create/update/delete/assign success and user-error responses to the panel `code/msg/ts/data` envelope, made missing plan/user operations fail explicitly instead of silently updating or inserting, and kept the Plans page compatible with legacy, enveloped, nested, and `code=-1` payloads.
- Normalized admin order management list/detail/status/paid/cancel success and user-error responses to the panel `code/msg/ts/data` envelope, made missing orders and missing related plans fail explicitly instead of silently updating zero rows, and kept the Orders page compatible with legacy, enveloped, nested, localized, and `code=-1` payloads.
- Normalized user registration success/error and order-save success/error responses to the panel `code/msg/ts/data` envelope while preserving the existing token and order payloads under `data`.
- Normalized user login success/error responses to the panel `code/msg/ts/data` envelope while keeping the Login page compatible with enveloped token and error payloads.
- Normalized user order list and detail success/error responses to the panel `code/msg/ts/data` envelope while keeping the Orders page compatible with legacy and enveloped payloads.
- Normalized user subscription info success/error responses to the panel `code/msg/ts/data` envelope while keeping the Subscribe page compatible with legacy and enveloped payloads.
- Normalized user profile success/error responses to the panel `code/msg/ts/data` envelope while keeping the user store compatible with the enveloped profile payload.
- Normalized user dashboard success/error responses to the panel `code/msg/ts/data` envelope while preserving the subscription payload under `data.subscription`.
- Normalized user plan list success/database-error responses to the panel `code/msg/ts/data` envelope while keeping the Plans page compatible with legacy, enveloped, and `code=-1` payloads.
- Normalized user coupon-check success and business-error responses to the panel `code/msg/ts/data` envelope while keeping the Plans page compatible with legacy and enveloped coupon payloads.
- Normalized user knowledge list and detail success/error responses to the panel `code/msg/ts/data` envelope while keeping the Knowledge page compatible with legacy and enveloped article lists.
- Normalized user ticket list, create, detail, reply, and close success/error responses to the panel `code/msg/ts/data` envelope while keeping the Tickets page compatible with legacy and enveloped ticket payloads.
- Normalized public payment methods and payment-status responses to the panel `code/msg/ts/data` envelope with handler coverage for key payload fields and missing payment records.
- Normalized admin ticket list, reply, and close success/error responses to the panel `code/msg/ts/data` envelope while keeping the Tickets admin page compatible with legacy and enveloped payloads.
- Normalized admin coupon list, create, and delete success/error responses to the panel `code/msg/ts/data` envelope while keeping the Coupons admin page compatible with legacy and enveloped payloads.
- Normalized admin knowledge list, create, update, and delete success/error responses to the panel `code/msg/ts/data` envelope, preserved partial-update sort/visibility fields when omitted, and restored missing Knowledge admin page locale strings.
- Normalized admin system audit-log success and database-error responses to the panel `code/msg/ts/data` envelope while keeping the System audit page compatible with legacy, enveloped, and `code=-1` payloads.
- Normalized admin system config CRUD success and user-error responses to the panel `code/msg/ts/data` envelope while retaining sensitive-value masking and keeping the System runtime/config views compatible with legacy, enveloped, and `code=-1` payloads.
- Normalized admin load balancer CRUD and health-check success/user-error responses to the panel `code/msg/ts/data` envelope while keeping the System load balancer view compatible with legacy, enveloped, and `code=-1` payloads.

### Documentation

- Updated WireGuard P0 status docs to reflect the implemented panel peer/subscription slice while keeping full V2bX runtime, WSS compatibility UI, traffic/limit behavior, migration evidence, and GitHub Actions relay verification marked incomplete.
- Documented the P0 WireGuard dual-node relay plan, including WireGuard user access, domestic entry termination, default GOST relay+QUIC transport, overseas exit NAT, and WSS compatibility mode.
- Added the initial repository, concurrency, and performance audit baselines plus root `ROADMAP.md`, root `TODO.md`, and `docs/manual-intervention.md`.
- Documented the production root deployment command, Go/Node/npm prerequisites, deploy script self-test, and common recovery hints.
- Added a SQLite-to-PostgreSQL migration runbook covering dry run, import, verification evidence, and rollback.
- Added the traffic stats operations runbook covering `v2_server_log` indexes, query bounds, and retention maintenance.
- Added forwarding design, API, security, and compatibility baselines under `docs/forwarding/`, documenting the current runtime boundaries, Flux-shaped route contract, security controls, and remaining clone/runtime gaps.

### CI/CD

- Added `config/scripts/generate_release_notes.py` with a CI self-test so tag releases attach deterministic `RELEASE_NOTES.md` generated from the current `CHANGELOG.md` instead of relying only on GitHub's generated notes.
- Added `config/scripts/verify_release_artifacts.py` with a CI self-test and a release-job verification step so tag releases fail before publishing if required artifacts are missing or `RELEASE_MANIFEST.json`/`SHA256SUMS.txt` disagree with the release directory.
- Moved release manifest generation into `config/scripts/generate_release_manifest.py` with a CI self-test so release artifact metadata generation is directly validated instead of living only as inline workflow code.
- Added a machine-readable `RELEASE_MANIFEST.json` to GitHub Release assets with tag, commit, run metadata, CI build-source marker, manual-deployment flag, artifact sizes, and artifact SHA-256 hashes; the release workflow policy guard now fails if the manifest is removed.
- Attached SQLite-to-PostgreSQL migration dry-run output as `migration-dry-run.txt` in release artifacts, included it in release checksums, and extended the release workflow policy guard to prevent removing that evidence.
- Added a release workflow policy CI gate with self-tests so tag-gated release jobs keep their quality/security/race/test prerequisites, multi-platform binary matrix, Docker metadata, frontend archives, checksums, SBOM, operator deployment runbook, and generated release notes.
- Added a documentation sync CI gate with self-tests so implementation, frontend, deployment, workflow, or config changes must include maintained status documentation such as `CHANGELOG.md`, `TODO.md`, `docs/features.md`, audit docs, forwarding docs, guide docs, reference docs, or manual intervention notes.
- Added Go quality gates for `go mod tidy`, `gofmt`, `go vet`, full `go test ./...`, race testing, benchmark smoke testing, `govulncheck`, blocking production runtime `gosec`, non-blocking full-repository `golangci-lint` reports, and Docker build smoke testing.
- Promoted full-repository `golangci-lint` from report-only to a blocking CI gate after clearing the baseline.
- Updated the former full-repository `gosec` report to exclude generated code after all non-generated findings were cleared.
- Promoted the generated-file-excluded full-repository `gosec` report to a blocking CI gate while preserving the JSON artifact upload.
- Added `.golangci.yml` to keep frontend dependency trees out of Go lint reports.
- Added a SQLite-to-PostgreSQL migration dry-run CI gate and made release/backend builds depend on it.
- Added realistic service benchmarks for hourly traffic and user-ranking queries so benchmark smoke covers stats hot paths.
- Added forwarding runtime job benchmarks for admin listing filters and clean-agent heartbeat claiming so the service benchmark smoke covers runtime queue hot paths.
- Added a frontend bundle size report script and CI artifact that tracks total JS/CSS output plus heavy admin/runtime chunks such as Forward, Nodes, System, NodeX, LocalRuntime, echarts, and G6.
- Replaced the placeholder production deploy job with a release-attached `OPERATOR_DEPLOYMENT.md` manual deployment runbook.
- Added SPDX JSON SBOM generation to release assets using `anchore/sbom-action`.
