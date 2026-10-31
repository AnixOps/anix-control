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
