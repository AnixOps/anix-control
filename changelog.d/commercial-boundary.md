### Added

- `config/commercial-boundary.json` and `check_commercial_boundary.py` record
  which code forms the commercial set (payment, order, coupon, invite
  commission) and fail a change that adds a new dependency on it from the
  rest of Control. No behavior changes; see
  `docs/architecture/commercial-boundary.md`.
