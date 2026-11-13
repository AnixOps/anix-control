### Added

- ANX licensing on the panel side, off by default (`internal/entitlement`,
  anx spec §2.4.1). A generic feature entitlement with `anx` as its first
  feature: the panel's license CA, derived from the root key, signs a
  7-day ANX device certificate when a client refreshes its subscription with
  `X-AnixOps-Device`, while the plan includes ANX or the account's one trial
  runs and a seat of the user's device limit is free. Devices are kept in
  `v5_entitlement_device` (migration `0037_entitlement_devices`). With
  `entitlement.anx.enabled` on, ANX nodes get `license_ca` and, with
  `entitlement.anx.require`, `require_license`. No console page yet.
