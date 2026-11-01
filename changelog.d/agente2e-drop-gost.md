### Removed

- The Agent end-to-end suite (`internal/tests/agente2e`) no longer sets up
  gost: the `gost` shim, the gost-aware `systemctl` stand-in, the
  `Forward.Gost` configuration and `ANIXOPS_GOST_BIN` are gone, since the
  Agent no longer runs gost.
