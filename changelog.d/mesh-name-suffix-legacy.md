### Changed

- Upgrade: a domain suffix saved earlier under `home.arpa` (or any other
  suffix outside `internal`) stops intranet names on every node as soon
  as the panel is upgraded. Open 组网 → 设置 before or right after the
  upgrade and save a suffix under `internal`, such as `mesh.internal`.
  Until then the settings, the name list and the device list say the
  names are not delivered, and the devices show no full names.
- Mesh: while the saved suffix breaks the rule, the disabled `dns`
  section carries no `resolver_address` either, so no node intercepts
  queries it would not answer. The suffix is lowercased for ASCII letters
  only, as in the client and the settings form.
