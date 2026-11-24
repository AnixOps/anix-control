-- 0043_lease_generation (SQLite): the fencing token of a lease
-- (backend review ARC-003). Raised each time a lease changes holder; leader
-- work checks it in the transaction of its writes (internal/lease Check).
-- Additive; existing leases start at 0 and are raised at their next takeover.
ALTER TABLE `v4_kernel_lease` ADD `generation` integer NOT NULL DEFAULT 0;
