### Changed

- The console has one node page. 节点 (`/admin/nodes`) lists every machine
  with a 角色 column: 落地 for a node with protocols, 中转 for one without
  protocols or carrying forward-route hops, with its hop count and hop
  errors. The forwarding area loses its 节点 section: `/admin/forward/inventory`
  opens the node page's new 转发主机 tab, which holds Ansible hosts and
  forward nodes not yet converted (with 转为常规节点). A node's 转发设置 is in
  its row menu.
