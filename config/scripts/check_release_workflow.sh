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
contract: stable/alpha/beta/release-candidate tag gating, blocking quality/security/race/test prerequisites,
multi-platform binaries, Docker metadata, frontend archives, checksums, SBOM,
operator deployment and upgrade runbooks, deterministic release notes,
machine-readable release manifest, and generated GitHub release notes.
EOF
}

fail() {
  echo "error: $*" >&2
  return 1
}

require_text() {
  local needle="$1"
  local description="$2"

  if grep -Fq -- "${needle}" "${WORKFLOW_PATH}"; then
    echo "ok: ${description}"
    return 0
  fi

  fail "missing ${description}: ${needle}"
}

reject_text() {
  local needle="$1"
  local description="$2"

  if grep -Fq -- "${needle}" "${WORKFLOW_PATH}"; then
    fail "forbidden ${description}: ${needle}"
    return 1
  fi

  echo "ok: ${description} is absent"
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
  require_text '^v[0-9]+\.[0-9]+\.[0-9]+(-(alpha|beta|rc)(\.[0-9]+)?)?$' "stable and prerelease tag gate" || failed=1
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
  require_text "anix-control-frontend.tar.gz" "primary frontend tar archive" || failed=1
  require_text "anix-control-frontend.zip" "primary frontend zip archive" || failed=1
  reject_text "v2board-frontend.tar.gz" "legacy frontend release alias" || failed=1
  reject_text "v2board-frontend.zip" "legacy frontend release alias" || failed=1
  reject_text "v2board-source.sbom.spdx.json" "legacy source SBOM release alias" || failed=1
  reject_text "v2board-linux-amd64.tar.gz" "legacy Linux release alias" || failed=1
  reject_text "v2board-windows-amd64.exe.zip" "legacy Windows release alias" || failed=1
  reject_text '${{ env.DOCKER_NAMESPACE }}/v2board' "legacy Docker image tag" || failed=1
  require_text "docker-image.txt" "Docker image metadata artifact" || failed=1
  require_text "digest=\${{ steps.build.outputs.digest }}" "Docker digest metadata" || failed=1
  require_text "Upload migration dry-run report" "migration dry-run report upload step" || failed=1
  require_text "Download migration dry-run report" "migration dry-run report download step" || failed=1
  require_text "migration-dry-run-report" "migration dry-run report artifact" || failed=1
  require_text "migration-dry-run.txt" "migration dry-run report release asset" || failed=1
  require_text "anchore/sbom-action" "SBOM generation action" || failed=1
  require_text "spdx-json" "SPDX JSON SBOM format" || failed=1
  require_text "anix-control-source.sbom.spdx.json" "primary source SBOM release asset" || failed=1
  require_text "OPERATOR_DEPLOYMENT.md" "operator deployment runbook" || failed=1
  require_text "UPGRADE.md" "upgrade and rollback runbook release asset" || failed=1
  require_text "No Local Release Builds" "operator no-local-build release warning" || failed=1
  require_text "Generate release notes file" "release notes generation step" || failed=1
  require_text "config/scripts/generate_release_notes.py" "release notes generator script" || failed=1
  require_text "RELEASE_NOTES.md" "release notes release asset" || failed=1
  require_text "Generate release manifest" "release manifest generation step" || failed=1
  require_text "config/scripts/generate_release_manifest.py" "release manifest generator script" || failed=1
  require_text "RELEASE_MANIFEST.json" "release manifest asset" || failed=1
  require_text "--build-source github-actions" "release manifest CI build source" || failed=1
  require_text "--manual-deployment-required true" "release manifest manual deployment flag" || failed=1
  require_text "sha256sum * > SHA256SUMS.txt" "release checksum generation" || failed=1
  require_text "Verify release artifacts" "release artifact verification step" || failed=1
  require_text "config/scripts/verify_release_artifacts.py" "release artifact verification script" || failed=1
  require_text "--require OPERATOR_DEPLOYMENT.md" "operator runbook verification requirement" || failed=1
  require_text "--require UPGRADE.md" "upgrade runbook verification requirement" || failed=1
  require_text "--require RELEASE_NOTES.md" "release notes verification requirement" || failed=1
  require_text "--require anix-control-linux-amd64.tar.gz" "primary linux amd64 release artifact verification requirement" || failed=1
  require_text "--require anix-control-windows-arm64.exe.zip" "primary windows arm64 release artifact verification requirement" || failed=1
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
          [[ "${GITHUB_REF_NAME}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-(alpha|beta|rc)(\.[0-9]+)?)?$ ]]

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

  migration-dry-run-test:
    steps:
      - name: Upload migration dry-run report
        uses: actions/upload-artifact@v7
        with:
          name: migration-dry-run-report
          path: migration-dry-run.txt

  release:
    if: ${{ needs.tag-gate.outputs.is_release_tag == 'true' }}
    steps:
      - uses: anchore/sbom-action@v0.24.0
        with:
          format: spdx-json
          output-file: release/anix-control-source.sbom.spdx.json
      - uses: actions/download-artifact@v8
        with:
          name: frontend-dist
      - name: Download migration dry-run report
        uses: actions/download-artifact@v8
        with:
          name: migration-dry-run-report
      - run: |
          cp migration-dry-run.txt release/migration-dry-run.txt
          echo "No Local Release Builds" > release/OPERATOR_DEPLOYMENT.md
          cp docs/UPGRADE.md release/UPGRADE.md
          tar -czvf release/anix-control-frontend.tar.gz -C web/public .
          zip -r release/anix-control-frontend.zip web/public
      - name: Generate release notes file
        run: |
          python3 config/scripts/generate_release_notes.py \
            --changelog CHANGELOG.md \
            --output release/RELEASE_NOTES.md
      - name: Generate release manifest
        run: |
          python3 config/scripts/generate_release_manifest.py \
            --release-dir release \
            --output release/RELEASE_MANIFEST.json \
            --build-source github-actions \
            --manual-deployment-required true
      - name: Create release checksums
        run: |
          (cd release && sha256sum * > SHA256SUMS.txt)
      - name: Verify release artifacts
        run: |
          python3 config/scripts/verify_release_artifacts.py \
            --require OPERATOR_DEPLOYMENT.md \
            --require UPGRADE.md \
            --require RELEASE_NOTES.md \
            --require anix-control-linux-amd64.tar.gz \
            --require anix-control-windows-arm64.exe.zip
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

  cp "${fixture}" "${fixture}.missing-migration-report"
  sed -i '/migration-dry-run-report/d' "${fixture}.missing-migration-report"
  if RELEASE_WORKFLOW_PATH="${fixture}.missing-migration-report" "${BASH_SOURCE[0]}" >/dev/null 2>&1; then
    echo "self-test failed: missing migration report should fail" >&2
    return 1
  fi

  cp "${fixture}" "${fixture}.missing-manifest"
  sed -i '/RELEASE_MANIFEST.json/d' "${fixture}.missing-manifest"
  if RELEASE_WORKFLOW_PATH="${fixture}.missing-manifest" "${BASH_SOURCE[0]}" >/dev/null 2>&1; then
    echo "self-test failed: missing release manifest should fail" >&2
    return 1
  fi

  cp "${fixture}" "${fixture}.missing-artifact-verification"
  sed -i '/verify_release_artifacts.py/d' "${fixture}.missing-artifact-verification"
  if RELEASE_WORKFLOW_PATH="${fixture}.missing-artifact-verification" "${BASH_SOURCE[0]}" >/dev/null 2>&1; then
    echo "self-test failed: missing artifact verification should fail" >&2
    return 1
  fi

  cp "${fixture}" "${fixture}.missing-upgrade-runbook"
  sed -i '/UPGRADE.md/d' "${fixture}.missing-upgrade-runbook"
  if RELEASE_WORKFLOW_PATH="${fixture}.missing-upgrade-runbook" "${BASH_SOURCE[0]}" >/dev/null 2>&1; then
    echo "self-test failed: missing upgrade runbook should fail" >&2
    return 1
  fi

  cp "${fixture}" "${fixture}.missing-release-notes"
  sed -i '/Generate release notes file/d;/generate_release_notes.py/d;/RELEASE_NOTES.md/d' "${fixture}.missing-release-notes"
  if RELEASE_WORKFLOW_PATH="${fixture}.missing-release-notes" "${BASH_SOURCE[0]}" >/dev/null 2>&1; then
    echo "self-test failed: missing release notes should fail" >&2
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
