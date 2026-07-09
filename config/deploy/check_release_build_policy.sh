#!/usr/bin/env bash
# Enforce that local deployment scripts cannot build release artifacts by
# default. GitHub Actions is the only approved release build source.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

usage() {
  cat <<'EOF'
Usage: config/deploy/check_release_build_policy.sh [--self-test|--help]

Scans local deployment shell scripts for Go/frontend build commands. Any script
that performs a local build must also document the GitHub Actions-only release
policy and require ALLOW_LOCAL_BUILD opt-in.
EOF
}

find_script_files() {
  local root="$1"
  find "${root}/config/deploy" "${root}/config/scripts" \
    -type f \
    -name '*.sh' \
    ! -name 'check_release_build_policy.sh' \
    -print \
    | sort
}

script_has_build_command() {
  local file="$1"
  grep -Eq \
    '(^|[^[:alnum:]_])(go|"\$\{GO_BIN\}"|\$\{GO_BIN\})[[:space:]]+build([^[:alnum:]_]|$)|(^|[^[:alnum:]_])(npm|"\$\{NPM_BIN\}"|\$\{NPM_BIN\})[[:space:]]+run[[:space:]]+build([^[:alnum:]_]|$)|(^|[^[:alnum:]_])(vite|node_modules/vite/bin/vite\.js)[[:space:]]+build([^[:alnum:]_]|$)' \
    "${file}"
}

script_has_release_policy_guard() {
  local file="$1"
  grep -Fq 'Release builds must be produced by GitHub Actions' "${file}" &&
    grep -Fq 'ALLOW_LOCAL_BUILD' "${file}"
}

check_policy() {
  local root="$1"
  local failed=0
  local file rel

  while IFS= read -r file; do
    if ! script_has_build_command "${file}"; then
      continue
    fi

    rel="${file#${root}/}"
    if script_has_release_policy_guard "${file}"; then
      echo "ok: ${rel} local build is guarded"
      continue
    fi

    echo "error: ${rel} contains a local build command without the release build guard" >&2
    failed=1
  done < <(find_script_files "${root}")

  return "${failed}"
}

run_self_test() {
  local tmpdir
  tmpdir="$(mktemp -d)"
  trap 'rm -rf "${tmpdir}"' RETURN

  mkdir -p "${tmpdir}/config/deploy" "${tmpdir}/config/scripts"
  cat >"${tmpdir}/config/deploy/guarded.sh" <<'EOF'
#!/usr/bin/env bash
# Release builds must be produced by GitHub Actions release workflows.
if [[ "${ALLOW_LOCAL_BUILD:-}" != "1" ]]; then
  exit 1
fi
go build ./cmd/server
EOF

  cat >"${tmpdir}/config/scripts/plain.sh" <<'EOF'
#!/usr/bin/env bash
echo "no build here"
EOF

  if ! check_policy "${tmpdir}" >/dev/null; then
    echo "self-test failed: guarded build should pass" >&2
    return 1
  fi

  cat >"${tmpdir}/config/scripts/unguarded.sh" <<'EOF'
#!/usr/bin/env bash
npm run build
EOF

  if check_policy "${tmpdir}" >/dev/null 2>&1; then
    echo "self-test failed: unguarded build should fail" >&2
    return 1
  fi

  echo "release build policy self-test passed"
}

case "${1:-}" in
  --self-test)
    run_self_test
    exit 0
    ;;
  -h|--help)
    usage
    exit 0
    ;;
  "")
    check_policy "${REPO_ROOT}"
    ;;
  *)
    usage >&2
    exit 2
    ;;
esac
