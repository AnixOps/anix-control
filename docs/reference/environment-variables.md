# Environment Variables

Every scalar and list configuration key has an `ANIX_CONTROL_*` variable; the
name is `ANIX_CONTROL_` plus the upper-cased YAML path joined with
underscores. Containers normally use only these variables and no config file.
Rules (precedence, `_FILE` secrets, lists, map-valued keys) are in
[`configuration.md`](configuration.md#environment-variables).

- `NAME_FILE=/path` reads the value from a file (Docker and Kubernetes secrets).
- The defaults below are the built-in container defaults
  ([`internal/config/defaults.yaml`](../../internal/config/defaults.yaml)),
  used when no config file is loaded. With a config file, its values apply
  first and variables override them.
- The running image prints the same list: `docker run --rm <image> -print-env`.

Variables that are not configuration keys:

| Variable | Purpose |
|----------|---------|
| `ANIX_CONTROL_CONFIG` | config file to load instead of the built-in defaults |
| `NODE_DEFAULT_AUTH_KEY` | seeds a default node authorized key at startup |
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
| `ANIX_CONTROL_SUBSCRIBE_PATH` | `subscribe_path` | string | `s` |
| `ANIX_CONTROL_DATA_DIR` | `data_dir` | string | `/var/lib/anix-control` |
<!-- END GENERATED -->
