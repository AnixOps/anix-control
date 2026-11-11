### Removed

- Node protocols no longer offer the `h2` transport, and Reality on `h2` is
  refused when a protocol is saved: neither the Agent nor the kernel has an
  h2 transport, so such a protocol never started. A stored protocol on `h2`
  keeps showing it; move it to `tcp`, `ws`, `grpc` or `httpupgrade`. The JSON
  tab takes `h2` or `quic` only for a protocol that already has it.
