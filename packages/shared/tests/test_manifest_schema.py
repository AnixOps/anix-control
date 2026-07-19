from __future__ import annotations

import base64
import hashlib
import importlib.util
import io
import json
import os
import subprocess
import sys
import tarfile
import tempfile
import unittest
from unittest import mock
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parents[3]
BUILDER = REPO_ROOT / "packages" / "shared" / "build_package.py"
RELEASE_STAGE = REPO_ROOT / "config" / "scripts" / "check_release_stage.py"
BUILD_PACKAGE_SPEC = importlib.util.spec_from_file_location("shared_build_package", BUILDER)
if BUILD_PACKAGE_SPEC is None or BUILD_PACKAGE_SPEC.loader is None:
    raise RuntimeError("cannot load shared package builder")
BUILD_PACKAGE_MODULE = importlib.util.module_from_spec(BUILD_PACKAGE_SPEC)
sys.modules[BUILD_PACKAGE_SPEC.name] = BUILD_PACKAGE_MODULE
BUILD_PACKAGE_SPEC.loader.exec_module(BUILD_PACKAGE_MODULE)
RELEASE_STAGE_SPEC = importlib.util.spec_from_file_location("release_stage", RELEASE_STAGE)
if RELEASE_STAGE_SPEC is None or RELEASE_STAGE_SPEC.loader is None:
    raise RuntimeError("cannot load release-stage checker")
RELEASE_STAGE_MODULE = importlib.util.module_from_spec(RELEASE_STAGE_SPEC)
sys.modules[RELEASE_STAGE_SPEC.name] = RELEASE_STAGE_MODULE
RELEASE_STAGE_SPEC.loader.exec_module(RELEASE_STAGE_MODULE)


ED25519_SPKI_PREFIX = bytes.fromhex("302a300506032b6570032100")


def run_command(arguments: list[str], *, cwd: Path = REPO_ROOT, input_bytes: bytes | None = None) -> subprocess.CompletedProcess[bytes]:
    return subprocess.run(arguments, cwd=cwd, input=input_bytes, check=False, capture_output=True)


def generate_ed25519_signer(root: Path, name: str) -> tuple[Path, Path]:
    private_key = root / f"{name}.private.pem"
    generated = run_command(["openssl", "genpkey", "-algorithm", "ED25519", "-out", str(private_key)])
    if generated.returncode != 0:
        raise AssertionError(generated.stderr.decode("utf-8", errors="replace"))
    os.chmod(private_key, 0o600)

    derived = run_command(["openssl", "pkey", "-in", str(private_key), "-pubout", "-outform", "DER"])
    if derived.returncode != 0 or not derived.stdout.startswith(ED25519_SPKI_PREFIX):
        raise AssertionError(derived.stderr.decode("utf-8", errors="replace"))
    raw_public_key = derived.stdout[len(ED25519_SPKI_PREFIX) :]
    if len(raw_public_key) != 32:
        raise AssertionError("generated Ed25519 public key has an invalid length")
    official_root = root / f"{name}.official-public-key.raw"
    official_root.write_bytes(base64.b64encode(raw_public_key) + b"\n")
    return private_key, official_root


def generate_rsa_signer(root: Path) -> Path:
    private_key = root / "rsa.private.pem"
    generated = run_command(
        ["openssl", "genpkey", "-algorithm", "RSA", "-pkeyopt", "rsa_keygen_bits:2048", "-out", str(private_key)]
    )
    if generated.returncode != 0:
        raise AssertionError(generated.stderr.decode("utf-8", errors="replace"))
    os.chmod(private_key, 0o600)
    return private_key


def agent_binary_arguments(
    root: Path,
    platforms: tuple[str, ...] = ("linux/amd64",),
    package_ids: tuple[str, ...] = ("machine-telemetry", "nftables-forward", "gost-mesh", "nat-egress"),
) -> list[str]:
    arguments: list[str] = []
    for package_id in package_ids:
        for platform in platforms:
            binary = root / f"{package_id}-{platform.replace('/', '-')}.bin"
            binary.write_bytes(b"#!/bin/sh\nexit 0\n")
            os.chmod(binary, 0o755)
            arguments.extend(["--agent-binary", f"{package_id}@{platform}={binary}"])
    return arguments


