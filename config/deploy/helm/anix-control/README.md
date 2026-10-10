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

Without `image.digest` or `image.tag` the chart runs
`ghcr.io/anixops/anix-control:<appVersion>`: `appVersion` in `Chart.yaml` is the
Control release this chart ships with. `prepare_release.py` sets it with the
other version surfaces and the release tag gate fails a stale one. The chart's
own `version` is a separate SemVer, bumped when a template or value changes.

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
- Reverse proxy trust: `config.ANIX_CONTROL_SERVER_TRUSTED_PROXIES` lists the
  peers whose `X-Forwarded-*` headers Control honours (links in install
  scripts and subscriptions, the client IP). The default keeps loopback (the
  UI server's `/api` proxy) and the private ranges; narrow the private ranges
  to the ingress controller's pod CIDR, and do not expose the Service
  directly while they are trusted (`docs/DEPLOYMENT.md` section 6.1).
- `ansible.existingSecret` mounts `inventory.ini` and SSH material for the
  local-ansible forward runtime.
- CA key: a fresh install creates `<fullname>-ca-kek` (see below), so Agents
  can enroll as soon as gRPC has TLS.

`values.yaml` documents every value.

## AnixOps Agents: the CA key and gRPC TLS

Agents enroll (AgentEnrollment) and connect over gRPC with TLS, and Control
signs their certificates with its built-in CA, sealed by
`module_runtime.ca_kek`.

- **CA key.** Without `caKek.existingSecret`, the chart creates the Secret
  `<fullname>-ca-kek` (key `ca_kek`, 32 random bytes, base64) on the first
  install, reads it back with `lookup` on every upgrade, marks it
  `immutable`, and keeps it on `helm uninstall`
  (`helm.sh/resource-policy: keep`). Back it up with the database and never
  change it: the CA is sealed with it, and with another key every Agent must
  enroll again. `helm template` and GitOps tools (Argo CD, Flux) render
  without `lookup`; give them a Secret you manage instead:

  ```bash
  kubectl -n anix create secret generic anix-control-ca-kek \
    --from-literal=ca_kek="$(openssl rand -base64 32)"
  # values: caKek.existingSecret: anix-control-ca-kek
  ```

  A key mapped with `secrets.files.<key>: ANIX_CONTROL_MODULE_RUNTIME_CA_KEK`
  (the 4.1 way) takes precedence, and the chart then creates none.
- **gRPC TLS.** Agents verify Control's certificate against their system CA
  roots and cannot be given a private CA, so use a publicly trusted
  certificate for the name they dial. With cert-manager and a Let's Encrypt
  issuer:

  ```yaml
  apiVersion: cert-manager.io/v1
  kind: Certificate
  metadata:
    name: control-grpc
    namespace: anix
  spec:
    secretName: control-grpc-tls
    dnsNames: [grpc.example.com]
    issuerRef: {kind: ClusterIssuer, name: letsencrypt}
  ```

  ```bash
  helm upgrade control config/deploy/helm/anix-control -n anix --reuse-values \
    --set grpc.enabled=true --set grpc.tls.secretName=control-grpc-tls \
    --set config.ANIX_CONTROL_AGENT_INSTALL_GRPC_TARGET=grpc.example.com:50051
  ```

  Point `grpc.example.com` at the `grpc` Service (a TCP load balancer: TLS
  passes through to Control). Control loads the certificate at start:
  restart it after cert-manager renews (`kubectl -n anix rollout restart
  deploy/control-anix-control`, or a reloader). `helm install` prints what is
  still missing.

## Network modules

A package can run as its own Deployment (a network module) instead of a child
process of Control; see `docs/architecture/module-runtime.md`. The chart runs
`identity-platform` this way:

```bash
# 1. Control with the module runtime (it uses the CA key above). Control then
#    listens for modules on port 7443 (mTLS) behind its Service.
helm upgrade control config/deploy/helm/anix-control -n anix --reuse-values \
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
- **Guards.** Rendering fails without a CA key (`caKek.generate=false` and
  no other source), or with a module enabled while `moduleRuntime` is off.

`config/scripts/modules_kind_smoke.sh` runs these steps on kind in CI: two
replicas, NetworkPolicy on, login through the module, and pod replacement.
