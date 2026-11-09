# Cross-Border Standby Channels And Drill

When a cross-border transport (ANX-A, the A2 jacket: kind `anx`, not trusted,
in the network model) is blocked, its traffic can move in one call to a
public-protocol standby channel between the same two proxy nodes, and back
once the transport recovers (anx `docs/FUTURE-IMPROVEMENTS.md` F-54).

Two tiers, prepared ahead of time and switched per transport or all at once:

| Tier | Protocol | Looks like | Use when |
|------|----------|------------|----------|
| `reality` (tier 1) | VLESS + REALITY + Vision (`xtls-rprx-vision`), Chrome fingerprint, borrowed site's certificate | TLS to a large foreign site | the A2 transport is blocked; fastest to switch, but the same TLS family as A2 |
| `xhttp` (tier 2) | VLESS + XHTTP (`packet-up`) over our own domain's real TLS, behind nginx on the exit node | HTTPS to our own site | tier 1 is blocked too, or the whole TLS-jacket family is targeted |

Shadowsocks 2022 is not used across the border. A drill switches to **both**
tiers in turn.

## How It Works

- Control keeps one channel per tier for each (source node, exit node) pair
  a cross-border transport serves: the ingress and egress nodes of each
  enabled path through it, or the transport's own two ends (table
  `v5_l2_standby`, `internal/l2standby`). A channel holds a uuid, a REALITY
  key pair and short id, an XHTTP path, and a relay port of 31000–31999 on
  the exit node; the API never answers the uuid or the private key.
- Switching a transport marks its tier's channels active. The desired
  configuration member `egress` (the same one as the egress routes) then
  carries, within a minute through each node's Agent:
  - on the exit node a relay `{protocol: "vless", port, settings}`:
    REALITY on the public port for tier 1; for tier 2 the relay listens on
    `127.0.0.1` behind the node's TLS front;
  - on the source node a catch-all route `{domains: ["regexp:."], server:
    {protocol: "vless", host, port, settings}}` with an id above every egress
    route's (they are tried first).
- Traffic the source node can name (domain, SNI, Host or DNS map) leaves by
  the standby; traffic to bare IP addresses keeps the transport.
- Switching back (tier `off`) removes both; the transport carries everything
  again.

## Prerequisites (once)

1. Node kernels with VLESS egress relays: networkcore `node-runtime`
   built with the `vless` feature (networkcore PR #191: `egress` relays and
   routes with `protocol: "vless"`, relay `listen`, `settings`), in anixcore
   and in anx-node.
2. An Agent that passes the `egress` member's `settings` and `listen` fields
   through to the kernel unchanged. **Check on one pair before relying on
   it:** prepare, switch one transport, and look at the source node's
   `GET /v1/egress` (kernel admin API): the route must carry
   `server.settings.uuid`, and the exit node's relay `settings.reality` (or
   `listen: 127.0.0.1`). An Agent that drops them leaves the relay refusing
   the configuration: the node's state in 分流 shows `failed`.
3. Firewall: open the tier 1 relay port (31000–31999, shown per channel) on
   the exit node to the source node.
4. Tier 2 only: a domain of our own (not the ICP-filed one) resolving to
   the exit node, with a real certificate, and nginx in front:

   ```nginx
   server {
       listen 443 ssl http2;
       server_name edge.example.net;
       ssl_certificate     /etc/letsencrypt/live/edge.example.net/fullchain.pem;
       ssl_certificate_key /etc/letsencrypt/live/edge.example.net/privkey.pem;
       root /var/www/edge;                 # an ordinary site for everyone else
       location /<xhttp_path>/ {           # the channel's xhttp_path
           proxy_pass http://127.0.0.1:<relay_port>;
           proxy_http_version 1.1;
           proxy_buffering off;
           proxy_request_buffering off;
           client_max_body_size 0;
           proxy_read_timeout 300s;
       }
   }
   ```

   The path and port are on 节点 → 过境应急 (or `GET
   /api/v4/kernel/network/standby`) once the channel is prepared.

## Prepare The Channels

节点 → 过境应急 → 准备通道, or:

