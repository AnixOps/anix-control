-- 0019_drop_load_balancer (SQLite): the load balancer feature is gone. It
-- had a CRUD page and statistics but nothing ever selected a node through
-- it, so v2_load_balancer (and its group index) is dropped with the model.
DROP TABLE IF EXISTS `v2_load_balancer`;
