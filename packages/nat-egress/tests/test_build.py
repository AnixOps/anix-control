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
                BUILD.EXPECTED_CAPABILITIES,
                manifest["capabilities"],
            )
            self.assertEqual(BUILD.EXPECTED_PERMISSIONS, manifest["permissions"])
            self.assertEqual(BUILD.EXPECTED_WEBUI_PERMISSIONS, manifest["webui"]["permissions"])
            self.assertEqual(
                list(BUILD.RUNTIME_CONFIG_FIELDS),
                manifest["config_schema"]["required"],
            )
            self.assertIs(False, manifest["config_schema"]["additionalProperties"])
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
                defaults_file = archive.extractfile(BUILD.DEFAULTS_PATH)
                schema_file = archive.extractfile(BUILD.SCHEMA_PATH)
                self.assertIsNotNone(defaults_file)
                self.assertIsNotNone(schema_file)
                self.assertEqual(BUILD.RUNTIME_DEFAULTS, json.load(defaults_file))
                self.assertEqual(manifest["config_schema"], json.load(schema_file))

    def test_config_schema_matches_agent_validation_boundaries(self) -> None:
        schema = json.loads((PACKAGE_ROOT / BUILD.SCHEMA_PATH).read_text(encoding="utf-8"))
        defaults = json.loads((PACKAGE_ROOT / BUILD.DEFAULTS_PATH).read_text(encoding="utf-8"))
        BUILD.validate_config_source(schema, defaults)

        self.assertEqual(BUILD.RUNTIME_DEFAULTS, defaults)
        self.assertEqual(list(BUILD.RUNTIME_CONFIG_FIELDS), schema["required"])
        self.assertEqual(set(BUILD.RUNTIME_CONFIG_FIELDS), set(schema["properties"]))
        self.assertEqual({"minimum": 1, "maximum": 65535}, {
            "minimum": schema["properties"]["default_mark"]["minimum"],
            "maximum": schema["properties"]["default_mark"]["maximum"],
        })
        self.assertEqual({"minimum": 1, "maximum": 252}, {
            "minimum": schema["properties"]["policy_table"]["minimum"],
            "maximum": schema["properties"]["policy_table"]["maximum"],
        })
        self.assertEqual({"minimum": 1, "maximum": 32765}, {
            "minimum": schema["properties"]["rule_priority"]["minimum"],
            "maximum": schema["properties"]["rule_priority"]["maximum"],
        })
        self.assertEqual({"minimum": 5, "maximum": 300}, {
            "minimum": schema["properties"]["health_check_interval_seconds"]["minimum"],
            "maximum": schema["properties"]["health_check_interval_seconds"]["maximum"],
        })
        self.assertEqual({"minimum": 1, "maximum": 30}, {
            "minimum": schema["properties"]["health_check_timeout_seconds"]["minimum"],
            "maximum": schema["properties"]["health_check_timeout_seconds"]["maximum"],
        })
        for name in ("table_name", "chain_name"):
            self.assertEqual(1, schema["properties"][name]["minLength"])
            self.assertEqual(48, schema["properties"][name]["maxLength"])
            self.assertEqual("^[a-z][a-z0-9_]*$", schema["properties"][name]["pattern"])
        self.assertEqual(15, schema["properties"]["egress_interface"]["maxLength"])
        self.assertEqual("^[A-Za-z0-9_.:@-]+$", schema["properties"]["egress_interface"]["pattern"])
        self.assertEqual(259, schema["properties"]["health_check_target"]["maxLength"])
        self.assertEqual(2, len(schema["anyOf"]))
        self.assertEqual(2, len(schema["allOf"]))
        self.assertIs(True, schema["properties"]["rollback_on_exit"]["const"])

        permissive = json.loads(json.dumps(schema))
        permissive["additionalProperties"] = True
        with self.assertRaisesRegex(BUILD.PackageError, "additional properties"):
            BUILD.validate_config_source(permissive, defaults)

        incomplete = dict(defaults)
        incomplete.pop("apply")
        with self.assertRaisesRegex(BUILD.PackageError, "Agent runtime contract"):
            BUILD.validate_config_source(schema, incomplete)

        unsafe_cleanup = json.loads(json.dumps(schema))
        unsafe_cleanup["properties"]["rollback_on_exit"].pop("const")
        with self.assertRaisesRegex(BUILD.PackageError, "fixed to true"):
            BUILD.validate_config_source(unsafe_cleanup, defaults)

    def test_webui_is_dependency_free(self) -> None:
        source = (PACKAGE_ROOT / BUILD.WEBUI_PATH).read_bytes()
        BUILD.validate_webui_source(source)
        text = source.decode("utf-8")
        self.assertNotRegex(text, BUILD.STATIC_IMPORT)
        self.assertNotRegex(text, BUILD.DYNAMIC_IMPORT)
        self.assertIn("webuiApiVersion: 'anixops.webui/v1'", text)
        self.assertIn("const STATUS_PATH = '/api/v3/plugins/nat-egress/status?limit=200'", text)
        self.assertIn("Signed Agent runtime status", text)
        self.assertIn("pinned Agent revision and namespace evidence", text)

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
