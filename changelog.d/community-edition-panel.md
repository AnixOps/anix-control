### Changed

- The panel no longer shows the commercial entry points. The public
  configuration (`GET /api/v4/public/config`) reports the `community` edition
  and hides the `affiliate`, `order` and `payment` packages, so the 商业 menu
  group (orders, coupons, payment, invite commission) and the user plans and
  orders pages are gone. The plans admin page is listed as subscription
  templates. The routes themselves are unchanged until the commercial set
  moves to its own repository.
