### Fixed

- Route detail: on an ANX route (entry and exit on the network model) the
  entry checks each target through the whole path, entry to exit to target.
  These checks were shown as the entry's own upstreams ("hop 1 has an
  upstream with an open circuit"), which read as if the entry dialed the
  target directly. They now appear under the targets ("checked end to end by
  <entry>"), the banner names them end-to-end checks and says when no target
  is reachable, the health table labels them "End to end", and they no
  longer mark the entry's dot or feed the entry latency card, which keeps
  the entry-to-next-hop link round trip.
