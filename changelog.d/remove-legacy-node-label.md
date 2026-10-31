### Removed

- The node list and the node page no longer show the 旧版 (Legacy) and API key
  stream connection types: the Agent channels are the mTLS stream, third-party
  node software and offline. `GET /api/v4/kernel/agents/transports` drops
  `summary.legacy`, `legacy_only`, `connection.type` values `legacy` and
  `apikey_stream`, `status: legacy` and the `legacy` required blocker; sightings
  of the retired channels (API key stream, REST, WebSocket, clean agent) are not
  listed. `anix-control agents transports` loses `--legacy-only`, and the
  startup warning only names nodes that never enrolled.
