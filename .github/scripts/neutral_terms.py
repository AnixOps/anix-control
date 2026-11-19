#!/usr/bin/env python3
"""Fail when a tracked file uses a term that guidelines/terminology.md replaces.

Baseline: AnixOps/AnixOps-design scripts/neutral_terms.py + guidelines/neutral-terms.json.
Product repositories copy both into .github/scripts/ unchanged.

Usage: neutral_terms.py [--list LIST] [--root ROOT]
  A line containing "neutral-terms: allow" is skipped (old values a parser must keep reading).
  ROOT/.neutral-terms-allow lists extra exempt globs, one per line, "#" starts a comment.
"""
import argparse
import fnmatch
import json
import os
import re
import subprocess
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
MARKER = "neutral-terms: allow"


def default_list():
    for candidate in (os.path.join(HERE, "neutral-terms.json"),
                      os.path.join(HERE, "..", "guidelines", "neutral-terms.json")):
        if os.path.exists(candidate):
            return candidate
    sys.exit("neutral-terms.json not found next to the script or in guidelines/")


def exempt_globs(root, base):
    globs = list(base)
    path = os.path.join(root, ".neutral-terms-allow")
    if os.path.exists(path):
        with open(path, encoding="utf-8") as handle:
            for line in handle:
                line = line.split("#", 1)[0].strip()
                if line:
                    globs.append(line)
    return globs


def is_exempt(path, globs):
    return any(fnmatch.fnmatchcase(path, g) or fnmatch.fnmatchcase(os.path.basename(path), g) for g in globs)


def tracked_files(root):
    out = subprocess.run(["git", "-C", root, "ls-files", "-z"], check=True, capture_output=True).stdout
    return [p for p in out.decode("utf-8").split("\0") if p]


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--list", default=None)
    parser.add_argument("--root", default=".")
    args = parser.parse_args()

    with open(args.list or default_list(), encoding="utf-8") as handle:
        spec = json.load(handle)
    terms = [(t["id"], re.compile(t["pattern"]), t["use"]) for t in spec["terms"]]
    globs = exempt_globs(args.root, spec.get("exempt", []))

    hits = 0
    for path in tracked_files(args.root):
        if is_exempt(path, globs):
            continue
        full = os.path.join(args.root, path)
        if not os.path.isfile(full):
            continue
        try:
            with open(full, encoding="utf-8") as handle:
                lines = handle.readlines()
        except (UnicodeDecodeError, OSError):
            continue
        for number, line in enumerate(lines, 1):
            if MARKER in line:
                continue
            for term_id, pattern, use in terms:
                match = pattern.search(line)
                if match:
                    hits += 1
                    print(f"{path}:{number}: [{term_id}] \"{match.group(0)}\" -> {use}")
    if hits:
        print(f"\n{hits} term(s) to replace; see AnixOps-design guidelines/terminology.md.", file=sys.stderr)
        return 1
    print("neutral terms: ok")
    return 0


if __name__ == "__main__":
    sys.exit(main())
