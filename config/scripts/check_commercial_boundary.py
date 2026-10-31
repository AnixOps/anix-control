#!/usr/bin/env python3
"""Keep the commercial feature set separable from the rest of Control.

config/commercial-boundary.json names the files that form the commercial set
(payment, order, coupon, invite commission) and the symbols that identify it.
A non-test Go file outside the set may use one of those symbols only if it is
in allowed_dependents. The list is a ratchet: a new dependency fails the gate,
and a listed file that no longer depends on the set must be removed from it.
"""
from __future__ import annotations

import argparse
import json
import re
import sys
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]
BOUNDARY_FORMAT = "anixops.commercial-boundary/v1"
SCAN_ROOTS = ("cmd", "internal", "packages", "sdk", "identity")


class BoundaryError(Exception):
    pass


def load_boundary(path: Path) -> dict:
    data = json.loads(path.read_text(encoding="utf-8"))
    if data.get("format") != BOUNDARY_FORMAT:
        raise BoundaryError(f"{path}: format must be {BOUNDARY_FORMAT}")
    for key in ("commercial", "wiring", "symbols", "allowed_dependents"):
        if not isinstance(data.get(key), list):
            raise BoundaryError(f"{path}: {key} must be a list")
    return data


def glob_regex(pattern: str) -> re.Pattern[str]:
    out = []
    index = 0
    while index < len(pattern):
        if pattern.startswith("**", index):
            out.append(".*")
            index += 2
        elif pattern[index] == "*":
            out.append("[^/]*")
            index += 1
        else:
            out.append(re.escape(pattern[index]))
            index += 1
    return re.compile("^" + "".join(out) + "$")


def go_files(root: Path) -> list[str]:
    files = []
    for top in SCAN_ROOTS:
        base = root / top
        if not base.is_dir():
            continue
        for path in base.rglob("*.go"):
            if path.name.endswith("_test.go"):
                continue
            files.append(path.relative_to(root).as_posix())
    return sorted(files)


def dependents(root: Path, boundary: dict) -> list[str]:
    exempt = [glob_regex(p) for p in boundary["commercial"] + boundary["wiring"]]
    symbols = re.compile(r"\b(?:" + "|".join(boundary["symbols"]) + r")\b")
    found = []
    for rel in go_files(root):
        if any(p.match(rel) for p in exempt):
            continue
        text = (root / rel).read_text(encoding="utf-8", errors="replace")
        if symbols.search(text):
            found.append(rel)
    return found


def check(root: Path, boundary: dict) -> list[str]:
    allowed = set(boundary["allowed_dependents"])
    found = set(dependents(root, boundary))
    problems = []
    for rel in sorted(found - allowed):
        problems.append(f"{rel}: new dependency on the commercial set; move the code behind a neutral extension point or add it to allowed_dependents with the owner's approval")
    for rel in sorted(allowed - found):
        problems.append(f"{rel}: no longer depends on the commercial set; remove it from allowed_dependents")
    return problems


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", type=Path, default=REPO_ROOT)
    parser.add_argument("--boundary", type=Path)
    parser.add_argument("--list", action="store_true", help="print the current dependents and exit")
    args = parser.parse_args(argv)
    boundary = load_boundary(args.boundary or args.root / "config" / "commercial-boundary.json")
    if args.list:
        print("\n".join(dependents(args.root, boundary)))
        return 0
    problems = check(args.root, boundary)
    for problem in problems:
        print(f"commercial boundary: {problem}", file=sys.stderr)
    return 1 if problems else 0


if __name__ == "__main__":
    sys.exit(main())
