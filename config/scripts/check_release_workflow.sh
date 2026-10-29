#!/usr/bin/env bash
# Guard the GitHub Actions release workflow contract.

set -euo pipefail

REPO_ROOT_DEFAULT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
REPO_ROOT="${RELEASE_WORKFLOW_REPO_ROOT:-${REPO_ROOT_DEFAULT}}"
WORKFLOW_PATH="${RELEASE_WORKFLOW_PATH:-${REPO_ROOT}/.github/workflows/ci.yml}"

# Jobs that must pass before a tag is built, signed or published. Every one
# must run on a tag push: GitHub skips a job whose need was skipped. Smoke Tests
# is left out because Backend Tests already runs its packages. E2E Tests is in
# the list because Backend Tests excludes internal/tests/e2e.
RELEASE_BINARY_GATES=(
  go-quality
  go-lint
  go-security
  backend-test
  postgres-stats-test
  migration-dry-run-test
  postgres-restore-rehearsal
  plugin-package-release-test
  plugin-package-publish
  forward-runtime-test
  grpc-test
  cross-repository-agent-e2e
  e2e-test
  cmd-test
  tag-gate
  release-workflow-policy
  deploy-script-test
  package-storage-postgres
  docker-smoke
  kubernetes-smoke
)
# The signing job must not run before the Go gates have passed.
PLUGIN_PACKAGE_PUBLISH_GATES=(
  go-quality
  go-security
  backend-test
)

