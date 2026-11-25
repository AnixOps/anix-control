### Added

- Temporary SOCKS5 proxies on proxy nodes. 节点 → 协议 → 临时 SOCKS5 代理
  opens a SOCKS5 proxy on an internal address of the node (10/8, 172.16/12,
  192.168/16, 100.64/10, link-local, loopback or fc00::/7; never a wildcard or
  public address) for 1 to 1440 minutes, 15 by default, so that devices of
  that network send traffic out through the node. Control generates the
  username and password and shows the password once. The node's kernel
  closes the proxy at its end time by itself; it can also be closed early.
  At most 8 open proxies per node. Routes: `GET`/`POST
  /api/v4/kernel/nodes/:id/temporary-proxies`, `DELETE
  /api/v4/kernel/nodes/:id/temporary-proxies/:proxy_id`; table
  `v5_node_temporary_proxy` (migration 0044); audited in the operation log
  (module `temporary_proxy`). The desired configuration carries the open
  proxies in `temporary_proxies`. It needs an Agent and anx-node kernel that
  run temporary proxies: an older Agent ignores the member, and a newer
  Agent on an older kernel reports the configuration as failed while a proxy
  is open.
