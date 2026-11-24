### Fixed

- Leases are fenced (backend review ARC-003): a lease's generation is
  raised each time another Control process takes it, and the singleton
  work's high-risk writes (Agent upgrade campaigns, reverse proxy
  certificates, automatic Reality target switches) check holder,
  generation and expiry in their transaction, so a process that lost the
  lease writes nothing more. Lease times come from the database clock
  (PostgreSQL `now()`), not from each host's clock.

### Changed

- Migration `0043_lease_generation` adds `v4_kernel_lease.generation`
  (additive). Back the database up before the upgrade; a previous release
  does not open a database with this step.
