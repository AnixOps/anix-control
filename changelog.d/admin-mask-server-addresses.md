### Added

- Server IP addresses are masked on the admin pages by default
  (`203.0.113.7` shows as `203.*.*.7`, `2001:db8::1` as `2001:****`), so a
  screen share or a screenshot does not give them away: the node list and
  node overview, entries and IWAN lines, joining machines, the live monitor,
  forward node and route details, and the entry HA card's records and node
  addresses. Account menu → 显示服务器 IP (or the command palette) shows
  them; the choice lasts for the browser tab, and a new tab starts masked
  again. Forms, copy buttons, diagnostics output and the API keep the real
  values.
