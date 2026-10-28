### Changed

- Network model transports: bare `anx_line` is only for trusted segments
  (IX, private line, domestic) and needs `trusted: true` in the document.
  Every other segment, cross-border ones above all, takes the new kind
  `anx` (ANX-A): planned as an `anx_line` link under the A2 jacket (`l2`,
  server name required, c1 only), which anx-server shapes. WireGuard is not
  used across borders.
