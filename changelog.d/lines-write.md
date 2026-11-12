### Added

- 网络 → 线路 creates, edits, copies and deletes lines (`/api/v5/net/lines`).
  创建线路 opens a sheet: pick the entries (several back each other up), the
  devices the line passes in order and the exit, then a protocol on the exit
  or fixed targets; a preview shows the path as it will be. A stored line's
  sheet has 编辑 and, under `⋯`, 复制线路 and 删除线路, which asks for the
  line's name. A protocol's own direct line is still edited with the
  protocol.
- 节点 → 协议 → 网络 has a 扫描云 switch: lines through a machine on a cloud
  that scans its hosts forward in the kernel there.
