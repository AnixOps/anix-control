#!/usr/bin/env python3
"""Build deterministic signed AnixOps v2 package artifacts."""

from __future__ import annotations

import argparse
import base64
import binascii
import gzip
import hashlib
import hmac
import io
import json
import os
import shutil
import stat
import subprocess
import tarfile
import tempfile
from contextlib import contextmanager
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Iterator


SHARED_ROOT = Path(__file__).resolve().parent
PACKAGES_ROOT = SHARED_ROOT.parent
REPO_ROOT = PACKAGES_ROOT.parent
MANIFEST_API_VERSION = "v2"
AGENT_RUNTIME_API_VERSION_V110 = "anixops.agent.sdk/v1.1.0"
PACKAGE_FORMAT = "anixops.package/v2"
MAX_ARTIFACT_BYTES = 64 << 20
MAX_RUNTIME_BINARY_BYTES = 32 << 20
ED25519_PUBLIC_KEY_BYTES = 32
ED25519_SPKI_PREFIX = bytes.fromhex("302a300506032b6570032100")
SAFE_SEGMENT_CHARACTERS = frozenset("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789._+-")
WEBUI_VERSION_TOKEN = b"__ANIXOPS_PACKAGE_VERSION__"
ENTRYPOINT_INDEX_FORMAT = "anixops.package-entrypoints/v1"
CONTROL_ENTRYPOINT_INDEX_PATH = "bin/control-entrypoints.json"
AGENT_ENTRYPOINT_INDEX_PATH = "agent/entrypoints.json"
SUPPORTED_PLATFORMS = (("linux", "amd64"), ("linux", "arm64"))


@dataclass(frozen=True)
class PackageSpec:
    package_id: str
    targets: tuple[str, ...]
    runtimes: tuple[str, ...] = ()


PACKAGE_SPECS = (
    PackageSpec("identity-platform", ("control",)),
    PackageSpec("subscription", ("control",)),
    PackageSpec("proxy-node", ("control",)),
    PackageSpec("plan", ("control",)),
    PackageSpec("order", ("control",)),
    PackageSpec("payment", ("control",)),
    PackageSpec("forward", ("control",)),
    PackageSpec("ticket", ("control",)),
    PackageSpec("notification", ("control",)),
    PackageSpec("knowledge", ("control",)),
    PackageSpec("machine-telemetry", ("control", "agent")),
    PackageSpec("nftables-forward", ("control", "agent")),
    PackageSpec("gost-mesh", ("control", "agent"), ("gost",)),
    PackageSpec("nat-egress", ("control", "agent")),
    PackageSpec("wireguard", ("control",)),
    PackageSpec("protocol-runtime", ("control",)),
)

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
    "control_entrypoint",
    "agent_entrypoint",
    "migrations",
    "compatibility_routes",
    "route_contract_digest",
    "runtime_api_version",
)


class PackageBuildError(RuntimeError):
    """Raised when a package source or generated output violates the contract."""


@dataclass(frozen=True)
class ArchiveEntry:
    path: str
    data: bytes
    mode: int