usage() {
  cat <<'EOF'
Usage: config/scripts/check_release_workflow.sh [--self-test|--help]

Statically checks the release workflow essentials: a version tag (with an
optional alpha, beta or rc suffix) that must match the tree's declared version,
tests gating the release, every official package built in the slim built-in
format and signed with the protected official root before the binaries embed
it, linux/amd64 and linux/arm64 binaries only, a signed multi-architecture GHCR
image with SBOM and provenance, a source SBOM, checksums signed with the official
root (SHA256SUMS.txt.sig), no package asset, and a GitHub Release whose body is
the tag's CHANGELOG section. See docs/RELEASING.md.
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

# Every anix-agent checkout takes its ref from the one AGENT_REF in the top-level
# env. A checkout left on a raw commit would build and test a different Agent
# than the pin says, and the pin check above would not notice.
require_agent_checkouts_use_pin() {
  local checkouts
  local pinned

  checkouts="$(grep -cF -- "repository: AnixOps/anix-agent" "${WORKFLOW_PATH}" || true)"
  pinned="$(grep -cF -- 'ref: ${{ env.AGENT_REF }}' "${WORKFLOW_PATH}" || true)"
  if [[ "${checkouts}" -gt 0 && "${checkouts}" == "${pinned}" ]]; then
    echo "ok: all ${checkouts} Agent checkouts use AGENT_REF"
    return 0
  fi

  fail "${checkouts} Agent checkouts but ${pinned} use 'ref: \${{ env.AGENT_REF }}'"
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

frontend_checks_block() {
  awk '
    /^  frontend-checks:/ { in_job = 1; next }
    in_job && /^  [A-Za-z0-9_-]+:/ { exit }
    in_job { print }
  ' "${WORKFLOW_PATH}"
}

require_live_control_webui_gate() {
  local block
  block="$(frontend_checks_block)"
  if [[ -z "${block}" ]]; then
    fail "frontend-checks job is missing"
    return 1
  fi

  local required
  for required in \
    "actions/checkout@v7" \
    "actions/setup-go@v6" \
    "actions/setup-python@v6" \
    "actions/setup-node@v6" \
    "npm ci" \
    "npx playwright install --with-deps chromium" \
    "npx playwright test --config playwright.live-control.config.js"; do
    if ! grep -Fq -- "${required}" <<<"${block}"; then
      fail "frontend-checks live Control WebUI gate is missing: ${required}"
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
  require_text "AGENT_REF: 'ef1c6a05cb20368aabbf4094074b1d0619a59fdc'" "pinned Agent source commit" || failed=1
  require_agent_checkouts_use_pin || failed=1
  require_job_text plugin-package-release-test "govulncheck ./cmd/nftables-forward" "pinned Agent plugin vulnerability scan" || failed=1
  require_text "-exclude-dir=config/scripts/testdata" "full gosec excludes the non-compiling AST fixture directory" || failed=1
  # The required frontend job audits shipped dependencies only, strictly. The
  # development tools and their dated waivers belong to the nightly security
  # workflow, so an advisory with no fix cannot block unrelated pull requests.
  require_named_step_text frontend-checks "Audit shipped frontend dependencies" "npm audit --omit=dev --audit-level=moderate" "required frontend job audits shipped dependencies strictly" || failed=1
  reject_job_text frontend-checks "audit:check" "development-dependency waivers in the required frontend job" || failed=1
  require_text "scripts/tests/test_plugin_execution_install.sh" "package execution installer regression test" || failed=1

  # Tests gate the release.
  for dependency in "${RELEASE_BINARY_GATES[@]}"; do
    require_release_job_dependency "${dependency}" || failed=1
  done
  for dependency in "${PLUGIN_PACKAGE_PUBLISH_GATES[@]}"; do
    require_job_dependency plugin-package-publish "${dependency}" || failed=1
  done
  require_live_control_webui_gate || failed=1
  # "Backend Tests" aggregates its shards: it must run when a shard fails and
  # fail unless every shard passed.
  require_job_dependency backend-test backend-test-shard || failed=1
  require_job_text backend-test '!cancelled()' "Backend Tests runs when a shard fails" || failed=1
  require_job_text backend-test 'test "${SHARDS_RESULT}" = success' "Backend Tests requires every shard to pass" || failed=1
  require_job_text backend-test "--merge-coverage coverage.out" "Backend Tests merges the shard coverage profiles" || failed=1

  # Official packages: all of them, in the built-in format, signed with the
  # protected root. They are built before the binaries, which embed them.
  require_job_text plugin-package-publish "packages/shared/build_package.py" "official package builder" || failed=1
  require_job_text plugin-package-publish "--all" "every official package is built" || failed=1
  require_job_text plugin-package-publish "--builtin" "built-in package format" || failed=1
  require_job_text plugin-package-publish "--formal-release" "signed package build" || failed=1
  require_job_text plugin-package-publish "--verify-release" "signed package verification" || failed=1
  require_job_text plugin-package-publish "ANIXOPS_PLUGIN_SIGNING_PRIVATE_KEY" "protected package signing key" || failed=1
  require_job_text plugin-package-publish "candidate configuration does not match the protected official package root" "signing root matches the shipped trust root" || failed=1
  require_job_text plugin-package-publish "unset ANIXOPS_PLUGIN_SIGNING_PRIVATE_KEY ANIXOPS_PLUGIN_OFFICIAL_PUBLIC_KEY" "signing secret environment cleanup" || failed=1
  require_job_text plugin-package-publish "name: builtin-packages" "built-in package set artifact" || failed=1
  require_job_text plugin-package-publish "path: release-assets/" "signed release asset artifact holds the release assets only" || failed=1
  require_job_text plugin-package-publish "name: signed-plugin-releases" "signed release asset artifact" || failed=1
  reject_job_text plugin-package-publish "--release-archive" "per-release packages archive" || failed=1
  reject_job_text plugin-package-publish "--agent-binary" "Agent plugin payloads in the release" || failed=1
  require_job_text release-binaries "name: builtin-packages" "release binaries download the built-in package set" || failed=1
  require_job_text release-binaries "path: internal/builtinpkg/dist/" "release binaries embed the built-in package set" || failed=1
  require_named_step_text release-binaries "Verify the built-in package set" "--verify-release" "release binaries verify the built-in set against the shipped root" || failed=1

  # Binaries carry the Agent release tag they were released with: a Control
  # patch release can ship an unchanged Agent, so the installer must not ask
  # for "v" + the Control version.
  require_named_step_text release-binaries "Build binary" "config/AGENT_RELEASE" "release binaries read the pinned Agent release tag" || failed=1
  require_named_step_text release-binaries "Build binary" "-X main.agentRelease=\${AGENT_RELEASE}" "release binaries carry the pinned Agent release tag" || failed=1
  for pair in \
    linux/amd64 \
    linux/arm64; do
    require_release_binary_pair "${pair}" || failed=1
  done
  # Control runs its built-in package hosts as Linux processes.
  if [[ "$(release_binary_pairs | wc -l)" -ne 2 ]]; then
    echo "release binary matrix pairs found:" >&2
    release_binary_pairs >&2
    fail "the release binary matrix must hold linux/amd64 and linux/arm64 only" || failed=1
  else
    echo "ok: release binary matrix is linux/amd64 and linux/arm64 only"
  fi
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
  reject_job_text docker "bootstrap/identity-platform" "identity bootstrap package in the image" || failed=1
  require_job_text docker "bootstrap/agent-install" "Agent installer signature in the image" || failed=1
  require_job_text plugin-package-publish "release-assets/agent-install.sh.sig" "signed Agent installer" || failed=1
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
  require_named_step_text release "Sign the release checksums" "-rawin -in release/SHA256SUMS.txt" "signed release checksums" || failed=1
  require_named_step_text release "Sign the release checksums" "release/SHA256SUMS.txt.sig" "release checksum signature asset" || failed=1
  require_job_text release "config/scripts/verify_release_artifacts.py" "release artifact verification" || failed=1
  require_job_text release "--builtin-packages builtin-packages" "release manifest lists the built-in packages" || failed=1
  require_job_text release "--official-public-key release/official-public-key.raw" "release checksums verified against the shipped root" || failed=1
  for asset in \
    official-public-key.raw \
    agent-install.sh \
    agent-install.sh.sig \
    SHA256SUMS.txt.sig \
    anix-control-frontend.tar.gz \
    anix-control-linux-amd64.tar.gz \
    anix-control-linux-arm64.tar.gz; do
    require_job_text release "--require ${asset}" "release asset ${asset}" || failed=1
  done
  # The packages are inside the binaries: no package asset is published.
  reject_job_text release "--require anix-control-packages-" "packages archive release asset" || failed=1
  reject_job_text release '--require "identity-platform-' "identity bootstrap package release asset" || failed=1
  reject_job_text release "--verify-release-archive" "packages archive verification" || failed=1
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

# Renames one entry of a job's needs list to a job that does not exist, in a
# copy of the real workflow, for each gate in turn. The check must fail for
# every one, so a removed gate cannot slip through.
self_test_needs() {
  local tmpdir="$1"
  local job_name="$2"
  shift 2
  local gate

  for gate in "$@"; do
    sed "/^  ${job_name}:\$/,/^  [[:alnum:]_-]*:\$/ { /needs: \\[/ s/\\([[ ]\\)${gate}\\([],]\\)/\\1removed-job\\2/; }" "${WORKFLOW_PATH}" > "${tmpdir}/ci.yml"
    if cmp -s "${WORKFLOW_PATH}" "${tmpdir}/ci.yml"; then
      echo "self-test failed: ${gate} is not in the needs list of ${job_name}" >&2
      return 1
    fi
    if WORKFLOW_PATH="${tmpdir}/ci.yml" check_release_workflow >/dev/null 2>&1; then
      echo "self-test failed: dropping ${gate} from ${job_name} should fail the check" >&2
      return 1
    fi
  done
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
    "s/-X main.agentRelease=/-X main.agentReleaseX=/" \
    "s#< config/AGENT_RELEASE#< config/agent-release#" \
    "s/npm audit --omit=dev --audit-level=moderate/npm run audit:check/" \
    "s/AGENT_REF: '/AGENT_REF_X: '/" \
    '0,/ref: \${{ env.AGENT_REF }}/s//ref: 0000000000000000000000000000000000000000/' \
    "s#govulncheck ./cmd/nftables-forward#echo ./cmd/nftables-forward#" \
    "s/anix-control-linux-amd64.tar.gz/anix-control-linux.tar.gz/g" \
    "s/go-security, backend-test/backend-test/" \
    "s/--builtin-packages builtin-packages/--packages-dir builtin-packages/" \
    "s#release/SHA256SUMS.txt.sig#release/SHA256SUMS.sig#g" \
    "s#path: internal/builtinpkg/dist/#path: builtin-packages/#" \
    "s/            --builtin \\\\/            --full \\\\/" \
    "s/            goarch: arm64/            goarch: arm64\n          - goos: windows\n            goarch: amd64/" \
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

  self_test_needs "${tmpdir}" release-binaries "${RELEASE_BINARY_GATES[@]}" || return 1
  self_test_needs "${tmpdir}" plugin-package-publish "${PLUGIN_PACKAGE_PUBLISH_GATES[@]}" || return 1

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
