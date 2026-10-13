### Added

- The root key: `data_dir/root.key` (`/var/lib/anix-control`, mode 0600 in
  a 0700 directory) holds 32 random bytes in a versioned text format. The
  server generates it at its first start, or adopts a configured
  `module_runtime.ca_kek` into it so that the CA, ACME keys, DNS credentials
  and join tokens stay readable. The JWT secret and the identity signing key
  KEK are derived from it with HKDF-SHA256 (`anixops jwt v1`, `anixops
  identity kek v1`); an explicitly set `jwt.secret` or `identity.kek` is
  recorded in `root.key` and kept until a rotation. Every start checks, after
  the migrations, that the root key opens the stored CA keys and refuses
  otherwise, naming what to restore; `anix-control doctor` reports the same
  check. Back up `root.key` with the database.
- `anix-control config check [--file]` loads a configuration strictly and
  prints its warnings, layout and root key state (exit 1 when Control would
  refuse it). `anix-control config rewrite --in <old> --out <new>` turns a
  4.x file into the operator keys (`docs/UPGRADE.md`, "Configuration Rewrite
  On Upgrade").
- The `-debug` flag runs a development instance (gin debug mode, debug
  logging); `make run` uses it with its database and root key in `.dev/`,
  the web app and API on `127.0.0.1:19080` and gRPC on `127.0.0.1:50052`.

### Changed

- The configuration is nine operator keys: `domain`, `tls.mode`,
  `tls.cert_file`, `tls.key_file`, `listen`, `grpc.port`, `grpc.public_name`,
  `subscribe_path` and `data_dir`. A configuration file overrides the built-in
  values key by key instead of replacing them, and then the `ANIX_CONTROL_*`
  variables. The file is read strictly: every 4.x key is in the deprecated
  list (`internal/config/deprecated_keys.yaml`, the table in
  `docs/reference/configuration.md`) and is logged once, either still read
  (its feature still exists: the database, the first administrator, the flux
  forward runtime, the legacy listener and gRPC keys, the registration
  policy, the legacy API tokens, `agent_install.*`) or ignored (the 4.2
  default stands); any other key stops the start, naming the closest key.
  `environment-variables.md` lists the nine variables.
- `config/config.yaml.example` is the one-line template `domain:
  panel.example.com`; `config/config.prod.yaml` and
  `config/config.dev.yaml.example` are gone. The systemd installer writes the
  template (its domain line kept a comment) and the first administrator, no
  longer `env`, `jwt.secret` or `app.api_token`, and its unit sets
  `ANIX_CONTROL_DATA_DIR=<install dir>/data`.
- Containers listen on one port (`ANIX_CONTROL_LISTEN=0.0.0.0:8080`; the web
  port 3000 is gone) and keep the root key in a `control-data` volume (Helm:
  an in-memory directory, the CA key Secret being the lasting copy; the chart
  refuses to render without it).

### Deprecated

- `env`, `server.mode`, the timeouts and CORS lists, `auth` rate limits,
  `alerts.*`, `agent_control.*`, `jwt.expire`, `forward.anixops_experimental`,
  `forward_runtime.latency.*`, the `plugins.control_host_*` limits and
  timeouts, `cache.*`, `app.name`, `app.version`, `app.traffic_log_enable`,
  `tls.domain` and `log.output`/`file_path`/`max_*` are ignored: their
  built-in values are the 4.2 defaults (registration rate limiting and the
  alert monitor always on, Agents always enrolled).
