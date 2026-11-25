### Fixed

- 组网: every change that gives an address reads its region under the region's lock, so a range changed at the same time can no longer leave a node, device or credential with an address of the old range; an enrollment by latency checks again that the chosen region is still enabled. A boundary node added while a region is created joins the new region too. A change that collided waits a short random time before it runs again. The name table's SHA-256 is checked by the tests, so the copy the node kernel uses cannot drift apart unnoticed.
