### Added

- Dynamic traffic categories: 访问统计 now judges the categories by the
  built-in rules, remote sources (domain and network lists fetched from a
  URL on a schedule, in plain, v2fly domain-list-community or Clash format)
  and the administrator's own rules (domain, network or protocol), each
  layer overriding the one before, without a release. Edited at 流量与监控 →
  流量分类 (with a test of where a destination falls) and through
  `/api/v4/admin/traffic/categories`; changes apply to the recorded history
  at once (migration 0024, docs/reference/destination-stats.md).
