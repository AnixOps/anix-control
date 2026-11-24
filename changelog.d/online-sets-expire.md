### Fixed

- A node's online users expire 5 minutes after its last traffic report.
  The sets used to stay in memory until the next report of the same node,
  so a node that went offline kept its users online for the device limit
  (alive list) indefinitely, outside the cache's size limit. Each node's
  set is now replaced as a whole and only by a report whose window ends no
  earlier than the current one's, so a replayed older batch no longer
  brings back users who have left, and readers never see a half-written
  set.
- The dashboard's online user count counts the users online on any node.
  It read a set nothing wrote and always showed 0.

### Removed

- The memory cache's set operations (`SAdd`, `SCard`, `Expire`, `Keys`) and
  the unused `ServerService.GetUserOnlineCount`, whose pattern never matched.
