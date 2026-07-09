#!/usr/bin/env bash
# Guard the GitHub Actions release workflow contract.

set -euo pipefail

REPO_ROOT_DEFAULT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
REPO_ROOT="${RELEASE_WORKFLOW_REPO_ROOT:-${REPO_ROOT_DEFAULT}}"
WORKFLOW_PATH="${RELEASE_WORKFLOW_PATH:-${REPO_ROOT}/.github/workflows/ci.yml}"

usage() {
  cat <<'EOF'
Usage: config/scripts/check_release_workflow.sh [--self-test|--help]

Statically checks that the release workflow still provides the required release
contract: strict tag gating, blocking quality/security/race/test prerequisites,
multi-platform binaries, Docker metadata, frontend archives, checksums, SBOM,
operator deployment runbook, and generated GitHub release notes.
EOF
}

fail() {
  echo "error: $*" >&2
  return 1
}

require_text() {
  local needle="$1"
  local description="$2"

  if grep -Fq "${needle}" "${WORKFLOW_PATH}"; then
    echo "ok: ${description}"
    return 0
  fi

  fail "missing ${description}: ${needle}"
}

release_binary_pairs() {
  awk '
    /^  release-binaries:/ { in_job = 1; next }
    /^  docker:/ { in_job = 0 }
    in_job && /- goos:/ { goos = $3 }
    in_job && /goarch:/ && goos != "" { print goos "/" $2; goos = "" }
  ' "${WORKFLOW_PATH}" | sort -u
}

require_release_binary_pair() {
  local pair="$1"
  if release_binary_pairs | grep -Fxq "${pair}"; then
    echo "ok: release binary matrix includes ${pair}"
    return 0
  fi

  echo "release binary matrix pairs found:" >&2
  release_binary_pairs >&2
  fail "missing release binary matrix target ${pair}"
}

