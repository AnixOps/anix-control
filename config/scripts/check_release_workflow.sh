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
  require_text "python3 config/scripts/check_release_version.py --tag" "release tag/source version consistency gate" || failed=1
  require_text "needs.tag-gate.outputs.is_release_tag == 'true'" "release-only job gate" || failed=1

  for dependency in \
    go-quality \
    go-lint \
    go-security \
    go-race \
    backend-test \
    postgres-stats-test \
    migration-dry-run-test \
    postgres-restore-rehearsal \
    plugin-package-release-test \
    plugin-package-publish \
    forward-runtime-test \
    grpc-test \
    cross-repository-agent-e2e \
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
  # GitHub expression must remain literal.
  # shellcheck disable=SC2016
  reject_text '${{ env.DOCKER_NAMESPACE }}/v2board' "legacy Docker image tag" || failed=1
  require_text "docker-image.txt" "Docker image metadata artifact" || failed=1
  require_text "digest=\${{ steps.build.outputs.digest }}" "Docker digest metadata" || failed=1
  require_text "Upload migration dry-run report" "migration dry-run report upload step" || failed=1
  require_text "Download migration dry-run report" "migration dry-run report download step" || failed=1
  require_text "migration-dry-run-report" "migration dry-run report artifact" || failed=1
  require_text "migration-dry-run.txt" "migration dry-run report release asset" || failed=1
  require_text "Run destructive restore rehearsal on disposable PostgreSQL" "PostgreSQL restore rehearsal execution step" || failed=1
  require_text "scripts/postgres_restore_rehearsal.sh" "PostgreSQL restore rehearsal script invocation" || failed=1
  require_text "name: postgres-restore-rehearsal" "PostgreSQL restore rehearsal evidence artifact" || failed=1
  require_text "Plugin Package Release Contracts" "official plugin package release job" || failed=1
  require_text "packages/machine-telemetry/tests/release_gate.sh" "machine telemetry package release gate invocation" || failed=1
  require_text "packages/nftables-forward/tests/release_gate.sh" "nftables forward package release gate invocation" || failed=1
  require_text "--agent-binary package-build/nftables-forward-agent" "real nftables forward Agent package input" || failed=1
  require_text '[[ "$(package-build/nftables-forward-agent --version)" == "nftables-forward 1.0.0" ]]' "nftables forward binary version gate" || failed=1
  require_text "packages/nftables-forward/tests/webui_smoke.mjs" "nftables forward WebUI smoke gate" || failed=1
  require_text "packages/gost-mesh/tests/release_gate.sh" "gost mesh package release gate invocation" || failed=1
  require_text "--agent-binary package-build/gost-mesh-agent" "real GOST mesh Agent package input" || failed=1
  require_text "--gost package-build/gost" "pinned GOST runtime package input" || failed=1
  require_text "packages/gost-mesh/tests/webui_smoke.mjs" "gost mesh WebUI smoke gate" || failed=1
  require_text "packages/nat-egress/tests/release_gate.sh" "nat egress package release gate invocation" || failed=1
  require_text "packages/nat-egress/tests/webui_smoke.mjs" "nat egress WebUI smoke gate" || failed=1
  require_text "set -o pipefail" "official plugin package gate failure propagation" || failed=1
  require_text "name: plugin-package-contract-reports" "official plugin package contract artifact" || failed=1
  require_text "nftables-forward-package-contract.txt" "nftables forward package contract report" || failed=1
  require_text "gost-mesh-package-contract.txt" "gost mesh package contract report" || failed=1
  require_text "nat-egress-package-contract.txt" "nat egress package contract report" || failed=1
  require_text "Publish Signed Official Plugin Packages" "signed official plugin package publish job" || failed=1
  require_text "ANIXOPS_PLUGIN_SIGNING_PRIVATE_KEY" "production plugin signing secret" || failed=1
  require_text "scripts/sign_plugin_release.sh" "production plugin signing script" || failed=1
  require_text "machine-telemetry-signed-release" "signed plugin release artifact" || failed=1
  require_text "nftables-forward-signed-release" "signed nftables forward plugin release artifact" || failed=1
  require_text "gost-mesh-signed-release" "signed GOST mesh plugin release artifact" || failed=1
  require_text "nat-egress-signed-release" "signed NAT egress plugin release artifact" || failed=1
  require_text "anixops-machine-telemetry-1.1.0.SHA256SUMS.txt" "signed plugin checksum evidence" || failed=1
  require_text "anixops-nftables-forward-1.0.0.SHA256SUMS.txt" "signed nftables forward checksum evidence" || failed=1
  require_text "anixops-gost-mesh-1.0.0.SHA256SUMS.txt" "signed GOST mesh checksum evidence" || failed=1
  require_text "anixops-nat-egress-1.0.0.SHA256SUMS.txt" "signed NAT egress checksum evidence" || failed=1
  require_text "GOST_VERSION: '3.2.6'" "pinned GOST runtime version" || failed=1
  require_text "b39037b0380ea001fb3c0c28441c2e10bfc694f90682739a65b53e55dce5238b" "pinned GOST archive checksum" || failed=1
  require_text "a2aea24efb4597b5f57b35b8e1bbcc59f439b80723854d4371f6828b46682ffb" "pinned GOST binary checksum" || failed=1
  require_text "./cmd/gost-mesh" "GOST mesh Agent release binary build" || failed=1
  require_text "packages/gost-mesh/build.py build" "deterministic GOST mesh package build" || failed=1
  require_text "./cmd/nat-egress" "NAT egress Agent release binary build" || failed=1
  require_text "packages/nat-egress/build.py build" "deterministic NAT egress package build" || failed=1
  require_text "Download signed Machine Telemetry package" "signed plugin release download" || failed=1
  require_text "Download signed nftables Forward package" "signed nftables forward release download" || failed=1
  require_text "Download signed GOST Mesh package" "signed GOST mesh release download" || failed=1
  require_text "Download signed NAT Egress package" "signed NAT egress release download" || failed=1
  require_text "--require machine-telemetry-1.1.0.tar" "signed plugin package verification requirement" || failed=1
  require_text "--require nftables-forward-1.0.0.tar" "signed nftables forward package verification requirement" || failed=1
  require_text "--require gost-mesh-1.0.0.tar" "signed GOST mesh package verification requirement" || failed=1
  require_text "--require anixops-gost-mesh-1.0.0.manifest.json" "signed GOST mesh manifest verification requirement" || failed=1
  require_text "--require anixops-gost-mesh-1.0.0.sig" "signed GOST mesh signature verification requirement" || failed=1
  require_text "--require anixops-gost-mesh-1.0.0.public-key.pem" "signed GOST mesh PEM key verification requirement" || failed=1
  require_text "--require anixops-gost-mesh-1.0.0.public-key.raw" "signed GOST mesh raw key verification requirement" || failed=1
  require_text "--require anixops-gost-mesh-1.0.0.SHA256SUMS.txt" "signed GOST mesh checksum verification requirement" || failed=1
  require_text "--require nat-egress-1.0.0.tar" "signed NAT egress package verification requirement" || failed=1
  require_text "--require anixops-nat-egress-1.0.0.manifest.json" "signed NAT egress manifest verification requirement" || failed=1
  require_text "--require anixops-nat-egress-1.0.0.sig" "signed NAT egress signature verification requirement" || failed=1
  require_text "--require anixops-nat-egress-1.0.0.public-key.pem" "signed NAT egress PEM key verification requirement" || failed=1
  require_text "--require anixops-nat-egress-1.0.0.public-key.raw" "signed NAT egress raw key verification requirement" || failed=1
  require_text "--require anixops-nat-egress-1.0.0.SHA256SUMS.txt" "signed NAT egress checksum verification requirement" || failed=1
  # Markdown backticks must remain literal.
  # shellcheck disable=SC2016
  require_text 'The signed `machine-telemetry`, `nftables-forward`, `gost-mesh`, and `nat-egress` packages' "release notes include GOST mesh and NAT egress" || failed=1
  require_text "canary-only until Secret ID" "GOST mesh stable-release limitation" || failed=1
  require_text "Control to Agent Process E2E" "cross-repository Agent process E2E job" || failed=1
  require_text "ref: 676b5ad1c339a157075ae46028eee06540ffc26b" "pinned Agent fixture commit" || failed=1
  require_text "ANIXOPS_CROSS_REPO_E2E: '1'" "cross-repository Agent process E2E opt-in" || failed=1
  require_text "KernelOperationBridgeCrossRepositoryAgentProcess" "cross-repository Agent process E2E test" || failed=1
  require_text "AgentPluginPackageCrossRepositoryE2E" "signed Agent package cross-repository E2E test" || failed=1
  require_text "cross-repository-agent-e2e" "release dependency on cross-repository Agent process E2E" || failed=1
  require_text "anchore/sbom-action" "SBOM generation action" || failed=1
  require_text "spdx-json" "SPDX JSON SBOM format" || failed=1
  require_text "anix-control-source.sbom.spdx.json" "primary source SBOM release asset" || failed=1
  require_text "OPERATOR_DEPLOYMENT.md" "operator deployment runbook" || failed=1
  require_text "UPGRADE.md" "upgrade and rollback runbook release asset" || failed=1
  require_text "release/verify-machine-telemetry-signature.py" "release-bound Machine Telemetry signature verifier" || failed=1
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
  require_text "--require verify-machine-telemetry-signature.py" "signature verifier verification requirement" || failed=1
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

