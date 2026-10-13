#!/usr/bin/env python3
"""Prepare a release commit: set every declared version and date the CHANGELOG.

Usage: config/scripts/prepare_release.py 4.1.0 [--date YYYY-MM-DD] [--agent-tag vX.Y.Z]

It sets the version wherever check_release_version.py looks for it, merges the
changelog fragments of changelog.d/ into the CHANGELOG "## Unreleased" section
(changelog_fragments.py consume) and deletes them, turns the "## Unreleased"
entries into "## <version> - <date>" (leaving an empty Unreleased section on
top), and runs check_release_version.py. Entries written directly under
"## Unreleased" are kept and released together with the fragments.

--agent-tag names the anix-agent release this Control installs on nodes
(config/AGENT_RELEASE, injected into the release build). Without it the file
keeps its value: a Control patch release that ships an unchanged Agent names
the Agent tag it already had.
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
AGENT_RELEASE_FILE = "config/AGENT_RELEASE"
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


def dated_changelog(content: str, version: str, date: str) -> str:
    """The changelog text with "## Unreleased" turned into the dated section."""
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
    return content[: heading.start()] + dated + content[heading.end():]


def load_version_checker():
    spec = importlib.util.spec_from_file_location("check_release_version", Path(__file__).with_name("check_release_version.py"))
    if spec is None or spec.loader is None:
        raise PrepareError("cannot load check_release_version.py")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def load_fragment_collector():
    spec = importlib.util.spec_from_file_location("changelog_fragments", Path(__file__).with_name("changelog_fragments.py"))
    if spec is None or spec.loader is None:
        raise PrepareError("cannot load changelog_fragments.py")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def set_agent_release(path: Path, tag: str) -> None:
    if not path.is_file():
        raise PrepareError(f"{path}: not found")
    path.write_text(tag + "\n", encoding="utf-8")


def prepare_release(root: Path, version: str, date: str, agent_tag: str | None = None) -> list[str]:
    """Prepare the tree for the release; returns the names of the consumed changelog fragments."""
    if not VERSION_PATTERN.fullmatch(version):
        raise PrepareError(f"unsupported version {version!r}: use X.Y.Z or X.Y.Z-(alpha|beta|rc)[.N]")
    checker = load_version_checker()
    if agent_tag is not None:
        # Refused before any file changes.
        try:
            checker.validate_agent_tag(agent_tag)
        except ValueError as error:
            raise PrepareError(str(error)) from error
    # The fragments are merged and the section dated in memory first, so an
    # invalid fragment or a CHANGELOG that cannot be released changes no file.
    collector = load_fragment_collector()
    changelog = root / "CHANGELOG.md"
    try:
        fragments = collector.collect(root)
        merged = collector.merge_into_changelog(changelog.read_text(encoding="utf-8"), fragments)
    except collector.FragmentError as error:
        raise PrepareError(str(error)) from error
    changelog.write_text(dated_changelog(merged, version, date), encoding="utf-8")
    if agent_tag is not None:
        set_agent_release(root / AGENT_RELEASE_FILE, agent_tag)
    for relative, pattern in TEXT_SURFACES:
        set_text_version(root / relative, pattern, version)
    for relative, count in JSON_SURFACES:
        set_json_versions(root / relative, count, version)
    checker.check_release_version(root, f"v{version}")
    # The fragments are deleted last: when a surface above fails, they are
    # still in the tree and the checkout can be restored as it was.
    collector.remove_fragments(root, fragments)
    return [fragment.name for fragment in fragments]


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
        # Without --agent-tag the Agent release keeps its value.
        agent_release = root / AGENT_RELEASE_FILE
        assert agent_release.read_text(encoding="utf-8") == "v4.2.0\n"
        for bad, message in (("4.1.0", "already has"), ("4.2.0", "is empty"), ("v4.2", "unsupported")):
            try:
                prepare_release(root, bad, "2026-10-02")
            except PrepareError as error:
                if message not in str(error):
                    raise AssertionError(f"{bad}: unexpected error {error}") from error
            else:
                raise AssertionError(f"{bad}: should have been refused")

    # --agent-tag sets the pinned Agent release; a malformed tag is refused
    # before anything changes.
    with tempfile.TemporaryDirectory() as temporary:
        root = Path(temporary)
        checker.write_fixture(root, "4.0.0")
        changelog = root / "CHANGELOG.md"
        changelog_text = "# Changelog\n\n## Unreleased\n\n- A change.\n\n## 4.0.0 - 2026-07-17\n"
        changelog.write_text(changelog_text, encoding="utf-8")
        agent_release = root / AGENT_RELEASE_FILE
        for bad in ("4.3.0", "v4.3", "latest", "v4.3.0-beta.1", ""):
            try:
                prepare_release(root, "4.1.0", "2026-10-01", agent_tag=bad)
            except PrepareError as error:
                if "Agent release tag" not in str(error):
                    raise AssertionError(f"{bad!r}: unexpected error {error}") from error
            else:
                raise AssertionError(f"{bad!r}: should have been refused")
            if changelog.read_text(encoding="utf-8") != changelog_text or agent_release.read_text(encoding="utf-8") != "v4.2.0\n":
                raise AssertionError(f"{bad!r}: files changed although the tag was refused")
        prepare_release(root, "4.1.0", "2026-10-01", agent_tag="v4.3.0-rc.1")
        if agent_release.read_text(encoding="utf-8") != "v4.3.0-rc.1\n":
            raise AssertionError(f"Agent release not set: {agent_release.read_text(encoding='utf-8')!r}")
        checker.check_release_version(root, "v4.1.0")

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

    # Fragments of changelog.d/ join the release section and are removed; the
    # README stays. An invalid fragment is refused before anything changes.
    with tempfile.TemporaryDirectory() as temporary:
        root = Path(temporary)
        checker.write_fixture(root, "4.0.0")
        changelog = root / "CHANGELOG.md"
        changelog_text = "# Changelog\n\n## Unreleased\n\n### Fixed\n\n- An existing fix.\n\n## 4.0.0 - 2026-07-17\n"
        changelog.write_text(changelog_text, encoding="utf-8")
        directory = root / "changelog.d"
        directory.mkdir()
        (directory / "README.md").write_text("# Changelog fragments\n", encoding="utf-8")
        (directory / "b-feature.md").write_text("### Added\n\n- Feature B.\n\n### Fixed\n\n- Fix B.\n", encoding="utf-8")
        (directory / "a-feature.md").write_text("### Added\n\n- Feature A.\n", encoding="utf-8")
        (directory / "bad.md").write_text("- An entry before any heading.\n", encoding="utf-8")
        try:
            prepare_release(root, "4.1.0", "2026-10-01")
        except PrepareError as error:
            if "changelog.d/bad.md:1:" not in str(error):
                raise AssertionError(f"unexpected error {error}") from error
        else:
            raise AssertionError("an invalid fragment should have been refused")
        if changelog.read_text(encoding="utf-8") != changelog_text or "VERSION := 4.0.0\n" not in (root / "Makefile").read_text(encoding="utf-8"):
            raise AssertionError("files changed although a fragment was refused")
        if sorted(path.name for path in directory.iterdir()) != ["README.md", "a-feature.md", "b-feature.md", "bad.md"]:
            raise AssertionError("fragments were removed although one was refused")
        (directory / "bad.md").unlink()
        consumed = prepare_release(root, "4.1.0", "2026-10-01")
        if consumed != ["a-feature.md", "b-feature.md"]:
            raise AssertionError(f"consumed {consumed}")
        expected = (
            "# Changelog\n\n## Unreleased\n\n## 4.1.0 - 2026-10-01\n\n"
            "### Added\n\n- Feature A.\n- Feature B.\n\n"
            "### Fixed\n\n- An existing fix.\n- Fix B.\n\n"
            "## 4.0.0 - 2026-07-17\n"
        )
        if changelog.read_text(encoding="utf-8") != expected:
            raise AssertionError(f"CHANGELOG not prepared from fragments as expected:\n{changelog.read_text(encoding='utf-8')}")
        if sorted(path.name for path in directory.iterdir()) != ["README.md"]:
            raise AssertionError("consumed fragments are still in changelog.d")
        checker.check_release_version(root, "v4.1.0")
        try:
            prepare_release(root, "4.1.1", "2026-10-02")
        except PrepareError as error:
            if "is empty" not in str(error):
                raise AssertionError(f"unexpected error {error}") from error
        else:
            raise AssertionError("a release without entries or fragments should have been refused")

    # An empty Unreleased section is enough when fragments carry the entries.
    with tempfile.TemporaryDirectory() as temporary:
        root = Path(temporary)
        checker.write_fixture(root, "4.0.0")
        changelog = root / "CHANGELOG.md"
        changelog.write_text("# Changelog\n\n## Unreleased\n\n## 4.0.0 - 2026-07-17\n", encoding="utf-8")
        (root / "changelog.d").mkdir()
        (root / "changelog.d" / "only-fragment.md").write_text("### Security\n\n- A fix.\n", encoding="utf-8")
        if prepare_release(root, "4.0.1", "2026-10-01") != ["only-fragment.md"]:
            raise AssertionError("the fragment was not consumed")
        text = changelog.read_text(encoding="utf-8")
        if "## Unreleased\n\n## 4.0.1 - 2026-10-01\n\n### Security\n\n- A fix.\n\n## 4.0.0" not in text:
            raise AssertionError(f"CHANGELOG not prepared from a fragment alone:\n{text}")
        checker.check_release_version(root, "v4.0.1")
    print("prepare_release self-test passed")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("version", nargs="?", help="release version, without the leading v")
    parser.add_argument("--date", default=datetime.date.today().isoformat(), help="release date (default: today)")
    parser.add_argument(
        "--agent-tag",
        help="the anix-agent release tag this Control installs on nodes, such as v4.2.0 (default: keep config/AGENT_RELEASE)",
    )
    parser.add_argument("--self-test", action="store_true", help="run the built-in self-test")
    args = parser.parse_args()
    if args.self_test:
        self_test()
        return 0
    if not args.version:
        parser.error("the release version is required")
    try:
        consumed = prepare_release(REPO_ROOT, args.version, args.date, args.agent_tag)
    except (PrepareError, ValueError) as error:
        print(f"error: {error}", file=sys.stderr)
        return 1
    if consumed:
        print(f"Merged {len(consumed)} changelog fragment(s) from changelog.d/ into the {args.version} section.")
    print(f"Prepared {args.version}. Commit it through a pull request, then tag the merged commit v{args.version}.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
