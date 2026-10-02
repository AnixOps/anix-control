from __future__ import annotations

import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

import check_mojibake as gate

SCRIPT = Path(__file__).resolve().parent / "check_mojibake.py"


def garble(text: str) -> str:
    """Decode UTF-8 text as GBK the way the corrupting editor did.

    gb18030 decodes byte pairs like cp936, including the user-defined pairs
    that become private-use characters.
    """
    raw = text.encode("utf-8")
    if len(raw) % 2:
        # The odd final byte is lost, and "?" takes its place.
        return raw[:-1].decode("gb18030") + "?"
    return raw.decode("gb18030")


class FindMojibakeTest(unittest.TestCase):
    def test_clean_chinese_passes(self) -> None:
        for line in (
            "// 所有源码和文档必须使用 UTF-8（无 BOM），不要保存为 GBK/ANSI。",
            "// @Tags 管理端-通知",
            '"message": "缺少 API Key",',
            "对话框关闭按钮统一使用 × 或 ✕。",
            "价格 ¥10，约 €1？",
        ):
            with self.subTest(line=line):
                self.assertEqual([], gate.find_mojibake(line))
                self.assertEqual([], gate.check_text("x.go", line))

    def test_finds_a_garbled_run(self) -> None:
        line = f'"message": "{garble("节点已被禁用")}",'
        found = gate.find_mojibake(line)
        self.assertEqual(1, len(found))
        self.assertEqual("节点已被禁用", found[0][2])

    def test_finds_a_run_that_swallowed_an_ascii_byte(self) -> None:
        garbled = garble("纯Go实现的SQLite驱动")
        self.assertNotIn("G", garbled)
        repaired = "".join(fixed for _, _, fixed in gate.find_mojibake(f"// {garbled}"))
        self.assertIn("纯G", repaired)
        self.assertIn("驱动", repaired)

    def test_finds_a_run_that_lost_its_final_byte(self) -> None:
        garbled = garble("管理端")
        self.assertTrue(garbled.endswith("?"))
        found = gate.find_mojibake(f"// @Description 执行命令 @Tags {garbled}Agent")
        self.assertEqual(1, len(found))
        self.assertEqual(garbled, found[0][1])
        self.assertEqual("管理" + gate.REPLACEMENT, found[0][2])

    def test_finds_a_single_character_that_lost_its_final_byte(self) -> None:
        found = gate.find_mojibake(f"// {garble('从')}Header")
        self.assertEqual(1, len(found))

    def test_finds_private_use_from_cp936_user_defined_pairs(self) -> None:
        # cp936 decodes the user-defined pair AA E6 to U+E045.
        line = "// " + bytes.fromhex("aae6").decode("gb18030")
        problems = gate.check_text("x.go", line)
        self.assertTrue(any("private-use character U+E045" in problem for problem in problems), problems)

    def test_reports_replacement_characters_and_bom(self) -> None:
        problems = gate.check_text("x.md", "\N{ZERO WIDTH NO-BREAK SPACE}title\nbroken " + chr(0xFFFD) + "?")
        self.assertIn("x.md:1: UTF-8 BOM", problems)
        self.assertIn("x.md:2:8: U+FFFD replacement character", problems)

    def test_allowlist_masks_only_the_listed_text(self) -> None:
        example = garble("生产")
        text = f"examples: `{example}` and `{garble('编译')}`"
        problems = gate.check_text("AGENTS.md", text, [example])
        self.assertEqual(1, len(problems), problems)
        self.assertIn("编译", problems[0])


class RepositoryAllowlistTest(unittest.TestCase):
    def test_every_allowlisted_snippet_is_real_mojibake(self) -> None:
        for path, snippet in sorted(gate.ALLOWLIST):
            with self.subTest(path=path, snippet=snippet):
                self.assertTrue(gate.find_mojibake(snippet), "allowlist only intentional mojibake")


class CommandLineTest(unittest.TestCase):
    def run_gate(self, files: dict[str, bytes]) -> subprocess.CompletedProcess[str]:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            for name, data in files.items():
                (root / name).write_bytes(data)
            return subprocess.run(
                [sys.executable, str(SCRIPT), "--root", str(root), *files],
                check=False,
                text=True,
                capture_output=True,
            )

    def test_passes_clean_and_binary_files(self) -> None:
        result = self.run_gate({"a.go": "// 获取数据库实例\n".encode(), "b.png": b"\x89PNG\0\xb7\xff"})
        self.assertEqual(0, result.returncode, result.stderr)
        self.assertIn("2 files clean", result.stdout)

    def test_fails_on_mojibake_and_invalid_utf8(self) -> None:
        result = self.run_gate(
            {
                "a.go": f"// {garble('获取数据库实例')}\n".encode(),
                "b.txt": "中文".encode("gbk"),
            }
        )
        self.assertEqual(1, result.returncode)
        self.assertIn("a.go:1:4: GBK mojibake", result.stderr)
        self.assertIn("b.txt:1: not valid UTF-8", result.stderr)

    def test_reports_a_stale_allowlist_entry(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            (root / "AGENTS.md").write_text("clean\n", encoding="utf-8")
            problems = gate.scan(root, ["AGENTS.md"], frozenset({("AGENTS.md", garble("生产"))}))
        self.assertEqual(1, len(problems))
        self.assertIn("stale allowlist entry", problems[0])


if __name__ == "__main__":
    unittest.main()
