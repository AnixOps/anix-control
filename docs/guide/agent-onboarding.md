# Installing The Agent With One Command

A machine becomes a node of Control with one command. The command carries
Control's address and a token, and the token says what the machine becomes:

- a **node group's join token** (`anixjoin_...`) makes it a new node of the
  group. The same command works on any number of machines; each becomes a
  node with the group's defaults and protocols
  ([node groups](#joining-a-node-group));
- a node's **one-time enrollment token** (`anixagt_...`) makes it that
  existing node: for moving a node to a new machine, or after a credential
  rotation ([existing nodes](#enrolling-an-existing-node)).

The install script Control serves installs the AnixOps Agent, removes the
legacy forward runtime of the machine, runs `anix-agent join` to exchange
the token for the node's certificate, writes the configuration for the node
the join named and starts the Agent
([design](../architecture/forward-sdk.md#9-node-onboarding), 5.0 node
groups: `docs/design/node-onboarding`).

## Before You Start

> **Needs an anix-agent release with `anix-agent join` (5.0).** The script
> runs `anix-agent join` to obtain the node's identity before the Agent
> starts, and writes a configuration that carries no token and no node API
> key. An Agent release without the `join` command stops the script at
> "the join failed".

On Control:

- The built-in agent CA is on (`module_runtime.ca_kek` with `pki: builtin`)
  and the gRPC listener has TLS (`grpc.enabled`, `grpc.tls_cert_file`):
  the Agent joins there (`docs/UPGRADE.md`,
  "Agent Transports: Preparing For v4.2"). Fresh installs generate the key;
  the certificate must be publicly trusted for the gRPC target, because the
  Agent verifies it against the node's system CAs and accepts no private CA
  ([release installation](release-installation.md#agent-access),
  `docs/DEPLOYMENT.md` 2.0.2). Node group join tokens are sealed with the
  same `module_runtime.ca_kek`: without it no group can be created
  (`409 ca_kek_missing`).
- Nodes reach Control over **https**. Set `agent_install.public_url` to the
  address nodes use (for example `https://panel.example.com`) unless the
  request's origin is already right (behind a reverse proxy it only is when
  the proxy is in `server.trusted_proxies`). Set `agent_install.grpc_target`
  when nodes dial the gRPC listener at another host or port than
  `public_url`'s host and `grpc.port`. Behind a reverse proxy, also list it
  in `server.trusted_proxies`: the script asks `GET /install/ip` for the
  machine's address, and only a trusted proxy's forwarded address counts.
- The install paths (`/install.sh`, `/install.sh.sig`, `/install/agent.env`,
  `/install/ip` and `/install/agent/<tag>/<asset>`) are served on both
  listeners, the API port (8080) and the UI port (3000, which forwards them),
  so a reverse proxy that sends the whole host to the UI port serves them too.
- Only a super administrator (an administrator who is not staff and not
  banned) can show a group's command or reset its token, and issue
  enrollment tokens.

On the node: Linux on amd64 or arm64 with systemd 240 or later, root
(sudo), `curl`, `sha256sum`, `runuser` or `setpriv` (util-linux) and
`unzip` (or `python3`); for an offline bundle also `tar` and OpenSSL 3. A
forward node needs Linux 5.10 and nftables 0.9.7 or later. OpenRC is not
supported yet. The script checks all of this before it changes anything
([preflight](#preflight-checks)).

## Joining A Node Group

A node group owns a join token and the defaults of the nodes it creates.
Its kind is `proxy` (the machines serve users: `v2_node` rows) or `forward`
(entry and relay machines of forward routes: forward nodes).

### In The Console

**节点 › 接入节点** is the one entry point for a new machine (it replaces
the registration key, the parent-node Ansible helper and adding a node by
hand):

1. **Control checks itself first.** Before it shows a command it checks,
   with no network call, that the command can work: the address nodes reach
   is https, the gRPC target resolves and its listener has TLS, the CA key
   (`module_runtime.ca_kek`) is usable, `agent_control.mtls` is not `off`,
   the pinned Agent release has a source, and the install script's
   signature verifies when one is configured. A failing check is shown with
   what to change instead of a command that would fail; a failing warning
   (the signature, an `artifact_dir` without the release) sits above the
   command. The same answer is `GET /api/v4/kernel/agents/install-readiness`.
2. **A node group is picked or made.** On a panel with no node group the
   first open creates the default one without asking for anything: **默认**,
   a proxy group whose template is one VLESS Reality protocol on a random port
   from 20000 to 40000 (Reality site `www.microsoft.com`), in the default
   subscription group, so the first node is in subscriptions as soon as it
   joins. Edit it later under **节点组**. **新建节点组** makes another one with
   the minimum: a name, proxy or forward and, for a proxy group, one of three
   protocol presets (it joins the default subscription group, and the
   others stay one click away in the group's sheet).
3. **The command.** Super administrators see it with the download source
   tabs (control, mainland mirror, GitHub), a copy button, when the token
   ends and **重置令牌** (after a confirmation: every earlier command stops
   working, nodes that joined keep their certificates). The uninstall command
   is under it. Other administrators see a note that only a super
   administrator can show the command.
4. **Machines appear as they join.** The list under the command shows
   "等待机器接入…" and then "hk-01 已接入" with the node's id, address,
   approval and whether it is online (polled every three seconds while the
   dialog is open). In a group that requires approval the machine carries
   **批准** and **拒绝**; the node list has a **待确认** filter, a badge on the
   row and a banner for every machine that waits (forward groups included).

**节点 › 节点组** lists the groups: kind, live and pending nodes, the approval
switch, when the token ends and the row actions **接入节点** (the dialog for
that group), **编辑** and **删除** (refused while the group has nodes). The
edit sheet holds the defaults the nodes copy (rate, tags, monthly limit and
reset day, sort), the token's validity, the node cap, approval, the
subscription groups and the protocol template: each protocol's type, name,
transport, TLS (none, or Reality for VLESS), fixed port or a range, and the
Reality site. A changed template applies to machines that join afterwards.

### Using The API

The console uses the same routes
([reference](api-reference.md#节点组与接入令牌)):

1. Create the group (any administrator):

   ```sh
   curl -fsS -X POST https://panel.example.com/api/v4/kernel/node-groups \
     -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' -d '{
       "name": "HK", "kind": "proxy", "rate": 1, "tags": ["hk"],
       "require_approval": false, "max_nodes": 0,
       "subscription_group_ids": [1],
       "protocol_template": [
         {"name": "VLESS Reality", "type": "vless", "port": 443, "tls": "reality",
          "reality": {"server_name": "www.example.com", "dest": "www.example.com:443"},
          "settings": {"flow": "xtls-rprx-vision"}},
         {"name": "SS 2022", "type": "shadowsocks", "port_range": [20000, 40000],
          "settings": {"method": "2022-blake3-aes-128-gcm"}}
       ]}'
   ```

2. Show its command (super administrators; every read is in the operation
   log as `node_group_join_token_reveal`):

   ```sh
   curl -fsS https://panel.example.com/api/v4/kernel/node-groups/1/command \
     -H "Authorization: Bearer $ADMIN_TOKEN"
   ```

   The answer lists the command for each download source (`control`,
   `cn`, `github`, as in [enrolling an existing node](#enrolling-an-existing-node)),
   the token, its expiry and the uninstall command.

3. Run it on each machine as root:

   ```sh
   curl -fsSL https://panel.example.com/install.sh | sudo bash -s -- -u https://panel.example.com -t anixjoin_8Kq...
   ```

   | Flag | Meaning |
   |---|---|
   | `-u`, `--control` | Control's address (in the copied command) |
   | `-t`, `--token` | the join token (in the copied command); `ANIX_AGENT_TOKEN` in the environment works too and keeps it out of the shell history |
   | `--name hk-01` | the new node's name; default: the machine's host name. Control makes it unique in the group with `-2`, `-3` |
   | `--host 203.0.113.5` | the address clients connect to; default: the machine's public address as Control sees it |
   | `--mirror cn\|github`, `--offline <bundle>` | where the Agent comes from ([sources](#enrolling-an-existing-node), [offline](#installing-without-internet-access)) |
   | `--reset` | discard this machine's identity and join as a new node |
   | `--forward` | turn IP forwarding on for a proxy node too; a forward group's node always gets it |
   | `--timeout 180` | how long the join may take, in seconds |

   The script prints the node it joined, for example
   `已接入：hk-01 (proxy-12)`, and reminds you that the command, token
   included, is in the shell history.

What Control does with a join, in one transaction:

1. It finds the group by the token's SHA-256. An unknown token, a deleted
   group, an expired token or a group at `max_nodes` gets one answer,
   `join_rejected`, without detail; the operation log has the reason and
   the machine's address (`agent_join_rejected`). Joins are rate limited
   per address.
2. A machine that lost its local state (a reinstall, `uninstall --purge`)
   [rejoins](#rejoining) its earlier node when the rule holds.
3. Otherwise it creates the node: the name and host above, the group's
   rate, tags, monthly traffic limit and reset day and sort, and, in a
   proxy group, the template's protocols with their own secrets and ports,
   in the group's subscription groups.
4. It issues the node's certificate (enrollment method `join_token`) and
   writes `agent_join` to the operation log; after the commit every
   administrator gets a notification (in-app, e-mail when configured,
   Telegram when bound): `节点已接入：hk-01 (proxy-12)`, or `节点待审批：...`.

**Protocol templates.** Each entry of `protocol_template` is one protocol
every new node of the group gets. Control generates each node's secrets:
the Reality key pair and short ID, the Shadowsocks 2022 server key, the
WireGuard server key pair; a template that sets one is refused.

| Field | Meaning |
|---|---|
| `name` | the protocol's name (default: its type) |
| `type` | `vless`, `vmess`, `shadowsocks` or `wireguard` in this release |
| `port` or `port_range` | a fixed port, or `[from, to]`: a random port per node, distinct from the node's other protocols. Control cannot see which ports are busy on the machine; a port in use shows in the node's runtime health, and you edit it |
| `transport` | `tcp` (default), `ws`, `grpc`, `h2`, `httpupgrade` for VLESS and VMess; Reality runs on `tcp`, `grpc` and `h2` |
| `tls` | `none` (default) or `reality` (VLESS). `acme` and `self_signed` need certificates on the node and come in a later release |
| `reality` | `server_name`, `dest` (default `<server_name>:443`), `fingerprint` (default `chrome`) |
| `settings`, `transport_settings` | JSON objects copied to the protocol (for example `flow`, a Shadowsocks `method`, a WebSocket `path`) |

Changing a group's template applies to later joins; nodes that joined keep
their protocols.

**The token.** The command can be shown again at any time (owner decision
D8): Control keeps the token's SHA-256 and a copy sealed with AES-256-GCM
under a key derived from `module_runtime.ca_kek`. `POST
/api/v4/kernel/node-groups/<id>/reset-token` replaces it: every earlier
command stops working, nodes that joined keep their certificates.
`token_expires_at` and `max_nodes` bound a group's token; deleting a group
(refused while it has live nodes) ends it.

**Approval.** A group with `require_approval` creates its nodes pending:
they get their certificate and connect, but Control sends them neither
their configuration nor their users, and offers their protocols to no
subscriber, until an administrator approves them
(`POST /api/v4/kernel/nodes/<node>/approve`). Rejecting one
(`.../reject`) disables it and revokes its certificates. The script says
so when it joined a pending node.

### Rejoining

A machine whose local state is gone presents the same machine fingerprint
again (a SHA-256 of its `/etc/machine-id`). Cloned images often share
`/etc/machine-id`, so the fingerprint alone never hands a machine another
machine's node. Control rejoins the earlier node only when all of these
hold:

- a live, enabled node of the same group has the same fingerprint;
- the public address it joined from is one of the machine's addresses now;
- it has not been seen on the Agent Control stream (or by a heartbeat) for
  5 minutes;
- `--reset` was not given.

Then the node's old certificates are revoked and a new one is issued for it
(`rejoined proxy-12` in the script's summary). In every other case a new
node is created; the earlier node stays until you delete it.

## Enrolling An Existing Node

To move a node to a new machine, or to enroll it again after its identity
was revoked, issue a single-use enrollment token for the node (1 hour by
default, at most 7 days; super administrators): **节点 › node › 部署 ›
换到新机器** for a proxy node, the forward node's page for a forward node,
or `POST /api/v4/kernel/agents/install-tokens` with
`{"node": "proxy-12"}`. The token is shown once and works once.

1. Pick the download source and copy its command:

   ```sh
   curl -fsSL https://panel.example.com/install.sh | sudo bash -s -- \
     -u https://panel.example.com -t anixagt_...
   ```

   | Source | Script from | Agent from | Checksum from |
   |---|---|---|---|
   | `control` (default) | Control | Control when it holds the release (`agent_install.artifact_dir`), else GitHub | Control, else GitHub |
   | `cn` (`--mirror cn`) | Control | the mainland mirror (`agent_install.cn_mirror_url`), else Control, else GitHub | Control, else GitHub |
   | `github` (`--mirror github`) | GitHub release asset `agent-install.sh` | GitHub | GitHub |

   A mirror never supplies the checksum it is checked against. The token
   names its node, so the command has no node flag.
2. Paste it on the node as root (add `--reset` on a machine that runs the
   Agent of another node). The script prints what it did:

   ```text
   [anixops] 已接入：relay-41 (forward-41)
   [anixops] AnixOps Agent v5.0.0 is running (install)
     node:        forward-41 (relay-41)
     identity:    spiffe://anixops/prod/agent/forward-41 (serial ..., expires ...)
     join:        enrolled as forward-41
     forwarding:  wrote /etc/sysctl.d/90-anixops-forward.conf (net.ipv4.ip_forward=1 net.ipv6.conf.all.forwarding=1); applied
     legacy:      removed
                    - nftables table inet v2b_forward
                    - systemd unit v2forward-agent.service
   ```

The Agent then shows as connected over mTLS in the node list's connection and
certificate columns and on the node's page (`anix-control agents transports`
lists every node).

## What The Script Does

1. Checks its arguments, root, the architecture and systemd; a first install
   without a token (`-t`) stops here. A machine that ran the script before
   knows its node from `/etc/anixops/agent/config.json`.
2. Reads `https://<control>/install/agent.env`: the Agent release (the one
   this Control was released with, `config/AGENT_RELEASE`: a Control patch
   release can ship an unchanged Agent, so it need not carry the Control's
   number; `agent_install.agent_version` overrides it), the gRPC target, the
   mirrors and, when Control holds the release, its SHA-256. With
   `--offline` it reads the bundle instead.
3. Runs the [preflight checks](#preflight-checks) and stops, with a reason
   and a fix for each failed check, before anything changes.
4. Downloads the release, checks its SHA-256 and, when the release has a
   `.sig`, its Ed25519 signature by the official release key (an offline
   bundle: both signatures are required). Nothing on the machine changes
   before these checks pass.
5. Creates the system users `anixops-agent` (in the `anixops-gost` group,
   which guards gost's API socket) and `anixops-gost`.
6. Stops a running Agent (forwarding keeps running), installs
   `/usr/lib/anixops-agent/anix-agent` (and the pinned gost when the release
   ships it) with `/usr/local/bin/anix-agent` linking to it.
7. Creates the Agent's directories: its state in `/var/lib/anixops-agent`
   (0700), its key and certificate in `/var/lib/anixops-agent/pki`, the
   gost driver's files in `/var/lib/anixops-gost` (0750). A machine that
   runs, or ran, the Agent of anix-agent's own root installer (a unit
   without `User=anixops-agent`, or `/var/lib/anix-agent` or
   `/var/lib/anixops/plugins`) first gets
   `anix-agent migrate-paths --chown anixops-agent`: the Agent's identity,
   stream state and plugins are copied to `/var/lib/anixops-agent`, owned
   by the Agent's user, which cannot read the old root-owned directories.
   The old directories stay. A failed copy stops the script before the
   Agent starts.
8. Removes the legacy forward runtime of this machine: the nftables tables
   `inet v2b_forward`, `ip v2b_forward` and `ip anixops_forward`, and the
   clean agent (`v2forward-agent.service`, `/etc/v2board-forward-agent`,
   `/usr/local/bin/v2forward-agent`). It touches no other table or service.
9. Writes `anix-agent.service` and `anixops-gost.service`, and a polkit rule
   that lets `anixops-agent` start, stop and reload `anixops-gost.service`
   only; enables them.
10. Joins Control, unless the machine already holds a valid identity (a
    re-run): asks `https://<control>/install/ip` over IPv4 and IPv6 for the
    machine's public addresses (a family that does not answer is left out),
    writes the token to `/var/lib/anixops-agent/join.token` (0600, the
    Agent's) and runs, as `anixops-agent` and within `--timeout` (180 s):

    ```sh
    anix-agent join --control <url> --grpc <grpc_target> --token-file /var/lib/anixops-agent/join.token \
      [--name N] [--host H] [--address A]... [--reset] --pki-dir /var/lib/anixops-agent/pki --json
    ```

    The token file is removed as soon as the join returns. The join writes
    the key and certificate into the identity store and answers the node it
    joined (`proxy-<id>` or `forward-<id>`), its name, its group and whether
    it waits for approval; a failed join stops the script with Control's
    reason before the Agent starts.
11. Writes `/etc/anixops/agent/config.json` (root:anixops-agent, 0640) with
    Control's address, the gRPC target, that node and the identity paths:
    no token and no API key. A configuration for another node is kept as
    `config.json.bak.<time>`.
12. On a forward node the join made (its kind is known only now), runs the
    forward [preflight checks](#preflight-checks); a failure stops the
    script before the Agent starts, and the same command continues once it
    is fixed (the identity is kept).
13. On a forward node (`forward-<id>`, or a proxy node with `--forward`)
    turns on IP forwarding, which nftables DNAT and gost relays need: writes
    `/etc/sysctl.d/90-anixops-forward.conf` (`net.ipv4.ip_forward = 1`,
    `net.ipv6.conf.all.forwarding = 1`, as `anix-agent forward
    sysctl-dropin` prints it) and applies it with
    `sysctl -e -p` (`-e`: a host without IPv6 still gets IPv4). When it
    cannot apply now, the summary notes it and the file applies at the next
    boot. A proxy node without `--forward` is left as it is. With
    `--accept-ra` it adds `net.ipv6.conf.<if>.accept_ra = 2` for the
    interfaces that have an IPv6 default route from router advertisements
    (SLAAC), and later runs keep those lines.
14. Starts the Agent and prints the summary, and, when the token came on the
    command line, a reminder that the command is in the shell history.

The Agent runs as `anixops-agent`, not root, with only `CAP_NET_ADMIN` and
`CAP_NET_BIND_SERVICE`, `NoNewPrivileges`, `ProtectSystem=strict` (writable:
`/var/lib/anixops-agent`, `/var/lib/anixops-gost`, and its
`RuntimeDirectory` `/run/anixops-agent` (0750) for the plugins' sockets),
`ProtectHome`,
`PrivateTmp` and `RestrictAddressFamilies=AF_INET AF_INET6 AF_NETLINK
AF_UNIX`. gost runs as `anixops-gost` with `CAP_NET_BIND_SERVICE` only.

## Running It Again

The same command is safe to repeat. With a valid identity it upgrades the
Agent in place, keeps the identity and the configuration, and leaves the
token unused (`already joined as proxy-12; the token was not used`). A
failed run can be re-run after fixing the cause. Re-runs run the preflight
checks too: a check that now fails (for example a moved gRPC port) stops
the upgrade until it is fixed or `--skip-preflight` is given.

To move the machine to another node, or to join again after its identity
was revoked, run a command with a token for the new node or group and add
`--reset`: the script discards the identity, joins with the token (with a
join token Control then always creates a new node) and rewrites the
configuration (the old one is kept as `config.json.bak.<time>`).

## Rotating A Node's Credentials

When a node's identity may have leaked (a stolen disk, a key left on a build
host) rotate its credentials instead of waiting for the certificate to
expire. One call, for super administrators only
([reference](../reference/node-credential-rotation.md)):

```sh
curl -fsS -X POST https://panel.example.com/api/v4/kernel/agents/rotate-credentials \
  -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' \
  -d '{"node":"proxy-12","rotate_api_key":true,"reason":"disk of the host was stolen"}'
```

Control, in one transaction:

1. revokes every Agent certificate, enrollment (an unused enrollment token
   included) and forward link certificate of the node;
2. with `rotate_api_key` on a proxy node, replaces the node's API key;
3. issues a fresh single-use enrollment credential (`anixagt_...`), valid
   for one hour unless `ttl_seconds` says otherwise (at most 7 days), and
   shows it in the answer, once.

It never contacts the node, so it works while the node is offline. A
running Agent loses its stream at its next heartbeat
(`agent_cert_revoked`); an Agent that is off finds out when it next
connects. Repeating the call revokes the previous credential and issues
another: only the last one enrolls.

Give the credential to the machine and enroll it again with the install
command of the node, adding `--reset` (the old identity is discarded):

```sh
curl -fsSL https://panel.example.com/install.sh | sudo bash -s -- \
  -u https://panel.example.com -t anixagt_... --reset
```

**The node's API key.** The Agent holds a certificate, not the key, but the
node's configuration may still hold it, and an Agent configured with it
(the way Agents were installed before the credential-only configuration)
enrolls again by itself with it after its certificate is revoked. That is
the recovery of a healthy node, and also why a rotation without
`rotate_api_key` does not lock out whoever holds the key: the old key keeps
working, so it can still enroll an Agent. If the key may be exposed, set
`rotate_api_key`. It is opt-in because replacing the key breaks every
process that still polls Control with it until it is given the new one, and
an Agent that holds only the old key can then enroll only with the new
credential (the install command with `--reset` needs the anix-agent release
named in [Before You Start](#before-you-start)). Read the new key with the
audited `GET /api/v2/admin/nodes/<id>/credentials` (action `reveal`); the
rotation answer never carries it. `rotate_api_key` is not accepted for a
forward node: its token belongs to the frozen legacy forward runtime.

**In the console.** The same action is *Rotate credentials…* in the node
page's Credentials section (and on a forwarding node's page, for a node with
an Agent). It asks first, says what is revoked, takes the reason, how long the
credential lives and, for a proxy node, whether to replace the API key too,
then shows the new credential once, masked and with a copy button, together
with when it expires. Closing the dialog clears it. The console does not know
whether you are a super administrator: a refusal appears in the dialog. It
shows no install command; give the credential to the machine with the
command above.

Every rotation is in the operation log as `agent_credentials_rotate` (with
your reason and what was revoked) and `agent_enrollment_token_issue`,
without any credential. A disabled node answers `node_disabled`: enable it
first.

## Preflight Checks

After reading Control's metadata (or the offline bundle) and before it
changes anything, the script checks the machine. Each check prints `ok`,
`skip`, a warning (`note: preflight: ...`, repeated in the summary) or
`FAIL` with a `fix:` line. When a check fails the script stops after
running all of them and says how many failed; nothing on the machine has
changed. `--skip-preflight` installs anyway and notes it in the summary.

The forward checks (kernel 5.10, nftables, polkit, firewalls, IPv6 SLAAC)
need to know that the machine forwards. Before a fresh join they run up
front only with `--forward` or on a machine whose configuration names a
forward node; a machine that a join makes a forward node gets them right
after the join, before its Agent starts (step 12 above). Pass `--forward`
to have them checked before anything changes.

| Check | Fails when | Warns when |
|---|---|---|
| systemd (`systemctl --version`) | older than 240 (the units use `Type=exec`) | older than 247 (`ProtectProc=` and `ProcSubset=` are ignored: a weaker gost sandbox) |
| Kernel (`uname -r`) | older than 5.10 on a forward node (the nftables driver's comments) | older than 5.10 on a proxy node |
| nftables (forward nodes) | `nft` missing or older than 0.9.7 | `tc` missing (no bandwidth limits); `nf_conntrack` neither loaded nor a module |
| polkit (forward nodes) | | `pkaction` missing, or older than 0.106: the Agent cannot start or reload `anixops-gost.service`, so gost hops fail (nftables hops work) |
| Firewalls (forward nodes) | | firewalld active; ufw enabled with `DEFAULT_FORWARD_POLICY="DROP"`; the iptables `FORWARD` policy is `DROP` (Docker sets it) |
| IPv6 SLAAC (forward nodes) | | an interface with a default route from router advertisements has `accept_ra` other than 2: with forwarding on the kernel ignores router advertisements and the route expires. Re-run with `--accept-ra` |
| Ports | | with `--port-range FROM-TO` (the node's forward port range in Control), a listener in that range (`ss`). The Agent itself listens on no port; without the flag the check is skipped |
| Disk | less than 200 MiB free for `/usr/lib/anixops-agent` or the download directory | |
| Control over https | online: `/install/agent.env` cannot be read with a verified certificate (stops before preflight) | offline: Control's https address is unreachable |
| gRPC target | it does not resolve, refuses or times out, its certificate does not verify against the system CAs (the Agent checks it the same way), or the TLS handshake fails | `curl` has no HTTP/2 and the handshake failed, or the result is inconclusive |
| Clock | more than 5 minutes off Control's `Date` header (certificates stop verifying) | more than 30 seconds off, or no `Date` header |

The script writes no polkit `.pkla` file for polkit 0.105 (Ubuntu 22.04):
a `.pkla` grants an action for every unit, so it would let the Agent start
or stop any service. On such a machine the gost driver cannot run; upgrade
polkit (Ubuntu 24.04, Debian 12) to forward with gost.

## Installing Without Internet Access

A node that cannot download from Control's mirrors or GitHub installs from
an offline bundle. The node still needs to reach Control's gRPC target to
join; only the downloads move to the bundle.

1. Put the Agent release in `agent_install.artifact_dir` on Control
   (`<dir>/<tag>/`): the zips and their `.sig` files, `SHA256SUMS` and
   `SHA256SUMS.sig` from the anix-agent release.
2. On Control, write the bundle for the node's architecture:

   ```sh
   anix-control agent offline-bundle -arch amd64 -o agent-offline-amd64.tar.gz
   ```

   `-control https://<control>` names the address nodes reach when
   `agent_install.public_url` is not set. The command checks both
   signatures with `plugins.official_public_key` and that `SHA256SUMS`
   lists the zip's digest, and prints the files and the install command.
3. Copy the bundle and `install.sh` to the node (the bundle contains the
   script too), copy the command (a group's or a node's), and run it with
   `--offline`:

   ```sh
   sudo bash install.sh --offline agent-offline-amd64.tar.gz \
     -u https://panel.example.com -t anixjoin_...
   ```

   The join still needs Control's gRPC target. When Control's https address
   cannot be reached either, the script passes no public address, and a new
   node's host is the address Control's gRPC listener saw: give `--host`.

The bundle is a `tar.gz` of flat files:

| File | What it is |
|---|---|
| `agent.env` | Control's `/install/agent.env` (the release, the gRPC target) |
| `anix-agent-linux-64.zip` or `anix-agent-linux-arm64-v8a.zip`, with `.sig` | the Agent release for one architecture and its signature |
| `SHA256SUMS`, `SHA256SUMS.sig` | the release's checksums and their signature |
| `install.sh`, `install.sh.sig` | the install script and its release signature (when Control has it) |

Without Control's command, build it from the release assets on a connected
machine: download the four release files for the architecture, `agent-install.sh`
(as `install.sh`) and `curl -fsSL https://<control>/install/agent.env -o agent.env`,
then `tar -czf agent-offline-amd64.tar.gz agent.env anix-agent-linux-64.zip anix-agent-linux-64.zip.sig SHA256SUMS SHA256SUMS.sig install.sh`.

The script refuses any other entry, and verifies a bundle with the same
official key as a download, but every check is required: `SHA256SUMS.sig`
must verify, `SHA256SUMS` must list the zip with its digest, and the zip's
`.sig` must verify (OpenSSL 3 is needed). The bundle's `agent.env` is not
signed and never supplies a checksum.

## Uninstalling

```sh
curl -fsSL https://panel.example.com/install.sh | sudo bash -s -- uninstall
curl -fsSL https://panel.example.com/install.sh | sudo bash -s -- uninstall --purge
```

`uninstall` stops and disables `anix-agent.service`, then
`anixops-gost.service`, removes both units, the polkit rule,
`/usr/lib/anixops-agent` (the Agent and gost) and the
`/usr/local/bin/anix-agent` link. It keeps the identity, configuration and
state (`/etc/anixops/agent`, `/var/lib/anixops-agent`,
`/var/lib/anixops-gost`), the users and the sysctl drop-in, so that the
same install command later re-installs the Agent with its identity. The
forwarding rules in the nftables table `inet anixops_fwd` stay in the
kernel until a reboot or `--purge`.

`uninstall --purge` also removes:

- the nftables table `inet anixops_fwd`, only when it carries the driver's
  ownership comment `anixops-forward-driver v1` (otherwise it is listed as
  kept);
- root HTB qdiscs with the driver's handle `af00:` on the configured
  `LimitInterfaces` and every other interface; a qdisc with another handle
  is never touched;
- `/var/lib/anixops-agent`, `/var/lib/anixops-gost`, `/etc/anixops/agent`
  and `/etc/sysctl.d/90-anixops-forward.conf` (IP forwarding stays on until
  the next boot; `sysctl -w net.ipv4.ip_forward=0` turns it off now);
- the users and groups `anixops-agent` and `anixops-gost`.

No other table, qdisc, service or file is touched. The script cannot tell
Control: the node keeps its enrollment until you **revoke its credentials or
delete the node** in Control, which revokes the Agent certificate.

## Verifying The Script

`install.sh` is the same file on every Control and in every release
(`agent-install.sh` among the release assets), signed with the official
release key (Ed25519, the key of `plugins.official_public_key`):

```sh
curl -fsSLO https://panel.example.com/install.sh
curl -fsSLO https://panel.example.com/install.sh.sig
printf '%s' 'MCowBQYDK2VwAyEAjW26nr2tbthASoeq6RmIpx8Ah+uhPNIv9V1ewRVb1VE=' | base64 -d >official.der
base64 -d install.sh.sig >install.sh.sig.bin
openssl pkeyutl -verify -pubin -keyform DER -inkey official.der -rawin \
  -in install.sh -sigfile install.sh.sig.bin
sudo bash install.sh -u https://panel.example.com -t anixjoin_...
```

Control serves `/install.sh.sig` only when the signature verifies its script
(the release image ships it; `agent_install.signature_file`); otherwise use
the release assets. OpenSSL 3 is needed for `-rawin`.

## Serving The Agent From Control Or A Mirror

To let nodes download the Agent from Control (`--mirror control`), put the
Agent release in `agent_install.artifact_dir` as `<dir>/<tag>/<asset>` with
the `.dgst` files (and `.sig` files when published):

```text
/srv/anixops/agent/v4.2.0/anix-agent-linux-64.zip
/srv/anixops/agent/v4.2.0/anix-agent-linux-64.zip.dgst
/srv/anixops/agent/v4.2.0/anix-agent-linux-arm64-v8a.zip
/srv/anixops/agent/v4.2.0/anix-agent-linux-arm64-v8a.zip.dgst
```

For offline bundles also put `SHA256SUMS`, `SHA256SUMS.sig` and the zips'
`.sig` files there ([offline](#installing-without-internet-access)).
Control then serves the zips at `/install/agent/<tag>/<asset>` and publishes
their SHA-256 in `/install/agent.env`, which also lets the `cn` mirror work
without GitHub. A mainland mirror (`agent_install.cn_mirror_url`) mirrors the
GitHub release downloads: `<base>/<tag>/<asset>`.

## Upgrading Agents From Control

Control upgrades the Agents it installed: a super administrator starts a
campaign, and Control pushes the release to the nodes in batches (5%, then
25%, then all of them, at least 30 minutes each, the same canary nodes
every time). A batch in which more than 5% of the nodes fail is rolled
back: those nodes go back to their previous Agent and the campaign stops.
A node never upgrades on its own.

```bash
# The release of this Control, from agent_install.artifact_dir (verified):
anix-control agent upgrade start -reason "v4.2.0" [-exclude forward-3] [-exclude-tag edge]
anix-control agent upgrade status
anix-control agent upgrade pause -id <campaign>
anix-control agent upgrade resume -id <campaign>
anix-control agent upgrade abort -id <campaign> [-rollback]
```

The same is `POST /api/v4/kernel/agents/upgrades` (and `GET`, `/:id`,
`/:id/pause`, `/:id/resume`, `/:id/abort`). In the console, **升级节点**
(⌘K → 升级节点) shows the Agent release this
Control pins, how many nodes would be upgraded and the batches (5%, 25%, 100%
by default; the 30 minute minimum cannot be changed), starts the campaign
after a confirmation and shows the latest one (super administrators start,
pause, resume and abort). Requirements:

- the release in `agent_install.artifact_dir/<tag>/` with its signatures
  (see [serving the Agent from Control](#serving-the-agent-from-control-or-a-mirror));
- `agent_install.public_url`: nodes download the release from Control;
- Agents installed by this release's script (it writes
  `anixops-agent-updater.path` and `anixops-agent-updater.service`, the root
  updater the Agent hands the verified release to). Older Agents are
  skipped (`upgrade_unsupported`): re-run the install command on them once.

Per-node results in `status`:

| State or code | Meaning |
|---|---|
| `skipped` `upgrade_unsupported` | the Agent cannot be upgraded by Control: re-run the installer |
| `skipped` `node_offline` | the node was offline for its whole batch; the next campaign retries it |
| `skipped` `offer_failed` | the Agent was connected but never acknowledged the upgrade during its batch: check its stream (`journalctl -u anix-agent.service`) |
| `failed` `rejected`, `apply_failed` | the Agent refused or failed (the message carries its `upgrade_*` code: download, digest, signature, updater) |
| `failed` `reconnect_timeout` | no reconnect with the new version within 10 minutes: `journalctl -u anixops-agent-updater.service -u anix-agent.service` |
| `failed` `reverted` | the Agent came back with its old version: the updater could not start the new one and reinstated the old |
| `failed` `config_apply_failed` | the new Agent could not apply the node's configuration |
| `rolled_back` | the batch failed and this node returned to its previous Agent |

## When An Agent Certificate Was Not Renewed

An Agent renews its certificate (7 days) at two thirds of its life, over the
connection it already has. When a node's Agent has not, Control raises the
alert `agent_certificate_expiring` (and `link_certificate_expiring` for a
forward node's link certificate): a notification to the administrators and an
entry in `GET /api/v4/kernel/alerts`
([alert reference](../reference/kernel-alerts.md)). The alert comes about 28
hours before the certificate ends, as a warning, and turns critical in the
last 7 hours and after the end.

1. **Look at the node first.** The usual cause is a node that is off or
   cannot reach Control's gRPC target. Once the Agent connects again with a
   certificate that has not ended, it renews by itself and the alert clears
   within one scan (15 minutes by default).
2. **The certificate ended.** An Agent whose certificate ended cannot
   connect or renew. Enroll it again: issue a new install command on the
   node's page (or `POST /api/v4/kernel/agents/install-tokens`) and run it on
   the node with `--reset`, as in [Running It Again](#running-it-again). A
   node of a group can also rejoin with the group's command when the
   [rejoin rule](#rejoining) holds. The alert clears when the new
   certificate exists.
3. **The node is gone for good.** Disable or delete the node: its
   certificates are revoked and the alert resolves. A revoked certificate
   never alerts.

The same page lists the other alerts (CA near its end, a node credential
split or an identity cutover left unfinished) with what clears each.

## Troubleshooting

| Message | Cause and fix |
|---|---|
| `agent_install_unconfigured` on the node page | Control does not know an https address nodes reach: set `agent_install.public_url`; on a development build also `agent_install.agent_version` |
| the Agent download answers 404 | `/install/agent.env` names an Agent tag the Agent repository does not have. A release build names the Agent release it was released with; an older Control (4.2.1 and before) asks for `v` + its own version, so set `agent_install.agent_version` to an existing tag such as `v4.2.0` |
| `agent_pki_disabled` | the built-in agent CA is off: set `module_runtime.ca_kek` |
| `node_disabled` on a rotation | enable the node first ([rotation](#rotating-a-nodes-credentials)) |
| `cannot reach .../install/agent.env` | the node cannot reach Control over https (DNS, firewall, certificate) |
| `cannot get the checksum of ...` | `--mirror cn` without GitHub access: put the release in `agent_install.artifact_dir` |
| `the join failed` with `join_rejected` | the group's token was reset, has expired, its group was deleted or is at `max_nodes`: copy the current command (`GET /api/v4/kernel/node-groups/<id>/command`); the operation log's `agent_join_rejected` entry has the reason |
| `the join failed` with `agent_enrollment_rejected` | the enrollment token was used, expired or revoked, or its node is disabled: issue a new one |
| `the join did not finish within 180s` | the gRPC target must be reachable with TLS from this machine; `--timeout` gives the join longer |
| `ResourceExhausted` from the join | too many joins from this address in a short time: wait a minute and re-run |
| `already joined as proxy-12; the token was not used` | the machine keeps its node: add `--reset` to join with the token instead |
| `--node is gone`, `--group is gone` | an old command: copy the current one (`-u <control> -t <token>`) |
| `ca_kek_missing` when creating a group | set `module_runtime.ca_kek`: join tokens are sealed with it |
| `preflight found N problem(s)` | each `FAIL` line above has a `fix:`; re-run after fixing, or add `--skip-preflight` |
| `cannot connect to the gRPC target` | open `grpc.port` from the node to Control, or set `agent_install.grpc_target` to an address nodes reach |
| `the certificate of the gRPC target ... does not verify` | the gRPC listener needs a certificate for that host from a CA in the node's trust store |
| `this host's clock is ...s off Control's` | `timedatectl set-ntp true` (or chrony/ntpd) |
| `the release signature of SHA256SUMS does not verify` | the offline bundle was altered or built from another release; make it again |
| `the offline bundle has no ... : it is not for arm64` | make the bundle with `-arch` of the node |

## Not In This Release

- The console's pages for node groups (接入节点, the 节点组 tab, the list of
  pending nodes): use the API above until they ship.
- Protocol templates with `tls` `acme` or `self_signed`, and the types that
  need a certificate on the node (Trojan, Hysteria2, TUIC, AnyTLS).
- Deleting a rejected node: rejecting disables it and revokes its
  certificates; the node row stays until node deletion becomes a soft
  delete.
- OpenRC.
- gost forwarding on polkit older than 0.106 (Ubuntu 22.04 ships 0.105,
  which reads `.pkla` files instead of `/etc/polkit-1/rules.d`): preflight
  warns, the script installs no rule, and the Agent cannot start or reload
  `anixops-gost.service` itself (see [preflight](#preflight-checks)).
- `anix-agent uninstall --purge` removing the `anixops-agent` and
  `anixops-gost` users and the forwarding objects (`inet anixops_fwd`, the
  `af00:` qdiscs). The Agent's own command (4.2.0-rc.1 and later) removes
  what the installer wrote, like `install.sh uninstall [--purge]`, and with
  `--purge` also the configuration, identity and state, but it lists those
  users and objects as kept; use `install.sh uninstall --purge` to remove
  them.
