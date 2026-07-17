#!/usr/bin/env bash
# Require status docs or changelog updates when implementation, CI, or
# deployment surfaces change.

set -euo pipefail

REPO_ROOT_DEFAULT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
REPO_ROOT="${DOC_SYNC_REPO_ROOT:-${REPO_ROOT_DEFAULT}}"
BASE_REV=""
HEAD_REV="HEAD"

usage() {
  cat <<'EOF'
Usage: config/scripts/check_docs_updated.sh [--base REV] [--head REV|--worktree] [--self-test|--help]

Checks the changed files between BASE and HEAD. Use --worktree to check tracked
uncommitted files against BASE. If implementation, frontend,
CI, deployment, or configuration surfaces changed, the same diff must also
update a maintained status document such as CHANGELOG.md, TODO.md,
docs/features.md, docs/manual-intervention.md, docs/audit/*.md, docs/guide/*.md,
docs/reference/*.md, or docs/forwarding/*.md.
EOF
}

is_documentation_evidence() {
  local file="$1"
  case "${file}" in
    README.md|CHANGELOG.md|TODO.md|ROADMAP.md)
      return 0
      ;;
    docs/README.md|docs/DEPLOYMENT.md|docs/FEATURE_ROADMAP.md|docs/features.md|docs/manual-intervention.md)
      return 0
      ;;
    docs/audit/*.md|docs/forwarding/*.md|docs/guide/*.md|docs/reference/*.md)
      return 0
      ;;
  esac
  return 1
}

is_implementation_surface() {
  local file="$1"
  case "${file}" in
    .github/workflows/*)
      return 0
      ;;
    api/*|cmd/*|internal/*)
      return 0
      ;;
    web/src/*|web/index.html|web/package.json|web/package-lock.json|web/vite.config.*|web/vitest.config.*|web/playwright.config.*|web/scripts/*)
      return 0
      ;;
    go.mod|go.sum|Dockerfile|docker-compose*.yml)
      return 0
      ;;
    config/deploy/*|config/scripts/*|config/docker/*)
      return 0
      ;;
    config/config.yaml.example|config/config.prod.yaml)
      return 0
      ;;
  esac
  return 1
}

rev_exists() {
  local rev="$1"
  git -C "${REPO_ROOT}" rev-parse --verify --quiet "${rev}^{commit}" >/dev/null
}

default_base_rev() {
  if rev_exists "HEAD~1"; then
    echo "HEAD~1"
    return 0
  fi
  return 1
}

changed_files() {
  local base="$1"
  local head="$2"

  if [[ -z "${base}" ]]; then
    if ! base="$(default_base_rev)"; then
      return 0
    fi
  fi

  if ! rev_exists "${base}"; then
    echo "error: base revision is not available: ${base}" >&2
    return 2
  fi
  if [[ "${head}" == "WORKTREE" ]]; then
    {
      git -C "${REPO_ROOT}" diff --name-only --diff-filter=ACMR "${base}" --
      git -C "${REPO_ROOT}" ls-files --others --exclude-standard
    } | sort -u
    return 0
  fi

  if ! rev_exists "${head}"; then
    echo "error: head revision is not available: ${head}" >&2
    return 2
  fi

  git -C "${REPO_ROOT}" diff --name-only --diff-filter=ACMR "${base}" "${head}" -- | sort
}

check_docs_updated() {
  local changed=()
  local changed_output
  local changed_status=0
  local implementation_files=()
  local documentation_files=()
  local file

  changed_output="$(changed_files "${BASE_REV}" "${HEAD_REV}")" || changed_status=$?
  if (( changed_status != 0 )); then
    return "${changed_status}"
  fi
  if [[ -n "${changed_output}" ]]; then
    mapfile -t changed <<<"${changed_output}"
  fi

  for file in "${changed[@]}"; do
    if is_implementation_surface "${file}"; then
      implementation_files+=("${file}")
    fi
    if is_documentation_evidence "${file}"; then
      documentation_files+=("${file}")
    fi
  done

  if (( ${#implementation_files[@]} == 0 )); then
    echo "ok: no implementation, CI, deployment, or config surfaces changed"
    return 0
  fi

  if (( ${#documentation_files[@]} > 0 )); then
    echo "ok: documentation evidence found for implementation changes"
    printf 'implementation changes:\n'
    printf '  %s\n' "${implementation_files[@]}"
    printf 'documentation evidence:\n'
    printf '  %s\n' "${documentation_files[@]}"
    return 0
  fi

  echo "error: implementation, CI, deployment, or config surfaces changed without documentation evidence" >&2
  printf 'implementation changes:\n' >&2
  printf '  %s\n' "${implementation_files[@]}" >&2
  cat >&2 <<'EOF'

Update at least one maintained status document in the same change:
  CHANGELOG.md
  TODO.md
  ROADMAP.md
  docs/features.md
  docs/manual-intervention.md
  docs/audit/*.md
  docs/forwarding/*.md
  docs/guide/*.md
  docs/reference/*.md
EOF
  return 1
}

run_self_test() {
  local tmpdir
  local base
  local base_docs_only

  tmpdir="$(mktemp -d)"
  trap 'rm -rf "${tmpdir}"' RETURN

  git -C "${tmpdir}" init -q
  git -C "${tmpdir}" config user.email "ci@example.invalid"
  git -C "${tmpdir}" config user.name "CI"

  mkdir -p "${tmpdir}/internal" "${tmpdir}/docs/audit"
  cat >"${tmpdir}/internal/example.go" <<'EOF'
package internal

func example() {}
EOF
  touch "${tmpdir}/CHANGELOG.md" "${tmpdir}/TODO.md" "${tmpdir}/ROADMAP.md"
  touch "${tmpdir}/docs/features.md" "${tmpdir}/docs/manual-intervention.md"
  touch "${tmpdir}/docs/audit/test-gap.md"
  git -C "${tmpdir}" add .
  git -C "${tmpdir}" commit -q -m "initial"

  base="$(git -C "${tmpdir}" rev-parse HEAD)"
  printf '\nfunc changed() {}\n' >>"${tmpdir}/internal/example.go"
  git -C "${tmpdir}" add internal/example.go
  git -C "${tmpdir}" commit -q -m "source only"

  if DOC_SYNC_REPO_ROOT="${tmpdir}" "${BASH_SOURCE[0]}" --base "${base}" --head HEAD >/dev/null 2>&1; then
    echo "self-test failed: source-only change should require docs" >&2
    return 1
  fi

  printf '\n- Document source change.\n' >>"${tmpdir}/CHANGELOG.md"
  git -C "${tmpdir}" add CHANGELOG.md
  git -C "${tmpdir}" commit -q -m "document source change"

  if ! DOC_SYNC_REPO_ROOT="${tmpdir}" "${BASH_SOURCE[0]}" --base "${base}" --head HEAD >/dev/null; then
    echo "self-test failed: source plus changelog should pass" >&2
    return 1
  fi

  base_docs_only="$(git -C "${tmpdir}" rev-parse HEAD)"
  printf '\n- Docs-only update.\n' >>"${tmpdir}/TODO.md"
  git -C "${tmpdir}" add TODO.md
  git -C "${tmpdir}" commit -q -m "docs only"

  if ! DOC_SYNC_REPO_ROOT="${tmpdir}" "${BASH_SOURCE[0]}" --base "${base_docs_only}" --head HEAD >/dev/null; then
    echo "self-test failed: docs-only change should pass" >&2
    return 1
  fi

  echo "documentation sync self-test passed"
}

while (($#)); do
  case "$1" in
    --base)
      BASE_REV="${2:-}"
      if [[ -z "${BASE_REV}" ]]; then
        usage >&2
        exit 2
      fi
      shift 2
      ;;
    --head)
      HEAD_REV="${2:-}"
      if [[ -z "${HEAD_REV}" ]]; then
        usage >&2
        exit 2
      fi
      shift 2
      ;;
    --worktree)
      HEAD_REV="WORKTREE"
      shift
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
      usage >&2
      exit 2
      ;;
  esac
done

check_docs_updated
