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

job_block() {
  local job_name="$1"

  awk -v header="  ${job_name}:" '
    $0 == header { in_job = 1; next }
    in_job && /^  [[:alnum:]_-]+:$/ { exit }
    in_job { print }
  ' "${WORKFLOW_PATH}"
}

require_job_dependency() {
  local job_name="$1"
  local dependency="$2"
  local block
  local needs_line

  block="$(job_block "${job_name}")"
  needs_line="$(grep -E '^[[:space:]]*needs:[[:space:]]*\[' <<<"${block}" || true)"
  if [[ -n "${needs_line}" && "${needs_line}" == *"${dependency}"* ]]; then
    echo "ok: ${job_name} job depends on ${dependency}"
    return 0
  fi

  fail "${job_name} job must depend on ${dependency}"
}

reject_job_text() {
  local job_name="$1"
  local needle="$2"
  local description="$3"
  local block

  block="$(job_block "${job_name}")"
  if grep -Fq -- "${needle}" <<<"${block}"; then
    fail "forbidden ${description}: ${needle}"
    return 1
  fi

  echo "ok: ${description} is absent"
}

named_step_block() {
  local job_name="$1"
  local step_name="$2"

  job_block "${job_name}" | awk -v header="      - name: ${step_name}" '
    $0 == header { in_step = 1; next }
    in_step && /^      - name:/ { exit }
    in_step { print }
  '
}

require_named_step_text() {
  local job_name="$1"
  local step_name="$2"
  local needle="$3"
  local description="$4"
  local block

  block="$(named_step_block "${job_name}" "${step_name}")"
  if grep -Fq -- "${needle}" <<<"${block}"; then
    echo "ok: ${description}"
    return 0
  fi

  fail "missing ${description}: ${needle}"
}

reject_named_step_text() {
  local job_name="$1"
  local step_name="$2"
  local needle="$3"
  local description="$4"
  local block

  block="$(named_step_block "${job_name}" "${step_name}")"
  if grep -Fq -- "${needle}" <<<"${block}"; then
    fail "forbidden ${description}: ${needle}"
    return 1
  fi

  echo "ok: ${description} is absent"
}

frontend_build_block() {
  awk '
    /^  frontend-build:/ { in_job = 1; next }
    in_job && /^  [A-Za-z0-9_-]+:/ { exit }
    in_job { print }
  ' "${WORKFLOW_PATH}"
}

require_live_control_webui_gate() {
  local block
  block="$(frontend_build_block)"
  if [[ -z "${block}" ]]; then
    fail "frontend-build job is missing"
    return 1
  fi

  local required
  for required in \
    "actions/checkout@v7" \
    "actions/setup-go@v6" \
    "actions/setup-python@v6" \
    "actions/setup-node@v6" \
    "Checkout pinned Agent source for live Control WebUI E2E" \
    "ANIXOPS_AGENT_ROOT: \${{ github.workspace }}/V2bX_AnixOps" \
    "npm ci" \
    "npx playwright install --with-deps chromium" \
    "npx playwright test --config playwright.live-control.config.js"; do
    if ! grep -Fq -- "${required}" <<<"${block}"; then
      fail "frontend-build live Control WebUI gate is missing: ${required}"
      return 1
    fi
  done

  echo "ok: live Control signed WebUI E2E gate dependencies"
}

