### Added

- A node runs every enabled protocol, also two of one type (shadowsocks
  `aes-256-gcm` and shadowsocks 2022 on two ports). The configuration
  snapshot's `legacy_pull` gains `protocols`: each enabled protocol's
  UniProxy answer, by protocol id. UniProxy's config pull takes
  `protocol_id` for the same answer. Left out when the node has a raw
  configuration. Agent 5.0.0-rc.2 runs each of them in its own inbound;
  older Agents keep running the node's first protocol.
