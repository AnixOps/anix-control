### Added

- Forwarding keeps a latency history: every upstream health check the
  nodes report is folded into five-minute buckets per route, hop, node and
  upstream (`v4_kernel_forward_latency`, migration 0014), kept for 7 days.
  `ForwardControl.GetLatency` and `GET /api/v4/forward/routes/{id}/latency`
  answer them, and the route page draws each hop's latency over the last
  24 hours or 7 days with the failed checks counted.
