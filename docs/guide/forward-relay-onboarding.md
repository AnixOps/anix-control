# Forward Relay Onboarding

This guide answers one operational question:

How do you make a relay node really join the forwarding runtime after you create it in the panel?

## What "Really Joined" Means

A relay is really attached only when the selected runtime path can execute against it.

Creating a `ForwardNode` only means:

- the relay metadata is saved
- the panel can reference that relay later

It does not mean:

- NodeX can already control it
- gost API is healthy
- ansible can SSH into it
- forwarding rules already exist on that host

## Path A: NodeX/Gost

Use this when you want the private stateful runtime.

### Required pieces

1. `v2board` with:

```yaml
forward_runtime:
  backend: "gost"
  nodex:
    base_url: "http://127.0.0.1:18081"
    token: "replace-with-shared-token"
    timeout_seconds: 15
```

2. NodeX control-plane running on the configured `base_url`
3. relay host running gost with management API enabled
   - current verified version: `3.2.6`
   - older `3.0.0-rc10` was not compatible with the current dynamic config flow
4. relay `ForwardNode` filled with:
   - `host`
   - `api_port`
   - `api_token`
5. one tunnel with a valid ingress node
6. one forward bound to that tunnel

### Execution chain

1. panel creates or updates forward state
2. `v2board` calls NodeX
3. NodeX calls relay gost API
4. relay gost stores services and limiters

### Acceptance checks

- NodeX health works
- NodeX runtime status works
- `v2_forward_runtime_job.backend='gost'` shows `success`
- relay gost API returns the expected service

### Verified single-UI example

The current real-machine validation baseline used:

- temporary Debian relay/control host: `143.20.204.14`
- panel UI: `http://143.20.204.14:3000`
- panel API: `http://143.20.204.14:8080`
- NodeX control-plane: `http://143.20.204.14:18081`
- relay gost API: `http://143.20.204.14:18080`

Important port split:

- `18081` is the outer NodeX control-plane address that `v2board` must call
- `18080` is the relay gost management API that NodeX calls after it accepts the request

This verified example forwarded the same target service through two runtime paths:

- target TCP service: `155.117.224.30:11111`
- `143.20.204.14:11111` via `iptables_ansible`
- `143.20.204.14:11112` via `gost`

That proof is recorded in the smoke guide and the NodeX internal runbook. The public-panel side takeaway is simple:

- a relay is not "joined" because the panel saved a `ForwardNode`
- it is joined only after the selected backend leaves relay-side evidence

## Path B: `iptables_ansible`

Use this when you want stateless relay execution without NodeX.

### Required pieces

1. `v2board` with:

```yaml
forward_runtime:
  backend: "iptables_ansible"
  iptables_ansible:
    inventory: "config/deploy/ansible/inventory.ini"
    apply_playbook: "config/deploy/ansible/playbooks/forward_apply.yml"
    remove_playbook: "config/deploy/ansible/playbooks/forward_remove.yml"
    working_dir: "config/deploy/ansible"
    target_pattern: "{{node.host}}"
    environment:
      ANSIBLE_CONFIG: "config/deploy/ansible/ansible.cfg"
    timeout_seconds: 120
```

2. `ansible-playbook` available on the machine running `v2board`
3. an ansible inventory file, referenced by `config/config.yaml.forward_runtime.iptables_ansible.inventory`, that can SSH into the relay host
4. a supported tunnel and forward
5. a relay `ForwardNode`

### Important current limitation

Current `ForwardNode` does not store SSH credentials.

That means ansible attachment depends on:

- inventory files
- ansible inventory files referenced from `config/config.yaml.forward_runtime`
- SSH and sudo behavior on the executor host

not on extra fields inside `v2_forward_node`.

### Execution chain

1. panel creates or updates forward state
2. `v2board` stores a pending runtime job
3. local executor runs `ansible-playbook`
4. ansible reaches the relay host over SSH
5. relay host receives `iptables` rules

### Acceptance checks

- `v2_forward_runtime_job.backend='iptables_ansible'` shows `success`
- relay host contains the expected `iptables` rules

## Current UI Caveat

The panel forward-node online flag is not an attachment check.

It is only a TCP reachability check to `host:port`.

A green node can still fail because:

- NodeX base URL is wrong
- NodeX token is wrong
- relay gost API is not listening on `api_port`
- relay API token is wrong
- ansible inventory is wrong
- SSH login or sudo fails

## Real attachment checklist

Treat a relay as really attached only when all of these line up:

1. control plane evidence
   - `/api/v2/admin/forward/runtime/status` is reachable
   - `/api/v2/admin/forward/runtime/doctor` reports the expected backend
2. job evidence
   - `v2_forward_runtime_job` shows the latest action as `success`
3. relay evidence
   - `gost` path: `/api/config/services` contains the service
   - `iptables_ansible` path: `iptables-save` contains the NAT rules
4. network evidence
   - the forwarded port behaves the same way as the source target service

## Fast Command Checklist

NodeX side:

```powershell
Invoke-WebRequest http://127.0.0.1:18081/health
Invoke-WebRequest http://127.0.0.1:18081/api/v2/internal/forward/runtime/status -Headers @{ Authorization = 'Bearer <FORWARD_API_TOKEN>' }
```

Relay gost side:

```bash
curl -u admin:<RELAY_API_TOKEN> http://<RELAY_HOST>:<API_PORT>/api/config/services
```

Ansible side:

```bash
ansible all -i config/deploy/ansible/inventory.ini -m ping
ansible-playbook -i config/deploy/ansible/inventory.ini config/deploy/ansible/playbooks/forward_apply.yml
```

Database side:

```sql
SELECT id, backend, action, status, node_id, result, error, created_at
FROM v2_forward_runtime_job
ORDER BY id DESC
LIMIT 20;
```
