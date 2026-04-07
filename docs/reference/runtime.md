# Runtime Modes

This page is the reference entrypoint for the forward runtime split in `v2board_AnixOps`.

## Control Plane Vs Execution Plane

- `v2board_AnixOps`
  - public control plane
  - admin UI
  - persistence
  - `/api/v2/admin/forward/runtime/*` proxy and diagnostics endpoints
- `NodeX`
  - internal-only execution plane
  - stateful gost runtime
  - doctor, version, upgrade, and operator workflows
- `iptables_ansible`
  - stateless execution path
  - ansible + SSH + playbook driven forwarding

## Mode Split

### NodeX Mode

Use when you want the private stateful runtime.

Required env seed:

```env
FORWARD_RUNTIME_NODEX_MODE=true
FORWARD_RUNTIME_BACKEND=gost
FORWARD_RUNTIME_NODEX_BASE_URL=http://127.0.0.1:18080
FORWARD_RUNTIME_NODEX_TOKEN=replace-with-shared-token
FORWARD_RUNTIME_NODEX_TIMEOUT_SECONDS=15
```

Operational implications:
- ingress or entry-node semantics are valid only in this mode
- `/admin/system` should expose runtime status and doctor information
- `/admin/forward/nodes` refers to forward execution nodes, not proxy nodes

### `iptables_ansible` Mode

Use when you want stateless relay execution without NodeX ingress orchestration.

Required env seed:

```env
FORWARD_RUNTIME_NODEX_MODE=false
FORWARD_RUNTIME_BACKEND=iptables_ansible
FORWARD_RUNTIME_ANSIBLE_CONFIG_JSON={"inventory":"config/deploy/ansible/inventory.ini","playbookApply":"config/deploy/ansible/playbooks/forward_apply.yml","playbookRemove":"config/deploy/ansible/playbooks/forward_remove.yml","workingDir":"config/deploy/ansible","targetPattern":"{{node.host}}","timeoutSeconds":120,"environment":{"ANSIBLE_CONFIG":"config/deploy/ansible/ansible.cfg"}}
```

Operational implications:
- only the forward execution node matters
- no proxy-node ingress selector should be required
- SSH or inventory playbook material becomes the execution dependency

## Operator Entry Points

Use these together:

- startup and config: [`startup-config.md`](startup-config.md)
- config source-of-truth: [`configuration.md`](configuration.md)
- NodeX boundary: [`../guide/nodex-internal-extension.md`](../guide/nodex-internal-extension.md)
- runtime operations: [`../guide/forward-tunnel-runtime-ops.md`](../guide/forward-tunnel-runtime-ops.md)
- manual smoke tests: [`../guide/forward-tunnel-smoke-test.md`](../guide/forward-tunnel-smoke-test.md)

## Resource Naming Rule

Do not mix these resource types in docs or UI copy:

- `/admin/nodes`
  - proxy-node inventory
- `/admin/forward/nodes`
  - forward execution nodes
- `/admin/forward/tunnel`
  - tunnel inventory bound to forward execution behavior
