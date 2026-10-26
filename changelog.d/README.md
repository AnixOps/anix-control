# Changelog Fragments

Every change adds one file here instead of adding bullets under
`## Unreleased` in `CHANGELOG.md`. Two open pull requests then never edit the
same lines: merging one does not make the other conflict, rebase and start a
new CI run. The release collects the files (`config/scripts/prepare_release.py`),
so `CHANGELOG.md` keeps the history and this directory holds only the entries of
the release that is not cut yet.

## Add A Fragment

Create `changelog.d/<slug>.md` in the pull request that makes the change.

- **Name.** `<slug>.md`: lowercase letters and digits, words separated by a
  single dash (`node-group-token-reset.md`). Name the change, not the date; the
  branch name without its `feat/` or `fix/` prefix is a good slug. A release
  lists the entries in file-name order.
- **Content.** One or more `###` sections, each a heading and its entries:

  ```markdown
  ### Fixed

  - A plan created in the admin UI is on sale. `POST /api/v2/admin/plans`
    stores `show = 1` when the body omits it; an explicit `show: 0` is kept.

  ### Added

  - The plan form has an On sale switch.
  ```

- **Headings.** The first line is a heading: `### Added`, `### Changed`,
  `### Deprecated`, `### Removed`, `### Fixed` or `### Security`. A file with
  an entry before its first heading is an error. A fragment may hold several
  sections, each heading at most once.
- **Entries.** A line that starts with `- ` begins an entry, as in
  `CHANGELOG.md`. Continuation lines (wrapped text, nested lists, code blocks)
  are indented by two spaces; a blank line between entries is allowed.
  Describe the change for the operator who upgrades: what changed, which
  setting, route or page, and what to do about it.
- **Hygiene.** UTF-8 without BOM, LF line endings, no tabs, no trailing
  whitespace, one newline at the end of the file, no merge-conflict markers.

`README.md` (this file) is documentation and is never collected.

## Check And Preview

```bash
python3 config/scripts/changelog_fragments.py check    # file:line messages, exit 1 on a problem
python3 config/scripts/changelog_fragments.py render   # the Markdown the release will contain
```

The "Documentation Sync Check" job runs `check` on every pull request. It also
accepts a fragment as the documentation evidence of a change
(`config/scripts/check_docs_updated.sh`), as it does `CHANGELOG.md`:
`changelog.d/README.md` alone is not evidence.

## At Release

`config/scripts/prepare_release.py <version>` merges the fragments into the
`## Unreleased` section of `CHANGELOG.md`, dates the section as the release,
and deletes the fragments with `git rm`. Entries join the headings that are
already under `## Unreleased` (after the entries there) and no heading appears
twice; headings come in the order of the list above. An invalid fragment stops
the preparation before any file changes. `docs/RELEASING.md` has the whole
flow. To merge by hand: `changelog_fragments.py consume --into CHANGELOG.md`.

## Direct Edits Of CHANGELOG.md

Bullets written directly under `## Unreleased` are still accepted, and the
release keeps them together with the fragments. New changes use a fragment, so
that they do not conflict.

## Release Branches

A fix that is cherry-picked to a `release/vX.Y` branch carries its fragment,
and that branch's release consumes it. In the follow-up pull request that
merges the release back to `main`, delete the fragments with the same file
names from `main`'s `changelog.d/` (`docs/RELEASING.md`), as its entries
already shipped.