def sha256_bytes(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def canonical_json(value: Any) -> bytes:
    encoded = json.dumps(value, ensure_ascii=False, separators=(",", ":"))
    # encoding/json escapes these values, and manifests are signed by Go as
    # well as by this builder.
    encoded = (
        encoded.replace("&", "\\u0026")
        .replace("<", "\\u003c")
        .replace(">", "\\u003e")
        .replace("\u2028", "\\u2028")
        .replace("\u2029", "\\u2029")
    )
    try:
        return encoded.encode("utf-8")
    except UnicodeEncodeError as error:
        raise PackageBuildError("canonical manifest must contain valid UTF-8") from error


def pretty_json(value: Any) -> bytes:
    return (json.dumps(value, ensure_ascii=True, indent=2, sort_keys=True) + "\n").encode("utf-8")


def sorted_json_value(value: Any) -> Any:
    if isinstance(value, dict):
        return {key: sorted_json_value(value[key]) for key in sorted(value)}
    if isinstance(value, list):
        return [sorted_json_value(item) for item in value]
    return value


def load_json(path: Path, label: str) -> Any:
    try:
        return json.loads(path.read_text(encoding="utf-8"))
    except (OSError, UnicodeError, json.JSONDecodeError) as error:
        raise PackageBuildError(f"invalid {label}: {error}") from error


def read_source(path: Path, label: str) -> bytes:
    try:
        value = path.read_bytes()
    except OSError as error:
        raise PackageBuildError(f"read {label}: {error}") from error
    if not value:
        raise PackageBuildError(f"{label} must not be empty")
    return value


def require_relative_path(value: Any, label: str) -> str:
    if not isinstance(value, str) or not value:
        raise PackageBuildError(f"{label} path is required")
    candidate = Path(value)
    if candidate.is_absolute() or "\\" in value or value != candidate.as_posix() or ".." in candidate.parts:
        raise PackageBuildError(f"{label} path must be canonical and relative")
    return value


def require_safe_segment(value: Any, label: str) -> str:
    if (
        not isinstance(value, str)
        or not value
        or len(value) > 120
        or value != value.strip()
        or value in {".", ".."}
        or any(character not in SAFE_SEGMENT_CHARACTERS for character in value)
    ):
        raise PackageBuildError(f"{label} must be a safe path segment")
    return value


def platform_name(platform: tuple[str, str]) -> str:
    return platform[0] + "/" + platform[1]


def platform_path_suffix(platform: tuple[str, str]) -> str:
    return platform[0] + "-" + platform[1]


def parse_platform(value: str) -> tuple[str, str]:
    if not isinstance(value, str) or value.count("/") != 1:
        raise PackageBuildError("platform must use GOOS/GOARCH")
    goos, goarch = value.split("/", 1)
    if (goos, goarch) not in SUPPORTED_PLATFORMS:
        raise PackageBuildError(f"unsupported package platform {value!r}")
    return goos, goarch


def normalize_platforms(values: list[str]) -> tuple[tuple[str, str], ...]:
    platforms = tuple(sorted({parse_platform(value) for value in values}, key=platform_name))
    if not platforms:
        raise PackageBuildError("at least one package platform is required")
    if len(platforms) != len(values):
        raise PackageBuildError("duplicate package platform")
    return platforms


def parse_agent_binary_inputs(values: list[str]) -> dict[tuple[str, tuple[str, str]], Path]:
    agent_package_ids = {spec.package_id for spec in PACKAGE_SPECS if "agent" in spec.targets}
    inputs: dict[tuple[str, tuple[str, str]], Path] = {}
    for value in values:
        if not isinstance(value, str) or value.count("=") != 1:
            raise PackageBuildError("agent binary must use PACKAGE_ID@GOOS/GOARCH=PATH")
        package_platform, raw_path = value.split("=", 1)
        if package_platform.count("@") != 1:
            raise PackageBuildError("agent binary must use PACKAGE_ID@GOOS/GOARCH=PATH")
        package_id, raw_platform = package_platform.split("@", 1)
        if package_id not in agent_package_ids:
            raise PackageBuildError(f"agent binary package {package_id!r} is not an Agent-target package")
        platform = parse_platform(raw_platform)
        path = Path(raw_path)
        if not path.is_absolute():
            raise PackageBuildError("agent binary path must be absolute")
        key = (package_id, platform)
        if key in inputs:
            raise PackageBuildError(f"duplicate agent binary input for {package_id}@{platform_name(platform)}")
        inputs[key] = path
    return inputs


def parse_runtime_binary_inputs(values: list[str]) -> dict[tuple[str, str, tuple[str, str]], Path]:
    runtime_keys = {(spec.package_id, runtime_name) for spec in PACKAGE_SPECS for runtime_name in spec.runtimes}
    inputs: dict[tuple[str, str, tuple[str, str]], Path] = {}
    for value in values:
        if not isinstance(value, str) or value.count("=") != 1:
            raise PackageBuildError("runtime binary must use PACKAGE_ID:RUNTIME@GOOS/GOARCH=PATH")
        package_runtime_platform, raw_path = value.split("=", 1)
        if package_runtime_platform.count("@") != 1:
            raise PackageBuildError("runtime binary must use PACKAGE_ID:RUNTIME@GOOS/GOARCH=PATH")
        package_runtime, raw_platform = package_runtime_platform.split("@", 1)
        if package_runtime.count(":") != 1:
            raise PackageBuildError("runtime binary must use PACKAGE_ID:RUNTIME@GOOS/GOARCH=PATH")
        package_id, runtime_name = package_runtime.split(":", 1)
        if (package_id, runtime_name) not in runtime_keys:
            raise PackageBuildError(f"runtime binary {package_id}:{runtime_name} is not declared by the package matrix")
        platform = parse_platform(raw_platform)
        path = Path(raw_path)
        if not path.is_absolute():
            raise PackageBuildError("runtime binary path must be absolute")
        key = (package_id, runtime_name, platform)
        if key in inputs:
            raise PackageBuildError(
                f"duplicate runtime binary input for {package_id}:{runtime_name}@{platform_name(platform)}"
            )
        inputs[key] = path
    return inputs


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


def deterministic_artifact(entries: list[ArchiveEntry]) -> bytes:
    raw_artifact = deterministic_tar(entries)
    if len(raw_artifact) <= MAX_ARTIFACT_BYTES:
        return raw_artifact

    output = io.BytesIO()
    with gzip.GzipFile(fileobj=output, mode="wb", compresslevel=9, mtime=0, filename="") as compressor:
        compressor.write(raw_artifact)
    artifact = output.getvalue()
    if len(artifact) > MAX_ARTIFACT_BYTES:
        raise PackageBuildError(f"package artifact exceeds {MAX_ARTIFACT_BYTES} bytes even after deterministic gzip encoding")
    return artifact


def package_root(package_id: str) -> Path:
    return PACKAGES_ROOT / package_id


def source_webui(package_id: str, version: str) -> bytes:
    source = read_source(package_root(package_id) / "webui" / "index.mjs", f"{package_id} WebUI bundle")
    if source.count(WEBUI_VERSION_TOKEN) != 1:
        raise PackageBuildError(f"{package_id} WebUI bundle must declare exactly one release version token")
    return source.replace(WEBUI_VERSION_TOKEN, version.encode("utf-8"))


def generated_control_host(package_id: str) -> bytes:
    return (
        "#!/bin/sh\n"
        f"printf '%s\\n' 'AnixOps package {package_id} host is not implemented in the manifest stage' >&2\n"
        "exit 78\n"
    ).encode("utf-8")


def generated_migrations_index(package_id: str, version: str) -> bytes:
    return pretty_json(
        {
            "format": "anixops.migrations/v1",
            "migrations": [],
            "package_id": package_id,
            "version": version,
        }
    )


def generated_compatibility_routes(package_id: str) -> bytes:
    return pretty_json(
        {
            "api_version": "v2",
            "package_id": package_id,
            "routes": [],
        }
    )


def source_control_host(spec: PackageSpec, version: str, platform: tuple[str, str]) -> bytes:
    source = package_root(spec.package_id) / "control"
    if shutil.which("go") is None:
        raise PackageBuildError(f"{spec.package_id} Control host requires the Go toolchain")
    package_path = f"./packages/{spec.package_id}/control" if source.is_dir() else "./packages/shared/controlhost"
    link_flags = f"-s -w -buildid= -X main.packageVersion={version}"
    if not source.is_dir():
        link_flags = f"{link_flags} -X main.packageID={spec.package_id}"
    with tempfile.TemporaryDirectory(prefix=f"anixops-{spec.package_id}-host-") as temporary:
        output = Path(temporary) / "control-host"
        environment = os.environ.copy()
        environment.update({"CGO_ENABLED": "0", "GOARCH": platform[1], "GOOS": platform[0], "GOWORK": "off"})
        result = subprocess.run(
            [
                "go",
                "build",
                "-trimpath",
                "-buildvcs=false",
                "-ldflags",
                link_flags,
                "-o",
                str(output),
                package_path,
            ],
            cwd=REPO_ROOT,
            env=environment,
            check=False,
            text=True,
            capture_output=True,
        )
        if result.returncode != 0:
            detail = result.stderr.strip() or result.stdout.strip()
            raise PackageBuildError(f"build {spec.package_id} Control host: {detail}")
        return read_source(output, f"{spec.package_id} compiled Control host")


def source_executable_input(path: Path, label: str, maximum: int) -> bytes:
    if path.is_symlink() or not path.is_file():
        raise PackageBuildError(f"{label} must be a regular file")
    try:
        mode = stat.S_IMODE(path.stat().st_mode)
    except OSError as error:
        raise PackageBuildError(f"inspect {label}: {error}") from error
    if mode & 0o111 == 0:
        raise PackageBuildError(f"{label} must be executable")
    binary = read_source(path, label)
    if len(binary) > maximum:
        raise PackageBuildError(f"{label} exceeds {maximum} bytes")
    return binary


def source_agent_binary(package_id: str, platform: tuple[str, str], inputs: dict[tuple[str, tuple[str, str]], Path]) -> bytes:
    path = inputs.get((package_id, platform))
    if path is None:
        raise PackageBuildError(f"{package_id} agent binary for {platform_name(platform)} is required")
    return source_executable_input(
        path,
        f"{package_id} Agent binary for {platform_name(platform)}",
        MAX_ARTIFACT_BYTES,
    )


def runtime_binary_contract(package_id: str, runtime_name: str, platform: tuple[str, str]) -> dict[str, str]:
    if (package_id, runtime_name) != ("gost-mesh", "gost"):
        raise PackageBuildError(f"no pinned runtime contract exists for {package_id}:{runtime_name}")
    contract = load_json(package_root(package_id) / "runtime-contract.json", f"{package_id} runtime contract")
    if (
        not isinstance(contract, dict)
        or set(contract) != {"format", "platforms", "runtime", "version"}
        or contract.get("format") != "anixops.runtime-contract/v1"
        or contract.get("runtime") != runtime_name
        or contract.get("version") != "3.2.6"
        or not isinstance(contract.get("platforms"), dict)
    ):
        raise PackageBuildError(f"{package_id} runtime contract is invalid")
    entry = contract["platforms"].get(platform_name(platform))
    if (
        not isinstance(entry, dict)
        or set(entry) != {"archive_sha256", "binary_sha256"}
        or not valid_digest(entry.get("archive_sha256"))
        or not valid_digest(entry.get("binary_sha256"))
    ):
        raise PackageBuildError(f"{package_id} runtime contract has no valid {runtime_name} entry for {platform_name(platform)}")
    return {"archive_sha256": entry["archive_sha256"], "binary_sha256": entry["binary_sha256"]}


def runtime_entrypoint_name(runtime_name: str, platform: tuple[str, str]) -> str:
    return f"runtime-{runtime_name}-{platform_path_suffix(platform)}"


def runtime_archive_path(runtime_name: str, platform: tuple[str, str]) -> str:
    return f"runtime/{platform_path_suffix(platform)}/{runtime_name}"


def generated_runtime_entrypoints(spec: PackageSpec, platforms: tuple[tuple[str, str], ...]) -> dict[str, str]:
    return {
        runtime_entrypoint_name(runtime_name, platform): runtime_archive_path(runtime_name, platform)
        for runtime_name in spec.runtimes
        for platform in platforms
    }


def source_runtime_binary(
    package_id: str,
    runtime_name: str,
    platform: tuple[str, str],
    inputs: dict[tuple[str, str, tuple[str, str]], Path],
    *,
    require_pinned: bool = True,
) -> bytes:
    path = inputs.get((package_id, runtime_name, platform))
    if path is None:
        raise PackageBuildError(f"{package_id} {runtime_name} runtime binary for {platform_name(platform)} is required")
    label = f"{package_id} {runtime_name} runtime binary for {platform_name(platform)}"
    binary = source_executable_input(path, label, MAX_RUNTIME_BINARY_BYTES)
    if require_pinned:
        expected = runtime_binary_contract(package_id, runtime_name, platform)["binary_sha256"]
        actual = sha256_bytes(binary)
        if not hmac.compare_digest(actual, expected):
            raise PackageBuildError(
                f"{package_id} {runtime_name} runtime binary SHA-256 mismatch for {platform_name(platform)}"
            )
    return binary


def generated_entrypoint_index(entries: list[dict[str, str]]) -> bytes:
    ordered = sorted(entries, key=lambda entry: entry["architecture"])
    return pretty_json({"entries": ordered, "format": ENTRYPOINT_INDEX_FORMAT})


def source_compatibility_routes(package_id: str) -> bytes:
    source = package_root(package_id) / "compat" / "v2-routes.json"
    if not source.is_file():
        return generated_compatibility_routes(package_id)
    routes = load_json(source, f"{package_id} compatibility routes")
    if not isinstance(routes, dict) or routes.get("api_version") != "v2" or routes.get("package_id") != package_id or not isinstance(routes.get("routes"), list):
        raise PackageBuildError(f"{package_id} compatibility routes are invalid")
    return read_source(source, f"{package_id} compatibility routes")


def source_migrations(package_id: str, version: str) -> tuple[bytes, list[ArchiveEntry]]:
    source_root = package_root(package_id)
    index_path = source_root / "migrations" / "index.json"
    if not index_path.is_file():
        return generated_migrations_index(package_id, version), []
    index = load_json(index_path, f"{package_id} migrations index")
    if (
        not isinstance(index, dict)
        or index.get("format") != "anixops.migrations/v1"
        or index.get("package_id") != package_id
        or index.get("version") != version
        or not isinstance(index.get("migrations"), list)
    ):
        raise PackageBuildError(f"{package_id} migrations index is invalid")
    entries: list[ArchiveEntry] = []
    seen: set[str] = set()
    for migration in index["migrations"]:
        if not isinstance(migration, dict) or not isinstance(migration.get("id"), str):
            raise PackageBuildError(f"{package_id} migration entry is invalid")
        relative_path = require_relative_path(migration.get("path"), f"{package_id} migration")
        if not relative_path.startswith("migrations/") or relative_path == "migrations/index.json" or relative_path in seen:
            raise PackageBuildError(f"{package_id} migration path is invalid")
        seen.add(relative_path)
        entries.append(ArchiveEntry(relative_path, read_source(source_root / relative_path, f"{package_id} {relative_path}"), 0o644))
    return read_source(index_path, f"{package_id} migrations index"), entries


def package_entries(
    spec: PackageSpec,
    version: str,
    platforms: tuple[tuple[str, str], ...],
    agent_inputs: dict[tuple[str, tuple[str, str]], Path],
    runtime_inputs: dict[tuple[str, str, tuple[str, str]], Path],
    *,
    formal_release: bool = False,
) -> tuple[list[ArchiveEntry], dict[str, Any]]:
    migrations, migration_entries = source_migrations(spec.package_id, version)
    routes = source_compatibility_routes(spec.package_id)
    webui = source_webui(spec.package_id, version)
    entries = [
        ArchiveEntry("compat/v2-routes.json", routes, 0o644),
        ArchiveEntry("migrations/index.json", migrations, 0o644),
        ArchiveEntry("webui/index.mjs", webui, 0o644),
    ]
    control_index_entries: list[dict[str, str]] = []
    for platform in platforms:
        control_host = source_control_host(spec, version, platform)
        control_path = f"bin/control-host-{platform_path_suffix(platform)}"
        entries.append(ArchiveEntry(control_path, control_host, 0o755))
        control_index_entries.append(
            {"architecture": platform_name(platform), "path": control_path, "sha256": sha256_bytes(control_host)}
        )
    control_entrypoint = generated_entrypoint_index(control_index_entries)
    entries.append(ArchiveEntry(CONTROL_ENTRYPOINT_INDEX_PATH, control_entrypoint, 0o644))
    entries.extend(migration_entries)
    source_root = package_root(spec.package_id)
    for relative_path in ("config.schema.json", "config.defaults.json"):
        source = source_root / relative_path
        if source.is_file():
            entries.append(ArchiveEntry(relative_path, read_source(source, f"{spec.package_id} {relative_path}"), 0o644))

    agent_entrypoint = b""
    if "agent" in spec.targets:
        agent_index_entries: list[dict[str, str]] = []
        for platform in platforms:
            agent = source_agent_binary(spec.package_id, platform, agent_inputs)
            agent_path = f"agent/{platform_path_suffix(platform)}/plugin"
            entries.append(ArchiveEntry(agent_path, agent, 0o755))
            agent_index_entries.append(
                {"architecture": platform_name(platform), "path": agent_path, "sha256": sha256_bytes(agent)}
            )
        agent_entrypoint = generated_entrypoint_index(agent_index_entries)
        entries.append(ArchiveEntry(AGENT_ENTRYPOINT_INDEX_PATH, agent_entrypoint, 0o644))

    runtime_entrypoints = generated_runtime_entrypoints(spec, platforms)
    for runtime_name in spec.runtimes:
        for platform in platforms:
            runtime_binary = source_runtime_binary(
                spec.package_id,
                runtime_name,
                platform,
                runtime_inputs,
                require_pinned=formal_release,
            )
            entries.append(
                ArchiveEntry(runtime_archive_path(runtime_name, platform), runtime_binary, 0o755)
            )

    entries.sort(key=lambda entry: entry.path)
    file_index = [
        {
            "mode": f"{entry.mode:04o}",
            "path": entry.path,
            "sha256": sha256_bytes(entry.data),
            "size": len(entry.data),
        }
        for entry in entries
    ]
    package_index = {
        "files": file_index,
        "format": PACKAGE_FORMAT,
        "id": spec.package_id,
        "targets": list(spec.targets),
        "version": version,
    }
    entries.append(ArchiveEntry("package.json", pretty_json(package_index), 0o644))
    entries.sort(key=lambda entry: entry.path)
    return entries, {
        "agent_entrypoint": agent_entrypoint,
        "control_entrypoint": control_entrypoint,
        "migrations": migrations,
        "routes": routes,
        "runtime_entrypoints": runtime_entrypoints,
        "webui": webui,
    }


def ordered_manifest(template: Any) -> dict[str, Any]:
    if not isinstance(template, dict):
        raise PackageBuildError("manifest template must be a JSON object")
    missing = [field for field in MANIFEST_FIELDS if field not in template]
    unknown = sorted(set(template) - set(MANIFEST_FIELDS))
    if missing or unknown:
        raise PackageBuildError(f"manifest template fields are invalid: missing={missing}, unknown={unknown}")
    return {field: template[field] for field in MANIFEST_FIELDS}


def emitted_manifest_fields(spec: PackageSpec) -> tuple[str, ...]:
    if "agent" in spec.targets:
        return MANIFEST_FIELDS
    return tuple(field for field in MANIFEST_FIELDS if field not in {"agent_entrypoint", "runtime_api_version"})


def generated_manifest(
    spec: PackageSpec,
    version: str,
    platforms: tuple[tuple[str, str], ...],
    artifact: bytes,
    contents: dict[str, Any],
) -> dict[str, Any]:
    template_path = package_root(spec.package_id) / "manifest.template.json"
    manifest = ordered_manifest(load_json(template_path, f"{spec.package_id} manifest template"))
    if manifest["id"] != spec.package_id:
        raise PackageBuildError(f"{spec.package_id} manifest template identity is invalid")
    if tuple(manifest["targets"]) != spec.targets:
        raise PackageBuildError(f"{spec.package_id} manifest targets do not match the package matrix")

    webui = manifest["webui"]
    if not isinstance(webui, dict) or not isinstance(webui.get("bundle"), dict):
        raise PackageBuildError(f"{spec.package_id} manifest WebUI bundle is required")
    bundle_path = require_relative_path(webui["bundle"].get("path"), f"{spec.package_id} WebUI bundle")
    if bundle_path != "webui/index.mjs":
        raise PackageBuildError(f"{spec.package_id} manifest WebUI bundle must use webui/index.mjs")

    manifest["version"] = version
    manifest["api_version"] = MANIFEST_API_VERSION
    manifest["architectures"] = [platform_name(platform) for platform in platforms]
    manifest["artifact_sha256"] = sha256_bytes(artifact)
    manifest["config_schema"] = sorted_json_value(manifest["config_schema"])
    manifest["entrypoints"] = contents["runtime_entrypoints"]
    manifest["migration_version"] = 0
    manifest["frontend_sha256"] = sha256_bytes(contents["webui"])
    webui["bundle"]["sha256"] = manifest["frontend_sha256"]
    manifest["control_entrypoint"] = {
        "path": CONTROL_ENTRYPOINT_INDEX_PATH,
        "sha256": sha256_bytes(contents["control_entrypoint"]),
    }
    manifest["migrations"] = {
        "index": "migrations/index.json",
        "sha256": sha256_bytes(contents["migrations"]),
    }
    manifest["compatibility_routes"] = {
        "path": "compat/v2-routes.json",
        "sha256": sha256_bytes(contents["routes"]),
    }
    # This contract is deliberately the final compatibility-routes file bytes.
    manifest["route_contract_digest"] = sha256_bytes(contents["routes"])
    if "agent" in spec.targets:
        manifest["agent_entrypoint"] = {
            "path": AGENT_ENTRYPOINT_INDEX_PATH,
            "sha256": sha256_bytes(contents["agent_entrypoint"]),
        }
        manifest["runtime_api_version"] = AGENT_RUNTIME_API_VERSION_V110
    else:
        del manifest["agent_entrypoint"]
        del manifest["runtime_api_version"]
    validate_manifest_value(manifest, spec)
    return manifest


def valid_digest(value: Any) -> bool:
    return isinstance(value, str) and len(value) == 64 and all(char in "0123456789abcdef" for char in value)


def validate_manifest_value(manifest: dict[str, Any], spec: PackageSpec) -> None:
    if tuple(manifest) != emitted_manifest_fields(spec):
        raise PackageBuildError("generated manifest field order is not canonical")
    if manifest.get("api_version") != MANIFEST_API_VERSION:
        raise PackageBuildError("generated manifest API version is invalid")
    if manifest.get("publisher") != "AnixOps":
        raise PackageBuildError("generated manifest publisher is invalid")
    if tuple(manifest.get("targets", [])) != spec.targets:
        raise PackageBuildError("generated manifest targets do not match the package matrix")
    architectures = manifest.get("architectures")
    if not isinstance(architectures, list):
        raise PackageBuildError("generated manifest architectures are invalid")
    try:
        platforms = tuple(parse_platform(architecture) for architecture in architectures)
    except PackageBuildError as error:
        raise PackageBuildError("generated manifest architectures are invalid") from error
    if len(platforms) != len(architectures) or len(set(platforms)) != len(platforms):
        raise PackageBuildError("generated manifest architectures are invalid")
    if manifest.get("entrypoints") != generated_runtime_entrypoints(spec, platforms):
        raise PackageBuildError("generated manifest runtime entrypoints are invalid")
    for value in (
        manifest.get("artifact_sha256"),
        manifest.get("frontend_sha256"),
        manifest.get("route_contract_digest"),
        manifest.get("control_entrypoint", {}).get("sha256") if isinstance(manifest.get("control_entrypoint"), dict) else None,
        manifest.get("migrations", {}).get("sha256") if isinstance(manifest.get("migrations"), dict) else None,
        manifest.get("compatibility_routes", {}).get("sha256") if isinstance(manifest.get("compatibility_routes"), dict) else None,
    ):
        if not valid_digest(value):
            raise PackageBuildError("generated manifest digest is invalid")
    for key, field in (("control_entrypoint", "path"), ("migrations", "index"), ("compatibility_routes", "path")):
        value = manifest.get(key)
        if not isinstance(value, dict):
            raise PackageBuildError(f"generated manifest {key} is required")
        require_relative_path(value.get(field), f"generated manifest {key}")
    agent_entrypoint = manifest.get("agent_entrypoint")
    if "agent" in spec.targets:
        if not isinstance(agent_entrypoint, dict):
            raise PackageBuildError("generated Agent-target manifest requires agent_entrypoint")
        require_relative_path(agent_entrypoint.get("path"), "generated manifest agent_entrypoint")
        if not valid_digest(agent_entrypoint.get("sha256")):
            raise PackageBuildError("generated manifest Agent entrypoint digest is invalid")
        if manifest.get("runtime_api_version") != AGENT_RUNTIME_API_VERSION_V110:
            raise PackageBuildError("generated manifest runtime API version is invalid")
    elif agent_entrypoint is not None or manifest.get("runtime_api_version") is not None:
        raise PackageBuildError("Control-only manifest declares Agent runtime metadata")


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


@dataclass(frozen=True)
class Signer:
    private_key: Path
    public_key: bytes

    def sign(self, manifest: bytes) -> bytes:
        with tempfile.TemporaryDirectory(prefix="anixops-package-sign-") as temporary:
            root = Path(temporary)
            manifest_path = root / "manifest.json"
            signature_path = root / "manifest.sig"
            manifest_path.write_bytes(manifest)
            result = subprocess.run(
                [
                    "openssl",
                    "pkeyutl",
                    "-sign",
                    "-inkey",
                    str(self.private_key),
                    "-rawin",
                    "-in",
                    str(manifest_path),
                    "-out",
                    str(signature_path),
                ],
                check=False,
                text=True,
                capture_output=True,
            )
            if result.returncode != 0:
                raise PackageBuildError(f"sign canonical manifest: {result.stderr.strip() or result.stdout.strip()}")
            return signature_path.read_bytes()


def ed25519_public_key_from_der(value: bytes, label: str) -> bytes:
    if len(value) != len(ED25519_SPKI_PREFIX) + ED25519_PUBLIC_KEY_BYTES or not hmac.compare_digest(
        value[: len(ED25519_SPKI_PREFIX)], ED25519_SPKI_PREFIX
    ):
        raise PackageBuildError(f"{label} must be Ed25519")
    return value[len(ED25519_SPKI_PREFIX) :]


def ed25519_public_key_pem(public_key: bytes) -> bytes:
    if len(public_key) != ED25519_PUBLIC_KEY_BYTES:
        raise PackageBuildError("official public key must contain 32 raw Ed25519 bytes")
    encoded = base64.b64encode(ED25519_SPKI_PREFIX + public_key).decode("ascii")
    lines = [encoded[index : index + 64] for index in range(0, len(encoded), 64)]
    return ("-----BEGIN PUBLIC KEY-----\n" + "\n".join(lines) + "\n-----END PUBLIC KEY-----\n").encode("ascii")


def derive_ed25519_public_key(private_key: Path) -> bytes:
    with tempfile.TemporaryDirectory(prefix="anixops-package-public-key-") as temporary:
        result = subprocess.run(
            ["openssl", "pkey", "-in", str(private_key), "-pubout", "-outform", "DER"],
            check=False,
            capture_output=True,
        )
        if result.returncode != 0:
            detail = result.stderr.decode("utf-8", errors="replace").strip() or result.stdout.decode("utf-8", errors="replace").strip()
            raise PackageBuildError(f"derive signing public key: {detail}")
        return ed25519_public_key_from_der(result.stdout, "signing key")


def derive_ed25519_public_key_from_pem(public_key: Path) -> bytes:
    result = subprocess.run(
        ["openssl", "pkey", "-pubin", "-in", str(public_key), "-pubout", "-outform", "DER"],
        check=False,
        capture_output=True,
    )
    if result.returncode != 0:
        detail = result.stderr.decode("utf-8", errors="replace").strip() or result.stdout.decode("utf-8", errors="replace").strip()
        raise PackageBuildError(f"read generated public key: {detail}")
    return ed25519_public_key_from_der(result.stdout, "generated public key")


def load_official_public_key(path: Path | None, mode: str) -> bytes:
    if path is None:
        raise PackageBuildError(f"{mode} requires --official-public-key")
    if path.is_symlink() or not path.is_file():
        raise PackageBuildError("official public key must be a regular file")
    try:
        encoded = b"".join(path.read_bytes().split())
    except OSError as error:
        raise PackageBuildError(f"read official public key: {error}") from error
    try:
        public_key = base64.b64decode(encoded, validate=True)
    except (ValueError, binascii.Error) as error:
        raise PackageBuildError("official public key must be Base64-encoded raw Ed25519 bytes") from error
    if len(public_key) != ED25519_PUBLIC_KEY_BYTES:
        raise PackageBuildError("official public key must contain 32 raw Ed25519 bytes")
    return public_key


@contextmanager
def signer_for_build(signing_key: Path | None, official_public_key: bytes | None = None) -> Iterator[Signer]:
    if shutil.which("openssl") is None:
        raise PackageBuildError("OpenSSL executable is required for package signing")
    if signing_key is not None:
        if signing_key.is_symlink() or not signing_key.is_file():
            raise PackageBuildError("signing key must be a regular file")
        mode = stat.S_IMODE(signing_key.stat().st_mode)
        if mode & 0o077:
            raise PackageBuildError("signing key must not be readable by group or others")
        public_key = derive_ed25519_public_key(signing_key)
        if official_public_key is not None and not hmac.compare_digest(public_key, official_public_key):
            raise PackageBuildError("signing key does not match official public key")
        yield Signer(private_key=signing_key, public_key=ed25519_public_key_pem(public_key))
        return

    if official_public_key is not None:
        raise PackageBuildError("formal release requires --signing-key")

    # Local builds need a verifiable signature without persisting a private
    # key. Release jobs provide --signing-key and retain their official trust
    # root; this temporary key is intentionally not a release credential.
    with tempfile.TemporaryDirectory(prefix="anixops-package-ephemeral-key-") as temporary:
        private_key = Path(temporary) / "private-key.pem"
        result = subprocess.run(
            ["openssl", "genpkey", "-algorithm", "ED25519", "-out", str(private_key)],
            check=False,
            text=True,
            capture_output=True,
        )
        if result.returncode != 0:
            raise PackageBuildError(f"generate local signing key: {result.stderr.strip() or result.stdout.strip()}")
        os.chmod(private_key, 0o600)
        public_key = derive_ed25519_public_key(private_key)
        yield Signer(private_key=private_key, public_key=ed25519_public_key_pem(public_key))


def sbom_document(spec: PackageSpec, version: str, artifact_name: str, artifact: bytes) -> bytes:
    digest = sha256_bytes(artifact)
    return pretty_json(
        {
            "SPDXID": "SPDXRef-DOCUMENT",
            "creationInfo": {
                "created": "1970-01-01T00:00:00Z",
                "creators": ["Tool: AnixOps Package Builder"],
            },
            "dataLicense": "CC0-1.0",
            "documentNamespace": f"https://anixops.invalid/spdx/{spec.package_id}/{version}/{digest}",
            "files": [
                {
                    "SPDXID": "SPDXRef-PackageArtifact",
                    "checksums": [{"algorithm": "SHA256", "checksumValue": digest}],
                    "fileName": artifact_name,
                }
            ],
            "name": f"anixops-{spec.package_id}-{version}",
            "packages": [
                {
                    "SPDXID": "SPDXRef-Package",
                    "downloadLocation": "NOASSERTION",
                    "filesAnalyzed": True,
                    "name": spec.package_id,
                    "versionInfo": version,
                }
            ],
            "spdxVersion": "SPDX-2.3",
        }
    )


def verify_signature(manifest_path: Path, signature_path: Path, public_key_path: Path) -> None:
    with tempfile.TemporaryDirectory(prefix="anixops-package-verify-") as temporary:
        raw_signature = Path(temporary) / "manifest.sig"
        try:
            raw_signature.write_bytes(base64.b64decode(b"".join(signature_path.read_bytes().split()), validate=True))
        except (OSError, ValueError) as error:
            raise PackageBuildError(f"decode manifest signature: {error}") from error
        result = subprocess.run(
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
                str(raw_signature),
            ],
            check=False,
            text=True,
            capture_output=True,
        )
        if result.returncode != 0:
            raise PackageBuildError(f"verify manifest signature: {result.stderr.strip() or result.stdout.strip()}")


