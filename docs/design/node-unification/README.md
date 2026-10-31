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
- **No conversion.** An Agent forward node is not converted (the convert
  endpoint is gone): delete it and install the Agent again with a node
  group's command.

## One node page

The console shows one node list (`/admin/nodes`) with one kind of node: no
role column, no 转发主机 tab, no convert action. `/admin/forward/inventory`
opens the node page. Relaying runs on the network model (ANX).

## Not yet

- Merging the tables (`v2_forward_node` remains for Ansible hosts and
  Agent forward nodes from before rc.11).
- `v2_node.parent_id` relay children, which become network-model paths.
- Agents stop sending a node kind once no Agent forward node is left.
