### Added

- Node network settings for NAT and multi-homed machines. The node page's
  协议 tab has a 网络 section (`GET`/`PUT /api/v4/kernel/nodes/{id}/network`,
  audited as `network_node_settings`) for the egress interface, the IPv4/IPv6
  source address and gateway, and the hop port. The automatic line, egresses
  and hop ingress of the routes through the node follow these settings
  instead of the default route. When a NAT entry's port map covers the hop
  port, the hop ingress is of kind `nat` at the entry's host, and the
  previous device dials the mapped public port.
