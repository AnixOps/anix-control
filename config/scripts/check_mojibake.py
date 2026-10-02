#!/usr/bin/env python3
"""Reject GBK mojibake and other text corruption in tracked files.

GBK mojibake is UTF-8 text that was once decoded as GBK (Windows cp936) and
saved again as UTF-8: `"发送心跳".encode().decode("gbk")`. Every Chinese
character becomes one and a half GBK characters, and an odd final byte is
lost and shows up as `?`, which often swallows the next ASCII byte (a newline,
space, quote or parenthesis). The gate scans every tracked text file and
reports:

  mojibake  a run of non-ASCII characters whose GBK bytes decode as UTF-8
            Chinese, including a run that lost its final byte and ends in `?`
  U+FFFD    the Unicode replacement character
  PUA       a private-use character (U+E000-U+F8FF); cp936 decodes its
            user-defined byte pairs to these
  encoding  a file that is not valid UTF-8, or starts with a BOM

Binary files (any NUL byte) are skipped. Intentional examples go in
ALLOWLIST as (path, exact text) pairs; an entry that no longer matches fails
the gate, so the list cannot go stale. Write test fixtures with escapes or
build them at runtime instead of allowlisting them.

Usage: check_mojibake.py [--root DIR] [PATH ...]
With no paths it scans `git ls-files` under the root.
"""

from __future__ import annotations

import argparse
import re
import subprocess
import sys
from pathlib import Path

# (path, text) pairs that are intentional: the corruption examples in
# AGENTS.md's "UTF-8 And Chinese Copy" section.
ALLOWLIST: frozenset[tuple[str, str]] = frozenset(
    {
        ("AGENTS.md", "\u9422\u71b6\u9a87"),
        ("AGENTS.md", "\u7f02\u682c\u7627"),
    }
)

CJK = re.compile("[\u4e00-\u9fff]")
NON_ASCII_RUN = re.compile("[^\x00-\x7f]+")
PUA = re.compile("[\ue000-\uf8ff]")
REPLACEMENT = "\ufffd"
BOM = "\ufeff"


def gbk_bytes(text: str) -> bytes | None:
    """Return the bytes a cp936 decode turned into text, or None.

    cp936 maps its user-defined byte pairs to private-use characters, which
    Python's gbk codec refuses to encode; gb18030 encodes them back to the
    same two bytes. cp936 also decodes the lone byte 0x80 as the euro sign.
    A character outside GBK (four bytes in gb18030) cannot be mojibake.
    """
    out = bytearray()
    for char in text:
        if char == "\N{EURO SIGN}":
            out += b"\x80"
            continue
        try:
            encoded = char.encode("gb18030")
        except UnicodeEncodeError:
            return None
        if len(encoded) > 2:
            return None
        out += encoded
    return bytes(out)


def _shrinks(fixed: str, garbled_length: int) -> bool:
    # The repaired text holds Chinese and fewer non-ASCII characters than the
    # garbled run. An ASCII byte swallowed by a GBK pair comes back as ASCII.
    non_ascii = sum(1 for char in fixed if ord(char) > 0x7F)
    return bool(CJK.search(fixed)) and non_ascii < garbled_length


def repair(segment: str, lost_byte: bool) -> str | None:
    """Return the repaired text for a garbled segment, or None.

    With lost_byte the segment was followed by `?`: its GBK bytes must end in
    the first two bytes of a Chinese character, shown as U+FFFD.
    """
    raw = gbk_bytes(segment)
    if raw is None:
        return None
    if not lost_byte:
        try:
            fixed = raw.decode("utf-8")
        except UnicodeDecodeError:
            return None
        return fixed if _shrinks(fixed, len(segment)) else None
    head, tail = raw[:-2], raw[-2:]
    if len(tail) != 2 or not (0xE4 <= tail[0] <= 0xE9 and 0x80 <= tail[1] <= 0xBF):
        return None
    try:
        fixed = head.decode("utf-8") + REPLACEMENT
    except UnicodeDecodeError:
        return None
    # The "?" stands in for the lost character's last byte.
    return fixed if _shrinks(fixed.replace(REPLACEMENT, "\u4e00"), len(segment) + 1) else None


