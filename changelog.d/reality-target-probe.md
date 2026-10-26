### Added

- Reality target probing (目标站探测, phase 1). A proxy node's Agent probes
  candidate REALITY targets (dest and SNI) from the node's own network with
  the new `agent.diagnostic` action `reality.probe` (anix-agent advertises
  the capability `reality.probe`), and Control ranks the answers and
  recommends one, instead of every node borrowing `www.microsoft.com`.
  `POST /api/v4/kernel/nodes/{id}/reality-targets/probe` probes the
  candidates (your own `candidates`, host or host:port, plus a built-in list
  unless `include_defaults` is false; at most 24, 4 at a time) and answers
  the ranked report; `GET /api/v4/kernel/nodes/{id}/reality-targets` answers
  the latest one, the target each Reality protocol of the node uses now and
  whether its Agent can probe. A target passes on TLS 1.3, ALPN h2, an X25519
  key exchange, a certificate valid for the name and no redirect to another
  host; passing targets are scored by the node's address prefix, handshake
  time, post-quantum key exchange, OCSP stapling and chain size, less a CDN
  and the other nodes already using the target. Probes are stored in the new
  table `v5_reality_target_probe` (migration 0008) and every run is in the
  operation log (module `reality_target`). Nothing in a node's configuration
  changes: applying a target is a later phase. Needs an anix-agent with
  `reality.probe`; an older Agent answers `reality_probe_unsupported`.
