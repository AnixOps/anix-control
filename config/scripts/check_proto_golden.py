#!/usr/bin/env python3
"""Keep the protobuf contract golden additions-only against a base revision.

internal/tests/protocompat fails when an element listed in
contracts/proto/descriptors.golden disappears from the generated
descriptors, and its -update flag only ever adds lines. The golden file
itself is still a text file, though: a pull request could delete or rewrite
a line together with the element. This gate compares the golden file with
the base revision's and fails when a line was removed or changed.

A package in DRAFT_PACKAGES may still edit its own lines in the pull request
that changes its proto (the draft golden policy). There is none now:
anixops.forward.v1 left the draft policy when F3a served it
(docs/architecture/forward-sdk.md section 15).
"""
from __future__ import annotations

import argparse
import subprocess
import sys
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]
GOLDEN = "contracts/proto/descriptors.golden"

# Protobuf packages whose golden lines may still change. Never add a package
# that a released Control, Agent or package serves or calls.
DRAFT_PACKAGES: frozenset[str] = frozenset()


def golden_lines(text: str) -> set[str]:
    """Return the element lines of a golden file, without comments."""
    lines = set()
    for line in text.splitlines():
        line = line.strip()
        if line and not line.startswith("#"):
            lines.add(line)
    return lines


def element_name(line: str) -> str:
    """Return the full name of a golden line's element ("rpc a.b.S.M(...)")."""
    parts = line.split(" ", 2)
    if len(parts) < 2:
        return ""
    return parts[1].split("(", 1)[0]


def in_draft(line: str, drafts: frozenset[str]) -> bool:
    name = element_name(line)
    return any(name.startswith(package + ".") for package in drafts)


def removed(base: str, head: str, drafts: frozenset[str] = DRAFT_PACKAGES) -> list[str]:
    """Return the base lines that head lost, outside the draft packages."""
    head_lines = golden_lines(head)
    return sorted(line for line in golden_lines(base) - head_lines if not in_draft(line, drafts))


def read_revision(revision: str) -> str | None:
    """Return the golden file at revision, None when it has none."""
    result = subprocess.run(
        ["git", "-C", str(REPO_ROOT), "show", f"{revision}:{GOLDEN}"],
        check=False, capture_output=True, text=True,
    )
    if result.returncode != 0:
        return None
    return result.stdout


def main(argv: list[str]) -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--base", required=True, help="the revision to compare with, such as origin/go_dev")
    parser.add_argument("--head", help="the revision to check; the working tree when omitted")
    args = parser.parse_args(argv)
    base = read_revision(args.base)
    if base is None:
        print(f"{GOLDEN} is not in {args.base}; nothing to compare")
        return 0
    if args.head:
        head = read_revision(args.head)
        if head is None:
            print(f"{GOLDEN} is missing from {args.head}", file=sys.stderr)
            return 1
    else:
        head = (REPO_ROOT / GOLDEN).read_text(encoding="utf-8")
    lost = removed(base, head)
    if lost:
        print(f"{GOLDEN} lost {len(lost)} line(s) of frozen contracts; contracts only grow:", file=sys.stderr)
        for line in lost:
            print(f"  {line}", file=sys.stderr)
        return 1
    print(f"{GOLDEN} only grew: {len(golden_lines(head) - golden_lines(base))} new line(s)")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