require_release_job_dependency() {
  local dependency="$1"
  local line

  line="$(awk '
    /^  release-binaries:/ { in_job = 1; next }
    /^  docker:/ { in_job = 0 }
    in_job && /needs: \[/ { print; exit }
  ' "${WORKFLOW_PATH}")"
  if [[ -n "${line}" && "${line}" == *"${dependency}"* ]]; then
    echo "ok: release binary job depends on ${dependency}"
    return 0
  fi

  fail "release binary job must depend on ${dependency}"
}

check_release_workflow() {
  local failed=0

  [[ -f "${WORKFLOW_PATH}" ]] || fail "workflow file not found: ${WORKFLOW_PATH}" || return 1

  require_text "tags: [ 'v*.*.*' ]" "tag trigger pattern" || failed=1
  require_text '^v[0-9]+\.[0-9]+\.[0-9]+$' "strict semantic release tag gate" || failed=1
  require_text "needs.tag-gate.outputs.is_release_tag == 'true'" "release-only job gate" || failed=1

  for dependency in \
    go-quality \
    go-lint \
    go-security \
    go-race \
    backend-test \
    postgres-stats-test \
    migration-dry-run-test \
    forward-runtime-test \
    grpc-test \
    cmd-test \
    tag-gate; do
    require_release_job_dependency "${dependency}" || failed=1
  done

  for pair in \
    linux/amd64 \
    linux/arm64 \
    windows/amd64 \
    windows/arm64 \
    darwin/amd64 \
    darwin/arm64; do
    require_release_binary_pair "${pair}" || failed=1
  done

  require_text "release-binary-\${{ matrix.goos }}-\${{ matrix.goarch }}" "per-platform release binary artifact upload" || failed=1
  require_text "name: frontend-dist" "frontend artifact download/upload contract" || failed=1
  require_text "v2board-frontend.tar.gz" "frontend tar archive" || failed=1
  require_text "v2board-frontend.zip" "frontend zip archive" || failed=1
  require_text "docker-image.txt" "Docker image metadata artifact" || failed=1
  require_text "digest=\${{ steps.build.outputs.digest }}" "Docker digest metadata" || failed=1
  require_text "anchore/sbom-action" "SBOM generation action" || failed=1
  require_text "spdx-json" "SPDX JSON SBOM format" || failed=1
  require_text "v2board-source.sbom.spdx.json" "source SBOM release asset" || failed=1
  require_text "OPERATOR_DEPLOYMENT.md" "operator deployment runbook" || failed=1
  require_text "No Local Release Builds" "operator no-local-build release warning" || failed=1
  require_text "sha256sum * > SHA256SUMS.txt" "release checksum generation" || failed=1
  require_text "softprops/action-gh-release" "GitHub release creation action" || failed=1
  require_text "generate_release_notes: true" "generated release notes" || failed=1
  require_text "files: release/*" "release asset upload glob" || failed=1

  return "${failed}"
}

run_self_test() {
  local tmpdir
  local fixture

  tmpdir="$(mktemp -d)"
  trap 'rm -rf "${tmpdir}"' RETURN
  fixture="${tmpdir}/ci.yml"

  cat >"${fixture}" <<'EOF'
on:
  push:
    tags: [ 'v*.*.*' ]

jobs:
  tag-gate:
    steps:
      - run: |
          [[ "${GITHUB_REF_NAME}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]

  release-binaries:
    needs: [go-quality, go-lint, go-security, go-race, backend-test, postgres-stats-test, migration-dry-run-test, forward-runtime-test, grpc-test, cmd-test, tag-gate]
    if: ${{ needs.tag-gate.outputs.is_release_tag == 'true' }}
    strategy:
      matrix:
        include:
          - goos: linux
            goarch: amd64
          - goos: linux
            goarch: arm64
          - goos: windows
            goarch: amd64
          - goos: windows
            goarch: arm64
          - goos: darwin
            goarch: amd64
          - goos: darwin
            goarch: arm64
    steps:
      - uses: actions/upload-artifact@v7
        with:
          name: release-binary-${{ matrix.goos }}-${{ matrix.goarch }}

  docker:
    if: ${{ needs.tag-gate.outputs.is_release_tag == 'true' }}
    steps:
      - run: |
          echo "digest=${{ steps.build.outputs.digest }}" > release/docker-image.txt

  release:
    if: ${{ needs.tag-gate.outputs.is_release_tag == 'true' }}
    steps:
      - uses: anchore/sbom-action@v0.24.0
        with:
          format: spdx-json
          output-file: release/v2board-source.sbom.spdx.json
      - uses: actions/download-artifact@v8
        with:
          name: frontend-dist
      - run: |
          echo "No Local Release Builds" > release/OPERATOR_DEPLOYMENT.md
          tar -czvf release/v2board-frontend.tar.gz -C web/public .
          zip -r release/v2board-frontend.zip web/public
          (cd release && sha256sum * > SHA256SUMS.txt)
      - uses: softprops/action-gh-release@v3
        with:
          files: release/*
          generate_release_notes: true
EOF

  if ! RELEASE_WORKFLOW_PATH="${fixture}" "${BASH_SOURCE[0]}" >/dev/null; then
    echo "self-test failed: complete fixture should pass" >&2
    return 1
  fi

  cp "${fixture}" "${fixture}.missing-platform"
  sed -i '/goos: darwin/{N;N;d;}' "${fixture}.missing-platform"
  if RELEASE_WORKFLOW_PATH="${fixture}.missing-platform" "${BASH_SOURCE[0]}" >/dev/null 2>&1; then
    echo "self-test failed: missing release platform should fail" >&2
    return 1
  fi

  cp "${fixture}" "${fixture}.missing-checksum"
  sed -i '/sha256sum/d' "${fixture}.missing-checksum"
  if RELEASE_WORKFLOW_PATH="${fixture}.missing-checksum" "${BASH_SOURCE[0]}" >/dev/null 2>&1; then
    echo "self-test failed: missing checksum should fail" >&2
    return 1
  fi

  cp "${fixture}" "${fixture}.missing-sbom"
  sed -i '/anchore\/sbom-action/d' "${fixture}.missing-sbom"
  if RELEASE_WORKFLOW_PATH="${fixture}.missing-sbom" "${BASH_SOURCE[0]}" >/dev/null 2>&1; then
    echo "self-test failed: missing SBOM should fail" >&2
    return 1
  fi

  echo "release workflow self-test passed"
}

case "${1:-}" in
  --self-test)
    run_self_test
    ;;
  -h|--help)
    usage
    ;;
  "")
    check_release_workflow
    ;;
  *)
    usage >&2
    exit 2
    ;;
esac
