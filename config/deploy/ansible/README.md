# Ansible Runtime Assets

These files back the local `iptables_ansible` forward runtime extension.

Default paths used by Docker and the one-click installer:

- `inventory`: `/app/config/deploy/ansible/inventory.ini`
- `playbookApply`: `/app/config/deploy/ansible/playbooks/forward_apply.yml`
- `playbookRemove`: `/app/config/deploy/ansible/playbooks/forward_remove.yml`
- `workingDir`: `/app/config/deploy/ansible`
- `ANSIBLE_CONFIG`: `/app/config/deploy/ansible/ansible.cfg`

How to use:

1. Copy `inventory.ini.example` to `inventory.ini`.
2. Add your ingress/relay hosts and SSH details.
3. Place SSH keys under `config/deploy/ssh/` when you use key-based auth.
4. Set `FORWARD_RUNTIME_BACKEND=iptables_ansible` in `.env`, or switch it in `System.vue`.
5. If you use SSH password auth instead of a private key, set `FORWARD_RUNTIME_ANSIBLE_HOST`, `FORWARD_RUNTIME_ANSIBLE_USER`, and `FORWARD_RUNTIME_ANSIBLE_PASSWORD` in `.env`.
6. For non-root SSH users, also set `FORWARD_RUNTIME_ANSIBLE_BECOME=true` and `FORWARD_RUNTIME_ANSIBLE_BECOME_PASSWORD`.

Notes:

- The Flux-compatible `/admin/forward` page stays unchanged. Runtime controls remain in `System.vue`.
- The bundled playbooks already handle `tcp`, `udp`, `both`, and multi-target `round` / `rand` strategies.
