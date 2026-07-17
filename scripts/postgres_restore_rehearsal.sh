#!/usr/bin/env bash
# Rehearse SQLite -> PostgreSQL migration plus a physical restore on a
# disposable database. The script does not touch application runtime code.

set -Eeuo pipefail

readonly SCRIPT_PATH="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/$(basename "${BASH_SOURCE[0]}")"
readonly ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

OUTPUT_DIR="${POSTGRES_REHEARSAL_OUTPUT_DIR:-postgres-restore-rehearsal}"
TARGET_DSN="${POSTGRES_REHEARSAL_DSN:-${POSTGRES_TEST_DSN:-host=127.0.0.1 port=5432 user=postgres dbname=v2board_restore_rehearsal sslmode=disable TimeZone=UTC}}"
PG_CLI_DSN=""
SOURCE_DB="${POSTGRES_REHEARSAL_SOURCE:-}"
GO_BIN="${GO_BIN:-go}"
UNAVAILABLE_MODE="fail"
ALLOW_NON_DISPOSABLE=0
WORK_DIR=""
SOURCE_MODE=""
SELF_TEST_TMPDIR=""

readonly SNAPSHOT_TABLES=(v2_user v2_plan v2_system_config)

usage() {
  cat <<'EOF'
Usage: scripts/postgres_restore_rehearsal.sh [options]

Runs a disposable PostgreSQL restore rehearsal:
  1. creates a deterministic SQLite fixture (unless --source is supplied)
  2. imports it with cmd/sqlite2postgres -reset
  3. captures schema, rows, counts, and hashes
  4. creates a pg_dump custom-format backup
  5. drops/recreates the target schema and restores the dump
  6. verifies schema and data equivalence

Options:
  --source PATH                 Existing SQLite database (default: generated fixture)
  --target-dsn DSN              PostgreSQL DSN (default: POSTGRES_REHEARSAL_DSN,
                                POSTGRES_TEST_DSN, or local rehearsal database)
  --output-dir PATH             Evidence directory (default: postgres-restore-rehearsal)
  --unavailable fail|skip       PostgreSQL/dependency failure policy (default: fail)
  --allow-non-disposable        Allow a DSN without a test/rehearsal marker
  --go PATH                     Go executable (default: GO_BIN or go)
  --self-test                   Run dependency/skip/fail behavior checks without PostgreSQL
  -h, --help                    Show this help

The target DSN is destructive. By default its text must contain one of:
test, rehearsal, restore, migration, ci, sandbox, or staging.
EOF
}

error() {
  printf '[ERROR] %s\n' "$*" >&2
}

unavailable() {
  local reason="$1"
  if [[ "${UNAVAILABLE_MODE}" == "skip" ]]; then
    printf 'SKIP: PostgreSQL unavailable: %s\n' "${reason}"
    exit 0
  fi
  error "PostgreSQL unavailable: ${reason}"
  exit 2
}

require_option_value() {
  local option="$1"
  local value="${2:-}"
  [[ -n "${value}" ]] || {
    error "${option} requires a value"
    exit 2
  }
}

parse_args() {
  while (($# > 0)); do
    case "$1" in
      --source)
        require_option_value "$1" "${2:-}"
        SOURCE_DB="$2"
        shift 2
        ;;
      --target-dsn)
        require_option_value "$1" "${2:-}"
        TARGET_DSN="$2"
        shift 2
        ;;
      --output-dir)
        require_option_value "$1" "${2:-}"
        OUTPUT_DIR="$2"
        shift 2
        ;;
      --unavailable)
        require_option_value "$1" "${2:-}"
        case "$2" in
          fail|skip) UNAVAILABLE_MODE="$2" ;;
          *) error "--unavailable must be fail or skip"; exit 2 ;;
        esac
        shift 2
        ;;
      --allow-non-disposable)
        ALLOW_NON_DISPOSABLE=1
        shift
        ;;
      --go)
        require_option_value "$1" "${2:-}"
        GO_BIN="$2"
        shift 2
        ;;
      --self-test)
        run_self_test
        exit 0
        ;;
      -h|--help)
        usage
        exit 0
        ;;
      *)
        error "unknown option: $1"
        usage >&2
        exit 2
        ;;
    esac
  done
}

