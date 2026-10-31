# Ansible Runtime Assets

These files back the legacy local Ansible forward path (`iptables_ansible`).
The nftables playbooks and the nftables forward engine were removed: relays
run on the network model (ANX) now.

## How To Use

1. Copy `inventory.ini.example` to `inventory.ini`.
2. Add your relay hosts and SSH details.
3. Place SSH keys under `config/deploy/ssh/` when you use key-based auth.
4. If you use SSH password auth instead of a private key, encode the credentials in `config/deploy/ansible/inventory.ini` or the selected ansible block's `extra_vars`.
5. For non-root SSH users, set `forward_runtime.iptables_ansible.become: true`, and keep sudo credentials in the inventory or other ansible-supported vars.

## Default Paths

- `inventory`: `config/deploy/ansible/inventory.ini`
- `workingDir`: `config/deploy/ansible`
- `ANSIBLE_CONFIG`: `config/deploy/ansible/ansible.cfg`

The iptables playbooks:

- `config/deploy/ansible/playbooks/forward_apply.yml`
- `config/deploy/ansible/playbooks/forward_remove.yml`

`forward_legacy_cleanup.yml` removes the old forward runtimes (including the
old nftables tables) from a host.

## Notes

- The bundled playbooks handle `tcp`, `udp`, `both`, IPv4 and IPv6 targets, and multi-target `round` or `rand` strategies. `fifo` and `hash` use only the first target of each address family, and speed limits are not enforced on this path.
