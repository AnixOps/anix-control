# Forward Runtime Migration

This page is the explicit migration note for the `forward_runtime` config cleanup.

Current rule:

- runtime backend selection now comes only from `config/config.yaml.forward_runtime`
- startup writes that YAML snapshot into `v2_system_config`
- old `FORWARD_RUNTIME_*` runtime config inputs are retired and no longer read
- local worker tuning also lives in YAML now under `forward_runtime.jobs` and `forward_runtime.gost_stats`

## What Changed

Before this cleanup, some deployments mixed:

- `config/config.yaml.forward_runtime`
- `.env`
- Compose-exported `FORWARD_RUNTIME_*`
- ad-hoc shell exports

That made the real runtime source of truth hard to reason about.

Now the runtime config path is single-source:

1. edit `config/config.yaml`
2. restart `v2board`
3. verify `/admin/system` or the runtime diagnostics page if needed

## Old To New Mapping

| Old input | New YAML key (recommended) | Legacy-compatible key |
|-----------|-----------------------------|-----------------------|
| `FORWARD_RUNTIME_BACKEND` | `forward_runtime.backend` | same |
| `FORWARD_RUNTIME_NODEX_MODE` | `forward_runtime.nodex_mode` | same |
| `FORWARD_RUNTIME_NODEX_BASE_URL` | `forward_runtime.nodex.base_url` | same |
| `FORWARD_RUNTIME_NODEX_TOKEN` | `forward_runtime.nodex.token` | same |
| `FORWARD_RUNTIME_NODEX_TIMEOUT_SECONDS` | `forward_runtime.nodex.timeout_seconds` | same |
| `FORWARD_RUNTIME_ANSIBLE_INVENTORY` | `forward_runtime.nftables_ansible.inventory` | `forward_runtime.iptables_ansible.inventory` |
| `FORWARD_RUNTIME_ANSIBLE_PLAYBOOK_APPLY` | `forward_runtime.nftables_ansible.apply_playbook` | `forward_runtime.iptables_ansible.apply_playbook` |
| `FORWARD_RUNTIME_ANSIBLE_PLAYBOOK_REMOVE` | `forward_runtime.nftables_ansible.remove_playbook` | `forward_runtime.iptables_ansible.remove_playbook` |
| `FORWARD_RUNTIME_ANSIBLE_WORKDIR` | `forward_runtime.nftables_ansible.working_dir` | `forward_runtime.iptables_ansible.working_dir` |
| `FORWARD_RUNTIME_ANSIBLE_TARGET_PATTERN` | `forward_runtime.nftables_ansible.target_pattern` | `forward_runtime.iptables_ansible.target_pattern` |
| `FORWARD_RUNTIME_ANSIBLE_COMMAND` | `forward_runtime.nftables_ansible.command` | `forward_runtime.iptables_ansible.command` |
| `FORWARD_RUNTIME_ANSIBLE_TIMEOUT_SECONDS` | `forward_runtime.nftables_ansible.timeout_seconds` | `forward_runtime.iptables_ansible.timeout_seconds` |
| `FORWARD_RUNTIME_ANSIBLE_BECOME` | `forward_runtime.nftables_ansible.become` | `forward_runtime.iptables_ansible.become` |
| `FORWARD_RUNTIME_ANSIBLE_EXTRA_VARS_JSON` | `forward_runtime.nftables_ansible.extra_vars` | `forward_runtime.iptables_ansible.extra_vars` |
| `FORWARD_RUNTIME_ANSIBLE_ENV_JSON` | `forward_runtime.nftables_ansible.environment` | `forward_runtime.iptables_ansible.environment` |
| `FORWARD_RUNTIME_ANSIBLE_CONFIG_JSON` | split into the individual `forward_runtime.nftables_ansible.*` keys above | split into `forward_runtime.iptables_ansible.*` keys |

Historical note:

- old `FORWARD_RUNTIME_ANSIBLE_*` values were originally consumed by the iptables-oriented local path.
- migration now recommends placing those values under `forward_runtime.nftables_ansible.*`.
- use `forward_runtime.iptables_ansible.*` only when you intentionally keep the legacy iptables playbook path.

Worker tuning also moved into YAML:

