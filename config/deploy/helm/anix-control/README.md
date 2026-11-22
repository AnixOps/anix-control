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
  --set config.ANIX_CONTROL_DATABASE_HOST=postgres.example.internal
# On a fresh database: the one-time link that creates the first administrator.
kubectl -n anix exec deploy/control-anix-control -c control -- /app/anix-control admin claim-link
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
  keeps serving for 5 seconds after readiness fails on shutdown, within
  `terminationGracePeriodSeconds: 90`.
- Security: uid 10001, read-only root filesystem, all capabilities dropped,
  `RuntimeDefault` seccomp, an exec-capable `emptyDir` at `/tmp`.
- `replicaCount` must be 1 and the strategy is `Recreate`: background workers
  and plugin hosts are still single-instance.
- Services: `http` (8080: the web app, the API, subscriptions and `/install`
  on one port), plus an optional `grpc` Service for nodes (`grpc.enabled`).
  Optional Ingress and Prometheus Operator `ServiceMonitor`.
- Root key: Control's data directory is an in-memory `emptyDir` at
  `/var/lib/anix-control`; the CA key Secret below is the root key's lasting
  copy, adopted into `root.key` at every start. The chart refuses to render
  without one.
- Reverse proxy trust: `config.ANIX_CONTROL_SERVER_TRUSTED_PROXIES` lists the
  peers whose `X-Forwarded-*` headers Control honours (links in install
  scripts and subscriptions, the client IP). The default keeps loopback and
  the private ranges; narrow the private ranges
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

