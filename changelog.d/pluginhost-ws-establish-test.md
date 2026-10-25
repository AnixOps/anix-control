### Fixed

- **Tests:** `TestWebSocketRelayRegistersBeforeOpeningStream` no longer fails
  now and then under load (5 of 3000 runs). Its fake host client opened the
  stream when the test released it even if the relay's context was already
  cancelled, because the test could finish `Drain` and release the open before
  the client goroutine reached its `select`, which then picked either ready
  case.