require_release_stage_tag_gate() {
  local tag_gate_block
  local normalized

  tag_gate_block="$(awk '
    /^  tag-gate:$/ { in_job = 1 }
    in_job && /^  [[:alnum:]_-]+:$/ && $0 != "  tag-gate:" { exit }
    in_job { print }
  ' "${WORKFLOW_PATH}")"
  normalized="$(printf '%s\n' "${tag_gate_block}" | tr '\n' ' ' | sed 's/\\//g' | tr -s ' ')"

  if ! grep -Fq 'python3 config/scripts/check_release_stage.py' <<<"${normalized}"; then
    fail "missing product release-stage scope gate"
    return 1
  fi
  if ! grep -Fq -- '--tag "${GITHUB_REF_NAME}"' <<<"${normalized}" \
    || ! grep -Fq -- '--github-output "${stage_output}"' <<<"${normalized}" \
    || ! grep -Fq 'cat "${stage_output}" >> "$GITHUB_OUTPUT"' <<<"${normalized}"; then
    fail "missing release-stage GitHub output relay"
    return 1
  fi
  if ! grep -Fq 'release_eligible: ${{ steps.check.outputs.release_eligible }}' <<<"${tag_gate_block}" \
    || ! grep -Fq 'grep -Fxq "release_eligible=true" "${stage_output}"' <<<"${tag_gate_block}" \
    || ! grep -Fq 'grep -Fxq "release_eligible=false" "${stage_output}"' <<<"${tag_gate_block}" \
    || ! grep -Fq 'grep -Fxq "release_stage_classification=historical-preview" "${stage_output}"' <<<"${tag_gate_block}"; then
    fail "missing publishable versus audit-only release-stage decision"
    return 1
  fi
  if ! grep -Fq 'Historical preview tag audited; release-only jobs are disabled' <<<"${tag_gate_block}"; then
    fail "missing historical preview audit-only release gate"
    return 1
  fi

  echo "ok: product release-stage scope gate distinguishes publishable and audit-only tags"
}

