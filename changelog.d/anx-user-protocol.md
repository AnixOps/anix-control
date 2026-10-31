### Added

- ANX-A is a node protocol users connect to. A node protocol of type `anx`
  (default port 8443, A2 jacket over TCP) runs on the node's anx-node kernel
  with `server_names`, `dest` and `alpn` settings. The AnixOps Net
  subscription format carries it as an `anx://` link with the user's key
  (derived from the UUID), the SNI and the server public key the Agent
  reported; other subscription formats leave ANX nodes out. The protocol
  editor has an ANX form. A node that has not reported its server key yet has
  no link.
