#!/usr/bin/env python3
"""Keep business background workers out of the kernel.

Background workers of business domains belong in package hosts. The kernel
still starts seven legacy domain workers; this gate lists them and fails when
a new one appears in cmd/server, or when a listed one is gone but still
listed, so the list can only shrink as domains are extracted.
"""
from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parents[2]

# Kernel workers that stay in the kernel. The order payment reconciler
# completes orders through the kernel's own plan grant (v2_user), which no
# package writes (docs/architecture/order-service.md).
KERNEL_WORKERS = frozenset({
    "NewOrderPaymentReconciler",
    "NewTopologyDeploymentExecutor",
})

# Legacy domain workers still started by the kernel. Remove an entry when its
# domain moves into a package host; never add one.
LEGACY_DOMAIN_WORKERS = frozenset({
    "NewForwardAgentBridgeWorker",
    "NewForwardAnsibleStatsWorker",
    "NewForwardFlowResetWorker",
    "NewForwardGostStatsWorker",
    "NewForwardLatencyProber",
    "NewNodeMonthlyResetWorker",
    "NewPanelForwardRuntimeJobExecutor",
})
MAX_LEGACY_DOMAIN_WORKERS = 7

WORKER_CONSTRUCTOR = re.compile(
    r"\bservice\.(New[A-Za-z0-9]*(?:Worker|Executor|Prober|Poller|Scheduler|Reconciler|Watcher))\("
)


def worker_constructors(server_root: Path) -> dict[str, list[str]]:
    found: dict[str, list[str]] = {}
    for source in sorted(server_root.glob("*.go")):
        if source.name.endswith("_test.go"):
            continue
        for number, line in enumerate(source.read_text(encoding="utf-8").splitlines(), start=1):
            for match in WORKER_CONSTRUCTOR.finditer(line):
                found.setdefault(match.group(1), []).append(f"{source.name}:{number}")
    return found


def check(server_root: Path) -> list[str]:
    errors: list[str] = []
    if len(LEGACY_DOMAIN_WORKERS) > MAX_LEGACY_DOMAIN_WORKERS:
        errors.append("the legacy domain worker list may only shrink")
    found = worker_constructors(server_root)
    for name, places in sorted(found.items()):
        if name not in KERNEL_WORKERS and name not in LEGACY_DOMAIN_WORKERS:
            errors.append(
                f"new domain worker {name} in {', '.join(places)}: business workers belong in a package host"
            )
    for name in sorted(LEGACY_DOMAIN_WORKERS - set(found)):
        errors.append(f"legacy domain worker {name} is no longer started; remove it from LEGACY_DOMAIN_WORKERS")
    return errors


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--server", type=Path, default=REPO_ROOT / "cmd" / "server")
    arguments = parser.parse_args()
    errors = check(arguments.server)
    if errors:
        for error in errors:
            print(f"plugin-only worker gate: {error}", file=sys.stderr)
        return 1
    print(f"plugin-only worker gate passed ({len(LEGACY_DOMAIN_WORKERS)} legacy domain workers remain)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
