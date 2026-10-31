### Added

- SDK: `NetworkPlan.reverse_proxies` (an HTTP(S) reverse proxy listener:
  sites by host, routes by path prefix to balanced upstreams, optional TLS
  certificate, next hop an egress or a route transport) and
  `NetworkStatus.reverse_proxies`, with plan and status validation. Needs
  a kernel with the `reverse_proxy` feature.
