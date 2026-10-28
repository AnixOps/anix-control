### Added

- Entry HA can judge entry addresses from the clients' side: a DNS
  binding's optional `vantage` selector (a node label such as `region=cn`)
  has the forwarding nodes carrying it dial every published entry address
  (`NodeForwardState.vantage_probes`, reported back as
  `NodeForwardReport.vantage`). An address more of them find down than up
  leaves the DNS records, with the never-empty rule kept, and the route's
  DNS status shows the unreachable addresses (migration 0015). The binding
  form and the entry HA card on the route page show both. Agents need the
  matching probe loop to report results; until then nothing changes.