def find_mojibake(line: str) -> list[tuple[int, str, str]]:
    """Return (column, garbled text, repaired guess) for each mojibake run."""
    found: list[tuple[int, str, str]] = []
    for match in NON_ASCII_RUN.finditer(line):
        run = match.group()
        followed_by_question = line[match.end() : match.end() + 1] == "?"
        start = 0
        while start < len(run):
            hit = None
            # The longest garbled segment that starts here.
            for end in range(len(run), start, -1):
                segment = run[start:end]
                fixed = repair(segment, lost_byte=False)
                if fixed is None and end == len(run) and followed_by_question:
                    fixed = repair(segment, lost_byte=True)
                    segment += "?" if fixed is not None else ""
                if fixed is not None:
                    hit = (match.start() + start, segment, fixed, end)
                    break
            if hit is None:
                start += 1
                continue
            found.append(hit[:3])
            start = hit[3]
    return found


def check_text(path: str, text: str, allowed: list[str] | None = None) -> list[str]:
    """Return the problems in text; allowed snippets are masked first."""
    problems: list[str] = []
    allowed = allowed or []
    if text.startswith(BOM):
        problems.append(f"{path}:1: UTF-8 BOM")
    for number, line in enumerate(text.split("\n"), 1):
        for snippet in allowed:
            line = line.replace(snippet, " " * len(snippet))
        for column, garbled, fixed in find_mojibake(line):
            problems.append(f"{path}:{number}:{column + 1}: GBK mojibake {garbled!r} (likely {fixed!r})")
        if REPLACEMENT in line:
            problems.append(f"{path}:{number}:{line.index(REPLACEMENT) + 1}: U+FFFD replacement character")
        for match in PUA.finditer(line):
            problems.append(f"{path}:{number}:{match.start() + 1}: private-use character U+{ord(match.group()):04X}")
    return problems


def read_text(root: Path, path: str) -> tuple[str | None, list[str]]:
    """Return (text, problems); text is None for binary or invalid files."""
    file_path = root / path
    if not file_path.is_file():
        return None, []
    data = file_path.read_bytes()
    if b"\0" in data:
        return None, []
    try:
        return data.decode("utf-8"), []
    except UnicodeDecodeError as error:
        line = data[: error.start].count(b"\n") + 1
        return None, [f"{path}:{line}: not valid UTF-8 ({error.reason})"]


def tracked_files(root: Path) -> list[str]:
    output = subprocess.run(["git", "-C", str(root), "ls-files", "-z"], check=True, capture_output=True).stdout
    return [name for name in output.decode("utf-8").split("\0") if name]


def scan(root: Path, paths: list[str], allowlist: frozenset[tuple[str, str]] = ALLOWLIST) -> list[str]:
    problems: list[str] = []
    for path in paths:
        text, read_problems = read_text(root, path)
        problems.extend(read_problems)
        if text is None:
            continue
        allowed = sorted({snippet for allowed_path, snippet in allowlist if allowed_path == path})
        problems.extend(check_text(path, text, allowed))
        for snippet in allowed:
            if snippet not in text:
                problems.append(f"{path}: stale allowlist entry {snippet!r}; remove it from check_mojibake.py")
    return problems


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=(__doc__ or "").splitlines()[0])
    parser.add_argument("--root", default=".", help="repository root (default: the current directory)")
    parser.add_argument("paths", nargs="*", help="files to scan, relative to the root (default: git ls-files)")
    args = parser.parse_args(argv)

    root = Path(args.root)
    paths = args.paths or tracked_files(root)
    problems = scan(root, paths)
    if problems:
        for problem in problems:
            print(problem, file=sys.stderr)
        print(f"check_mojibake: {len(problems)} problem(s); see AGENTS.md \"UTF-8 And Chinese Copy\"", file=sys.stderr)
        return 1
    print(f"check_mojibake: {len(paths)} files clean")
    return 0


if __name__ == "__main__":
    sys.exit(main())
