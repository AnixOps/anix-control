# Compose secrets

`docker-compose.prod.yml` reads three files from `./secrets/` (or
`ANIX_CONTROL_SECRETS_DIR`) and mounts them at `/run/secrets/`:

| File | Setting |
|------|---------|
| `jwt_secret` | `jwt.secret` (required in production; random, at least 32 bytes) |
| `db_password` | `database.password` |
| `module_ca_kek` | `module_runtime.ca_kek`: seals the built-in CA that signs Agent (and module) certificates; 32 random bytes, base64 |

The container runs as uid 10001, and Docker Compose bind-mounts secret files
with their host ownership and mode, so create them readable by that uid only.
`init-secrets.sh` (next to this file) creates `jwt_secret` and `module_ca_kek`
when they are missing and never replaces them; it prints only the key's
fingerprint (`sha256sum secrets/module_ca_kek | cut -c1-16`). The database
password is yours:

```bash
install -d -m 0750 secrets
printf '%s' 'database-password' | install -m 0400 -o 10001 -g 10001 /dev/stdin secrets/db_password
sudo bash init-secrets.sh
```

**`module_ca_kek` cannot be changed.** The CA in the database is sealed with
it: with another key Control cannot sign Agent certificates ("module CA …
key cannot be unsealed: wrong key-encryption key") and every Agent must
enroll again. If `control.env` already sets
`ANIX_CONTROL_MODULE_RUNTIME_CA_KEK`, `init-secrets.sh` stops: move that value
into `secrets/module_ca_kek` and delete the line (Control refuses to start
with both).

Keep the directory out of version control and back it up with `control.env`:
together they are everything needed to run the same deployment on another host.

## gRPC TLS for Agents

Agents enroll and connect over gRPC with TLS, and verify Control's certificate
against the node's system CA roots: they cannot be given a private CA, so the
certificate must be publicly trusted (Let's Encrypt, or your reverse proxy's
certificate) for the name they dial. With one,

```bash
sudo bash init-secrets.sh --grpc-name grpc.example.com \
  --grpc-tls-cert /etc/letsencrypt/live/grpc.example.com/fullchain.pem \
  --grpc-tls-key /etc/letsencrypt/live/grpc.example.com/privkey.pem
# .env: ANIX_CONTROL_GRPC_BIND=0.0.0.0, then
docker compose -f docker-compose.prod.yml up -d
```

checks the certificate the way an Agent does (refusing a self-signed one),
copies it to `tls/` (mounted at `/run/anix-control/tls`), and appends
`ANIX_CONTROL_GRPC_ENABLED=true`, the TLS file paths and
`ANIX_CONTROL_AGENT_INSTALL_GRPC_TARGET` to `control.env`. With only
`--grpc-name`, it uses `/etc/letsencrypt/live/<name>/`. Control loads the
certificate at start: after a renewal, run the same command again and
`docker compose -f docker-compose.prod.yml restart control` (a certbot
`--deploy-hook` can do both).
