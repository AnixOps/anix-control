#!/usr/bin/env bash
# Network module smoke test with the production Compose files.
#
# Starts Control with docker-compose.prod.yml + docker-compose.modules.yml and
# identity-platform as a separate container, then logs in through the v2 API:
# the request travels kernel gateway -> remote identity module over mTLS ->
# package bridge -> kernel. The identity package is built and signed with a
# throwaway key that the kernel is told to trust. It then hands logins to the
# identity module (import, cutover), checks native login, revocation and
# logout, rolls back, cuts over again and finalizes.
#
# Required environment:
#   CONTROL_IMAGE     Control image (for example the Dockerfile "source" target)
#   IDENTITY_IMAGE    identity-platform module image (Dockerfile.module)
#   DB_HOST DB_PORT DB_USER DB_PASSWORD DB_NAME
#                     an empty PostgreSQL database reachable from containers
set -Eeuo pipefail

: "${CONTROL_IMAGE:?}" "${IDENTITY_IMAGE:?}" "${DB_HOST:?}" "${DB_PORT:?}" "${DB_USER:?}" "${DB_PASSWORD:?}" "${DB_NAME:?}"
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
work="$(mktemp -d)"
# grep -q exits at the first match; under pipefail the writer then dies of
# SIGPIPE and fails the pipeline, so match against captured output instead.
contains() { grep -q -- "$2" <<<"$1"; }
sudo_install() { if [[ "$(id -u)" == 0 ]]; then install "$@"; else sudo install "$@"; fi; }

cleanup() {
  status=$?
  if [[ -f "${work}/docker-compose.prod.yml" ]]; then
    (cd "${work}" && "${compose[@]}" logs --no-color 2>/dev/null | tail -80) || true
    [[ -n "${SMOKE_KEEP_WORK:-}" ]] || (cd "${work}" && "${compose[@]}" down -v >/dev/null 2>&1) || true
  fi
  if [[ -n "${SMOKE_KEEP_WORK:-}" ]]; then
    echo "kept ${work}"
  elif [[ "$(id -u)" == 0 ]]; then rm -rf "${work}"; else sudo rm -rf "${work}"; fi
  exit "${status}"
}
trap cleanup EXIT

echo "== signed identity package (throwaway trust root)"
openssl genpkey -algorithm ed25519 -out "${work}/signing.pem" 2>/dev/null
public_key="$(openssl pkey -in "${work}/signing.pem" -pubout -outform DER 2>/dev/null | tail -c 32 | base64)"
mkdir -p "${work}/package" "${work}/bootstrap"
GOWORK=off python3 "${repo_root}/packages/shared/build_package.py" --package identity-platform --version 4.0.0 \
  --out "${work}/package" --signing-key "${work}/signing.pem" >/dev/null
for suffix in anxp manifest.json manifest.sig; do
  install -m 0644 "${work}/package/identity-platform-4.0.0.${suffix}" "${work}/bootstrap/"
done

cp "${repo_root}/docker-compose.prod.yml" "${repo_root}/docker-compose.modules.yml" "${work}/"
cat > "${work}/control.env" <<EOF
ANIX_CONTROL_DATABASE_DRIVER=postgres
ANIX_CONTROL_DATABASE_HOST=${DB_HOST}
ANIX_CONTROL_DATABASE_PORT=${DB_PORT}
ANIX_CONTROL_DATABASE_USERNAME=${DB_USER}
ANIX_CONTROL_DATABASE_DATABASE=${DB_NAME}
ANIX_CONTROL_DATABASE_SSLMODE=disable
ANIX_CONTROL_LOG_FORMAT=json
ANIX_CONTROL_ADMIN_EMAIL=admin@example.test
ANIX_CONTROL_ADMIN_PASSWORD=ModuleSmoke-0123456789
ANIX_CONTROL_PLUGINS_OFFICIAL_PUBLIC_KEY=${public_key}
ANIX_CONTROL_PLUGINS_IDENTITY_BOOTSTRAP_PACKAGE_DIR=/app/bootstrap/identity-platform
EOF
# The test package is mounted where the release image carries the signed one.
cat > "${work}/docker-compose.smoke.yml" <<'EOF'
services:
  migrate:
    volumes:
      - ./bootstrap:/app/bootstrap/identity-platform:ro
  control:
    volumes:
      - ./bootstrap:/app/bootstrap/identity-platform:ro
