### Added

- Mesh: subnet requests from full nodes (R35). A full node without an
  Agent, such as a router, asks with its device token to advertise its
  local subnets (`POST /api/v5/mesh/device/subnet-request`, at most 16
  private IPv4 networks; an empty list withdraws the request). The device
  list shows the pending request, and an administrator approves it, which
  replaces the confirmed subnets under the usual overlap checks, or
  rejects it (`/api/v4/kernel/mesh/devices/:device_id/subnet-request`;
  migration 0052).
