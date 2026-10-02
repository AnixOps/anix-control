# Configuration

This page defines the unified configuration scheme for `anix-control`.

## Source Of Truth

| Location | Role | Notes |
|------|------|------|
 | `config/config.yaml` (template: [`config/config.yaml.example`](../../config/config.yaml.example)) | canonical app and forward runtime config | read on startup when it exists, or when named by `-config` / `ANIX_CONTROL_CONFIG` |
 | built-in defaults ([`internal/config/defaults.yaml`](../../internal/config/defaults.yaml)) | production-shaped container defaults | used instead of a file when no config file is present |
 | `ANIX_CONTROL_*` environment variables | per-key overrides and secrets | applied on top of the file or the built-in defaults; see [Environment Variables](#environment-variables) |
 | `v2_system_config` | persisted runtime snapshot | written on startup from the loaded config and consumed by runtime services and `/admin/system` |

Important:

- local `go run` does not auto-load `.env` (template: [`.env.example`](../../.env.example))
- `jwt.secret`, `app.api_token`, `admin.*`, database, cache, and frontend settings come from the config file or its `ANIX_CONTROL_*` override
- the runtime selection resides in `forward_runtime`; startup writes exactly those values into `v2_system_config`

## Environment Variables

Every scalar and list key can be set from the environment. The variable name
is `ANIX_CONTROL_` plus the upper-cased YAML path joined with underscores:

| YAML key | Variable |
|------|------|
| `database.password` | `ANIX_CONTROL_DATABASE_PASSWORD` |
| `jwt.secret` | `ANIX_CONTROL_JWT_SECRET` |
| `plugins.control_execution_enabled` | `ANIX_CONTROL_PLUGINS_CONTROL_EXECUTION_ENABLED` |
| `server.trusted_proxies` | `ANIX_CONTROL_SERVER_TRUSTED_PROXIES` (comma-separated) |

Rules:

- Precedence: config file (or built-in defaults) first, then the environment.
- `NAME_FILE=/path` reads the value from a file, for Docker and Kubernetes
  secrets. Setting both `NAME` and `NAME_FILE` is an error.
- Booleans accept `true`/`false`/`1`/`0`; lists are comma-separated.
- Map-valued keys (`forward_runtime.nftables_ansible.environment` and
  `extra_vars`) can only be set in the YAML file.
- Unknown `ANIX_CONTROL_*` names are logged at startup, so typos are visible.
- `anix-control -print-env` prints every variable with its type and built-in
  default; secret defaults are never printed.
- `server.trusted_proxies` (IPs or CIDRs) are the reverse proxies whose
  `X-Forwarded-Proto`, `X-Forwarded-Host`, `X-Forwarded-For` and
  `X-Real-IP` Control honours (`internal/requestorigin`); from any other peer
  they are ignored. Unset means `127.0.0.1/32,::1/128`; an empty list trusts
  none; a list replaces the default. See `docs/DEPLOYMENT.md` section 6.1.

Without a config file, Control starts from the built-in defaults: `env:
production`, API on `0.0.0.0:8080`, UI on `3000`, PostgreSQL
`anix_control@127.0.0.1:5432`, Control package execution enabled with private
temporary host directories, gRPC disabled. In production `jwt.secret` is
required and must not be a template value, or the server refuses to start.

Container-related settings:

| Key | Default | Notes |
|------|------|------|
| `server.shutdown_drain_delay` | empty (`5s` in built-in defaults) | time to keep serving after `/readyz` starts failing on shutdown; at most `5m` |
| `log.format` | `text` | `json` writes one JSON object per line to stdout (standard log lines, access logs, GORM); `text` keeps the historical output |
| `log.level` | `info` | minimum level for structured records in `json` format; standard log lines are always written |

PostgreSQL connection settings:

| Key | Default | Notes |
|------|------|------|
| `database.sslmode` | `disable` | libpq `sslmode`: `disable`, `allow`, `prefer`, `require`, `verify-ca`, `verify-full` |
| `database.timezone` | `Asia/Shanghai` | session `TimeZone`; must be a valid IANA name |
| `database.dsn` | empty | full connection string used verbatim instead of the fields above |

## Product Edition

| Key | Default | Notes |
|------|------|------|
| `app.edition` (`ANIX_CONTROL_APP_EDITION`) | `community` | `community` hides orders, payments (every gateway and callback), coupons, the invite commission and plan purchase; plans stay as free subscription templates. `commercial` serves everything. Any other value stops the server at start-up. |

The commercial packages and routes are listed in `config/editions.json`.
In the community edition their `/api/v2` routes answer like an undeclared
route, the web app hides their menus and pages, and the release leaves the
packages out. The web app reads the edition from the public
`GET /api/v4/public/config`. Installs that use payments must set
`commercial` before upgrading past 4.1.0-rc.2 (`docs/UPGRADE.md`).

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
  `ANIX_CONTROL_HOST_MAX_RESPONSE_BYTES`; hosts built on `sdk/pluginhostsdk`
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
`plugin_response_too_large`, `sealed_secret_unavailable`,
`sealed_secret_refused`), and the
`anixops_v2_gateway_request_duration_seconds{package}` histogram. `route` is
the package route id from the signed declaration, never the request path;
requests that match no declared route are counted under
`package="unresolved",route="unresolved"`.

