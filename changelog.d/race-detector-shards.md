### Changed

- The nightly Go Race Detector runs in three shards and builds the pure-Go
  SQLite (`modernc.org/...`) without race instrumentation, so it finishes
  inside its timeout again; the Makefile `-race` targets use the same flag.
