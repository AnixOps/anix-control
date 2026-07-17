#!/usr/bin/env python3
"""Build and verify the deterministic Machine Telemetry combined package."""

from __future__ import annotations

import argparse
import hashlib
import io
import json
import os
import re
import stat
import sys
import tarfile
import tempfile
from dataclasses import dataclass
from pathlib import Path
from typing import Any


PACKAGE_ROOT = Path(__file__).resolve().parent
PLUGIN_ID = "machine-telemetry"
PLUGIN_VERSION = "1.1.0"
ARTIFACT_NAME = f"{PLUGIN_ID}-{PLUGIN_VERSION}.tar"
MANIFEST_NAME = "manifest.json"
REPORT_NAME = "build-report.json"
CHECKSUM_NAME = "SHA256SUMS.txt"
PACKAGE_INDEX_NAME = "package.json"
WEBUI_PATH = "webui/index.mjs"
SCHEMA_PATH = "config.schema.json"
DEFAULTS_PATH = "config.defaults.json"
MANIFEST_TEMPLATE_PATH = "manifest.template.json"
FORMAT_VERSION = "anixops.package/v1"
BUILD_REPORT_VERSION = "anixops.package-build/v1"
MAX_ARTIFACT_BYTES = 32 << 20
MAX_WEBUI_BYTES = 2 << 20
MANIFEST_FIELDS = (
    "id",
    "name",
    "version",
    "api_version",
    "publisher",
    "targets",
    "architectures",
    "artifact_sha256",
    "capabilities",
    "dependencies",
    "conflicts",
    "permissions",
    "config_schema",
    "secret_fields",
    "entrypoints",
    "migration_version",
    "control_routes",
    "frontend_sha256",
    "webui",
)
WEBUI_FIELDS = ("bundle", "permissions", "menus", "routes")
WEBUI_BUNDLE_FIELDS = ("path", "sha256")
WEBUI_MENU_FIELDS = ("id", "parent", "label", "icon", "route", "permission", "order")
WEBUI_ROUTE_FIELDS = ("id", "path", "export", "permission")
SAFE_GO_TOKEN = re.compile(r"^[a-z0-9][a-z0-9_]{0,31}$")
STATIC_IMPORT = re.compile(r"(?m)^\s*import(?:\s|\{|'|\")")
DYNAMIC_IMPORT = re.compile(r"\bimport\s*\(")


class PackageError(RuntimeError):
    """Raised when package source or generated output violates the contract."""


@dataclass(frozen=True)
class AgentInput:
    path: Path
    goos: str
    goarch: str

    @property
    def architecture(self) -> str:
        return f"{self.goos}/{self.goarch}"

    @property
    def entrypoint_name(self) -> str:
        return f"agent-{self.goos}-{self.goarch}"

    @property
    def archive_path(self) -> str:
        return f"agent/{self.goos}-{self.goarch}/plugin"


@dataclass(frozen=True)
class ArchiveEntry:
    path: str
    data: bytes
    mode: int


