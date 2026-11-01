### Removed

- The Control Center (`control-center/`: Go CLI/TUI, Vue web, Flutter client
  and its Cloudflare Workers API) and its `control-center`,
  `control-center-release` and `control-center-workers` workflows. It kept
  its own users and nodes apart from Control, several of its pages were
  mocks, and Control's admin UI already covers its plugin lifecycle page.
  The Cloudflare Worker (`api.anixops.com`) and its D1 database are not
  touched by this change and are retired by hand.
