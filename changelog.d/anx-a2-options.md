### Added

- The ANX protocol form offers every option of the A2 jacket: 无 SNI 模式
  (`allow_no_sni`: the node also accepts connections without SNI, and
  subscriptions leave `sni` out of the `anx://` link so clients dial the
  IP; needs an anx-node newer than alpha.7), turning traffic shaping off
  (`shape_plain`) and max streams per session (`max_streams`, 1 to 65535,
  default 256). Security shows as the A2 borrowed handshake instead of
  "no TLS". Control sends the new options to the Agent only when set, so
  nodes on an older anx-node keep working; needs the matching Agent.
