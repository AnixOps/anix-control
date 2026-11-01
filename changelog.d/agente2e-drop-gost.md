### Removed

- The Agent end-to-end suite (`internal/tests/agente2e`) no longer sets up
  gost: the `gost` shim, the gost-aware `systemctl` stand-in, the
  `Forward.Gost` configuration and `ANIXOPS_GOST_BIN` are gone, since the
  Agent no longer runs gost.
- `scripts/upgrade_legacy_panel_v2.5.0.sh`, the one-off upgrade helper for
  the old native v2board layout on pl-1, and its `docs/UPGRADE.md` section.
  Production runs the installer layout on OVH-PL.
