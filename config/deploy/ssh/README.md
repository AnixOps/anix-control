Place SSH private keys for the `iptables_ansible` runtime here.

Examples:

- `config/deploy/ssh/id_ed25519`
- `config/deploy/ssh/id_rsa`

The Docker container mounts this directory to `/home/v2board/.ssh` as read-only.
Keep private key permissions restricted on the host.
