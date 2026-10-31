# Node Onboarding And Admin Cleanup (5.0)

**Status:** approved by the owner on 2026-10-12 (decisions D1-D8 below). Work in
progress; each phase lands as its own pull request.

**Compatibility:** none. Control and the Agent are maintained and run by one
person, and the production panel had no nodes when this was decided, so 5.0
changes the Agent configuration, the APIs and the schema without migration
paths for old nodes. Every node is installed with the new command.

## Why

Adding a node today means choosing between three generations of onboarding
that sit side by side on the node page:

| Generation | Entry | How it works | State |
|---|---|---|---|
| Registration key | 节点 › 注册密钥 | A global key in the Agent's `config.json`; the Agent registers over `POST /api/v2/node/register` | Refused by the 4.2 default `agent_control.mtls: required` (403 `agent_mtls_required`). It cannot work on a default install |
| Ansible | 节点 › 部署父节点 | The browser builds an inventory with node API keys and SSH passwords in clear text; a playbook installs the Agent as a `V2bX` unit | Hard-coded `/home/dev/...` paths; covers root nodes of the visible page only |
| One-command install (4.2, O1-O4) | Node page › 部署 › 复制安装命令 | `install.sh --control … --node proxy-<id> --token anixagt_…` | Works, but the token is bound to a node that must already exist, so the node is created by hand first; `--group` is refused |

The third generation is one step away from the target: a token that stands
for a **group** and creates the node the first time a machine uses it. This
document designs that step and the removal of everything else.

Two smaller problems are fixed on the way:

- Topologies on 部署编排 cannot be deleted: there is no endpoint, and their
  name is a plain unique index.
- Seven sidebar extension pages are empty. Thirteen packages ship a
  placeholder WebUI that renders a heading and a version; machine-telemetry's
  real page needs the plugin host, which is off by default.

## Decisions

| | Question | Decision |
|---|---|---|
| D1 | Version | **5.0.0**. The Go module path becomes `github.com/AnixOps/anix-control/v5`; the mechanical rename lands last, in the release PR, so it does not conflict with the feature PRs |
| D2 | Third-party agents (UniProxy, v2board gRPC) | **Removed**, together with node API keys, node shared secrets and signature authentication. A node's only identity is its agent certificate |
| D3 | `parent_id` relay chains | **Removed.** A forward route whose exit is a protocol of a proxy node can be published to subscriptions instead (Phase 10) |
| D4 | New nodes | **Per-group setting, default automatic.** A group can require approval; every join notifies the administrator |
| D5 | Machine Telemetry extension page | **Removed.** The node list gets a 服务状态 column; node detail keeps its services section |
| D6 | Deleting a topology whose revision is live | **Tear down first, then flag.** An internal deployment disables every vertex; on success `deleted` becomes 1 |
| D7 | NodeX Agents page | **Removed.** Its diagnostics (service status, restart, log tail, forward checks) move to a 诊断 tab on node detail; they already run over the Agent Control stream |
| D8 | Group join token | **Re-showable.** Stored sealed, so the command can be copied at any time; resetting it invalidates every earlier command |

Defaults accepted with them:

- A machine that still holds its identity reuses it; a machine that lost its
  local state rejoins its old node only under the rule in "Rejoining".
- Deleting a node and running the command again on that machine creates a
  new node.

## The Administrator's Flow

1. 节点 › **接入节点**. Pick a node group, or create one inline (a name; the
   rest has defaults).
2. Copy the command shown and run it on the machine:

   ```sh
   curl -fsSL https://panel.example.com/install.sh | sudo bash -s -- -u https://panel.example.com -t anixjoin_8Kq...
   ```

   Optional flags: `--name hk-01` (default: the host name), `--host 203.0.113.5`
   (the address clients connect to; default: detected), `--mirror cn|github`,
   `--offline <bundle>`, `--reset`.
3. The dialog lists machines as they join ("hk-01 已接入"). The node is online,
   its protocols exist, and it is in subscriptions (or waits for approval when
   the group requires it).

The same command works on any number of machines; each one becomes a node of
the group. Uninstall stays `sudo anix-agent uninstall [--purge]`.

## Node Groups

