### Fixed

- The cross-repository E2E graceful-restart scenario looks for the Agent's
  "Took the forwarding state" line: anix-agent 5.0.0 (#131, forward driver
  framework removed) no longer logs "Applied the forwarding state", so the
  v5.0.0 tag build failed although the Agent reapplied generation 1.
