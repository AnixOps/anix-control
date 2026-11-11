### Removed

- Five v2 administrator routes no client called, with their native and
  legacy handlers and tests: `GET` and `PUT /api/v2/admin/nodes/:id/raw-config`
  and `POST /api/v2/admin/nodes/validate-config` (the node update route
  carries the raw configuration), `POST /api/v2/admin/agent/execute` (it was
  `POST /api/v2/admin/agent/tasks` without the `type` field) and
  `GET /api/v2/admin/subscription/formats`.
