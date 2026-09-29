# Configuration

This page defines the unified configuration scheme for `anix-control`.

## Source Of Truth

| Location | Role | Notes |
|------|------|------|
 | `config/config.yaml` (template: [`config/config.yaml.example`](../../config/config.yaml.example)) | canonical app and forward runtime config | always read on backend startup |
 | `v2_system_config` | persisted runtime snapshot | written on startup from the YAML config and consumed by runtime services and `/admin/system` |

Important:

- local `go run` does not auto-load `.env` (template: [`.env.example`](../../.env.example))
- `jwt.secret`, `app.api_token`, `admin.*`, database, cache, and frontend settings still come from `config/config.yaml`
- the runtime selection resides in `config/config.yaml.forward_runtime`; startup writes exactly those values into `v2_system_config`

## Unified Forward Runtime Layout

Put the runtime selection in `config/config.yaml`:

```yaml
forward_runtime:
  backend: "gost"

  jobs:
    poll_interval: "5s"
    idle_poll_interval: "30s"
    error_log_interval: "1m"
    batch_size: 10
    timeout_seconds: 120

  gost_stats:
    poll_interval: "30s"
    idle_poll_interval: "2m"
    error_log_interval: "5m"

  nodex:
    base_url: "http://127.0.0.1:18081"
    token: "replace-with-shared-token"
    timeout_seconds: 15

  nftables_ansible:
    inventory: "config/deploy/ansible/inventory.ini"
    apply_playbook: "config/deploy/ansible/playbooks/forward_apply_nftables.yml"
    remove_playbook: "config/deploy/ansible/playbooks/forward_remove_nftables.yml"
    become: false
    extra_vars: {}
    command: ""
    working_dir: "config/deploy/ansible"
    target_pattern: "{{node.host}}"
    environment:
      ANSIBLE_CONFIG: "config/deploy/ansible/ansible.cfg"
    timeout_seconds: 120

  iptables_ansible:
    # Legacy compatibility only
    inventory: "config/deploy/ansible/inventory.ini"
    apply_playbook: "config/deploy/ansible/playbooks/forward_apply.yml"
    remove_playbook: "config/deploy/ansible/playbooks/forward_remove.yml"
    become: false
    extra_vars: {}
    command: ""
    working_dir: "config/deploy/ansible"
    target_pattern: "{{node.host}}"
    environment:
      ANSIBLE_CONFIG: "config/deploy/ansible/ansible.cfg"
    timeout_seconds: 120
```

Startup behavior:

1. backend loads `config/config.yaml`
2. `forward_runtime` is parsed
3. merged values from `forward_runtime` are persisted into `v2_system_config`
4. `/admin/system` and forward runtime services consume the persisted values

Optional worker tuning now also stays inside `forward_runtime`:

- `forward_runtime.jobs.*` controls the local ansible runtime job executor
- `forward_runtime.gost_stats.*` controls the gost traffic polling worker
- both use the same YAML file instead of separate env overrides

## `nodex_mode` Compatibility Switch

`forward_runtime.nodex_mode` remains supported for compatibility:

- `true` forces backend to `gost`
- `false` forces backend to the local ansible path, defaulting to `nftables_ansible`
- if omitted, `forward_runtime.backend` is used directly

If both are present, `nodex_mode` wins.

## NodeX Mode

Use this for the private stateful runtime:

```yaml
forward_runtime:
  backend: "gost"
  nodex:
    base_url: "http://127.0.0.1:18081"
    token: "replace-with-shared-token"
    timeout_seconds: 15
```

Required values:

- `forward_runtime.nodex.base_url`
- `forward_runtime.nodex.token`

## Local Ansible Mode (`nftables_ansible` Recommended)

Use this for the local stateless executor:

```yaml
forward_runtime:
  backend: "nftables_ansible"
  nftables_ansible:
    inventory: "config/deploy/ansible/inventory.ini"
    apply_playbook: "config/deploy/ansible/playbooks/forward_apply_nftables.yml"
    remove_playbook: "config/deploy/ansible/playbooks/forward_remove_nftables.yml"
    working_dir: "config/deploy/ansible"
    target_pattern: "{{node.host}}"
    environment:
      ANSIBLE_CONFIG: "config/deploy/ansible/ansible.cfg"
    timeout_seconds: 120
```

Operational rules:

