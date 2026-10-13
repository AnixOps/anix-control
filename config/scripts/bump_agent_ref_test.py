from __future__ import annotations

import contextlib
import io
import shutil
import tempfile
import unittest
from pathlib import Path

import bump_agent_ref as bump

REPO_ROOT = Path(__file__).resolve().parents[2]
COMMIT = "0123456789abcdef0123456789abcdef01234567"
FILES = (bump.CI_WORKFLOW, bump.WORKFLOW_CHECK, bump.AGENT_E2E_BUILD, bump.AGENT_RELEASE_FILE)


class BumpAgentRefTest(unittest.TestCase):
    def setUp(self) -> None:
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        self.root = Path(directory.name)
        # A copy of the real files, so the test fails when one of them no
        # longer has the pin the script edits.
        for relative in FILES:
            target = self.root / relative
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(REPO_ROOT / relative, target)

    def read(self, relative: str) -> str:
        return (self.root / relative).read_text(encoding="utf-8")

    def snapshot(self) -> dict[str, str]:
        return {relative: self.read(relative) for relative in FILES}

    def run_main(self, *argv: str) -> tuple[int, str]:
        stderr = io.StringIO()
        with contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(stderr):
            try:
                status = bump.main(list(argv), root=self.root)
            except SystemExit as error:
                status = error.code
        return status, stderr.getvalue()

    def test_moves_every_pin_and_the_workflow_check_passes(self) -> None:
        status, stderr = self.run_main(COMMIT, "v5.0.0-rc.1")
        self.assertEqual(0, status, stderr)
        workflow = self.read(bump.CI_WORKFLOW)
        self.assertIn(f"  AGENT_REF: '{COMMIT}'   # anix-agent v5.0.0-rc.1\n", workflow)
        self.assertIn('-ldflags "-X github.com/AnixOps/anix-agent/v5/cmd.version=5.0.0-rc.1-e2e"', workflow)
        self.assertIn(f"require_text \"AGENT_REF: '{COMMIT}'\" \"pinned Agent source commit\"", self.read(bump.WORKFLOW_CHECK))
        build = self.read(bump.AGENT_E2E_BUILD)
        self.assertIn('"-X github.com/AnixOps/anix-agent/v5/cmd.version=" + agentE2EVersion', build)
        self.assertIn('\nconst agentE2EVersion = "5.0.0-rc.1-e2e"\n', build)
        self.assertEqual("v5.0.0-rc.1\n", self.read(bump.AGENT_RELEASE_FILE))
        # A second run with the same arguments changes nothing.
        before = self.snapshot()
        self.assertEqual(0, self.run_main(COMMIT, "v5.0.0-rc.1")[0])
        self.assertEqual(before, self.snapshot())

    def test_refuses_a_malformed_commit_or_tag_before_any_change(self) -> None:
        before = self.snapshot()
        for commit, tag in (
            (COMMIT[:12], "v5.0.0-rc.1"),
            (COMMIT.upper(), "v5.0.0-rc.1"),
            ("g" * 40, "v5.0.0-rc.1"),
            (COMMIT + "0", "v5.0.0-rc.1"),
            ("", "v5.0.0-rc.1"),
            (COMMIT, "5.0.0-rc.1"),
            (COMMIT, "v5.0"),
            (COMMIT, "v5.0.0-beta.1"),
            (COMMIT, "latest"),
            (COMMIT, "v5.0.0-rc.1 "),
        ):
            with self.subTest(commit=commit, tag=tag):
                status, stderr = self.run_main(commit, tag)
                self.assertEqual(2, status)
                self.assertIn("error:", stderr)
                self.assertEqual(before, self.snapshot())

    def test_a_missing_or_repeated_pin_changes_no_file(self) -> None:
        workflow = self.root / bump.CI_WORKFLOW
        original = workflow.read_text(encoding="utf-8")
        line = next(line for line in original.splitlines() if "cmd.version=" in line)
        for broken in (original.replace(line + "\n", ""), original.replace(line + "\n", line + "\n" + line + "\n")):
            with self.subTest(matches="none" if len(broken) < len(original) else "two"):
                workflow.write_text(broken, encoding="utf-8")
                before = self.snapshot()
                status, stderr = self.run_main(COMMIT, "v5.0.0-rc.1")
                self.assertEqual(2, status)
                self.assertIn(bump.CI_WORKFLOW, stderr)
                self.assertEqual(before, self.snapshot())


if __name__ == "__main__":
    unittest.main()
