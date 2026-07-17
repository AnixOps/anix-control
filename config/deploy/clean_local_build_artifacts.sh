#!/usr/bin/env bash
# Remove local source-tree build outputs.
#
# Release builds must be produced by GitHub Actions. This helper is for
# cleaning stale local artifacts only; it does not build or deploy anything.
set -euo pipefail

REPO_ROOT="${CLEAN_REPO_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)}"

EXACT_ARTIFACTS=(
  "v2board"
  "v2board.exe"
  "server"
  "migrate"
  "coverage.out"
  "coverage.html"
  "web/public"
  "web/public-check"
  "web/coverage"
  "web/bundle-reports"
  "web/bundle-reports-check"
  "release"
)

GLOB_ARTIFACTS=()

DEPLOY_BACKUP_GLOB_ARTIFACTS=(
  "backups/frontend_*.tar.gz"
  "backups/frontend_*.zip"
  "internal/*/backups/*.zip"
  "internal/*/backups/*.tar.gz"
  "internal/*/backups/*.tgz"
)

usage() {
  cat <<'USAGE'
Usage: config/deploy/clean_local_build_artifacts.sh [--dry-run] [--include-deploy-backups] [--self-test]

Removes known local build outputs from the repository checkout. It deliberately
does not remove config files, databases, certificates, backup databases, or web/node_modules.

Options:
  --dry-run                 Print artifacts that would be removed.
  --include-deploy-backups  Also remove ignored local deploy archive leftovers,
                            such as backups/frontend_*.tar.gz and
                            internal/*/backups/*.zip. Database backups are kept.
  --self-test               Run the cleanup self-test in a temporary directory.
  -h, --help                Show this help.
USAGE
}

is_safe_relative_path() {
  local rel="$1"
  [[ -n "${rel}" && "${rel}" != "." && "${rel}" != /* && "${rel}" != *".."* ]]
}

add_target() {
  local root="$1"
  local path="$2"
  local map_name="$3"
  local -n target_map="${map_name}"
  local rel

  rel="${path#"${root}/"}"
  if ! is_safe_relative_path "${rel}"; then
    echo "Refusing unsafe cleanup target: ${path}" >&2
    return 1
  fi
  target_map["${rel}"]="${path}"
}

collect_glob_targets() {
  local root="$1"
  local map_name="$2"
  local pattern path

  shift 2

  shopt -s nullglob
  for pattern in "$@"; do
    for path in "${root}"/${pattern}; do
      [[ -e "${path}" || -L "${path}" ]] || continue
      add_target "${root}" "${path}" "${map_name}"
    done
  done
  shopt -u nullglob
}

collect_targets() {
  local root="$1"
  local map_name="$2"
  local include_deploy_backups="$3"
  local rel

  for rel in "${EXACT_ARTIFACTS[@]}"; do
    if [[ -e "${root}/${rel}" || -L "${root}/${rel}" ]]; then
      add_target "${root}" "${root}/${rel}" "${map_name}"
    fi
  done

  collect_glob_targets "${root}" "${map_name}" "${GLOB_ARTIFACTS[@]}"

  if [[ "${include_deploy_backups}" -eq 1 ]]; then
    collect_glob_targets "${root}" "${map_name}" "${DEPLOY_BACKUP_GLOB_ARTIFACTS[@]}"
  fi
}

clean_artifacts() {
  local root="$1"
  local dry_run="$2"
  local include_deploy_backups="$3"
  local rel path errfile
  local -a failed=()
  declare -A targets=()

  collect_targets "${root}" targets "${include_deploy_backups}"

  if [[ "${#targets[@]}" -eq 0 ]]; then
    echo "No local build artifacts found."
    return 0
  fi

  if [[ "${dry_run}" -eq 1 ]]; then
    echo "Local build artifacts that would be removed:"
    for rel in "${!targets[@]}"; do
      echo "  ${rel}"
    done | sort
    return 0
  fi

  errfile="$(mktemp)"
  for rel in "${!targets[@]}"; do
    path="${targets[${rel}]}"
    if rm -rf -- "${path}" 2>"${errfile}"; then
      echo "Removed ${rel}"
    else
      echo "Could not remove ${rel}" >&2
      if [[ -s "${errfile}" ]]; then
        sed -n '1,8p' "${errfile}" >&2
      fi
      failed+=("${rel}")
    fi
    : >"${errfile}"
  done
  rm -f "${errfile}"

  if [[ "${#failed[@]}" -gt 0 ]]; then
    echo >&2
    echo "Some artifacts require elevated permissions. From the repository root, run:" >&2
    echo "  cd ${root}" >&2
    printf "  sudo rm -rf --" >&2
    for rel in "${failed[@]}"; do
      printf " %q" "${rel}" >&2
    done
    printf "\n" >&2
    return 1
  fi
}

assert_exists() {
  local path="$1"
  local label="$2"
  if [[ ! -e "${path}" ]]; then
    echo "self-test failed: expected ${label} to exist" >&2
    exit 1
  fi
}

assert_missing() {
  local path="$1"
  local label="$2"
  if [[ -e "${path}" ]]; then
    echo "self-test failed: expected ${label} to be removed" >&2
    exit 1
  fi
}

run_self_test() {
  local tmpdir
  tmpdir="$(mktemp -d)"

  cleanup_self_test_dir() {
    trap - RETURN
    rm -rf "${tmpdir:-}"
  }
  trap cleanup_self_test_dir RETURN

  mkdir -p "${tmpdir}/config" "${tmpdir}/data" "${tmpdir}/web/node_modules/pkg/dist"
  mkdir -p "${tmpdir}/web/public/assets" "${tmpdir}/web/public-check/assets"
  mkdir -p "${tmpdir}/web/coverage" "${tmpdir}/web/bundle-reports" "${tmpdir}/web/bundle-reports-check"
  mkdir -p "${tmpdir}/backups" "${tmpdir}/internal/handler/backups"
  mkdir -p "${tmpdir}/release"
  touch "${tmpdir}/v2board" "${tmpdir}/v2board.exe" "${tmpdir}/server" "${tmpdir}/migrate"
  touch "${tmpdir}/coverage.out" "${tmpdir}/coverage.html" "${tmpdir}/v2board.bak.20260709"
  touch "${tmpdir}/web/public/assets/app.js" "${tmpdir}/web/bundle-reports/report.md"
  touch "${tmpdir}/web/public-check/assets/app.js" "${tmpdir}/web/coverage/coverage-final.json"
  touch "${tmpdir}/web/bundle-reports-check/bundle-size.md"
  touch "${tmpdir}/release/v2board-linux-amd64.tar.gz"
  touch "${tmpdir}/backups/frontend_20260709_000000.tar.gz"
  touch "${tmpdir}/backups/backup_20260709_000000.db"
  touch "${tmpdir}/internal/handler/backups/backup_20260709_000000.zip"
  touch "${tmpdir}/internal/handler/backups/backup_20260709_000000.db"
  touch "${tmpdir}/config/config.yaml" "${tmpdir}/data/v2board.db"
  touch "${tmpdir}/web/node_modules/pkg/dist/index.js"

  clean_artifacts "${tmpdir}" 1 0 >/dev/null
  assert_exists "${tmpdir}/web/public/assets/app.js" "dry-run artifact"

  clean_artifacts "${tmpdir}" 0 0 >/dev/null

  assert_missing "${tmpdir}/v2board" "backend binary"
  assert_missing "${tmpdir}/web/public" "frontend build output"
  assert_missing "${tmpdir}/web/public-check" "frontend check build output"
  assert_missing "${tmpdir}/web/coverage" "frontend coverage output"
  assert_missing "${tmpdir}/web/bundle-reports" "bundle report output"
  assert_missing "${tmpdir}/web/bundle-reports-check" "bundle report check output"
  assert_missing "${tmpdir}/release" "release staging directory"

  assert_exists "${tmpdir}/backups/frontend_20260709_000000.tar.gz" "deploy backup archive without opt-in"
  assert_exists "${tmpdir}/internal/handler/backups/backup_20260709_000000.zip" "internal backup archive without opt-in"
  assert_exists "${tmpdir}/config/config.yaml" "config file"
  assert_exists "${tmpdir}/data/v2board.db" "database file"
  assert_exists "${tmpdir}/v2board.bak.20260709" "local deploy backup"
  assert_exists "${tmpdir}/backups/backup_20260709_000000.db" "database backup"
  assert_exists "${tmpdir}/internal/handler/backups/backup_20260709_000000.db" "internal database backup"
  assert_exists "${tmpdir}/web/node_modules/pkg/dist/index.js" "node_modules package dist"

  clean_artifacts "${tmpdir}" 0 1 >/dev/null

  assert_missing "${tmpdir}/backups/frontend_20260709_000000.tar.gz" "deploy backup archive"
  assert_missing "${tmpdir}/internal/handler/backups/backup_20260709_000000.zip" "internal backup archive"
  assert_exists "${tmpdir}/backups/backup_20260709_000000.db" "database backup after opt-in cleanup"
  assert_exists "${tmpdir}/internal/handler/backups/backup_20260709_000000.db" "internal database backup after opt-in cleanup"

  echo "clean_local_build_artifacts.sh self-test passed"
}

main() {
  local dry_run=0
  local include_deploy_backups=0

  while [[ "$#" -gt 0 ]]; do
    case "$1" in
      --dry-run)
        dry_run=1
        ;;
      --include-deploy-backups)
        include_deploy_backups=1
        ;;
      --self-test)
        run_self_test
        return
        ;;
      -h|--help)
        usage
        return
        ;;
      *)
        echo "Unknown argument: $1" >&2
        usage >&2
        return 2
        ;;
    esac
    shift
  done

  clean_artifacts "${REPO_ROOT}" "${dry_run}" "${include_deploy_backups}"
}

main "$@"
