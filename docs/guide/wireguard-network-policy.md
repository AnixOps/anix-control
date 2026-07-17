# WireGuard Entry Network Policy

WSS entry nodes with multiple uplinks can define `relay.network_policy`. Nodes
without this object keep using the operating system main route exactly as
before. Version 1 implements active/standby failover and accepts any number of
IPv4 or IPv6 paths; it does not assume two interfaces and does not perform
per-packet load balancing.

Version 1 is limited to WSS because its health check must complete a real TCP
connection to the relay. QUIC keeps its existing single-route behavior until
an authenticated UDP probe is implemented.

```json
{
  "network_policy": {
    "version": 1,
    "strategy": "failover",
    "paths": [
      {"name":"cn2","interface":"eth1","source":"10.8.0.112","gateway":"10.8.0.1","priority":10},
      {"name":"9929","interface":"eth0","source":"10.7.0.112","gateway":"10.7.0.1","priority":20}
    ],
    "health_check": {
      "interval_seconds": 10,
      "timeout_seconds": 3,
      "failure_threshold": 3,
      "recovery_threshold": 2,
      "failback_delay_seconds": 300
    }
  }
}
```

Each path gets an independent source-policy routing table so replies leave by
the same uplink. A separate destination rule steers only the GOST connection
to `relay.server`, leaving the host default route untouched. For this reason,
`relay.server` must be a literal IP when network policy is enabled. Paths whose
address family differs from the relay IP still receive source routing for
symmetry but are not relay failover candidates. V2bX probes the relay TCP port
from each candidate source address and restarts GOST after switching paths.

Generated table IDs and rule priorities are deterministic. Operators may
override them with `routing_table` and `rule_priority` on a path, plus
`active_table` and `active_priority` on the policy, when the host already uses
those ranges. The panel validates these fields before publishing node config.
