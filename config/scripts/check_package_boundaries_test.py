from __future__ import annotations

import os
import subprocess
import tempfile
import unittest
from pathlib import Path


SCRIPT = Path(__file__).resolve().parent / "check_package_boundaries.sh"
MODULE = "github.com/AnixOps/anix-control/v4"
SDK_MODULE = "github.com/AnixOps/anix-control/sdk"
IDENTITY_MODULE = "github.com/AnixOps/anix-control/identity"


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
            "packages/demo/lib/lib.go": "package lib\n\nconst Name = \"demo\"\n",
            "packages/demo/control/main.go": f"package main\n\nimport \"{MODULE}/packages/demo/lib\"\n\nfunc main() {{ _ = lib.Name }}\n",
            "internal/kernel/kernel.go": "package kernel\n\nconst Secret = 1\n",
            "sdk/go.mod": f"module {SDK_MODULE}\n\ngo 1.22\n",
            "sdk/moduletls/moduletls.go": "package moduletls\n\nconst Domain = \"anixops\"\n",
        })
        self.assertEqual(0, result.returncode, result.stderr)
        self.assertIn("package boundary gate passed", result.stdout)
        self.assertIn("1 SDK packages", result.stdout)

    def test_rejects_a_transitive_internal_dependency(self) -> None:
        result = self.run_gate({
            "internal/kernel/kernel.go": "package kernel\n\nconst Secret = 1\n",
            "packages/demo/lib/lib.go": f"package lib\n\nimport \"{MODULE}/internal/kernel\"\n\nconst Name = kernel.Secret\n",
            "packages/demo/control/main.go": f"package main\n\nimport \"{MODULE}/packages/demo/lib\"\n\nfunc main() {{ _ = lib.Name }}\n",
        })
        self.assertNotEqual(0, result.returncode)
        self.assertIn(f"{MODULE}/packages/demo/control depends on internal packages", result.stderr)
        self.assertIn(f"{MODULE}/packages/demo/lib depends on internal packages", result.stderr)

    def test_rejects_an_unlisted_test_import(self) -> None:
        result = self.run_gate({
            "internal/kernel/kernel.go": "package kernel\n\nconst Secret = 1\n",
            "packages/demo/lib/lib.go": "package lib\n\nconst Name = \"demo\"\n",
            "packages/demo/lib/lib_test.go": f"package lib_test\n\nimport (\n\t\"testing\"\n\n\t\"{MODULE}/internal/kernel\"\n)\n\nfunc TestX(t *testing.T) {{ _ = kernel.Secret }}\n",
        })
        self.assertNotEqual(0, result.returncode)
        self.assertIn(f"{MODULE}/packages/demo/lib tests import {MODULE}/internal/kernel", result.stderr)

    def test_rejects_an_sdk_that_depends_on_the_kernel_module(self) -> None:
        result = self.run_gate({
            "packages/demo/control/main.go": "package main\n\nfunc main() {}\n",
            "kernelapi/api.go": "package kernelapi\n\nconst Version = 4\n",
            "sdk/go.mod": f"module {SDK_MODULE}\n\ngo 1.22\n\nrequire {MODULE} v4.0.0\n\nreplace {MODULE} => ../\n",
            "sdk/client/client.go": f"package client\n\nimport \"{MODULE}/kernelapi\"\n\nconst Version = kernelapi.Version\n",
        })
        self.assertNotEqual(0, result.returncode)
        self.assertIn("the SDK module depends on the kernel module", result.stderr)
        self.assertIn(f"{MODULE}/kernelapi", result.stderr)

    def test_rejects_an_identity_module_that_depends_on_the_kernel_module(self) -> None:
        result = self.run_gate({
            "packages/demo/control/main.go": "package main\n\nfunc main() {}\n",
            "kernelapi/api.go": "package kernelapi\n\nconst Version = 4\n",
            "identity/go.mod": f"module {IDENTITY_MODULE}\n\ngo 1.22\n\nrequire {MODULE} v4.0.0\n\nreplace {MODULE} => ../\n",
            "identity/token/token.go": f"package token\n\nimport \"{MODULE}/kernelapi\"\n\nconst Version = kernelapi.Version\n",
        })
        self.assertNotEqual(0, result.returncode)
        self.assertIn("the identity module depends on the kernel module", result.stderr)

    def test_counts_identity_packages(self) -> None:
        result = self.run_gate({
            "packages/demo/control/main.go": "package main\n\nfunc main() {}\n",
            "identity/go.mod": f"module {IDENTITY_MODULE}\n\ngo 1.22\n",
            "identity/token/token.go": "package token\n\nconst Issuer = \"anixops-identity\"\n",
        })
        self.assertEqual(0, result.returncode, result.stderr)
        self.assertIn("1 identity packages", result.stdout)


if __name__ == "__main__":
    unittest.main()
