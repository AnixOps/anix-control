-- 0017_node_groups_make_regular_nodes: one machine is one node
-- (docs/design/node-unification). Every node group makes regular (proxy)
-- nodes now; a former forward group, which has no protocol template, makes
-- relays: nodes without protocols. The forward nodes such groups made stay
-- in them until they are converted.
UPDATE v5_node_group SET kind = 'proxy' WHERE kind = 'forward';
