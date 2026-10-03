#!/usr/bin/env python3
"""Classify a change for the tiered CI pipeline.

Pull requests run the fast lane: the required checks plus the heavy jobs
whose paths changed. Everything else runs the full lane: go_dev pushes,
tags, the nightly schedule, manual runs, pull requests labelled ci:full, and
changes to the Go dependencies or this classifier.

A pull request that changes .github/workflows/ci.yml runs the full lane
unless the change is confined to the bodies of jobs gated on one class (see
workflow_change_classes); then it runs the classes of the changed jobs and of
the jobs that need them.

Prints GitHub output lines (name=true|false):
  full     run every job
  code     anything besides documentation (Go checks and tests)
  web      the frontend
  db       schemas, storage and migrations (the PostgreSQL jobs)
  modules  the module runtime, identity, images and deployment (Docker and
           Kubernetes smokes)
  forward  forwarding: the v4.1 forward runtime regression and the forward
           SDK's network namespace end-to-end suite (Forward Netns E2E)
  agent    the Agent channel (gRPC and the cross-repository Agent E2E)
  packages official package sources (the package release contracts)
"""

from __future__ import annotations

import argparse
import fnmatch
import re
import subprocess
import sys
from pathlib import Path

CLASSES = ("code", "web", "db", "modules", "forward", "agent", "packages")

# Any of these makes the change run the full lane.
FULL_PATTERNS = (
    ".github/actions/*",
    "go.mod",
    "go.sum",
    "sdk/go.mod",
    "sdk/go.sum",
    "identity/go.mod",
    "identity/go.sum",
    "config/scripts/classify_changes.py",
)

WORKFLOW = ".github/workflows/ci.yml"

