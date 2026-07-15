# Runtime Modes

This page is the runtime reference for the forward split in `anix-control`.

## Core Rule

Creating a `ForwardNode` under `/admin/forward/nodes` only creates control-plane metadata.

Real runtime attachment happens only when one of these execution paths succeeds:

- `gost` via NodeX
- `nftables_ansible` via the local panel-host executor (recommended default)
- `iptables_ansible` via the same local executor (legacy compatibility only)

## Unified Config Entry

Both modes now start from the same place:

```yaml
forward_runtime:
  backend: "gost"
  nodex:
    base_url: "http://127.0.0.1:18081"
    token: "replace-with-shared-token"
    timeout_seconds: 15
  nftables_ansible:
    inventory: "config/deploy/ansible/inventory.ini"
    apply_playbook: "config/deploy/ansible/playbooks/forward_apply_nftables.yml"
    remove_playbook: "config/deploy/ansible/playbooks/forward_remove_nftables.yml"
    working_dir: "config/deploy/ansible"
    target_pattern: "{{node.host}}"
    environment:
      ANSIBLE_CONFIG: "config/deploy/ansible/ansible.cfg"
    timeout_seconds: 120
  iptables_ansible:
    # Optional legacy override. Use only for old relay environments.
    inventory: "config/deploy/ansible/inventory.ini"
    apply_playbook: "config/deploy/ansible/playbooks/forward_apply.yml"
    remove_playbook: "config/deploy/ansible/playbooks/forward_remove.yml"
```

Runtime configuration now comes exclusively from `config/config.yaml.forward_runtime`. Legacy `FORWARD_RUNTIME_*` variables are no longer consulted during startup.

## Control Plane Vs Execution Plane

- `v2board_AnixOps`
  - public control plane
  - admin UI
  - persistence
  - diagnostics and job views
- `NodeX`
  - internal-only execution control plane
  - stateful `gost` runtime
  - `Node-Agent` and operator tooling (status, doctor, runtime jobs)
- `nftables_ansible` / `iptables_ansible`
  - stateless local execution path on the panel host
  - background executor inside `v2board`
  - `ansible-playbook` plus SSH and playbooks

## Admin Entry Points

- `/admin/forward/ansible-machines`
  - machine inventory for stateless Ansible execution
  - does not require NodeX control-plane or Node-Agent
- `/admin/forward/local`
  - local stateless runtime settings and diagnostics
  - preferred backend is `nftables_ansible`
- `/admin/forward/nodex`
  - dedicated NodeX operator page
  - stateful control-plane settings, health, doctor, and `gost` jobs
- `/admin/forward/nodes`
  - NodeX topology view (relay/exit semantics)
  - separate from Ansible execution-machine management

## NodeX Mode

Use this when you want the private stateful runtime:

```yaml
forward_runtime:
  backend: "gost"
  nodex:
    base_url: "http://127.0.0.1:18081"
    token: "replace-with-shared-token"
    timeout_seconds: 15
```

Operational implications:

- `InNodeID` ingress semantics apply only in this mode
- `forward_runtime.nodex.base_url` and `forward_runtime.nodex.token` are required
- `ForwardNode.host`, `ForwardNode.api_port`, and `ForwardNode.api_token` must point to a reachable relay gost API
- NodeX-side operator path is expected (`Node-Agent`, control-plane status, doctor workflow)
- the runtime is not attached until NodeX can create or update relay services and limiters

Actual flow:

1. `v2board` builds a runtime request from the selected forward and tunnel
2. `v2board` calls NodeX
3. NodeX talks to the relay gost API
4. NodeX creates or updates relay services and limiters

## Local Ansible Mode (`nftables_ansible` Recommended)

Use this when you want stateless relay execution without NodeX:

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

Operational implications:

- no NodeX control-plane is required
- no Node-Agent is required
- no proxy ingress node is required in this stateless path
- `ansible-playbook` must exist on the `v2board` executor host
- SSH credentials come from the configured ansible inventory
- the runtime is attached only after the playbook succeeds on the relay host

Actual flow:

1. `v2board` stores a pending runtime job in `v2_forward_runtime_job`
2. the local executor polls pending local-ansible jobs
3. the executor runs `ansible-playbook`
4. the relay host receives or removes `nftables` rules

Legacy compatibility:

- `iptables_ansible` is still supported for old relays or old playbooks.
- It uses the same panel-host executor, but applies `iptables` playbooks.
- Do not treat `iptables_ansible` as the default path.

## Current Health Semantics

The current forward-node online indicator is limited:

- panel health only tests TCP reachability to `ForwardNode.host:ForwardNode.port`
- a green node does not prove:
  - NodeX is reachable
  - relay gost API is healthy
  - `api_token` matches
  - ansible can SSH into the host
  - active forward rules already exist

Treat `/admin/forward/nodes` status as coarse reachability only.

## Resource Naming Rule

Keep these resource types separate:

- `/admin/nodes`
  - proxy nodes
- `/admin/forward/nodes`
  - forward execution nodes
- `/admin/forward/tunnel`
  - tunnel inventory

## Related Docs

- [`configuration.md`](configuration.md)
- [`startup-config.md`](startup-config.md)
- [`../guide/forward-relay-onboarding.md`](../guide/forward-relay-onboarding.md)
- [`../guide/forward-tunnel-smoke-test.md`](../guide/forward-tunnel-smoke-test.md)
- [`../guide/nodex-internal-extension.md`](../guide/nodex-internal-extension.md)
