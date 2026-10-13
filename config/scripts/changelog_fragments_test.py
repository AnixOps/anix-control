from __future__ import annotations

import contextlib
import io
import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path

import changelog_fragments as fragments

CHANGELOG = """# Changelog

## Unreleased

### Fixed

- An existing fix.

### Added

- An existing feature.

## 4.2.1 - 2026-10-12

### Added

- A released feature.
"""


def write_fragments(root: Path, files: dict[str, str]) -> None:
    directory = root / "changelog.d"
    directory.mkdir(exist_ok=True)
    for name, text in files.items():
        (directory / name).write_bytes(text.encode("utf-8"))


def parse(text: str):
    return fragments.parse_fragment(text, "changelog.d/x.md")


class TemporaryRoot(unittest.TestCase):
    def setUp(self) -> None:
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        self.root = Path(directory.name)
        self.changelog = self.root / "CHANGELOG.md"

    def listing(self) -> list[str]:
        return sorted(path.name for path in (self.root / "changelog.d").iterdir())


class ParseTest(unittest.TestCase):
    def test_sections_entries_and_continuation_lines(self) -> None:
        text = (
            "### Fixed\n\n"
            "- First entry that wraps\n"
            "  onto a second line.\n"
            "- Second entry with a nested list:\n"
            "  - nested\n"
            "\n"
            "  and a second paragraph.\n"
            "\n"
            "### Added\n\n"
            "- Third.\n"
        )
        fragment, problems = parse(text)
        self.assertEqual([], problems)
        self.assertEqual("x.md", fragment.name)
        self.assertEqual(["Fixed", "Added"], [heading for heading, _ in fragment.sections])
        fixed = fragment.sections[0][1]
        self.assertEqual(("- First entry that wraps", "  onto a second line."), fixed[0].lines)
        self.assertEqual(
            ("- Second entry with a nested list:", "  - nested", "", "  and a second paragraph."), fixed[1].lines
        )
        self.assertEqual(("- Third.",), fragment.sections[1][1][0].lines)

    def test_every_heading_of_the_changelog_is_accepted(self) -> None:
        for heading in ("Added", "Changed", "Fixed", "Security", "Removed", "Deprecated"):
            with self.subTest(heading=heading):
                fragment, problems = parse(f"### {heading}\n\n- Entry.\n")
                self.assertEqual([], problems)
                self.assertEqual(heading, fragment.sections[0][0])

    def test_invalid_fragments_name_the_line(self) -> None:
        cases = (
            ("bullet before a heading", "- An entry.\n", 1, "entry before any heading"),
            ("text before a heading", "Some words.\n\n### Added\n\n- X.\n", 1, "text outside an entry"),
            ("unknown heading", "### Tooling\n\n- X.\n", 1, "unknown heading '### Tooling'"),
            ("heading level", "## Added\n\n- X.\n", 1, "unknown heading"),
            ("sub-heading", "### Added\n\n#### More\n\n- X.\n", 3, "unknown heading"),
            ("lowercase heading", "### added\n\n- X.\n", 1, "unknown heading"),
            ("no space after hashes", "###Added\n\n- X.\n", 1, "unknown heading"),
            ("trailing space", "### Added\n\n- X. \n", 3, "trailing whitespace"),
            ("whitespace only line", "### Added\n\n- X.\n  \n- Y.\n", 4, "trailing whitespace"),
            ("tab", "### Added\n\n- X\n\tcontinued\n", 4, "tab character"),
            ("conflict start", "### Added\n\n<<<<<<< HEAD\n- X.\n", 3, "merge conflict marker '<<<<<<<'"),
            ("conflict middle", "### Added\n\n- X.\n=======\n- Y.\n", 4, "merge conflict marker '======='"),
            ("conflict end", "### Added\n\n- X.\n>>>>>>> topic\n", 4, "merge conflict marker '>>>>>>>'"),
            ("no final newline", "### Added\n\n- X.", 3, "no newline at the end"),
            ("blank line at the end", "### Added\n\n- X.\n\n", 4, "blank line at the end"),
            ("carriage return", "### Added\r\n\r\n- X.\r\n", 1, "carriage return"),
            ("byte order mark", "\ufeff### Added\n\n- X.\n", 1, "byte order mark"),
            ("empty file", "", 1, "no entries"),
            ("heading only", "### Added\n", 1, "section '### Added' has no entries"),
            ("empty section", "### Added\n\n### Fixed\n\n- X.\n", 1, "section '### Added' has no entries"),
            ("duplicate heading", "### Added\n\n- X.\n\n### Added\n\n- Y.\n", 5, "duplicate heading '### Added' (first at line 1)"),
            ("star bullet", "### Added\n\n* X.\n", 3, "use '- '"),
            ("unindented continuation", "### Added\n\n- X\ncontinued.\n", 4, "text outside an entry"),
            ("one space continuation", "### Added\n\n- X\n continued.\n", 4, "text outside an entry"),
            ("empty entry", "### Added\n\n-\n", 3, "empty entry"),
            ("indented text first", "### Added\n\n  continued.\n", 3, "indented text outside an entry"),
        )
        for label, text, line, message in cases:
            with self.subTest(label):
                fragment, problems = parse(text)
                self.assertIsNone(fragment)
                self.assertTrue(
                    any(problem.line == line and message in problem.message for problem in problems),
                    f"want line {line}: {message!r}; got {[str(problem) for problem in problems]}",
                )
                self.assertTrue(all(str(problem).startswith("changelog.d/x.md:") for problem in problems))


