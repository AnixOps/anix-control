### Added

- The JumpServer workflow gains two actions. `control` calls Control's
  administrator API with an Admin API token kept in the `production`
  environment (`CONTROL_ADMIN_TOKEN`), prints the answer with node keys and
  other secrets replaced by `***`, and can generate Reality and Shadowsocks
  2022 keys inside the request body so they never appear in the public
  inputs or log. `install-agent` issues a node's install token and runs
  Control's install command on one JumpServer host without printing the
  token.
