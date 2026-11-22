### Added

- `source_url` (default `https://github.com/AnixOps/anix-control`): the source
  code address the sign-in page and the account menu link to, the AGPL-3.0
  offer of the source to the panel's users. An operator who runs modified
  code points it at their own repository. `GET /api/v4/public/config` returns
  it.
- `internal/quota`: the check before a new user (identity platform
  `CreateSubscriber`) or node (manual node, node group join, forward node) is
  created. The community edition sets no check, so nothing is limited; a test
  fails if public code ever sets one. The commercial edition sets it from its
  license.
- `web/src/extensions`: the routes and menu items the commercial edition adds
  to the web app. Empty in the community edition.
