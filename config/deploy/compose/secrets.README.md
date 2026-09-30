# Compose secrets

`docker-compose.prod.yml` reads two files from `./secrets/` (or
`ANIX_CONTROL_SECRETS_DIR`) and mounts them at `/run/secrets/`:

| File | Setting |
|------|---------|
| `jwt_secret` | `jwt.secret` (required in production; random, at least 32 bytes) |
| `db_password` | `database.password` |

The container runs as uid 10001, and Docker Compose bind-mounts secret files
with their host ownership and mode, so create them readable by that uid only:

```bash
install -d -m 0750 secrets
openssl rand -hex 32 | install -m 0400 -o 10001 -g 10001 /dev/stdin secrets/jwt_secret
printf '%s' 'database-password' | install -m 0400 -o 10001 -g 10001 /dev/stdin secrets/db_password
```

Keep the directory out of version control and back it up with `control.env`:
together they are everything needed to run the same deployment on another host.
