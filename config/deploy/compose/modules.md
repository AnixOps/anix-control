# Network modules with Compose

`docker-compose.modules.yml` is an overlay for `docker-compose.prod.yml`. It
runs `identity-platform` as its own container (a network module) instead of
a child process of Control. The design is in
[`docs/architecture/module-runtime.md`](../../../docs/architecture/module-runtime.md).

What the overlay changes:

- **Control and `migrate`** turn on the module runtime and mount the CA key
  `secrets/module_ca_kek`. Control then listens for modules on `:7443`
  (mTLS). The port is reachable only on the Compose network, not published on
  the host.
- **`identity`** runs the module image. It enrolls with a credential, stores
  its key and certificates in the `identity-state` volume, and binds to
  Control. Its container is read-only, runs as uid 65532 and has no
  capabilities.

## First start

Run these from the directory that holds `docker-compose.prod.yml`,
`control.env` and `secrets/`:

```bash
c="docker compose -f docker-compose.prod.yml -f docker-compose.modules.yml"

# 1. CA key: 32 random bytes that encrypt the module CA private key in the
#    database. Back it up with the other secrets; without it the CA cannot
#    issue certificates.
openssl rand -base64 32 | install -m 0400 -o 10001 -g 10001 /dev/stdin secrets/module_ca_kek

# 2. Schema and the module CA (created on the first run with the key).
$c run --rm -T migrate

# 3. Trust bundle for the modules: the CA certificates they accept Control
#    with.
$c run --rm -T --no-deps migrate module ca bundle \
  | install -m 0444 /dev/stdin secrets/module_ca_bundle

# 4. Enrollment credential. It is reusable, so a module that lost its volume
#    can enroll again; it is shown once, only its hash is stored, and
#    "module token revoke <id>" withdraws it.
$c run --rm -T --no-deps migrate module token create -package identity-platform -reusable -ttl 8760h \
  | jq -r .credential | install -m 0400 -o 65532 -g 65532 /dev/stdin secrets/identity_enrollment

# 4b. Identity signing key KEK: 32 random bytes that seal identity's token
#     signing keys in its storage. Every identity replica needs the same one;
#     back it up. Without it identity signs no tokens.
openssl rand -base64 32 | install -m 0400 -o 65532 -g 65532 /dev/stdin secrets/identity_kek

# 5. Run identity-platform remotely. The choice applies when Control next
#    starts the package.
$c run --rm -T --no-deps migrate module runtime set identity-platform remote

# 6. Start everything, with the module image pinned by digest.
export ANIX_MODULE_IDENTITY_IMAGE=ghcr.io/anixops/anix-module-identity-platform@sha256:<digest>
$c up -d
```

Check that it worked:

- Control's log says `serving from remote instances`.
- The identity log says `bound to generation`.
- `POST /api/v2/login` returns a token.

## Settings

| Variable | Default | Meaning |
|----------|---------|---------|
| `ANIX_MODULE_IDENTITY_IMAGE` | required | module image, pinned by digest |
| `ANIX_MODULE_CLUSTER` | `default` | SPIFFE cluster name in certificates; same for Control and modules |
| `ANIX_MODULE_DATABASE_HOST` | Control's database host | PostgreSQL address modules use for their storage leases, when it differs from Control's |

**Image versions.** The module's package version must match the version
installed in Control.
- `:edge` and `:sha-<commit>` images follow `go_dev`.
- Keep Control and the module image on the same commit.

## Operations

- **Restart or upgrade the module.** `$c up -d identity` after changing the
  image. The module reuses its stored certificate and binds again. Logins
  answer 503 until it has; tokens already issued keep working.
- **Back to the local runtime.** Stop the module, then switch Control back
  and restart it:

  ```bash
  $c stop identity
  $c run --rm -T --no-deps migrate module runtime set identity-platform local
  $c restart control
  ```

  The package then runs as a child process again.
- **Rotate the CA.**
  1. `$c run --rm -T --no-deps migrate module ca rotate` adds the next CA.
  2. Refresh `secrets/module_ca_bundle` as in step 3.
  3. Restart the module. It also picks up the bundle from Control when it
     renews its certificate.
- **Revoke a module.** `module token list` shows the credentials.
  `module token revoke <id>` withdraws one together with the certificates
  issued through it, and Control refuses those certificates at once.

## Test

`config/scripts/modules_compose_smoke.sh` runs this whole flow against an
empty PostgreSQL database and logs in through the module; CI runs it on every
change. See the script header for its environment.
