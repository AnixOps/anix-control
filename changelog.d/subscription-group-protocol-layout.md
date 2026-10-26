### Added

- Subscription groups can order their node protocols and give each one a
  display name. 订阅分组 → 节点协议 has an 排序与名称 sheet: drag a row (or
  use ↑/↓ on its handle, or the arrow buttons) into place and type the name
  subscribers see; an empty name keeps "<node> - <protocol>", and a node with
  entries still appends " | <entry>". Subscriptions list each group's
  protocols in that order at once; linked protocols the order leaves out
  follow by id. The table shows each protocol's name in the subscription.
  `GET` and `PUT /api/v4/admin/subscription-groups/{id}/protocol-layout`
  read and replace the order and names (table
  `v5_subscription_group_protocol`, migration
  `0011_subscription_group_protocols`).

### Changed

- A subscription that combines several subscription groups lists the nodes
  group by group, in the user's group order, instead of by protocol id.
