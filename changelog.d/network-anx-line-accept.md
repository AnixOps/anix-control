### Changed

- The accepting end of an `anx_line` transport gets its listen addresses in
  its plan (`tcp_addr` / `quic_addr` / `faketcp_addr`: the hop ingress's
  listen address with the transport's port per carrier); `server_public`
  stays the dialing end's only.