env:
  GOST_VERSION: '3.2.6'
  GOST_LINUX_AMD64_ARCHIVE_SHA256: 'b39037b0380ea001fb3c0c28441c2e10bfc694f90682739a65b53e55dce5238b'
  GOST_LINUX_AMD64_BINARY_SHA256: 'a2aea24efb4597b5f57b35b8e1bbcc59f439b80723854d4371f6828b46682ffb'

jobs:
  tag-gate:
    steps:
      - run: |
          [[ "${GITHUB_REF_NAME}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-(alpha|beta|rc)(\.[0-9]+)?)?$ ]]
          python3 config/scripts/check_release_version.py --tag "${GITHUB_REF_NAME}"

  release-binaries:
    needs: [go-quality, go-lint, go-security, go-race, backend-test, postgres-stats-test, migration-dry-run-test, postgres-restore-rehearsal, plugin-package-release-test, plugin-package-publish, forward-runtime-test, grpc-test, cross-repository-agent-e2e, cmd-test, tag-gate]
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

  postgres-restore-rehearsal:
    steps:
      - name: Run destructive restore rehearsal on disposable PostgreSQL
        run: bash scripts/postgres_restore_rehearsal.sh --unavailable fail
      - uses: actions/upload-artifact@v7
        with:
          name: postgres-restore-rehearsal

  plugin-package-release-test:
    name: Plugin Package Release Contracts
    steps:
      - name: Build and verify real package inputs
        run: |
          go -C V2bX_AnixOps build -o package-build/nftables-forward-agent ./cmd/nftables-forward
          [[ "$(package-build/nftables-forward-agent --version)" == "nftables-forward 1.0.0" ]]
      - name: Run reproducible unsigned and signed package contract
        run: |
          set -o pipefail
          bash packages/machine-telemetry/tests/release_gate.sh | tee package-contract.txt
          bash packages/nftables-forward/tests/release_gate.sh \
            --agent-binary package-build/nftables-forward-agent \
            | tee nftables-forward-package-contract.txt
          bash packages/gost-mesh/tests/release_gate.sh \
            --agent-binary package-build/gost-mesh-agent \
            --gost package-build/gost \
            | tee gost-mesh-package-contract.txt
          bash packages/nat-egress/tests/release_gate.sh | tee nat-egress-package-contract.txt
          node packages/nftables-forward/tests/webui_smoke.mjs
          node packages/gost-mesh/tests/webui_smoke.mjs
          node packages/nat-egress/tests/webui_smoke.mjs
      - uses: actions/upload-artifact@v7
        with:
          name: plugin-package-contract-reports

  plugin-package-publish:
    name: Publish Signed Official Plugin Packages
    needs: [tag-gate, plugin-package-release-test, cross-repository-agent-e2e]
    if: ${{ needs.tag-gate.outputs.is_release_tag == 'true' }}
    steps:
      - env:
          ANIXOPS_PLUGIN_SIGNING_PRIVATE_KEY: ${{ secrets.ANIXOPS_PLUGIN_SIGNING_PRIVATE_KEY }}
        run: |
          scripts/sign_plugin_release.sh
          go -C V2bX_AnixOps build -o package-build/gost-mesh-agent ./cmd/gost-mesh
          python3 packages/gost-mesh/build.py build \
            --agent-binary package-build/gost-mesh-agent \
            --gost package-build/gost
          go -C V2bX_AnixOps build ./cmd/nat-egress
          python3 packages/nat-egress/build.py build
          echo "machine-telemetry-signed-release"
          echo "nftables-forward-signed-release"
          echo "gost-mesh-signed-release"
          echo "nat-egress-signed-release"
          echo "anixops-machine-telemetry-1.1.0.SHA256SUMS.txt"
          echo "anixops-nftables-forward-1.0.0.SHA256SUMS.txt"
          echo "anixops-gost-mesh-1.0.0.SHA256SUMS.txt"
          echo "anixops-nat-egress-1.0.0.SHA256SUMS.txt"

  cross-repository-agent-e2e:
    name: Control to Agent Process E2E
    steps:
      - uses: actions/checkout@v7
        with:
          repository: AnixOps/anix-agent
          ref: 676b5ad1c339a157075ae46028eee06540ffc26b
          path: V2bX_AnixOps
      - env:
          ANIXOPS_CROSS_REPO_E2E: '1'
        run: |
          go test -v -count=1 ./internal/grpc -run '^Test(KernelOperationBridgeCrossRepositoryAgentProcess|AgentPluginPackageCrossRepositoryE2E)$'

  release:
    needs: [frontend-build, release-binaries, docker, plugin-package-publish, tag-gate]
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
      - name: Download signed Machine Telemetry package
        uses: actions/download-artifact@v8
        with:
          name: machine-telemetry-signed-release
      - name: Download signed nftables Forward package
        uses: actions/download-artifact@v8
        with:
          name: nftables-forward-signed-release
      - name: Download signed GOST Mesh package
        uses: actions/download-artifact@v8
        with:
          name: gost-mesh-signed-release
      - name: Download signed NAT Egress package
        uses: actions/download-artifact@v8
        with:
          name: nat-egress-signed-release
      - run: |
          cp migration-dry-run.txt release/migration-dry-run.txt
          echo "No Local Release Builds" > release/OPERATOR_DEPLOYMENT.md
          echo 'The signed `machine-telemetry`, `nftables-forward`, `gost-mesh`, and `nat-egress` packages are attached; gost-mesh is canary-only until Secret ID materialization is complete.' >> release/OPERATOR_DEPLOYMENT.md
          cp docs/UPGRADE.md release/UPGRADE.md
          cp packages/machine-telemetry/verify_signature.py release/verify-machine-telemetry-signature.py
          tar -czvf release/anix-control-frontend.tar.gz -C web/public .
          zip -r release/anix-control-frontend.zip web/public
      - name: Generate release notes file
        run: |
          python3 config/scripts/generate_release_notes.py \
            --changelog CHANGELOG.md \
            --output release/RELEASE_NOTES.md
          echo 'The signed `machine-telemetry`, `nftables-forward`, `gost-mesh`, and `nat-egress` packages are attached; gost-mesh is canary-only until Secret ID materialization is complete.' >> release/RELEASE_NOTES.md
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
            --require verify-machine-telemetry-signature.py \
            --require RELEASE_NOTES.md \
            --require anix-control-linux-amd64.tar.gz \
            --require anix-control-windows-arm64.exe.zip \
            --require machine-telemetry-1.1.0.tar \
            --require nftables-forward-1.0.0.tar \
            --require gost-mesh-1.0.0.tar \
            --require anixops-gost-mesh-1.0.0.manifest.json \
            --require anixops-gost-mesh-1.0.0.sig \
            --require anixops-gost-mesh-1.0.0.public-key.pem \
            --require anixops-gost-mesh-1.0.0.public-key.raw \
            --require anixops-gost-mesh-1.0.0.SHA256SUMS.txt \
            --require nat-egress-1.0.0.tar \
            --require anixops-nat-egress-1.0.0.manifest.json \
            --require anixops-nat-egress-1.0.0.sig \
            --require anixops-nat-egress-1.0.0.public-key.pem \
            --require anixops-nat-egress-1.0.0.public-key.raw \
            --require anixops-nat-egress-1.0.0.SHA256SUMS.txt
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

  cp "${fixture}" "${fixture}.missing-plugin-package-gate"
  sed -i '/plugin-package-release-test/d;/Plugin Package Release Contracts/d;/packages\/machine-telemetry\/tests\/release_gate.sh/d;/packages\/nftables-forward\/tests\/release_gate.sh/d;/packages\/nftables-forward\/tests\/webui_smoke.mjs/d;/plugin-package-contract-reports/d;/nftables-forward-package-contract.txt/d' "${fixture}.missing-plugin-package-gate"
  if RELEASE_WORKFLOW_PATH="${fixture}.missing-plugin-package-gate" "${BASH_SOURCE[0]}" >/dev/null 2>&1; then
    echo "self-test failed: missing plugin package gate should fail" >&2
    return 1
  fi

  cp "${fixture}" "${fixture}.missing-nftables-version-gate"
  sed -i '/nftables-forward-agent --version/d' "${fixture}.missing-nftables-version-gate"
  if RELEASE_WORKFLOW_PATH="${fixture}.missing-nftables-version-gate" "${BASH_SOURCE[0]}" >/dev/null 2>&1; then
    echo "self-test failed: missing nftables forward binary version gate should fail" >&2
    return 1
  fi

  cp "${fixture}" "${fixture}.missing-signed-plugin-publish"
  sed -i '/plugin-package-publish:/,/cross-repository-agent-e2e/d;/Publish Signed Official Plugin Packages/d;/ANIXOPS_PLUGIN_SIGNING_PRIVATE_KEY/d;/scripts\/sign_plugin_release.sh/d;/machine-telemetry-signed-release/d;/nftables-forward-signed-release/d;/anixops-machine-telemetry-1.1.0.SHA256SUMS.txt/d;/anixops-nftables-forward-1.0.0.SHA256SUMS.txt/d' "${fixture}.missing-signed-plugin-publish"
  if RELEASE_WORKFLOW_PATH="${fixture}.missing-signed-plugin-publish" "${BASH_SOURCE[0]}" >/dev/null 2>&1; then
    echo "self-test failed: missing signed plugin publish gate should fail" >&2
    return 1
  fi

  cp "${fixture}" "${fixture}.missing-nat-egress-signed-publish"
  # Markdown backticks must remain literal.
  # shellcheck disable=SC2016
  sed -i '/cmd\/nat-egress/d;/packages\/nat-egress\/build.py build/d;/nat-egress-signed-release/d;/anixops-nat-egress-1.0.0/d;/--require nat-egress-1.0.0.tar/d;/The signed `machine-telemetry`, `nftables-forward`, and `nat-egress` packages/d' "${fixture}.missing-nat-egress-signed-publish"
  if RELEASE_WORKFLOW_PATH="${fixture}.missing-nat-egress-signed-publish" "${BASH_SOURCE[0]}" >/dev/null 2>&1; then
    echo "self-test failed: missing signed NAT egress publish chain should fail" >&2
    return 1
  fi

  cp "${fixture}" "${fixture}.missing-gost-mesh-signed-publish"
  sed -i '/cmd\/gost-mesh/d;/packages\/gost-mesh\/build.py build/d;/gost-mesh-signed-release/d;/anixops-gost-mesh-1.0.0/d;/--require gost-mesh-1.0.0.tar/d;/--agent-binary package-build\/gost-mesh-agent/d;/--gost package-build\/gost/d;/canary-only until Secret ID/d' "${fixture}.missing-gost-mesh-signed-publish"
  if RELEASE_WORKFLOW_PATH="${fixture}.missing-gost-mesh-signed-publish" "${BASH_SOURCE[0]}" >/dev/null 2>&1; then
    echo "self-test failed: missing signed GOST mesh publish chain should fail" >&2
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
