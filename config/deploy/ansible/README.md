# Ansible Runtime Assets

These files back the local `iptables_ansible` forward runtime extension.

## Canonical Config Entry

Put the runtime selection in `config/config.yaml`:

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

## How To Use

1. Copy `inventory.ini.example` to `inventory.ini`.
2. Add your relay hosts and SSH details.
3. Place SSH keys under `config/deploy/ssh/` when you use key-based auth.
4. Keep the runtime backend in `config/config.yaml` set to `iptables_ansible`.
5. If you use SSH password auth instead of a private key, encode the credentials in `config/deploy/ansible/inventory.ini` or `forward_runtime.iptables_ansible.extra_vars`.
6. For non-root SSH users, set `forward_runtime.iptables_ansible.become: true`, and keep sudo credentials in the inventory or other ansible-supported vars.

## Default Paths

Default paths used by the local binary, Docker, and the one-click installer:

- `inventory`: `config/deploy/ansible/inventory.ini`
- `playbookApply`: `config/deploy/ansible/playbooks/forward_apply.yml`
- `playbookRemove`: `config/deploy/ansible/playbooks/forward_remove.yml`
- `workingDir`: `config/deploy/ansible`
- `ANSIBLE_CONFIG`: `config/deploy/ansible/ansible.cfg`

## Notes

- The Flux-compatible `/admin/forward` page stays unchanged. Runtime controls remain in `System.vue`.
- The bundled playbooks already handle `tcp`, `udp`, `both`, and multi-target `round` or `rand` strategies.
