### Fixed

- The container image's health check works in every `tls.mode`. It probed `http://127.0.0.1:8080/readyz` only,
  which exists in `external` mode; with `domain` set and the default `acme` mode, or in `files` mode, Control
  listens on 443 and 80 and not on 8080, so the container turned `unhealthy` after its start period. The probe
  now falls back to `https://127.0.0.1:443/readyz`.
