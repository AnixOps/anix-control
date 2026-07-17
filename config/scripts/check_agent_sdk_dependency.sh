#!/usr/bin/env bash
set -euo pipefail

forbidden='^github\.com/AnixOps/anix-agent/v4(/|$)'
go_workspace="${GOWORK:-off}"

if [[ -d "api/grpc/agent/v1" ]]; then
  echo "Control local Agent v1 protobuf must be removed in favor of the SDK." >&2
  exit 1
fi

if grep -Eq '^[[:space:]]*github\.com/AnixOps/anix-agent/v4([[:space:]]|$)' go.mod; then
  echo "Control go.mod must not require anix-agent/v4." >&2
  exit 1
fi

modules="$(GOWORK="${go_workspace}" go list -m -f '{{if not .Main}}{{.Path}}{{end}}' all)"
packages="$(GOWORK="${go_workspace}" go list -deps -f '{{.ImportPath}}' ./...)"
unexpected="$(printf '%s\n%s\n' "${modules}" "${packages}" | grep -E "${forbidden}" || true)"

if [[ -n "${unexpected}" ]]; then
  echo "Control must depend on anix-agent/sdk, not anix-agent/v4:" >&2
  echo "${unexpected}" >&2
  exit 1
fi
