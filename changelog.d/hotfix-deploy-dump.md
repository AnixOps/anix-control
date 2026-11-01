### Fixed

- The hotfix deploy dumps the database of the running Control before it
  upgrades it, also on a host managed by AnixOps-ansible (its backup script,
  into `/var/backups/anixops/pg`), and refuses to upgrade when it cannot
  dump; it used to skip the dump silently on a host without
  `/root/pg-backups`.
