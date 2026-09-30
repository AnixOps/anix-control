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

## Network modules

A package can run as its own Deployment (a network module) instead of a child
process of Control; see `docs/architecture/module-runtime.md`. The chart runs
`identity-platform` this way:

```bash
# 1. Control with the module runtime: a 32-byte CA key in the Secret and
#    moduleRuntime.enabled. Control then listens for modules on port 7443
#    (mTLS) behind its Service.
kubectl -n anix patch secret anix-control --type merge \
  -p "{\"stringData\":{\"module_ca_kek\":\"$(openssl rand -base64 32)\"}}"
helm upgrade control config/deploy/helm/anix-control -n anix --reuse-values \
  --set secrets.files.module_ca_kek=ANIX_CONTROL_MODULE_RUNTIME_CA_KEK \
  --set moduleRuntime.enabled=true

# 2. Trust bundle and a reusable enrollment credential for the module pods.
control() { kubectl -n anix exec deploy/control-anix-control -c control -- /app/anix-control "$@"; }
control module ca bundle > ca.pem
kubectl -n anix create configmap anix-module-ca --from-file=ca.pem
control module token create -package identity-platform -reusable -ttl 8760h \
  | jq -r .credential > credential   # shown once
kubectl -n anix create secret generic anix-identity-enrollment --from-file=credential
rm credential

# 3. Switch the package to the remote runtime and start the module.
control module runtime set identity-platform remote
helm upgrade control config/deploy/helm/anix-control -n anix --reuse-values \
  --set modules.identity-platform.enabled=true \
  --set modules.identity-platform.image.repository=ghcr.io/anixops/anix-module-identity-platform \
  --set modules.identity-platform.image.digest=sha256:<digest> \
  --set modules.identity-platform.replicas=2 \
  --set modules.identity-platform.trustBundleConfigMap=anix-module-ca \
  --set modules.identity-platform.enrollmentSecret=anix-identity-enrollment
kubectl -n anix rollout restart deploy/control-anix-control   # applies the runtime switch
```

What the chart does for modules:

- **Deployment per module.**
  - Pods have their own labels (`app.kubernetes.io/name: anix-module`), so
    they never sit behind the Control Service.
  - Each pod enrolls with the reusable credential and keeps its certificates
    in an `emptyDir`; a new pod enrolls again.
  - The pod IP is the address Control dials, and the pod name is the instance
    ID.
- **Probes.** `/livez` and `/readyz` on port 8081. A pod is ready once it is
  bound to Control.
- **Security.** uid 65532, read-only root filesystem, all capabilities
  dropped, `RuntimeDefault` seccomp, no service account token.
- **NetworkPolicy** (`moduleRuntime.networkPolicy.enabled`):
  - ingress to the host port only from Control pods, plus probes;
  - egress only to Control's module port, DNS and PostgreSQL (narrowed by
    `databaseCIDR`).
- **Guards.** Rendering fails without a CA key mapping, or with a module
  enabled while `moduleRuntime` is off.

`config/scripts/modules_kind_smoke.sh` runs these steps on kind in CI: two
replicas, NetworkPolicy on, login through the module, and pod replacement.
