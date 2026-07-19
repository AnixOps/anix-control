from __future__ import annotations

import importlib.util
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parents[2]
RELEASE_STAGE = REPO_ROOT / "config" / "scripts" / "check_release_stage.py"
BUILDER = REPO_ROOT / "packages" / "shared" / "build_package.py"
RELEASE_STAGE_SPEC = importlib.util.spec_from_file_location("release_stage", RELEASE_STAGE)
if RELEASE_STAGE_SPEC is None or RELEASE_STAGE_SPEC.loader is None:
    raise RuntimeError("cannot load release-stage checker")
RELEASE_STAGE_MODULE = importlib.util.module_from_spec(RELEASE_STAGE_SPEC)
sys.modules[RELEASE_STAGE_SPEC.name] = RELEASE_STAGE_MODULE
RELEASE_STAGE_SPEC.loader.exec_module(RELEASE_STAGE_MODULE)


class V4ReleaseStageContractTest(unittest.TestCase):
    def test_v4_stage_requires_identity_platform_and_sixteen_artifacts(self) -> None:
        contract = RELEASE_STAGE_MODULE.read_contract(REPO_ROOT / "config" / "scripts" / "release-stage-contract.json")
        decision = RELEASE_STAGE_MODULE.validate_stage(REPO_ROOT, contract, "v4.0.0")

        self.assertIn("identity-platform", decision.package_ids)
        self.assertEqual(16, len(decision.package_ids))

        with tempfile.TemporaryDirectory() as temporary:
            output = Path(temporary) / "packages"
            result = subprocess.run(
                [sys.executable, str(BUILDER), "--all", "--version", "4.0.0", "--out", str(output)],
                cwd=REPO_ROOT,
                check=False,
                text=True,
                capture_output=True,
            )

            self.assertEqual(0, result.returncode, result.stderr or result.stdout)
            self.assertEqual(16, len(list(output.glob("*.anxp"))))

    def test_self_test_rejects_a_v4_contract_without_identity_platform(self) -> None:
        contract = json.loads((REPO_ROOT / "config" / "scripts" / "release-stage-contract.json").read_text(encoding="utf-8"))
        v4_stage = next(stage for stage in contract["stages"] if stage["id"] == "4.0")
        v4_stage["required_packages"] = [
            package for package in v4_stage["required_packages"] if package["id"] != "identity-platform"
        ]

        with tempfile.TemporaryDirectory() as temporary:
            declaration = Path(temporary) / "release-stage-contract.json"
            declaration.write_text(json.dumps(contract), encoding="utf-8")
            result = subprocess.run(
                [sys.executable, str(RELEASE_STAGE), "--self-test", "--declaration", str(declaration)],
                cwd=REPO_ROOT,
                check=False,
                text=True,
                capture_output=True,
            )

        self.assertNotEqual(0, result.returncode)
        self.assertIn("identity-platform", result.stdout + result.stderr)


if __name__ == "__main__":
    unittest.main()
