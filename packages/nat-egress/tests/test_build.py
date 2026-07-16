from __future__ import annotations

import importlib.util
import json
import os
import sys
import tarfile
import tempfile
import unittest
from pathlib import Path


PACKAGE_ROOT = Path(__file__).resolve().parents[1]
SPEC = importlib.util.spec_from_file_location("nat_egress_build", PACKAGE_ROOT / "build.py")
if SPEC is None or SPEC.loader is None:
    raise RuntimeError("cannot load nat-egress build module")
BUILD = importlib.util.module_from_spec(SPEC)
sys.modules[SPEC.name] = BUILD
SPEC.loader.exec_module(BUILD)


class NatEgressPackageTest(unittest.TestCase):
    def make_agent(self, root: Path) -> BUILD.AgentInput:
        path = root / "nat-egress-agent"
        path.write_bytes(b"#!/bin/sh\nprintf 'nat-egress-test\\n'\n")
        path.chmod(0o755)
        return BUILD.AgentInput(path=path, goos="linux", goarch="amd64")

    def test_build_is_byte_identical_and_verifiable(self) -> None:
        with tempfile.TemporaryDirectory() as temporary_root:
            root = Path(temporary_root)
            first = BUILD.build_package(self.make_agent(root), root / "first")
            second = BUILD.build_package(self.make_agent(root), root / "second")
            self.assertEqual(first, second)
            BUILD.verify_output_dir(root / "first")
            BUILD.verify_output_dir(root / "second")

    def test_manifest_and_archive_bind_agent_config_and_webui(self) -> None:
        with tempfile.TemporaryDirectory() as temporary_root:
            root = Path(temporary_root)
            output = root / "dist"
            agent = self.make_agent(root)
            BUILD.build_package(agent, output)
            manifest_bytes = (output / BUILD.MANIFEST_NAME).read_bytes()
            manifest = json.loads(manifest_bytes)
            self.assertEqual("nat-egress", manifest["id"])
            self.assertEqual(["control", "agent"], manifest["targets"])
            self.assertEqual(["linux/amd64"], manifest["architectures"])
            self.assertEqual(
                ["forward.nat.egress", "forward.route.policy", "forward.egress.health"],
                manifest["capabilities"],
            )
            self.assertEqual(
                ["default_mark", "egress_interface", "health_check_interval_seconds", "health_check_target", "ipv4_masquerade", "ipv6_masquerade", "policy_table"],
                manifest["config_schema"]["required"],
            )
            self.assertEqual("/api/v3/plugins/nat-egress/status", manifest["control_routes"][0])
            self.assertEqual("webui/index.mjs", manifest["webui"]["bundle"]["path"])
            with tarfile.open(output / BUILD.ARTIFACT_NAME, mode="r:") as archive:
                self.assertEqual(
                    {"agent/linux-amd64/plugin", "config.defaults.json", "config.schema.json", "package.json", "webui/index.mjs"},
                    {member.name for member in archive.getmembers()},
                )
                extracted = archive.extractfile("agent/linux-amd64/plugin")
                self.assertIsNotNone(extracted)
                self.assertEqual(agent.path.read_bytes(), extracted.read())

    def test_webui_is_dependency_free(self) -> None:
        source = (PACKAGE_ROOT / BUILD.WEBUI_PATH).read_bytes()
        BUILD.validate_webui_source(source)
        text = source.decode("utf-8")
        self.assertNotRegex(text, BUILD.STATIC_IMPORT)
        self.assertNotRegex(text, BUILD.DYNAMIC_IMPORT)
        self.assertIn("webuiApiVersion: 'anixops.webui/v1'", text)
        self.assertIn("const STATUS_PATH = '/api/v3/plugins/nat-egress/status?limit=200'", text)

    def test_non_executable_or_symlink_agent_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as temporary_root:
            root = Path(temporary_root)
            agent_path = root / "agent"
            agent_path.write_bytes(b"not executable")
            agent_path.chmod(0o644)
            with self.assertRaisesRegex(BUILD.PackageError, "executable"):
                BUILD.build_package(BUILD.AgentInput(agent_path, "linux", "amd64"), root / "dist")
            agent_path.chmod(0o755)
            symlink = root / "agent-link"
            try:
                os.symlink(agent_path, symlink)
            except (OSError, NotImplementedError):
                self.skipTest("symbolic links are unavailable")
            with self.assertRaisesRegex(BUILD.PackageError, "symbolic link"):
                BUILD.build_package(BUILD.AgentInput(symlink, "linux", "amd64"), root / "dist-link")


if __name__ == "__main__":
    unittest.main()
