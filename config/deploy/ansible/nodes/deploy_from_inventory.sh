#!/usr/bin/env bash
set -euo pipefail

# Release builds must be produced by GitHub Actions. Local V2bX builds are kept
# only for explicitly approved development or emergency operator work and
# require ALLOW_LOCAL_BUILD=1.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PANEL_ROOT="$(cd "${SCRIPT_DIR}/../../../.." && pwd)"
V2BX_ROOT_DEFAULT="$(cd "${PANEL_ROOT}/.." && pwd)/V2bX_AnixOps"

INVENTORY_PATH="${INVENTORY_PATH:-${SCRIPT_DIR}/inventory.ini}"
PLAYBOOK_PATH="${PLAYBOOK_PATH:-${SCRIPT_DIR}/deploy_v2bx.yml}"
ANSIBLE_CONFIG_PATH="${ANSIBLE_CONFIG_PATH:-${SCRIPT_DIR}/../ansible.cfg}"
PANEL_CONFIG_PATH="${PANEL_CONFIG_PATH:-${PANEL_ROOT}/config/config.yaml}"
V2BX_ROOT="${V2BX_ROOT:-${V2BX_ROOT_DEFAULT}}"
GO_BIN="${GO_BIN:-go}"
BUILD_TAGS="${BUILD_TAGS:-sing xray hysteria2 with_quic with_grpc with_utls with_wireguard with_acme with_gvisor}"
BUILD_DIR="${BUILD_DIR:-${V2BX_ROOT}/build/inventory}"
AMD64_OUTPUT="${AMD64_OUTPUT:-${BUILD_DIR}/V2bX_linux_amd64}"
ARM64_OUTPUT="${ARM64_OUTPUT:-${BUILD_DIR}/V2bX_linux_arm64}"
DEFAULT_ARCH="${DEFAULT_ARCH:-amd64}"

usage() {
  cat <<'EOF'
Usage:
  deploy_from_inventory.sh [options] [-- extra ansible-playbook args]

Options:
  --all                    Deploy all hosts from [v2bx_nodes] (default)
  --host <alias>           Deploy a single host alias, repeatable
  --hosts <a,b,c>          Deploy a comma-separated host list
  --check                  Run ansible-playbook --check
  --skip-build             Skip local V2bX builds and use prebuilt artifact paths
  --list                   Show parsed hosts and exit
  --inventory <path>       Use a custom inventory file
  --admin-token <token>    Pass admin_token to ansible-playbook
  -h, --help               Show this help

Host inventory hints:
  Add v2bx_arch=arm64 on ARM nodes (for example Oracle ARM).
  If omitted, the script defaults the host to amd64.

Release build policy:
  Use GitHub Actions artifacts for normal node rollouts. Set AMD64_OUTPUT and
  ARM64_OUTPUT to the downloaded artifact paths and pass --skip-build. Local
  builds require ALLOW_LOCAL_BUILD=1 and must not be used for release builds.
EOF
}

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

require_local_build_opt_in() {
  if [[ "${ALLOW_LOCAL_BUILD:-}" == "1" ]]; then
    return 0
  fi

  cat >&2 <<'EOF'
!! deploy_from_inventory.sh would build V2bX locally.
!! Release builds must be produced by GitHub Actions release workflows.
!! Use --skip-build with verified GitHub Actions artifact paths instead.
!! For development or emergency operator use, rerun with ALLOW_LOCAL_BUILD=1.
EOF
  return 1
}