check_release_workflow() {
  local failed=0

  [[ -f "${WORKFLOW_PATH}" ]] || fail "workflow file not found: ${WORKFLOW_PATH}" || return 1

  require_text "tags: [ 'v*.*.*' ]" "tag trigger pattern" || failed=1
  require_text '^v[0-9]+\.[0-9]+\.[0-9]+(-(alpha|beta|rc)(\.[0-9]+)?)?$' "stable and prerelease tag gate" || failed=1
  require_text "python3 config/scripts/check_release_version.py --tag" "release tag/source version consistency gate" || failed=1
  require_release_stage_tag_gate || failed=1
  require_text "python3 config/scripts/check_release_stage.py --self-test" "release-stage contract self-test" || failed=1
  require_text "Setup Go for v4 route-gate contracts" "v4 route-gate Go toolchain setup" || failed=1
  require_text "needs.tag-gate.outputs.is_release_tag == 'true'" "release-only job gate" || failed=1
  require_job_dependency docker plugin-package-publish || failed=1
  require_text "  v4-public-rehearsal:" "public v4 rehearsal job" || failed=1
  require_text "  v4-release-evidence:" "formal v4 evidence job" || failed=1
  require_job_dependency v4-public-rehearsal plugin-package-publish || failed=1
  require_job_dependency v4-release-evidence v4-public-rehearsal || failed=1
  require_job_dependency release v4-release-evidence || failed=1
  require_text "packages/shared/build_package.py" "formal v4 package builder" || failed=1
  require_text "--formal-release" "formal v4 package signing mode" || failed=1
  require_text "Build formal v4 Agent plugin binaries" "formal v4 real Agent binary build step" || failed=1
  require_text "formal_agent_args+=(--agent-binary" "formal v4 explicit Agent binary mappings" || failed=1
  require_text "Download checksum-pinned formal v4 GOST runtimes" "formal v4 pinned GOST runtime download step" || failed=1
  require_text "GOST_LINUX_ARM64_BINARY_SHA256" "formal v4 arm64 GOST binary checksum" || failed=1
  require_text "formal_runtime_args+=(--runtime-binary" "formal v4 explicit GOST runtime mappings" || failed=1
  require_text "--platform linux/amd64" "formal v4 amd64 package platform" || failed=1
  require_text "--platform linux/arm64" "formal v4 arm64 package platform" || failed=1
  require_text "-exclude-dir=config/scripts/testdata" "full gosec excludes the non-compiling AST fixture directory" || failed=1
  require_text "scripts/tests/test_identity_bootstrap_install.sh" "identity bootstrap installer regression test" || failed=1
  require_text "config/scripts/run_v4_rehearsal.sh" "v4 release rehearsal invocation" || failed=1
  require_text "config/scripts/verify_v4_evidence.py" "v4 release evidence verification" || failed=1
  require_text "ANIXOPS_V4_CANARY_APPROVAL" "v4 signed canary approval input" || failed=1
  require_text "ANIXOPS_V4_SUPPORT_APPROVAL" "v4 signed support approval input" || failed=1
  require_text "ANIXOPS_PLUGIN_OFFICIAL_PUBLIC_KEY" "protected official package root input" || failed=1
  require_text "candidate configuration does not match the protected official package root" "candidate root pin comparison" || failed=1
  require_text "unset ANIXOPS_PLUGIN_SIGNING_PRIVATE_KEY ANIXOPS_PLUGIN_OFFICIAL_PUBLIC_KEY" "v4 signing secret environment cleanup" || failed=1
  require_text "--postgres-dsn" "v4 PostgreSQL rehearsal input" || failed=1
  require_text "docker run --rm --detach" "on-demand v4 PostgreSQL rehearsal" || failed=1
  reject_job_text v4-public-rehearsal "services:" "job-level v4 PostgreSQL service" || failed=1
  require_named_step_text v4-public-rehearsal "Run public v4 rehearsal gates" "--results-output" "public v4 rehearsal result collection" || failed=1
  require_named_step_text v4-public-rehearsal "Run public v4 rehearsal gates" "docker run --rm --detach" "public v4 PostgreSQL rehearsal execution" || failed=1
  for secret_input in \
    "ANIXOPS_PLUGIN_SIGNING_PRIVATE_KEY" \
    "ANIXOPS_PLUGIN_OFFICIAL_PUBLIC_KEY" \
    "ANIXOPS_V4_CANARY_APPROVAL" \
    "ANIXOPS_V4_SUPPORT_APPROVAL" \
    "--signing-key" \
    "--canary-evidence" \
    "--support-evidence"; do
    reject_job_text v4-public-rehearsal "${secret_input}" "public v4 rehearsal job secret input" || failed=1
  done
  require_named_step_text v4-release-evidence "Sign v4 rehearsal evidence" "--results artifacts/v4-rehearsal-results.json" "private v4 evidence signing uses public results" || failed=1
  require_named_step_text v4-release-evidence "Sign v4 rehearsal evidence" "--signing-key" "private v4 evidence signing input" || failed=1
  reject_named_step_text v4-release-evidence "Sign v4 rehearsal evidence" "--postgres-dsn" "private v4 signing test input" || failed=1
  reject_named_step_text v4-release-evidence "Sign v4 rehearsal evidence" "docker run --rm --detach" "private v4 signing test runtime" || failed=1
  require_text "--trusted-official-public-key" "independent v4 evidence trust root" || failed=1
  require_text "--archive release/v4-release-evidence.tar.gz" "safe v4 evidence archive verification" || failed=1
  require_text "--release-packages-dir release" "published v4 package evidence parity verification" || failed=1
  require_text "v4-rehearsal-evidence.json.sig" "signed v4 evidence record" || failed=1
  require_text "canary-approval.json" "normalized public canary approval record" || failed=1
  require_text "support-approval.json" "normalized public support approval record" || failed=1
  reject_text "tar -xzf release/v4-release-evidence.tar.gz" "unsafe v4 evidence extraction" || failed=1
  require_text "official-public-key.raw" "v4 formal public root artifact" || failed=1
  require_text "v4-release-evidence.tar.gz" "v4 evidence release asset" || failed=1

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
  require_live_control_webui_gate || failed=1
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
  require_text '[[ "$(package-build/nftables-forward-agent --version)" == "nftables-forward 1.2.0" ]]' "nftables forward binary version gate" || failed=1
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
  require_text "release_package_ids: \${{ steps.check.outputs.release_package_ids }}" "tag-gate package id output" || failed=1
  require_text "release_package_scope: \${{ steps.check.outputs.release_package_scope }}" "tag-gate package scope output" || failed=1
  require_text "RELEASE_PACKAGE_IDS: \${{ needs.tag-gate.outputs.release_package_ids }}" "stage-selected package publish input" || failed=1
  require_text "has_package()" "stage-selected package build guard" || failed=1
  require_text "Upload declared signed package releases" "stage-selected signed package upload" || failed=1
  require_text "name: signed-plugin-releases" "single declared package artifact" || failed=1
  require_text "Download declared signed plugin packages" "stage-selected package release download" || failed=1
  reject_text "machine-telemetry-signed-release" "legacy unconditional telemetry artifact" || failed=1
  reject_text "nftables-forward-signed-release" "legacy unconditional nftables artifact" || failed=1
  reject_text "gost-mesh-signed-release" "legacy unconditional GOST artifact" || failed=1
  reject_text "nat-egress-signed-release" "legacy unconditional NAT artifact" || failed=1
  require_text "GOST_VERSION: '3.2.6'" "pinned GOST runtime version" || failed=1
  require_text "b39037b0380ea001fb3c0c28441c2e10bfc694f90682739a65b53e55dce5238b" "pinned GOST archive checksum" || failed=1
  require_text "a2aea24efb4597b5f57b35b8e1bbcc59f439b80723854d4371f6828b46682ffb" "pinned GOST binary checksum" || failed=1
  require_text "./cmd/gost-mesh" "GOST mesh Agent release binary build" || failed=1
  require_text "packages/gost-mesh/build.py build" "deterministic GOST mesh package build" || failed=1
  require_text "./cmd/nat-egress" "NAT egress Agent release binary build" || failed=1
  require_text "packages/nat-egress/build.py build" "deterministic NAT egress package build" || failed=1
  require_text "package_requirements=()" "stage-selected package verification accumulator" || failed=1
  require_text "require_package_assets()" "stage-selected package artifact verification" || failed=1
  require_text "release workflow has no artifact verifier for declared package" "fail-closed package verifier coverage" || failed=1
  require_text "The signed official package IDs for this product stage" "stage-scoped release notes" || failed=1
  require_text "canary-only until Secret ID" "GOST mesh stable-release limitation" || failed=1
  require_text "Control to Agent Process E2E" "cross-repository Agent process E2E job" || failed=1
  require_text "ref: c459383955027ee00719fa2dac1a87813c88a511" "pinned Agent fixture commit" || failed=1
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
    outputs:
      release_eligible: ${{ steps.check.outputs.release_eligible }}
      release_package_ids: ${{ steps.check.outputs.release_package_ids }}
      release_package_scope: ${{ steps.check.outputs.release_package_scope }}
    steps:
      - name: Setup Go for v4 route-gate contracts
        uses: actions/setup-go@v6
      - run: |
          [[ "${GITHUB_REF_NAME}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-(alpha|beta|rc)(\.[0-9]+)?)?$ ]]
          stage_output="$(mktemp)"
          python3 config/scripts/check_release_stage.py \
            --tag "${GITHUB_REF_NAME}" \
            --github-output "${stage_output}"
          cat "${stage_output}" >> "$GITHUB_OUTPUT"
          if grep -Fxq "release_eligible=true" "${stage_output}"; then
            python3 config/scripts/check_release_version.py --tag "${GITHUB_REF_NAME}"
            echo "is_release_tag=true" >> "$GITHUB_OUTPUT"
          else
            grep -Fxq "release_eligible=false" "${stage_output}"
            grep -Fxq "release_stage_classification=historical-preview" "${stage_output}"
            echo "is_release_tag=false" >> "$GITHUB_OUTPUT"
            echo "Historical preview tag audited; release-only jobs are disabled: ${GITHUB_REF_NAME}"
          fi

  deploy-script-test:
    steps:
      - run: python3 config/scripts/check_release_stage.py --self-test
      - run: bash scripts/tests/test_identity_bootstrap_install.sh

  frontend-build:
    steps:
      - uses: actions/checkout@v7
      - name: Checkout pinned Agent source for live Control WebUI E2E
        uses: actions/checkout@v7
        with:
          repository: AnixOps/anix-agent
          ref: c459383955027ee00719fa2dac1a87813c88a511
          path: V2bX_AnixOps
      - uses: actions/setup-go@v6
      - uses: actions/setup-python@v6
      - uses: actions/setup-node@v6
      - run: npm ci
      - run: npx playwright install --with-deps chromium
      - name: Run live Control signed WebUI E2E gate
        env:
          ANIXOPS_AGENT_ROOT: ${{ github.workspace }}/V2bX_AnixOps
        run: npx playwright test --config playwright.live-control.config.js

  go-security:
    steps:
      - run: gosec -exclude-generated -exclude-dir=config/scripts/testdata -fmt=json ./...

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
    needs: [backend-build, frontend-build, plugin-package-publish, tag-gate]
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
          [[ "$(package-build/nftables-forward-agent --version)" == "nftables-forward 1.2.0" ]]
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
    env:
      RELEASE_PACKAGE_IDS: ${{ needs.tag-gate.outputs.release_package_ids }}
      RELEASE_PACKAGE_SCOPE: ${{ needs.tag-gate.outputs.release_package_scope }}
    steps:
      - name: Build formal v4 Agent plugin binaries
        run: |
          --platform linux/amd64
          --platform linux/arm64
          formal_agent_args+=(--agent-binary machine-telemetry@linux/amd64=/tmp/machine-telemetry)
      - name: Download checksum-pinned formal v4 GOST runtimes
        run: |
          GOST_LINUX_ARM64_BINARY_SHA256=343c3e003996ca0437b9cc47dd1500cd0475ba09f5a5f17e50851854e06a1ca7
          formal_runtime_args+=(--runtime-binary gost-mesh:gost@linux/amd64=/tmp/gost-linux-amd64)
      - env:
          ANIXOPS_PLUGIN_SIGNING_PRIVATE_KEY: ${{ secrets.ANIXOPS_PLUGIN_SIGNING_PRIVATE_KEY }}
          ANIXOPS_PLUGIN_OFFICIAL_PUBLIC_KEY: ${{ secrets.ANIXOPS_PLUGIN_OFFICIAL_PUBLIC_KEY }}
        run: |
          scripts/sign_plugin_release.sh
          candidate configuration does not match the protected official package root
          unset ANIXOPS_PLUGIN_SIGNING_PRIVATE_KEY ANIXOPS_PLUGIN_OFFICIAL_PUBLIC_KEY
          has_package() { [[ ",${RELEASE_PACKAGE_IDS}," == *",$1,"* ]]; }
          go -C V2bX_AnixOps build -o package-build/gost-mesh-agent ./cmd/gost-mesh
          python3 packages/gost-mesh/build.py build \
            --agent-binary package-build/gost-mesh-agent \
            --gost package-build/gost
          go -C V2bX_AnixOps build ./cmd/nat-egress
          python3 packages/nat-egress/build.py build
      - name: Upload declared signed package releases
        uses: actions/upload-artifact@v7
        with:
          name: signed-plugin-releases

  v4-public-rehearsal:
    name: V4 Public Rehearsal Gates
    needs: [tag-gate, plugin-package-publish]
    if: ${{ needs.tag-gate.outputs.is_release_tag == 'true' }}
    steps:
      - name: Run public v4 rehearsal gates
        run: |
          python3 packages/shared/build_package.py --formal-release
          docker run --rm --detach postgres:16
          bash config/scripts/run_v4_rehearsal.sh \
            --postgres-dsn test-dsn \
            --official-public-key official-public-key.raw
          python3 config/scripts/render_v4_release_evidence.py \
            --postgres-dsn test-dsn \
            --results-output artifacts/v4-rehearsal-results.json

  v4-release-evidence:
    name: V4 Formal Release Evidence
    needs: [tag-gate, plugin-package-publish, v4-public-rehearsal]
    if: ${{ needs.tag-gate.outputs.is_release_tag == 'true' }}
    steps:
      - name: Sign v4 rehearsal evidence
        env:
          ANIXOPS_PLUGIN_SIGNING_PRIVATE_KEY: ${{ secrets.ANIXOPS_PLUGIN_SIGNING_PRIVATE_KEY }}
          ANIXOPS_PLUGIN_OFFICIAL_PUBLIC_KEY: ${{ secrets.ANIXOPS_PLUGIN_OFFICIAL_PUBLIC_KEY }}
          ANIXOPS_V4_CANARY_APPROVAL: ${{ secrets.ANIXOPS_V4_CANARY_APPROVAL }}
          ANIXOPS_V4_SUPPORT_APPROVAL: ${{ secrets.ANIXOPS_V4_SUPPORT_APPROVAL }}
        run: |
          unset ANIXOPS_PLUGIN_SIGNING_PRIVATE_KEY ANIXOPS_PLUGIN_OFFICIAL_PUBLIC_KEY
          --signing-key signing-key.pem
          --canary-evidence canary-approval.json
          --support-evidence support-approval.json
          --results artifacts/v4-rehearsal-results.json
          cp v4-rehearsal-evidence.json.sig canary-approval.json support-approval.json release/
          python3 config/scripts/verify_v4_evidence.py \
            --archive release/v4-release-evidence.tar.gz \
            --trusted-official-public-key official-public-key.raw \
            --release-packages-dir release
          tar -czf v4-release-evidence.tar.gz v4-rehearsal-evidence.json

  cross-repository-agent-e2e:
    name: Control to Agent Process E2E
    steps:
      - uses: actions/checkout@v7
        with:
          repository: AnixOps/anix-agent
          ref: c459383955027ee00719fa2dac1a87813c88a511
          path: V2bX_AnixOps
      - env:
          ANIXOPS_CROSS_REPO_E2E: '1'
        run: |
          go test -v -count=1 ./internal/grpc -run '^Test(KernelOperationBridgeCrossRepositoryAgentProcess|AgentPluginPackageCrossRepositoryE2E)$'

  release:
    needs: [frontend-build, release-binaries, docker, plugin-package-publish, v4-release-evidence, tag-gate]
    if: ${{ needs.tag-gate.outputs.is_release_tag == 'true' }}
    env:
      RELEASE_PACKAGE_IDS: ${{ needs.tag-gate.outputs.release_package_ids }}
      RELEASE_PACKAGE_SCOPE: ${{ needs.tag-gate.outputs.release_package_scope }}
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
      - name: Download declared signed plugin packages
        uses: actions/download-artifact@v8
        with:
          name: signed-plugin-releases
      - run: |
          echo v4-release-evidence.tar.gz
          echo official-public-key.raw
      - run: |
          cp migration-dry-run.txt release/migration-dry-run.txt
          echo "No Local Release Builds" > release/OPERATOR_DEPLOYMENT.md
          echo 'The signed official package IDs for this product stage are attached.' >> release/OPERATOR_DEPLOYMENT.md
          echo 'gost-mesh is canary-only until Secret ID materialization is complete.' >> release/OPERATOR_DEPLOYMENT.md
          cp docs/UPGRADE.md release/UPGRADE.md
          cp packages/machine-telemetry/verify_signature.py release/verify-machine-telemetry-signature.py
          tar -czvf release/anix-control-frontend.tar.gz -C web/public .
          zip -r release/anix-control-frontend.zip web/public
      - name: Generate release notes file
        run: |
          python3 config/scripts/generate_release_notes.py \
            --changelog CHANGELOG.md \
            --output release/RELEASE_NOTES.md
          echo 'The signed official package IDs for this product stage are attached.' >> release/RELEASE_NOTES.md
          echo 'gost-mesh is canary-only until Secret ID materialization is complete.' >> release/RELEASE_NOTES.md
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
          package_requirements=()
          require_package_assets() { :; }
          echo "release workflow has no artifact verifier for declared package"
          python3 config/scripts/verify_release_artifacts.py \
            --require OPERATOR_DEPLOYMENT.md \
            --require UPGRADE.md \
            --require verify-machine-telemetry-signature.py \
            --require RELEASE_NOTES.md \
            --require anix-control-linux-amd64.tar.gz \
            --require anix-control-windows-arm64.exe.zip \
            "${package_requirements[@]}"
      - uses: softprops/action-gh-release@v3
        with:
          files: release/*
          generate_release_notes: true
EOF

  if ! RELEASE_WORKFLOW_PATH="${fixture}" "${BASH_SOURCE[0]}" >/dev/null; then
    echo "self-test failed: complete fixture should pass" >&2
    return 1
  fi

  cp "${fixture}" "${fixture}.missing-formal-v4-agent-mappings"
  sed -i '/formal_agent_args+=(--agent-binary/d' "${fixture}.missing-formal-v4-agent-mappings"
  if RELEASE_WORKFLOW_PATH="${fixture}.missing-formal-v4-agent-mappings" "${BASH_SOURCE[0]}" >/dev/null 2>&1; then
    echo "self-test failed: missing formal v4 Agent mappings should fail" >&2
    return 1
  fi

  cp "${fixture}" "${fixture}.missing-formal-v4-runtime-mappings"
  sed -i '/formal_runtime_args+=(--runtime-binary/d' "${fixture}.missing-formal-v4-runtime-mappings"
  if RELEASE_WORKFLOW_PATH="${fixture}.missing-formal-v4-runtime-mappings" "${BASH_SOURCE[0]}" >/dev/null 2>&1; then
    echo "self-test failed: missing formal v4 runtime mappings should fail" >&2
    return 1
  fi

  cp "${fixture}" "${fixture}.missing-docker-package-dependency"
  sed -i 's/needs: \[backend-build, frontend-build, plugin-package-publish, tag-gate\]/needs: [backend-build, frontend-build, tag-gate]/' "${fixture}.missing-docker-package-dependency"
  if RELEASE_WORKFLOW_PATH="${fixture}.missing-docker-package-dependency" "${BASH_SOURCE[0]}" >/dev/null 2>&1; then
    echo "self-test failed: Docker release push without signed package publish dependency should fail" >&2
    return 1
  fi

  cp "${fixture}" "${fixture}.missing-platform"
  sed -i '/goos: darwin/{N;N;d;}' "${fixture}.missing-platform"
  if RELEASE_WORKFLOW_PATH="${fixture}.missing-platform" "${BASH_SOURCE[0]}" >/dev/null 2>&1; then
    echo "self-test failed: missing release platform should fail" >&2
    return 1
  fi

  cp "${fixture}" "${fixture}.missing-stage-contract"
  sed -i '/check_release_stage.py/d' "${fixture}.missing-stage-contract"
  if RELEASE_WORKFLOW_PATH="${fixture}.missing-stage-contract" "${BASH_SOURCE[0]}" >/dev/null 2>&1; then
    echo "self-test failed: missing release-stage contract gate should fail" >&2
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

  cp "${fixture}" "${fixture}.missing-live-control-webui-gate"
  sed -i '/playwright.live-control.config.js/d' "${fixture}.missing-live-control-webui-gate"
  if RELEASE_WORKFLOW_PATH="${fixture}.missing-live-control-webui-gate" "${BASH_SOURCE[0]}" >/dev/null 2>&1; then
    echo "self-test failed: missing live Control WebUI gate should fail" >&2
    return 1
  fi

  cp "${fixture}" "${fixture}.missing-nftables-version-gate"
  sed -i '/nftables-forward-agent --version/d' "${fixture}.missing-nftables-version-gate"
  if RELEASE_WORKFLOW_PATH="${fixture}.missing-nftables-version-gate" "${BASH_SOURCE[0]}" >/dev/null 2>&1; then
    echo "self-test failed: missing nftables forward binary version gate should fail" >&2
    return 1
  fi

  cp "${fixture}" "${fixture}.missing-signed-plugin-publish"
  sed -i '/plugin-package-publish:/,/cross-repository-agent-e2e/d;/Publish Signed Official Plugin Packages/d;/ANIXOPS_PLUGIN_SIGNING_PRIVATE_KEY/d;/scripts\/sign_plugin_release.sh/d;/machine-telemetry-signed-release/d;/nftables-forward-signed-release/d;/anixops-machine-telemetry-1.1.0.SHA256SUMS.txt/d;/anixops-nftables-forward-1.2.0.SHA256SUMS.txt/d' "${fixture}.missing-signed-plugin-publish"
  if RELEASE_WORKFLOW_PATH="${fixture}.missing-signed-plugin-publish" "${BASH_SOURCE[0]}" >/dev/null 2>&1; then
    echo "self-test failed: missing signed plugin publish gate should fail" >&2
    return 1
  fi

  cp "${fixture}" "${fixture}.missing-v4-evidence"
  sed -i '/  v4-release-evidence:/,/  cross-repository-agent-e2e:/d' "${fixture}.missing-v4-evidence"
  if RELEASE_WORKFLOW_PATH="${fixture}.missing-v4-evidence" "${BASH_SOURCE[0]}" >/dev/null 2>&1; then
    echo "self-test failed: missing v4 formal evidence job should fail" >&2
    return 1
  fi

  cp "${fixture}" "${fixture}.missing-v4-root-pin"
  sed -i '/ANIXOPS_PLUGIN_OFFICIAL_PUBLIC_KEY/d' "${fixture}.missing-v4-root-pin"
  if RELEASE_WORKFLOW_PATH="${fixture}.missing-v4-root-pin" "${BASH_SOURCE[0]}" >/dev/null 2>&1; then
    echo "self-test failed: missing protected v4 root input should fail" >&2
    return 1
  fi

  cp "${fixture}" "${fixture}.v4-job-service"
  sed -i '/name: V4 Public Rehearsal Gates/a\    services:' "${fixture}.v4-job-service"
  if RELEASE_WORKFLOW_PATH="${fixture}.v4-job-service" "${BASH_SOURCE[0]}" >/dev/null 2>&1; then
    echo "self-test failed: v4 job-level service should fail" >&2
    return 1
  fi

  cp "${fixture}" "${fixture}.missing-v4-public-isolation"
  sed -i 's/needs: \[tag-gate, plugin-package-publish, v4-public-rehearsal\]/needs: [tag-gate, plugin-package-publish]/' "${fixture}.missing-v4-public-isolation"
  if RELEASE_WORKFLOW_PATH="${fixture}.missing-v4-public-isolation" "${BASH_SOURCE[0]}" >/dev/null 2>&1; then
    echo "self-test failed: v4 evidence signing without an isolated public rehearsal job should fail" >&2
    return 1
  fi

  cp "${fixture}" "${fixture}.missing-v4-package-parity"
  sed -i '/--release-packages-dir release/d' "${fixture}.missing-v4-package-parity"
  if RELEASE_WORKFLOW_PATH="${fixture}.missing-v4-package-parity" "${BASH_SOURCE[0]}" >/dev/null 2>&1; then
    echo "self-test failed: missing published v4 package parity verification should fail" >&2
    return 1
  fi

  cp "${fixture}" "${fixture}.public-v4-secret"
  sed -i '/name: Run public v4 rehearsal gates/a\          ANIXOPS_PLUGIN_SIGNING_PRIVATE_KEY=leak' "${fixture}.public-v4-secret"
  if RELEASE_WORKFLOW_PATH="${fixture}.public-v4-secret" "${BASH_SOURCE[0]}" >/dev/null 2>&1; then
    echo "self-test failed: public v4 rehearsal secret input should fail" >&2
    return 1
  fi

  cp "${fixture}" "${fixture}.private-v4-test-runtime"
  sed -i '/--results artifacts\/v4-rehearsal-results.json/a\          --postgres-dsn test-dsn' "${fixture}.private-v4-test-runtime"
  if RELEASE_WORKFLOW_PATH="${fixture}.private-v4-test-runtime" "${BASH_SOURCE[0]}" >/dev/null 2>&1; then
    echo "self-test failed: private v4 signing test runtime should fail" >&2
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
