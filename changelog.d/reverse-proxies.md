### Added

- 反向代理 (Reverse proxies), under 节点 → 反向代理
  (`/admin/forward/reverse-proxies`, `GET/POST /api/v4/kernel/reverse-proxies`,
  `PUT/DELETE /api/v4/kernel/reverse-proxies/{id}`). A reverse proxy publishes
  HTTP(S) sites on entry nodes. Each request goes by host name
  (`example.com`, `*.example.com`, `*`) and path prefix to a route's upstreams,
  with balancing, health checks and a circuit breaker. The upstreams are dialed
  from the entry, or from an exit node that Control links to over ANX-A, as it
  does for a forward route's hops. TLS uses an automatic certificate or an
  uploaded one, and an optional HTTP port redirects to HTTPS. Control seals the private key under
  `module_runtime.ca_kek`, and no answer returns it.
  - The entry nodes run it on anixcore and need an Agent and an anx-node with
    the `reverse_proxy` feature.
  - Automatic certificates: Control obtains them from Let's Encrypt (or the
    directory in `ANIX_CONTROL_ACME_DIRECTORY_URL`) with a DNS-01 challenge.
    The TXT records are written through one of the forwarding DNS providers
    (Cloudflare, Alibaba Cloud DNS, DNSPod, Huawei Cloud DNS, webhook) in the
    zone you name. Control renews them 30 days before expiry and retries a
    failure after an hour, and the page shows the error. Only the
    singleton-worker process issues them.
  - The forwarding DNS providers also write TXT records.
  - Migration `0022_reverse_proxies` creates `v5_reverse_proxy` and
    `v5_reverse_proxy_acme_account` (the ACME account, its key sealed).
