### Fixed

- A node's traffic report is not counted at rate 1 when the node's rate
  cannot be read. The batch is now left unacknowledged (or acknowledged as
  `report_unavailable` to Agents that negotiated it) and counted once, at
  the node's rate, when the Agent resends it.

### Security

- A node's traffic report counts only for the users the node serves: the
  users of the node's plan group (every user when the node has no group),
  whether active, expired, over quota or banned. Entries for users of
  another group or unknown ids are not counted, the rest of the batch is,
  and the agent-control log records the node, batch id and user ids with
  reason `traffic_user_not_served`. Traffic of a user moved to another
  group is counted only if it reaches Control before the move.
