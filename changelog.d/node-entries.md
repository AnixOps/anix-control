### Added

- Node entries (入口): the addresses subscribers reach a proxy node at beyond
  its host, for machines with several lines or address families, such as a
  domestic relay with a CN2 and a 9929 line where each line has a native
  IPv6 address and a provider NAT IPv4 address. An entry is direct (the
  address on each protocol's own port) or NAT (a public address whose port
  ranges map to protocol ports, typed in by hand). A node with enabled
  entries is offered in subscriptions once per entry and protocol, in place
  of its host, named `<node> - <protocol> | <entry>`; a protocol port that a
  NAT entry does not map is left out for that entry. Nodes without entries
  are unchanged. `GET /api/v4/kernel/nodes/{id}/entries` lists them and
  `PUT /api/v4/kernel/nodes/{id}/entries` replaces the list (at most 32,
  audited, module `node_entry`). They are stored in the new table
  `v5_node_entry` (migration 0010). The node page shows them under 协议 with
  an editor sheet (port map lines such as `21000-21099 -> 30000-30099`,
  public ports first).

### Fixed

- Share links (vless, trojan, ss, hy2, tuic, anytls) and Quantumult X lines
  put an IPv6 server address in brackets (`[2001:db8::1]:443`); without
  them the link did not parse.
