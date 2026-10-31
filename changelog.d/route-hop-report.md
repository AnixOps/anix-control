### Fixed

- An ANX route no longer shows every node, the exit included, as never
  reported, nor only its first hop. A node on the network model reports in its
  `NetworkStatus` (network.v1), which now fills its summary, and
  `GET /routes/{id}/health` adds each hop's own transport and exit egress with
  the round trip its device measured. Needs an Agent that reports link round
  trips for them to show.