A node group is the unit that owns a join token and the defaults of the nodes
it creates. Table `v5_node_group`; the unused `v2_node_group` model is
deleted.

| Field | Meaning |
|---|---|
| `name` | Unique among live groups |
| `kind` | `proxy` (serves users; creates `v2_node` rows) or `forward` (entry and relay machines for forward routes; creates `v2_forward_node` rows) |
| `join_token_hash` | SHA-256 of the token, unique; the only value used for lookup |
| `join_token_sealed` | The token sealed with AES-256-GCM under a key derived from `module_runtime.ca_kek` (HKDF, label `anixops node-group join token v1`), so the command can be shown again (D8) |
| `token_reset_at` | When the token was last reset |
| `token_expires_at` | Optional; after it the token joins nothing |
| `max_nodes` | Optional cap on live nodes in the group |
| `require_approval` | D4; default false |
| `rate`, `tags`, `monthly_limit_bytes`, `reset_day`, `sort` | Copied to each new node |
| `deleted`, `deleted_at` | Soft delete (see "Soft delete") |

Nodes get `node_group_id`, `machine_fingerprint`, `approval` (`approved` or
`pending`), `deleted` and `deleted_at`. Moving a node to another group changes
only `node_group_id`; it does not re-apply defaults.

Token format: `anixjoin_` + base64url(32 random bytes). Resetting replaces
the hash and the sealed value in one update; nodes that already joined are
unaffected because they authenticate with certificates. Deleting a group
refuses while it has live nodes.

`module_runtime.ca_kek` is already required by `agent_control.mtls: required`,
which 5.0 makes the only mode, so sealing needs no new configuration.

## The Install Command

`install.sh` stays one static, signed script (`/install.sh`,
`agent-install.sh` with `.sig`). Changes:

- `-u` / `--control` and `-t` / `--token` are the only required flags. The
  token prefix decides the path: `anixjoin_` joins a group, `anixagt_`
  enrolls an existing node (kept for "move this node to a new machine" and
  for credential rotation). `--node` goes away: an `anixagt_` token already
  names its node.
- New `GET /install/ip` answers the caller's address as Control sees it
  (gin `ClientIP`, which honours `server.trusted_proxies` through
  `internal/requestorigin`). The script calls it over IPv4 and IPv6 and passes
  the results to the join, because the gRPC peer address is the proxy's when
  Control sits behind one.
- After installing the binary and units, the script runs the join
  synchronously as the `anixops-agent` user:
  `anix-agent join --control <url> --grpc <target> --token-file <file> [--name] [--host] [--address …] [--reset] --json`.
  It prints the node it joined (`已接入：hk-01 (proxy-12)`) or the error, then
  writes `config.json` with `AgentNode` set and starts the service. The
  Agent's runtime model (one configured node per process, identity under
  `pki/<node>/`) does not change.
- A machine that already has a valid identity skips the join (today's
  "re-running upgrades in place").
- At the end the script reminds the operator that the command, token
  included, is in the shell history.

## Join Protocol

New RPC on the `AgentEnrollment` service (`sdk/api/agent/v1/agent_enrollment.proto`):

```proto
// Join exchanges a node group's join token and a CSR for the certificate of
// a new node of that group, or of this machine's earlier node (see
// "Rejoining"). Control ignores the CSR's subject and SANs.
rpc Join(JoinRequest) returns (JoinResponse);

message JoinRequest {
  bytes csr_der = 1;
  string join_token = 2;            // anixjoin_...
  string machine_fingerprint = 3;   // hex SHA-256 of "anixops-join-v1\n" + /etc/machine-id
  string hostname = 4;
  string requested_name = 5;        // --name
  string connect_host = 6;          // --host
  repeated string public_addresses = 7; // from GET /install/ip, IPv4 first
  string arch = 8;
  string os = 9;
  string agent_version = 10;
  string instance_id = 11;
  bool reset = 12;                  // --reset: never rejoin, always create
}

message JoinResponse {
  AgentCertificate certificate = 1;
  string node = 2;          // "proxy-<id>" | "forward-<id>"
  string node_name = 3;
  string group = 4;
  bool created = 5;         // false when the machine rejoined its node
  string approval = 6;      // "approved" | "pending"
}
```

Control, in one transaction:

