### Fixed

- Cache `networkmodel.Service.DevicePlan` per `(ConnPool, node, generation, planHash)` to eliminate per-minute full-cluster replan and AES-GCM decrypt storms across connected Agent sessions.
- Reject multi-hop cycles in `networkmodel.Plan` so forwarding paths without a data-plane TTL cannot loop.
- Remove the 30-minute `MaxConnectionAge` cap on the Agent gRPC server so healthy control streams are not forcefully cycled.
