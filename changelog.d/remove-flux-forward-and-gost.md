### Removed

- The v3 flux forwarding panel and its runtime, replaced by forward.v1 on
  the network model: the admin forward, tunnel, NodeX, test-connection and
  forward statistics routes, the speed-limit routes, the user forward
  routes, the forward-agent register/heartbeat/report routes,
  `/api/v2/forward/agent/rules`, `/flow/upload` and the internal forward
  traffic routes now answer 404. `POST /api/v2/user/reset` stays and
  accepts only `type: 1` (the user's own traffic).
- The NodeX, local and Ansible forward runtimes, the gost and Ansible
  statistics workers, the flux flow-reset worker, the latency prober, the
  forward-agent bridge worker and `anix-control forward legacy`. The
  `forward_runtime.*` configuration keys are ignored (a warning names them)
  and the 系统设置 › 运行时 page is gone. The v3 forward tables stay in the
  database untouched.
- The KernelNodeOps `forward.*`, `diagnose.forward`, `diagnose.node_stats`,
  `diagnose.tunnel`, `diagnose.forward_backend` and `cleanagent.issue`
  kinds answer UNIMPLEMENTED; `diagnose.endpoints` stays.
- The `gost-mesh` official package (its Control host, WebUI, release
  gate and pinned gost runtime). Control now builds 17 official packages.
- 诊断's generic actions name only `anix-agent`: the Agent no longer runs
  gost, so `gost` is refused as a service.

### Changed

- The Agent install script no longer creates `anixops-gost.service`, the
  `anixops-gost` account or `/var/lib/anixops-gost`; it retires them with
  the rest of the legacy forward runtime when an earlier install left them.
  The Agent unit drops the `anixops-gost` supplementary group and the polkit
  rule only lets the Agent restart its own unit.
