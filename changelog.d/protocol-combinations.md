### Changed

- Node protocols offer only what the Agent's kernel runs, listed in
  `contracts/agent/v1/protocol-combinations.json`; tests in Control, the web
  editor and the Agent read that file, so a combination cannot be offered
  without being mapped. Two more combinations that never ran are gone with it:
  - Reality is VLESS directly on `tcp`. Reality with Trojan, or on `grpc`, is
    refused when a protocol is saved: the Agent refused Reality on gRPC and
    served a Trojan Reality protocol with the node's own certificate.
  - Trojan is always TLS in the editor, as the node always serves it with its
    certificate; a stored Trojan protocol with another TLS mode opens as TLS.