1. Find the group by `join_token_hash`; refuse (`join_rejected`, no detail) if
   missing, deleted, expired or at `max_nodes`. Every refusal is audited with
   the peer address; the RPC is rate limited per peer.
2. Rejoin check (below). Otherwise create the node: name from
   `requested_name`, else `hostname`, de-duplicated within the group with a
   `-2`, `-3` suffix; host from `connect_host`, else the first
   `public_addresses` entry; the group's defaults; `approval` from
   `require_approval`.
3. For a proxy group, create the protocols of the template, generating the
   per-node secrets (Reality key pair and short IDs, Shadowsocks 2022 server
   key, WireGuard server key) and ports, and attach them to the group's
   subscription groups.
4. Record an `AgentEnrollment` with method `join_token` (new constant), issue
   the certificate exactly as `Enroll` does, write the audit entry
   (`agent_join`), and queue the administrator notification.

**Pending nodes** get their certificate and connect, but Control withholds the
`config.v1` and `users.v1` snapshots until an administrator approves them.
Rejecting a pending node soft-deletes it and revokes its certificates.

### Rejoining

A machine whose local state is gone (reinstalled Agent, `--purge`) may present
the same `machine_fingerprint`. Cloned VPS images often share `/etc/machine-id`,
so a fingerprint alone must never hand one machine another machine's node.
Control rejoins only when all of these hold:

- a live node of the same group has the same fingerprint;
- its last known public address equals one of `public_addresses`;
- it has not been seen on the Agent Control stream for 5 minutes;
- `reset` is false.

Then the old certificates are revoked and a new one is issued for that node
(`created: false`). In every other case a new node is created.

## Agent Changes

- `anix-agent join` subcommand (above). It writes the identity into the
  existing store layout (`pki/<node>/identity.pem`, `ca.pem`, `identity.json`)
  with the store's ownership and mode rules, and prints JSON for the script.
- Inbound TLS comes from Control (Phase 7). Today a node installed by the
  script has no `CertConfig`: xray and sing silently drop TLS on a TLS
  protocol and hysteria2 refuses to start.
- Removed in Phase 9: the REST panel client, the v2board gRPC client, the
  WebSocket sync, `AutoRegister`/`AuthKey`/`ApiKey`/`CredentialFile`/
  `EncryptCredential`/`EnableSign` and the other legacy node fields,
  `scripts/install.sh` and the `anix-agent.sh` manager, `anix-agent update`
  (it ran a script from the `dev_new` branch), the `V2bX` symlink, and the
  `Include` of remote URLs (broken today).

## Protocol Templates (removed)

A node group no longer carries a protocol template or subscription groups
(migration `0020_node_groups_without_templates`, along with its `kind`).
A node that joins is clean: it has no protocol and only runs the network model
(ANX). Protocols are added on the node page.

## Forward Routes In Subscriptions (Phase 10, D3)

Today a relay needs two unrelated objects: a forward route that actually
forwards, and a child proxy node (`parent_id`) that puts the relay's address
into subscriptions with the parent's protocol and Shadowsocks 2022 key. The
child never reaches a subscription: it runs no Agent, so it never reports and
the five-minute online filter drops it.

5.0 replaces the child node with an option on the route. When a route's exit
is a protocol of a proxy node, **加入订阅** publishes one subscription entry
per route entry: address and port of the entry hop, protocol, settings and
keys of the exit protocol, the exit protocol's subscription groups. The entry
is online when the route is healthy and the exit node is online. `parent_id`,
`AccumulateTrafficOnly` and the parent-derived Shadowsocks 2022 key are
removed.

Open point for that phase: a per-entry billing rate (an IEPL entry billed at
2x) needs the exit to tell entries apart, for example one inbound port per
route on the exit node. Today's `parent_id` cannot do it either.

## Soft Delete

Topologies (D6), node groups and nodes get `deleted` (integer, 0 or 1, not
null, default 0) and `deleted_at`. Rows are never removed. Queries exclude
deleted rows in one place (GORM's soft-delete flag mode, or a shared scope
covered by tests), so a forgotten filter cannot leak them. Names that must be unique become unique among live rows only
(a partial index `WHERE deleted = 0`, supported by SQLite and PostgreSQL); the
old indexes are dropped explicitly because AutoMigrate never drops one.