class CollectTest(TemporaryRoot):
    def test_missing_directory_has_no_fragments(self) -> None:
        self.assertEqual([], fragments.collect(self.root))

    def test_readme_is_never_collected_or_checked(self) -> None:
        write_fragments(self.root, {"README.md": "no heading, <<<<<<< not a fragment  \n", "a.md": "### Added\n\n- A.\n"})
        collected = fragments.collect(self.root)
        self.assertEqual(["a.md"], [fragment.name for fragment in collected])

    def test_fragments_come_in_file_name_order(self) -> None:
        write_fragments(
            self.root,
            {"zebra.md": "### Added\n\n- Z.\n", "alpha.md": "### Added\n\n- A.\n", "10-ten.md": "### Added\n\n- T.\n"},
        )
        self.assertEqual(["10-ten.md", "alpha.md", "zebra.md"], [f.name for f in fragments.collect(self.root)])

    def test_file_names_are_checked(self) -> None:
        names = (
            "Upper.md",
            "under_score.md",
            "double--dash.md",
            "-leading.md",
            "trailing-.md",
            "space name.md",
            "notes.txt",
            "a.markdown",
            ".gitkeep",
            "readme.md.bak",
        )
        for name in names:
            with self.subTest(name):
                directory = self.root / "changelog.d"
                shutil.rmtree(directory, ignore_errors=True)
                write_fragments(self.root, {name: "### Added\n\n- A.\n"})
                with self.assertRaises(fragments.FragmentError) as raised:
                    fragments.collect(self.root)
                self.assertTrue(str(raised.exception).startswith(f"changelog.d/{name}:1: file name must be <slug>.md"))

    def test_valid_slugs(self) -> None:
        names = ["a.md", "n37.md", "node-group-token-reset.md", "2026-10-13-fix.md"]
        write_fragments(self.root, {name: "### Added\n\n- A.\n" for name in names})
        self.assertEqual(sorted(names), [f.name for f in fragments.collect(self.root)])

    def test_a_subdirectory_is_refused(self) -> None:
        write_fragments(self.root, {})
        (self.root / "changelog.d" / "nested").mkdir()
        with self.assertRaises(fragments.FragmentError) as raised:
            fragments.collect(self.root)
        self.assertIn("changelog.d/nested:1: not a file", str(raised.exception))

    def test_every_problem_of_every_file_is_reported(self) -> None:
        write_fragments(
            self.root,
            {
                "good.md": "### Added\n\n- Fine.\n",
                "bad-heading.md": "### Tooling\n\n- X.\n",
                "bad-space.md": "### Added\n\n- X. \n",
                "Bad_Name.md": "### Added\n\n- X.\n",
            },
        )
        with self.assertRaises(fragments.FragmentError) as raised:
            fragments.collect(self.root)
        messages = str(raised.exception).splitlines()
        self.assertEqual(3, len(messages), messages)
        self.assertTrue(messages[0].startswith("changelog.d/Bad_Name.md:1: "))
        self.assertTrue(messages[1].startswith("changelog.d/bad-heading.md:1: unknown heading"))
        self.assertTrue(messages[2].startswith("changelog.d/bad-space.md:3: trailing whitespace"))
        self.assertEqual(3, len(raised.exception.problems))

    def test_invalid_utf8_is_refused(self) -> None:
        directory = self.root / "changelog.d"
        directory.mkdir()
        (directory / "latin.md").write_bytes(b"### Added\n\n- caf\xe9.\n")
        with self.assertRaises(fragments.FragmentError) as raised:
            fragments.collect(self.root)
        self.assertIn("changelog.d/latin.md:1: not valid UTF-8", str(raised.exception))

    def test_non_ascii_text_is_kept(self) -> None:
        write_fragments(self.root, {"cn.md": "### Added\n\n- 节点列表有服务状态列。\n"})
        self.assertEqual("### Added\n\n- 节点列表有服务状态列。\n", fragments.render(fragments.collect(self.root)))


