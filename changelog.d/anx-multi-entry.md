### Added

- ANX routes can have several entry devices for entry HA (with
  `listen.entry_hostname`). Each entry device runs the route's port
  forward: the first on the transport the route names, the others on a
  transport of their own into the same ingress (or, on a single-hop route,
  an internet egress of their own), chosen by the network model. An entry
  without one waits on its own while the others run. Health, latency,
  diagnosis and the DNS binding judge every entry device, so DNS fails
  over between them.
