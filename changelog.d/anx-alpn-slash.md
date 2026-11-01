### Fixed

- An ANX node protocol with the default ALPN `h2,http/1.1` saves again.
  The settings check refused `/` in every list entry, which rejected the
  RFC 7301 ID `http/1.1`; ALPN entries now refuse only whitespace and `,`
  (at most 255 bytes), and server names still refuse `/`.
