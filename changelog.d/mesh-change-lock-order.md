### Fixed

- 组网: a region change request no longer locks its target region after the member or device, the order opposite to approvals and moves, so a request at the same time as a move no longer ends in a database error. Deleting a device or removing a node locks its row before its pending changes, in the order an approval takes them. A device's configuration that gives it an address uses the region as read under the lock.
