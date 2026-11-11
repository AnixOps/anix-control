### Removed

- The kernel's legacy handlers for the 27 v2 routes the platform (audit
  log), machine-telemetry (dashboard, hourly traffic, user traffic ranking)
  and notification (23 routes) packages already answered natively in
  production, with the kernel statistics and Telegram bot service methods
  only they used. The three package hosts now run these routes natively
  from the start (`pluginhostsdk.RouterConfig.NativeOnly`).

### Changed

- The compatibility tests of these routes (`platformcompat`,
  `machinetelemetrycompat`, `notificationcompat`) no longer compare with a
  legacy handler: they compare each native answer, on SQLite and
  PostgreSQL, with the one recorded in `testdata` while the route still
  matched its legacy handler byte for byte. `PACKAGECOMPAT_RECORD=1`
  records the answers again after an intended change.