check_disposable_target() {
  if ((ALLOW_NON_DISPOSABLE == 1)); then
    printf '[WARN] --allow-non-disposable was supplied; the target schema will be destroyed.\n' >&2
    return
  fi

  if [[ ! "${TARGET_DSN,,}" =~ (test|rehearsal|restore|migration|ci|sandbox|staging) ]]; then
    error "target DSN does not look disposable; include test/rehearsal/restore/migration/ci/sandbox/staging or pass --allow-non-disposable"
    exit 2
  fi
}

check_commands() {
  local missing=()
  local command
  for command in python3 psql pg_isready pg_dump pg_restore sha256sum awk cmp mktemp sed grep wc; do
    command -v "${command}" >/dev/null 2>&1 || missing+=("${command}")
  done
  command -v "${GO_BIN}" >/dev/null 2>&1 || missing+=("${GO_BIN}")
  if ((${#missing[@]} > 0)); then
    unavailable "missing required commands: ${missing[*]}"
  fi
}

check_postgres() {
  local server_major dump_major restore_major
  # PostgreSQL CLI tools reject the GORM-specific session TimeZone token.
  # Keep the original DSN for sqlite2postgres and use this sanitized copy for
  # psql, pg_dump, pg_restore, and pg_isready.
  PG_CLI_DSN="$(printf '%s\n' "${TARGET_DSN}" | sed -E 's/[[:space:]]+TimeZone=[^[:space:]]+//g')"
  if ! pg_isready --dbname="${PG_CLI_DSN}" >/dev/null 2>&1; then
    unavailable "pg_isready did not report an accepting server"
  fi
  if ! psql "${PG_CLI_DSN}" -X -v ON_ERROR_STOP=1 -At -c 'SELECT 1' >/dev/null 2>&1; then
    unavailable "could not authenticate or query the target database"
  fi
  server_major="$(psql "${PG_CLI_DSN}" -X -v ON_ERROR_STOP=1 -At -c \
    "SELECT split_part(current_setting('server_version'), '.', 1)")"
  dump_major="$(pg_dump --version | sed -E 's/.*PostgreSQL\) ([0-9]+).*/\1/')"
  restore_major="$(pg_restore --version | sed -E 's/.*PostgreSQL\) ([0-9]+).*/\1/')"
  if [[ "${dump_major}" =~ ^[0-9]+$ && "${restore_major}" =~ ^[0-9]+$ && "${server_major}" =~ ^[0-9]+$ ]]; then
    if ((dump_major < server_major || restore_major < server_major)); then
      unavailable "PostgreSQL client tools are older than the server (server=${server_major}, pg_dump=${dump_major}, pg_restore=${restore_major})"
    fi
  fi
}

make_fixture() {
  SOURCE_DB="${SOURCE_DB:-${WORK_DIR}/v2board-restore-fixture.db}"
  python3 - "${SOURCE_DB}" <<'PY'
import os
import sqlite3
import sys

path = sys.argv[1]
os.makedirs(os.path.dirname(os.path.abspath(path)), exist_ok=True)
if os.path.exists(path):
    os.unlink(path)
conn = sqlite3.connect(path)
conn.executescript("""
CREATE TABLE v2_user (id INTEGER PRIMARY KEY, email TEXT, token TEXT);
CREATE TABLE v2_plan (id INTEGER PRIMARY KEY, name TEXT);
CREATE TABLE v2_system_config (id INTEGER PRIMARY KEY, key TEXT, value TEXT);
""")
conn.executemany(
    "INSERT INTO v2_user (id, email, token) VALUES (?, ?, ?)",
    [(1, "migration.example", "migration-token"),
     (2, "restore.example", "restore-token")],
)
conn.execute("INSERT INTO v2_plan (id, name) VALUES (?, ?)", (1, "Migration Plan"))
conn.execute(
    "INSERT INTO v2_system_config (id, key, value) VALUES (?, ?, ?)",
    (1, "migration.fixture", "true"),
)
conn.commit()
conn.close()
print(path)
PY
}

reset_public_schema() {
  psql "${PG_CLI_DSN}" -X -v ON_ERROR_STOP=1 >/dev/null <<'SQL'
DROP SCHEMA IF EXISTS public CASCADE;
CREATE SCHEMA public;
SQL
}

run_migration() {
  (
    cd "${ROOT_DIR}"
    "${GO_BIN}" run ./cmd/sqlite2postgres \
      -source "${SOURCE_DB}" \
      -target-dsn "${TARGET_DSN}" \
      -reset
  ) 2>&1 | tee "${OUTPUT_DIR}/migration-import.log"
}

table_query() {
  case "$1" in
    v2_user) printf '%s' 'SELECT id, email, token FROM v2_user ORDER BY id' ;;
    v2_plan) printf '%s' 'SELECT id, name FROM v2_plan ORDER BY id' ;;
    v2_system_config) printf '%s' 'SELECT id, key, value FROM v2_system_config ORDER BY id' ;;
    *) error "no deterministic snapshot query for table $1"; return 1 ;;
  esac
}

