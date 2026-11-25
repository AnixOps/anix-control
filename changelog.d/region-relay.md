### Added

- Region relay `anix-relay` (`cmd/anix-relay`, `docs/design/region-relay`):
  a machine inside a region, mainland China first, that forwards the
  region's node connections to Control byte for byte (TLS and the client
  certificate stay end to end, so the relay cannot read or forge a stream)
  and keeps a read-only cache of release files addressed by SHA-256. It has
  no web interface and no API.
- `grpc.relay_sources` (`ANIX_CONTROL_GRPC_RELAY_SOURCES`): the relays'
  addresses. Their connections must open with a PROXY v2 header, whose
  address then stands for the node's; no header is read from any other peer.
  Not combinable with `grpc.edge_proxies`. The configuration is now eleven
  operator keys.
