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
SPEC = importlib.util.spec_from_file_location("machine_telemetry_build", PACKAGE_ROOT / "build.py")
if SPEC is None or SPEC.loader is None:
    raise RuntimeError("cannot load machine-telemetry build module")
BUILD = importlib.util.module_from_spec(SPEC)
sys.modules[SPEC.name] = BUILD
SPEC.loader.exec_module(BUILD)


class MachineTelemetryPackageTest(unittest.TestCase):
    def make_agent(self, root: Path) -> BUILD.AgentInput:
        path = root / "machine-telemetry-agent"
        path.write_bytes(b"#!/bin/sh\nprintf 'machine-telemetry-test\\n'\n")
        path.chmod(0o755)
        return BUILD.AgentInput(path=path, goos="linux", goarch="amd64")

    def test_build_is_byte_identical_and_verifiable(self) -> None:
        with tempfile.TemporaryDirectory() as temporary_root:
            root = Path(temporary_root)
            agent = self.make_agent(root)
            first = BUILD.build_package(agent, root / "first")
            second = BUILD.build_package(agent, root / "second")

            self.assertEqual(first, second)
            BUILD.verify_output_dir(root / "first")
            BUILD.verify_output_dir(root / "second")
            self.assertEqual(
                (root / "first" / BUILD.ARTIFACT_NAME).read_bytes(),
                (root / "second" / BUILD.ARTIFACT_NAME).read_bytes(),
            )

    def test_manifest_and_archive_bind_real_agent_and_webui(self) -> None:
        with tempfile.TemporaryDirectory() as temporary_root:
            root = Path(temporary_root)
            output = root / "dist"
            agent = self.make_agent(root)
            BUILD.build_package(agent, output)

            manifest_bytes = (output / BUILD.MANIFEST_NAME).read_bytes()
            manifest = json.loads(manifest_bytes)
            self.assertEqual(BUILD.canonical_manifest_json(manifest), manifest_bytes)
            self.assertEqual(list(BUILD.MANIFEST_FIELDS), list(manifest))
            self.assertEqual(["control", "agent"], manifest["targets"])
            self.assertEqual(["linux/amd64"], manifest["architectures"])
            self.assertEqual(
                {"agent-linux-amd64": "agent/linux-amd64/plugin"},
                manifest["entrypoints"],
            )
            self.assertEqual(
                manifest["frontend_sha256"],
                manifest["webui"]["bundle"]["sha256"],
            )
            self.assertEqual(
                BUILD.sha256_bytes((output / BUILD.ARTIFACT_NAME).read_bytes()),
                manifest["artifact_sha256"],
            )

            with tarfile.open(output / BUILD.ARTIFACT_NAME, mode="r:") as archive:
                members = archive.getmembers()
                names = [member.name for member in members]
                self.assertEqual(sorted(names), names)
                self.assertEqual(
                    {
                        "agent/linux-amd64/plugin",
                        "config.defaults.json",
                        "config.schema.json",
                        "package.json",
                        "webui/index.mjs",
                    },
                    set(names),
                )
                for member in members:
                    self.assertTrue(member.isfile())
                    self.assertEqual(0, member.mtime)
                    self.assertEqual(0, member.uid)
                    self.assertEqual(0, member.gid)
                    self.assertEqual("", member.uname)
                    self.assertEqual("", member.gname)
                agent_member = archive.getmember("agent/linux-amd64/plugin")
                self.assertEqual(0o755, agent_member.mode & 0o777)
                extracted = archive.extractfile(agent_member)
                self.assertIsNotNone(extracted)
                self.assertEqual(agent.path.read_bytes(), extracted.read())

    def test_webui_is_dependency_free(self) -> None:
        source = (PACKAGE_ROOT / BUILD.WEBUI_PATH).read_bytes()
        BUILD.validate_webui_source(source)
        text = source.decode("utf-8")
        self.assertNotRegex(text, BUILD.STATIC_IMPORT)
        self.assertNotRegex(text, BUILD.DYNAMIC_IMPORT)
        self.assertIn("webuiApiVersion: 'anixops.webui/v1'", text)
        self.assertIn("export default function create(host)", text)
        self.assertIn("request(STATUS_PATH", text)
        self.assertIn("const STATUS_PATH = '/api/v3/plugins/machine-telemetry/status?limit=200'", text)

    def test_tampered_artifact_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as temporary_root:
            root = Path(temporary_root)
            output = root / "dist"
            BUILD.build_package(self.make_agent(root), output)
            artifact_path = output / BUILD.ARTIFACT_NAME
            artifact = bytearray(artifact_path.read_bytes())
            artifact[len(artifact) // 2] ^= 0x01
            artifact_path.write_bytes(artifact)
            with self.assertRaisesRegex(BUILD.PackageError, "checksum mismatch"):
                BUILD.verify_output_dir(output)

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
