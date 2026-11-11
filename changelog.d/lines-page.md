### Added

- 网络 → 线路 (`/admin/lines`): every line a user can connect to, read from
  the network as lines (`GET /api/v4/kernel/network/lines`). Each row shows
  the path from entry to exit, where the protocol is served, the rate and the
  last 24 hours of traffic; the filters separate relay lines, direct lines,
  fixed-target forwards and lines that need attention. Opening a line shows
  its devices, protocol, port and split rules. The page is read-only: lines
  are still changed under 节点 and 转发.
