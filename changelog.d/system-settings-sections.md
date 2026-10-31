### Changed

- 系统设置 is sorted into sections you pick from: 通用 (subscription domains
  and links to the mail and sign-in security settings), a new 数据与保留
  (the access statistics switch and a preset retention list), and a new 高级
  that holds the raw configuration key table, with a troubleshooting warning.
  The configuration key list no longer shows the scheduler's internal
  `scheduler.*` keys (`GET /api/v2/admin/system/configs`).

### Added

- `GET /api/v5/settings/catalog` lists the options 系统设置 offers (key,
  section, kind, choices, limits, the value in force); the console draws
  the 通用 links and the 数据与保留 choices from it.
