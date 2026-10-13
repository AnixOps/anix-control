### Changed

- Changelog entries are one file per change, `changelog.d/<slug>.md` (format in `changelog.d/README.md`), instead of
  bullets under `## Unreleased` in `CHANGELOG.md`, so a merged pull request no longer makes every other open pull
  request conflict in the changelog and rebase into a new CI run. A fragment starts with a `### Added`,
  `### Changed`, `### Deprecated`, `### Removed`, `### Fixed` or `### Security` line and lists `- ` entries. The
  "Documentation Sync Check" job validates them (`config/scripts/changelog_fragments.py check`) and accepts a
  fragment as the documentation evidence of a change. `prepare_release.py` merges the fragments into the release
  section, joining the headings already under `## Unreleased`, and deletes them. Entries written directly under
  `## Unreleased` stay legal and are released with the fragments.
