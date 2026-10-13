### Added

- Control obtains and renews its own certificate. With `domain:
  panel.example.com` (one line) `tls.mode` is `acme`: Control serves the
  web app, the API, subscriptions and `/install.sh` over https on 443,
  answers the ACME HTTP-01 challenge and redirects to https on 80, and
  serves the same certificate on the node gRPC listener (`grpc.port`,
  50051), which `domain` turns on. Certificates come from the public CA
  over TLS-ALPN-01 (443) or HTTP-01 (80), are renewed a third of their
  lifetime before the end and reach new connections without a restart.
  They are kept in the new table `v5_acme_cache`, sealed with
  `module_runtime.ca_kek` (now required for ACME), so a restore does not
  request them again. While no certificate can be obtained Control starts
  anyway with a self-signed one.
- The CDN mode: `tls.mode: external` with `listen` (default loopback)
  serves plain http for a CDN or reverse proxy that terminates TLS, and
  `grpc.public_name` names a DNS-only host for the nodes; Control obtains
  its certificate over ACME and hands it out in `/install/agent.env`
  (`grpc_target`). `tls.mode: files` serves your own `tls.cert_file` and
  `tls.key_file`, reloaded when they change.
- `GET /api/v5/panel/status` and `POST /api/v5/panel/certificates/retry`
  (administrators), and a banner in the admin console while a certificate
  fails, is pending, fails to renew or is not publicly trusted, with the
  reason and the fix.
- `anix-control tls status`, `anix-control tls retry` and `anix-control
  doctor` (configuration, root key, certificates, DNS records against this
  machine's addresses including a CDN hint, ports).
- Alerts `panel_certificate_expiring` and
  `panel_certificate_renewal_failing`.
- Install readiness checks `panel_certificate` and `grpc_certificate`: the
  join command is withheld while the panel or gRPC certificate is not valid
  and publicly trusted, because Agents verify it and fail closed.

### Changed

- One HTTP listener: the API listener (8080) and the web listener (3000)
  with its reverse proxy to the API are one handler. Without `domain` and
  `listen`, Control binds `server.host:server.port` and, with
  `frontend.enable`, `server.host:frontend.port` with that same handler; a
  path under `/api/`, `/install/` or the subscription path that matches no
  route answers 404 instead of the web app's HTML.
- The built-in `server.host` is `127.0.0.1` (an empty one is loopback
  too); the container image sets `ANIX_CONTROL_SERVER_HOST=0.0.0.0`, so
  containers are unchanged. Configuration files that set `server.host`
  keep it.
- `grpc.tls_cert_file`/`grpc.tls_key_file` are reloaded when the files
  change instead of read once at start.
- `agent_install.public_url` defaults to `https://<domain>` and
  `agent_install.grpc_target` to `<grpc.public_name or domain>:<grpc.port>`.
- `tls.enable` is ignored (logged); `tls.domain` is the older spelling of
  `domain`.

### Security

- No HSTS header over Control's own TLS: a browser that remembered it could
  not open the panel while the self-signed fallback is served. HSTS stays
  when a trusted proxy reports https.
