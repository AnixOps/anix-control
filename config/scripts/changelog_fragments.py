#!/usr/bin/env python3
"""Collect per-change changelog fragments from changelog.d/.

A change adds one file, changelog.d/<slug>.md, instead of editing the
"## Unreleased" section of CHANGELOG.md. Two open pull requests then never
touch the same lines, so merging one does not make the other conflict and
rebase. changelog.d/README.md documents the format.

A fragment names a section on its first line and lists entries under it:

    ### Fixed

    - The first entry. A long entry wraps, and its continuation lines are
      indented by two spaces.
    - A second entry.

    ### Added

    - An entry under another heading.

Commands (run from anywhere; --root names another checkout):

    changelog_fragments.py check
        Validate every fragment (file name, headings, entries, whitespace,
        merge-conflict markers). Prints file:line messages and exits 1.
    changelog_fragments.py render
        Print the fragments as Markdown, grouped by heading in the order of
        HEADINGS, entries in file-name order.
    changelog_fragments.py consume --into CHANGELOG.md
        Merge the fragments into the "## Unreleased" section of the file and
        delete them: "git rm" inside a git work tree, a plain delete
        otherwise. prepare_release.py does this before dating the section.

Python 3 standard library only.
"""

from __future__ import annotations

import argparse
import re
import subprocess
import sys
from pathlib import Path
from typing import NamedTuple

REPO_ROOT = Path(__file__).resolve().parents[2]
FRAGMENT_DIR = "changelog.d"
README_NAME = "README.md"

# The section headings CHANGELOG.md uses for ordinary entries, in the order
# the collector writes them (the Keep a Changelog order). Curated headings of a
# release section ("Highlights since ...", "Known issues", "Tooling") are
# written by hand at release time and are not fragment headings.
HEADINGS: tuple[str, ...] = ("Added", "Changed", "Deprecated", "Removed", "Fixed", "Security")

NAME_PATTERN = re.compile(r"[a-z0-9]+(?:-[a-z0-9]+)*\.md")
UNRELEASED_PATTERN = re.compile(r"^## Unreleased[ \t]*$", re.MULTILINE)
NEXT_RELEASE_PATTERN = re.compile(r"^## ", re.MULTILINE)
HEADING_PATTERN = re.compile(r"### (\S.*?)\s*")
FENCE_PREFIXES = ("```", "~~~")


class Problem(NamedTuple):
    path: str
    line: int
    message: str

    def __str__(self) -> str:
        return f"{self.path}:{self.line}: {self.message}"


class Entry(NamedTuple):
    """One "- " entry: the bullet line and its indented continuation lines."""

    lines: tuple[str, ...]


class Fragment(NamedTuple):
    """A parsed fragment: sections in file order, each a heading and its entries."""

    name: str
    sections: tuple[tuple[str, tuple[Entry, ...]], ...]


class FragmentError(Exception):
    """Raised when fragments or the changelog cannot be used."""

    def __init__(self, message: str, problems: tuple[Problem, ...] = ()) -> None:
        super().__init__(message)
        self.problems = problems


def heading_names() -> str:
    return ", ".join(f"### {heading}" for heading in HEADINGS)


def conflict_marker(line: str) -> str | None:
    for marker in ("<<<<<<<", ">>>>>>>", "|||||||"):
        if line == marker or line.startswith(marker + " "):
            return marker
    if line == "=======":
        return "======="
    return None


