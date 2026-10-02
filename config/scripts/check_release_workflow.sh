#!/usr/bin/env bash
# Guard the GitHub Actions release workflow contract.

set -euo pipefail

REPO_ROOT_DEFAULT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
REPO_ROOT="${RELEASE_WORKFLOW_REPO_ROOT:-${REPO_ROOT_DEFAULT}}"
WORKFLOW_PATH="${RELEASE_WORKFLOW_PATH:-${REPO_ROOT}/.github/workflows/ci.yml}"

usage() {
  cat <<'EOF'
Usage: config/scripts/check_release_workflow.sh [--self-test|--help]

Statically checks the release workflow essentials: a version tag (with an
optional alpha, beta or rc suffix) that must match the tree's declared version,
tests gating the release, every official package signed with the protected
official root and shipped in one signed packages archive (plus the identity
bootstrap package the installer and image use), multi-platform binaries, a signed multi-architecture GHCR image
with SBOM and provenance, a source SBOM, checksums, and a GitHub Release whose
body is the tag's CHANGELOG section. See docs/RELEASING.md.
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

require_job_text() {
  local job_name="$1"
  local needle="$2"
  local description="$3"
  local block

  block="$(job_block "${job_name}")"
  if grep -Fq -- "${needle}" <<<"${block}"; then
    echo "ok: ${description}"
    return 0
  fi

  fail "missing ${description} in ${job_name} job: ${needle}"
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

check_release_workflow() {
  local failed=0

  [[ -f "${WORKFLOW_PATH}" ]] || fail "workflow file not found: ${WORKFLOW_PATH}" || return 1

  # Tag and version.
  require_text "tags: [ 'v*.*.*' ]" "tag trigger pattern" || failed=1
  require_text '^v[0-9]+\.[0-9]+\.[0-9]+(-(alpha|beta|rc)(\.[0-9]+)?)?$' "stable and prerelease tag gate" || failed=1
  require_job_text tag-gate "python3 config/scripts/check_release_version.py --tag" "release tag/source version consistency gate" || failed=1
  require_text "needs.tag-gate.outputs.is_release_tag == 'true'" "release-only job gate" || failed=1

  # Pinned toolchains and inputs.
  reject_job_text go-quality "arduino/setup-protoc@v3" "third-party protoc setup action" || failed=1
  require_named_step_text go-quality "Install pinned protoc" "protocolbuffers/protobuf/releases/download/v29.2/protoc-29.2-linux-x86_64.zip" "pinned official protoc download" || failed=1
  require_named_step_text go-quality "Install pinned protoc" "sha256sum -c" "pinned protoc checksum verification" || failed=1
  require_text "GOST_VERSION: '3.2.6'" "pinned GOST runtime version" || failed=1
  require_text "GOST_LINUX_ARM64_BINARY_SHA256" "pinned arm64 GOST binary checksum" || failed=1
  require_text "ref: c459383955027ee00719fa2dac1a87813c88a511" "pinned Agent source commit" || failed=1
  require_text "-exclude-dir=config/scripts/testdata" "full gosec excludes the non-compiling AST fixture directory" || failed=1
  require_text "scripts/tests/test_identity_bootstrap_install.sh" "identity bootstrap installer regression test" || failed=1

  # Tests gate the release.
  for dependency in \
    go-quality \
    go-lint \
    go-security \
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
  require_live_control_webui_gate || failed=1
  # "Backend Tests" aggregates its shards: it must run when a shard fails and
  # fail unless every shard passed.
  require_job_dependency backend-test backend-test-shard || failed=1
  require_job_text backend-test '!cancelled()' "Backend Tests runs when a shard fails" || failed=1
  require_job_text backend-test 'test "${SHARDS_RESULT}" = success' "Backend Tests requires every shard to pass" || failed=1
  require_job_text backend-test "--merge-coverage coverage.out" "Backend Tests merges the shard coverage profiles" || failed=1

  # Official packages: all of them, signed with the protected root.
  require_job_text plugin-package-publish "packages/shared/build_package.py" "official package builder" || failed=1
  require_job_text plugin-package-publish "--all" "every official package is built" || failed=1
  require_job_text plugin-package-publish "--formal-release" "signed package build" || failed=1
  require_job_text plugin-package-publish "--verify-release" "signed package verification" || failed=1
  require_job_text plugin-package-publish "ANIXOPS_PLUGIN_SIGNING_PRIVATE_KEY" "protected package signing key" || failed=1
  require_job_text plugin-package-publish "candidate configuration does not match the protected official package root" "signing root matches the shipped trust root" || failed=1
  require_job_text plugin-package-publish "unset ANIXOPS_PLUGIN_SIGNING_PRIVATE_KEY ANIXOPS_PLUGIN_OFFICIAL_PUBLIC_KEY" "signing secret environment cleanup" || failed=1
  require_job_text plugin-package-publish "agent_args+=(--agent-binary" "explicit Agent binary mappings" || failed=1
  require_job_text plugin-package-publish "runtime_args+=(--runtime-binary" "explicit GOST runtime mappings" || failed=1
  require_job_text plugin-package-publish "--platform linux/amd64" "amd64 package platform" || failed=1
  require_job_text plugin-package-publish "--platform linux/arm64" "arm64 package platform" || failed=1
  require_job_text plugin-package-publish "--release-archive release-assets" "signed packages archive" || failed=1
  require_job_text plugin-package-publish "path: release-assets/" "signed package artifact holds the release assets only" || failed=1
  require_job_text plugin-package-publish "name: signed-plugin-releases" "signed package artifact" || failed=1

  # Binaries.
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

  # Container image: supply-chain signed.
  require_job_dependency docker plugin-package-publish || failed=1
  require_job_dependency docker release-binaries || failed=1
  require_job_text docker "ghcr.io/anixops/anix-control" "GHCR release image" || failed=1
  require_job_text docker "target: release" "release image built from the release artifacts" || failed=1
  require_job_text docker "linux/amd64,linux/arm64" "multi-architecture release image" || failed=1
  require_job_text docker "provenance: mode=max" "release image provenance attestation" || failed=1
  require_job_text docker "sbom: true" "release image SBOM attestation" || failed=1
  require_job_text docker "cosign sign" "signed release image digest" || failed=1
  require_job_text docker "identity-platform" "identity bootstrap package in the image" || failed=1
  reject_job_text docker "DOCKER_PASSWORD" "Docker Hub release credentials" || failed=1
  require_text "digest=\${{ steps.build.outputs.digest }}" "Docker digest metadata" || failed=1

  # GitHub Release.
  require_job_dependency release docker || failed=1
  require_job_dependency release plugin-package-publish || failed=1
  require_job_text release "anchore/sbom-action" "source SBOM generation" || failed=1
  require_job_text release "anix-control-source.sbom.spdx.json" "source SBOM release asset" || failed=1
  require_job_text release "config/scripts/generate_release_notes.py" "release notes from the CHANGELOG" || failed=1
  require_job_text release "config/scripts/generate_release_manifest.py" "release manifest" || failed=1
  require_job_text release "sha256sum * > SHA256SUMS.txt" "release checksums" || failed=1
  require_job_text release "config/scripts/verify_release_artifacts.py" "release artifact verification" || failed=1
  require_job_text release "--verify-release-archive" "packages archive signature and contents verification" || failed=1
  for asset in \
    '"anix-control-packages-${version}.tar.gz"' \
    '"anix-control-packages-${version}.tar.gz.sig"' \
    '"identity-platform-${version}.anxp"' \
    '"identity-platform-${version}.manifest.json"' \
    '"identity-platform-${version}.manifest.sig"' \
    anix-control-frontend.tar.gz \
    anix-control-linux-amd64.tar.gz \
    anix-control-linux-arm64.tar.gz \
    anix-control-windows-arm64.exe.zip \
    anix-control-darwin-arm64.tar.gz; do
    require_job_text release "--require ${asset}" "release asset ${asset}" || failed=1
  done
  require_job_text release "softprops/action-gh-release" "GitHub release creation action" || failed=1
  require_job_text release "body_path: release/RELEASE_NOTES.md" "release body from the CHANGELOG" || failed=1
  require_job_text release "prerelease: \${{ contains(github.ref_name, '-') }}" "suffixed tags are prereleases" || failed=1
  require_job_text release "files: release/*" "release asset upload glob" || failed=1

  # One frontend format and one packages archive: no per-package assets.
  reject_job_text release "--require anix-control-frontend.zip" "required frontend zip asset" || failed=1
  reject_job_text release "zip -r release/" "frontend zip release asset" || failed=1
  reject_job_text release '--require "${stem}' "per-package release assets" || failed=1

  # Legacy names stay retired.
  reject_text "v2board-frontend.tar.gz" "legacy frontend release alias" || failed=1
  reject_text "v2board-linux-amd64.tar.gz" "legacy Linux release alias" || failed=1

  return "${failed}"
}

# The self-test breaks one essential at a time in a copy of the real workflow
# and expects the check to fail.
run_self_test() {
  local tmpdir
  tmpdir="$(mktemp -d)"
  trap 'rm -rf "${tmpdir}"' RETURN
  local original="${WORKFLOW_PATH}"
  cp "${original}" "${tmpdir}/ci.yml"

  if ! WORKFLOW_PATH="${tmpdir}/ci.yml" check_release_workflow >/dev/null; then
    echo "self-test failed: the current workflow should pass" >&2
    return 1
  fi

  local mutation
  for mutation in \
    "s/cosign sign/cosign attest/" \
    "s/provenance: mode=max/provenance: false/" \
    "s/--formal-release/--unsigned-release/" \
    "s/check_release_version.py --tag/check_release_version.py --self-test/" \
    "s/body_path: release\/RELEASE_NOTES.md/generate_release_notes: true/" \
    "s/anix-control-linux-amd64.tar.gz/anix-control-linux.tar.gz/g" \
    "s/go-security, backend-test/backend-test/" \
    "s/--verify-release-archive/--skip-release-archive/" \
    's/test "\${SHARDS_RESULT}" = success/true/' \
    's/needs: \[changes, backend-test-shard\]/needs: [changes]/' \
    's/!cancelled() && needs.changes.outputs.code/needs.changes.outputs.code/' \
    "s#tar -czvf release/anix-control-frontend.tar.gz -C web/public .#&\n          zip -r release/anix-control-frontend.zip web/public#"; do
    sed "${mutation}" "${original}" > "${tmpdir}/ci.yml"
    if cmp -s "${original}" "${tmpdir}/ci.yml"; then
      echo "self-test failed: mutation matched nothing: ${mutation}" >&2
      return 1
    fi
    if WORKFLOW_PATH="${tmpdir}/ci.yml" check_release_workflow >/dev/null 2>&1; then
      echo "self-test failed: mutation should fail the check: ${mutation}" >&2
      return 1
    fi
  done

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
