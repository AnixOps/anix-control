### Changed

- Shadowsocks 2022 server keys are stored instead of computed: migration
  `0041_ss2022_server_keys` stores, for every 2022 protocol without a
  `server_key`, the key earlier releases computed from its node's creation
  time, so clients keep their key; Control no longer computes keys. A new or
  edited Shadowsocks 2022 protocol needs a `server_key`: the admin form fills
  a random one of the cipher's length. Upgrade notes in
  [`docs/UPGRADE.md`](docs/UPGRADE.md#shadowsocks-2022-server-keys-are-stored).
