#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CONTROL_HOME="${ANIX_CONTROL_HOME:-${V2BOARD_HOME:-/opt/anixops/control}}"

pick_existing_file() {
  local fallback="$1"
  shift
  local candidate
  for candidate in "${fallback}" "$@"; do
    if [[ -f "${candidate}" ]]; then
      printf '%s\n' "${candidate}"
      return
    fi
  done
  printf '%s\n' "${fallback}"
}

SOURCE_DB="${SOURCE_DB:-$(pick_existing_file \
  "${CONTROL_HOME}/config/data/v2board.db" \
  "${ROOT_DIR}/config/data/v2board.db" \
  "/opt/v2board/config/data/v2board.db" \
  "/var/lib/v2board/data/v2board.db")}"
TARGET_CONFIG="${TARGET_CONFIG:-$(pick_existing_file \
  "${CONTROL_HOME}/config/config.yaml" \
  "/opt/v2board/config/config.yaml" \
  "/etc/v2board/config.yaml")}"
TOOL="${TOOL:-${V2BOARD_MIGRATION_TOOL:-/tmp/anix-control-sqlite2postgres}}"
BACKUP_DIR="${BACKUP_DIR:-${V2BOARD_BACKUP_DIR:-${CONTROL_HOME}/backups}}"
SERVICE_NAME="${SERVICE_NAME:-}"

if [[ -z "${GO_BIN:-}" ]]; then
  GO_BIN="$(command -v go 2>/dev/null || true)"
  [[ -n "${GO_BIN}" ]] || GO_BIN="/usr/local/go/bin/go"
fi

if [[ -z "${SERVICE_NAME}" ]]; then
  SERVICE_NAME="anix-control.service"
  if ! systemctl list-unit-files anix-control.service --no-legend 2>/dev/null | grep -q '^anix-control\.service' &&
     systemctl list-unit-files v2board.service --no-legend 2>/dev/null | grep -q '^v2board\.service'; then
    SERVICE_NAME="v2board.service"
  fi
fi

if [[ "${EUID}" -ne 0 ]]; then
  echo "Run as root: sudo $0" >&2
  exit 1
fi

if [[ ! -f "$SOURCE_DB" ]]; then
  echo "SQLite source not found: $SOURCE_DB" >&2
  exit 1
fi

if [[ ! -f "$TARGET_CONFIG" ]]; then
  echo "Target config not found: $TARGET_CONFIG" >&2
  exit 1
fi

if [[ ! -x "$TOOL" ]]; then
  if [[ ! -x "$GO_BIN" ]]; then
    echo "Migration tool missing and Go not found: $GO_BIN" >&2
    exit 1
  fi
  "$GO_BIN" build -o "$TOOL" "$ROOT_DIR/cmd/sqlite2postgres"
fi

mkdir -p "$BACKUP_DIR"

service_stopped=0
restart_on_failure() {
  local exit_code=$?
  if [[ "$service_stopped" -eq 1 ]]; then
    echo "Migration failed; starting ${SERVICE_NAME} back up..." >&2
    systemctl start "${SERVICE_NAME}" || true
  fi
  exit "$exit_code"
}
trap restart_on_failure ERR

eval "$(python3 - "$TARGET_CONFIG" <<'PY'
import shlex
import sys

def parse_simple_section(path, section):
    data = {}
    in_section = False
    with open(path, "r", encoding="utf-8") as f:
        for raw in f:
            line = raw.split("#", 1)[0].rstrip()
            if not line.strip():
                continue
            if not raw.startswith((" ", "\t")) and line.endswith(":"):
                in_section = line[:-1].strip() == section
                continue
            if not in_section:
                continue
            if not raw.startswith((" ", "\t")):
                break
            stripped = line.strip()
            if ":" not in stripped:
                continue
            key, value = stripped.split(":", 1)
            value = value.strip().strip('"').strip("'")
            data[key.strip()] = value
    return data

db = parse_simple_section(sys.argv[1], "database")
server = parse_simple_section(sys.argv[1], "server")
if str(db.get("driver", "")).lower() not in ("postgres", "postgresql"):
    raise SystemExit("target config database.driver is not postgres")
values = {
    "PGHOST": db.get("host") or "127.0.0.1",
    "PGPORT": str(db.get("port") or 5432),
    "PGUSER": db.get("username") or "anix_control",
    "PGPASSWORD": db.get("password") or "",
    "PGDATABASE": db.get("database") or "anix_control",
    "PANEL_API_PORT": str(server.get("port") or 8080),
}
for key, value in values.items():
    print(f"export {key}={shlex.quote(str(value))}")
PY
)"

ts="$(date +%Y%m%d_%H%M%S)"
backup="$BACKUP_DIR/postgres_before_sqlite_import_$ts.sql.gz"

echo "Source SQLite: $SOURCE_DB"
echo "Target PostgreSQL: ${PGUSER}@${PGHOST}:${PGPORT}/${PGDATABASE}"
echo "Backup: $backup"

echo "Stopping ${SERVICE_NAME}..."
systemctl stop "${SERVICE_NAME}"
service_stopped=1

echo "Backing up current PostgreSQL..."
pg_dump -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d "$PGDATABASE" | gzip -9 > "$backup"

echo "Importing SQLite into PostgreSQL..."
"$TOOL" -source "$SOURCE_DB" -target-config "$TARGET_CONFIG" -reset

echo "Starting ${SERVICE_NAME}..."
systemctl start "${SERVICE_NAME}"
service_stopped=0

echo "Verifying service and row counts..."
curl -fsS "http://127.0.0.1:${PANEL_API_PORT}/health"
echo
psql -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d "$PGDATABASE" -c \
  "select 'v2_user' table_name, count(*) from v2_user union all select 'v2_node', count(*) from v2_node union all select 'v2_node_protocol', count(*) from v2_node_protocol union all select 'v2_plan', count(*) from v2_plan order by table_name;"

echo "Done."
