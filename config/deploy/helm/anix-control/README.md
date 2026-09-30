# anix-control Helm chart

Runs AnixOps Control as a single replica on Kubernetes with an external
PostgreSQL. The image is the released `ghcr.io/anixops/anix-control`, which
needs no config file: settings are `ANIX_CONTROL_*` variables.

```bash
kubectl create namespace anix
kubectl -n anix create secret generic anix-control \
  --from-literal=jwt_secret="$(openssl rand -hex 32)" \
  --from-literal=db_password='database-password'
helm install control config/deploy/helm/anix-control -n anix \
  --set secrets.existingSecret=anix-control \
  --set image.digest=sha256:<digest from docker-image.txt> \
  --set config.ANIX_CONTROL_DATABASE_HOST=postgres.example.internal \
  --set config.ANIX_CONTROL_ADMIN_EMAIL=admin@example.com
kubectl -n anix logs deploy/control-anix-control -c migrate   # generated admin password
```

What the chart does:

- `migrate` init container: `anix-control migrate` (schema and seeds under a
  PostgreSQL advisory lock) before the server starts. An init container rather
  than a Helm hook, so it always sees the release's Secret and ConfigMap.
- Probes: startup and liveness on `/livez`, readiness on `/readyz`; the server
  keeps serving for `ANIX_CONTROL_SERVER_SHUTDOWN_DRAIN_DELAY` after readiness
  fails on shutdown, within `terminationGracePeriodSeconds: 90`.
- Security: uid 10001, read-only root filesystem, all capabilities dropped,
  `RuntimeDefault` seccomp, an exec-capable `emptyDir` at `/tmp`.
- `replicaCount` must be 1 and the strategy is `Recreate`: background workers
  and plugin hosts are still single-instance.
- Services: `http` (8080) and `web` (3000; the UI server also proxies `/api`),
  plus an optional `grpc` Service for nodes (`grpc.enabled`). Optional Ingress
  and Prometheus Operator `ServiceMonitor`.
- `ansible.existingSecret` mounts `inventory.ini` and SSH material for the
  local-ansible forward runtime.

`values.yaml` documents every value.
