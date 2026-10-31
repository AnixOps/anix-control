### Added

- `config/commercial-boundary.json` and `check_commercial_boundary.py` record
  which code forms the commercial set (payment, order, coupon, invite
  commission) and fail a change that adds a dependency on it from the
  rest of Control. No behavior changes; see
  `docs/architecture/commercial-boundary.md`.

### Changed

- The order admin routes, the dashboard's order and income figures, the
  order-paid notification and invite-code registration moved behind small
  extension points (`RegisterDashboardContributor`, `RegisterInviteRedeemer`)
  so nothing outside the commercial set references it. Routes and responses
  are unchanged.

### Removed

- The legacy handlers of the affiliate, order and payment routes. The 43
  routes (invite commission, orders and coupons, payment gateways, callbacks
  and records) are `native` in `config/package-extraction.json`: the package
  hosts, which already served them by default, answer alone and the router
  binds the bare gateway. The old handler tests and the legacy-versus-native
  parity packages `ordercompat` and `paymentcompat` are gone with them, and
  the generated Swagger documents no longer list those routes.
