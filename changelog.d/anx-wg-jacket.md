### Added

- The ANX protocol form takes a WireGuard-shaped UDP port (`wg_listen_port`,
  the anx wg jacket) for users dialing a domestic entry; it backs up the
  no-SNI A2 mode. Control sends it to the Agent only when set, and the
  AnixOps Net subscription adds `wg_port` to the `anx://` link so clients
  try that port first and fall back to A2. The UDP port has to be open in
  the node's firewall. Needs an anx-node with the wg jacket and the matching
  Agent.
