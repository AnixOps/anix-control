### Changed

- The cross-repository Agent E2E runs the Agent's VLESS node on anixcore (built
  from the pinned `networkcore_anixops`, `NETWORKCORE_REF`) instead of the
  sing-box core, which the Agent no longer carries. The Agent is built with
  `-tags anixcore`; the dev release workflow drops the removed core tags.
