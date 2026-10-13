#!/usr/bin/env python3
"""Move every file that pins the anix-agent source to an Agent release commit.

Usage: config/scripts/bump_agent_ref.py <agent-release-commit-sha> <agent-tag>

    config/scripts/bump_agent_ref.py 0123456789abcdef0123456789abcdef01234567 v5.0.0-rc.1

Run it in the pull request that prepares the Control release, after the Agent
tag exists (docs/RELEASING.md: tag the Agent first, then set the pins). The
commit is the full 40-character lowercase SHA the Agent tag points to; the tag
is vX.Y.Z or vX.Y.Z-rc.N, the format check_release_version.py accepts in
config/AGENT_RELEASE. It edits exactly these places, and refuses to change any
of them unless it finds each one exactly once:

  .github/workflows/ci.yml                   AGENT_REF in the top-level env, and
                                             the Agent ldflags (module path and
                                             version label) of the
                                             cross-repository E2E build
  config/scripts/check_release_workflow.sh   the same commit, checked literally
  internal/tests/agente2e/build_test.go      the Agent ldflags module path and
                                             agentE2EVersion
  config/AGENT_RELEASE                       the tag nodes install

The Agent module path is github.com/AnixOps/anix-agent/v<major of the tag>,
and the E2E version label is the tag without its v, plus "-e2e". It then runs
config/scripts/check_release_workflow.sh and fails when the check does.
"""

from __future__ import annotations

import argparse
import importlib.util
import re
import subprocess
import sys
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]
SHA_PATTERN = re.compile(r"[0-9a-f]{40}")

CI_WORKFLOW = ".github/workflows/ci.yml"
WORKFLOW_CHECK = "config/scripts/check_release_workflow.sh"
AGENT_E2E_BUILD = "internal/tests/agente2e/build_test.go"
AGENT_RELEASE_FILE = "config/AGENT_RELEASE"


class BumpError(RuntimeError):
    """Raised when the tree cannot take the new Agent pin."""


def load_version_checker():
    spec = importlib.util.spec_from_file_location("check_release_version", Path(__file__).with_name("check_release_version.py"))
    if spec is None or spec.loader is None:
        raise BumpError("cannot load check_release_version.py")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def validate(commit: str, tag: str) -> None:
    if SHA_PATTERN.fullmatch(commit) is None:
        raise BumpError(f"unsupported Agent commit {commit!r}: use the full 40-character lowercase hex SHA of the Agent tag")
    try:
        load_version_checker().validate_agent_tag(tag)
    except ValueError as error:
        raise BumpError(str(error)) from error


def edits(commit: str, tag: str) -> list[tuple[str, str, str]]:
    """(file, pattern, replacement): each pattern must match exactly once."""
    major = re.match(r"v([0-9]+)\.", tag).group(1)
    label = tag[1:] + "-e2e"
    module = f"github.com/AnixOps/anix-agent/v{major}"
    return [
        (CI_WORKFLOW, r"^  AGENT_REF: '[0-9a-f]{40}'.*$", f"  AGENT_REF: '{commit}'   # anix-agent {tag}"),
        (
            CI_WORKFLOW,
            r'-X github\.com/AnixOps/anix-agent/v[0-9]+/cmd\.version=[^" ]+',
            f"-X {module}/cmd.version={label}",
        ),
        (WORKFLOW_CHECK, r"""^  require_text "AGENT_REF: '[0-9a-f]{40}'" """, f"""  require_text "AGENT_REF: '{commit}'" """),
        (AGENT_E2E_BUILD, r'"-X github\.com/AnixOps/anix-agent/v[0-9]+/cmd\.version="', f'"-X {module}/cmd.version="'),
        (AGENT_E2E_BUILD, r'^const agentE2EVersion = "[^"]+"$', f'const agentE2EVersion = "{label}"'),
    ]


def bump(root: Path, commit: str, tag: str) -> list[str]:
    """Set the Agent pin in the tree at root; returns the files it wrote.

    Every edit is made in memory first, so a pin that is missing or appears
    twice changes no file.
    """
    validate(commit, tag)
    contents: dict[str, str] = {}
    for relative, pattern, replacement in edits(commit, tag):
        if relative not in contents:
            path = root / relative
            if not path.is_file():
                raise BumpError(f"{relative}: not found")
            contents[relative] = path.read_text(encoding="utf-8")
        new, count = re.subn(pattern, lambda _match: replacement, contents[relative], flags=re.MULTILINE)
        if count != 1:
            raise BumpError(f"{relative}: expected one match of {pattern!r}, found {count}")
        contents[relative] = new
    if not (root / AGENT_RELEASE_FILE).is_file():
        raise BumpError(f"{AGENT_RELEASE_FILE}: not found")
    contents[AGENT_RELEASE_FILE] = tag + "\n"
    for relative, text in contents.items():
        (root / relative).write_text(text, encoding="utf-8")
    return sorted(contents)


def run_workflow_check(root: Path) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        ["bash", str(root / WORKFLOW_CHECK)],
        cwd=root,
        check=False,
        text=True,
        capture_output=True,
    )


def main(argv: list[str] | None = None, root: Path = REPO_ROOT) -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("commit", help="the anix-agent commit the Agent tag points to (40 lowercase hex characters)")
    parser.add_argument("tag", help="the anix-agent release tag, vX.Y.Z or vX.Y.Z-rc.N")
    args = parser.parse_args(argv)
    try:
        written = bump(root, args.commit, args.tag)
    except BumpError as error:
        print(f"error: {error}", file=sys.stderr)
        return 2
    for relative in written:
        print(f"updated {relative}")
    check = run_workflow_check(root)
    if check.returncode != 0:
        sys.stderr.write(check.stdout + check.stderr)
        print(f"error: {WORKFLOW_CHECK} failed after the bump", file=sys.stderr)
        return 1
    print(f"AGENT_REF is {args.commit} (anix-agent {args.tag}); {WORKFLOW_CHECK} passes")
    return 0


if __name__ == "__main__":
    sys.exit(main())
