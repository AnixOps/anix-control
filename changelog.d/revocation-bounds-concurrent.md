### Fixed

- Concurrent sign-outs, password changes or bans of one user no longer lose
  a revocation. Writing a user's revocation bounds read the row and wrote
  it back without a lock, so on PostgreSQL a later commit could restore an
  older token version or not-before time (re-validating tokens that had
  been revoked), and two first revocations of a user could fail on the
  primary key. The row is now created if missing, locked, merged and
  updated in one transaction; the bounds only move forward.
