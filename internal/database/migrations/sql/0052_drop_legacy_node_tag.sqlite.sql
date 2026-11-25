-- 0052_drop_legacy_node_tag: removes the "legacy-node" tag. It marked
-- the nodes from before the 5.0 Agent; every node now runs it, and the
-- node list showed the tag as if it were a node group. Only a node whose
-- only tag is this one changes.
UPDATE v5_node SET tags = NULL WHERE tags IN ('legacy-node', '["legacy-node"]');
