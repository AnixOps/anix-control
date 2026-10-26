### Added

- 访问统计: each user's traffic by destination domain and by category
  (video, music, messaging, social, gaming, AI, downloads, websites,
  unknown). Administrators read it under 流量与监控 → 访问统计
  (`/admin/monitor/destinations`, `GET /api/v4/admin/traffic/destinations`)
  and every user reads their own on the new 访问统计 page (`/user/traffic`,
  `GET /api/v4/user/traffic/destinations`). Only registrable domains are
  recorded, never URLs or content, in the new table `v5_destination_stat`
  (migration `0006_destination_stats`). The statistics are off by default:
  turn them on and set the retention (7 to 180 days, default 30) on the
  same page (`PUT /api/v4/admin/traffic/destinations/settings`), and tell
  your users what is recorded. They need an anix-agent release that
  sends the new `DestinationReport` (`destinations.v1` on the Agent Control
  stream); `docs/reference/destination-stats.md` has the details.
