### Added

- Lines carry their forwarding policy and limits: `POST`/`PUT /api/v5/net/lines` take `policy` (PROXY protocol,
  balance over fixed targets, health check, circuit breaker, bandwidth, max connections), `hop_balance`,
  `quota_bytes` and `expires_at_unix_ms`, which `GET /api/v4/kernel/network/lines` now also answers. The line sheet
  edits them under 高级设置 / Advanced, so they stay editable once the old route editor goes.

### Changed

- `PUT /api/v5/net/lines/{id}` replaces the line's policy and limits with the draft's instead of keeping the stored
  ones; send what the read answered to keep them.
