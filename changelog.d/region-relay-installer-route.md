### Changed

- With `--relay`, `install.sh` never contacts Control directly (R34): every
  request to Control's https address goes through the first relay that
  forwards Control's https port (same port number, AnixOps-ansible
  `region_relay_control_https`); the rendered command fetches the script
  through the first relay too (`curl --connect-to`). `/install/ip` is not
  asked through a relay. With `--release-cache`, the Agent zip comes from
  the caches first, by its SHA-256.
- Re-running a group's command with relays on a node installed directly
  rewrites its configuration with the route (backup kept), so it moves
  behind the relays.
- Relay and cache ports must be plain numbers (no sign, no leading zero),
  as the installer requires.
- The `control_route` contract: an empty or missing relay list never means
  "reach Control directly"; only an explicit `direct: true`, which Control
  does not send yet, would.
