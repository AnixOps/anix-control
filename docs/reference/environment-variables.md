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

This table is generated from the code and checked by
`TestEnvironmentVariableReferenceIsCurrent`.

<!-- BEGIN GENERATED: anix-control -print-env -->
| Variable | Key | Type | Built-in default |
|----------|-----|------|------------------|
| `ANIX_CONTROL_ADMIN_EMAIL` | `admin.email` | string |  |
| `ANIX_CONTROL_ADMIN_PASSWORD` | `admin.password` | string | secret, no default |
| `ANIX_CONTROL_AGENT_CONTROL_MTLS` | `agent_control.mtls` | string | `optional` |
| `ANIX_CONTROL_APP_API_TOKEN` | `app.api_token` | string | secret, no default |
| `ANIX_CONTROL_APP_NAME` | `app.name` | string | `AnixOps Control` |
| `ANIX_CONTROL_APP_SUBSCRIBE_PATH` | `app.subscribe_path` | string | `s` |
| `ANIX_CONTROL_APP_TRAFFIC_LOG_ENABLE` | `app.traffic_log_enable` | bool | `true` |
| `ANIX_CONTROL_APP_VERSION` | `app.version` | string |  |
| `ANIX_CONTROL_AUTH_LOGIN_RATE_LIMIT_ENABLED` | `auth.login_rate_limit.enabled` | bool | `true` |
| `ANIX_CONTROL_AUTH_LOGIN_RATE_LIMIT_LOCKOUT_SECONDS` | `auth.login_rate_limit.lockout_seconds` | int | `600` |
| `ANIX_CONTROL_AUTH_LOGIN_RATE_LIMIT_MAX_ATTEMPTS` | `auth.login_rate_limit.max_attempts` | int | `6` |
| `ANIX_CONTROL_AUTH_LOGIN_RATE_LIMIT_WINDOW_SECONDS` | `auth.login_rate_limit.window_seconds` | int | `300` |
| `ANIX_CONTROL_AUTH_REGISTER_RATE_LIMIT_ENABLED` | `auth.register_rate_limit.enabled` | bool | `true` |
| `ANIX_CONTROL_AUTH_REGISTER_RATE_LIMIT_LOCKOUT_SECONDS` | `auth.register_rate_limit.lockout_seconds` | int | `3600` |
| `ANIX_CONTROL_AUTH_REGISTER_RATE_LIMIT_MAX_ATTEMPTS` | `auth.register_rate_limit.max_attempts` | int | `5` |
| `ANIX_CONTROL_AUTH_REGISTER_RATE_LIMIT_WINDOW_SECONDS` | `auth.register_rate_limit.window_seconds` | int | `3600` |
| `ANIX_CONTROL_AUTH_REGISTRATION_ALLOWED_EMAIL_DOMAINS` | `auth.registration.allowed_email_domains` | list |  |
| `ANIX_CONTROL_AUTH_REGISTRATION_BLOCKED_EMAIL_DOMAINS` | `auth.registration.blocked_email_domains` | list |  |
| `ANIX_CONTROL_AUTH_REGISTRATION_ENABLED` | `auth.registration.enabled` | bool | `true` |
| `ANIX_CONTROL_AUTH_REGISTRATION_REQUIRE_INVITE` | `auth.registration.require_invite` | bool | `false` |
| `ANIX_CONTROL_CACHE_DRIVER` | `cache.driver` | string |  |
| `ANIX_CONTROL_CACHE_REDIS_DB` | `cache.redis_db` | int | `0` |
| `ANIX_CONTROL_CACHE_REDIS_HOST` | `cache.redis_host` | string |  |
| `ANIX_CONTROL_CACHE_REDIS_PASSWORD` | `cache.redis_password` | string | secret, no default |
| `ANIX_CONTROL_CACHE_REDIS_PORT` | `cache.redis_port` | int | `0` |
| `ANIX_CONTROL_DATABASE_CONN_MAX_LIFETIME` | `database.conn_max_lifetime` | int | `3600` |
| `ANIX_CONTROL_DATABASE_DATABASE` | `database.database` | string | `anix_control` |
| `ANIX_CONTROL_DATABASE_DRIVER` | `database.driver` | string | `postgres` |
| `ANIX_CONTROL_DATABASE_DSN` | `database.dsn` | string | secret, no default |
| `ANIX_CONTROL_DATABASE_HOST` | `database.host` | string | `127.0.0.1` |
| `ANIX_CONTROL_DATABASE_LOG_LEVEL` | `database.log_level` | string | `error` |
| `ANIX_CONTROL_DATABASE_MAX_IDLE_CONNS` | `database.max_idle_conns` | int | `10` |
| `ANIX_CONTROL_DATABASE_MAX_OPEN_CONNS` | `database.max_open_conns` | int | `50` |
| `ANIX_CONTROL_DATABASE_PASSWORD` | `database.password` | string | secret, no default |
| `ANIX_CONTROL_DATABASE_PORT` | `database.port` | int | `5432` |
| `ANIX_CONTROL_DATABASE_SSLMODE` | `database.sslmode` | string | `disable` |
| `ANIX_CONTROL_DATABASE_TIMEZONE` | `database.timezone` | string | `Asia/Shanghai` |
| `ANIX_CONTROL_DATABASE_USERNAME` | `database.username` | string | `anix_control` |
| `ANIX_CONTROL_ENV` | `env` | string | `production` |
| `ANIX_CONTROL_FORWARD_RUNTIME_BACKEND` | `forward_runtime.backend` | string |  |
| `ANIX_CONTROL_FORWARD_RUNTIME_CLEAN_AGENT_ACTION_TIMEOUT_SECONDS` | `forward_runtime.clean_agent.action_timeout_seconds` | int | `0` |
| `ANIX_CONTROL_FORWARD_RUNTIME_CLEAN_AGENT_HEARTBEAT_INTERVAL_SECONDS` | `forward_runtime.clean_agent.heartbeat_interval_seconds` | int | `0` |
| `ANIX_CONTROL_FORWARD_RUNTIME_CLEAN_AGENT_LEGACY_BRIDGE_ENABLED` | `forward_runtime.clean_agent.legacy_bridge_enabled` | bool | `false` |
| `ANIX_CONTROL_FORWARD_RUNTIME_CLEAN_AGENT_PUBLIC_URL` | `forward_runtime.clean_agent.public_url` | string |  |
| `ANIX_CONTROL_FORWARD_RUNTIME_CLEAN_AGENT_TOKEN_EXPIRE_SECONDS` | `forward_runtime.clean_agent.token_expire_seconds` | int | `0` |
| `ANIX_CONTROL_FORWARD_RUNTIME_GOST_STATS_ERROR_LOG_INTERVAL` | `forward_runtime.gost_stats.error_log_interval` | string | `5m` |
| `ANIX_CONTROL_FORWARD_RUNTIME_GOST_STATS_IDLE_POLL_INTERVAL` | `forward_runtime.gost_stats.idle_poll_interval` | string | `2m` |
| `ANIX_CONTROL_FORWARD_RUNTIME_GOST_STATS_POLL_INTERVAL` | `forward_runtime.gost_stats.poll_interval` | string | `30s` |
| `ANIX_CONTROL_FORWARD_RUNTIME_JOBS_BATCH_SIZE` | `forward_runtime.jobs.batch_size` | int | `10` |
| `ANIX_CONTROL_FORWARD_RUNTIME_JOBS_ERROR_LOG_INTERVAL` | `forward_runtime.jobs.error_log_interval` | string | `1m` |
| `ANIX_CONTROL_FORWARD_RUNTIME_JOBS_IDLE_POLL_INTERVAL` | `forward_runtime.jobs.idle_poll_interval` | string | `30s` |
| `ANIX_CONTROL_FORWARD_RUNTIME_JOBS_POLL_INTERVAL` | `forward_runtime.jobs.poll_interval` | string | `5s` |
| `ANIX_CONTROL_FORWARD_RUNTIME_JOBS_TIMEOUT_SECONDS` | `forward_runtime.jobs.timeout_seconds` | int | `120` |
| `ANIX_CONTROL_FORWARD_RUNTIME_LATENCY_CONCURRENCY` | `forward_runtime.latency.concurrency` | int | `16` |
| `ANIX_CONTROL_FORWARD_RUNTIME_LATENCY_DIALS` | `forward_runtime.latency.dials` | int | `5` |
| `ANIX_CONTROL_FORWARD_RUNTIME_LATENCY_DIAL_TIMEOUT` | `forward_runtime.latency.dial_timeout` | string | `3s` |
| `ANIX_CONTROL_FORWARD_RUNTIME_LATENCY_ERROR_LOG_INTERVAL` | `forward_runtime.latency.error_log_interval` | string | `5m` |
| `ANIX_CONTROL_FORWARD_RUNTIME_LATENCY_IDLE_POLL_INTERVAL` | `forward_runtime.latency.idle_poll_interval` | string | `60s` |
| `ANIX_CONTROL_FORWARD_RUNTIME_LATENCY_POLL_INTERVAL` | `forward_runtime.latency.poll_interval` | string | `60s` |
| `ANIX_CONTROL_FORWARD_RUNTIME_LATENCY_RETENTION_DAYS` | `forward_runtime.latency.retention_days` | int | `7` |
| `ANIX_CONTROL_FORWARD_RUNTIME_NFTABLES_ANSIBLE_APPLY_PLAYBOOK` | `forward_runtime.nftables_ansible.apply_playbook` | string |  |
| `ANIX_CONTROL_FORWARD_RUNTIME_NFTABLES_ANSIBLE_BECOME` | `forward_runtime.nftables_ansible.become` | bool | `false` |
| `ANIX_CONTROL_FORWARD_RUNTIME_NFTABLES_ANSIBLE_COMMAND` | `forward_runtime.nftables_ansible.command` | string |  |
| `ANIX_CONTROL_FORWARD_RUNTIME_NFTABLES_ANSIBLE_INVENTORY` | `forward_runtime.nftables_ansible.inventory` | string |  |
| `ANIX_CONTROL_FORWARD_RUNTIME_NFTABLES_ANSIBLE_REMOVE_PLAYBOOK` | `forward_runtime.nftables_ansible.remove_playbook` | string |  |
| `ANIX_CONTROL_FORWARD_RUNTIME_NFTABLES_ANSIBLE_TARGET_PATTERN` | `forward_runtime.nftables_ansible.target_pattern` | string |  |
| `ANIX_CONTROL_FORWARD_RUNTIME_NFTABLES_ANSIBLE_TIMEOUT_SECONDS` | `forward_runtime.nftables_ansible.timeout_seconds` | int | `0` |
| `ANIX_CONTROL_FORWARD_RUNTIME_NFTABLES_ANSIBLE_WORKING_DIR` | `forward_runtime.nftables_ansible.working_dir` | string |  |
| `ANIX_CONTROL_FORWARD_RUNTIME_NODEX_BASE_URL` | `forward_runtime.nodex.base_url` | string |  |
| `ANIX_CONTROL_FORWARD_RUNTIME_NODEX_MODE` | `forward_runtime.nodex_mode` | bool |  |
| `ANIX_CONTROL_FORWARD_RUNTIME_NODEX_TIMEOUT_SECONDS` | `forward_runtime.nodex.timeout_seconds` | int | `0` |
| `ANIX_CONTROL_FORWARD_RUNTIME_NODEX_TOKEN` | `forward_runtime.nodex.token` | string | secret, no default |
| `ANIX_CONTROL_FRONTEND_ENABLE` | `frontend.enable` | bool | `true` |
| `ANIX_CONTROL_FRONTEND_PATH` | `frontend.path` | string | `web/public` |
| `ANIX_CONTROL_FRONTEND_PORT` | `frontend.port` | int | `3000` |
| `ANIX_CONTROL_GRPC_API_TOKEN` | `grpc.api_token` | string | secret, no default |
| `ANIX_CONTROL_GRPC_ENABLED` | `grpc.enabled` | bool | `false` |
| `ANIX_CONTROL_GRPC_HOST` | `grpc.host` | string | `0.0.0.0` |
| `ANIX_CONTROL_GRPC_PORT` | `grpc.port` | int | `50051` |
| `ANIX_CONTROL_GRPC_TLS_CERT_FILE` | `grpc.tls_cert_file` | string |  |
| `ANIX_CONTROL_GRPC_TLS_KEY_FILE` | `grpc.tls_key_file` | string |  |
| `ANIX_CONTROL_IDENTITY_KEK` | `identity.kek` | string | secret, no default |
| `ANIX_CONTROL_JWT_EXPIRE` | `jwt.expire` | int | `86400` |
| `ANIX_CONTROL_JWT_SECRET` | `jwt.secret` | string | secret, no default |
| `ANIX_CONTROL_LOG_FILE_PATH` | `log.file_path` | string |  |
| `ANIX_CONTROL_LOG_FORMAT` | `log.format` | string | `text` |
| `ANIX_CONTROL_LOG_LEVEL` | `log.level` | string | `info` |
| `ANIX_CONTROL_LOG_MAX_AGE` | `log.max_age` | int | `0` |
| `ANIX_CONTROL_LOG_MAX_BACKUPS` | `log.max_backups` | int | `0` |
| `ANIX_CONTROL_LOG_MAX_SIZE` | `log.max_size` | int | `0` |
| `ANIX_CONTROL_LOG_OUTPUT` | `log.output` | string |  |
| `ANIX_CONTROL_MODULE_RUNTIME_BIND_TIMEOUT` | `module_runtime.bind_timeout` | string | `2m` |
| `ANIX_CONTROL_MODULE_RUNTIME_CA_KEK` | `module_runtime.ca_kek` | string | secret, no default |
| `ANIX_CONTROL_MODULE_RUNTIME_CERT_FILE` | `module_runtime.cert_file` | string |  |
| `ANIX_CONTROL_MODULE_RUNTIME_CERT_LIFETIME` | `module_runtime.cert_lifetime` | string | `24h` |
| `ANIX_CONTROL_MODULE_RUNTIME_CLUSTER` | `module_runtime.cluster` | string | `default` |
| `ANIX_CONTROL_MODULE_RUNTIME_DATABASE_HOST` | `module_runtime.database_host` | string |  |
| `ANIX_CONTROL_MODULE_RUNTIME_ENABLED` | `module_runtime.enabled` | bool | `false` |
| `ANIX_CONTROL_MODULE_RUNTIME_KEY_FILE` | `module_runtime.key_file` | string |  |
| `ANIX_CONTROL_MODULE_RUNTIME_LISTEN` | `module_runtime.listen` | string | `:7443` |
| `ANIX_CONTROL_MODULE_RUNTIME_PKI` | `module_runtime.pki` | string | `builtin` |
| `ANIX_CONTROL_MODULE_RUNTIME_TRUST_BUNDLE_FILE` | `module_runtime.trust_bundle_file` | string |  |
| `ANIX_CONTROL_PLUGINS_CONTROL_EXECUTION_ENABLED` | `plugins.control_execution_enabled` | bool | `true` |
| `ANIX_CONTROL_PLUGINS_CONTROL_HOST_ARTIFACT_DIR` | `plugins.control_host_artifact_dir` | string |  |
| `ANIX_CONTROL_PLUGINS_CONTROL_HOST_MAX_REQUEST_BYTES` | `plugins.control_host_max_request_bytes` | int | `0` |
| `ANIX_CONTROL_PLUGINS_CONTROL_HOST_MAX_RESPONSE_BYTES` | `plugins.control_host_max_response_bytes` | int | `0` |
| `ANIX_CONTROL_PLUGINS_CONTROL_HOST_REQUEST_TIMEOUT` | `plugins.control_host_request_timeout` | string | `30s` |
| `ANIX_CONTROL_PLUGINS_CONTROL_HOST_RUNTIME_DIR` | `plugins.control_host_runtime_dir` | string |  |
| `ANIX_CONTROL_PLUGINS_CONTROL_HOST_STARTUP_TIMEOUT` | `plugins.control_host_startup_timeout` | string | `5s` |
| `ANIX_CONTROL_PLUGINS_CONTROL_HOST_WEBSOCKET_SESSION_TIMEOUT` | `plugins.control_host_websocket_session_timeout` | string | `24h` |
| `ANIX_CONTROL_PLUGINS_CONTROL_POLL_INTERVAL` | `plugins.control_poll_interval` | string | `5s` |
| `ANIX_CONTROL_PLUGINS_DISPATCH_ENABLED` | `plugins.dispatch_enabled` | bool | `false` |
| `ANIX_CONTROL_PLUGINS_DISPATCH_POLL_INTERVAL` | `plugins.dispatch_poll_interval` | string | `5s` |
| `ANIX_CONTROL_PLUGINS_IDENTITY_BOOTSTRAP_PACKAGE_DIR` | `plugins.identity_bootstrap_package_dir` | string |  |
| `ANIX_CONTROL_PLUGINS_OFFICIAL_PUBLIC_KEY` | `plugins.official_public_key` | string | `lvbhRmhzVbSAbrw3vm0k7vYqpEu4/dF/ZqVbp2gS7uM=` |
| `ANIX_CONTROL_PLUGINS_TOPOLOGY_EXECUTION_ENABLED` | `plugins.topology_execution_enabled` | bool | `false` |
| `ANIX_CONTROL_PLUGINS_TOPOLOGY_POLL_INTERVAL` | `plugins.topology_poll_interval` | string | `5s` |
| `ANIX_CONTROL_SERVER_CORS_ALLOWED_HEADERS` | `server.cors.allowed_headers` | list | `Content-Type,Authorization,X-API-Key` |
| `ANIX_CONTROL_SERVER_CORS_ALLOWED_METHODS` | `server.cors.allowed_methods` | list | `GET,POST,PUT,DELETE,OPTIONS,PATCH` |
| `ANIX_CONTROL_SERVER_CORS_ALLOWED_ORIGINS` | `server.cors.allowed_origins` | list |  |
| `ANIX_CONTROL_SERVER_CORS_ALLOW_CREDENTIALS` | `server.cors.allow_credentials` | bool | `false` |
| `ANIX_CONTROL_SERVER_CORS_MAX_AGE` | `server.cors.max_age` | int | `86400` |
| `ANIX_CONTROL_SERVER_HOST` | `server.host` | string | `0.0.0.0` |
| `ANIX_CONTROL_SERVER_MODE` | `server.mode` | string | `release` |
| `ANIX_CONTROL_SERVER_PORT` | `server.port` | int | `8080` |
| `ANIX_CONTROL_SERVER_READ_TIMEOUT` | `server.read_timeout` | int | `30` |
| `ANIX_CONTROL_SERVER_SHUTDOWN_DRAIN_DELAY` | `server.shutdown_drain_delay` | string | `5s` |
| `ANIX_CONTROL_SERVER_TRUSTED_PROXIES` | `server.trusted_proxies` | list | `127.0.0.1,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16` |
| `ANIX_CONTROL_SERVER_WRITE_TIMEOUT` | `server.write_timeout` | int | `30` |
| `ANIX_CONTROL_TLS_CERT_FILE` | `tls.cert_file` | string |  |
| `ANIX_CONTROL_TLS_DOMAIN` | `tls.domain` | string |  |
| `ANIX_CONTROL_TLS_ENABLE` | `tls.enable` | bool | `false` |
| `ANIX_CONTROL_TLS_KEY_FILE` | `tls.key_file` | string |  |
<!-- END GENERATED -->