EOF
install -d -m 0755 "${work}/secrets"
openssl rand -hex 32 | sudo_install -m 0400 -o 10001 -g 10001 /dev/stdin "${work}/secrets/jwt_secret"
printf '%s' "${DB_PASSWORD}" | sudo_install -m 0400 -o 10001 -g 10001 /dev/stdin "${work}/secrets/db_password"
openssl rand -base64 32 | sudo_install -m 0400 -o 10001 -g 10001 /dev/stdin "${work}/secrets/module_ca_kek"
openssl rand -base64 32 | sudo_install -m 0400 -o 65532 -g 65532 /dev/stdin "${work}/secrets/identity_kek"

cd "${work}"
export ANIX_CONTROL_IMAGE="${CONTROL_IMAGE}" ANIX_MODULE_IDENTITY_IMAGE="${IDENTITY_IMAGE}"
compose=(docker compose -f docker-compose.prod.yml -f docker-compose.modules.yml -f docker-compose.smoke.yml)
"${compose[@]}" config --quiet

echo "== bootstrap: schema, module CA, trust bundle, enrollment credential, remote runtime"
"${compose[@]}" run --rm -T migrate >/dev/null 2>&1
"${compose[@]}" run --rm -T --no-deps migrate module ca bundle 2>/dev/null > "${work}/bundle.pem"
grep -q "BEGIN CERTIFICATE" "${work}/bundle.pem"
sudo_install -m 0444 "${work}/bundle.pem" "${work}/secrets/module_ca_bundle"
"${compose[@]}" run --rm -T --no-deps migrate module token create -package identity-platform -reusable -ttl 24h 2>/dev/null \
  | python3 -c 'import json,sys; print(json.load(sys.stdin)["credential"])' \
  | sudo_install -m 0400 -o 65532 -g 65532 /dev/stdin "${work}/secrets/identity_enrollment"
contains "$("${compose[@]}" run --rm -T --no-deps migrate module runtime set identity-platform remote 2>/dev/null)" '"runtime": "remote"'

echo "== start Control and the identity module"
"${compose[@]}" up -d
login() {
  curl -fsS -H 'Content-Type: application/json' \
    -d '{"email":"admin@example.test","password":"ModuleSmoke-0123456789"}' \
    http://127.0.0.1:8080/api/v2/login
}
logged_in=0
for _ in $(seq 1 60); do
  if response="$(login 2>/dev/null)" && grep -q '"token"' <<<"${response}"; then logged_in=1; break; fi
  sleep 3
done
test "${logged_in}" = 1 || { echo "login through the remote identity module failed: ${response:-}"; exit 1; }
contains "$("${compose[@]}" logs control 2>/dev/null)" 'serving from remote instances'
contains "$("${compose[@]}" logs identity 2>/dev/null)" 'bound to generation'
echo "login through the remote identity module: ok"

echo "== restart the module: it reuses its certificate and binds again"
"${compose[@]}" restart identity >/dev/null
logged_in=0
for _ in $(seq 1 40); do
  if response="$(login 2>/dev/null)" && grep -q '"token"' <<<"${response}"; then logged_in=1; break; fi
  sleep 3
done
test "${logged_in}" = 1 || { echo "login after the module restart failed: ${response:-}"; exit 1; }