def parse_fragment(text: str, label: str) -> tuple[Fragment | None, list[Problem]]:
    """Parse one fragment's text; label is the name used in messages.

    Returns the fragment, or None with the problems when it is not valid.
    """
    problems: list[Problem] = []

    def bad(number: int, message: str) -> None:
        problems.append(Problem(label, number, message))

    if text.startswith("\ufeff"):
        bad(1, "byte order mark: save the file as UTF-8 without BOM")
        text = text[1:]
    lines = text.split("\n")
    if lines[-1] == "":
        lines.pop()
    elif text:
        bad(len(lines), "no newline at the end of the file")

    # Each section is [heading, entries as lists of lines, line number of the heading].
    sections: list[list] = []
    section_by_name: dict[str, list] = {}
    current: list | None = None  # the section new entries go to
    entry: list[str] | None = None  # the entry continuation lines go to
    skipping = False  # after a reported heading or entry problem: do not repeat it for every line
    pending_blank = 0
    carriage_return_reported = False

    for number, raw in enumerate(lines, start=1):
        line = raw
        if line.endswith("\r"):
            if not carriage_return_reported:
                bad(number, "carriage return: use LF line endings")
                carriage_return_reported = True
            line = line[:-1]
        marker = conflict_marker(line)
        if marker is not None:
            bad(number, f"merge conflict marker {marker!r}: resolve the conflict")
            continue
        if "\t" in line:
            bad(number, "tab character: indent with spaces")
        if line != line.rstrip():
            bad(number, "trailing whitespace")
        if not line.strip():
            pending_blank += 1
            continue
        blanks, pending_blank = pending_blank, 0

        if line.startswith("#"):
            match = HEADING_PATTERN.fullmatch(line)
            heading = match.group(1) if match else None
            entry = None
            if heading not in HEADINGS:
                bad(number, f"unknown heading {line.strip()!r}: use one of {heading_names()}")
                current, skipping = None, True
            elif heading in section_by_name:
                current = section_by_name[heading]
                bad(number, f"duplicate heading '### {heading}' (first at line {current[2]}): keep its entries under one")
            else:
                current, skipping = [heading, [], number], False
                sections.append(current)
                section_by_name[heading] = current
        elif line.startswith("- ") or line == "-":
            if not line[1:].strip():
                bad(number, "empty entry")
            elif current is None:
                if not skipping:
                    bad(number, "entry before any heading: start the file with a heading such as '### Changed'")
                    skipping = True
            else:
                entry = [line]
                current[1].append(entry)
        elif line.startswith("  "):
            if entry is not None:
                if blanks:
                    entry.append("")
                entry.append(line)
            elif not skipping:
                bad(number, "indented text outside an entry: it must continue a '- ' entry")
                skipping = True
        elif line.startswith(("* ", "+ ")):
            bad(number, "use '- ' for entries, the bullet of CHANGELOG.md")
        else:
            bad(number, "text outside an entry: start an entry with '- ' and indent its continuation lines by two spaces")

    if lines and not lines[-1].strip():
        bad(len(lines), "blank line at the end of the file")
    for heading, entries, number in sections:
        if not entries:
            bad(number, f"section '### {heading}' has no entries")
    if not sections and not problems:
        bad(1, "fragment has no entries: start with a heading such as '### Changed' and add '- ' entries")

    if problems:
        return None, problems
    parsed = tuple((heading, tuple(Entry(tuple(lines_)) for lines_ in entries)) for heading, entries, _ in sections)
    return Fragment(label.rsplit("/", 1)[-1], parsed), []


def collect(root: Path) -> list[Fragment]:
    """Read and validate every fragment of root/changelog.d, in file-name order.

    README.md is documentation and is never collected. Raises FragmentError
    with every problem found when any file is not a valid fragment.
    """
    directory = root / FRAGMENT_DIR
    if not directory.is_dir():
        return []
    problems: list[Problem] = []
    fragments: list[Fragment] = []
    for path in sorted(directory.iterdir(), key=lambda item: item.name):
        label = f"{FRAGMENT_DIR}/{path.name}"
        if path.name == README_NAME and path.is_file():
            continue
        if not path.is_file():
            problems.append(Problem(label, 1, f"not a file: only <slug>.md fragments and {README_NAME} belong in {FRAGMENT_DIR}"))
            continue
        if NAME_PATTERN.fullmatch(path.name) is None:
            problems.append(
                Problem(
                    label,
                    1,
                    "file name must be <slug>.md, the slug lowercase letters and digits with single dashes "
                    "(for example 'node-group-token-reset.md')",
                )
            )
            continue
        try:
            text = path.read_bytes().decode("utf-8")
        except UnicodeDecodeError as error:
            problems.append(Problem(label, 1, f"not valid UTF-8: {error.reason}"))
            continue
        fragment, found = parse_fragment(text, label)
        problems.extend(found)
        if fragment is not None:
            fragments.append(fragment)
    if problems:
        raise FragmentError("\n".join(str(problem) for problem in problems), tuple(problems))
    return fragments


def grouped(fragments: list[Fragment]) -> dict[str, list[Entry]]:
    """The entries per heading, headings in HEADINGS order, entries in file-name order.

    A heading without entries is left out.
    """
    result: dict[str, list[Entry]] = {heading: [] for heading in HEADINGS}
    for fragment in sorted(fragments, key=lambda item: item.name):
        for heading, entries in fragment.sections:
            result[heading].extend(entries)
    return {heading: entries for heading, entries in result.items() if entries}


def entry_lines(entries: list[Entry]) -> list[str]:
    return [line for entry in entries for line in entry.lines]


def render(fragments: list[Fragment]) -> str:
    """The fragments as Markdown; empty when there are none."""
    blocks = [
        "\n".join([f"### {heading}", "", *entry_lines(entries)]) + "\n" for heading, entries in grouped(fragments).items()
    ]
    return "\n".join(blocks)


def split_unreleased(body: str) -> tuple[list[str], list[list]]:
    """Split the lines of the Unreleased section into text before the first
    "###" heading and its sections: [heading, lines], blank edges trimmed."""
    preamble: list[str] = []
    sections: list[list] = []
    fenced = False
    for line in body.split("\n"):
        if line.lstrip().startswith(FENCE_PREFIXES):
            fenced = not fenced
        match = None if fenced else HEADING_PATTERN.fullmatch(line)
        if match:
            sections.append([match.group(1), []])
        else:
            (sections[-1][1] if sections else preamble).append(line)

    def trim(lines: list[str]) -> list[str]:
        start, end = 0, len(lines)
        while start < end and not lines[start].strip():
            start += 1
        while end > start and not lines[end - 1].strip():
            end -= 1
        return lines[start:end]

    return trim(preamble), [[heading, trim(lines)] for heading, lines in sections]