class RenderTest(TemporaryRoot):
    def test_groups_by_heading_in_canonical_order_and_entries_by_file_name(self) -> None:
        write_fragments(
            self.root,
            {
                "b-second.md": "### Fixed\n\n- Fix B.\n\n### Added\n\n- Add B,\n  wrapped.\n",
                "a-first.md": "### Security\n\n- Sec A.\n\n### Added\n\n- Add A.\n- Add A2.\n",
                "c-third.md": "### Removed\n\n- Rem C.\n\n### Changed\n\n- Chg C.\n\n### Deprecated\n\n- Dep C.\n",
            },
        )
        self.assertEqual(
            "### Added\n\n- Add A.\n- Add A2.\n- Add B,\n  wrapped.\n\n"
            "### Changed\n\n- Chg C.\n\n"
            "### Deprecated\n\n- Dep C.\n\n"
            "### Removed\n\n- Rem C.\n\n"
            "### Fixed\n\n- Fix B.\n\n"
            "### Security\n\n- Sec A.\n",
            fragments.render(fragments.collect(self.root)),
        )

    def test_nothing_to_render(self) -> None:
        self.assertEqual("", fragments.render([]))
        write_fragments(self.root, {"README.md": "# Fragments\n"})
        self.assertEqual("", fragments.render(fragments.collect(self.root)))

    def test_render_is_stable(self) -> None:
        write_fragments(self.root, {"a.md": "### Added\n\n- A.\n", "b.md": "### Fixed\n\n- B.\n"})
        first = fragments.render(fragments.collect(self.root))
        self.assertEqual(first, fragments.render(fragments.collect(self.root)))
        self.assertEqual(first, fragments.render(list(reversed(fragments.collect(self.root)))))


