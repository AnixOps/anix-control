### Security

- `anix-relay` cache: a source is compared and remembered in one canonical
  form (lower-case host, re-encoded path; a query or fragment is refused),
  so another spelling of an allowed file cannot start a new download. A
  source's learned SHA-256 is forgotten after an hour, so a replaced release
  can be fetched again.
- `anix-relay`: the per-source limits count an IPv6 /64 as one source, both
  `max_connections_per_source` and the cache's `max_requests_per_source`.
- `anix-relay` cache: new `max_connections` (1024) bounds the open
  connections from everyone together, and a response may take at most one
  minute plus twice `fetch_timeout`, so a reader that stops reading cannot
  hold a connection for ever.
- `grpc.relay_sources` refuses IPv4-mapped IPv6 ranges such as
  `::ffff:0.0.0.0/96`, which Go reads as all of IPv4: write IPv4 ranges as
  IPv4.
- `anix-relay` cache: downloads are shared per source, whatever hash each
  request asks for, and keep the file under its real SHA-256; a request
  that waited for a download slot finds a download that finished meanwhile.
  New `max_misses_per_source` (30 an hour) bounds the downloads one address
  starts, and one address never waits on as many sources as there are
  download slots.
- `grpc.relay_sources` refuses prefix lengths with a leading zero
  (`10.0.0.0/08`), which took effect as `/8` without the width check.
