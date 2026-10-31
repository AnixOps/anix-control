### Removed

- The load balancer feature. Nothing ever selected a node through it: the
  system settings page 负载均衡, the `/api/v2/admin/loadbalancers*` routes
  (and their `proxy.loadbalancer.*` package routes, `LoadBalancerService`
  and the proxy-node `kernel.storage.adopt:v2_load_balancer`,
  `kernel.view:kapi_forward_node_v1` and `kernel.nodeops.diagnose.v1`
  capabilities) are gone. Forward load balancing and failover are
  unaffected.

### Changed

- Migration `0021_drop_load_balancer` drops the unused `v2_load_balancer`
  table (SQLite and PostgreSQL).
