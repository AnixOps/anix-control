### Fixed

- The hotfix deploy's database dump runs outside `/root`: the AnixOps-ansible
  backup script runs as `postgres`, whose `find` failed to return to `/root`
  and stopped the deploy before the upgrade.
