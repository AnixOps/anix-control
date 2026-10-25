### Fixed

- The JumpServer workflow's `run` action works end to end: `jms.py` marks its
  job as a one-off (`is_periodic: false`), which JumpServer's job serializer
  otherwise assumes true; with `show_output` it reads the job's Ansible log,
  which holds each host's output, from JumpServer's task log websocket (the
  only place current JumpServer serves it); a log it cannot fetch no longer
  fails a job that succeeded; and a command containing `{{ }}` is no longer
  rejected by Ansible's templating.
