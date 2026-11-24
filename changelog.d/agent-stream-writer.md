### Fixed

- Each Agent control stream has one writer: heartbeat, report and
  maintenance acknowledgements are written before waiting configuration,
  user, alive-list and operation messages, and an Agent that stops reading
  its stream for 2 minutes has its session ended, so the stream's receive
  loop and the senders waiting on it no longer block until the connection
  dies (backend review ARC-001).
