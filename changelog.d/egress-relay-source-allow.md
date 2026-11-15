### Added

- An egress route's relays only accept its source node. Every exit (the main
  one and the fallbacks) gets `allow` with the source node's host and the
  public address it joined from, and Agent 5.0.7+ drops every other address
  on the relay port. Older Agents ignore it and leave the port open.