- **Topology:** see D6. In-progress deployments block the delete (409);
  planned ones can no longer be applied. Revisions, deployments, steps and
  operations stay for the audit trail.
- **Node:** revokes the node's certificates and enrollments, removes it from
  subscriptions and from forward routes (routes through it become invalid and
  show so), and stops its stream session.
- **Node group:** refused while the group has live nodes.

## Removals

| Area | Removed | Phase |
|---|---|---|
| Registration keys | `v2_authorized_key`, `/api/v2/admin/auth-keys`, `/api/v2/internal/auth-keys`, `NODE_DEFAULT_AUTH_KEY`, `POST /api/v2/node/register`, gRPC `NodeService.Register`, the 注册密钥 sheet, the package-native auth key routes | 8 |
| Ansible node deploy | `config/deploy/ansible/nodes/`, `deploy_from_inventory.sh`, the 部署父节点 sheet and its settings and composable | 8 |
| Manual create | 添加节点 (create mode of the node form; edit stays) | 8 |
| Legacy agent channels | `/api/v2/agent/*`, `/api/v2/node/*`, `/api/v2/forward/agent/rules`, `/api/v2/forward-agent/*`, API-key authentication on the control stream, the WebSocket agent hub, the `agent_control.mtls` setting (mTLS is the only mode) | 8 |
| Third-party agents (D2) | UniProxy, v2board gRPC, node API keys and shared secrets, `SignatureAuth`, the node 凭据 section | 8 |
| Pages | NodeX Agents (D7), Agent 连接方式 (its columns move to the node list; the upgrade campaign moves to the node page with a start button) | 8 |
| Extensions (D5) | The WebUI of 13 placeholder packages and of machine-telemetry; WebUI becomes optional for packages | 2 |
| Relay chains (D3) | `parent_id` and everything that reads it | 10 |
| Agent legacy | See "Agent changes" | 9 |

A one-time 5.0 migration drops the removed tables after the release notes tell
the operator to back up the database.

## Phases

| # | Change | Repository | Depends on |
|---|---|---|---|
| 0 | This document | Control | - |
| 1 | Topology soft delete (D6) | Control | - |
| 2 | Placeholder extensions, machine-telemetry page, 服务状态 column, 部署编排 subtitle and menu group (D5) | Control | - |
| 3 | `Join` RPC in the SDK | Control `sdk/` | - |
| 4 | Node groups, join tokens, the `Join` server, `install.sh -u/-t`, `/install/ip`, approval, notifications, node and group soft delete | Control | 3 |
| 5 | `anix-agent join` | Agent | 3 |
| 6 | 接入节点 dialog, 节点组 tab, live join list, approvals, node list columns, upgrade start button, 诊断 tab (D7) | Control web | 4 |
| 7 | Protocol templates with ACME and self-signed TLS delivered by Control | Both | 4, 5 |
| 8 | Control removals (several PRs: registration keys and Ansible and pages; legacy channels; third-party agents) | Control | 4-6 |
| 9 | Agent removals and configuration cleanup | Agent | 5 |
| 10 | Forward routes in subscriptions; `parent_id` removed (D3) | Control | 8 |
| 11 | 5.0.0: `/v5` module path, docs, upgrade notes, cleanup migration, tags on both repositories | Both | all |

Phases 1, 2 and 3 run in parallel, then 4 and 5. Acceptance at the end: the
owner runs the command on a real VPS against the production panel.

## Security Notes

- A join token lets a machine into its group: the node receives the users of
  the group's subscription groups (UUIDs, passwords) and its traffic reports
  are billed to them. Controls: reset in one click, optional expiry and node
  cap, a notification on every join, approval per group (D4), audit entries
  for every join and refusal.
- The command is in the shell history of every machine it ran on. The script
  says so at the end.
- The machine fingerprint is never authentication; it only selects which node
  a valid token may rejoin, under the conditions in "Rejoining".
- Tokens are compared by hash; the sealed copy is only opened to render the
  command for an administrator, and that read is audited.

## Not In Scope

Users, plans, subscriptions, payments, tickets, the forward route engine
itself, and the separate `control-center/workers` application. One machine
serving as both a proxy node and a forward node needs two installs today and
stays that way.
