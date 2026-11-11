### Removed

- The kernel's legacy handlers for the last 10 protocol-runtime routes that
  ran them in production: a node's protocols (list, creation, update,
  deletion), node synchronization, Agent Control status and operations, the
  agent list, the monitoring data and diagnostic task creation. The package
  host serves all its routes natively only, and the SDK router's `Flagged`
  option is gone with them.

### Changed

- **Behaviour change on upgrade:** these 10 routes switch from the kernel's
  handler to the package's native handler. Their compatibility tests proved
  the two answer the same bytes; the answers are now recorded in `testdata`.
- A node protocol write the gateway cannot seal is no longer served by the
  legacy handler: a body that is not JSON, a protocol id that is not a
  number and a settings document that is not JSON (for example a
  `custom_config` of plain text, which no node configuration ever used) are
  refused with 400 `plugin_request_invalid`.
- Creating a diagnostic task with a negative `timeout` answers 400 instead
  of falling back to the legacy handler.
