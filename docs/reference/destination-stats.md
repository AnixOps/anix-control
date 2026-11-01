# Destination Statistics (访问统计)

Each user's traffic by destination domain, and by the category the domain
falls in (video, music, messaging, social, gaming, AI, downloads, websites,
unknown). Administrators read every user's statistics under
**流量与监控 → 访问统计** (`/admin/monitor/destinations`); every user reads
their own under **访问统计** (`/user/traffic`), so they know what the nodes
record about them and for how long.

The statistics are **off** until an administrator turns them on. They record
which sites a user reached, which is personal data: before turning them on,
tell the users in the terms of service what is recorded, for how long and who
can read it.

## What is recorded

| | |
|---|---|
| Source | The Agent's connection hooks (sing-box, Xray, Hysteria 2): each user's connections and bytes by destination, summed per report window and sent on the Agent Control stream (`DestinationReport`, `destinations.v1`, [`PROTOCOL.md`](../../sdk/api/agent/v1/PROTOCOL.md)) |
| Destination | The **registrable domain** the connection was sniffed or dialed to (TLS SNI, HTTP Host, QUIC SNI): `www.youtube.com` and `rr3---sn-x.googlevideo.com` count as `youtube.com` and `googlevideo.com`. No path, URL, query or content: encrypted traffic shows none of them. A destination without a domain is recorded as its address network (`149.154.167.0/24`, a `/48` for IPv6). The traffic of a user beyond the Agent's per-window limit of destinations is one row with an empty domain ("other"). |
| Protocol | The sniffed protocol (`tls`, `http`, `quic`, `bittorrent`, ...) or none |
| Storage | `v5_destination_stat`: one row per UTC hour, node, user, destination and protocol, with upload and download bytes and the connections opened. Raw bytes, without the node's traffic rate. |
| Not recorded | The user's own address, the destination's address when a domain is known, full host names, URLs |

WireGuard nodes route packets, not connections: they report nothing.

## Categories

A destination's category is judged when it is read, so a rule change applies
to the history too. The rules come in three layers; a later layer's rule for
the same domain, network or protocol replaces an earlier one's:

1. **Built-in rules** (`internal/destinationstats/categories.go`, updated with
   each release): lists that start from v2fly's `domain-list-community`
   categories, narrowed to the domains that serve one kind of traffic,
   Telegram's networks and the `bittorrent` protocol.
2. **Remote sources** (`v5_traffic_category_source`): lists fetched from a URL
   every `refresh_hours` (1 to 168, default 24), the whole list in one
   category. A list may be plain domains, addresses and CIDRs, v2fly
   `domain-list-community` (`domain:`, `full:`; `keyword:`, `regexp:` and
   `include:` are skipped), Clash classical rules (`DOMAIN-SUFFIX`, `DOMAIN`,
   `IP-CIDR`, `IP-CIDR6`) or a Clash rule provider (`payload:`). At most
   16 MiB and 200000 entries; a failed or empty fetch keeps the previous list
   and records `fetch_error`. The singleton worker checks every 10 minutes.
3. **Custom rules** (`v5_traffic_category_rule`): an administrator's domain
   (with its subdomains), network (an address or CIDR) or sniffed protocol,
   in any category but unknown. A rule to `web` takes a destination out of a
   built-in or remote category.

A destination is judged in this order:

1. A sniffed protocol with a rule decides alone (`bittorrent` is a download).
2. The longest rule domain the domain ends with.
3. The most specific network rule containing an address network.
4. Any other domain: websites. Any other address, and "other": unknown.

Control rebuilds its classifier when a rule or a source changed, checked at
most every 15 seconds, so another Control process follows within that time.

The categories are estimates. A domain shared by several services
(`google.com`, `apple.com`, `qq.com`, CDNs such as `cloudfront.net`) counts as
websites, and large downloads over a general CDN do too. ECH hides the real
domain behind the provider's public one.

### Rules API (administrators, audited as `traffic_category`)

| Route | |
|---|---|
| `GET /api/v4/admin/traffic/categories` | `categories`, `builtin` (by category: `domains`, `networks`, `protocols`), `rules`, `sources` |
| `GET /api/v4/admin/traffic/categories/classify?domain=&protocol=` | The category the current rules give a destination |
| `POST /api/v4/admin/traffic/categories/rules`, `PUT`/`DELETE .../rules/{id}` | `{kind: domain\|network\|protocol, value, category, note}`; 409 `duplicate` for a second rule of the same kind and value |
| `POST /api/v4/admin/traffic/categories/sources`, `PUT`/`DELETE .../sources/{id}` | `{name, url, category, enabled, refresh_hours}` |
| `POST /api/v4/admin/traffic/categories/sources/{id}/refresh` | Fetch now (up to a minute) |

