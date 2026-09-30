#!/usr/bin/env bash
# Package hosts (packages/...) and the SDK module (sdk/) are built and shipped
# apart from the kernel.
# - Non-test package code may not reach the kernel's internal/ packages, even
#   transitively. A test may import internal/ directly only when it is listed
#   below with a reason.
# - The SDK module may not depend on the kernel module at all; Go already
#   forbids it from importing the kernel's internal packages.
set -euo pipefail

module='github.com/AnixOps/anix-control/v4'
go_workspace="${GOWORK:-off}"

# "<package> <internal import>": reason.
allowed_test_imports=(
)

failed=0
packages="$(GOWORK="${go_workspace}" go list ./packages/...)"

while IFS= read -r package; do
  [[ -n "${package}" ]] || continue
  production="$(GOWORK="${go_workspace}" go list -deps -f '{{.ImportPath}}' "${package}" | grep -E "^${module}/internal(/|$)" || true)"
  if [[ -n "${production}" ]]; then
    echo "${package} depends on internal packages:" >&2
    printf '  %s\n' ${production} >&2
    failed=1
  fi
  test_imports="$(GOWORK="${go_workspace}" go list -f '{{join .TestImports "\n"}}{{"\n"}}{{join .XTestImports "\n"}}' "${package}" | grep -E "^${module}/internal(/|$)" || true)"
  while IFS= read -r import; do
    [[ -n "${import}" ]] || continue
    allowed=0
    for entry in "${allowed_test_imports[@]}"; do
      if [[ "${entry}" == "${package} ${import}" ]]; then
        allowed=1
      fi
    done
    if [[ "${allowed}" -eq 0 ]]; then
      echo "${package} tests import ${import}; add a reasoned entry to ${BASH_SOURCE[0]} or use an sdk/ fixture" >&2
      failed=1
    fi
  done <<< "${test_imports}"
done <<< "${packages}"

sdk_count=0
if [[ -f sdk/go.mod ]]; then
  sdk_dependencies="$(cd sdk && GOWORK="${go_workspace}" go list -deps -test -f '{{.ImportPath}}' ./... | grep -E "^${module}(/|$)" || true)"
  if [[ -n "${sdk_dependencies}" ]]; then
    echo "the SDK module depends on the kernel module:" >&2
    printf '  %s\n' ${sdk_dependencies} >&2
    failed=1
  fi
  sdk_count="$(cd sdk && GOWORK="${go_workspace}" go list ./... | wc -l)"
fi

# The identity core is product-neutral: besides third-party code it may use
# only the SDK module, never the kernel.
identity_count=0
if [[ -f identity/go.mod ]]; then
  identity_dependencies="$(cd identity && GOWORK="${go_workspace}" go list -deps -test -f '{{.ImportPath}}' ./... | grep -E "^${module}(/|$)" || true)"
  if [[ -n "${identity_dependencies}" ]]; then
    echo "the identity module depends on the kernel module:" >&2
    printf '  %s\n' ${identity_dependencies} >&2
    failed=1
  fi
  identity_count="$(cd identity && GOWORK="${go_workspace}" go list ./... | wc -l)"
fi

if [[ "${failed}" -ne 0 ]]; then
  exit 1
fi
echo "package boundary gate passed ($(wc -l <<< "${packages}") packages, ${sdk_count} SDK packages, ${identity_count} identity packages)"
