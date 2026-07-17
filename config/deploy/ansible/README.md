# Ansible Runtime Assets

These files back the local stateless Ansible runtime path.

Recommended backend:

- `nftables_ansible`

Legacy compatibility:

- `iptables_ansible`

## Canonical Config Entry

Put the runtime selection in `config/config.yaml`:

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

## How To Use

1. Copy `inventory.ini.example` to `inventory.ini`.
2. Add your relay hosts and SSH details.
3. Place SSH keys under `config/deploy/ssh/` when you use key-based auth.
4. Keep the runtime backend in `config/config.yaml` set to `nftables_ansible` unless you intentionally need the legacy iptables path.
5. If you use SSH password auth instead of a private key, encode the credentials in `config/deploy/ansible/inventory.ini` or the selected ansible block's `extra_vars`.
6. For non-root SSH users, set `forward_runtime.nftables_ansible.become: true`, and keep sudo credentials in the inventory or other ansible-supported vars. If you still use the legacy backend, apply the same setting under `forward_runtime.iptables_ansible.become`.

## Default Paths

Default paths used by the local binary, Docker, and the one-click installer:

- `inventory`: `config/deploy/ansible/inventory.ini`
- `playbookApply`: `config/deploy/ansible/playbooks/forward_apply_nftables.yml`
- `playbookRemove`: `config/deploy/ansible/playbooks/forward_remove_nftables.yml`
- `workingDir`: `config/deploy/ansible`
- `ANSIBLE_CONFIG`: `config/deploy/ansible/ansible.cfg`

Legacy iptables playbooks remain available:

- `config/deploy/ansible/playbooks/forward_apply.yml`
- `config/deploy/ansible/playbooks/forward_remove.yml`

## Notes

- The Flux-compatible `/admin/forward` page stays unchanged. Runtime controls now live on the dedicated `Local Runtime`, `Ansible Machines`, and `NodeX Runtime` pages.
- The bundled playbooks already handle `tcp`, `udp`, `both`, and multi-target `round` or `rand` strategies.
