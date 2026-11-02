### Removed

- Package hosts no longer support the `shadow` route mode, and
  `sdk/shadowsample` is gone. Route modes come only from
  `config/package-route-defaults.json`, which lists native routes; nothing
  ever selected `shadow` after the 4.1 rehearsal. The
  `anixops_package_shadow_*` metrics are no longer exported.
- `cmd/wgrotate` no longer falls back to `/etc/v2board/config.yaml`; it reads
  `/opt/anixops/control/config/config.yaml` unless `-config` is given.

### Changed

- The 4.0.x RC roadmap and evidence snapshot moved to `docs/archive/`.
