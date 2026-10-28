### Removed

- The WireGuard dual-node relay (a domestic entry carried to an overseas
  exit over a gost `relay+quic`/`relay+wss` tunnel), removed with gost; the
  Agent already refuses a WireGuard node with relay settings. Control
  refuses `settings.relay` on a WireGuard protocol (clear it to save the
  protocol), no longer adds a default gost relay or `tunnel_type` to the
  node config, and the protocol editor has no relay section. A protocol
  stored with a relay keeps reaching its Agent as stored, which refuses it
  rather than egress from the entry. The protocol templates describe a
  single-node WireGuard, and `scripts/deploy_wireguard_gost_quic.sh` is
  gone. Cross-border paths run on the network model (ANX-A).
