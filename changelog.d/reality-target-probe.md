### Added

- Reality target probing (目标站探测, phase 1). A proxy node's Agent probes
  candidate REALITY targets (dest and SNI) from the node's own network with
  the new `agent.diagnostic` action `reality.probe` (anix-agent advertises
  the capability `reality.probe`), and Control ranks the answers and
  recommends one, instead of every node borrowing `www.microsoft.com`.
  `POST /api/v4/kernel/nodes/{id}/reality-targets/probe` probes the
  candidates (your own `candidates`, host or host:port, plus a built-in list
  unless `include_defaults` is false; at most 24, 8 at a time) and answers
  the ranked report; `GET /api/v4/kernel/nodes/{id}/reality-targets` answers
  the latest one, the target each Reality protocol of the node uses now and
  whether its Agent can probe. A target passes on TLS 1.3, ALPN h2, an X25519
  key exchange, a certificate valid for the name, no redirect to another
  host and a real REALITY handshake through it on the node (a loopback
  REALITY server and client in the Agent; www.microsoft.com passed every TLS
  check from the Germany node and still broke REALITY); passing targets are scored by the node's address prefix, handshake
  time, post-quantum key exchange, OCSP stapling and chain size, less a CDN
  and the other nodes already using the target. Probes are stored in the new
  table `v5_reality_target_probe` (migration 0008) and every run is in the
  operation log (module `reality_target`). Needs an anix-agent with
  `reality.probe`; an older Agent answers `reality_probe_unsupported`.
- Applying a Reality target. `POST /api/v4/kernel/nodes/{id}/reality-targets/apply`
  (`protocol_id`, `server_name`, `port`, `force`) writes the target into one
  Reality protocol of the node: `reality_settings` `dest`, `server_name` and,
  where listed, `server_names`, and `tls_settings` `server_name`, the SNI of
  the subscription links. The private key and every other setting stay. The
  node must have probed the target with verdict ok unless `force` is true.
  Clients must refresh their subscription after a change, or they keep the
  old SNI and cannot connect. Audited (action `reality_target_apply`).
- The node page has a 目标站 (Targets) section between 诊断 and 日志: the
  current target of each Reality protocol, a probe run (your own candidates
  and the built-in list), the ranked results with the reasons a target
  fails, the recommendation, and an Apply button per protocol, confirmed.
