### Added

- An ANX route hop whose network transport is automatic can pick its
  transport mode in the route editor: ANX with the A2 jacket (the default,
  hop port 24443), the bare ANX-Line for trusted lines (hop port 24445, TCP
  and QUIC, with carrier order, multipath and FEC), or a dial-back bare
  ANX-Line whose next device dials the previous one's port 24444 and opens
  no port. The API takes it as `LinkTransport.network_mode`,
  `network_carriers`, `network_multipath` and `network_fec`. Open TCP and
  UDP 24445 on devices that accept a bare ANX-Line, and TCP 24444 on the
  device a dial-back link starts from.

### Fixed

- The dialing end of a QUIC carrier reaches a NAT hop ingress at its mapped
  public port, as TCP already did.
