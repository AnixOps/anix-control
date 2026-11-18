### Changed

- Revoking an ANX device takes effect on the nodes within a minute instead of
  when its license runs out. While ANX licensing is on, the anx inbound gets
  `license_revoked` (`<user_id hex>:<device_id hex>` of each revoked device
  whose last license has not expired), so nodes that require a license drop
  the device and refuse it. The same device under another account keeps
  working. Needs an Agent and anx-node that know the field; older ones ignore
  it.
