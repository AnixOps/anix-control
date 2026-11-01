### Removed

- Node registration keys: `GET`, `POST /api/v2/admin/auth-keys` and
  `DELETE /api/v2/admin/auth-keys/:id` are gone and answer 404. Nothing used
  a key since node registration was removed. Migration 0025 drops
  `v2_authorized_key`, the `kapi_registration_key_v1` view and the keys'
  moved credentials. The KernelNodeOps kinds `regkey.issue` and
  `regkey.revoke` stay in the contract and answer `UNIMPLEMENTED`; the
  proxy-node package no longer holds `kernel.nodeops.credentials.v1`.
