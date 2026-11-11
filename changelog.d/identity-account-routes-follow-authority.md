### Changed

- identity-platform's 7 routes outside group A (the user's profile and
  dashboard, the administrator's user list, detail and statistics, and the
  traffic and subscription resets) now run natively while identity is
  authoritative, like group A, and return to legacy on a rollback. Before,
  they stayed legacy after the cutover. Nothing changes before the cutover.