- no NodeX control-plane is required
- no proxy ingress node is required
- the executor host must have `ansible-playbook`
- SSH credentials still come from inventory files referenced by the selected ansible block under `config/config.yaml.forward_runtime`
- `iptables_ansible` remains available only for legacy relay environments that still need the old firewall driver and playbooks

## Path Resolution

The selected local ansible block (`forward_runtime.nftables_ansible` by default, `forward_runtime.iptables_ansible` for legacy hosts) has these path fields normalized on startup:

- `inventory`
- `apply_playbook`
- `remove_playbook`
- `working_dir`
- `environment.ANSIBLE_CONFIG`

Relative paths are resolved from the config file and project/runtime location so local binary, Windows, Linux, and Docker stay aligned.

## Runtime Keys Written Into System Config

Startup persists the merged runtime snapshot into these keys:

- `forward.runtime_backend`
- `forward.runtime.nodex_mode`
- `forward.runtime.ansible.backend`
- `forward.runtime.nodex.base_url`
- `forward.runtime.nodex.token`
- `forward.runtime.nodex.timeout_seconds`
- `forward.runtime.ansible.config`
- `forward.runtime.ansible.inventory`
- `forward.runtime.ansible.apply_playbook`
- `forward.runtime.ansible.remove_playbook`
- `forward.runtime.ansible.become`
- `forward.runtime.ansible.extra_vars_json`

Legacy compatibility readers still understand:

- `forward.runtime.iptables_ansible.config`
- `forward.ansible.*`

## Control Package Payload Limits

`/api/v2` compatibility routes are served by signed control packages. Two
`plugins` keys bound the bodies that cross the package boundary:

```yaml
plugins:
  control_host_max_request_bytes: 1048576   # 0 or unset = 1 MiB
  control_host_max_response_bytes: 1048576  # 0 or unset = 1 MiB
```

| Key | Enforced by | Error when exceeded |
|------|------|------|
| `control_host_max_request_bytes` | the v2 gateway, before dispatch | `413` `plugin_request_too_large` |
| `control_host_max_response_bytes` | the package bridge (legacy handler body), the package-host SDK (package response body), the kernel host client | `502` `plugin_response_too_large` |

- Both values are integers in bytes, from `0` (default 1 MiB) to `67108864`
  (64 MiB). Startup fails on a negative or larger value.
- The response limit applies to the body only. The SDK additionally allows a
  fixed 64 KiB for status, headers, and identifiers, so a body the bridge
  accepts is never rejected by the host for its envelope.
- The kernel passes the response limit to each package host as
  `ANIX_CONTROL_HOST_MAX_RESPONSE_BYTES`; hosts built on `pkg/pluginhostsdk`
  read it with `MaxResponseBytesFromEnvironment`.
- gRPC receive sizes grow with the limits, so values above 4 MiB work end to
  end. Package hosts should build their gRPC server with
  `pluginhostsdk.HostServerOptions()`.
- WebSocket frames are not affected by these keys.

The gateway exports per-package traffic on `/metrics`:
`anixops_v2_gateway_requests_total{package,route,code_class}`,
`anixops_v2_gateway_errors_total{package,route,code}` (gateway error codes such
as `package_unavailable`, `package_route_not_found`, `plugin_host_unavailable`,
`plugin_host_incompatible`, `plugin_request_too_large`,
`plugin_response_too_large`), and the
`anixops_v2_gateway_request_duration_seconds{package}` histogram. `route` is
the package route id from the signed declaration, never the request path;
requests that match no declared route are counted under
`package="unresolved",route="unresolved"`.

While package execution is enabled, `/metrics` also reports each supervised
package host: `anixops_plugin_host_starts_total`,
`anixops_plugin_host_unexpected_exits_total`,
`anixops_plugin_host_restarts_total` and `anixops_plugin_host_failures_total`
(all `{package}`), and `anixops_plugin_host_state{package,state}`, which is `1`
for the current state (`running`, `restarting`, `failed`, `exited`,
`stopped`) and `0` for the others.

## Related Docs

- [`forward-runtime-migration.md`](forward-runtime-migration.md)
- [`startup-config.md`](startup-config.md)
- [`runtime.md`](runtime.md)
- [`../guide/forward-relay-onboarding.md`](../guide/forward-relay-onboarding.md)
- [`../guide/forward-tunnel-smoke-test.md`](../guide/forward-tunnel-smoke-test.md)
