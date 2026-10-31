-- 0020_node_groups_without_templates: a node group only owns a join token
-- and the defaults of its nodes. The node types are gone (every group makes
-- the same kind of node) and a joining node stays clean, so the group's
-- protocol template and the subscription groups it joined are dropped.
ALTER TABLE v5_node_group DROP COLUMN kind;
ALTER TABLE v5_node_group DROP COLUMN protocol_template;
ALTER TABLE v5_node_group DROP COLUMN subscription_group_ids;