def sha256_bytes(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def canonical_json(value: Any) -> bytes:
    encoded = json.dumps(value, ensure_ascii=True, separators=(",", ":"))
    # Go's encoding/json escapes these characters even though they are valid
    # JSON. Match it because the Control kernel signs json.Marshal output.
    encoded = encoded.replace("&", "\\u0026").replace("<", "\\u003c").replace(">", "\\u003e")
    return encoded.encode("utf-8")


def pretty_json(value: Any) -> bytes:
    return (json.dumps(value, ensure_ascii=True, indent=2, sort_keys=True) + "\n").encode("utf-8")


def sorted_json_value(value: Any) -> Any:
    if isinstance(value, dict):
        return {key: sorted_json_value(value[key]) for key in sorted(value)}
    if isinstance(value, list):
        return [sorted_json_value(item) for item in value]
    return value


def ordered_fields(value: Any, fields: tuple[str, ...], context: str) -> dict[str, Any]:
    if not isinstance(value, dict):
        raise PackageError(f"{context} must be a JSON object")
    missing = [field for field in fields if field not in value]
    unknown = sorted(set(value) - set(fields))
    if missing or unknown:
        raise PackageError(f"{context} fields are invalid: missing={missing}, unknown={unknown}")
    return {field: value[field] for field in fields}


def canonical_manifest_value(value: Any) -> dict[str, Any]:
    manifest = ordered_fields(value, MANIFEST_FIELDS, "manifest")
    manifest["config_schema"] = sorted_json_value(manifest["config_schema"])
    entrypoints = manifest["entrypoints"]
    if not isinstance(entrypoints, dict) or any(not isinstance(key, str) or not isinstance(path, str) for key, path in entrypoints.items()):
        raise PackageError("manifest entrypoints must be a string map")
    manifest["entrypoints"] = {key: entrypoints[key] for key in sorted(entrypoints)}

    webui = ordered_fields(manifest["webui"], WEBUI_FIELDS, "manifest webui")
    webui["bundle"] = ordered_fields(webui["bundle"], WEBUI_BUNDLE_FIELDS, "manifest webui bundle")
    menus = webui["menus"]
    routes = webui["routes"]
    if not isinstance(menus, list) or not isinstance(routes, list):
        raise PackageError("manifest WebUI menus and routes must be arrays")
    webui["menus"] = [ordered_fields(menu, WEBUI_MENU_FIELDS, "manifest WebUI menu") for menu in menus]
    webui["routes"] = [ordered_fields(route, WEBUI_ROUTE_FIELDS, "manifest WebUI route") for route in routes]
    manifest["webui"] = webui
    return manifest


def canonical_manifest_json(value: Any) -> bytes:
    return canonical_json(canonical_manifest_value(value))


def load_json(path: Path, context: str) -> Any:
    try:
        return json.loads(path.read_text(encoding="utf-8"))
    except (OSError, UnicodeError, json.JSONDecodeError) as exc:
        raise PackageError(f"invalid {context}: {exc}") from exc


def validate_agent_input(agent: AgentInput) -> bytes:
    if not SAFE_GO_TOKEN.fullmatch(agent.goos):
        raise PackageError(f"invalid GOOS: {agent.goos!r}")
    if not SAFE_GO_TOKEN.fullmatch(agent.goarch):
        raise PackageError(f"invalid GOARCH: {agent.goarch!r}")
    if agent.path.is_symlink():
        raise PackageError("Agent binary must not be a symbolic link")
    try:
        file_stat = agent.path.stat()
    except OSError as exc:
        raise PackageError(f"read Agent binary metadata: {exc}") from exc
    if not stat.S_ISREG(file_stat.st_mode):
        raise PackageError("Agent binary must be a regular file")
    if file_stat.st_size <= 0:
        raise PackageError("Agent binary must not be empty")
    if file_stat.st_mode & 0o111 == 0:
        raise PackageError("Agent binary must have an executable mode bit")
    try:
        return agent.path.read_bytes()
    except OSError as exc:
        raise PackageError(f"read Agent binary: {exc}") from exc


def validate_webui_source(source: bytes) -> None:
    if not source or len(source) > MAX_WEBUI_BYTES:
        raise PackageError(f"WebUI bundle must be between 1 and {MAX_WEBUI_BYTES} bytes")
    try:
        text = source.decode("utf-8")
    except UnicodeDecodeError as exc:
        raise PackageError("WebUI bundle must be UTF-8") from exc
    if STATIC_IMPORT.search(text) or DYNAMIC_IMPORT.search(text):
        raise PackageError("WebUI bundle must not contain static or dynamic imports")
    if "export default function create" not in text or "export const anixopsExtension" not in text:
        raise PackageError("WebUI bundle must export the extension descriptor and host factory")
    if "webuiApiVersion: 'anixops.webui/v1'" not in text:
        raise PackageError("WebUI bundle must declare anixops.webui/v1")


def validate_config_source(schema: Any, defaults: Any) -> None:
    if not isinstance(schema, dict) or schema.get("type") != "object":
        raise PackageError("configuration schema must be a JSON object schema")
    if not isinstance(defaults, dict):
        raise PackageError("configuration defaults must be a JSON object")
    properties = schema.get("properties")
    if not isinstance(properties, dict):
        raise PackageError("configuration schema properties must be an object")
    for name, definition in properties.items():
        if not isinstance(name, str) or not isinstance(definition, dict):
            raise PackageError(f"configuration schema property {name!r} is invalid")
    unknown = sorted(set(defaults) - set(properties))
    if unknown:
        raise PackageError(f"configuration defaults contain unknown keys: {unknown}")
    required = schema.get("required", [])
    if not isinstance(required, list) or any(key not in defaults for key in required):
        raise PackageError("configuration defaults do not cover required properties")
    interval = defaults.get("interval_seconds")
    interval_schema = properties.get("interval_seconds", {})
    if interval_schema.get("type") != "integer":
        raise PackageError("interval_seconds schema must be an integer")
    minimum = interval_schema.get("minimum")
    maximum = interval_schema.get("maximum")
    if not isinstance(minimum, int) or isinstance(minimum, bool) or not isinstance(maximum, int) or isinstance(maximum, bool) or minimum > maximum:
        raise PackageError("interval_seconds schema bounds are invalid")
    if not isinstance(interval, int) or isinstance(interval, bool):
        raise PackageError("interval_seconds default must be an integer")
    if interval < minimum or interval > maximum:
        raise PackageError("interval_seconds default is outside its schema bounds")
def validate_manifest_contract(manifest: dict[str, Any], architecture: str | None = None) -> None:
    if manifest.get("id") != PLUGIN_ID or manifest.get("version") != PLUGIN_VERSION:
        raise PackageError("generated manifest identity is invalid")
    if manifest.get("api_version") != "v1" or manifest.get("publisher") != "AnixOps":
        raise PackageError("generated manifest publisher or API version is invalid")
    if manifest.get("targets") != ["control", "agent"]:
        raise PackageError("generated manifest must contain control and agent targets")
    architectures = manifest.get("architectures")
    if not isinstance(architectures, list) or len(architectures) != 1 or not isinstance(architectures[0], str):
        raise PackageError("generated manifest must contain one architecture")
    if architecture is not None and architectures[0] != architecture:
        raise PackageError("generated manifest architecture does not match Agent input")
    if not re.fullmatch(r"[0-9a-f]{64}", str(manifest.get("artifact_sha256", ""))):
        raise PackageError("generated manifest artifact digest is invalid")
    if not re.fullmatch(r"[0-9a-f]{64}", str(manifest.get("frontend_sha256", ""))):
        raise PackageError("generated manifest frontend digest is invalid")


def source_bytes(relative_path: str) -> bytes:
    try:
        return (PACKAGE_ROOT / relative_path).read_bytes()
    except OSError as exc:
        raise PackageError(f"read package source {relative_path}: {exc}") from exc


def package_entries(agent: AgentInput, agent_binary: bytes) -> tuple[list[ArchiveEntry], dict[str, Any]]:
    webui = source_bytes(WEBUI_PATH)
    validate_webui_source(webui)
    schema_value = sorted_json_value(load_json(PACKAGE_ROOT / SCHEMA_PATH, "configuration schema"))
    defaults_value = sorted_json_value(load_json(PACKAGE_ROOT / DEFAULTS_PATH, "configuration defaults"))
    validate_config_source(schema_value, defaults_value)
    schema = pretty_json(schema_value)
    defaults = pretty_json(defaults_value)

    payload_entries = [
        ArchiveEntry(agent.archive_path, agent_binary, 0o755),
        ArchiveEntry(DEFAULTS_PATH, defaults, 0o644),
        ArchiveEntry(SCHEMA_PATH, schema, 0o644),
        ArchiveEntry(WEBUI_PATH, webui, 0o644),
    ]
    payload_entries.sort(key=lambda entry: entry.path)
    file_index = [
        {
            "mode": f"{entry.mode:04o}",
            "path": entry.path,
            "sha256": sha256_bytes(entry.data),
            "size": len(entry.data),
        }
        for entry in payload_entries
    ]
    package_index = {
        "architectures": [agent.architecture],
        "entrypoints": {agent.entrypoint_name: agent.archive_path},
        "files": file_index,
        "format": FORMAT_VERSION,
        "id": PLUGIN_ID,
        "targets": ["control", "agent"],
        "version": PLUGIN_VERSION,
    }
    entries = payload_entries + [ArchiveEntry(PACKAGE_INDEX_NAME, pretty_json(package_index), 0o644)]
    entries.sort(key=lambda entry: entry.path)
    return entries, package_index


def deterministic_tar(entries: list[ArchiveEntry]) -> bytes:
    output = io.BytesIO()
    with tarfile.open(fileobj=output, mode="w", format=tarfile.USTAR_FORMAT) as archive:
        for entry in sorted(entries, key=lambda item: item.path):
            info = tarfile.TarInfo(entry.path)
            info.size = len(entry.data)
            info.mode = entry.mode
            info.uid = 0
            info.gid = 0
            info.uname = ""
            info.gname = ""
            info.mtime = 0
            info.type = tarfile.REGTYPE
            archive.addfile(info, io.BytesIO(entry.data))
    return output.getvalue()


def generated_manifest(agent: AgentInput, artifact: bytes, webui: bytes, schema: Any) -> dict[str, Any]:
    template = load_json(PACKAGE_ROOT / MANIFEST_TEMPLATE_PATH, "manifest template")
    if not isinstance(template, dict):
        raise PackageError("manifest template must be a JSON object")
    if template.get("id") != PLUGIN_ID or template.get("version") != PLUGIN_VERSION:
        raise PackageError("manifest template identity does not match package identity")
    if template.get("targets") != ["control", "agent"]:
        raise PackageError("manifest template must declare control and agent targets")
    webui_digest = sha256_bytes(webui)
    template["architectures"] = [agent.architecture]
    template["artifact_sha256"] = sha256_bytes(artifact)
    template["config_schema"] = sorted_json_value(schema)
    template["entrypoints"] = {agent.entrypoint_name: agent.archive_path}
    template["frontend_sha256"] = webui_digest
    try:
        template["webui"]["bundle"]["sha256"] = webui_digest
    except (KeyError, TypeError) as exc:
        raise PackageError("manifest template WebUI bundle is invalid") from exc
    manifest = canonical_manifest_value(template)
    validate_manifest_contract(manifest, agent.architecture)
    return manifest


def output_report(agent: AgentInput, agent_binary: bytes, artifact: bytes, manifest: bytes, webui: bytes, schema: bytes) -> dict[str, Any]:
    return {
        "agent": {
            "architecture": agent.architecture,
            "entrypoint": agent.entrypoint_name,
            "path": agent.archive_path,
            "sha256": sha256_bytes(agent_binary),
            "size": len(agent_binary),
        },
        "artifact": {
            "format": "ustar",
            "name": ARTIFACT_NAME,
            "sha256": sha256_bytes(artifact),
            "size": len(artifact),
        },
        "format": BUILD_REPORT_VERSION,
        "manifest": {
            "name": MANIFEST_NAME,
            "sha256": sha256_bytes(manifest),
            "size": len(manifest),
        },
        "plugin_id": PLUGIN_ID,
        "schema": {
            "path": SCHEMA_PATH,
            "sha256": sha256_bytes(schema),
            "size": len(schema),
        },
        "version": PLUGIN_VERSION,
        "webui": {
            "path": WEBUI_PATH,
            "sha256": sha256_bytes(webui),
            "size": len(webui),
        },
    }


def atomic_write(path: Path, data: bytes, mode: int = 0o644) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    descriptor, temporary_name = tempfile.mkstemp(prefix=f".{path.name}.", dir=path.parent)
    temporary = Path(temporary_name)
    try:
        with os.fdopen(descriptor, "wb") as handle:
            handle.write(data)
            handle.flush()
            os.fsync(handle.fileno())
        os.chmod(temporary, mode)
        os.replace(temporary, path)
    finally:
        if temporary.exists():
            temporary.unlink()


def checksum_bytes(files: dict[str, bytes]) -> bytes:
    return "".join(f"{sha256_bytes(files[name])}  {name}\n" for name in sorted(files)).encode("ascii")


def build_package(agent: AgentInput, output_dir: Path) -> dict[str, bytes]:
    agent_binary = validate_agent_input(agent)
    entries, _ = package_entries(agent, agent_binary)
    artifact = deterministic_tar(entries)
    if len(artifact) > MAX_ARTIFACT_BYTES:
        raise PackageError(f"package artifact exceeds {MAX_ARTIFACT_BYTES} bytes")
    webui = source_bytes(WEBUI_PATH)
    schema_value = sorted_json_value(load_json(PACKAGE_ROOT / SCHEMA_PATH, "configuration schema"))
    schema = pretty_json(schema_value)
    manifest_value = generated_manifest(agent, artifact, webui, schema_value)
    manifest = canonical_manifest_json(manifest_value)
    report = pretty_json(output_report(agent, agent_binary, artifact, manifest, webui, schema))
    generated = {
        ARTIFACT_NAME: artifact,
        MANIFEST_NAME: manifest,
        REPORT_NAME: report,
    }
    generated[CHECKSUM_NAME] = checksum_bytes(generated)
    for name, data in generated.items():
        atomic_write(output_dir / name, data)
    verify_output_dir(output_dir)
    return generated


def parse_checksums(data: bytes) -> dict[str, str]:
    try:
        lines = data.decode("ascii").splitlines()
    except UnicodeDecodeError as exc:
        raise PackageError("checksum file must be ASCII") from exc
    checksums: dict[str, str] = {}
    for line_number, line in enumerate(lines, start=1):
        parts = line.split(maxsplit=1)
        if len(parts) != 2 or not re.fullmatch(r"[0-9a-f]{64}", parts[0]):
            raise PackageError(f"invalid checksum line {line_number}")
        name = parts[1].lstrip("*")
        if Path(name).name != name or name in checksums:
            raise PackageError(f"invalid checksum file name on line {line_number}")
        checksums[name] = parts[0]
    return checksums


def read_required_output(output_dir: Path, name: str) -> bytes:
    path = output_dir / name
    if not path.is_file():
        raise PackageError(f"generated output is missing: {name}")
    try:
        return path.read_bytes()
    except OSError as exc:
        raise PackageError(f"read generated output {name}: {exc}") from exc


def read_archive(artifact: bytes) -> tuple[dict[str, bytes], dict[str, int]]:
    contents: dict[str, bytes] = {}
    modes: dict[str, int] = {}
    try:
        with tarfile.open(fileobj=io.BytesIO(artifact), mode="r:") as archive:
            members = archive.getmembers()
            names = [member.name for member in members]
            if names != sorted(names) or len(names) != len(set(names)):
                raise PackageError("artifact entries must be unique and sorted")
            for member in members:
                if not member.isfile() or member.uid != 0 or member.gid != 0 or member.mtime != 0:
                    raise PackageError(f"artifact entry metadata is not deterministic: {member.name}")
                if member.uname or member.gname:
                    raise PackageError(f"artifact entry owner names must be empty: {member.name}")
                handle = archive.extractfile(member)
                if handle is None:
                    raise PackageError(f"artifact entry cannot be read: {member.name}")
                contents[member.name] = handle.read()
                modes[member.name] = member.mode & 0o777
    except (tarfile.TarError, OSError) as exc:
        raise PackageError(f"invalid package artifact: {exc}") from exc
    return contents, modes


def verify_package_index(contents: dict[str, bytes], modes: dict[str, int], manifest: dict[str, Any]) -> None:
    try:
        index = json.loads(contents[PACKAGE_INDEX_NAME])
    except (KeyError, json.JSONDecodeError, UnicodeDecodeError) as exc:
        raise PackageError("artifact package index is invalid") from exc
    if index.get("format") != FORMAT_VERSION or index.get("id") != PLUGIN_ID or index.get("version") != PLUGIN_VERSION:
        raise PackageError("artifact package index identity is invalid")
    if index.get("targets") != ["control", "agent"]:
        raise PackageError("artifact package index targets are invalid")
    if index.get("architectures") != manifest.get("architectures") or index.get("entrypoints") != manifest.get("entrypoints"):
        raise PackageError("artifact package index does not match manifest runtime targets")
    rows = index.get("files")
    if not isinstance(rows, list):
        raise PackageError("artifact package file index must be an array")
    indexed_paths: set[str] = set()
    for row in rows:
        if not isinstance(row, dict) or not isinstance(row.get("path"), str):
            raise PackageError("artifact package file index entry is invalid")
        path = row["path"]
        if path in indexed_paths or path == PACKAGE_INDEX_NAME or path not in contents:
            raise PackageError(f"artifact package file index path is invalid: {path}")
        indexed_paths.add(path)
        expected_mode = f"{modes[path]:04o}"
        if row.get("mode") != expected_mode or row.get("size") != len(contents[path]) or row.get("sha256") != sha256_bytes(contents[path]):
            raise PackageError(f"artifact package file index mismatch: {path}")
    if indexed_paths != set(contents) - {PACKAGE_INDEX_NAME}:
        raise PackageError("artifact package file index is incomplete")


def verify_output_dir(output_dir: Path) -> None:
    artifact = read_required_output(output_dir, ARTIFACT_NAME)
    if len(artifact) > MAX_ARTIFACT_BYTES:
        raise PackageError(f"package artifact exceeds {MAX_ARTIFACT_BYTES} bytes")
    manifest_bytes = read_required_output(output_dir, MANIFEST_NAME)
    report_bytes = read_required_output(output_dir, REPORT_NAME)
    checksums = parse_checksums(read_required_output(output_dir, CHECKSUM_NAME))
    expected_files = {
        ARTIFACT_NAME: artifact,
        MANIFEST_NAME: manifest_bytes,
        REPORT_NAME: report_bytes,
    }
    if set(checksums) != set(expected_files):
        raise PackageError("checksum file does not cover the exact generated output set")
    for name, data in expected_files.items():
        if checksums[name] != sha256_bytes(data):
            raise PackageError(f"generated output checksum mismatch: {name}")
    try:
        manifest = json.loads(manifest_bytes)
        report = json.loads(report_bytes)
    except (json.JSONDecodeError, UnicodeDecodeError) as exc:
        raise PackageError("generated manifest or report is invalid JSON") from exc
    if canonical_manifest_json(manifest) != manifest_bytes:
        raise PackageError("generated manifest is not canonical JSON")
    validate_manifest_contract(manifest)
    if manifest.get("artifact_sha256") != sha256_bytes(artifact):
        raise PackageError("generated manifest artifact digest mismatch")

    contents, modes = read_archive(artifact)
    expected_paths = {PACKAGE_INDEX_NAME, WEBUI_PATH, SCHEMA_PATH, DEFAULTS_PATH}
    entrypoints = manifest.get("entrypoints")
    if not isinstance(entrypoints, dict) or len(entrypoints) != 1:
        raise PackageError("generated manifest must contain one architecture entrypoint")
    entrypoint_name, entrypoint_path = next(iter(entrypoints.items()))
    if not re.fullmatch(r"agent-[a-z0-9_]+-[a-z0-9_]+", entrypoint_name):
        raise PackageError("generated manifest Agent entrypoint name is invalid")
    if entrypoint_path not in contents:
        raise PackageError("generated manifest Agent entrypoint is missing from artifact")
    expected_paths.add(entrypoint_path)
    if set(contents) != expected_paths:
        raise PackageError("artifact contains an unexpected file set")
    if modes[entrypoint_path] != 0o755 or any(modes[path] != 0o644 for path in expected_paths - {entrypoint_path}):
        raise PackageError("artifact entry modes are invalid")
    validate_webui_source(contents[WEBUI_PATH])
    bundle = manifest.get("webui", {}).get("bundle", {})
    if bundle.get("path") != WEBUI_PATH or bundle.get("sha256") != sha256_bytes(contents[WEBUI_PATH]):
        raise PackageError("generated manifest WebUI digest mismatch")
    if manifest.get("frontend_sha256") != bundle.get("sha256"):
        raise PackageError("generated manifest frontend digest mismatch")
    try:
        archive_schema = json.loads(contents[SCHEMA_PATH])
        archive_defaults = json.loads(contents[DEFAULTS_PATH])
    except (json.JSONDecodeError, UnicodeDecodeError) as exc:
        raise PackageError("artifact configuration files are invalid JSON") from exc
    validate_config_source(archive_schema, archive_defaults)
    if manifest.get("config_schema") != archive_schema:
        raise PackageError("generated manifest configuration schema mismatch")
    verify_package_index(contents, modes, manifest)

    if report.get("format") != BUILD_REPORT_VERSION:
        raise PackageError("build report format is invalid")
    if report.get("artifact", {}).get("sha256") != sha256_bytes(artifact):
        raise PackageError("build report artifact digest mismatch")
    if report.get("manifest", {}).get("sha256") != sha256_bytes(manifest_bytes):
        raise PackageError("build report manifest digest mismatch")
    if report.get("webui", {}).get("sha256") != sha256_bytes(contents[WEBUI_PATH]):
        raise PackageError("build report WebUI digest mismatch")
    if report.get("agent", {}).get("sha256") != sha256_bytes(contents[entrypoint_path]):
        raise PackageError("build report Agent digest mismatch")


def run_self_test() -> None:
    with tempfile.TemporaryDirectory() as temporary_root:
        root = Path(temporary_root)
        agent_path = root / "machine-telemetry-agent"
        agent_path.write_bytes(b"#!/bin/sh\nexit 0\n")
        agent_path.chmod(0o755)
        agent = AgentInput(agent_path, "linux", "amd64")
        first = root / "first"
        second = root / "second"
        first_outputs = build_package(agent, first)
        second_outputs = build_package(agent, second)
        if first_outputs != second_outputs:
            raise PackageError("self-test package builds are not byte-identical")
        verify_output_dir(first)
        artifact_path = first / ARTIFACT_NAME
        tampered = bytearray(artifact_path.read_bytes())
        tampered[0] ^= 0x01
        artifact_path.write_bytes(tampered)
        try:
            verify_output_dir(first)
        except PackageError:
            pass
        else:
            raise PackageError("self-test failed to reject a tampered artifact")
    print("machine-telemetry package self-test passed")


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    subparsers = parser.add_subparsers(dest="command", required=True)

    build_parser = subparsers.add_parser("build", help="Build and verify a combined package artifact.")
    build_parser.add_argument("--agent-binary", required=True, type=Path, help="Path to the real Agent plugin executable.")
    build_parser.add_argument("--goos", required=True, help="GOOS represented by the Agent binary.")
    build_parser.add_argument("--goarch", required=True, help="GOARCH represented by the Agent binary.")
    build_parser.add_argument("--output-dir", required=True, type=Path, help="Directory for generated package outputs.")

    verify_parser = subparsers.add_parser("verify", help="Verify an existing generated package directory.")
    verify_parser.add_argument("--output-dir", required=True, type=Path, help="Directory containing generated package outputs.")

    subparsers.add_parser("self-test", help="Run a hermetic deterministic-build and tamper self-test.")
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    try:
        if args.command == "self-test":
            run_self_test()
            return 0
        if args.command == "verify":
            verify_output_dir(args.output_dir)
            print(f"verified {args.output_dir}")
            return 0
        agent = AgentInput(args.agent_binary, args.goos.strip().lower(), args.goarch.strip().lower())
        build_package(agent, args.output_dir)
        print(f"built and verified {args.output_dir / ARTIFACT_NAME}")
        return 0
    except PackageError as exc:
        print(f"{Path(__file__).name}: error: {exc}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
