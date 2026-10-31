### Removed

- The V2bX Ansible node deploy helper (`config/deploy/ansible/nodes/`:
  `deploy_v2bx.yml`, `deploy_from_inventory.sh`, `bootstrap_ssh_key.yml` and
  their templates). Nodes are onboarded and upgraded by the AnixOps Agent
  through Control; no third-party node program is deployed from this repository.

### Security

- `follow-redirects` 1.16.2 in `web/package-lock.json` (GHSA-mrwp-f3q8-3g9p); the
  Frontend Build audit gate failed on 1.16.1.
