### Fixed

- The JumpServer workflow's `run` action no longer fails with "Periodic or
  recurring settings required": `jms.py` now marks its job as a one-off
  (`is_periodic: false`), which JumpServer's job serializer otherwise
  assumes true.