assert_snapshot_tables() {
  local table exists
  for table in "${SNAPSHOT_TABLES[@]}"; do
    exists="$(psql "${PG_CLI_DSN}" -X -v ON_ERROR_STOP=1 -At -c \
      "SELECT EXISTS (SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace WHERE n.nspname = 'public' AND c.relname = '${table}');")"
    [[ "${exists}" == "t" ]] || {
      error "required table is missing from PostgreSQL: ${table}"
      exit 1
    }
  done
}

snapshot() {
  local directory="$1"
  local table query count hash
  mkdir -p "${directory}"
  assert_snapshot_tables
  # PostgreSQL 15+ adds random \restrict/\unrestrict guard tokens to each
  # dump. Strip those nonce lines so the schema hash is reproducible.
  pg_dump --schema-only --no-owner --no-acl --no-comments \
    --dbname="${PG_CLI_DSN}" | sed -E '/^\\(restrict|unrestrict)/d' >"${directory}/schema.sql"
  sha256sum "${directory}/schema.sql" | awk '{print $1}' >"${directory}/schema.sha256"
  pg_dump --data-only --column-inserts --no-owner --no-acl \
    --dbname="${PG_CLI_DSN}" | sed -E '/^\\(restrict|unrestrict)/d' >"${directory}/data.sql"
  sha256sum "${directory}/data.sql" | awk '{print $1}' >"${directory}/data.sha256"

  : >"${directory}/manifest.tsv"
  for table in "${SNAPSHOT_TABLES[@]}"; do
    query="$(table_query "${table}")"
    psql "${PG_CLI_DSN}" -X -v ON_ERROR_STOP=1 -At -F $'\t' -P footer=off \
      -c "${query}" >"${directory}/${table}.tsv"
    count="$(psql "${PG_CLI_DSN}" -X -v ON_ERROR_STOP=1 -At \
      -c "SELECT count(*) FROM ${table}")"
    printf '%s\n' "${count}" >"${directory}/${table}.count"
    hash="$(sha256sum "${directory}/${table}.tsv" | awk '{print $1}')"
    printf '%s\t%s\t%s\n' "${table}" "${count}" "${hash}" >>"${directory}/manifest.tsv"
  done
}

compare_snapshot() {
  local table
  cmp -s "${OUTPUT_DIR}/before/schema.sql" "${OUTPUT_DIR}/after/schema.sql" || {
    error "schema snapshot changed after restore"
    return 1
  }
  cmp -s "${OUTPUT_DIR}/before/data.sql" "${OUTPUT_DIR}/after/data.sql" || {
    error "database-wide row snapshot changed after restore"
    return 1
  }
  for table in "${SNAPSHOT_TABLES[@]}"; do
    cmp -s "${OUTPUT_DIR}/before/${table}.tsv" "${OUTPUT_DIR}/after/${table}.tsv" || {
      error "row snapshot changed after restore: ${table}"
      return 1
    }
    cmp -s "${OUTPUT_DIR}/before/${table}.count" "${OUTPUT_DIR}/after/${table}.count" || {
      error "row count changed after restore: ${table}"
      return 1
    }
  done
  cmp -s "${OUTPUT_DIR}/before/manifest.tsv" "${OUTPUT_DIR}/after/manifest.tsv" || {
    error "snapshot manifest changed after restore"
    return 1
  }
}

