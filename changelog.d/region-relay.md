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

### Security

- The PROXY v2 listener no longer clears gRPC's handshake deadline after
  reading a relay's header, so a peer that sends one byte and stays silent
  is timed out instead of holding the connection.
- `anix-relay`: Control must answer within `handshake_timeout` (30 s) of the
  node's first bytes; `max_connections_per_source` (256) bounds one node
  address; `dial_timeout` defaults to 2 s, under the Agent's 5-second wait.
  The cache refuses another hash for a source whose SHA-256 it already
  knows instead of downloading it again, bounds open requests per address
  (`max_requests_per_source`), can serve `allowed_sources` only, follows
  redirects only to the origins' hosts and `redirect_hosts`, refuses more
  reserved ranges (including NAT64 and 6to4), and keys concurrent downloads
  by hash and source.
- `grpc.relay_sources` refuses a CIDR wider than /24 (IPv6 /64).
