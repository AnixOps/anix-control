### Removed

- The kernel's legacy handlers for the knowledge and ticket routes (14 v2
  routes) and their native-vs-legacy compatibility tests. The knowledge and
  ticket package hosts now run every route natively from the start
  (`pluginhostsdk.RouterConfig.NativeOnly`), so the routes no longer depend
  on the host's first route-mode poll. First batch of the legacy-handler
  removal; the other packages keep theirs for now.
