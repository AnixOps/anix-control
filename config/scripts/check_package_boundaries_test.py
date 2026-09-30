from __future__ import annotations

import os
import subprocess
import tempfile
import unittest
from pathlib import Path


SCRIPT = Path(__file__).resolve().parent / "check_package_boundaries.sh"
MODULE = "github.com/AnixOps/anix-control/v4"


class PackageBoundariesTest(unittest.TestCase):
    def run_gate(self, files: dict[str, str]) -> subprocess.CompletedProcess[str]:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            (root / "go.mod").write_text(f"module {MODULE}\n\ngo 1.22\n", encoding="utf-8")
            for name, content in files.items():
                path = root / name
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_text(content, encoding="utf-8")
            environment = dict(os.environ, GOWORK="off", GOFLAGS="-mod=mod")
            return subprocess.run(
                ["bash", str(SCRIPT)], cwd=root, env=environment, check=False, text=True, capture_output=True
            )

    def test_accepts_packages_that_use_only_public_code(self) -> None:
        result = self.run_gate({
            "pkg/sdk/sdk.go": "package sdk\n\nconst Name = \"sdk\"\n",
            "packages/demo/control/main.go": f"package main\n\nimport \"{MODULE}/pkg/sdk\"\n\nfunc main() {{ _ = sdk.Name }}\n",
            "internal/kernel/kernel.go": "package kernel\n\nconst Secret = 1\n",
        })
        self.assertEqual(0, result.returncode, result.stderr)
        self.assertIn("package boundary gate passed", result.stdout)

    def test_rejects_a_transitive_internal_dependency(self) -> None:
        result = self.run_gate({
            "internal/kernel/kernel.go": "package kernel\n\nconst Secret = 1\n",
            "pkg/sdk/sdk.go": f"package sdk\n\nimport \"{MODULE}/internal/kernel\"\n\nconst Name = kernel.Secret\n",
            "packages/demo/control/main.go": f"package main\n\nimport \"{MODULE}/pkg/sdk\"\n\nfunc main() {{ _ = sdk.Name }}\n",
        })
        self.assertNotEqual(0, result.returncode)
        self.assertIn(f"{MODULE}/packages/demo/control depends on internal packages", result.stderr)
        self.assertIn(f"{MODULE}/pkg/sdk depends on internal packages", result.stderr)

    def test_rejects_an_unlisted_test_import(self) -> None:
        result = self.run_gate({
            "internal/kernel/kernel.go": "package kernel\n\nconst Secret = 1\n",
            "pkg/sdk/sdk.go": "package sdk\n\nconst Name = \"sdk\"\n",
            "packages/demo/control/main.go": "package main\n\nfunc main() {}\n",
            "pkg/sdk/sdk_test.go": f"package sdk_test\n\nimport (\n\t\"testing\"\n\n\t\"{MODULE}/internal/kernel\"\n)\n\nfunc TestX(t *testing.T) {{ _ = kernel.Secret }}\n",
        })
        self.assertNotEqual(0, result.returncode)
        self.assertIn(f"{MODULE}/pkg/sdk tests import {MODULE}/internal/kernel", result.stderr)


if __name__ == "__main__":
    unittest.main()
