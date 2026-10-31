### Removed

- The `/api/v3` topology and deployment endpoints (`/topologies*`, `/deployments*`,
  `/observed-states`), the topology deployment executor, diagnosis and delete
  services. Agent plugin dispatch stays off until M4. The `v3_kernel_topology_*`
  tables are left in place; a later migration can drop them.
