### Fixed

- 组网: renaming a device while it moves to another region or gets a new address no longer writes back its old region and address; each change writes only its own fields. A full node's new address locks the device before its full node record, in the order a delete takes them. A host group or name whose gateway is removed at the same time is refused instead of keeping a gateway that left the mesh.
