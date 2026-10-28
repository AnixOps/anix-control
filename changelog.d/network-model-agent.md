### Added

- Agent Control serves `network.v1`: an Agent that lists it with its
  `device_capabilities` (and config.v1) gets its stamped `NetworkPlan` as
  the configuration member `network`, pushed as soon as a model write moves
  its plan generation. Its `NetworkStatus` arrives as a package report
  (`network` / `network.status` / `v1`, forward nodes included); Control
  keeps the latest one per device (shown under `devices[].status` of
  `GET /api/v4/kernel/network`) and its public keys, and a new ANX-Line
  server key plans the paths that were waiting for it.