class MergeTest(TemporaryRoot):
    def merged(self, changelog: str, files: dict[str, str]) -> str:
        write_fragments(self.root, files)
        return fragments.merge_into_changelog(changelog, fragments.collect(self.root))

    def test_entries_join_existing_headings_after_their_own(self) -> None:
        text = self.merged(CHANGELOG, {"a.md": "### Fixed\n\n- A fix.\n\n### Added\n\n- A feature.\n"})
        self.assertEqual(
            "# Changelog\n\n## Unreleased\n\n"
            "### Fixed\n\n- An existing fix.\n- A fix.\n\n"
            "### Added\n\n- An existing feature.\n- A feature.\n\n"
            "## 4.2.1 - 2026-10-12\n\n### Added\n\n- A released feature.\n",
            text,
        )

    def test_no_heading_appears_twice(self) -> None:
        text = self.merged(
            CHANGELOG,
            {
                "a.md": "### Fixed\n\n- A.\n\n### Added\n\n- A.\n",
                "b.md": "### Fixed\n\n- B.\n\n### Changed\n\n- B.\n",
                "c.md": "### Changed\n\n- C.\n\n### Security\n\n- C.\n",
            },
        )
        unreleased = text.split("## 4.2.1")[0]
        headings = [line for line in unreleased.splitlines() if line.startswith("### ")]
        self.assertEqual(len(headings), len(set(headings)), headings)
        self.assertEqual(["### Fixed", "### Added", "### Changed", "### Security"], headings)

    def test_new_headings_go_where_the_canonical_order_puts_them(self) -> None:
        changelog = (
            "# Changelog\n\n## Unreleased\n\n### Fixed\n\n- F.\n\n### Added\n\n- A.\n\n"
            "### Changed\n\n- C.\n\n### Removed\n\n- R.\n\n## 4.2.1 - 2026-10-12\n"
        )
        text = self.merged(changelog, {"a.md": "### Deprecated\n\n- D.\n\n### Security\n\n- S.\n"})
        headings = [line for line in text.splitlines() if line.startswith("### ")]
        self.assertEqual(
            ["### Fixed", "### Added", "### Changed", "### Deprecated", "### Removed", "### Security"], headings
        )

    def test_a_new_heading_goes_before_a_later_one_when_nothing_earlier_exists(self) -> None:
        changelog = "# Changelog\n\n## Unreleased\n\n### Fixed\n\n- F.\n\n## 4.2.1 - 2026-10-12\n"
        text = self.merged(changelog, {"a.md": "### Added\n\n- A.\n"})
        self.assertEqual(
            "# Changelog\n\n## Unreleased\n\n### Added\n\n- A.\n\n### Fixed\n\n- F.\n\n## 4.2.1 - 2026-10-12\n", text
        )

    def test_an_empty_unreleased_section_is_filled_in_canonical_order(self) -> None:
        changelog = "# Changelog\n\n## Unreleased\n\n## 4.2.1 - 2026-10-12\n\n### Added\n\n- Old.\n"
        text = self.merged(changelog, {"a.md": "### Fixed\n\n- F.\n\n### Added\n\n- A.\n"})
        self.assertEqual(
            "# Changelog\n\n## Unreleased\n\n### Added\n\n- A.\n\n### Fixed\n\n- F.\n\n"
            "## 4.2.1 - 2026-10-12\n\n### Added\n\n- Old.\n",
            text,
        )

    def test_unreleased_as_the_last_section(self) -> None:
        text = self.merged("# Changelog\n\n## Unreleased\n", {"a.md": "### Added\n\n- A.\n"})
        self.assertEqual("# Changelog\n\n## Unreleased\n\n### Added\n\n- A.\n", text)

    def test_bare_bullets_and_unknown_headings_are_kept(self) -> None:
        changelog = (
            "# Changelog\n\n## Unreleased\n\n- A bare entry.\n\n### Tooling\n\n- A curated entry.\n\n"
            "## 4.2.1 - 2026-10-12\n"
        )
        text = self.merged(changelog, {"a.md": "### Added\n\n- A.\n\n### Fixed\n\n- F.\n"})
        self.assertEqual(
            "# Changelog\n\n## Unreleased\n\n- A bare entry.\n\n### Tooling\n\n- A curated entry.\n\n"
            "### Added\n\n- A.\n\n### Fixed\n\n- F.\n\n## 4.2.1 - 2026-10-12\n",
            text,
        )

    def test_nothing_outside_unreleased_changes_and_inner_blank_lines_stay(self) -> None:
        changelog = (
            "# Changelog\n\nIntro text.\n\n## Unreleased\n\n### Added\n\n- One,\n  wrapped.\n\n- Two after a blank line.\n"
            "\n## 4.2.1 - 2026-10-12\n\n### Added\n\n- Old.\n\n## 4.2.0 - 2026-10-10\n\n### Added\n\n- Older.\n"
        )
        text = self.merged(changelog, {"a.md": "### Added\n\n- New.\n"})
        self.assertTrue(text.startswith("# Changelog\n\nIntro text.\n\n## Unreleased\n\n### Added\n\n"))
        self.assertIn("- One,\n  wrapped.\n\n- Two after a blank line.\n- New.\n\n## 4.2.1 - 2026-10-12\n", text)
        self.assertEqual(changelog[changelog.index("## 4.2.1") :], text[text.index("## 4.2.1") :])

    def test_a_hash_line_in_a_code_block_is_not_a_heading(self) -> None:
        changelog = (
            "# Changelog\n\n## Unreleased\n\n### Added\n\n- Shell:\n\n  ```\n### Fixed\n  ```\n\n## 4.2.1 - 2026-10-12\n"
        )
        text = self.merged(changelog, {"a.md": "### Fixed\n\n- F.\n"})
        headings = [line for line in text.split("## 4.2.1")[0].splitlines() if line.startswith("### ")]
        self.assertEqual(["### Added", "### Fixed", "### Fixed"], headings)  # the code block's line stays inside Added
        self.assertLess(text.index("- F."), text.index("## 4.2.1"))

    def test_no_fragments_leave_the_text_alone(self) -> None:
        self.assertEqual(CHANGELOG, fragments.merge_into_changelog(CHANGELOG, []))
        self.assertEqual("no heading at all", fragments.merge_into_changelog("no heading at all", []))

    def test_a_changelog_without_unreleased_is_refused(self) -> None:
        write_fragments(self.root, {"a.md": "### Added\n\n- A.\n"})
        with self.assertRaises(fragments.FragmentError) as raised:
            fragments.merge_into_changelog("# Changelog\n\n## 4.2.1 - 2026-10-12\n", fragments.collect(self.root))
        self.assertIn("no '## Unreleased' section", str(raised.exception))

    def test_a_repeated_merge_adds_to_the_same_headings(self) -> None:
        # Merging twice keeps one heading each; consume deletes the files so
        # a real second run has nothing to add.
        write_fragments(self.root, {"a.md": "### Added\n\n- A.\n"})
        once = fragments.merge_into_changelog(CHANGELOG, fragments.collect(self.root))
        twice = fragments.merge_into_changelog(once, fragments.collect(self.root))
        self.assertEqual(once.count("### Added"), twice.count("### Added"))
        self.assertEqual(once.count("### Fixed"), twice.count("### Fixed"))


