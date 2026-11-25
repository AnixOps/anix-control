### Changed

- Control's `/install*` routes rate limit region relays (`grpc.relay_sources`)
  with a budget of their own (50/s, burst 200) instead of the public
  per-address one (5/s, burst 10): every install of a region reaches
  Control from its relay's address.
- `install.sh` takes any HTTP answer through a relay (TLS completed with
  Control's certificate) as the relay carrying Control; a busy Control
  (HTTP 429) is retried by the downloads instead of ruling the relay out.
  A metadata failure through a relay names the relay and Control's load
  instead of suggesting a direct request.
- With several relays, the rendered command tries each in turn for the
  script and reads it whole before bash runs it.
- An Agent zip from a release cache is installed only with its release
  signature; the signature comes from the mirror or from Control through
  the relay.
- Re-running a group's command on a node whose configuration lacks the
  route sets `GRPCRelays` and `ReleaseCaches` in place and keeps every
  other line (the Forward section, manual edits); a configuration that
  cannot be edited line by line is rewritten as before, the old one kept.
- A zip from a release cache needs OpenSSL 3 to check its signature: the
  installer requires `openssl` with `--release-cache`, and refuses the zip
  when its OpenSSL cannot verify Ed25519.
- A configuration rewritten for the route (one that cannot be edited line
  by line) keeps the node's one-line Forward section.
- Running a group's command with relays removes the route the Agent stored
  from Control (`control-route-*.json` in the stream state directory), so
  the command's route applies; Control sends its current route again over
  the stream.
