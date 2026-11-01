### Changed

- Payment, order, coupon and invite commission (the `payment`, `order` and
  `affiliate` packages, `internal/payment`, `internal/kernelorder` and their
  handlers, services and `/api/v2` routes) moved to the private
  `anix-control-commercial` repository. This repository is now the community
  edition: it builds and tests without them, the public config reports
  `edition: community`, and the commercial build overlays them back. Invite
  codes stay in the community edition. The route catalogs and package list
  describe the community set (12 packages).
