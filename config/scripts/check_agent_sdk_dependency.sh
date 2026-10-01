#!/usr/bin/env bash
# Control owns the Agent contract (anix.agent.v1) in its SDK module:
# sdk/api/agent/v1, sdk/agentcontrol and sdk/plugincontrol. It must not depend
# on any anix-agent module: not the Agent runtime (anix-agent/v4), and not the
# Agent SDK (anix-agent/sdk), which is frozen at v1.1.0 and would register a
# second copy of anix.agent.v1.
set -euo pipefail

forbidden='^github\.com/AnixOps/anix-agent(/v4|/sdk)?(/|$)'
go_workspace="${GOWORK:-off}"

if [[ -d "api/grpc/agent/v1" ]]; then
  echo "Control local Agent v1 protobuf must be removed in favor of the SDK." >&2
  exit 1
fi

if [[ ! -f "sdk/api/agent/v1/agent.proto" ]]; then
  echo "The Agent contract must live in the SDK module at sdk/api/agent/v1." >&2
  exit 1
fi

if grep -Eq '^[[:space:]]*(require[[:space:]]+)?github\.com/AnixOps/anix-agent(/v4|/sdk)?([[:space:]]|$)' go.mod sdk/go.mod; then
  echo "Control go.mod and sdk/go.mod must not require an anix-agent module." >&2
  exit 1
fi

modules="$(GOWORK="${go_workspace}" go list -m -f '{{if not .Main}}{{.Path}}{{end}}' all)"
packages="$(GOWORK="${go_workspace}" go list -deps -test -f '{{.ImportPath}}' ./...)"
sdk_packages="$(cd sdk && GOWORK="${go_workspace}" go list -deps -test -f '{{.ImportPath}}' ./...)"
unexpected="$(printf '%s\n%s\n%s\n' "${modules}" "${packages}" "${sdk_packages}" | grep -E "${forbidden}" | sort -u || true)"

if [[ -n "${unexpected}" ]]; then
  echo "Control must use the Agent contract from its own SDK module, not anix-agent:" >&2
  echo "${unexpected}" >&2
  exit 1
fi
