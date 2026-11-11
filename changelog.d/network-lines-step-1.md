### Added

- The network as lines, step 1 of the move to devices, entries, transports,
  exits and lines (`docs/design/network-lines/README.md`):
  `GET /api/v4/kernel/network/lines` reads what Control stores today (nodes
  and their protocols, node entries, forward routes and their subscriptions,
  egress routes, exit policies) as lines and compares the forwards those
  lines plan with the ones the routes plan now. It is read-only; nothing is
  stored and no node changes.
- A node's network settings (`GET` / `PUT /api/v4/kernel/nodes/{id}/network`)
  have `region`, where the device is, as a place key for the topology map;
  the lines snapshot answers it on each device (migration `0033_node_region`).

### Security

- Go 1.26.11 (toolchain, CI and the build images): a standard library
  vulnerability that govulncheck reports in 1.26.10.
