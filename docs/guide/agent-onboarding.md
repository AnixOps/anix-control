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

On the node: Linux on amd64 or arm64 with systemd 240 or later, root
(sudo), `curl`, `sha256sum` and `unzip` (or `python3`); for an offline
bundle also `tar` and OpenSSL 3. A forward node needs Linux 5.10 and
nftables 0.9.7 or later. OpenRC is not supported yet. The script checks
all of this before it changes anything ([preflight](#preflight-checks)).

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
   the release, its SHA-256. With `--offline` it reads the bundle instead.
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
7. Writes `/etc/anixops/agent/config.json` (root:anixops-agent, 0640) with
   Control's address, the gRPC target, the node and the identity paths, and
   the token to `/var/lib/anixops-agent/enroll.credential` (0600, owned by
   the Agent, which removes it after use). The Agent's state is in
   `/var/lib/anixops-agent` (0700), its key and certificate in
   `/var/lib/anixops-agent/pki`, the gost driver's files in
   `/var/lib/anixops-gost` (0750). A machine that runs, or ran, the Agent
   of anix-agent's own root installer (a unit without
   `User=anixops-agent`, or `/var/lib/anix-agent` or
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
9. On a forward node (`forward-<id>`, or a proxy node with `--forward`)
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
10. Writes `anix-agent.service` and `anixops-gost.service`, and a polkit rule
    that lets `anixops-agent` start, stop and reload `anixops-gost.service`
    only; enables and starts the Agent.
11. Waits (up to `--timeout`, 180 s by default) until `anix-agent identity`
    shows a valid certificate for the node.

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
token unused (it expires). A failed run can be re-run after fixing the cause.

To move the machine to another node, or to enroll again after its identity
was revoked, issue a new command for the node and add `--reset`: the script
discards the identity, rewrites the configuration (the old one is kept as
`config.json.bak.<time>`) and enrolls with the new token.

## Preflight Checks

After reading Control's metadata (or the offline bundle) and before it
changes anything, the script checks the machine. Each check prints `ok`,
`skip`, a warning (`note: preflight: ...`, repeated in the summary) or
`FAIL` with a `fix:` line. When a check fails the script stops after
running all of them and says how many failed; nothing on the machine has
changed. `--skip-preflight` installs anyway and notes it in the summary.

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
enroll; only the downloads move to the bundle.

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
   script too), generate the install command on the node page, and run it
   with `--offline`:

   ```sh
   sudo bash install.sh --offline agent-offline-amd64.tar.gz \
     --control https://panel.example.com --node forward-41 --token anixagt_...
   ```

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

For offline bundles also put `SHA256SUMS`, `SHA256SUMS.sig` and the zips'
`.sig` files there ([offline](#installing-without-internet-access)).
Control then serves the zips at `/install/agent/<tag>/<asset>` and publishes
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
| `--group` refused | node-group tokens are not in this release |
| `preflight found N problem(s)` | each `FAIL` line above has a `fix:`; re-run after fixing, or add `--skip-preflight` |
| `cannot connect to the gRPC target` | open `grpc.port` from the node to Control, or set `agent_install.grpc_target` to an address nodes reach |
| `the certificate of the gRPC target ... does not verify` | the gRPC listener needs a certificate for that host from a CA in the node's trust store |
| `this host's clock is ...s off Control's` | `timedatectl set-ntp true` (or chrony/ntpd) |
| `the release signature of SHA256SUMS does not verify` | the offline bundle was altered or built from another release; make it again |
| `the offline bundle has no ... : it is not for arm64` | make the bundle with `-arch` of the node |

## Not In This Release

- Node-group tokens (`--group`), where the first use creates the node: Control
  has no node-group model for Agent nodes yet, and creating the node on first
  enrollment needs a change to the agent PKI. Tokens are bound to an existing
  node.
- OpenRC.
- gost forwarding on polkit older than 0.106 (Ubuntu 22.04 ships 0.105,
  which reads `.pkla` files instead of `/etc/polkit-1/rules.d`): preflight
  warns, the script installs no rule, and the Agent cannot start or reload
  `anixops-gost.service` itself (see [preflight](#preflight-checks)).
- `anix-agent uninstall`: the Agent's own command predates this layout;
  use `install.sh uninstall [--purge]`.
