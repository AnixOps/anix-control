#!/usr/bin/env python3
"""Classify a change for the tiered CI pipeline.

Pull requests run the fast lane: the required checks plus the heavy jobs
whose paths changed. Everything else runs the full lane: go_dev pushes,
tags, the nightly schedule, manual runs, pull requests labelled ci:full, and
changes to the workflow, the Go dependencies or this classifier.

Prints GitHub output lines (name=true|false):
  full     run every job
  code     anything besides documentation (Go checks and tests)
  web      the frontend
  db       schemas, storage and migrations (the PostgreSQL jobs)
  modules  the module runtime, identity, images and deployment (Docker and
           Kubernetes smokes)
  forward  forwarding (the forward runtime regression)
  agent    the Agent channel (gRPC and the cross-repository Agent E2E)
  packages official package sources (the package release contracts)
"""

from __future__ import annotations

import argparse
import fnmatch
import subprocess
import sys

CLASSES = ("code", "web", "db", "modules", "forward", "agent", "packages")

# Any of these makes the change run the full lane.
FULL_PATTERNS = (
    ".github/*",
    "go.mod",
    "go.sum",
    "sdk/go.mod",
    "sdk/go.sum",
    "identity/go.mod",
    "identity/go.sum",
    "config/scripts/classify_changes.py",
)

DOC_PATTERNS = ("docs/*", "*.md", "LICENSE*")

CLASS_PATTERNS: dict[str, tuple[str, ...]] = {
    "web": ("web/*",),
    "db": (
        "internal/model/*",
        "internal/database/*",
        "internal/packagestore/*",
        "internal/service/*",
        "internal/subscriber/*",
        "internal/tests/*",
        "internal/kernelidentity/*",
        "internal/identity*",
        "cmd/sqlite2postgres/*",
        "cmd/migrate/*",
        "scripts/postgres_restore_rehearsal.sh",
        "packages/*/migrations/*",
        # Parity tests compare legacy handlers with package-native ones on
        # PostgreSQL as well.
        "packages/*/native/*",
        "internal/handler/*",
        "internal/kernelsubscriber/*",
        "internal/kernelorder/*",
        "internal/kernelsettings/*",
        "sdk/v2compat/*",
        "packages/identity-platform/*",
        "identity/*",
        "sdk/packagestoresdk/*",
    ),
    "modules": (
        "Dockerfile*",
        "docker-compose*",
        "config/deploy/*",
        "config/scripts/modules_*",
        "config/scripts/identity_cutover_acceptance.sh",
        "cmd/server/*",
        "internal/router/*",
        "internal/config/*",
        "internal/compat/*",
        "internal/authn/*",
        "internal/pluginhost/*",
        "internal/plugincontrol/*",
        "internal/packagebridge/*",
        "internal/modulepki/*",
        "internal/kernelidentity/*",
        "internal/identity*",
        "identity/*",
        "sdk/*",
        "packages/identity-platform/*",
        "packages/shared/*",
    ),
    "forward": (
        "internal/service/forward*",
        "internal/handler/forward*",
        "internal/gost/*",
        "packages/forward/*",
        "packages/gost-mesh/*",
        "packages/nftables-forward/*",
        "packages/nat-egress/*",
    ),
    "agent": (
        "internal/grpc/*",
        "api/*",
        "contracts/agent/*",
        "internal/handler/agent*",
        "internal/service/agent*",
        "internal/plugincontrol/*",
        "packages/machine-telemetry/*",
    ),
    "packages": ("packages/*", "scripts/sign_plugin_release.sh"),
}


def matches(path: str, patterns: tuple[str, ...]) -> bool:
    return any(fnmatch.fnmatchcase(path, pattern) for pattern in patterns)


def classify(paths: list[str], *, event: str, full_label: bool) -> dict[str, bool]:
    result = {name: False for name in ("full", *CLASSES)}
    if event != "pull_request" or full_label or not paths or any(matches(path, FULL_PATTERNS) for path in paths):
        return {name: True for name in result}
    for path in paths:
        if matches(path, DOC_PATTERNS):
            continue
        result["code"] = True
        for name, patterns in CLASS_PATTERNS.items():
            if matches(path, patterns):
                result[name] = True
    return result


def changed_paths(base: str, head: str) -> list[str]:
    output = subprocess.run(
        ["git", "diff", "--name-only", f"{base}...{head}"], check=True, text=True, capture_output=True
    ).stdout
    return [line for line in output.splitlines() if line]


def self_test() -> None:
    def check(paths: list[str], expected: set[str], *, event: str = "pull_request", label: bool = False) -> None:
        result = classify(paths, event=event, full_label=label)
        on = {name for name, value in result.items() if value}
        assert on == expected, f"{paths} ({event}, label={label}): {sorted(on)} != {sorted(expected)}"

    everything = {"full", *CLASSES}
    check(["docs/RELEASING.md", "CHANGELOG.md"], set())
    check(["web/src/App.vue"], {"code", "web"})
    check(["internal/handler/ticket.go"], {"code", "db"})
    check(["internal/middleware/audit_log.go"], {"code"})
    check(["packages/plan/native/plans.go"], {"code", "db", "packages"})
    check(["internal/service/order_service.go"], {"code", "db"})
    check(["internal/pluginhost/remote.go"], {"code", "modules"})
    check(["internal/service/forward_panel_flow.go"], {"code", "db", "forward"})
    check(["internal/grpc/node_server.go"], {"code", "agent"})
    check(["packages/knowledge/compat/v2-routes.json"], {"code", "packages"})
    check(["packages/order/native/orders.go"], {"code", "db", "packages"})
    check(["packages/identity-platform/native/auth.go"], {"code", "db", "modules", "packages"})
    check([".github/workflows/ci.yml"], everything)
    check(["go.sum"], everything)
    check(["internal/handler/ticket.go"], everything, event="push")
    check(["internal/handler/ticket.go"], everything, event="schedule")
    check(["internal/handler/ticket.go"], everything, label=True)
    check([], everything)
    print("classify_changes self-test passed")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--event", default="")
    parser.add_argument("--base", default="")
    parser.add_argument("--head", default="HEAD")
    parser.add_argument("--full-label", default="false")
    parser.add_argument("--self-test", action="store_true")
    args = parser.parse_args()
    if args.self_test:
        self_test()
        return 0
    paths = changed_paths(args.base, args.head) if args.event == "pull_request" and args.base else []
    result = classify(paths, event=args.event, full_label=args.full_label == "true")
    for name, value in result.items():
        print(f"{name}={'true' if value else 'false'}")
    print(f"classified {len(paths)} changed paths", file=sys.stderr)
    return 0


if __name__ == "__main__":
    sys.exit(main())
