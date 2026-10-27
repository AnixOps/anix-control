### Fixed

- The Agent installer installs nftables (apt-get, dnf, yum, apk, pacman or
  zypper) on a forward node that lacks `nft`, before its forward checks.
  A host that joined through a node group and only then turned out a
  forward node used to stop after the join with "nft is missing" and an
  Agent that never started (shown offline); it now installs nftables and
  starts the Agent. When no package manager can install it, the check
  fails as before with the command to run.
