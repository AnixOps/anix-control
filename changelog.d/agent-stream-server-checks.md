### Security

- An open Agent control stream is checked by Control itself, not only when
  the Agent sends a heartbeat: the client certificate is checked again
  every 30 seconds (with the 30-second revocation cache, a revoked or
  expired certificate ends the stream within a minute), and a stream that
  has sent nothing for three heartbeat intervals (60 s, or 180 s on
  forward nodes) is closed. A revocation check that fails, for example
  while the database is down, keeps the stream for at most 5 minutes after
  the last check that succeeded instead of indefinitely.
