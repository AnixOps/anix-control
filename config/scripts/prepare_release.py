#!/usr/bin/env python3
"""Prepare a release commit: set every declared version and date the CHANGELOG.

Usage: config/scripts/prepare_release.py 4.1.0 [--date YYYY-MM-DD]

It sets the version wherever check_release_version.py looks for it, turns
the CHANGELOG "## Unreleased" entries into "## <version> - <date>" (leaving
an empty Unreleased section on top), and runs check_release_version.py.
Commit the result through a pull request, then tag the merged commit
v<version>; docs/RELEASING.md has the whole flow.
"""

from __future__ import annotations

import argparse
import datetime
import importlib.util
import json
import re
import sys
import tempfile
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]
VERSION_PATTERN = re.compile(r"^[0-9]+\.[0-9]+\.[0-9]+(?:-(?:alpha|beta|rc)(?:\.[0-9]+)?)?$")

# (file, pattern whose group 1 is the version); the first match is replaced,
# as check_release_version.py reads the first match.
TEXT_SURFACES: tuple[tuple[str, str], ...] = (
    ("internal/branding/branding.go", r'^\s*DefaultVersion\s*=\s*"([^"]+)"'),
    ("Makefile", r"^VERSION\s*:=\s*(\S+)"),
    ("cmd/server/main.go", r"^//\s*@version\s+(\S+)"),
    ("config/config.yaml.example", r'^\s{2}version:\s*"([^"]+)"'),
    ("config/config.prod.yaml", r'^\s{2}version:\s*"([^"]+)"'),
    ("config/config.dev.yaml.example", r'^\s{2}version:\s*"([^"]+)"'),
    # The chart's default image tag. Its own `version` is not a release surface.
    ("config/deploy/helm/anix-control/Chart.yaml", r'^appVersion:\s*"([^"]+)"'),
    ("docs/docs.go", r'^\s*Version:\s*"([^"]+)"'),
    ("docs/swagger.yaml", r"^\s{2}version:\s*([^\s]+)"),
    ("README.md", r"^- Current release:\s+`v([^`]+)`"),
)
# (file, number of leading "version" string fields to set).
JSON_SURFACES: tuple[tuple[str, int], ...] = (
    ("web/package.json", 1),
    ("web/package-lock.json", 2),
    ("docs/swagger.json", 1),
)


class PrepareError(RuntimeError):
    """Raised when the tree cannot be prepared for the release."""


def set_text_version(path: Path, pattern: str, version: str) -> None:
    content = path.read_text(encoding="utf-8")
    match = re.search(pattern, content, re.MULTILINE)
    if match is None:
        raise PrepareError(f"{path}: version field not found")
    start, end = match.span(1)
    path.write_text(content[:start] + version + content[end:], encoding="utf-8")


def set_json_versions(path: Path, count: int, version: str) -> None:
    content = path.read_text(encoding="utf-8")
    pattern = re.compile(r'("version":\s*")([^"]*)(")')
    matches = list(pattern.finditer(content))[:count]
    if len(matches) < count:
        raise PrepareError(f"{path}: expected {count} version fields, found {len(matches)}")
    for match in reversed(matches):
        start, end = match.span(2)
        content = content[:start] + version + content[end:]
    json.loads(content)
    path.write_text(content, encoding="utf-8")


def date_changelog(path: Path, version: str, date: str) -> None:
    content = path.read_text(encoding="utf-8")
    if re.search(rf"^##\s+{re.escape(version)}(\s|$)", content, re.MULTILINE):
        raise PrepareError(f"CHANGELOG already has a {version} section")
    heading = re.search(r"^## Unreleased[ \t]*$", content, re.MULTILINE)
    if heading is None:
        raise PrepareError("CHANGELOG has no '## Unreleased' section")
    rest = content[heading.end():]
    following = re.search(r"^## ", rest, re.MULTILINE)
    entries = rest[: following.start()] if following else rest
    if not entries.strip():
        raise PrepareError("CHANGELOG '## Unreleased' is empty: nothing to release")
    dated = f"## Unreleased\n\n## {version} - {date}"
    path.write_text(content[: heading.start()] + dated + content[heading.end():], encoding="utf-8")


