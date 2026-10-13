### Added

- `config/scripts/bump_agent_ref.py <sha> <agent-tag>` moves every Agent pin of a release pull request at once:
  `AGENT_REF` in `ci.yml` and its literal in `check_release_workflow.sh`, the Agent module path and version label
  of the cross-repository E2E build (`ci.yml`, `internal/tests/agente2e/build_test.go`) and `config/AGENT_RELEASE`.
  It refuses a commit that is not a full lowercase SHA and a malformed tag, changes nothing unless it finds every
  pin, and runs the release workflow check (`docs/RELEASING.md`).