echo "== identity cutover: import, cutover, native login, revocation, logout, rollback, finalize"
base=http://127.0.0.1:8080
# call METHOD PATH TOKEN [BODY]: prints the HTTP status; the body lands in ${work}/body.
call() {
  local args=(-sS -o "${work}/body" -w '%{http_code}' -X "$1" -H 'Content-Type: application/json')
  [[ -n "$3" ]] && args+=(-H "Authorization: Bearer $3")
  [[ -n "${4:-}" ]] && args+=(-d "$4")
  curl "${args[@]}" "${base}$2"
}
# field EXPR: evaluates EXPR against the last body, bound to d.
field() { python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); print(eval(sys.argv[2]))' "${work}/body" "$1"; }
token_alg() { python3 -c 'import base64,json,sys; h=sys.argv[1].split(".")[0]; print(json.loads(base64.urlsafe_b64decode(h+"="*(-len(h)%4)))["alg"])' "$1"; }
login_as() {
  test "$(call POST /api/v2/login "" "{\"email\":\"$1\",\"password\":\"$2\"}")" = 200
  test "$(field 'd["code"]')" = 0 || { cat "${work}/body"; return 1; }
  field 'd["data"]["token"]'
}
# await EXPR: polls GET /api/v4/kernel/identity until EXPR holds.
await() {
  for _ in $(seq 1 60); do
    if [[ "$(call GET /api/v4/kernel/identity "${admin}")" == 200 && "$(field "$1")" == True ]]; then return 0; fi
    sleep 2
  done
  echo "identity status never satisfied $1:"; cat "${work}/body"; return 1
}
admin_password=ModuleSmoke-0123456789
admin="$(login_as admin@example.test "${admin_password}")"
test "$(token_alg "${admin}")" = HS256

test "$(call POST /api/v4/kernel/identity/import "${admin}" '{}')" = 202
await 'd["data"]["import"]["completed_at"] > 0 and d["data"]["state"] == "importing"'
test "$(call POST /api/v4/kernel/identity/cutover "${admin}")" = 202
await 'd["data"]["authority_change"] is not None and not d["data"]["authority_change"]["running"]'
test "$(field 'd["data"]["authority_change"].get("error", "")')" = "" || { cat "${work}/body"; exit 1; }
test "$(field 'd["data"]["state"]')" = identity
echo "cutover: ok"

native="$(login_as admin@example.test "${admin_password}")"
test "$(token_alg "${native}")" = EdDSA
test "$(call GET /api/v2/user/profile "${native}")" = 200
test "$(call GET /api/v2/user/profile "${admin}")" = 200 # legacy tokens work until finalize
echo "native login with an identity token: ok"

test "$(call POST /api/v2/admin/users "${native}" '{"email":"member@example.test","password":"Member-0123456789"}')" = 200
member_id="$(field 'd["data"]["id"]')"
member="$(login_as member@example.test Member-0123456789)"
test "$(call GET /api/v2/user/profile "${member}")" = 200
test "$(call POST "/api/v2/admin/users/${member_id}/ban" "${native}")" = 200
revoked=0
for _ in $(seq 1 5); do
  if [[ "$(call GET /api/v2/user/profile "${member}")" == 401 ]]; then revoked=1; break; fi
  sleep 1
done
test "${revoked}" = 1 || { echo "a banned member's token still works"; exit 1; }
test "$(call POST "/api/v2/admin/users/${member_id}/unban" "${native}")" = 200
echo "ban revokes within 5 seconds: ok"

session="$(login_as admin@example.test "${admin_password}")"
test "$(call POST /api/v4/identity/logout "${session}")" = 200
test "$(call GET /api/v2/user/profile "${session}")" = 401
test "$(call GET /api/v2/user/profile "${native}")" = 200
echo "logout ends one session: ok"

test "$(call POST /api/v4/kernel/identity/rollback "${native}")" = 202
await 'd["data"]["state"] == "importing" and not d["data"]["authority_change"]["running"]'
legacy="$(login_as member@example.test Member-0123456789)"
test "$(token_alg "${legacy}")" = HS256
echo "rollback: the natively created member logs in through the legacy handler: ok"

test "$(call POST /api/v4/kernel/identity/cutover "${admin}")" = 202
await 'd["data"]["state"] == "identity" and not d["data"]["authority_change"]["running"]'
test "$(call POST /api/v4/kernel/identity/finalize "${native}" '{"force":true}')" = 200
test "$(call GET /api/v2/user/profile "${admin}")" = 401 # HS256 is refused after finalize
test "$(token_alg "$(login_as member@example.test Member-0123456789)")" = EdDSA
echo "finalize: ok"
echo "modules compose smoke: ok"
