### Fixed

- The JumpServer workflow's `run` action works end to end: `jms.py` marks its
  job as a one-off (`is_periodic: false`), which JumpServer's job serializer
  otherwise assumes true, and with `show_output` it prints the job's Ansible
  log, which holds each host's output, instead of asking for a result
  endpoint JumpServer does not have. A log it cannot fetch no longer fails a
  job that succeeded.