resolve_local_sqlite_db_path() {
  python3 - "${PANEL_CONFIG_PATH}" "${PANEL_ROOT}" <<'PY'
import os
import sys

cfg_path, panel_root = sys.argv[1:3]
if not os.path.exists(cfg_path):
    raise SystemExit(0)

driver = ""
database = ""
in_database = False

with open(cfg_path, "r", encoding="utf-8") as handle:
    for raw_line in handle:
        line = raw_line.rstrip("\n")
        stripped = line.strip()
        if not stripped or stripped.startswith("#"):
            continue

        indent = len(line) - len(line.lstrip(" "))
        if indent == 0:
            in_database = stripped.startswith("database:")
            continue

        if not in_database:
            continue

        if stripped.startswith("driver:"):
            driver = stripped.split(":", 1)[1].strip().strip('"').strip("'")
        elif stripped.startswith("database:"):
            database = stripped.split(":", 1)[1].strip().strip('"').strip("'")

driver = driver.lower()
if driver not in ("", "sqlite", "sqlite3"):
    raise SystemExit(0)

database = database or "config/data/v2board.db"
if os.path.isabs(database):
    print(database)
    raise SystemExit

cfg_dir = os.path.dirname(os.path.abspath(cfg_path))
candidates = []
if os.path.basename(cfg_dir).lower() == "config":
    candidates.append(os.path.normpath(os.path.join(os.path.dirname(cfg_dir), database)))
candidates.append(os.path.normpath(os.path.join(cfg_dir, database)))
candidates.append(os.path.normpath(os.path.join(panel_root, database)))

for candidate in candidates:
    if os.path.exists(candidate):
        print(candidate)
        raise SystemExit

if candidates:
    print(candidates[0])
PY
}

HOST_FILTERS=()
RUN_CHECK=0
SKIP_BUILD=0
LIST_ONLY=0
ADMIN_TOKEN="${ADMIN_TOKEN:-}"
EXTRA_ARGS=()

while [[ $# -gt 0 ]]; do
  case "$1" in
    --all)
      shift
      ;;
    --host)
      HOST_FILTERS+=("$2")
      shift 2
      ;;
    --hosts)
      IFS=',' read -r -a extra_hosts <<<"$2"
      for host in "${extra_hosts[@]}"; do
        [[ -n "${host}" ]] && HOST_FILTERS+=("${host}")
      done
      shift 2
      ;;
    --check)
      RUN_CHECK=1
      shift
      ;;
    --skip-build)
      SKIP_BUILD=1
      shift
      ;;
    --list)
      LIST_ONLY=1
      shift
      ;;
    --inventory)
      INVENTORY_PATH="$2"
      shift 2
      ;;
    --admin-token)
      ADMIN_TOKEN="$2"
      shift 2
      ;;
    --)
      shift
      EXTRA_ARGS+=("$@")
      break
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      EXTRA_ARGS+=("$1")
      shift
      ;;
  esac
done

if [[ ! -f "${INVENTORY_PATH}" ]]; then
  echo "inventory not found: ${INVENTORY_PATH}" >&2
  exit 1
fi
if [[ ! -f "${PLAYBOOK_PATH}" ]]; then
  echo "playbook not found: ${PLAYBOOK_PATH}" >&2
  exit 1
fi

require_cmd python3

