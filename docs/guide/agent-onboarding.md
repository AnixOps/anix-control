# Installing The Agent With One Command

A node joins Control with one command copied from its page: Control issues a
single-use enrollment token bound to the node, and the install script it
serves installs the AnixOps Agent, removes the legacy forward runtime of the
machine and waits until the Agent has enrolled
([design](../architecture/forward-sdk.md#9-node-onboarding)).

## Before You Start

> **Needs the next anix-agent release.** The script writes a credential-only
> configuration (Control's address, the node and the enrollment token; no
> node API key and no proxy cores). The anix-agent release that accepts it
> is not out yet: today's Agent starts and refuses that configuration
> (`ApiKey is required`) and the script stops at "did not enroll". Until
> then, keep installing nodes the way `docs/UPGRADE.md` describes.

On Control:

- The built-in agent CA is on (`module_runtime.ca_kek` with `pki: builtin`)
  and the gRPC listener has TLS (`grpc.enabled`, `grpc.tls_cert_file`):
  the Agent enrolls there (`docs/UPGRADE.md`,
  "Agent Transports: Preparing For v4.2").
- Nodes reach Control over **https**. Set `agent_install.public_url` to the
  address nodes use (for example `https://panel.example.com`) unless the
  request's origin is already right (behind a reverse proxy it only is when
  the proxy is in `server.trusted_proxies`). Set `agent_install.grpc_target`
  when nodes dial the gRPC listener at another host or port than
  `public_url`'s host and `grpc.port`.
- Only a super administrator (an administrator who is not staff and not
  banned) can issue install tokens.

On the node: Linux on amd64 or arm64 with systemd, root (sudo), `curl`,
`sha256sum` and `unzip` (or `python3`). OpenRC is not supported yet.

## Install

1. Open the node: **节点 › node › 部署 › 复制安装命令** for a proxy node, or
   the forward node's page (**转发节点 › node › 概览 › 复制安装命令**).
2. Choose the token lifetime (1 hour by default, at most 7 days) and
   generate the command. The token is shown once and works once; closing the
   sheet drops it.
3. Pick the download source and copy its command:

   ```sh
   curl -fsSL https://panel.example.com/install.sh | sudo bash -s -- \
     --control https://panel.example.com --node forward-41 --token anixagt_...
   ```

   | Source | Script from | Agent from | Checksum from |
   |---|---|---|---|
   | `control` (default) | Control | Control when it holds the release (`agent_install.artifact_dir`), else GitHub | Control, else GitHub |
   | `cn` (`--mirror cn`) | Control | the mainland mirror (`agent_install.cn_mirror_url`), else Control, else GitHub | Control, else GitHub |
   | `github` (`--mirror github`) | GitHub release asset `agent-install.sh` | GitHub | GitHub |

   A mirror never supplies the checksum it is checked against.
4. Paste it on the node as root. The script prints what it did:

   ```text
   [anixops] AnixOps Agent v4.2.0 is running (install)
     node:        forward-41
     identity:    spiffe://anixops/prod/agent/forward-41 (serial ..., expires ...)
     forwarding:  wrote /etc/sysctl.d/90-anixops-forward.conf (net.ipv4.ip_forward=1 net.ipv6.conf.all.forwarding=1); applied
     legacy:      removed
                    - nftables table inet v2b_forward
                    - systemd unit v2forward-agent.service
   ```

The Agent then shows as connected over mTLS on **Agent 连接方式**
(`anix-control agents transports`).

## What The Script Does

1. Checks its arguments, root, the architecture and systemd; a first install
   without `--token` stops here.
2. Reads `https://<control>/install/agent.env`: the Agent release (Control's
   own version, H25), the gRPC target, the mirrors and, when Control holds
   the release, its SHA-256.
3. Downloads the release, checks its SHA-256 and, when the release has a
   `.sig`, its Ed25519 signature by the official release key. Nothing on the
   machine changes before these checks pass.
4. Creates the system users `anixops-agent` (in the `anixops-gost` group,
   which guards gost's API socket) and `anixops-gost`.
5. Stops a running Agent (forwarding keeps running), installs
   `/usr/lib/anixops-agent/anix-agent` (and the pinned gost when the release
   ships it) with `/usr/local/bin/anix-agent` linking to it.
6. Writes `/etc/anixops/agent/config.json` (root:anixops-agent, 0640) with
   Control's address, the gRPC target, the node and the identity paths, and
   the token to `/var/lib/anixops-agent/enroll.credential` (0600, owned by
   the Agent, which removes it after use). The Agent's state is in
   `/var/lib/anixops-agent` (0700), its key and certificate in
   `/var/lib/anixops-agent/pki`, the gost driver's files in
   `/var/lib/anixops-gost` (0750).
7. Removes the legacy forward runtime of this machine: the nftables tables
   `inet v2b_forward`, `ip v2b_forward` and `ip anixops_forward`, and the
   clean agent (`v2forward-agent.service`, `/etc/v2board-forward-agent`,
   `/usr/local/bin/v2forward-agent`). It touches no other table or service.
8. On a forward node (`forward-<id>`, or a proxy node with `--forward`)
   turns on IP forwarding, which nftables DNAT and gost relays need: writes
   `/etc/sysctl.d/90-anixops-forward.conf` (`net.ipv4.ip_forward = 1`,
   `net.ipv6.conf.all.forwarding = 1`) and applies it with
   `sysctl -e -p` (`-e`: a host without IPv6 still gets IPv4). When it
   cannot apply now, the summary notes it and the file applies at the next
   boot. A proxy node without `--forward` is left as it is.
9. Writes `anix-agent.service` and `anixops-gost.service`, and a polkit rule
   that lets `anixops-agent` start, stop and reload `anixops-gost.service`
   only; enables and starts the Agent.
10. Waits (up to `--timeout`, 180 s by default) until `anix-agent identity`
    shows a valid certificate for the node.

The Agent runs as `anixops-agent`, not root, with only `CAP_NET_ADMIN` and
`CAP_NET_BIND_SERVICE`, `NoNewPrivileges`, `ProtectSystem=strict` (writable:
`/var/lib/anixops-agent`, `/var/lib/anixops-gost`), `ProtectHome`,
`PrivateTmp` and `RestrictAddressFamilies=AF_INET AF_INET6 AF_NETLINK
AF_UNIX`. gost runs as `anixops-gost` with `CAP_NET_BIND_SERVICE` only.

## Running It Again

The same command is safe to repeat. With a valid identity it upgrades the
Agent in place, keeps the identity and the configuration, and leaves the
token unused (it expires). A failed run can be re-run after fixing the cause.

To move the machine to another node, or to enroll again after its identity
was revoked, issue a new command for the node and add `--reset`: the script
discards the identity, rewrites the configuration (the old one is kept as
`config.json.bak.<time>`) and enrolls with the new token.

## Verifying The Script

`install.sh` is the same file on every Control and in every release
(`agent-install.sh` among the release assets), signed with the official
release key (Ed25519, the key of `plugins.official_public_key`):

```sh
curl -fsSLO https://panel.example.com/install.sh
curl -fsSLO https://panel.example.com/install.sh.sig
printf '%s' 'MCowBQYDK2VwAyEAlvbhRmhzVbSAbrw3vm0k7vYqpEu4/dF/ZqVbp2gS7uM=' | base64 -d >official.der
base64 -d install.sh.sig >install.sh.sig.bin
openssl pkeyutl -verify -pubin -keyform DER -inkey official.der -rawin \
  -in install.sh -sigfile install.sh.sig.bin
sudo bash install.sh --control https://panel.example.com --node forward-41 --token anixagt_...
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

Control then serves them at `/install/agent/<tag>/<asset>` and publishes
their SHA-256 in `/install/agent.env`, which also lets the `cn` mirror work
without GitHub. A mainland mirror (`agent_install.cn_mirror_url`) mirrors the
GitHub release downloads: `<base>/<tag>/<asset>`.

## Troubleshooting

| Message | Cause and fix |
|---|---|
| `agent_install_unconfigured` on the node page | Control does not know an https address nodes reach: set `agent_install.public_url`; on a development build also `agent_install.agent_version` |
| `agent_pki_disabled` | the built-in agent CA is off: set `module_runtime.ca_kek` |
| `cannot reach .../install/agent.env` | the node cannot reach Control over https (DNS, firewall, certificate) |
| `cannot get the checksum of ...` | `--mirror cn` without GitHub access: put the release in `agent_install.artifact_dir` |
| `did not enroll within 180s` | `journalctl -u anix-agent.service -n 50`: a used or expired token needs a new command; the gRPC target must be reachable with TLS |
| `installed for another node` | the machine runs the Agent of another node: add `--reset` with a token for the new node |
| `--group` / `--offline` / `uninstall` refused | node-group tokens, offline packages and the uninstaller are not in this release |

## Not In This Release

- Node-group tokens (`--group`), where the first use creates the node: Control
  has no node-group model for Agent nodes yet, and creating the node on first
  enrollment needs a change to the agent PKI. Tokens are bound to an existing
  node.
- Preflight checks (kernel, nftables, ports, clock skew) and offline packages
  (`--offline`) come next; the uninstaller is `anix-agent uninstall`.
- OpenRC.
- polkit older than 0.106 (Ubuntu 22.04 ships 0.105, which reads `.pkla`
  files instead of `/etc/polkit-1/rules.d`): the script then installs no
  rule and says so, and the Agent cannot start or reload
  `anixops-gost.service` itself. Preflight (O2) will check it.
