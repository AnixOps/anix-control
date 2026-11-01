### Added

- 系统设置 → 通用 → 订阅域名 has a switch "只在订阅域名上提供订阅"
  (`app.subscribe_domains_only`, off by default). On, the subscription paths
  (`/s/<token>`) answer only on the
  configured subscription domains; the panel's own domain and any other host
  answer 404. It keeps the panel domain out of what clients fetch, so a
  blocked subscription domain can be swapped without touching the panel. It
  does nothing while no subscription domain is configured.