HOST_FILTER_CSV=""
if [[ ${#HOST_FILTERS[@]} -gt 0 ]]; then
  HOST_FILTER_CSV="$(IFS=,; echo "${HOST_FILTERS[*]}")"
fi

HOSTS_JSON="$(
python3 - "${INVENTORY_PATH}" "${HOST_FILTER_CSV}" "${DEFAULT_ARCH}" <<'PY'
import json
import shlex
import sys

inventory_path, filter_csv, default_arch = sys.argv[1:4]
selected = {item.strip() for item in filter_csv.split(",") if item.strip()}
hosts = []
current_group = ""

with open(inventory_path, "r", encoding="utf-8") as handle:
    for raw_line in handle:
        line = raw_line.strip()
        if not line or line.startswith("#"):
            continue
        if line.startswith("[") and line.endswith("]"):
            current_group = line[1:-1].strip()
            continue
        if current_group and current_group != "v2bx_nodes":
            continue

        parts = shlex.split(line, posix=True)
        if not parts:
            continue
        alias = parts[0]
        if selected and alias not in selected:
            continue

        vars_map = {}
        for token in parts[1:]:
            if "=" not in token:
                continue
            key, value = token.split("=", 1)
            vars_map[key] = value

        arch = vars_map.get("v2bx_arch", default_arch).strip() or default_arch
        hosts.append({
            "alias": alias,
            "vars": vars_map,
            "arch": arch,
        })

if selected:
    found = {host["alias"] for host in hosts}
    missing = sorted(selected - found)
    if missing:
        raise SystemExit("hosts not found in inventory: " + ", ".join(missing))

print(json.dumps(hosts))
PY
)"

export HOSTS_JSON

HOST_COUNT="$(python3 - <<'PY'
import json
import os
hosts = json.loads(os.environ["HOSTS_JSON"])
print(len(hosts))
PY
)"

if [[ "${HOST_COUNT}" == "0" ]]; then
  echo "no hosts selected from inventory" >&2
  exit 1
fi

LOCAL_SQLITE_DB_PATH="$(resolve_local_sqlite_db_path || true)"
export LOCAL_SQLITE_DB_PATH

if [[ -n "${LOCAL_SQLITE_DB_PATH}" && -f "${LOCAL_SQLITE_DB_PATH}" ]]; then
  HOSTS_WITH_META_JSON="$(
  python3 - <<'PY'
import json
import os
import sqlite3

hosts = json.loads(os.environ["HOSTS_JSON"])
db_path = os.environ.get("LOCAL_SQLITE_DB_PATH", "").strip()

resolved_aliases = []

if db_path and os.path.exists(db_path):
    conn = sqlite3.connect(db_path)
    conn.row_factory = sqlite3.Row
    try:
        for host in hosts:
            vars_map = host.get("vars", {})
            if vars_map.get("api_key"):
                continue
            node_id = vars_map.get("node_id")
            if not node_id:
                continue
            row = conn.execute("SELECT api_key FROM v2_node WHERE id = ?", (node_id,)).fetchone()
            if row and row["api_key"]:
                vars_map["api_key"] = row["api_key"]
                resolved_aliases.append(host["alias"])
    finally:
        conn.close()

print(json.dumps({
    "hosts": hosts,
    "resolved_aliases": resolved_aliases,
}))
PY
  )"
  export HOSTS_WITH_META_JSON
  HOSTS_JSON="$(python3 - <<'PY'
import json
import os
payload = json.loads(os.environ["HOSTS_WITH_META_JSON"])
print(json.dumps(payload["hosts"]))
PY
)"
  export HOSTS_JSON

  RESOLVED_FROM_DB="$(
  python3 - <<'PY'
import json
import os
payload = json.loads(os.environ["HOSTS_WITH_META_JSON"])
print(",".join(payload["resolved_aliases"]))
PY
  )"
  if [[ -n "${RESOLVED_FROM_DB}" ]]; then
    echo "resolved api_key from local panel db for: ${RESOLVED_FROM_DB}"
  fi
fi

if [[ "${LIST_ONLY}" == "1" ]]; then
  python3 - <<'PY'
import json
import os
hosts = json.loads(os.environ["HOSTS_JSON"])
for host in hosts:
    vars_map = host["vars"]
    print(f"{host['alias']}: arch={host['arch']} node_id={vars_map.get('node_id','')} host={vars_map.get('ansible_host','')}")
PY
  exit 0
fi

mapfile -t REQUIRED_ARCHES < <(python3 - <<'PY'
import json
import os
hosts = json.loads(os.environ["HOSTS_JSON"])
arches = sorted({host["arch"] for host in hosts})
for arch in arches:
    print(arch)
PY
)

output_path_for_arch() {
  case "$1" in
    amd64)
      printf '%s\n' "${AMD64_OUTPUT}"
      ;;
    arm64)
      printf '%s\n' "${ARM64_OUTPUT}"
      ;;
    *)
      echo "unsupported v2bx_arch in inventory: $1" >&2
      return 1
      ;;
  esac
}

