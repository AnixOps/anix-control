#!/usr/bin/env bash
# Identity cutover acceptance against a running Control with identity-platform
# (local or remote) and an empty user base apart from the administrator.
# It imports the accounts, hands logins to identity, checks native login, a
# ban and a demotion revoking within 5 seconds, and logout, rolls back (a natively created
# user logs in through the legacy handler), cuts over again and finalizes
# (legacy HS256 tokens are then refused). Finalize cannot be undone: run it
# against throwaway databases only.
#
# Required environment:
#   BASE_URL                    Control's HTTP address, e.g. http://127.0.0.1:8080
#   ADMIN_EMAIL ADMIN_PASSWORD  the administrator Control created
# On success it leaves ${IDENTITY_TOKEN_FILE} (if set) holding a valid
# identity token of the administrator.
set -Eeuo pipefail

: "${BASE_URL:?}" "${ADMIN_EMAIL:?}" "${ADMIN_PASSWORD:?}"
work="$(mktemp -d)"
trap 'rm -rf "${work}"' EXIT

echo "== identity cutover: import, cutover, native login, revocation, logout, rollback, finalize"
# call METHOD PATH TOKEN [BODY]: prints the HTTP status; the body lands in ${work}/body.
call() {
  local args=(-sS -o "${work}/body" -w '%{http_code}' -X "$1" -H 'Content-Type: application/json')
  [[ -n "$3" ]] && args+=(-H "Authorization: Bearer $3")
  [[ -n "${4:-}" ]] && args+=(-d "$4")
  curl "${args[@]}" "${BASE_URL}$2"
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
admin="$(login_as "${ADMIN_EMAIL}" "${ADMIN_PASSWORD}")"
test "$(token_alg "${admin}")" = HS256

test "$(call POST /api/v4/kernel/identity/import "${admin}" '{}')" = 202
await 'd["data"]["import"]["completed_at"] > 0 and d["data"]["state"] == "importing"'
test "$(call POST /api/v4/kernel/identity/cutover "${admin}")" = 202
await 'd["data"]["authority_change"] is not None and not d["data"]["authority_change"]["running"]'
test "$(field 'd["data"]["authority_change"].get("error", "")')" = "" || { cat "${work}/body"; exit 1; }
test "$(field 'd["data"]["state"]')" = identity
echo "cutover: ok"

native="$(login_as "${ADMIN_EMAIL}" "${ADMIN_PASSWORD}")"
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

test "$(call POST /api/v2/admin/users "${native}" '{"email":"ops@example.test","password":"Ops-0123456789","is_admin":1}')" = 200
ops_id="$(field 'd["data"]["id"]')"
ops="$(login_as ops@example.test Ops-0123456789)"
test "$(call GET /api/v4/kernel/identity "${ops}")" = 200
test "$(call PUT "/api/v2/admin/users/${ops_id}" "${native}" '{"is_admin":0}')" = 200
test "$(field 'd["code"]')" = 0 || { cat "${work}/body"; exit 1; }
demoted=0
for _ in $(seq 1 5); do
  if [[ "$(call GET /api/v4/kernel/identity "${ops}")" == 401 ]]; then demoted=1; break; fi
  sleep 1
done
test "${demoted}" = 1 || { echo "a demoted administrator's token still works"; exit 1; }
# Revocations have one-second precision: a token issued in the revocation's
# second is revoked too, so log in again from the next second on.
sleep 2
test "$(call GET /api/v4/kernel/identity "$(login_as ops@example.test Ops-0123456789)")" = 403
echo "demotion revokes and removes administrator access: ok"

session="$(login_as "${ADMIN_EMAIL}" "${ADMIN_PASSWORD}")"
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
if [[ -n "${IDENTITY_TOKEN_FILE:-}" ]]; then
  (umask 077 && printf '%s' "${native}" > "${IDENTITY_TOKEN_FILE}")
fi
