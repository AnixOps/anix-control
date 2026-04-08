# Configuration

This page defines the unified configuration scheme for `v2board_AnixOps`.

## Source Of Truth

| Location | Role | Notes |
|------|------|------|
 | [`config/config.yaml`](../../config/config.yaml) | canonical app and forward runtime config | always read on backend startup |
 | `v2_system_config` | persisted runtime snapshot | written on startup from the YAML config and consumed by runtime services and `/admin/system` |

Important:

- local `go run` does not auto-load [`.env`](../../.env)
- `jwt.secret`, `app.api_token`, `admin.*`, database, cache, and frontend settings still come from [`config/config.yaml`](../../config/config.yaml)
- the runtime selection resides in `config/config.yaml.forward_runtime`; startup writes exactly those values into `v2_system_config`

## Unified Forward Runtime Layout

Put the runtime selection in [`config/config.yaml`](../../config/config.yaml):

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

## Related Docs

- [`forward-runtime-migration.md`](forward-runtime-migration.md)
- [`startup-config.md`](startup-config.md)
- [`runtime.md`](runtime.md)
- [`../guide/forward-relay-onboarding.md`](../guide/forward-relay-onboarding.md)
- [`../guide/forward-tunnel-smoke-test.md`](../guide/forward-tunnel-smoke-test.md)
