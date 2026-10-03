#!/usr/bin/env bash
# Local staging rehearsal of route cutovers (legacy -> shadow -> native).
#
# Runbook: docs/guide/staging-rehearsal.md. Synthetic data only (H4): this
# script never reads a backup or production database and its containers have
# no route out of the host.
#
#   scripts/staging/rehearse.sh up                  build, start, install packages, seed
#   scripts/staging/rehearse.sh --batch 1 [--smoke] rehearse one batch, write its report
#   scripts/staging/rehearse.sh --batch 1 --rollback
#   scripts/staging/rehearse.sh routes list --package knowledge
#   scripts/staging/rehearse.sh status | logs [service] | down
#
# A batch run switches the batch's native-flagged read routes to shadow on the
# rehearsal instance, replays read traffic until every route has
# --min-requests compared requests and --min-shadow-duration in shadow,
# runs the batch's packagecompat parity suites, replays the write routes on
# two throwaway twins (legacy and native, identical database copies) and
# compares their tables, then writes reports/batch-N/report.{md,json}. It
# never switches the rehearsal instance to native: the report prints the
# command for that, to run after the owner signs the batch off (H7).
set -Eeuo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
export STAGING_WORK="${STAGING_WORK:-${repo_root}/scripts/staging/.work}"
export STAGING_PROJECT="${STAGING_PROJECT:-anix-staging}"
compose_file="${repo_root}/scripts/staging/compose.yml"
version="${STAGING_PACKAGE_VERSION:-$(sed -n 's/^\s*DefaultVersion\s*=\s*"\(.*\)"/\1/p' "${repo_root}/internal/branding/branding.go")}"
gost_version=3.2.6

batch=""
smoke=0
rollback=0
min_requests=200
min_duration=2h
rate=0
parity=1
reconcile=1
command=""
args=()

usage() { sed -n '2,22p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; }
die() { echo "rehearse: $*" >&2; exit 1; }

while [[ $# -gt 0 ]]; do
  case "$1" in
    --batch) batch="${2:?}"; shift 2 ;;
    --smoke) smoke=1; shift ;;
    --rollback) rollback=1; shift ;;
    --min-requests) min_requests="${2:?}"; shift 2 ;;
    --min-shadow-duration) min_duration="${2:?}"; shift 2 ;;
    --rate) rate="${2:?}"; shift 2 ;;
    --skip-parity) parity=0; shift ;;
    --skip-reconcile) reconcile=0; shift ;;
    -h|--help) usage; exit 0 ;;
    up|down|status|logs|routes|seed-reset)
      command="$1"; shift; args=("$@"); break ;;
    *) die "unknown argument $1 (see --help)" ;;
  esac
done
if [[ -z "${command}" && -n "${batch}" ]]; then command=batch; fi
[[ -n "${command}" ]] || { usage; exit 2; }
if [[ "${smoke}" == 1 ]]; then
  min_requests=20
  min_duration=2m
fi

compose() { docker compose -p "${STAGING_PROJECT}" -f "${compose_file}" "$@"; }
tool() { compose --profile tool run --rm -T tool "$@"; }
psql_staging() { compose exec -T postgres psql -v ON_ERROR_STOP=1 -U staging -d staging -qAt "$@"; }

load_env() {
  [[ -f "${STAGING_WORK}/staging.env" ]] || die "no stack: run scripts/staging/rehearse.sh up first"
  set -a
  # shellcheck disable=SC1091
  source "${STAGING_WORK}/staging.env"
  set +a
  export STAGING_UID STAGING_GID
  STAGING_UID="$(id -u)" STAGING_GID="$(id -g)"
}

