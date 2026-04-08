# Forward/Tunnel Manual Smoke Tests

This guide validates the two runtime paths separately.

Keep the resource split explicit in every step:

- `proxy node`
  - `/admin/nodes`
  - serves user proxy traffic
- `forward node`
  - `/admin/forward/nodes`
  - serves runtime execution targeting

## Before You Start

Do not treat these as proof of attachment:

- `ForwardNode` record exists
- `ForwardNode` shows online in the panel
- tunnel and forward records were saved successfully

Real attachment must be proven by runtime job success plus relay-side state.

## 1. NodeX/Gost Smoke

### Preconditions

1. NodeX control-plane is running.
2. `config/config.yaml` has `forward_runtime.backend=gost`.
3. `forward_runtime.nodex.base_url` and `forward_runtime.nodex.token` are set and match NodeX.
4. The relay host is already running gost with a management API.
5. The relay `ForwardNode` has the correct `host`, `api_port`, and `api_token`.

### Checks

1. Confirm `v2board` runtime config:
   - open `/admin/system`
   - confirm NodeX mode values
2. Confirm NodeX health:

```powershell
Invoke-WebRequest http://127.0.0.1:18081/health | Select-Object -ExpandProperty Content
```

3. Confirm NodeX runtime status:

```powershell
Invoke-WebRequest http://127.0.0.1:18081/api/v2/internal/forward/runtime/status `
  -Headers @{ Authorization = 'Bearer <FORWARD_API_TOKEN>' } |
  Select-Object -ExpandProperty Content
```

4. Create or update one tunnel and one forward in the panel.
5. Inspect the latest gost runtime jobs:

```sql
SELECT id, backend, action, status, node_id, result, error, created_at
FROM v2_forward_runtime_job
WHERE backend = 'gost'
ORDER BY id DESC
LIMIT 10;
```

6. Inspect the relay gost API directly:

```bash
curl -u admin:<RELAY_API_TOKEN> http://<RELAY_HOST>:<API_PORT>/api/config/services
curl -u admin:<RELAY_API_TOKEN> http://<RELAY_HOST>:<API_PORT>/api/config/limiters
```

### Pass criteria

- runtime job status is `success`
- relay gost contains the expected service
- relay gost contains the expected limiter when rate limits are enabled

## 2. `iptables_ansible` Smoke

### Preconditions

1. `config/config.yaml` has `forward_runtime.backend=iptables_ansible`.
2. `ansible-playbook` exists on the machine running `v2board`.
3. inventory and playbooks exist.
4. the configured ansible inventory can SSH into the relay host.
5. the selected tunnel supports ansible execution.

### Checks

1. Confirm the command exists:

```powershell
Get-Command ansible-playbook
```

2. Confirm the inventory path and playbooks exist.
3. Confirm `config/config.yaml.forward_runtime.iptables_ansible.inventory` points at the executor inventory file used by the panel host.
4. Run a manual ansible reachability check on the same executor host when possible:

```bash
ansible all -i config/deploy/ansible/inventory.ini -m ping
```

5. Create or update one tunnel and one forward in the panel.
6. Inspect the latest ansible runtime jobs:

```sql
SELECT id, backend, action, status, node_id, result, error, created_at
FROM v2_forward_runtime_job
WHERE backend = 'iptables_ansible'
ORDER BY id DESC
LIMIT 10;
```

7. Inspect relay-side `iptables` state:

```bash
sudo iptables -t nat -S
sudo iptables-save
```

### Pass criteria

- runtime job status is `success`
- relay host contains the expected `iptables` rules
- removing or pausing the forward removes the expected rules

## 3. Real-Machine Verified Example (2026-04-07)

The current authoritative proof was validated on a temporary Debian host with:

- `v2board` UI on `143.20.204.14:3000`
- `v2board` API on `143.20.204.14:8080`
- NodeX control-plane on `143.20.204.14:18081`
- relay gost API on `143.20.204.14:18080`

Verified forwarding case:

- source target: `155.117.224.30:11111`
- `143.20.204.14:11111` via `iptables_ansible`
- `143.20.204.14:11112` via `gost`

### TCP evidence

Observed results:

```text
155.117.224.30:11111 CONNECT_OK 0.276s RECV b''
143.20.204.14:11111 CONNECT_OK 0.197s RECV b''
143.20.204.14:11112 CONNECT_OK 0.196s RECV b''
```

Interpretation:

- the target service was a TCP service that accepted connections but returned no HTTP body
- both forwarded ports matched the same behavior
- that is valid proof for a non-HTTP TCP forwarding case

### Panel evidence

Observed forward records:

```text
id=4 name=Forward-Ansible-11111 runtimeBackend=iptables_ansible runtimeStatus=2 runtimeMessage="ansible runtime synchronized"
id=5 name=Forward-Gost-11112 runtimeBackend=gost runtimeStatus=2 runtimeMessage="gost runtime synchronized"
```

Observed runtime jobs:

```text
id=11 backend=iptables_ansible action=create status=2
id=12 backend=gost action=create status=2
```

### Relay evidence

Observed `iptables-save` lines:

```text
-A PREROUTING -p tcp -m tcp --dport 11111 -j V2B_FWD_4_TCP
-A POSTROUTING -d 155.117.224.30/32 -p tcp -m tcp --dport 11111 -j MASQUERADE
-A V2B_FWD_4_TCP -p tcp -j DNAT --to-destination 155.117.224.30:11111
```

Observed gost relay config:

```json
{
  "name": "panel-forward-5",
  "addr": "[::]:11112",
  "forwarder": {
    "nodes": [
      { "addr": "155.117.224.30:11111" }
    ]
  }
}
```

### Deployment conclusion

This is the currently validated path:

- `binary + SQLite + systemd`
- `iptables_ansible` verified on real relay rules
- `NodeX/gost` verified on real relay dynamic config

The Dockerized forward-runtime runbook is still a follow-up task. Do not treat container deployment as the already-proven path until that runbook and evidence are added.

## 4. Misleading Green Lights

The current `ForwardNode` online state is only a TCP dial to `host:port`.

It does not prove:

- gost API health on `api_port`
- relay `api_token` correctness
- SSH login correctness
- ansible privilege escalation correctness
- actual runtime services or rules already exist

## 5. Fast Failure Hints

- `401` on `/api/v2/admin/*`
  - admin auth problem, not relay runtime problem
- `backend='gost'` job fails before relay changes
  - check NodeX base URL, token, relay gost API reachability, relay API token
- `backend='iptables_ansible'` job stays pending
  - check local background executor and command availability
- `backend='iptables_ansible'` job fails during execution
  - check inventory, SSH, and sudo/become behavior

## 6. Related Docs

- runtime overview: [`../reference/runtime.md`](../reference/runtime.md)
- onboarding guide: [`forward-relay-onboarding.md`](forward-relay-onboarding.md)
- runtime operations: [`forward-tunnel-runtime-ops.md`](forward-tunnel-runtime-ops.md)
