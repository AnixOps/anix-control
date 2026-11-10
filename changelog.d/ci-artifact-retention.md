### Changed

- CI artifacts are kept for 3 days instead of the 90-day default, and the
  daily `Artifact prune` workflow deletes older ones, so the organization's
  shared artifact storage quota no longer fills up.
