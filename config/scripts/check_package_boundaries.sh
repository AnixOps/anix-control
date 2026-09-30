#!/usr/bin/env bash
# Package hosts (packages/...) and the public SDKs (pkg/...) are built and
# shipped apart from the kernel, so they must not depend on internal/.
# Non-test code may not reach internal/ at all, even transitively. A test may
# import internal/ directly only when it is listed below with a reason.
set -euo pipefail

module='github.com/AnixOps/anix-control/v4'
go_workspace="${GOWORK:-off}"

# "<package> <internal import>": reason.
allowed_test_imports=(
  # The SDK's contract tests run against the kernel's real bridge session.
  "${module}/pkg/packagebridgesdk ${module}/internal/packagebridge"
)

failed=0
packages="$(GOWORK="${go_workspace}" go list ./packages/... ./pkg/...)"

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
      echo "${package} tests import ${import}; add a reasoned entry to ${BASH_SOURCE[0]} or use a pkg/ fixture" >&2
      failed=1
    fi
  done <<< "${test_imports}"
done <<< "${packages}"

if [[ "${failed}" -ne 0 ]]; then
  exit 1
fi
echo "package boundary gate passed ($(wc -l <<< "${packages}") packages)"