# Jobs that steer every other job; any change to them runs the full lane.
WORKFLOW_FULL_JOBS = frozenset({"changes", "tag-gate"})

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
        "internal/kerneltelemetry/*",
        # The node credential split's writer and its PostgreSQL tests.
        "internal/nodesecrets/*",
        "internal/kernelnodeops/*",
        "internal/agentpki/*",
        "sdk/v2compat/*",
        "packages/identity-platform/*",
        "identity/*",
        "sdk/packagestoresdk/*",
        # It splits the PostgreSQL package storage tests into shards.
        "config/scripts/plan_test_shards.py",
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
        # The forward SDK (v4.2): model, planner, drivers and contract.
        "sdk/forward/*",
        "sdk/api/forward/*",
        "contracts/forward/*",
    ),
    "agent": (
        "internal/grpc/*",
        "internal/agentpki/*",
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


_JOB_HEADER = re.compile(r"^  ([A-Za-z0-9_-]+):\s*$")
_CLASS_GATE = re.compile(r"^    if: \$\{\{ needs\.changes\.outputs\.([a-z]+) == 'true' \}\}\s*$")
_YAML_ALIAS = re.compile(r"^\s*(- )?([A-Za-z0-9_.-]+:\s+)?[&*][A-Za-z0-9_-]+(\s|$)|^\s*<<:")


def split_workflow(text: str) -> tuple[str, dict[str, str]] | None:
    """Split the workflow into its text before `jobs:` and one text block per job.

    Returns None when the file does not have the plain layout this relies on
    (top-level `jobs:`, jobs at two-space indent, no YAML anchors).
    """
    lines = text.splitlines()
    if "jobs:" not in lines or any(_YAML_ALIAS.search(line) for line in lines):
        return None
    start = lines.index("jobs:")
    jobs: dict[str, list[str]] = {}
    current: list[str] | None = None
    for line in lines[start + 1 :]:
        header = _JOB_HEADER.match(line)
        if header:
            if header.group(1) in jobs:
                return None
            current = jobs.setdefault(header.group(1), [])
            continue
        if line.strip() and not line.startswith("  "):
            return None
        if current is None:
            if line.strip() and not line.strip().startswith("#"):
                return None
            continue
        current.append(line)
    return "\n".join(lines[:start]), {name: "\n".join(body) for name, body in jobs.items()}


def _job_line(block: str, key: str) -> str | None:
    found = [line for line in block.splitlines() if line.startswith(f"    {key}:")]
    return "\n".join(found) if found else None


def _job_needs(block: str) -> set[str]:
    line = _job_line(block, "needs") or ""
    inner = line.partition("[")[2].partition("]")[0]
    return {name.strip() for name in inner.split(",") if name.strip()}


def workflow_change_classes(base_text: str, head_text: str) -> set[str] | None:
    """Return the classes a ci.yml change needs, or None for the full lane.

    The fast lane applies only when every changed job keeps its `if:`,
    `needs:` and `outputs:` and is gated on exactly one class output, or
    always runs (no `if:`, no `needs:`). The class of every job that needs a
    changed job is added too. Anything else (the header, added or removed
    jobs, Classify Changes, the tag gate, release and schedule-only jobs, a
    layout this parser does not understand) returns None.
    """
    base, head = split_workflow(base_text), split_workflow(head_text)
    if base is None or head is None or base[0] != head[0] or set(base[1]) != set(head[1]):
        return None
    base_jobs, head_jobs = base[1], head[1]
    changed = {name for name in head_jobs if head_jobs[name] != base_jobs[name]}
    classes: set[str] = {"code"}
    for name in changed:
        if name in WORKFLOW_FULL_JOBS:
            return None
        before, after = base_jobs[name], head_jobs[name]
        for key in ("if", "needs", "outputs"):
            if _job_line(before, key) != _job_line(after, key):
                return None
        gate = _job_line(after, "if")
        if gate is None:
            if _job_line(after, "needs") is not None:
                return None
            continue
        match = _CLASS_GATE.match(gate)
        if not match or match.group(1) not in CLASSES:
            return None
        classes.add(match.group(1))
    # Jobs that need a changed job: their class runs too. Ungated or
    # event-gated dependents (Backend Build, the release and edge jobs) do not
    # run in a fast-lane pull request and read nothing a step change alters
    # there.
    for name, block in head_jobs.items():
        if not changed & _job_needs(block):
            continue
        match = _CLASS_GATE.match(_job_line(block, "if") or "")
        if match and match.group(1) in CLASSES:
            classes.add(match.group(1))
    return classes


def classify(
    paths: list[str],
    *,
    event: str,
    full_label: bool,
    workflow_classes: set[str] | None = None,
) -> dict[str, bool]:
    """Classify changed paths; workflow_classes is workflow_change_classes()
    for a change that touches the workflow (None runs the full lane)."""
    result = {name: False for name in ("full", *CLASSES)}
    if event != "pull_request" or full_label or not paths or any(matches(path, FULL_PATTERNS) for path in paths):
        return {name: True for name in result}
    if WORKFLOW in paths:
        if workflow_classes is None:
            return {name: True for name in result}
        for name in workflow_classes:
            result[name] = True
    for path in paths:
        if path == WORKFLOW or matches(path, DOC_PATTERNS):
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


def git_workflow_classes(base: str, head: str) -> set[str] | None:
    """workflow_change_classes for the workflow at the merge base and at head."""
    try:
        merge_base = subprocess.run(
            ["git", "merge-base", base, head], check=True, text=True, capture_output=True
        ).stdout.strip()
        texts = [
            subprocess.run(
                ["git", "show", f"{rev}:{WORKFLOW}"], check=True, text=True, capture_output=True
            ).stdout
            for rev in (merge_base, head)
        ]
    except subprocess.CalledProcessError:
        return None
    return workflow_change_classes(*texts)


def self_test() -> None:
    def check(paths: list[str], expected: set[str], *, event: str = "pull_request", label: bool = False) -> None:
        result = classify(paths, event=event, full_label=label)
        on = {name for name, value in result.items() if value}
        assert on == expected, f"{paths} ({event}, label={label}): {sorted(on)} != {sorted(expected)}"

    everything = {"full", *CLASSES}
    workflow = WORKFLOW
    check(["docs/RELEASING.md", "CHANGELOG.md"], set())
    check(["web/src/App.vue"], {"code", "web"})
    check(["internal/handler/ticket.go"], {"code", "db"})
    check(["internal/middleware/audit_log.go"], {"code"})
    check(["packages/plan/native/plans.go"], {"code", "db", "packages"})
    check(["internal/service/order_service.go"], {"code", "db"})
    check(["internal/pluginhost/remote.go"], {"code", "modules"})
    check(["internal/service/forward_panel_flow.go"], {"code", "db", "forward"})
    check(["sdk/forward/driver/nftables/driver.go"], {"code", "modules", "forward"})
    check(["sdk/forward/e2e/scenarios_test.go"], {"code", "modules", "forward"})
    check(["sdk/api/forward/v1/forward.pb.go"], {"code", "modules", "forward"})
    check(["contracts/forward/v1/nft/plan-single-hop-nftables-iepl-forward-11.nft"], {"code", "forward"})
    check(["sdk/agentcontrol/identity.go"], {"code", "modules"})
    check(["internal/grpc/node_server.go"], {"code", "agent"})
    check(["internal/agentpki/enrollment.go"], {"code", "db", "agent"})
    check(["packages/knowledge/compat/v2-routes.json"], {"code", "packages"})
    check(["packages/order/native/orders.go"], {"code", "db", "packages"})
    check(["packages/identity-platform/native/auth.go"], {"code", "db", "modules", "packages"})
    check([workflow], everything)
    check([".github/actions/setup/action.yml"], everything)
    check([".github/workflows/control-center.yml"], {"code"})
    check([".github/BRANCH_PROTECTION.md"], set())
    check(["go.sum"], everything)
    check(["internal/handler/ticket.go"], everything, event="push")
    check(["internal/handler/ticket.go"], everything, event="schedule")
    check(["internal/handler/ticket.go"], everything, label=True)
    check([], everything)

    # Workflow changes.
    def wf_check(paths: list[str], classes: set[str] | None, expected: set[str]) -> None:
        result = classify(paths, event="pull_request", full_label=False, workflow_classes=classes)
        on = {name for name, value in result.items() if value}
        assert on == expected, f"{paths} with {classes}: {sorted(on)} != {sorted(expected)}"

    wf_check([workflow], {"code", "modules"}, {"code", "modules"})
    wf_check([workflow, "internal/grpc/node_server.go"], {"code", "db"}, {"code", "db", "agent"})
    wf_check([workflow, "go.sum"], {"code"}, everything)
    wf_check([workflow], None, everything)

    base = "\n".join(
        [
            "name: CI",
            "on: [push]",
            "jobs:",
            "  changes:",
            "    runs-on: ubuntu-latest",
            "    steps:",
            "      - run: echo classify",
            "  docs-sync:",
            "    runs-on: ubuntu-latest",
            "    steps:",
            "      - run: echo docs",
            "  go-quality:",
            "    needs: [changes]",
            "    if: ${{ needs.changes.outputs.code == 'true' }}",
            "    steps:",
            "      - run: echo quality",
            "  build-images:",
            "    needs: [changes]",
            "    if: ${{ needs.changes.outputs.modules == 'true' }}",
            "    steps:",
            "      - run: echo build",
            "  docker-smoke:",
            "    needs: [changes, build-images]",
            "    if: ${{ needs.changes.outputs.modules == 'true' }}",
            "    services:",
            "      postgres:",
            "        image: postgres:16",
            "    steps:",
            "      - run: echo smoke",
            "  package-storage-postgres:",
            "    needs: [changes]",
            "    if: ${{ needs.changes.outputs.db == 'true' }}",
            "    steps:",
            "      - run: echo pg",
            "  go-benchmark-smoke:",
            "    needs: [changes]",
            "    if: ${{ needs.changes.outputs.full == 'true' }}",
            "    steps:",
            "      - run: echo bench",
            "  backend-build:",
            "    needs: [go-quality]",
            "    steps:",
            "      - run: echo build",
            "  docker:",
            "    needs: [tag-gate]",
            "    if: ${{ needs.tag-gate.outputs.is_release_tag == 'true' }}",
            "    steps:",
            "      - run: echo release",
            "  tag-gate:",
            "    runs-on: ubuntu-latest",
            "    steps:",
            "      - run: echo tag",
            "",
        ]
    )

    def wf(old: str, new: str, expected: set[str] | None) -> None:
        assert old in base, old
        got = workflow_change_classes(base, base.replace(old, new))
        assert got == expected, f"{old!r} -> {new!r}: {got} != {expected}"

    assert workflow_change_classes(base, base) == {"code"}
    wf("echo smoke", "echo smoke again", {"code", "modules"})
    wf("image: postgres:16", "image: postgres:17", {"code", "modules"})
    wf("echo build\n  docker-smoke", "echo build v2\n  docker-smoke", {"code", "modules"})
    wf("echo pg", "echo pg sharded", {"code", "db"})
    wf("echo docs", "echo docs more", {"code"})
    wf("echo quality", "echo quality more", {"code"})
    wf("echo classify", "echo classify more", None)
    wf("echo tag", "echo tag more", None)
    wf("echo bench", "echo bench more", None)
    wf("echo release", "echo release more", None)
    wf("    needs: [go-quality]\n    steps:\n      - run: echo build", "    needs: [go-quality]\n    steps:\n      - run: echo built", None)
    wf("needs: [changes, build-images]", "needs: [changes]", None)
    wf("if: ${{ needs.changes.outputs.db == 'true' }}", "if: ${{ always() }}", None)
    wf("name: CI", "name: CI/CD", None)
    wf("  tag-gate:\n", "  tag-gate-renamed:\n", None)
    wf("      - run: echo pg", "      - run: echo pg\n  extra:\n    steps:\n      - run: echo new", None)
    wf("        image: postgres:16", "        image: &pg postgres:16", None)
    wf("jobs:\n", "jobs:\nfoo: bar\n", None)
    # A dependent's class runs too: changing a modules job that a db job needs.
    dependent = base.replace(
        "    needs: [changes]\n    if: ${{ needs.changes.outputs.db == 'true' }}",
        "    needs: [changes, build-images]\n    if: ${{ needs.changes.outputs.db == 'true' }}",
    )
    got = workflow_change_classes(dependent, dependent.replace("echo build\n  docker-smoke", "echo b2\n  docker-smoke"))
    assert got == {"code", "modules", "db"}, got
    real = (Path(__file__).resolve().parents[2] / WORKFLOW).read_text(encoding="utf-8")
    assert split_workflow(real) is not None, "the real workflow must parse"
    assert workflow_change_classes(real, real) == {"code"}
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
    workflow_classes = git_workflow_classes(args.base, args.head) if WORKFLOW in paths else None
    if WORKFLOW in paths:
        print(f"workflow change classes: {sorted(workflow_classes) if workflow_classes else 'full lane'}", file=sys.stderr)
    result = classify(paths, event=args.event, full_label=args.full_label == "true", workflow_classes=workflow_classes)
    for name, value in result.items():
        print(f"{name}={'true' if value else 'false'}")
    print(f"classified {len(paths)} changed paths", file=sys.stderr)
    return 0


if __name__ == "__main__":
    sys.exit(main())
