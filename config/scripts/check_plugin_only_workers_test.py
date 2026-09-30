from __future__ import annotations

import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

import check_plugin_only_workers as gate


SCRIPT = Path(__file__).resolve().parent / "check_plugin_only_workers.py"


def server_source(*constructors: str) -> str:
    calls = "\n".join(f"\t_ = service.{name}(nil)" for name in constructors)
    return f"package main\n\nfunc start() {{\n{calls}\n}}\n"


class PluginOnlyWorkersTest(unittest.TestCase):
    def run_gate(self, source: str, test_source: str = "") -> subprocess.CompletedProcess[str]:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            (root / "workers.go").write_text(source, encoding="utf-8")
            if test_source:
                (root / "workers_test.go").write_text(test_source, encoding="utf-8")
            return subprocess.run(
                [sys.executable, str(SCRIPT), "--server", str(root)], check=False, text=True, capture_output=True
            )

    def test_accepts_the_listed_workers(self) -> None:
        result = self.run_gate(
            server_source(*sorted(gate.LEGACY_DOMAIN_WORKERS | gate.KERNEL_WORKERS)),
            test_source=server_source("NewTicketReminderWorker"),
        )
        self.assertEqual(0, result.returncode, result.stderr)
        self.assertIn("7 legacy domain workers remain", result.stdout)

    def test_rejects_a_new_domain_worker(self) -> None:
        result = self.run_gate(server_source(*sorted(gate.LEGACY_DOMAIN_WORKERS), "NewTicketReminderWorker"))
        self.assertNotEqual(0, result.returncode)
        self.assertIn("new domain worker NewTicketReminderWorker in workers.go:", result.stderr)

    def test_requires_removing_an_extracted_worker_from_the_list(self) -> None:
        remaining = sorted(gate.LEGACY_DOMAIN_WORKERS - {"NewForwardLatencyProber"})
        result = self.run_gate(server_source(*remaining))
        self.assertNotEqual(0, result.returncode)
        self.assertIn("NewForwardLatencyProber is no longer started", result.stderr)

    def test_the_legacy_list_only_shrinks(self) -> None:
        self.assertLessEqual(len(gate.LEGACY_DOMAIN_WORKERS), gate.MAX_LEGACY_DOMAIN_WORKERS)
        self.assertEqual(7, gate.MAX_LEGACY_DOMAIN_WORKERS)


if __name__ == "__main__":
    unittest.main()