def verify_signature_with_official_root(manifest_path: Path, signature_path: Path, official_public_key: bytes) -> None:
    with tempfile.TemporaryDirectory(prefix="anixops-package-official-root-") as temporary:
        public_key_path = Path(temporary) / "official-public-key.pem"
        public_key_path.write_bytes(ed25519_public_key_pem(official_public_key))
        verify_signature(manifest_path, signature_path, public_key_path)


def verify_output(
    output: Path,
    spec: PackageSpec,
    version: str,
    official_public_key: bytes | None = None,
    formal_release: bool = False,
) -> None:
    stem = f"{spec.package_id}-{version}"
    artifact_path = output / f"{stem}.anxp"
    manifest_path = output / f"{stem}.manifest.json"
    signature_path = output / f"{stem}.manifest.sig"
    public_key_path = output / f"{stem}.public-key.pem"
    sbom_path = output / f"{stem}.sbom.spdx.json"
    required = (artifact_path, manifest_path, signature_path, public_key_path, sbom_path)
    if any(not path.is_file() for path in required):
        raise PackageBuildError(f"{spec.package_id} generated output is incomplete")
    manifest = load_json(manifest_path, f"{spec.package_id} generated manifest")
    if not isinstance(manifest, dict):
        raise PackageBuildError("generated manifest must be an object")
    validate_manifest_value(manifest, spec)
    if manifest.get("id") != spec.package_id or manifest.get("version") != version:
        raise PackageBuildError("generated manifest identity does not match the selected package")
    if formal_release and manifest.get("architectures") != [platform_name(platform) for platform in SUPPORTED_PLATFORMS]:
        raise PackageBuildError("formal release manifest must declare linux/amd64 and linux/arm64")
    if canonical_json(manifest) != manifest_path.read_bytes():
        raise PackageBuildError("generated manifest is not canonical")
    artifact = artifact_path.read_bytes()
    if sha256_bytes(artifact) != manifest["artifact_sha256"]:
        raise PackageBuildError("generated artifact digest does not match manifest")
    if official_public_key is None:
        verify_signature(manifest_path, signature_path, public_key_path)
    else:
        # The configured raw root is the authority. The emitted PEM is checked
        # for consistency only after signature verification against that root.
        verify_signature_with_official_root(manifest_path, signature_path, official_public_key)
        emitted_public_key = derive_ed25519_public_key_from_pem(public_key_path)
        if not hmac.compare_digest(emitted_public_key, official_public_key):
            raise PackageBuildError("generated public key does not match official public key")
    try:
        archive_mode = "r:gz" if artifact.startswith(b"\x1f\x8b") else "r:"
        with tarfile.open(fileobj=io.BytesIO(artifact), mode=archive_mode) as archive:
            members = archive.getmembers()
            names = [member.name for member in members]
            if names != sorted(names) or len(names) != len(set(names)):
                raise PackageBuildError("artifact entries must be unique and sorted")
            for member in members:
                if not member.isfile() or member.mtime != 0 or member.uid != 0 or member.gid != 0:
                    raise PackageBuildError("artifact metadata is not deterministic")
            for runtime_name in spec.runtimes:
                for platform in (parse_platform(value) for value in manifest["architectures"]):
                    entrypoint = runtime_archive_path(runtime_name, platform)
                    member = archive.getmember(entrypoint)
                    if member.mode & 0o777 != 0o755:
                        raise PackageBuildError(f"{spec.package_id} runtime {entrypoint} must be executable")
                    handle = archive.extractfile(member)
                    if handle is None:
                        raise PackageBuildError(f"{spec.package_id} runtime {entrypoint} cannot be read")
                    runtime_binary = handle.read(MAX_RUNTIME_BINARY_BYTES + 1)
                    if len(runtime_binary) > MAX_RUNTIME_BINARY_BYTES:
                        raise PackageBuildError(f"{spec.package_id} runtime {entrypoint} exceeds {MAX_RUNTIME_BINARY_BYTES} bytes")
                    if formal_release:
                        expected = runtime_binary_contract(spec.package_id, runtime_name, platform)["binary_sha256"]
                        if not hmac.compare_digest(sha256_bytes(runtime_binary), expected):
                            raise PackageBuildError(
                                f"{spec.package_id} runtime {entrypoint} does not match the pinned runtime contract"
                            )
    except (tarfile.TarError, KeyError) as error:
        raise PackageBuildError(f"generated artifact is not a tar archive: {error}") from error
    sbom = load_json(sbom_path, f"{spec.package_id} SBOM")
    if not isinstance(sbom, dict) or manifest["artifact_sha256"] not in json.dumps(sbom, sort_keys=True):
        raise PackageBuildError("generated SBOM does not bind the final artifact digest")


