#!/usr/bin/env bash
# Legacy local deploy wrapper.
#
# Release binaries and frontend assets must be produced by GitHub Actions.
# This wrapper exists only for compatibility with old operator notes and
# delegates emergency local builds to the guarded deploy_panel.sh script.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

show_help() {
  cat <<'EOF'
Usage: config/scripts/deploy.sh [--self-test|--help]

Release builds must be produced by GitHub Actions release workflows.
Deploy GitHub Release artifacts after verifying SHA256SUMS.txt.

This legacy script does not build by default. For explicitly approved
development or emergency operator work, set ALLOW_LOCAL_BUILD=1; the script
will then delegate to config/deploy/deploy_panel.sh.
EOF
}

require_local_build_opt_in() {
  if [[ "${ALLOW_LOCAL_BUILD:-}" == "1" ]]; then
    return 0
  fi

  cat >&2 <<'EOF'
!! config/scripts/deploy.sh is a legacy local source-tree deploy path.
!! Release builds must be produced by GitHub Actions release workflows.
!! Download and deploy verified GitHub Release artifacts instead.
!! For development or emergency operator use, rerun with ALLOW_LOCAL_BUILD=1.
EOF
  return 1
}

run_self_test() {
  if require_local_build_opt_in >/dev/null 2>&1; then
    echo "self-test failed: default guard should reject local builds" >&2
    return 1
  fi

  if [[ ! -x "${REPO_ROOT}/config/deploy/deploy_panel.sh" ]]; then
    echo "self-test failed: config/deploy/deploy_panel.sh is not executable" >&2
    return 1
  fi

  echo "legacy deploy guard self-test passed"
}

case "${1:-}" in
  --self-test)
    run_self_test
    exit 0
    ;;
  -h|--help)
    show_help
    exit 0
    ;;
esac

require_local_build_opt_in || exit 1
exec "${REPO_ROOT}/config/deploy/deploy_panel.sh"
