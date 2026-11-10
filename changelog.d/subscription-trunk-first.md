### Changed

- Subscriptions list trunk nodes first in each group: a node whose users
  reach the internet through a domestic ingress and a shared link (a network
  model path with a transport, a forward route published in subscriptions, a
  Panabit iWAN line), before nodes subscribers dial across the border
  directly. Within each kind the group's order is unchanged, so a group's
  first entry, and the default of clients that start on it, is a trunk node
  whenever the group has one (anx FUTURE-IMPROVEMENTS F-51, 默认路径规则).
  No node is removed and no node configuration changes.
- The AnixOps Net subscription (`format=anixops`) puts a
  `#anixops:path=trunk` line before each trunk node's link. Clients that do
  not know it skip it as a comment.