```bash
curl -sS -X POST https://panel.example.com/api/v4/kernel/network/standby/prepare \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"reality_server_name":"www.microsoft.com",
       "xhttp_domains":{"50":"edge.example.net"}}'
```

- `reality_server_name` (and `reality_dest`, `host:port`, default the name
  on 443): the borrowed site of tier 1. Pick one the exit node reaches fast
  and that is plausible from its ASN.
- `xhttp_domains`: exit node id → our own domain. A node without one gets no
  tier 2 yet.
- `exit_hosts`: exit node id → the address the source dials for tier 1, when
  it is not the node's host.
- `transports`: limit to these transports; empty prepares all and removes the
  inactive channels of pairs that are gone.

Preparing changes nothing on the nodes. The overview's `ready` lists the
tiers every pair of a transport has.

## Switch And Switch Back

节点 → 过境应急: a row's actions, or 全部切到第一档 / 全部切到第二档 / 全部切回.
Or:

```bash
# one transport to tier 1
curl -sS -X POST .../api/v4/kernel/network/standby/switch -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' -d '{"transports":["t-hk-jp"],"tier":"reality"}'
# everything to tier 2
... -d '{"all":true,"tier":"xhttp"}'
# back to the transports
... -d '{"all":true,"tier":"off"}'
```

All or nothing: `409 not_ready` when a transport lacks a ready channel of the
tier for one of its pairs. Every prepare and switch is in the operation log
(module `l2_standby`).

## Alerts

The alert scan raises (see [kernel alerts](../reference/kernel-alerts.md)):

- `cross_border_link_down` on the transport, when every device that reports
  it has had it down (degraded, down or error) for 10 minutes. `critical`
  while it has no standby, `warning` once switched (the message says to
  switch back when it is up).
- `cross_border_wide_outage` (`critical`), when half or more of at least two
  cross-border transports are down at once: the jacket itself is targeted.

## Drill (Do It Before You Need It)

Run on one pair first, then on all. Each step is reversible with tier `off`.

1. **Prepare** both tiers; check `ready` lists `reality` and `xhttp` for the
   pair.
2. **Baseline**: through an entry on the source node, open a test page and
   note the exit IP (`https://www.cloudflare.com/cdn-cgi/trace`).
3. **Tier 1**: switch the transport to `reality`. Within a minute both
   nodes' states are applied; the same page still answers with the exit
   node's IP. Run a download for a minute (no stall), a video, and a voice
   call if the line carries one.
4. **Tier 2**: switch to `xhttp`; repeat step 3. Expect more latency and less
   throughput than tier 1 (HTTP framing through nginx).
5. **Back**: switch to `off`; repeat step 2.
6. Record in the drill log: date, pair, time from the switch to traffic
   flowing on each tier, throughput on each, anything that broke.

Do the drill on every new cross-border pair and after every kernel or Agent
upgrade that touches egress routes.

## Manual Plan When A Link Is Blocked

1. The alert names the transport and since when; the detail has the reasons
   the device reported (RST after the handshake, blackhole, AEAD failures:
   F-38 counters).
2. **One transport down**: switch it to `reality`. If tier 1 does not carry
   traffic within 5 minutes, or is blocked within hours, switch it to
   `xhttp`.
3. **Wide outage** (`cross_border_wide_outage`): switch **all** to
   `reality`; if tier 1 fails on more than one pair, switch all to `xhttp`.
   Do not rotate addresses in a wide outage: it burns addresses for nothing.
4. Check the client side: `GET /api/v4/kernel/client-failures` (what
   AnixOps Net clients report per node) tells user-side blocking from a
   cross-border one.
5. Find out why: compare the canary nodes (AnixOps-ansible
   `services/canary-egress` and `canary-probe`) that run the same jacket
   profiles — which ones were blocked, after how long, and how.
6. When the transport is up again for an hour (the alert resolved), switch
   back to `off`, one transport first.
7. Write down what happened, what you switched, and when, for the next
   drill.

What a cloud thread may not do: buy exit machines, deploy to real nodes, or
run a real switch. Those are the operator's.
