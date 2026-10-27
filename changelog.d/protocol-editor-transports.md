### Fixed

- The node protocol editor offers only the transports and TLS modes the
  chosen protocol runs on: Shadowsocks is TCP without TLS, Hysteria2 and TUIC
  are UDP with TLS, VLESS/VMess/Trojan take tcp, ws, grpc, h2 and
  httpupgrade, and Reality (VLESS and Trojan) runs on tcp, grpc and h2.
  Switching the type no longer keeps the previous type's transport (a
  Hysteria2 protocol switched to Shadowsocks showed UDP and every stream
  transport). Saving refuses a combination no kernel runs, in the editor and
  in Control (`ValidateNodeProtocol`); the transport label older editors
  stored for Shadowsocks, Hysteria2 and TUIC (tcp or udp) is still accepted.