The console edits them at 流量与监控 → 流量分类.

## QoS: priorities and per-user caps

The categories also steer the proxy nodes' traffic (`internal/trafficqos`,
console 流量与监控 → 流量分类 → QoS, `GET`/`PUT /api/v4/admin/traffic/qos`,
audited as `traffic_qos`). The settings are one JSON document in
`v2_system_config` (`traffic.qos`); without it the defaults apply: on,
downloads at low priority, Steam downloads (`trafficqos.SteamDownloadDomains`)
capped at 5 Mbit/s per user.

| Field | |
|---|---|
| `enabled` | Off, the nodes neither prioritize nor cap |
| `priorities` | Category → `high` or `low`; the rest is normal. `web` and `unknown` take none |
| `default_egress_mbps`, `node_egress_mbps` | A node's download capacity (by node id), for node-wide scheduling; 0 schedules within the users' speed limits only |
| `min_share_percent` | The least share of a busy capacity a lower class keeps (1–90, default 10) |
| `caps` | `{name, priority, limit_mbps, domains}`: each user's connections to the domains (and subdomains) together at most `limit_mbps` |

Each proxy node's desired configuration carries the member `qos` (refreshed
every minute): the caps first, then one class per prioritized category with
its domains, networks and protocols from the current category rules (the
capped domains left out, so the cap wins), at most 50000 entries. The Agent
sends it to anixcore as `PUT /v1/qos` (feature `qos`). There, only the
download direction is scheduled: lower classes use what the higher ones
leave of the node's capacity and of each speed-limited user's limit, and
back off within a few 100 ms windows when higher traffic needs it. A
connection is classed by its target name or address, or by its first bytes
(TLS SNI, HTTP Host, BitTorrent); Mux.Cool sessions and UDP are not
scheduled.

## Settings

| Setting | Key (`v2_system_config`) | Default | |
|---|---|---|---|
| On or off | `traffic.destinations.enabled` | `false` | Off, Control acknowledges the Agents' batches and drops them; the history kept is still shown. Takes effect at once, without reconnecting the Agents. |
| Retention | `traffic.destinations.retention_days` | `30` | 7 to 180 days. The retention worker (hourly) deletes the older rows. |

`GET` and `PUT /api/v4/admin/traffic/destinations/settings` read and change
them (administrators; the change is audited).

### Storage

About 25 to 50 KB per active user and day: each active user reaches 20 to 40
destinations in an hour of use.

| Active users | 30 days | 90 days |
|---|---|---|
| 100 | 75–150 MB | 225–450 MB |
| 1000 | 0.75–1.5 GB | 2.3–4.5 GB |

## API

| Route | Who | |
|---|---|---|
| `GET /api/v4/admin/traffic/destinations?hours=&user_id=&node_id=&limit=` | administrators | All users and nodes, or one of each |
| `GET /api/v4/user/traffic/destinations?hours=&limit=` | the signed-in user | Their own |

`hours` is 1 to 4320 (default 24): the whole UTC hours up to and including the
current one. `limit` is the number of destinations answered, 1 to 500 (default
50). The answer:

```json
{"data": {
  "since_unix_ms": 1792918800000, "until_unix_ms": 1793005200000,
  "upload": 421, "download": 1591, "connections": 14,
  "categories": [
    {"category": "video", "upload": 20, "download": 990, "connections": 4},
    {"category": "download", "upload": 400, "download": 600, "connections": 9}
  ],
  "domains": [
    {"domain": "youtube.com", "category": "video", "upload": 20, "download": 990, "connections": 4}
  ],
  "domain_count": 2,
  "settings": {"enabled": true, "retention_days": 30}
}}
```

`categories` and `domains` are ordered by bytes, largest first;
`domain_count` counts every destination of the range.

## Agents

Agents send `DestinationReport` once Control offers `destinations.v1`, which it
offers every proxy node's Agent that lists it, whatever the setting. The
batches are claimed once per node and batch id (`v4_kernel_agent_report_batch`,
kind `destinations`), as traffic batches are. The Agent's collector is
kernel-neutral: it receives the user, destination, sniffed protocol and bytes of
each connection, so another proxy kernel only has to report the same.
