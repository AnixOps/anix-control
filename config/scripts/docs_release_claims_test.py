"""Documentation claims about releases that CHANGELOG.md and the code can check.

Two kinds of statement went stale and nothing noticed:

* an install example that pins a release tag that was never published
  (`v4.0.1` in DEPLOYMENT.md, `v3.0.0-alpha.2` in the control-migration
  guide): the download answers 404; and
* an upgrade order the code cannot execute. A Control hands out only its own
  Agent release (`/install/agent.env` names `v` + Control's version unless
  `agent_install.agent_version` is set, and `agentupgrade.Start` refuses a
  `target_version` newer than Control, H25), so from a 4.2 build the order is
  Control first and the Agents after it; and a 4.1 Control serves no
  `/install/agent.env`, so the one-command installer cannot run on a node
  before Control is on 4.2.

The tests read README.md, docs/**/*.md (not docs/archive), scripts/*.sh and the
two root forwarders. DOC_CLAIMS_ROOT points them at another checkout or an
exported tree. A release is published when CHANGELOG.md has its
`## X.Y.Z - date` heading (the release body is that section), so the check
needs no git history, tags or network; a tag that was pushed but not
released, like v4.2.0-rc.3, is not a published release.

These tests run in the script unit tests of "Go Quality Gates", which a
documentation-only change skips.
"""

from __future__ import annotations

import bisect
import os
import re
import tempfile
import unittest
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]
DOCS_ROOT = Path(os.environ.get("DOC_CLAIMS_ROOT") or REPO_ROOT)

TAG = r"v\d+\.\d+\.\d+(?:-(?:alpha|beta|rc)(?:\.\d+)?)?"
# --version v1.2.3, VERSION=v1.2.3 (also TARGET, PREVIOUS, INSTALL_REF and
# ANIX_CONTROL_VERSION), and the tag of a raw.githubusercontent.com or
# releases/download URL of this repository.
PIN = re.compile(
    r"(?:--version[ =]+|[A-Z_]*(?:VERSION|TARGET|PREVIOUS|INSTALL_REF)=)[\"']?(?P<flag>" + TAG + r")(?![\w.-])"
    r"|AnixOps/anix-control/(?:releases/download/)?(?P<url>" + TAG + r")/"
)

RELEASE_HEADING = re.compile(r"^## (\d+\.\d+\.\d+(?:-[0-9A-Za-z.]+)?) - \d{4}-\d{2}-\d{2}\s*$", re.MULTILINE)
DEFAULT_VERSION = re.compile(r'DefaultVersion\s*=\s*"(\d+\.\d+\.\d+)(-[^"]+)?"')

# (pattern, why it is wrong). The text is joined across line breaks first.
STALE_AGENT_ORDER = (
    (
        re.compile(r"upgrade\s+the\s+agents\s+first", re.IGNORECASE),
        "a Control hands out only its own Agent release (agentupgrade.Start refuses a target "
        "newer than Control; /install/agent.env names Control's version): upgrade Control first, "
        "then the Agents",
    ),
    (
        re.compile(r"while\s+control\s+is\s+still\s+on\s+4\.1\s*,\s+clean\s+agent", re.IGNORECASE),
        "a 4.1 Control serves no /install/agent.env, so a node with no anix-agent (clean agent, "
        "NodeX, Ansible) cannot take the one-command installer before Control is on 4.2",
    ),
)


def release_pins(text: str) -> list[tuple[int, str]]:
    """Return (line number, tag) for every release tag the text pins."""
    pins = []
    for number, line in enumerate(text.splitlines(), start=1):
        for match in PIN.finditer(line):
            pins.append((number, match.group("flag") or match.group("url")))
    return pins


def stale_agent_order(text: str) -> list[tuple[int, str]]:
    """Return (line number, reason) for every stale Agent-order statement."""
    # Join the lines with single spaces so a sentence that wraps still matches,
    # and keep where each line starts to map a match back to its line.
    parts: list[str] = []
    starts: list[int] = []
    offset = 0
    for line in text.splitlines():
        stripped = line.strip()
        starts.append(offset)
        parts.append(stripped)
        offset += len(stripped) + 1
    flat = " ".join(parts)
    found = []
    for pattern, reason in STALE_AGENT_ORDER:
        for match in pattern.finditer(flat):
            found.append((bisect.bisect_right(starts, match.start()), reason))
    return sorted(found)


def documents(root: Path) -> list[Path]:
    """The files whose statements these tests check."""
    paths = [root / "README.md", root / "install.sh", root / "panel_install.sh"]
    paths += sorted((root / "scripts").glob("*.sh"))
    paths += sorted(
        path for path in (root / "docs").rglob("*.md") if "archive" not in path.relative_to(root / "docs").parts
    )
    return [path for path in paths if path.is_file()]


