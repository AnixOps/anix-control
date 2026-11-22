### Changed

- The "CLA" check now covers every commit author of a pull request, not only
  the pull request author, accepts new signature lines only for the pull
  request author, and rejects removed or changed signatures
  (`.github/scripts/check_cla.py`, with a self-test). `CLA.md` documents the
  rules. `docs/audit/commercial-5.1.md` records the 5.1 licensing decisions,
  the evidence of each step and how each public commitment is enforced.