def runtime_binary_arguments(root: Path, platforms: tuple[str, ...] = ("linux/amd64",)) -> list[str]:
    arguments: list[str] = []
    for platform in platforms:
        binary = root / f"gost-runtime-{platform.replace('/', '-')}.bin"
        binary.write_bytes(b"#!/bin/sh\nprintf 'gost v3.2.6 test runtime\\n'\n")
        os.chmod(binary, 0o755)
        arguments.extend(["--runtime-binary", f"gost-mesh:gost@{platform}={binary}"])
    return arguments


class V4PackageManifestSchemaTest(unittest.TestCase):
    def test_canonical_json_matches_go_encoding_for_utf8_and_special_runes(self) -> None:
        value = {
            "ascii": "plain",
            "control": "line\n\t\u0000",
            "html": "<&>",
            "separators": "before\u2028middle\u2029after",
            "utf8": "caf\u00e9 你好",
        }
        go_program = """package main

import (
    "encoding/json"
    "os"
)

func main() {
    var value any
    if err := json.NewDecoder(os.Stdin).Decode(&value); err != nil {
        panic(err)
    }
    encoded, err := json.Marshal(value)
    if err != nil {
        panic(err)
    }
    _, _ = os.Stdout.Write(encoded)
}
"""
        with tempfile.TemporaryDirectory() as temporary:
            source = Path(temporary) / "canonical.go"
            source.write_text(go_program, encoding="utf-8")
            payload = json.dumps(value, ensure_ascii=False, separators=(",", ":")).encode("utf-8")
            result = run_command(["go", "run", str(source)], input_bytes=payload)

        self.assertEqual(0, result.returncode, result.stderr.decode("utf-8", errors="replace"))
        self.assertIn("caf\u00e9 你好".encode("utf-8"), result.stdout)
        self.assertIn(b"\\u003c\\u0026\\u003e", result.stdout)
        self.assertIn(b"\\u2028", result.stdout)
        self.assertIn(b"\\u2029", result.stdout)
        self.assertEqual(result.stdout, BUILD_PACKAGE_MODULE.canonical_json(value))

    def test_unsafe_version_is_rejected_before_writing_any_artifact(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            output = root / "selected-output"
            result = run_command(
                [
                    sys.executable,
                    str(BUILDER),
                    "--package",
                    "subscription",
                    "--version",
                    "x/../../escaped",
                    "--out",
                    str(output),
                ]
            )

            self.assertNotEqual(0, result.returncode)
            self.assertIn(b"version must be a safe path segment", result.stderr)
            self.assertFalse(any(root.glob("escaped.*")), "unsafe version wrote outside the selected output directory")
            self.assertFalse(output.exists(), "unsafe version created selected output files")

        for version in ("../escape", r"4.0.0\\escape", "4.0.0 ", ".", ".."):
            with self.subTest(version=version), tempfile.TemporaryDirectory() as temporary:
                result = run_command(
                    [
                        sys.executable,
                        str(BUILDER),
                        "--package",
                        "subscription",
                        "--version",
                        version,
                        "--out",
                        str(Path(temporary) / "out"),
                    ]
                )
                self.assertNotEqual(0, result.returncode)
                self.assertIn(b"version must be a safe path segment", result.stderr)

    def test_formal_release_requires_matching_ed25519_root_and_verifies_artifacts(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            signing_key, official_root = generate_ed25519_signer(root, "official")
            _, wrong_root = generate_ed25519_signer(root, "wrong")
            rsa_key = generate_rsa_signer(root)
            malformed_root = root / "malformed.raw"
            malformed_root.write_bytes(base64.b64encode(b"not-an-ed25519-public-key") + b"\n")
            output = root / "packages"
            platforms = ("linux/amd64", "linux/arm64")
            command = [
                sys.executable,
                str(BUILDER),
                "--package",
                "identity-platform",
                "--version",
                "4.0.0",
                "--out",
                str(output),
                "--formal-release",
            ]

            cases = (
                ("missing signing key", command + ["--official-public-key", str(official_root)], b"formal release requires --signing-key"),
                ("missing official root", command + ["--signing-key", str(signing_key)], b"formal release requires --official-public-key"),
                (
                    "wrong official root",
                    command + ["--signing-key", str(signing_key), "--official-public-key", str(wrong_root)],
                    b"signing key does not match official public key",
                ),
                (
                    "non Ed25519 signing key",
                    command + ["--signing-key", str(rsa_key), "--official-public-key", str(official_root)],
                    b"signing key must be Ed25519",
                ),
                (
                    "malformed raw root",
                    command + ["--signing-key", str(signing_key), "--official-public-key", str(malformed_root)],
                    b"official public key must contain 32 raw Ed25519 bytes",
                ),
            )
            for name, arguments, expected_error in cases:
                with self.subTest(name=name):
                    result = run_command(arguments)
                    self.assertNotEqual(0, result.returncode)
                    self.assertIn(expected_error, result.stderr)

            built = run_command(
                command
                + ["--platform", platforms[0], "--platform", platforms[1]]
                + ["--signing-key", str(signing_key), "--official-public-key", str(official_root)]
            )
            self.assertEqual(0, built.returncode, built.stderr.decode("utf-8", errors="replace"))
            self.assertEqual(1, len(list(output.glob("*.anxp"))))
            self.assertEqual(1, len(list(output.glob("*.sbom.spdx.json"))))
            self.assertTrue((output / "identity-platform-4.0.0.anxp").is_file())

            verified = run_command(
                [
                    sys.executable,
                    str(BUILDER),
                    "--package",
                    "identity-platform",
                    "--version",
                    "4.0.0",
                    "--out",
                    str(output),
                    "--verify-release",
                    "--official-public-key",
                    str(official_root),
                ]
            )
            self.assertEqual(0, verified.returncode, verified.stderr.decode("utf-8", errors="replace"))

            rejected = run_command(
                [
                    sys.executable,
                    str(BUILDER),
                    "--package",
                    "identity-platform",
                    "--version",
                    "4.0.0",
                    "--out",
                    str(output),
                    "--verify-release",
                    "--official-public-key",
                    str(wrong_root),
                ]
            )
            self.assertNotEqual(0, rejected.returncode)
            self.assertIn(b"verify manifest signature", rejected.stderr)

    def test_v4_package_matrix_declares_agent_targets_only_for_real_agent_commands(self) -> None:
        self.assertEqual(
            [
                ("identity-platform", ("control",)),
                ("subscription", ("control",)),
                ("proxy-node", ("control",)),
                ("plan", ("control",)),
                ("order", ("control",)),
                ("payment", ("control",)),
                ("forward", ("control",)),
                ("ticket", ("control",)),
                ("notification", ("control",)),
                ("knowledge", ("control",)),
                ("machine-telemetry", ("control", "agent")),
                ("nftables-forward", ("control", "agent")),
                ("gost-mesh", ("control", "agent")),
                ("nat-egress", ("control", "agent")),
                ("wireguard", ("control",)),
                ("protocol-runtime", ("control",)),
            ],
            [(spec.package_id, spec.targets) for spec in BUILD_PACKAGE_MODULE.PACKAGE_SPECS],
        )

    def test_formal_agent_package_requires_explicit_agent_binary_for_each_platform(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            signing_key, official_root = generate_ed25519_signer(root, "official")
            result = run_command(
                [
                    sys.executable,
                    str(BUILDER),
                    "--package",
                    "machine-telemetry",
                    "--version",
                    "4.0.0",
                    "--out",
                    str(root / "packages"),
                    "--formal-release",
                    "--signing-key",
                    str(signing_key),
                    "--official-public-key",
                    str(official_root),
                    "--platform",
                    "linux/amd64",
                    "--platform",
                    "linux/arm64",
                ]
            )
            self.assertNotEqual(0, result.returncode)
            self.assertIn(b"agent binary", result.stderr.lower())

    def test_formal_release_requires_both_supported_platforms(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            signing_key, official_root = generate_ed25519_signer(root, "official")
            result = run_command(
                [
                    sys.executable,
                    str(BUILDER),
                    "--package",
                    "subscription",
                    "--version",
                    "4.0.0",
                    "--out",
                    str(root / "packages"),
                    "--formal-release",
                    "--signing-key",
                    str(signing_key),
                    "--official-public-key",
                    str(official_root),
                    "--platform",
                    "linux/amd64",
                ]
            )
            self.assertNotEqual(0, result.returncode)
            self.assertIn(b"formal release requires linux/amd64 and linux/arm64", result.stderr)

    def test_formal_multiarch_package_indexes_real_control_and_agent_entries(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            signing_key, official_root = generate_ed25519_signer(root, "official")
            output = root / "packages"
            platforms = ("linux/amd64", "linux/arm64")
            result = run_command(
                [
                    sys.executable,
                    str(BUILDER),
                    "--package",
                    "machine-telemetry",
                    "--version",
                    "4.0.0",
                    "--out",
                    str(output),
                    "--formal-release",
                    "--signing-key",
                    str(signing_key),
                    "--official-public-key",
                    str(official_root),
                    "--platform",
                    platforms[0],
                    "--platform",
                    platforms[1],
                    *agent_binary_arguments(root, platforms, ("machine-telemetry",)),
                ]
            )
            self.assertEqual(0, result.returncode, result.stderr.decode("utf-8", errors="replace"))

            manifest = json.loads((output / "machine-telemetry-4.0.0.manifest.json").read_text(encoding="utf-8"))
            self.assertEqual(list(platforms), manifest["architectures"])
            self.assertEqual(["/api/v3/plugins/machine-telemetry/status"], manifest["control_routes"])
            self.assertEqual("bin/control-entrypoints.json", manifest["control_entrypoint"]["path"])
            self.assertEqual("agent/entrypoints.json", manifest["agent_entrypoint"]["path"])

            with tarfile.open(output / "machine-telemetry-4.0.0.anxp", mode="r:") as archive:
                control_index_file = archive.extractfile("bin/control-entrypoints.json")
                agent_index_file = archive.extractfile("agent/entrypoints.json")
                self.assertIsNotNone(control_index_file)
                self.assertIsNotNone(agent_index_file)
                control_index = json.loads(control_index_file.read() if control_index_file else b"{}")
                agent_index = json.loads(agent_index_file.read() if agent_index_file else b"{}")
                for index in (control_index, agent_index):
                    self.assertEqual("anixops.package-entrypoints/v1", index["format"])
                    self.assertEqual(list(platforms), [entry["architecture"] for entry in index["entries"]])
                    for entry in index["entries"]:
                        selected = archive.extractfile(entry["path"])
                        self.assertIsNotNone(selected)
                        self.assertEqual(entry["sha256"], hashlib.sha256(selected.read() if selected else b"").hexdigest())

    def test_formal_gost_mesh_package_requires_pinned_runtime_inputs(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            signing_key, official_root = generate_ed25519_signer(root, "official")
            platforms = ("linux/amd64", "linux/arm64")
            result = run_command(
                [
                    sys.executable,
                    str(BUILDER),
                    "--package",
                    "gost-mesh",
                    "--version",
                    "4.0.0",
                    "--out",
                    str(root / "packages"),
                    "--formal-release",
                    "--signing-key",
                    str(signing_key),
                    "--official-public-key",
                    str(official_root),
                    "--platform",
                    platforms[0],
                    "--platform",
                    platforms[1],
                    *agent_binary_arguments(root, platforms, ("gost-mesh",)),
                ]
            )
            self.assertNotEqual(0, result.returncode)
            self.assertIn(b"runtime binary", result.stderr.lower())

    def test_gost_mesh_runtime_payloads_are_pinned_and_packaged(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            spec = next(item for item in BUILD_PACKAGE_MODULE.PACKAGE_SPECS if item.package_id == "gost-mesh")
            platforms = (("linux", "amd64"), ("linux", "arm64"))
            agent_inputs = {}
            runtime_inputs = {}
            runtime_payloads = {}
            for platform in platforms:
                suffix = "-".join(platform)
                agent = root / f"gost-mesh-agent-{suffix}"
                runtime_binary = root / f"gost-{suffix}"
                agent.write_bytes(b"#!/bin/sh\nexit 0\n")
                runtime_binary.write_bytes(("#!/bin/sh\nprintf 'gost v3.2.6 " + suffix + "\\n'\n").encode())
                agent.chmod(0o755)
                runtime_binary.chmod(0o755)
                agent_inputs[("gost-mesh", platform)] = agent
                runtime_inputs[("gost-mesh", "gost", platform)] = runtime_binary
                runtime_payloads[platform] = runtime_binary.read_bytes()

            def runtime_contract(package_id: str, runtime_name: str, platform: tuple[str, str]) -> dict[str, str]:
                self.assertEqual("gost-mesh", package_id)
                self.assertEqual("gost", runtime_name)
                return {"binary_sha256": hashlib.sha256(runtime_payloads[platform]).hexdigest()}

            with mock.patch.object(
                BUILD_PACKAGE_MODULE,
                "runtime_binary_contract",
                side_effect=runtime_contract,
                create=True,
            ):
                entries, contents = BUILD_PACKAGE_MODULE.package_entries(
                    spec,
                    "4.0.0",
                    platforms,
                    agent_inputs,
                    runtime_inputs,
                )
            artifact = BUILD_PACKAGE_MODULE.deterministic_tar(entries)
            manifest = BUILD_PACKAGE_MODULE.generated_manifest(spec, "4.0.0", platforms, artifact, contents)
            self.assertEqual(
                {
                    "runtime-gost-linux-amd64": "runtime/linux-amd64/gost",
                    "runtime-gost-linux-arm64": "runtime/linux-arm64/gost",
                },
                manifest["entrypoints"],
            )
            paths = {entry.path: entry.data for entry in entries}
            for platform in platforms:
                path = f"runtime/{'-'.join(platform)}/gost"
                self.assertEqual(runtime_payloads[platform], paths[path])

            mismatched_contract = {"binary_sha256": "0" * 64}
            with mock.patch.object(
                BUILD_PACKAGE_MODULE,
                "runtime_binary_contract",
                return_value=mismatched_contract,
                create=True,
            ):
                with self.assertRaisesRegex(BUILD_PACKAGE_MODULE.PackageBuildError, "runtime binary SHA-256 mismatch"):
                    BUILD_PACKAGE_MODULE.source_runtime_binary(
                        "gost-mesh",
                        "gost",
                        platforms[0],
                        runtime_inputs,
                    )

    def test_shared_gost_runtime_contract_matches_formal_release_workflow(self) -> None:
        contract = json.loads((REPO_ROOT / "packages" / "gost-mesh" / "runtime-contract.json").read_text(encoding="utf-8"))
        workflow = (REPO_ROOT / ".github" / "workflows" / "ci.yml").read_text(encoding="utf-8")
        self.assertEqual("anixops.runtime-contract/v1", contract["format"])
        self.assertEqual("gost", contract["runtime"])
        self.assertEqual("3.2.6", contract["version"])
        for goarch, workflow_prefix in (("amd64", "GOST_LINUX_AMD64"), ("arm64", "GOST_LINUX_ARM64")):
            entry = contract["platforms"]["linux/" + goarch]
            self.assertIn(f"{workflow_prefix}_ARCHIVE_SHA256: '{entry['archive_sha256']}'", workflow)
            self.assertIn(f"{workflow_prefix}_BINARY_SHA256: '{entry['binary_sha256']}'", workflow)

    def test_artifact_uses_deterministic_gzip_only_when_raw_tar_exceeds_limit(self) -> None:
        entries = [BUILD_PACKAGE_MODULE.ArchiveEntry("runtime/linux-amd64/gost", b"x" * 4096, 0o755)]
        with mock.patch.object(BUILD_PACKAGE_MODULE, "MAX_ARTIFACT_BYTES", 1024):
            first = BUILD_PACKAGE_MODULE.deterministic_artifact(entries)
            second = BUILD_PACKAGE_MODULE.deterministic_artifact(entries)
        self.assertEqual(first, second)
        self.assertTrue(first.startswith(b"\x1f\x8b"))
        with tarfile.open(fileobj=io.BytesIO(first), mode="r:gz") as archive:
            member = archive.extractfile("runtime/linux-amd64/gost")
            self.assertIsNotNone(member)
            self.assertEqual(b"x" * 4096, member.read() if member else b"")

    def test_v4_package_builder_materializes_each_webui_release_version(self) -> None:
        for spec in BUILD_PACKAGE_MODULE.PACKAGE_SPECS:
            with self.subTest(package_id=spec.package_id):
                bundle = BUILD_PACKAGE_MODULE.source_webui(spec.package_id, "4.0.0")
                self.assertNotIn(b"__ANIXOPS_PACKAGE_VERSION__", bundle)
                self.assertIn(b"4.0.0", bundle)

    def test_identity_platform_source_contract_is_a_control_v2_package(self) -> None:
        package_root = REPO_ROOT / "packages" / "identity-platform"
        manifest_path = package_root / "manifest.template.json"
        migrations_path = package_root / "migrations" / "index.json"
        routes_path = package_root / "compat" / "v2-routes.json"
        webui_path = package_root / "webui" / "index.mjs"
        self.assertTrue(manifest_path.is_file())
        self.assertTrue(migrations_path.is_file())
        self.assertTrue(routes_path.is_file())
        self.assertTrue(webui_path.is_file())

        manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
        migrations = json.loads(migrations_path.read_text(encoding="utf-8"))
        routes = json.loads(routes_path.read_text(encoding="utf-8"))

        self.assertEqual("identity-platform", manifest["id"])
        self.assertEqual("v2", manifest["api_version"])
        self.assertEqual(["control"], manifest["targets"])
        self.assertEqual("bin/control-host", manifest["control_entrypoint"]["path"])
        self.assertEqual("migrations/index.json", manifest["migrations"]["index"])
        self.assertEqual("compat/v2-routes.json", manifest["compatibility_routes"]["path"])
        self.assertEqual("webui/index.mjs", manifest["webui"]["bundle"]["path"])
        self.assertTrue(migrations)
        self.assertEqual("identity-platform", migrations["package_id"])
        self.assertTrue(routes)
        self.assertEqual("identity-platform", routes["package_id"])
        self.assertTrue(
            {"identity.auth.login", "identity.auth.register"}.issubset(
                {route["package_route"] for route in routes["routes"]}
            )
        )
        self.assertTrue(webui_path.read_bytes())

    def test_identity_platform_artifact_contains_its_executable_host_and_source_contract(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            output = Path(temporary) / "packages"
            result = run_command(
                [
                    sys.executable,
                    str(BUILDER),
                    "--package",
                    "identity-platform",
                    "--version",
                    "4.0.0",
                    "--out",
                    str(output),
                ]
            )
            self.assertEqual(0, result.returncode, result.stderr.decode("utf-8", errors="replace"))

            artifact = output / "identity-platform-4.0.0.anxp"
            with tarfile.open(artifact, mode="r:") as archive:
                index_file = archive.extractfile("bin/control-entrypoints.json")
                routes = archive.extractfile("compat/v2-routes.json")
                migrations = archive.extractfile("migrations/index.json")
                self.assertIsNotNone(index_file)
                self.assertIsNotNone(routes)
                self.assertIsNotNone(migrations)
                index = json.loads(index_file.read() if index_file else b"{}")
                self.assertEqual("anixops.package-entrypoints/v1", index["format"])
                self.assertEqual(["linux/amd64"], [entry["architecture"] for entry in index["entries"]])
                host = archive.extractfile(index["entries"][0]["path"])
                self.assertIsNotNone(host)
                host_bytes = host.read() if host else b""
                routes_value = json.loads(routes.read() if routes else b"{}")
                migrations_value = json.loads(migrations.read() if migrations else b"{}")

                self.assertTrue(host_bytes.startswith(b"\x7fELF"))
                self.assertIn(b"control host socket is required", host_bytes)
                self.assertIn("identity.auth.login", {route["package_route"] for route in routes_value["routes"]})
                self.assertIn("identity.auth.register", {route["package_route"] for route in routes_value["routes"]})
                migration_paths = [migration["path"] for migration in migrations_value["migrations"]]
                self.assertEqual(["migrations/001_identity_platform.sql"], migration_paths)
                self.assertIsNotNone(archive.extractfile("migrations/001_identity_platform.sql"))

    def test_fallback_control_package_uses_a_compiled_capability_host(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            output = Path(temporary) / "packages"
            result = run_command(
                [
                    sys.executable,
                    str(BUILDER),
                    "--package",
                    "knowledge",
                    "--version",
                    "4.0.0",
                    "--out",
                    str(output),
                ]
            )
            self.assertEqual(0, result.returncode, result.stderr.decode("utf-8", errors="replace"))

            artifact = output / "knowledge-4.0.0.anxp"
            with tarfile.open(artifact, mode="r:") as archive:
                index_file = archive.extractfile("bin/control-entrypoints.json")
                self.assertIsNotNone(index_file)
                index = json.loads(index_file.read() if index_file else b"{}")
                host = archive.extractfile(index["entries"][0]["path"])
                self.assertIsNotNone(host)
                host_bytes = host.read() if host else b""

            self.assertTrue(host_bytes.startswith(b"\x7fELF"))
            self.assertIn(b"package bridge capability is required", host_bytes)
            self.assertNotIn(b"host is not implemented in the manifest stage", host_bytes)

    def test_identity_platform_routes_match_the_v2_ownership_catalog(self) -> None:
        catalog = json.loads((REPO_ROOT / "config" / "v2-package-route-catalog.json").read_text(encoding="utf-8"))
        declaration = json.loads((REPO_ROOT / "packages" / "identity-platform" / "compat" / "v2-routes.json").read_text(encoding="utf-8"))
        expected = {
            (route["method"], route["path"], route["route_id"], route["envelope"], route["transport"])
            for route in catalog
            if route["owner"] == "identity-platform"
        }
        actual = {
            (route["method"], route["legacy_path"], route["package_route"], route["envelope"], route.get("transport", "http"))
            for route in declaration["routes"]
        }
        self.assertEqual(expected, actual)

    def test_package_cutover_wave_routes_match_the_v2_ownership_catalog(self) -> None:
        catalog = json.loads((REPO_ROOT / "config" / "v2-package-route-catalog.json").read_text(encoding="utf-8"))
        for package_id in (
            "knowledge",
            "ticket",
            "plan",
            "notification",
            "order",
            "payment",
            "subscription",
            "machine-telemetry",
            "proxy-node",
            "protocol-runtime",
            "wireguard",
            "forward",
            "gost-mesh",
            "nftables-forward",
            "nat-egress",
        ):
            declaration = json.loads((REPO_ROOT / "packages" / package_id / "compat" / "v2-routes.json").read_text(encoding="utf-8"))
            migrations = json.loads((REPO_ROOT / "packages" / package_id / "migrations" / "index.json").read_text(encoding="utf-8"))
            expected = {
                (route["method"], route["path"], route["route_id"], route["envelope"], route["transport"])
                for route in catalog
                if route["owner"] == package_id
            }
            actual = {
                (route["method"], route["legacy_path"], route["package_route"], route["envelope"], route.get("transport", "http"))
                for route in declaration["routes"]
            }
            self.assertEqual(expected, actual, package_id)
            self.assertEqual(package_id, migrations["package_id"])
            self.assertEqual("4.0.0", migrations["version"])

    def test_all_v4_packages_materialize_signed_v2_artifacts(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            output = root / "packages"
            result = subprocess.run(
                [
                    sys.executable,
                    str(BUILDER),
                    "--all",
                    "--version",
                    "4.0.0",
                    "--out",
                    str(output),
                    *agent_binary_arguments(root),
                    *runtime_binary_arguments(root),
                ],
                cwd=REPO_ROOT,
                check=False,
                text=True,
                capture_output=True,
            )
            self.assertEqual(0, result.returncode, result.stderr or result.stdout)

            artifacts = sorted(output.glob("*.anxp"))
            sboms = sorted(output.glob("*.sbom.spdx.json"))
            self.assertEqual(16, len(artifacts))
            self.assertEqual(16, len(sboms))
            self.assertIn(output / "identity-platform-4.0.0.anxp", artifacts)

            for artifact_path in artifacts:
                stem = artifact_path.name.removesuffix(".anxp")
                manifest_path = output / f"{stem}.manifest.json"
                signature_path = output / f"{stem}.manifest.sig"
                public_key_path = output / f"{stem}.public-key.pem"
                manifest = json.loads(manifest_path.read_text(encoding="utf-8"))

                self.assertEqual("v2", manifest["api_version"])
                self.assertEqual("bin/control-entrypoints.json", manifest["control_entrypoint"]["path"])
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
                    control_index = archive.extractfile("bin/control-entrypoints.json")
                    self.assertIsNotNone(control_index)
                    self.assertEqual(
                        ["linux/amd64"],
                        [entry["architecture"] for entry in json.loads(control_index.read() if control_index else b"{}")["entries"]],
                    )

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
        requirement = stage["required_packages"][0]
        package_id = requirement["id"]
        contract["stages"] = [{**stage, "required_packages": [requirement]}]

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
            package_root = root / "packages" / package_id
            (package_root / "webui").mkdir(parents=True)
            (package_root / "webui" / "index.mjs").write_text("export default {};\n", encoding="utf-8")
            (package_root / "manifest.template.json").write_text(
                json.dumps(
                    {
                        "id": package_id,
                        "version": "4.0.0",
                        "targets": ["control"],
                        "webui": {
                            "bundle": {"path": "webui/index.mjs", "sha256": "fixture"},
                            "menus": [{"id": f"{package_id}.main"}],
                            "routes": [{"id": f"{package_id}.main"}],
                        },
                    }
                ),
                encoding="utf-8",
            )

            with self.assertRaisesRegex(RELEASE_STAGE_MODULE.StageContractError, "api_version"):
                RELEASE_STAGE_MODULE.validate_stage(root, contract, "v4.0.0")


if __name__ == "__main__":
    unittest.main()