| Old input | New YAML key |
|-----------|--------------|
| `FORWARD_RUNTIME_JOB_POLL_INTERVAL` | `forward_runtime.jobs.poll_interval` |
| `FORWARD_RUNTIME_JOB_IDLE_POLL_INTERVAL` | `forward_runtime.jobs.idle_poll_interval` |
| `FORWARD_RUNTIME_JOB_ERROR_LOG_INTERVAL` | `forward_runtime.jobs.error_log_interval` |
| `FORWARD_GOST_STATS_POLL_INTERVAL` | `forward_runtime.gost_stats.poll_interval` |
| `FORWARD_GOST_STATS_IDLE_POLL_INTERVAL` | `forward_runtime.gost_stats.idle_poll_interval` |
| `FORWARD_GOST_STATS_ERROR_LOG_INTERVAL` | `forward_runtime.gost_stats.error_log_interval` |

## Removed Shortcut Path

These legacy variables were previously used to synthesize or override ansible inventory state and are now out of scope:

- `FORWARD_RUNTIME_ANSIBLE_HOST_ALIAS`
- `FORWARD_RUNTIME_ANSIBLE_HOST`
- `FORWARD_RUNTIME_ANSIBLE_PORT`
- `FORWARD_RUNTIME_ANSIBLE_USER`
- `FORWARD_RUNTIME_ANSIBLE_PASSWORD`
- `FORWARD_RUNTIME_ANSIBLE_BECOME_PASSWORD`

Use one of these instead:

- put SSH credentials directly in `config/deploy/ansible/inventory.ini`
- use SSH keys under `config/deploy/ssh/`
- pass ansible-level values through `forward_runtime.nftables_ansible.extra_vars` (or `forward_runtime.iptables_ansible.extra_vars` for legacy compatibility)

Do not expect `v2board` to generate inventory files from env variables anymore.

## Minimal Migration Examples

### NodeX / gost

Old habit:

```dotenv
FORWARD_RUNTIME_BACKEND=gost
FORWARD_RUNTIME_NODEX_BASE_URL=http://127.0.0.1:18081
FORWARD_RUNTIME_NODEX_TOKEN=replace-with-token
```

New config:

```yaml
forward_runtime:
  backend: "gost"
  nodex:
    base_url: "http://127.0.0.1:18081"
    token: "replace-with-token"
    timeout_seconds: 15
```

### Local ansible / nftables (recommended)

Old habit:

```dotenv
FORWARD_RUNTIME_BACKEND=nftables_ansible
FORWARD_RUNTIME_ANSIBLE_INVENTORY=config/deploy/ansible/inventory.ini
FORWARD_RUNTIME_ANSIBLE_PLAYBOOK_APPLY=config/deploy/ansible/playbooks/forward_apply_nftables.yml
FORWARD_RUNTIME_ANSIBLE_PLAYBOOK_REMOVE=config/deploy/ansible/playbooks/forward_remove_nftables.yml
FORWARD_RUNTIME_ANSIBLE_BECOME=true
```

New config:

```yaml
forward_runtime:
  backend: "nftables_ansible"
  nftables_ansible:
    inventory: "config/deploy/ansible/inventory.ini"
    apply_playbook: "config/deploy/ansible/playbooks/forward_apply_nftables.yml"
    remove_playbook: "config/deploy/ansible/playbooks/forward_remove_nftables.yml"
    working_dir: "config/deploy/ansible"
    target_pattern: "{{node.host}}"
    become: true
    timeout_seconds: 120
    environment:
      ANSIBLE_CONFIG: "config/deploy/ansible/ansible.cfg"
```

If you need the old firewall driver/playbooks, keep the same structure but switch `backend` and block name to `iptables_ansible`.
Historical note: older local deployments often used `FORWARD_RUNTIME_BACKEND=iptables_ansible` together with `forward_apply.yml` / `forward_remove.yml`; keep that combination only when you intentionally stay on the legacy iptables path.

## Operational Checks

After migrating:

1. start `v2board` with `-config ./config/config.yaml`
2. confirm any old `FORWARD_RUNTIME_*` shell exports or compose vars are removed from your deployment
3. confirm the expected mode in `runtime.md`
4. run the matching smoke path from `../guide/forward-tunnel-smoke-test.md`

## Related Pages

- [startup-config.md](./startup-config.md)
- [configuration.md](./configuration.md)
- [runtime.md](./runtime.md)
- [../guide/forward-relay-onboarding.md](../guide/forward-relay-onboarding.md)
