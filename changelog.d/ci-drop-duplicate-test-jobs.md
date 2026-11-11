### Removed

- CI no longer runs the Forward Runtime Regression job (every test it named
  was removed with the old forward runtime), nor the gRPC Core Tests, CLI
  Tests and Smoke Tests jobs: Backend Tests already runs those packages.
  Backend Tests now enforces the 80% coverage of `internal/grpc`. The
  `forward` change class is gone with its only job.
- The model tests that only compared table names and constants with
  themselves.

### Changed

- On pull requests the browser accessibility sweeps run two theme and
  viewport combinations (light desktop, dark phone) instead of four; the
  full lane still runs all four.
