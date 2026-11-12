### Added

- The network's lines and transports have their own tables (`v5_line`,
  `v5_transport`, migration `0035_network_lines`), and a device's network
  settings say whether it is on a cloud that scans its machines
  (`scan_cloud`). Nothing reads them yet: the planner still plans from the
  forward routes until the data moves over (docs/design/network-lines,
  step 2).
