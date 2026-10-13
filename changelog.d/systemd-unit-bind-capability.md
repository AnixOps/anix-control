### Fixed

- The systemd unit `scripts/install.sh` writes has `AmbientCapabilities=CAP_NET_BIND_SERVICE`. A non-root Control
  binds `:443` and `:80` in `tls.mode` `acme` (the default once `domain` is set) and `files`; the unit gave it no
  capability, so a fresh install that followed the one-key configuration stopped at the start with
  `https on :443: listen tcp :443: bind: permission denied`. `tls.mode: external` was unaffected.