def build_package(
    spec: PackageSpec,
    version: str,
    output: Path,
    signer: Signer,
    platforms: tuple[tuple[str, str], ...],
    agent_inputs: dict[tuple[str, tuple[str, str]], Path],
    runtime_inputs: dict[tuple[str, str, tuple[str, str]], Path],
    official_public_key: bytes | None = None,
    formal_release: bool = False,
) -> None:
    entries, contents = package_entries(
        spec,
        version,
        platforms,
        agent_inputs,
        runtime_inputs,
        formal_release=formal_release,
    )
    # Archive order and bytes are fixed before any generated digest is inserted
    # into the signed manifest.
    artifact = deterministic_artifact(entries)
    if len(artifact) > MAX_ARTIFACT_BYTES:
        raise PackageBuildError(f"{spec.package_id} artifact exceeds {MAX_ARTIFACT_BYTES} bytes")
    manifest = generated_manifest(spec, version, platforms, artifact, contents)
    manifest_bytes = canonical_json(manifest)
    signature = base64.b64encode(signer.sign(manifest_bytes)) + b"\n"
    stem = f"{spec.package_id}-{version}"
    artifact_name = f"{stem}.anxp"
    generated = {
        artifact_name: artifact,
        f"{stem}.manifest.json": manifest_bytes,
        f"{stem}.manifest.sig": signature,
        f"{stem}.public-key.pem": signer.public_key,
        f"{stem}.sbom.spdx.json": sbom_document(spec, version, artifact_name, artifact),
    }
    for name, data in generated.items():
        atomic_write(output / name, data, 0o644)
    verify_output(output, spec, version, official_public_key, formal_release)


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    scope = parser.add_mutually_exclusive_group(required=True)
    scope.add_argument("--all", action="store_true", help="Build every v4 package in the release-stage matrix.")
    scope.add_argument("--package", choices=[spec.package_id for spec in PACKAGE_SPECS], help="Build one package.")
    parser.add_argument("--version", required=True, help="Stable package version applied to every selected artifact.")
    parser.add_argument("--out", required=True, type=Path, help="Directory for .anxp, manifest, signature, and SBOM files.")
    parser.add_argument("--goos", default="linux", help="Legacy single-platform GOOS selector (default: linux).")
    parser.add_argument("--goarch", default="amd64", help="Legacy single-platform GOARCH selector (default: amd64).")
    parser.add_argument(
        "--platform",
        action="append",
        default=[],
        help="Package platform GOOS/GOARCH. Repeat for a multiarch package; supports linux/amd64 and linux/arm64.",
    )
    parser.add_argument(
        "--agent-binary",
        action="append",
        default=[],
        help="Real Agent binary input PACKAGE_ID@GOOS/GOARCH=ABSOLUTE_PATH. Repeat for every Agent package/platform.",
    )
    parser.add_argument(
        "--runtime-binary",
        action="append",
        default=[],
        help="Runtime binary input PACKAGE_ID:RUNTIME@GOOS/GOARCH=ABSOLUTE_PATH. Repeat for every declared runtime/platform.",
    )
    parser.add_argument("--signing-key", type=Path, help="PEM Ed25519 key used for an official release signature.")
    release_mode = parser.add_mutually_exclusive_group()
    release_mode.add_argument(
        "--formal-release",
        action="store_true",
        help="Require the signing key to match --official-public-key and verify each output against that configured root.",
    )
    release_mode.add_argument(
        "--verify-release",
        action="store_true",
        help="Verify existing selected artifacts against --official-public-key without building new artifacts.",
    )
    parser.add_argument(
        "--official-public-key",
        type=Path,
        help="File containing the Base64 raw Ed25519 root configured as plugins.official_public_key.",
    )
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    try:
        version = require_safe_segment(args.version, "version")
        selected = PACKAGE_SPECS if args.all else tuple(spec for spec in PACKAGE_SPECS if spec.package_id == args.package)
        if args.platform and (args.goos != "linux" or args.goarch != "amd64"):
            raise PackageBuildError("--platform cannot be combined with --goos or --goarch")
        platforms = normalize_platforms(args.platform or [args.goos + "/" + args.goarch])
        agent_inputs = parse_agent_binary_inputs(args.agent_binary)
        runtime_inputs = parse_runtime_binary_inputs(args.runtime_binary)
        selected_ids = {spec.package_id for spec in selected}
        for package_id, platform in agent_inputs:
            if package_id not in selected_ids:
                raise PackageBuildError(f"agent binary input {package_id}@{platform_name(platform)} is outside the selected package scope")
            if platform not in platforms:
                raise PackageBuildError(f"agent binary input {package_id}@{platform_name(platform)} is outside the selected platform scope")
        for package_id, runtime_name, platform in runtime_inputs:
            if package_id not in selected_ids:
                raise PackageBuildError(
                    f"runtime binary input {package_id}:{runtime_name}@{platform_name(platform)} is outside the selected package scope"
                )
            if platform not in platforms:
                raise PackageBuildError(
                    f"runtime binary input {package_id}:{runtime_name}@{platform_name(platform)} is outside the selected platform scope"
                )
        if args.official_public_key is not None and not (args.formal_release or args.verify_release):
            raise PackageBuildError("--official-public-key requires --formal-release or --verify-release")
        if args.verify_release:
            if args.signing_key is not None:
                raise PackageBuildError("release artifact verification does not accept --signing-key")
            official_public_key = load_official_public_key(args.official_public_key, "release artifact verification")
            for spec in selected:
                verify_output(args.out, spec, version, official_public_key, formal_release=True)
            print(f"verified {len(selected)} release package artifacts against the configured official root in {args.out}")
            return 0

        official_public_key = None
        if args.formal_release:
            if args.signing_key is None:
                raise PackageBuildError("formal release requires --signing-key")
            official_public_key = load_official_public_key(args.official_public_key, "formal release")
        with signer_for_build(args.signing_key, official_public_key) as signer:
            if args.formal_release and platforms != SUPPORTED_PLATFORMS:
                raise PackageBuildError("formal release requires linux/amd64 and linux/arm64 package platforms")
            for spec in selected:
                build_package(
                    spec,
                    version,
                    args.out,
                    signer,
                    platforms,
                    agent_inputs,
                    runtime_inputs,
                    official_public_key,
                    args.formal_release,
                )
    except PackageBuildError as error:
        print(f"build_package.py: error: {error}", file=os.sys.stderr)
        return 1
    print(f"built {len(selected)} signed package artifacts and {len(selected)} SBOM files in {args.out}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
