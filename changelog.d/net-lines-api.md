### Added

- Lines can be written: `POST /api/v5/net/lines`, `PUT /api/v5/net/lines/{id}`
  and `DELETE /api/v5/net/lines/{id}` (docs/design/network-lines, section 8).
  A line names its chain of devices and the exit device's protocol it serves
  or fixed targets, and whether it is offered in subscriptions; it is stored
  and planned as a forward route. `GET /api/v4/kernel/network/lines` gives
  each stored line's `revision`.
- A device's network settings (`/api/v4/kernel/nodes/{id}/network`) carry
  `scan_cloud`: a device on a cloud that scans its machines forwards in the
  kernel on the lines written after.

### Fixed

- Saving a device's network settings no longer clears its `scan_cloud` flag.
