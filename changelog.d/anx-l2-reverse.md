### Added

- The network model runs kind `anx` (the A2 jacket) reversed as well
  (`anx_line.reverse`, anx ANX-LINE-REVERSE §6.4): the overseas accepting
  device opens no port and dials back to the dialing device's
  `reverse_host:reverse_port` as the jacket's client, on c1. Its plan
  carries the dialing device's `server_public`, and the path waits until
  that device has reported its ANX-Line server key.
