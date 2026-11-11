### Removed

- The kernel's legacy handlers for 5 v2 routes that ran them in production
  until now: the node list, detail and deletion (proxy-node) and a
  subscription group's protocols and the protocol pool (subscription). The
  package hosts serve them natively only; a host whose storage lease lacks
  the node tables (impossible since the kernel refuses to start before the
  node credential split is finalized) now fails the request instead of
  falling back.

### Changed

- **Behaviour change on upgrade:** these 5 routes switch from the kernel's
  handler to the package's native handler. Their compatibility tests proved
  the two answer the same bytes; the answers are now recorded in `testdata`.
