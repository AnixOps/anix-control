### Fixed

- 组网: choosing a node's, a full node's or a device's address locks its region on PostgreSQL, so two changes at once can no longer give one address to two members; saving two full nodes' subnets at once takes turns, so overlapping subnets are refused. A full node credential keeps its address only until it expires; a region's new range clears the addresses its unused credentials keep, and deleting a region removes its unused credentials. The advertised subnets request is limited to 16 KiB.
