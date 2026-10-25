### Added

- A manual GitHub Actions workflow, JumpServer, reaches the node fleet through
  JumpServer (`docs/guide/jumpserver.md`). `scripts/jumpserver/jms.py` signs
  its requests with the Access Key of the `production` environment and
  checks the key, lists the hosts, or runs a shell command on them through
  the job center (Ansible). The repository and its workflow logs are public,
  so in Actions host addresses and names are masked and per-host output is printed
  only on request.
