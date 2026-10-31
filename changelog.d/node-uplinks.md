### Added

- Nodes with several NICs (uplinks) choose which one the traffic to each
  next device of a route leaves by. On the node page, 协议 → 网络 →
  上联网卡 lists the uplinks (interface, source address and gateway per
  family); each next device gets 自动 (lowest latency) or a pinned uplink.
  Each uplink probes the next device's hop ingress every 10 seconds and the
  page shows the round trip per uplink and the uplink in use. In auto mode
  Control starts on the fastest uplink, leaves a down one at once, and moves
  to a faster one only when it is faster by 10 ms and 15% for three status
  reports and the current one has been used for three minutes.
  `GET/PUT /api/v4/kernel/nodes/{id}/network` carry `uplinks`,
  `uplink_default` and `uplink_choices` and answer `uplink_hops`. Needs the
  matching Agent, which routes each link's peer through its uplink.
