# Environment Variables

Every operator key has an `ANIX_CONTROL_*` variable; the name is
`ANIX_CONTROL_` plus the upper-cased YAML path joined with underscores. The
rules (precedence, `_FILE`, lists) are in
[`configuration.md`](configuration.md#environment-variables).

- Precedence: the built-in values, then the configuration file key by key,
  then these variables.
- `NAME_FILE=/path` reads the value from a file (Docker and Kubernetes secrets).
- The defaults below are the built-in values
  ([`internal/config/defaults.yaml`](../../internal/config/defaults.yaml)).
- The variables of the deprecated keys (the 4.2 configuration) still work as
  their keys do, still read or ignored with a warning; the
  [deprecated key table](configuration.md#deprecated-keys) lists each with
  its variable.
- `anix-control -print-env` prints the same list.

Variables that are not configuration keys:

| Variable | Purpose |
|----------|---------|
| `ANIX_CONTROL_CONFIG` | config file to load instead of the built-in defaults |
| `TZ` | process time zone; traffic resets follow it (image default `Asia/Shanghai`) |
| `ANIX_CONTROL_ACME_DIRECTORY_URL` | another ACME directory than the public CA's, for tests and a staging validation ([testing ACME](configuration.md#testing-acme)) |
| `ANIX_CONTROL_ACME_CA_FILE` | extra PEM roots for that directory's own https (a test CA) |

This table is generated from the code and checked by
`TestEnvironmentVariableReferenceIsCurrent`.

<!-- BEGIN GENERATED: anix-control -print-env -->
| Variable | Key | Type | Built-in default |
|----------|-----|------|------------------|
| `ANIX_CONTROL_DOMAIN` | `domain` | string | none; required in `tls.mode` `acme` |
| `ANIX_CONTROL_TLS_MODE` | `tls.mode` | string | empty: `acme` with `domain`, else `external` |
| `ANIX_CONTROL_TLS_CERT_FILE` | `tls.cert_file` | string |  |
| `ANIX_CONTROL_TLS_KEY_FILE` | `tls.key_file` | string |  |
| `ANIX_CONTROL_LISTEN` | `listen` | string | empty: `127.0.0.1:8080` |
| `ANIX_CONTROL_GRPC_PORT` | `grpc.port` | int | `50051` |
| `ANIX_CONTROL_GRPC_PUBLIC_NAME` | `grpc.public_name` | string | empty: `domain` |
| `ANIX_CONTROL_GRPC_EDGE_PROXIES` | `grpc.edge_proxies` | list |  |
| `ANIX_CONTROL_SUBSCRIBE_PATH` | `subscribe_path` | string | `s` |
| `ANIX_CONTROL_DATA_DIR` | `data_dir` | string | `/var/lib/anix-control` |
<!-- END GENERATED -->
