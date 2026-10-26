### Added

- Panabit iWAN nodes. A node protocol of type `iwan` (anixcore's iWAN
  inbound, a UDP layer-3 tunnel without encryption) carries `pool`, `mtu`,
  `dns_upstream`, `allow_encrypt`, `allow_client_to_client` and
  `fixed_addresses` (panel user id to address) to the Agent over UniProxy
  and gRPC; the settings are validated when the protocol is saved. The admin
  protocol editor has an iWAN form that marks the protocol unencrypted.
  Users log in with their user id and the first 16 hex digits of their UUID;
  the subscription format `iwan` (`?type=iwan`) gives them an iwand
  configuration. Needs an anix-agent with the anixcore `iwan` mapping.
