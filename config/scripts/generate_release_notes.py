#!/usr/bin/env python3
"""Generate deterministic release notes for GitHub Actions release assets."""

from __future__ import annotations

import argparse
import os
import tempfile
from pathlib import Path


DEFAULT_SECTION = "Unreleased"


class ReleaseNotesError(RuntimeError):
    """Raised when release notes cannot be generated from the changelog."""


def default_run_url() -> str:
    server_url = os.environ.get("GITHUB_SERVER_URL", "https://github.com").rstrip("/")
    repository = os.environ.get("GITHUB_REPOSITORY", "")
    run_id = os.environ.get("GITHUB_RUN_ID", "")
    if repository and run_id:
        return f"{server_url}/{repository}/actions/runs/{run_id}"
    return ""


def extract_changelog_section(changelog: str, section: str = DEFAULT_SECTION) -> str:
    target = f"## {section}".lower()
    lines = changelog.splitlines()
    start = None
    for index, line in enumerate(lines):
        if line.strip().lower() == target:
            start = index + 1
            break

    if start is None:
        raise ReleaseNotesError(f"CHANGELOG section not found: {section}")

    end = len(lines)
    for index in range(start, len(lines)):
        if lines[index].startswith("## "):
            end = index
            break

    selected = lines[start:end]
    while selected and not selected[0].strip():
        selected.pop(0)
    while selected and not selected[-1].strip():
        selected.pop()

    if not selected:
        raise ReleaseNotesError(f"CHANGELOG section is empty: {section}")

    return "\n".join(selected)


def build_release_notes(
    *,
    tag: str,
    commit: str,
    run_url: str,
    changelog_section: str,
) -> str:
    tag = tag or "unknown"
    commit = commit or "unknown"
    short_commit = commit[:12] if commit != "unknown" else commit

    lines = [
        f"# Release {tag}",
        "",
        "Release artifacts for this tag are produced by GitHub Actions.",
        "Do not rebuild release binaries or frontend assets from a local checkout.",
        "",
        "## Build Metadata",
        "",
        f"- Commit: `{short_commit}`",
    ]
    if run_url:
        lines.append(f"- GitHub Actions run: {run_url}")
    lines.extend(
        [
            "",
            "## Changes",
            "",
            changelog_section,
            "",
        ]
    )
    return "\n".join(lines)


def write_release_notes(content: str, output: Path) -> None:
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(content, encoding="utf-8")


def expect_failure(label: str, func: object) -> None:
    try:
        assert callable(func)
        func()
    except ReleaseNotesError:
        return
    raise AssertionError(f"expected release notes generation failure: {label}")


def run_self_test() -> None:
    changelog = """# Changelog

## Unreleased

### Added

- Added deterministic release notes.

### Fixed

- Fixed release asset coverage.

## v2.3.1

- Older released change.
"""

    section = extract_changelog_section(changelog)
    assert "deterministic release notes" in section
    assert "Older released change" not in section

    notes = build_release_notes(
        tag="v2.4.0",
        commit="1234567890abcdef",
        run_url="https://github.com/AnixOps/anix-control/actions/runs/1",
        changelog_section=section,
    )
    assert notes.startswith("# Release v2.4.0\n")
    assert "`1234567890ab`" in notes
    assert "Do not rebuild release binaries" in notes
    assert "Older released change" not in notes

    with tempfile.TemporaryDirectory() as tmp:
        output = Path(tmp) / "RELEASE_NOTES.md"
        write_release_notes(notes, output)
        assert output.read_text(encoding="utf-8").endswith("\n")

    expect_failure("missing section", lambda: extract_changelog_section("# Changelog\n"))
    expect_failure("empty section", lambda: extract_changelog_section("# Changelog\n\n## Unreleased\n\n## v1.0.0\n"))

    print("release notes self-test passed")


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--changelog", default="CHANGELOG.md", help="Path to CHANGELOG.md.")
    parser.add_argument("--output", default="release/RELEASE_NOTES.md", help="Release notes output path.")
    parser.add_argument("--section", default=DEFAULT_SECTION, help="CHANGELOG section to publish.")
    parser.add_argument("--tag", default=os.environ.get("GITHUB_REF_NAME", ""), help="Release tag name.")
    parser.add_argument("--commit", default=os.environ.get("GITHUB_SHA", ""), help="Release commit SHA.")
    parser.add_argument("--run-url", default=default_run_url(), help="GitHub Actions run URL.")
    parser.add_argument("--self-test", action="store_true", help="Run the built-in release notes self-test.")
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    if args.self_test:
        run_self_test()
        return 0

    changelog_path = Path(args.changelog)
    if not changelog_path.is_file():
        raise SystemExit(f"changelog not found: {changelog_path}")

    section = extract_changelog_section(changelog_path.read_text(encoding="utf-8"), args.section)
    notes = build_release_notes(
        tag=args.tag,
        commit=args.commit,
        run_url=args.run_url,
        changelog_section=section,
    )
    output = Path(args.output)
    write_release_notes(notes, output)
    print(f"wrote {output}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