disk_check() {
  local used
  used="$(df --output=pcent "${STAGING_WORK%/*}" | tail -1 | tr -dc 0-9)"
  if (( used >= 95 )); then die "disk is ${used}% full: stopping before it fills up"; fi
}

# package_inputs prints the builder's Agent and runtime inputs for the named
# packages. Agent-side binaries are never run by the rehearsal (no agent
# connects): a stub stands in for them. gost-mesh embeds the pinned GOST
# runtime, which the builder checks byte for byte, so it is downloaded and
# checked against runtime-contract.json.
package_inputs() {
  for id in "$@"; do
    case "${id}" in
      machine-telemetry|nftables-forward|nat-egress)
        printf -- '--agent-binary %s@linux/amd64=%s ' "${id}" "${STAGING_WORK}/cache/stub-agent" ;;
      gost-mesh)
        local gost="${STAGING_WORK}/cache/gost-${gost_version}"
        if [[ ! -x "${gost}" ]]; then
          local archive="${STAGING_WORK}/cache/gost.tar.gz"
          curl -fsSL --retry 3 -o "${archive}" \
            "https://github.com/go-gost/gost/releases/download/v${gost_version}/gost_${gost_version}_linux_amd64.tar.gz"
          echo "b39037b0380ea001fb3c0c28441c2e10bfc694f90682739a65b53e55dce5238b  ${archive}" | sha256sum --check --strict --quiet >&2
          tar -xzf "${archive}" -C "${STAGING_WORK}/cache" gost
          mv "${STAGING_WORK}/cache/gost" "${gost}"
          rm -f "${archive}"
        fi
        printf -- '--agent-binary %s@linux/amd64=%s --runtime-binary gost-mesh:gost@linux/amd64=%s ' \
          "${id}" "${STAGING_WORK}/cache/stub-agent" "${gost}" ;;
    esac
  done
}

build() {
  mkdir -p "${STAGING_WORK}"/{bin,packages,bootstrap,cache,reports}
  echo "== build Control and stagingctl (static, from this checkout)"
  (cd "${repo_root}" && CGO_ENABLED=0 GOWORK=off go build -trimpath -ldflags "-s -w" \
      -o "${STAGING_WORK}/bin/anix-control" ./cmd/server)
  (cd "${repo_root}" && CGO_ENABLED=0 GOWORK=off go build -trimpath \
      -o "${STAGING_WORK}/bin/stagingctl" ./scripts/staging/stagingctl)
  "${STAGING_WORK}/bin/stagingctl" batches >/dev/null

  echo "== build and sign every package (commercial edition, throwaway key)"
  if [[ ! -f "${STAGING_WORK}/signing.pem" ]]; then
    openssl genpkey -algorithm ed25519 -out "${STAGING_WORK}/signing.pem" 2>/dev/null
    chmod 0600 "${STAGING_WORK}/signing.pem"
  fi
  printf '#!/bin/sh\necho "staging stub agent: not for nodes" >&2\nexit 1\n' > "${STAGING_WORK}/cache/stub-agent"
  chmod 0755 "${STAGING_WORK}/cache/stub-agent"
  rm -f "${STAGING_WORK}/packages/"*
  local selected=()
  if [[ -n "${STAGING_PACKAGES:-}" ]]; then
    IFS=, read -r -a selected <<<"identity-platform,${STAGING_PACKAGES}"
  fi
  if [[ ${#selected[@]} -eq 0 ]]; then
    # shellcheck disable=SC2046
    GOWORK=off python3 "${repo_root}/packages/shared/build_package.py" --all --edition commercial \
      --version "${version}" --out "${STAGING_WORK}/packages" --signing-key "${STAGING_WORK}/signing.pem" \
      $(package_inputs machine-telemetry nftables-forward gost-mesh nat-egress) >/dev/null
  else
    for id in "${selected[@]}"; do
      # shellcheck disable=SC2046
      GOWORK=off python3 "${repo_root}/packages/shared/build_package.py" --package "${id}" \
        --version "${version}" --out "${STAGING_WORK}/packages" --signing-key "${STAGING_WORK}/signing.pem" \
        $(package_inputs "${id}") >/dev/null
    done
  fi
  rm -f "${STAGING_WORK}/packages/"*.sbom.spdx.json "${STAGING_WORK}/packages/"*.public-key.pem
  rm -f "${STAGING_WORK}/bootstrap/"*
  for suffix in anxp manifest.json manifest.sig; do
    install -m 0644 "${STAGING_WORK}/packages/identity-platform-${version}.${suffix}" "${STAGING_WORK}/bootstrap/"
  done
  chmod 0755 "${STAGING_WORK}/bootstrap"
  echo "built $(ls "${STAGING_WORK}/packages/"*.anxp | wc -l) packages at ${version}"
}

write_env() {
  [[ -f "${STAGING_WORK}/staging.env" ]] && return 0
  local public_key
  public_key="$(openssl pkey -in "${STAGING_WORK}/signing.pem" -pubout -outform DER 2>/dev/null | tail -c 32 | base64)"
  umask 077
  cat > "${STAGING_WORK}/staging.env" <<EOF
STAGING_DB_PASSWORD=$(openssl rand -hex 16)
STAGING_JWT_SECRET=$(openssl rand -hex 32)
STAGING_ADMIN_PASSWORD=Stg-$(openssl rand -hex 12)
STAGING_PACKAGE_PUBLIC_KEY=${public_key}
STAGING_SEED=${STAGING_SEED:-20261002}
EOF
}

cmd_up() {
  disk_check
  build
  write_env
  load_env
  echo "== start PostgreSQL and Control (internal network, no route out)"
  compose up -d postgres migrate control
  tool wait --urls http://control:8080 --timeout 4m
  echo "== install and enable every package"
  tool install --packages /work/packages
  if [[ "$(psql_staging -c "SELECT count(*) FROM v2_user")" -le 1 ]]; then
    echo "== seed synthetic data (seed ${STAGING_SEED})"
    tool seed
    echo "== snapshot the seeded database as template staging_seed"
    compose stop control >/dev/null
    psql_staging -c "DROP DATABASE IF EXISTS staging_seed" -c "CREATE DATABASE staging_seed TEMPLATE staging"
    compose start control >/dev/null
    tool wait --urls http://control:8080 --timeout 4m
  else
    echo "== already seeded"
  fi
  echo "staging stack up: ${STAGING_PROJECT} (work dir ${STAGING_WORK})"
}

run_parity() {
  local out="${STAGING_WORK}/reports/batch-${batch}/parity.json"
  local suites
  suites="$("${STAGING_WORK}/bin/stagingctl" batches --parity-of "${batch}")"
  local entries=() postgres=false
  if [[ -n "${STAGING_PARITY_POSTGRES_DSN:-}" ]]; then postgres=true; fi
  for suite in ${suites}; do
    echo "== parity suite internal/tests/${suite}"
    local start end status=true log
    log="$(mktemp)"
    start="$(date +%s)"
    if ! (cd "${repo_root}" && ANIX_TEST_POSTGRES_DSN="${STAGING_PARITY_POSTGRES_DSN:-}" GOWORK=off \
          go test -count=1 "./internal/tests/${suite}/..." >"${log}" 2>&1); then
      status=false
    fi
    end="$(date +%s)"
    tail -3 "${log}"
    entries+=("$(python3 -c 'import json,sys; print(json.dumps({"name":sys.argv[1],"pass":sys.argv[2]=="true","seconds":int(sys.argv[3]),"postgres":sys.argv[4]=="true","tail":open(sys.argv[5]).read()[-1500:]}))' \
      "${suite}" "${status}" "$((end - start))" "${postgres}" "${log}")")
    rm -f "${log}"
  done
  local joined
  joined="$(IFS=,; echo "${entries[*]}")"
  printf '[%s]\n' "${joined}" > "${out}"
}

run_reconcile() {
  echo "== write reconciliation: twins from template staging_seed"
  compose --profile reconcile rm -sf recon-legacy recon-native >/dev/null 2>&1 || true
  for db in recon_legacy recon_native; do
    psql_staging -c "DROP DATABASE IF EXISTS ${db}" -c "CREATE DATABASE ${db} TEMPLATE staging_seed"
  done
  compose --profile reconcile up -d recon-legacy recon-native
  local status=0
  tool reconcile --batch "${batch}" || status=$?
  compose --profile reconcile rm -sf recon-legacy recon-native >/dev/null 2>&1 || true
  for db in recon_legacy recon_native; do psql_staging -c "DROP DATABASE IF EXISTS ${db}"; done
  return "${status}"
}

cmd_batch() {
  load_env
  disk_check
  [[ "${batch}" =~ ^[1-4]$ ]] || die "--batch must be 1-4"
  if [[ "${rollback}" == 1 ]]; then
    tool rollback --batch "${batch}"
    return
  fi
  mkdir -p "${STAGING_WORK}/reports/batch-${batch}"
  rm -f "${STAGING_WORK}/reports/batch-${batch}/"*.json "${STAGING_WORK}/reports/batch-${batch}/report.md"
  echo "== batch ${batch}: read routes to shadow (min ${min_requests} requests, ${min_duration} in shadow)"
  tool shadow --batch "${batch}"
  tool replay --batch "${batch}" --min-requests "${min_requests}" --min-shadow-duration "${min_duration}" --rate "${rate}" || true
  if [[ "${parity}" == 1 ]]; then run_parity; fi
  if [[ "${reconcile}" == 1 ]]; then run_reconcile || true; fi
  tool report --batch "${batch}"
}

case "${command}" in
  up) cmd_up ;;
  batch) cmd_batch ;;
  routes)
    load_env
    compose exec -T control /app/anix-control routes "${args[@]}" ;;
  status)
    load_env
    compose ps
    df -h "${STAGING_WORK%/*}" | tail -1 ;;
  logs)
    load_env
    compose logs --no-color --tail 200 "${args[@]}" ;;
  down)
    if [[ -f "${STAGING_WORK}/staging.env" ]]; then
      load_env
      compose --profile reconcile --profile tool down -v --remove-orphans
    fi
    if [[ " ${args[*]:-} " == *" --purge "* ]]; then rm -rf "${STAGING_WORK}"; else rm -f "${STAGING_WORK}/staging.env" "${STAGING_WORK}/seed-manifest.json"; fi
    echo "staging stack removed" ;;
  *) usage; exit 2 ;;
esac