class ConsumeTest(TemporaryRoot):
    def setUp(self) -> None:
        super().setUp()
        self.changelog.write_text(CHANGELOG, encoding="utf-8")

    def test_consume_merges_and_deletes_but_keeps_the_readme(self) -> None:
        write_fragments(
            self.root,
            {
                "README.md": "# How to\n",
                "b.md": "### Fixed\n\n- Fix B.\n",
                "a.md": "### Added\n\n- Add A.\n\n### Changed\n\n- Change A.\n",
            },
        )
        consumed = fragments.consume(self.root, self.changelog)
        self.assertEqual(["a.md", "b.md"], consumed)
        self.assertEqual(["README.md"], self.listing())
        text = self.changelog.read_text(encoding="utf-8")
        self.assertEqual(
            "# Changelog\n\n## Unreleased\n\n"
            "### Fixed\n\n- An existing fix.\n- Fix B.\n\n"
            "### Added\n\n- An existing feature.\n- Add A.\n\n"
            "### Changed\n\n- Change A.\n\n"
            "## 4.2.1 - 2026-10-12\n\n### Added\n\n- A released feature.\n",
            text,
        )

    def test_consume_twice_changes_nothing_the_second_time(self) -> None:
        write_fragments(self.root, {"README.md": "# How to\n", "a.md": "### Added\n\n- Add A.\n"})
        self.assertEqual(["a.md"], fragments.consume(self.root, self.changelog))
        after_first = self.changelog.read_text(encoding="utf-8")
        self.assertEqual([], fragments.consume(self.root, self.changelog))
        self.assertEqual(after_first, self.changelog.read_text(encoding="utf-8"))
        self.assertEqual(["README.md"], self.listing())

    def test_an_invalid_fragment_changes_nothing(self) -> None:
        write_fragments(self.root, {"good.md": "### Added\n\n- Good.\n", "bad.md": "### Tooling\n\n- Bad.\n"})
        with self.assertRaises(fragments.FragmentError):
            fragments.consume(self.root, self.changelog)
        self.assertEqual(CHANGELOG, self.changelog.read_text(encoding="utf-8"))
        self.assertEqual(["bad.md", "good.md"], self.listing())

    def test_a_changelog_without_unreleased_changes_nothing(self) -> None:
        self.changelog.write_text("# Changelog\n\n## 4.2.1 - 2026-10-12\n", encoding="utf-8")
        write_fragments(self.root, {"a.md": "### Added\n\n- A.\n"})
        with self.assertRaises(fragments.FragmentError):
            fragments.consume(self.root, self.changelog)
        self.assertEqual(["a.md"], self.listing())
        self.assertEqual("# Changelog\n\n## 4.2.1 - 2026-10-12\n", self.changelog.read_text(encoding="utf-8"))


