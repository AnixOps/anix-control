### Removed

- `sdk/forward/driver` (the engine driver interface, its fake driver and
  conformance suite) and `sdk/forward/leastconn`. Every engine driver was
  already gone and the Agent no longer imports either package (Agent #131):
  routes run on the network model.
