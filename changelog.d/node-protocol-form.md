### Changed

- The node protocol editor is a form first, like 3x-ui: choosing a protocol,
  TLS mode and transport shows only the fields that apply (VLESS flow,
  Shadowsocks cipher, WebSocket/gRPC path, host and service name, TLS SNI and
  fingerprint, Reality target, short id and keys, Hysteria2 bandwidth and
  obfuscation, TUIC congestion control). The whole protocol as JSON is the
  Advanced JSON tab and stays in step with the form; `custom_config` and keys
  the form does not know are kept.
- The node group template editor shows the same protocol and transport fields.