@unittest.skipUnless(shutil.which("git"), "git is not installed")
class ConsumeInGitTest(TemporaryRoot):
    def git(self, *args: str) -> str:
        return subprocess.run(
            ["git", "-C", str(self.root), "-c", "user.name=t", "-c", "user.email=t@example.invalid", *args],
            check=True,
            capture_output=True,
            text=True,
        ).stdout

    def test_tracked_fragments_are_removed_with_git_rm(self) -> None:
        self.git("init", "-q")
        self.changelog.write_text(CHANGELOG, encoding="utf-8")
        write_fragments(
            self.root,
            {"README.md": "# How to\n", "tracked.md": "### Added\n\n- Tracked.\n", "staged.md": "### Fixed\n\n- Staged.\n"},
        )
        self.git("add", "CHANGELOG.md", "changelog.d/README.md", "changelog.d/tracked.md")
        self.git("commit", "-q", "-m", "initial")
        # staged.md is untracked, and tracked.md has an uncommitted edit.
        (self.root / "changelog.d" / "tracked.md").write_text("### Added\n\n- Tracked, edited.\n", encoding="utf-8")

        self.assertEqual(["staged.md", "tracked.md"], fragments.consume(self.root, self.changelog))
        self.assertEqual(["README.md"], self.listing())
        status = self.git("status", "--porcelain").splitlines()
        self.assertIn("D  changelog.d/tracked.md", status)  # staged deletion: git rm ran
        self.assertNotIn("?? changelog.d/staged.md", status)
        self.assertIn(" M CHANGELOG.md", status)
        self.assertIn("- Tracked, edited.", self.changelog.read_text(encoding="utf-8"))

    def test_a_directory_inside_an_unrelated_repository_is_deleted_plainly(self) -> None:
        self.git("init", "-q")
        sub = self.root / "nested"
        sub.mkdir()
        write_fragments(sub, {"a.md": "### Added\n\n- A.\n"})
        changelog = sub / "CHANGELOG.md"
        changelog.write_text(CHANGELOG, encoding="utf-8")
        self.assertEqual(["a.md"], fragments.consume(sub, changelog))
        self.assertFalse((sub / "changelog.d" / "a.md").exists())


class CommandLineTest(TemporaryRoot):
    def run_main(self, *args: str) -> tuple[int, str, str]:
        out, err = io.StringIO(), io.StringIO()
        with contextlib.redirect_stdout(out), contextlib.redirect_stderr(err):
            code = fragments.main(["--root", str(self.root), *args])
        return code, out.getvalue(), err.getvalue()

    def test_check(self) -> None:
        write_fragments(self.root, {"README.md": "# How to\n", "a.md": "### Added\n\n- A.\n"})
        code, out, err = self.run_main("check")
        self.assertEqual((0, ""), (code, err))
        self.assertIn("1 in changelog.d/", out)

    def test_check_reports_file_and_line(self) -> None:
        write_fragments(self.root, {"a.md": "### Added\n\n- A. \n", "b.md": "- Before any heading.\n"})
        code, out, err = self.run_main("check")
        self.assertEqual(1, code)
        self.assertEqual("", out)
        self.assertIn("changelog.d/a.md:3: trailing whitespace", err)
        self.assertIn("changelog.d/b.md:1: entry before any heading", err)

    def test_render(self) -> None:
        write_fragments(self.root, {"a.md": "### Fixed\n\n- A.\n", "b.md": "### Added\n\n- B.\n"})
        code, out, _ = self.run_main("render")
        self.assertEqual(0, code)
        self.assertEqual("### Added\n\n- B.\n\n### Fixed\n\n- A.\n", out)

    def test_consume_resolves_into_against_the_root(self) -> None:
        self.changelog.write_text(CHANGELOG, encoding="utf-8")
        write_fragments(self.root, {"a.md": "### Added\n\n- A.\n"})
        code, out, err = self.run_main("consume", "--into", "CHANGELOG.md")
        self.assertEqual((0, ""), (code, err))
        self.assertIn("merged 1 fragment(s)", out)
        self.assertIn("- An existing feature.\n- A.\n", self.changelog.read_text(encoding="utf-8"))
        code, out, _ = self.run_main("consume", "--into", "CHANGELOG.md")
        self.assertEqual(0, code)
        self.assertIn("no fragments", out)

    def test_consume_failure_exits_non_zero(self) -> None:
        self.changelog.write_text("# Changelog\n", encoding="utf-8")
        write_fragments(self.root, {"a.md": "### Added\n\n- A.\n"})
        code, _, err = self.run_main("consume", "--into", "CHANGELOG.md")
        self.assertEqual(1, code)
        self.assertIn("no '## Unreleased' section", err)
        self.assertEqual(["a.md"], self.listing())


if __name__ == "__main__":
    unittest.main()