write_report() {
  local dump_hash
  dump_hash="$(sha256sum "${OUTPUT_DIR}/postgres.dump" | awk '{print $1}')"
  {
    printf 'status=PASS\n'
    printf 'source_mode=%s\n' "${SOURCE_MODE}"
    printf 'target=disposable-postgresql\n'
    printf 'dump_format=custom\n'
    printf 'dump_sha256=%s\n' "${dump_hash}"
    printf 'tables=%s\n' "${SNAPSHOT_TABLES[*]}"
    printf '\n[before]\n'
    cat "${OUTPUT_DIR}/before/manifest.tsv"
    printf 'schema_sha256\t%s\n' "$(cat "${OUTPUT_DIR}/before/schema.sha256")"
    printf 'data_sha256\t%s\n' "$(cat "${OUTPUT_DIR}/before/data.sha256")"
    printf '\n[after]\n'
    cat "${OUTPUT_DIR}/after/manifest.tsv"
    printf 'schema_sha256\t%s\n' "$(cat "${OUTPUT_DIR}/after/schema.sha256")"
    printf 'data_sha256\t%s\n' "$(cat "${OUTPUT_DIR}/after/data.sha256")"
  } >"${OUTPUT_DIR}/rehearsal-report.txt"
}

run_self_test() {
  local tmpdir dsn output
  tmpdir="$(mktemp -d)"
  SELF_TEST_TMPDIR="${tmpdir}"
  dsn='host=127.0.0.1 port=1 user=invalid dbname=v2board_restore_rehearsal connect_timeout=1'

  if output="$(bash "${SCRIPT_PATH}" --target-dsn "${dsn}" --output-dir "${tmpdir}/skip" --unavailable skip 2>&1)"; then
    grep -q '^SKIP: PostgreSQL unavailable:' <<<"${output}" || {
      error "self-test skip path did not emit the expected marker"
      return 1
    }
  else
    error "self-test skip path returned non-zero: ${output}"
    return 1
  fi

  if output="$(bash "${SCRIPT_PATH}" --target-dsn "${dsn}" --output-dir "${tmpdir}/fail" --unavailable fail 2>&1)"; then
    error "self-test fail path unexpectedly returned zero"
    return 1
  fi
  grep -q 'PostgreSQL unavailable:' <<<"${output}" || {
    error "self-test fail path did not emit the expected error"
    return 1
  }
  bash -n "${SCRIPT_PATH}"
  printf 'postgres restore rehearsal self-test passed\n'
}

main() {
  cleanup() {
    [[ -z "${WORK_DIR}" ]] || rm -rf "${WORK_DIR}"
    [[ -z "${SELF_TEST_TMPDIR}" ]] || rm -rf "${SELF_TEST_TMPDIR}"
  }
  trap cleanup EXIT
  parse_args "$@"
  check_disposable_target
  check_commands
  check_postgres

  WORK_DIR="$(mktemp -d)"
  mkdir -p "${OUTPUT_DIR}"
  rm -rf "${OUTPUT_DIR}/before" "${OUTPUT_DIR}/after" "${OUTPUT_DIR}/postgres.dump" \
    "${OUTPUT_DIR}/rehearsal-report.txt" "${OUTPUT_DIR}/migration-import.log"

  if [[ -n "${SOURCE_DB}" ]]; then
    [[ -f "${SOURCE_DB}" ]] || {
      error "SQLite source not found: ${SOURCE_DB}"
      exit 1
    }
    SOURCE_MODE=provided
  else
    SOURCE_MODE=fixture
    make_fixture
  fi

  printf '[INFO] resetting disposable PostgreSQL schema\n'
  reset_public_schema
  printf '[INFO] importing SQLite source with sqlite2postgres\n'
  run_migration
  printf '[INFO] capturing pre-restore schema and data\n'
  snapshot "${OUTPUT_DIR}/before"

  printf '[INFO] creating PostgreSQL custom-format dump\n'
  pg_dump --format=custom --no-owner --no-acl --file "${OUTPUT_DIR}/postgres.dump" \
    --dbname="${PG_CLI_DSN}"
  pg_restore --list "${OUTPUT_DIR}/postgres.dump" >"${OUTPUT_DIR}/postgres.dump.list"

  printf '[INFO] dropping schema and restoring dump\n'
  reset_public_schema
  pg_restore --clean --if-exists --no-owner --no-acl --exit-on-error \
    --dbname="${PG_CLI_DSN}" "${OUTPUT_DIR}/postgres.dump"
  printf '[INFO] capturing post-restore schema and data\n'
  snapshot "${OUTPUT_DIR}/after"

  if ! compare_snapshot; then
    error "PostgreSQL restore rehearsal failed; evidence is in ${OUTPUT_DIR}"
    exit 1
  fi
  write_report
  printf '[PASS] PostgreSQL restore rehearsal passed; evidence: %s\n' "${OUTPUT_DIR}"
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  main "$@"
fi
