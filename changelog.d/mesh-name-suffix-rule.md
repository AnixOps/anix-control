### Changed

- Mesh: the intranet domain suffix now follows the kernel's rule. It must
  have at least two labels and lie under `internal` (default
  `mesh.internal`). It is compared without one final dot and in
  lowercase, and has at most 172 characters. The root, any single label,
  `arpa` and everything under it (including `home.arpa`), `local` and
  everything under it, and every other domain are refused. The settings
  page explains the reason before saving. A suffix saved before this rule
  is no longer delivered: nodes get `dns.enabled: false` with no names,
  and the settings show `name_suffix_invalid` until a valid suffix is
  saved.