def released_tags(root: Path) -> set[str]:
    """The release tags CHANGELOG.md has a heading for, plus this tree's own.

    The tree's own version and the stable release its candidates lead to are
    allowed too: the docs of a release candidate name the release it becomes.
    """
    tags = set()
    changelog = root / "CHANGELOG.md"
    if changelog.is_file():
        tags.update("v" + version for version in RELEASE_HEADING.findall(changelog.read_text(encoding="utf-8")))
    branding = root / "internal" / "branding" / "branding.go"
    if branding.is_file():
        match = DEFAULT_VERSION.search(branding.read_text(encoding="utf-8"))
        if match:
            tags.add("v" + match.group(1))
            tags.add("v" + match.group(1) + (match.group(2) or ""))
    return tags


class ReleasePinsTest(unittest.TestCase):
    def test_finds_the_pins_operators_copy(self) -> None:
        text = "\n".join(
            [
                "sudo bash scripts/install.sh migrate --version v3.0.0-alpha.2",
                "export VERSION=v4.2.0   # the release tag to install",
                'export TARGET="v4.2.0" PREVIOUS=v4.1.0',
                "ANIX_CONTROL_VERSION=v4.2.0-rc.4 bash install.sh",
                "curl -fsSL https://raw.githubusercontent.com/AnixOps/anix-control/v4.0.1/docker-compose.prod.yml",
            ]
        )
        self.assertEqual(
            [(1, "v3.0.0-alpha.2"), (2, "v4.2.0"), (3, "v4.2.0"), (3, "v4.1.0"), (4, "v4.2.0-rc.4"), (5, "v4.0.1")],
            release_pins(text),
        )

    def test_ignores_prose_and_other_repositories(self) -> None:
        text = "\n".join(
            [
                "Releases after v4.0.0 carry no evidence bundle.",
                "Tags such as `v4.0.0-beta.1` are accepted.",
                "curl -fL https://github.com/AnixOps/anix-agent/releases/download/v9.9.9/x.zip",
                "--version latest",
            ]
        )
        self.assertEqual([], release_pins(text))

    def test_released_tags_come_from_the_changelog_and_the_tree_version(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "internal" / "branding").mkdir(parents=True)
            (root / "CHANGELOG.md").write_text(
                "# Changelog\n\n## Unreleased\n\n## 4.2.0-rc.4 - 2026-10-09\n\n"
                "> The `v4.2.0-rc.3` tag was pushed but not published.\n\n## 4.1.0 - 2026-10-04\n",
                encoding="utf-8",
            )
            (root / "internal" / "branding" / "branding.go").write_text(
                'const (\n\tDefaultVersion    = "4.2.0-rc.4"\n)\n', encoding="utf-8"
            )
            self.assertEqual({"v4.2.0-rc.4", "v4.2.0", "v4.1.0"}, released_tags(root))

    def test_documented_pins_name_released_tags(self) -> None:
        tags = released_tags(DOCS_ROOT)
        self.assertIn("v4.0.0", tags, "CHANGELOG.md release headings were not read")
        unknown = []
        for path in documents(DOCS_ROOT):
            for number, tag in release_pins(path.read_text(encoding="utf-8")):
                if tag not in tags:
                    unknown.append(f"{path.relative_to(DOCS_ROOT)}:{number}: {tag}")
        self.assertEqual(
            [],
            unknown,
            "these examples pin a tag that has no release heading in CHANGELOG.md and is not the "
            "release this tree prepares (a tag that was never published answers 404 on download); "
            "name a released tag",
        )


class AgentOrderTest(unittest.TestCase):
    def test_finds_stale_statements_across_line_breaks(self) -> None:
        text = (
            "intro\n2. Upgrade the Agents\n   first, with the asset.\n"
            "while Control\nis still on 4.1, clean agent nodes\n"
        )
        self.assertEqual([2, 4], [line for line, _ in stale_agent_order(text)])

    def test_accepts_the_order_the_code_supports(self) -> None:
        text = (
            "2. Deploy the rc.3 Control.\n3. Upgrade the Agents to rc.3 with a campaign.\n"
            "Update the anix-agent first, on every forward node that already runs one.\n"
        )
        self.assertEqual([], stale_agent_order(text))

    def test_the_documents_keep_the_order_the_code_supports(self) -> None:
        problems = []
        for path in documents(DOCS_ROOT):
            for line, reason in stale_agent_order(path.read_text(encoding="utf-8")):
                problems.append(f"{path.relative_to(DOCS_ROOT)}:{line}: {reason}")
        self.assertEqual([], problems)


if __name__ == "__main__":
    unittest.main()