if [[ "${SKIP_BUILD}" != "1" ]]; then
  require_local_build_opt_in || exit 1
  if [[ ! -d "${V2BX_ROOT}" ]]; then
    echo "V2bX repo not found: ${V2BX_ROOT}" >&2
    exit 1
  fi
  require_cmd "${GO_BIN}"
  mkdir -p "${BUILD_DIR}"
  export GOEXPERIMENT=jsonv2
  export CGO_ENABLED=0

  for arch in "${REQUIRED_ARCHES[@]}"; do
    output_path="$(output_path_for_arch "${arch}")"

    echo "building V2bX for linux/${arch} -> ${output_path}"
    (
      cd "${V2BX_ROOT}"
      GOOS=linux GOARCH="${arch}" "${GO_BIN}" build \
        -v \
        -trimpath \
        -tags "${BUILD_TAGS}" \
        -o "${output_path}" \
        ./main.go
    )
  done
else
  for arch in "${REQUIRED_ARCHES[@]}"; do
    output_path="$(output_path_for_arch "${arch}")"
    if [[ ! -f "${output_path}" ]]; then
      echo "missing prebuilt V2bX artifact for linux/${arch}: ${output_path}" >&2
      echo "download the matching GitHub Actions artifact or set AMD64_OUTPUT/ARM64_OUTPUT" >&2
      exit 1
    fi
  done
fi

TEMP_DIR="$(mktemp -d)"
cleanup() {
  rm -rf "${TEMP_DIR}"
}
trap cleanup EXIT

GENERATED_INVENTORY="${TEMP_DIR}/inventory.generated.ini"
export GENERATED_INVENTORY AMD64_OUTPUT ARM64_OUTPUT

python3 - <<'PY'
import json
import os

hosts = json.loads(os.environ["HOSTS_JSON"])
amd64_output = os.environ["AMD64_OUTPUT"]
arm64_output = os.environ["ARM64_OUTPUT"]
out_path = os.environ["GENERATED_INVENTORY"]

with open(out_path, "w", encoding="utf-8") as handle:
    handle.write("[v2bx_nodes]\n")
    for host in hosts:
        vars_map = dict(host["vars"])
        arch = host["arch"]
        vars_map["v2bx_arch"] = arch
        vars_map["v2bx_binary_local"] = arm64_output if arch == "arm64" else amd64_output
        tokens = [host["alias"]] + [f"{key}={value}" for key, value in vars_map.items()]
        handle.write(" ".join(tokens) + "\n")
PY

MISSING_API_KEY_HOSTS="$(
python3 - <<'PY'
import json
import os
hosts = json.loads(os.environ["HOSTS_JSON"])
missing = [host["alias"] for host in hosts if not host["vars"].get("api_key")]
print(",".join(missing))
PY
)"
if [[ -n "${MISSING_API_KEY_HOSTS}" && -z "${ADMIN_TOKEN}" ]]; then
  echo "hosts missing api_key and no admin token provided: ${MISSING_API_KEY_HOSTS}" >&2
  echo "add api_key=... in inventory or pass --admin-token / export ADMIN_TOKEN" >&2
  exit 1
fi

LIMIT_ARG=""
if [[ ${#HOST_FILTERS[@]} -gt 0 ]]; then
  LIMIT_ARG="$(IFS=,; echo "${HOST_FILTERS[*]}")"
else
  LIMIT_ARG="$(python3 - <<'PY'
import json
import os
hosts = json.loads(os.environ["HOSTS_JSON"])
print(",".join(host["alias"] for host in hosts))
PY
)"
fi

export ANSIBLE_CONFIG="${ANSIBLE_CONFIG_PATH}"

PLAY_ARGS=(-i "${GENERATED_INVENTORY}" "${PLAYBOOK_PATH}" -l "${LIMIT_ARG}")
if [[ -n "${ADMIN_TOKEN}" ]]; then
  PLAY_ARGS+=(-e "admin_token=${ADMIN_TOKEN}")
fi
if [[ "${RUN_CHECK}" == "1" ]]; then
  PLAY_ARGS+=(--check)
fi
if [[ ${#EXTRA_ARGS[@]} -gt 0 ]]; then
  PLAY_ARGS+=("${EXTRA_ARGS[@]}")
fi

echo "generated inventory: ${GENERATED_INVENTORY}"
echo "selected hosts: ${LIMIT_ARG}"
require_cmd ansible-playbook
ansible-playbook "${PLAY_ARGS[@]}"
