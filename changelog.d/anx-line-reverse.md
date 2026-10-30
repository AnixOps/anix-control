### Added

- ANX-Line reverse dial-back in the network model: a transport's
  `anx_line.reverse` (with `reverse_host`, `reverse_port`, default 24444)
  has the accepting device open no port and dial the dialing device's
  reverse listener instead. Plans carry `reverse`, `reverse_listen` (dialing
  end) and `reverse_dial` (accepting end) and no carrier addresses; only
  bare `anx_line` (l1, c1) runs reversed.
