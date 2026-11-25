### Fixed

- 组网: a device or node deleted while it was being moved, renamed or given an address no longer comes back. Writes to devices, regions, host groups and names now refuse a row deleted meanwhile, and a node is moved only while it is still in the mesh. A region change request locks its target region first, so the region cannot be deleted while a request for it is written. A device's configuration read just as its region is disabled answers no network.
