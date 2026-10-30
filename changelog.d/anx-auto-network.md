### Changed

- ANX routes no longer need a hand-written network model: the route editor
  lists every node whose Agent is connected, and a hop's transport and the
  exit's egress default to 自动. Control then adds what the route needs to
  the plans (a line on the device, an internet egress, and an ANX link on
  port 24443 with its link key), under `auto-` ids the model document
  cannot use. `GET /api/v4/kernel/network` returns these devices as
  `nodes`.
