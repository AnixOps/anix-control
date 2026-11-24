### Fixed

- A configuration snapshot an Agent could not apply for a passing reason
  (`config_apply_failed`, for example a port still in use) is sent again
  at each configuration refresh (every minute) until it applies, instead
  of waiting for a reconnect or the next configuration change. A snapshot
  the Agent refused as invalid (`config_invalid`) is not resent until the
  configuration changes.
