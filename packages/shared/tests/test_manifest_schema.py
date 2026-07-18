from __future__ import annotations

import hashlib
import importlib.util
import json
import base64
import subprocess
import sys
import tarfile
import tempfile
import unittest
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parents[3]
BUILDER = REPO_ROOT / "packages" / "shared" / "build_package.py"
RELEASE_STAGE = REPO_ROOT / "config" / "scripts" / "check_release_stage.py"
RELEASE_STAGE_SPEC = importlib.util.spec_from_file_location("release_stage", RELEASE_STAGE)
if RELEASE_STAGE_SPEC is None or RELEASE_STAGE_SPEC.loader is None:
    raise RuntimeError("cannot load release-stage checker")
RELEASE_STAGE_MODULE = importlib.util.module_from_spec(RELEASE_STAGE_SPEC)
sys.modules[RELEASE_STAGE_SPEC.name] = RELEASE_STAGE_MODULE
RELEASE_STAGE_SPEC.loader.exec_module(RELEASE_STAGE_MODULE)


class V4PackageManifestSchemaTest(unittest.TestCase):
    def test_all_v4_packages_materialize_signed_v2_artifacts(self) -> None:
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

            artifacts = sorted(output.glob("*.anxp"))
            sboms = sorted(output.glob("*.sbom.spdx.json"))
            self.assertEqual(15, len(artifacts))
            self.assertEqual(15, len(sboms))

            for artifact_path in artifacts:
                stem = artifact_path.name.removesuffix(".anxp")
                manifest_path = output / f"{stem}.manifest.json"
                signature_path = output / f"{stem}.manifest.sig"
                public_key_path = output / f"{stem}.public-key.pem"
                manifest = json.loads(manifest_path.read_text(encoding="utf-8"))

                self.assertEqual("v2", manifest["api_version"])
                self.assertTrue(manifest["control_entrypoint"]["path"])
                self.assertTrue(manifest["migrations"]["index"])
                self.assertTrue(manifest["compatibility_routes"]["path"])
                self.assertRegex(manifest["route_contract_digest"], r"^[0-9a-f]{64}$")
                self.assertEqual(hashlib.sha256(artifact_path.read_bytes()).hexdigest(), manifest["artifact_sha256"])

                raw_signature_path = output / f"{stem}.manifest.raw.sig"
                raw_signature_path.write_bytes(base64.b64decode(signature_path.read_bytes()))
                verify = subprocess.run(
                    [
                        "openssl",
                        "pkeyutl",
                        "-verify",
                        "-pubin",
                        "-inkey",
                        str(public_key_path),
                        "-rawin",
                        "-in",
                        str(manifest_path),
                        "-sigfile",
                        str(raw_signature_path),
                    ],
                    check=False,
                    text=True,
                    capture_output=True,
                )
                self.assertEqual(0, verify.returncode, verify.stderr or verify.stdout)

                with tarfile.open(artifact_path, mode="r:") as archive:
                    members = archive.getmembers()
                    self.assertEqual(sorted(member.name for member in members), [member.name for member in members])
                    self.assertTrue(all(member.mtime == 0 for member in members))

    def test_v4_release_stage_requires_the_materialized_manifest_contract(self) -> None:
        result = subprocess.run(
            [sys.executable, str(RELEASE_STAGE), "--tag", "v4.0.0"],
            cwd=REPO_ROOT,
            check=False,
            text=True,
            capture_output=True,
        )
        self.assertEqual(0, result.returncode, result.stderr or result.stdout)

    def test_v4_release_stage_rejects_legacy_manifest_without_host_metadata(self) -> None:
        contract = RELEASE_STAGE_MODULE.read_contract(REPO_ROOT / "config" / "scripts" / "release-stage-contract.json")
        stage = next(item for item in contract["stages"] if item["id"] == "4.0")
        contract["stages"] = [{**stage, "required_packages": [stage["required_packages"][0]]}]

        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            (root / "go.mod").write_text("module github.com/AnixOps/anix-control/v4\n", encoding="utf-8")
            for relative_path, values in {
                "config/config.yaml.example": (False, False, False),
                "config/config.prod.yaml": (False, False, False),
                "config/config.dev.yaml.example": (True, True, False),
            }.items():
                (root / relative_path).parent.mkdir(parents=True, exist_ok=True)
                (root / relative_path).write_text(
                    "plugins:\n"
                    f"  control_execution_enabled: {str(values[0]).lower()}\n"
                    f"  dispatch_enabled: {str(values[1]).lower()}\n"
                    f"  topology_execution_enabled: {str(values[2]).lower()}\n",
                    encoding="utf-8",
                )
            package_root = root / "packages" / "subscription"
            (package_root / "webui").mkdir(parents=True)
            (package_root / "webui" / "index.mjs").write_text("export default {};\n", encoding="utf-8")
            (package_root / "manifest.template.json").write_text(
                json.dumps(
                    {
                        "id": "subscription",
                        "version": "4.0.0",
                        "targets": ["control"],
                        "webui": {
                            "bundle": {"path": "webui/index.mjs", "sha256": "fixture"},
                            "menus": [{"id": "subscription.main"}],
                            "routes": [{"id": "subscription.main"}],
                        },
                    }
                ),
                encoding="utf-8",
            )

            with self.assertRaisesRegex(RELEASE_STAGE_MODULE.StageContractError, "api_version"):
                RELEASE_STAGE_MODULE.validate_stage(root, contract, "v4.0.0")


if __name__ == "__main__":
    unittest.main()