def insertion_index(sections: list[list], heading: str) -> int:
    """Where a new section goes: after the last section that comes earlier in
    HEADINGS, else before the first that comes later, else last."""
    rank = HEADINGS.index(heading)
    ranks = [HEADINGS.index(name) if name in HEADINGS else None for name, _ in sections]
    earlier = [index for index, other in enumerate(ranks) if other is not None and other < rank]
    if earlier:
        return earlier[-1] + 1
    later = [index for index, other in enumerate(ranks) if other is not None and other > rank]
    if later:
        return later[0]
    return len(sections)


def merge_into_changelog(changelog: str, fragments: list[Fragment]) -> str:
    """Return the changelog text with the fragments merged into "## Unreleased".

    Entries join an existing "### <heading>" of that section, after its own
    entries, and a missing heading is created where HEADINGS order puts it, so
    no heading appears twice. Nothing else in the file changes. Without
    fragments the text is returned as it is.
    """
    by_heading = grouped(fragments)
    if not by_heading:
        return changelog
    match = UNRELEASED_PATTERN.search(changelog)
    if match is None:
        raise FragmentError("CHANGELOG has no '## Unreleased' section")
    following = NEXT_RELEASE_PATTERN.search(changelog, match.end())
    end = following.start() if following else len(changelog)
    head, body, tail = changelog[: match.end()], changelog[match.end() : end], changelog[end:]

    preamble, sections = split_unreleased(body)
    for heading, entries in by_heading.items():
        added = entry_lines(entries)
        existing = next((section for section in sections if section[0] == heading), None)
        if existing is not None:
            existing[1] = [*existing[1], *added]
        else:
            sections.insert(insertion_index(sections, heading), [heading, added])

    blocks = ["\n".join(preamble)] if preamble else []
    blocks += ["\n".join([f"### {heading}", "", *lines]) if lines else f"### {heading}" for heading, lines in sections]
    merged = head + "\n\n" + "\n\n".join(blocks) + "\n"
    return merged + "\n" + tail if tail else merged


def git(root: Path, *args: str) -> subprocess.CompletedProcess[str] | None:
    try:
        return subprocess.run(["git", "-C", str(root), *args], capture_output=True, text=True, check=False)
    except OSError:
        return None


def remove_fragments(root: Path, fragments: list[Fragment]) -> None:
    """Delete the fragment files: git rm for tracked files inside a git work
    tree, a plain delete for the rest."""
    names = [f"{FRAGMENT_DIR}/{fragment.name}" for fragment in fragments]
    if not names:
        return
    inside = git(root, "rev-parse", "--is-inside-work-tree")
    if inside is not None and inside.returncode == 0 and inside.stdout.strip() == "true":
        listed = git(root, "ls-files", "-z", "--", *names)
        tracked = sorted(name for name in (listed.stdout.split("\0") if listed else []) if name)
        if tracked:
            git(root, "rm", "-q", "-f", "--", *tracked)
    for name in names:
        path = root / name
        if path.exists():
            path.unlink()


def consume(root: Path, changelog: Path) -> list[str]:
    """Merge the fragments into the changelog's Unreleased section and delete them.

    Nothing is written or deleted when a fragment is invalid or the changelog
    has no "## Unreleased" section. Returns the consumed file names; a second
    call finds no fragments and changes nothing.
    """
    fragments = collect(root)
    if not fragments:
        return []
    merged = merge_into_changelog(changelog.read_text(encoding="utf-8"), fragments)
    changelog.write_text(merged, encoding="utf-8")
    remove_fragments(root, fragments)
    return [fragment.name for fragment in fragments]


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--root", type=Path, default=REPO_ROOT, help="repository root (default: this checkout)")
    commands = parser.add_subparsers(dest="command", required=True)
    commands.add_parser("check", help="validate every fragment")
    commands.add_parser("render", help="print the fragments as Markdown")
    consume_parser = commands.add_parser("consume", help="merge the fragments into CHANGELOG.md and delete them")
    consume_parser.add_argument("--into", type=Path, required=True, help="the changelog file, relative to --root")
    args = parser.parse_args(argv)
    root = args.root.resolve()

    try:
        if args.command == "check":
            fragments = collect(root)
            print(f"changelog fragments ok: {len(fragments)} in {FRAGMENT_DIR}/")
        elif args.command == "render":
            sys.stdout.write(render(collect(root)))
        else:
            into = args.into if args.into.is_absolute() else root / args.into
            consumed = consume(root, into)
            if consumed:
                print(f"merged {len(consumed)} fragment(s) into {args.into}: {', '.join(consumed)}")
            else:
                print(f"no fragments in {FRAGMENT_DIR}/ to consume")
    except FragmentError as error:
        print(str(error), file=sys.stderr)
        if error.problems:
            print(f"error: {len(error.problems)} problem(s) in {FRAGMENT_DIR}/; see changelog.d/README.md", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
