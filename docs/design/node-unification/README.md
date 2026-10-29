# Node unification: one machine, one node

Status: implemented in 5.0.0-rc.11 (the base of the gost removal).

## Why

The console had two kinds of node, for historical reasons:

- regular (proxy) nodes, `v2_node`, Agent kind `proxy`: the v2board node
  table, serving subscription protocols;
- forward nodes, `v2_forward_node`, Agent kind `forward`: the merged
  flux/gost forward panel, the entries and relays of forward routes.

Both run the same Agent. A forward node's Agent runs no core, so it cannot
run the network model (`network.v1`) and so cannot carry ANX relays, which
replace gost (see `docs/design/network-model`, N1 and N8). The split also
doubled the onboarding paths and the id space (`proxy-3` and `forward-3`
are different machines).

## Decision

A relay is a regular node without user protocols. Every machine that runs
the Agent is a regular node; forward routes and the network model name it
`proxy-<id>`.

- **Node groups make regular nodes.** The group kind is stored as `proxy`
  (migration 0017 rewrites `forward` groups); `forward` is still accepted
  from older clients and stored as `proxy`. A group without a protocol
  template makes relays; the console shows it as a relay group.
- **No new Agent forward nodes.** `CreateForwardNode` takes Ansible hosts
  only (`NODE_TRANSPORT_ANSIBLE`); those run no Agent and stay forward
  nodes.
- **Relays forward.** The install command of a regular node without
  protocols (or of one already in the forwarding inventory), and of a node
  group without a template, carries `--forward`: the installer writes
  `Forward.Enable` with `NodeKind: proxy` and the node id, so the Agent
  forwards on its proxy stream.
- **Existing forward nodes are converted.**
  `POST /api/v4/kernel/nodes/forward-<id>/convert` (super administrators,
  `dry_run` first) creates the regular node (name, host, group, tags,
  machine fingerprint, approval; `show` 0; fresh credentials), copies the
  inventory row with its last reported engines, renames every route hop,
  port allocation and DNS node row, deletes the forward node (revoking its
  certificates) and replans, all in one transaction under the plan lock.
  Routes keep their ports. A refused plan, an Ansible host, or a network
  model that names the node refuses the conversion. The machine then
  reinstalls the Agent with the new node's install command; its hops are
  down until it connects. The forward Nodes page offers this as
  "Make a regular node".

## One node page

The console shows one node list (`/admin/nodes`). The forwarding area's own
node list (`/admin/forward/inventory`) is gone: it opens the node page's
转发主机 tab.

- **角色.** Each regular node shows 落地 when it has protocols and 中转 when
  it has none or carries hops of forward routes (both at once is fine), with
  its hop count and hop errors from the forwarding inventory.
- **转发主机.** The forwarding machines that are not regular nodes: Ansible
  hosts (添加 Ansible 主机 is here) and Agent forward nodes not yet converted
  (转为常规节点, and a banner on the 节点 tab while any is left). The tab is
  there only while the forward API answers.
- **转发设置.** A node in the forwarding inventory has 转发设置 in its row
  menu (`/admin/forward/inventory/<ref>`, ports, addresses and hosted hops);
  that page goes back to the node.

## Not yet

- Merging the tables (`v2_forward_node` remains for Ansible hosts and
  unconverted nodes).
- `v2_node.parent_id` relay children, which become network-model paths.
- Agents stop sending a node kind once no Agent forward node is left.