def load_version_checker():
    spec = importlib.util.spec_from_file_location("check_release_version", Path(__file__).with_name("check_release_version.py"))
    if spec is None or spec.loader is None:
        raise PrepareError("cannot load check_release_version.py")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def prepare_release(root: Path, version: str, date: str) -> None:
    if not VERSION_PATTERN.fullmatch(version):
        raise PrepareError(f"unsupported version {version!r}: use X.Y.Z or X.Y.Z-(alpha|beta|rc)[.N]")
    date_changelog(root / "CHANGELOG.md", version, date)
    for relative, pattern in TEXT_SURFACES:
        set_text_version(root / relative, pattern, version)
    for relative, count in JSON_SURFACES:
        set_json_versions(root / relative, count, version)
    load_version_checker().check_release_version(root, f"v{version}")


def self_test() -> None:
    checker = load_version_checker()
    with tempfile.TemporaryDirectory() as temporary:
        root = Path(temporary)
        checker.write_fixture(root, "4.0.0")
        changelog = root / "CHANGELOG.md"
        changelog.write_text("# Changelog\n\n## Unreleased\n\n### Added\n\n- A change.\n\n## 4.0.0 - 2026-07-17\n", encoding="utf-8")
        for relative, _ in TEXT_SURFACES:
            path = root / relative
            if not path.exists():
                raise AssertionError(f"fixture lacks {relative}")
        chart = root / "config/deploy/helm/anix-control/Chart.yaml"
        assert 'appVersion: "4.0.0"' in chart.read_text(encoding="utf-8")
        prepare_release(root, "4.1.0", "2026-10-01")
        checker.check_release_version(root, "v4.1.0")
        # The chart's default image follows the release; its own version stays.
        chart_text = chart.read_text(encoding="utf-8")
        if 'appVersion: "4.1.0"' not in chart_text or "\nversion: 0.3.0\n" not in chart_text:
            raise AssertionError(f"Chart.yaml not prepared as expected:\n{chart_text}")
        text = changelog.read_text(encoding="utf-8")
        if "## Unreleased\n\n## 4.1.0 - 2026-10-01\n\n### Added" not in text:
            raise AssertionError(f"CHANGELOG not dated as expected:\n{text}")
        for bad, message in (("4.1.0", "already has"), ("4.2.0", "is empty"), ("v4.2", "unsupported")):
            try:
                prepare_release(root, bad, "2026-10-02")
            except PrepareError as error:
                if message not in str(error):
                    raise AssertionError(f"{bad}: unexpected error {error}") from error
            else:
                raise AssertionError(f"{bad}: should have been refused")

    # A tree whose chart has no appVersion cannot be prepared: the release
    # would ship a chart that deploys another release's image.
    with tempfile.TemporaryDirectory() as temporary:
        root = Path(temporary)
        checker.write_fixture(root, "4.0.0")
        (root / "CHANGELOG.md").write_text("# Changelog\n\n## Unreleased\n\n- A change.\n\n## 4.0.0 - 2026-07-17\n", encoding="utf-8")
        chart = root / "config/deploy/helm/anix-control/Chart.yaml"
        chart.write_text("apiVersion: v2\nname: anix-control\nversion: 0.3.0\n", encoding="utf-8")
        try:
            prepare_release(root, "4.1.0", "2026-10-01")
        except PrepareError as error:
            if "Chart.yaml" not in str(error) or "version field not found" not in str(error):
                raise AssertionError(f"unexpected error {error}") from error
        else:
            raise AssertionError("a chart without appVersion should have been refused")
    print("prepare_release self-test passed")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("version", nargs="?", help="release version, without the leading v")
    parser.add_argument("--date", default=datetime.date.today().isoformat(), help="release date (default: today)")
    parser.add_argument("--self-test", action="store_true", help="run the built-in self-test")
    args = parser.parse_args()
    if args.self_test:
        self_test()
        return 0
    if not args.version:
        parser.error("the release version is required")
    try:
        prepare_release(REPO_ROOT, args.version, args.date)
    except (PrepareError, ValueError) as error:
        print(f"error: {error}", file=sys.stderr)
        return 1
    print(f"Prepared {args.version}. Commit it through a pull request, then tag the merged commit v{args.version}.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
