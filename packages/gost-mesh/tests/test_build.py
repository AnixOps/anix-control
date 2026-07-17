from __future__ import annotations

import contextlib
import hashlib
import importlib.util
import json
import os
import sys
import tarfile
import tempfile
import unittest
from pathlib import Path


PACKAGE_ROOT = Path(__file__).resolve().parents[1]
SPEC = importlib.util.spec_from_file_location("gost_mesh_build", PACKAGE_ROOT / "build.py")
if SPEC is None or SPEC.loader is None:
    raise RuntimeError("cannot load gost-mesh build module")
BUILD = importlib.util.module_from_spec(SPEC)
sys.modules[SPEC.name] = BUILD
SPEC.loader.exec_module(BUILD)


class GostMeshPackageTest(unittest.TestCase):
    def make_agent(self, root: Path) -> BUILD.AgentInput:
        path = root / "gost-mesh-agent"
        path.write_bytes(b"#!/bin/sh\nprintf 'gost-mesh 1.0.0\\n'\n")
        path.chmod(0o755)
        return BUILD.AgentInput(path=path, goos="linux", goarch="amd64")

    @contextlib.contextmanager
    def gost_fixture(self, root: Path, version: str = "3.2.6"):
        path = root / "gost"
        path.write_bytes(f"#!/bin/sh\nprintf 'gost v{version} (test runtime)\\n'\n".encode())
        path.chmod(0o755)
        gost = BUILD.GostInput(path=path, goos="linux", goarch="amd64")
        contract = BUILD.GOST_RUNTIME_CONTRACT[gost.architecture]
        original = contract["binary_sha256"]
        contract["binary_sha256"] = hashlib.sha256(path.read_bytes()).hexdigest()
        try:
            yield gost
        finally:
            contract["binary_sha256"] = original

    def test_build_is_byte_identical_and_verifiable(self) -> None:
        with tempfile.TemporaryDirectory() as temporary_root:
            root = Path(temporary_root)
            with self.gost_fixture(root) as gost:
                first = BUILD.build_package(self.make_agent(root), gost, root / "first")
                second = BUILD.build_package(self.make_agent(root), gost, root / "second")
                self.assertEqual(first, second)
                BUILD.verify_output_dir(root / "first")
                BUILD.verify_output_dir(root / "second")

    def test_manifest_archive_and_reports_bind_real_runtime_contract(self) -> None:
        with tempfile.TemporaryDirectory() as temporary_root:
            root = Path(temporary_root)
            agent = self.make_agent(root)
            with self.gost_fixture(root) as gost:
                output = root / "dist"
                BUILD.build_package(agent, gost, output)
                manifest = json.loads((output / BUILD.MANIFEST_NAME).read_bytes())
                report = json.loads((output / BUILD.REPORT_NAME).read_bytes())

                self.assertEqual("gost-mesh", manifest["id"])
                self.assertEqual(["control", "agent"], manifest["targets"])
                self.assertEqual(["linux/amd64"], manifest["architectures"])
                self.assertEqual(BUILD.EXPECTED_CAPABILITIES, manifest["capabilities"])
                self.assertEqual(BUILD.EXPECTED_PERMISSIONS, manifest["permissions"])
                self.assertEqual([], manifest["secret_fields"])
                self.assertEqual(BUILD.EXPECTED_WEBUI_PERMISSIONS, manifest["webui"]["permissions"])
                self.assertEqual({
                    "agent-linux-amd64": "agent/linux-amd64/plugin",
                    "runtime-gost-linux-amd64": "runtime/linux-amd64/gost",
                }, manifest["entrypoints"])
                self.assertEqual(list(BUILD.RUNTIME_CONFIG_FIELDS), manifest["config_schema"]["required"])
                self.assertIs(False, manifest["config_schema"]["additionalProperties"])
                self.assertNotIn("tuic", json.dumps(manifest).lower())

                with tarfile.open(output / BUILD.ARTIFACT_NAME, mode="r:") as archive:
                    self.assertEqual({
                        "agent/linux-amd64/plugin",
                        "config.defaults.json",
                        "config.schema.json",
                        "package.json",
                        "runtime/linux-amd64/gost",
                        "webui/index.mjs",
                    }, {member.name for member in archive.getmembers()})
                    self.assertEqual(agent.path.read_bytes(), archive.extractfile("agent/linux-amd64/plugin").read())
                    self.assertEqual(gost.path.read_bytes(), archive.extractfile("runtime/linux-amd64/gost").read())
                    package_index = json.load(archive.extractfile("package.json"))
                    self.assertEqual(BUILD.GOST_VERSION, package_index["runtime"]["gost"]["version"])
                    self.assertEqual(
                        BUILD.GOST_RUNTIME_CONTRACT[gost.architecture]["binary_sha256"],
                        package_index["runtime"]["gost"]["sha256"],
                    )
                    self.assertEqual(BUILD.RUNTIME_DEFAULTS, json.load(archive.extractfile(BUILD.DEFAULTS_PATH)))
                    self.assertEqual(manifest["config_schema"], json.load(archive.extractfile(BUILD.SCHEMA_PATH)))
                self.assertEqual(BUILD.GOST_VERSION, report["runtime"]["gost"]["version"])
                self.assertEqual("runtime-gost-linux-amd64", report["runtime"]["gost"]["entrypoint"])

    def test_config_schema_matches_aggregated_agent_contract(self) -> None:
        schema = json.loads((PACKAGE_ROOT / BUILD.SCHEMA_PATH).read_text(encoding="utf-8"))
        defaults = json.loads((PACKAGE_ROOT / BUILD.DEFAULTS_PATH).read_text(encoding="utf-8"))
        BUILD.validate_config_source(schema, defaults)
        self.assertEqual(BUILD.RUNTIME_DEFAULTS, defaults)
        self.assertEqual(["entry", "exit"], schema["$defs"]["tunnel"]["properties"]["role"]["enum"])
        self.assertEqual(["quic", "wss"], schema["$defs"]["tunnel"]["properties"]["transport"]["enum"])
        self.assertEqual(128, schema["properties"]["tunnels"]["maxItems"])
        self.assertIs(True, schema["properties"]["tunnels"]["uniqueItems"])
        self.assertEqual(9000, schema["$defs"]["tun"]["properties"]["mtu"]["maximum"])
        self.assertEqual(252, schema["$defs"]["routing"]["properties"]["table"]["maximum"])
        self.assertEqual(
            ["source_cidrs", "route_cidrs"],
            schema["$defs"]["routing"]["required"],
        )
        role_condition = schema["$defs"]["tunnel"]["allOf"][0]
        self.assertEqual(
            ["table", "priority"],
            role_condition["then"]["properties"]["routing"]["required"],
        )
        self.assertEqual(
            0,
            role_condition["else"]["properties"]["routing"]["properties"]["table"]["const"],
        )
        self.assertEqual(
            0,
            role_condition["else"]["properties"]["routing"]["properties"]["priority"]["const"],
        )
        self.assertEqual(4096, schema["$defs"]["tls"]["properties"]["ca_file"]["maxLength"])
        self.assertEqual(1, schema["allOf"][0]["then"]["properties"]["tunnels"]["minItems"])

        permissive = json.loads(json.dumps(schema))
        permissive["additionalProperties"] = True
        with self.assertRaisesRegex(BUILD.PackageError, "additional properties"):
            BUILD.validate_config_source(permissive, defaults)

        unsafe_cleanup = json.loads(json.dumps(schema))
        unsafe_cleanup["properties"]["rollback_on_exit"].pop("const")
        with self.assertRaisesRegex(BUILD.PackageError, "fixed to true"):
            BUILD.validate_config_source(unsafe_cleanup, defaults)

        false_transport = json.loads(json.dumps(schema))
        false_transport["$defs"]["tunnel"]["properties"]["transport"]["enum"].append("tuic")
        with self.assertRaisesRegex(BUILD.PackageError, "QUIC and WSS"):
            BUILD.validate_config_source(false_transport, defaults)

        duplicate_tunnels = json.loads(json.dumps(schema))
        duplicate_tunnels["properties"]["tunnels"].pop("uniqueItems")
        with self.assertRaisesRegex(BUILD.PackageError, "tunnels schema limits"):
            BUILD.validate_config_source(duplicate_tunnels, defaults)

        exit_allocates_policy_route = json.loads(json.dumps(schema))
        exit_allocates_policy_route["$defs"]["tunnel"]["allOf"][0]["else"]["properties"]["routing"]["properties"]["table"] = {"minimum": 1}
        with self.assertRaisesRegex(BUILD.PackageError, "omitted or 0"):
            BUILD.validate_config_source(exit_allocates_policy_route, defaults)

    def test_gost_digest_version_architecture_and_mode_are_enforced(self) -> None:
        with tempfile.TemporaryDirectory() as temporary_root:
            root = Path(temporary_root)
            with self.gost_fixture(root, version="9.9.9") as gost:
                with self.assertRaisesRegex(BUILD.PackageError, "version must be v3.2.6"):
                    BUILD.validate_gost_input(gost)

            gost_path = root / "wrong-gost"
            gost_path.write_bytes(b"#!/bin/sh\nprintf 'gost v3.2.6\\n'\n")
            gost_path.chmod(0o755)
            gost = BUILD.GostInput(gost_path, "linux", "amd64")
            with self.assertRaisesRegex(BUILD.PackageError, "SHA-256 mismatch"):
                BUILD.validate_gost_input(gost)

            gost_path.chmod(0o644)
            BUILD.GOST_RUNTIME_CONTRACT[gost.architecture]["binary_sha256"] = hashlib.sha256(gost_path.read_bytes()).hexdigest()
            try:
                with self.assertRaisesRegex(BUILD.PackageError, "executable mode"):
                    BUILD.validate_gost_input(gost)
            finally:
                BUILD.GOST_RUNTIME_CONTRACT[gost.architecture]["binary_sha256"] = (
                    "a2aea24efb4597b5f57b35b8e1bbcc59f439b80723854d4371f6828b46682ffb"
                )

            with self.assertRaisesRegex(BUILD.PackageError, "architectures must match"):
                BUILD.build_package(self.make_agent(root), BUILD.GostInput(gost_path, "linux", "arm64"), root / "bad")

    def test_webui_is_dependency_free_and_does_not_claim_tuic(self) -> None:
        source = (PACKAGE_ROOT / BUILD.WEBUI_PATH).read_bytes()
        BUILD.validate_webui_source(source)
        text = source.decode("utf-8")
        self.assertNotRegex(text, BUILD.STATIC_IMPORT)
        self.assertNotRegex(text, BUILD.DYNAMIC_IMPORT)
        self.assertIn("webuiApiVersion: 'anixops.webui/v1'", text)
        self.assertIn("const STATUS_PATH = '/api/v3/plugins/gost-mesh/status?limit=200'", text)
        self.assertNotIn("TUIC", text)

    def test_non_executable_or_symlink_agent_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as temporary_root:
            root = Path(temporary_root)
            agent_path = root / "agent"
            agent_path.write_bytes(b"not executable")
            agent_path.chmod(0o644)
            with self.gost_fixture(root) as gost:
                with self.assertRaisesRegex(BUILD.PackageError, "executable"):
                    BUILD.build_package(BUILD.AgentInput(agent_path, "linux", "amd64"), gost, root / "dist")
                agent_path.chmod(0o755)
                symlink = root / "agent-link"
                try:
                    os.symlink(agent_path, symlink)
                except (OSError, NotImplementedError):
                    self.skipTest("symbolic links are unavailable")
                with self.assertRaisesRegex(BUILD.PackageError, "symbolic link"):
                    BUILD.build_package(BUILD.AgentInput(symlink, "linux", "amd64"), gost, root / "dist-link")


if __name__ == "__main__":
    unittest.main()