The routes of `config/node-secret-fields.json` have their node secrets sealed
before a package host reads them (`docs/architecture/node-ops-service.md`
section 3.7).

- **The metric.** `anixops_v2_gateway_sealed_secrets_total`, with the labels
  `package`, `route`, `stage`, `result` and `reason`, counts the gateway's
  work:
  - `stage="request"`: requests sealed (`result="sealed"`), served by the
    kernel's legacy handler without the host because they could not be
    sealed (`legacy_fallback`), or refused (`refused`);
  - `stage="answer"`: handles expanded (`expanded`) or refused (`refused`).
- **Reasons.** `reason` is one of `not_json`, `value_not_string`,
  `target_invalid`, `too_large`, `fields_unavailable`, `answer_handle`,
  `store` or `none`.
- **Errors.** `sealed_secret_unavailable` (503) is a request that could not
  be sealed on a route with no legacy handler. `sealed_secret_refused` (502)
  is an answer that would show a handle.

While package execution is enabled, `/metrics` also reports each supervised
package host: `anixops_plugin_host_starts_total`,
`anixops_plugin_host_unexpected_exits_total`,
`anixops_plugin_host_restarts_total` and `anixops_plugin_host_failures_total`
(all `{package}`), and `anixops_plugin_host_state{package,state}`, which is `1`
for the current state (`running`, `restarting`, `failed`, `exited`,
`stopped`) and `0` for the others.

## Agent Client Certificates

`agent_control.mtls` (`ANIX_CONTROL_AGENT_CONTROL_MTLS`) sets how AnixOps
Agents authenticate on the gRPC listener (`grpc.*`):

```yaml
agent_control:
  mtls: "optional"   # optional | preferred | required
```

| Mode | Agents without a client certificate |
|------|------|
| `optional` (default) | authenticate with their node API key, as before |
| `preferred` | still authenticate; the control stream answers with `x-anix-auth-deprecated` |
| `required` | are refused by the Agent services; `Enroll` takes only one-time enrollment credentials |

- Agents get certificates from `anix.agent.v1.AgentEnrollment`, signed by the
  built-in CA. It needs only `module_runtime.ca_kek`
  (`ANIX_CONTROL_MODULE_RUNTIME_CA_KEK`, 32 bytes, base64 or hex) with
  `module_runtime.pki: builtin`; `module_runtime.enabled` and the module
  listener are not needed. Without the CA, or with `pki: external`,
  `optional` behaves exactly as before.
- `preferred` and `required` need `grpc.tls_cert_file`, `grpc.tls_key_file`
  and the built-in CA; startup fails otherwise.
- Enrollment credentials: `anix-control agent token create -node proxy-12`
  or `POST /api/v4/kernel/agents/enrollment-tokens`. Details in
  [`../architecture/module-runtime.md`](../architecture/module-runtime.md#agent-pki).

## Related Docs

- [`forward-runtime-migration.md`](forward-runtime-migration.md)
- [`startup-config.md`](startup-config.md)
- [`runtime.md`](runtime.md)
- [`../guide/forward-relay-onboarding.md`](../guide/forward-relay-onboarding.md)
- [`../guide/forward-tunnel-smoke-test.md`](../guide/forward-tunnel-smoke-test.md)
