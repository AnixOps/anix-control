#!/usr/bin/env bash
# Network module smoke test with the Helm chart on a kind cluster.
#
# Installs Control with the module runtime, bootstraps the module PKI from the
# Control pod, switches identity-platform to the remote runtime and runs it as
# its own Deployment (two replicas, NetworkPolicy on), then logs in through the
# v2 API. Deleting the module pods checks that new pods enroll again with the
# reusable credential. The identity package is signed with a throwaway key and
# baked into a test Control image.
#
# Required environment:
#   CONTROL_IMAGE   Control image built locally (the Dockerfile "source" target)
#   IDENTITY_IMAGE  identity-platform module image, repository:tag (Dockerfile.module)
#   KIND_CLUSTER    kind cluster name
#   NAMESPACE       namespace with a PostgreSQL Service "postgresql" (user
#                   anix_control) holding the empty database DB_NAME
#   DB_PASSWORD DB_NAME
set -Eeuo pipefail

: "${CONTROL_IMAGE:?}" "${IDENTITY_IMAGE:?}" "${KIND_CLUSTER:?}" "${NAMESPACE:?}" "${DB_PASSWORD:?}" "${DB_NAME:?}"
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
chart="${repo_root}/config/deploy/helm/anix-control"
release=modules
fullname="${release}-anix-control"
module_selector="app.kubernetes.io/name=anix-module,app.kubernetes.io/instance=${release}"
work="$(mktemp -d)"
port_forward=""
k() { kubectl -n "${NAMESPACE}" "$@"; }
# grep -q exits at the first match; under pipefail the writer then dies of
# SIGPIPE and fails the pipeline, so match against captured output instead.
contains() { grep -q -- "$2" <<<"$1"; }

cleanup() {
  status=$?
  if [[ -n "${port_forward}" ]]; then kill "${port_forward}" 2>/dev/null || true; fi
  if [[ "${status}" != 0 ]]; then
    k get pods -o wide || true
    k logs "deployment/${fullname}" -c control --tail=80 || true
    k logs -l "${module_selector}" --prefix --tail=80 || true
  fi
  rm -rf "${work}"
  exit "${status}"
}
trap cleanup EXIT

echo "== test Control image with a signed identity package (throwaway trust root)"
openssl genpkey -algorithm ed25519 -out "${work}/signing.pem" 2>/dev/null
public_key="$(openssl pkey -in "${work}/signing.pem" -pubout -outform DER 2>/dev/null | tail -c 32 | base64)"
mkdir -p "${work}/package" "${work}/bootstrap"
GOWORK=off python3 "${repo_root}/packages/shared/build_package.py" --package identity-platform --version 4.0.0 \
  --out "${work}/package" --signing-key "${work}/signing.pem" >/dev/null
for suffix in anxp manifest.json manifest.sig; do
  install -m 0644 "${work}/package/identity-platform-4.0.0.${suffix}" "${work}/bootstrap/"
done
# docker commit rather than docker build: a Buildx container builder cannot
# see the locally loaded CONTROL_IMAGE.
container="$(docker create "${CONTROL_IMAGE}")"
docker cp "${work}/bootstrap" "${container}:/app/bootstrap"
docker commit --change 'ENV ANIX_CONTROL_PLUGINS_IDENTITY_BOOTSTRAP_PACKAGE_DIR=/app/bootstrap' \
  "${container}" anix-control:modules-smoke >/dev/null
docker rm "${container}" >/dev/null
kind load docker-image anix-control:modules-smoke "${IDENTITY_IMAGE}" --name "${KIND_CLUSTER}"

echo "== install Control with the module runtime"
cat > "${work}/values.yaml" <<EOF
image:
  repository: anix-control
  tag: modules-smoke
  pullPolicy: Never
config:
  ANIX_CONTROL_DATABASE_DATABASE: ${DB_NAME}
  ANIX_CONTROL_ADMIN_EMAIL: admin@example.test
  ANIX_CONTROL_PLUGINS_OFFICIAL_PUBLIC_KEY: ${public_key}
secrets:
  files:
    admin_password: ANIX_CONTROL_ADMIN_PASSWORD
    module_ca_kek: ANIX_CONTROL_MODULE_RUNTIME_CA_KEK
  values:
    jwt_secret: $(openssl rand -hex 32)
    db_password: ${DB_PASSWORD}
    admin_password: ModuleSmoke-0123456789
    module_ca_kek: $(openssl rand -base64 32)
moduleRuntime:
  enabled: true
  networkPolicy:
    enabled: true
EOF
helm install "${release}" "${chart}" --namespace "${NAMESPACE}" -f "${work}/values.yaml" --wait --timeout 6m >/dev/null

echo "== bootstrap: trust bundle, enrollment credential, remote runtime"
control() { k exec "deployment/${fullname}" -c control -- /app/anix-control "$@"; }
control module ca bundle 2>/dev/null > "${work}/ca.pem"
grep -q "BEGIN CERTIFICATE" "${work}/ca.pem"
k create configmap anix-module-ca --from-file=ca.pem="${work}/ca.pem" >/dev/null
(umask 077 && control module token create -package identity-platform -reusable -ttl 24h 2>/dev/null \
  | python3 -c 'import json,sys; sys.stdout.write(json.load(sys.stdin)["credential"])' > "${work}/credential")
k create secret generic anix-identity-enrollment --from-file=credential="${work}/credential" >/dev/null
contains "$(control module runtime set identity-platform remote 2>/dev/null)" '"runtime": "remote"'

echo "== run identity-platform as its own Deployment"
helm upgrade "${release}" "${chart}" --namespace "${NAMESPACE}" --reuse-values \
  --set modules.identity-platform.enabled=true \
  --set modules.identity-platform.image.repository="${IDENTITY_IMAGE%:*}" \
  --set modules.identity-platform.image.tag="${IDENTITY_IMAGE##*:}" \
  --set modules.identity-platform.image.pullPolicy=Never \
  --set modules.identity-platform.replicas=2 \
  --set modules.identity-platform.trustBundleConfigMap=anix-module-ca \
  --set modules.identity-platform.enrollmentSecret=anix-identity-enrollment >/dev/null
# The runtime choice applies when Control next starts the package.
k rollout restart "deployment/${fullname}" >/dev/null
k rollout status "deployment/${fullname}" --timeout=6m
k rollout status "deployment/${fullname}-module-identity-platform" --timeout=6m

k port-forward "svc/${fullname}" 18090:8080 >"${work}/port-forward.log" 2>&1 &
port_forward=$!
login() {
  curl -fsS -H 'Content-Type: application/json' \
    -d '{"email":"admin@example.test","password":"ModuleSmoke-0123456789"}' \
    http://127.0.0.1:18090/api/v2/login
}
wait_for_login() {
  for _ in $(seq 1 60); do
    if response="$(login 2>/dev/null)" && grep -q '"token"' <<<"${response}"; then return 0; fi
    sleep 3
  done
  echo "login through the remote identity module failed: ${response:-}"
  return 1
}
wait_for_login
contains "$(k logs "deployment/${fullname}" -c control)" 'serving from remote instances'
test "$(k logs -l "${module_selector}" --tail=-1 | grep -c 'bound to generation')" -ge 2
echo "login through two remote identity pods: ok"

echo "== replace the module pods: new pods enroll again and bind"
k delete pod -l "${module_selector}" --wait=true >/dev/null
k rollout status "deployment/${fullname}-module-identity-platform" --timeout=6m
wait_for_login
helm uninstall "${release}" --namespace "${NAMESPACE}" --wait >/dev/null
echo "modules kind smoke: ok"
