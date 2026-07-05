#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SOURCE_DB="${SOURCE_DB:-$ROOT_DIR/config/data/v2board.db}"
TARGET_CONFIG="${TARGET_CONFIG:-/etc/v2board/config.yaml}"
TOOL="${TOOL:-/tmp/sqlite2postgres}"
GO_BIN="${GO_BIN:-/home/dev/.local/opt/go/bin/go}"
BACKUP_DIR="${BACKUP_DIR:-/var/lib/v2board/backups}"

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
    echo "Migration failed; starting v2board.service back up..." >&2
    systemctl start v2board || true
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
    "PGUSER": db.get("username") or "postgres",
    "PGPASSWORD": db.get("password") or "",
    "PGDATABASE": db.get("database") or "v2board",
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

echo "Stopping v2board.service..."
systemctl stop v2board
service_stopped=1

echo "Backing up current PostgreSQL..."
pg_dump -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d "$PGDATABASE" | gzip -9 > "$backup"

echo "Importing SQLite into PostgreSQL..."
"$TOOL" -source "$SOURCE_DB" -target-config "$TARGET_CONFIG" -reset

echo "Starting v2board.service..."
systemctl start v2board
service_stopped=0

echo "Verifying service and row counts..."
curl -fsS "http://127.0.0.1:${PANEL_API_PORT}/health"
echo
psql -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d "$PGDATABASE" -c \
  "select 'v2_user' table_name, count(*) from v2_user union all select 'v2_node', count(*) from v2_node union all select 'v2_node_protocol', count(*) from v2_node_protocol union all select 'v2_plan', count(*) from v2_plan order by table_name;"
psql -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d "$PGDATABASE" -c \
  "select id,email from v2_user where email in ('kalijerry@anixops.com','admin@kalijerry.uk','test@kalijerry.uk') order by id;"
if psql -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d "$PGDATABASE" -tAc \
  "select count(*) from v2_user where email = 'kalijerry@anixops.com';" | grep -qv '^0$'; then
  echo "Import verification failed: old account kalijerry@anixops.com still exists." >&2
  exit 1
fi

echo "Done."
