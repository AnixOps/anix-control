#!/usr/bin/env python3
"""Build deterministic signed AnixOps v2 package artifacts."""

from __future__ import annotations

import argparse
import base64
import hashlib
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
MAX_ARTIFACT_BYTES = 32 << 20


@dataclass(frozen=True)
class PackageSpec:
    package_id: str
    targets: tuple[str, ...]


PACKAGE_SPECS = (
    PackageSpec("subscription", ("control",)),
    PackageSpec("proxy-node", ("control", "agent")),
    PackageSpec("plan", ("control",)),
    PackageSpec("order", ("control",)),
    PackageSpec("payment", ("control",)),
    PackageSpec("forward", ("control", "agent")),
    PackageSpec("ticket", ("control",)),
    PackageSpec("notification", ("control",)),
    PackageSpec("knowledge", ("control",)),
    PackageSpec("machine-telemetry", ("control", "agent")),
    PackageSpec("nftables-forward", ("control", "agent")),
    PackageSpec("gost-mesh", ("control", "agent")),
    PackageSpec("nat-egress", ("control", "agent")),
    PackageSpec("wireguard", ("control", "agent")),
    PackageSpec("protocol-runtime", ("control", "agent")),
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
    encoded = json.dumps(value, ensure_ascii=True, separators=(",", ":"))
    # encoding/json escapes these values, and manifests are signed by Go as
    # well as by this builder.
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


def package_root(package_id: str) -> Path:
    return PACKAGES_ROOT / package_id


def source_webui(package_id: str) -> bytes:
    return read_source(package_root(package_id) / "webui" / "index.mjs", f"{package_id} WebUI bundle")


def generated_control_host(package_id: str) -> bytes:
    return (
        "#!/bin/sh\n"
        f"printf '%s\\n' 'AnixOps package {package_id} host is not implemented in the manifest stage' >&2\n"
        "exit 78\n"
    ).encode("utf-8")


def generated_agent_entrypoint(package_id: str) -> bytes:
    return (
        "#!/bin/sh\n"
        f"printf '%s\\n' 'AnixOps package {package_id} agent runtime is not implemented in the manifest stage' >&2\n"
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


def package_entries(spec: PackageSpec, version: str, goos: str, goarch: str) -> tuple[list[ArchiveEntry], dict[str, bytes]]:
    control_host = generated_control_host(spec.package_id)
    migrations = generated_migrations_index(spec.package_id, version)
    routes = generated_compatibility_routes(spec.package_id)
    webui = source_webui(spec.package_id)
    entries = [
        ArchiveEntry("bin/control-host", control_host, 0o755),
        ArchiveEntry("compat/v2-routes.json", routes, 0o644),
        ArchiveEntry("migrations/index.json", migrations, 0o644),
        ArchiveEntry("webui/index.mjs", webui, 0o644),
    ]
    source_root = package_root(spec.package_id)
    for relative_path in ("config.schema.json", "config.defaults.json"):
        source = source_root / relative_path
        if source.is_file():
            entries.append(ArchiveEntry(relative_path, read_source(source, f"{spec.package_id} {relative_path}"), 0o644))

    agent_path = ""
    agent = b""
    if "agent" in spec.targets:
        agent_path = f"agent/{goos}-{goarch}/plugin"
        agent = generated_agent_entrypoint(spec.package_id)
        entries.append(ArchiveEntry(agent_path, agent, 0o755))

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
        "agent": agent,
        "agent_path": agent_path.encode("utf-8"),
        "control_host": control_host,
        "migrations": migrations,
        "routes": routes,
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
    goos: str,
    goarch: str,
    artifact: bytes,
    contents: dict[str, bytes],
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

    architecture = f"{goos}/{goarch}"
    manifest["version"] = version
    manifest["api_version"] = MANIFEST_API_VERSION
    manifest["architectures"] = [architecture]
    manifest["artifact_sha256"] = sha256_bytes(artifact)
    manifest["config_schema"] = sorted_json_value(manifest["config_schema"])
    manifest["entrypoints"] = {}
    manifest["migration_version"] = 0
    manifest["control_routes"] = []
    manifest["frontend_sha256"] = sha256_bytes(contents["webui"])
    webui["bundle"]["sha256"] = manifest["frontend_sha256"]
    manifest["control_entrypoint"] = {
        "path": "bin/control-host",
        "sha256": sha256_bytes(contents["control_host"]),
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
            "path": contents["agent_path"].decode("utf-8"),
            "sha256": sha256_bytes(contents["agent"]),
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


def load_public_key(private_key: Path) -> bytes:
    with tempfile.TemporaryDirectory(prefix="anixops-package-public-key-") as temporary:
        public_key = Path(temporary) / "public-key.pem"
        result = subprocess.run(
            ["openssl", "pkey", "-in", str(private_key), "-pubout", "-out", str(public_key)],
            check=False,
            text=True,
            capture_output=True,
        )
        if result.returncode != 0:
            raise PackageBuildError(f"derive signing public key: {result.stderr.strip() or result.stdout.strip()}")
        return public_key.read_bytes()


@contextmanager
def signer_for_build(signing_key: Path | None) -> Iterator[Signer]:
    if shutil.which("openssl") is None:
        raise PackageBuildError("OpenSSL executable is required for package signing")
    if signing_key is not None:
        if signing_key.is_symlink() or not signing_key.is_file():
            raise PackageBuildError("signing key must be a regular file")
        mode = stat.S_IMODE(signing_key.stat().st_mode)
        if mode & 0o077:
            raise PackageBuildError("signing key must not be readable by group or others")
        yield Signer(private_key=signing_key, public_key=load_public_key(signing_key))
        return

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
        yield Signer(private_key=private_key, public_key=load_public_key(private_key))


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


def verify_output(output: Path, spec: PackageSpec, version: str) -> None:
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
    if canonical_json(manifest) != manifest_path.read_bytes():
        raise PackageBuildError("generated manifest is not canonical")
    if sha256_bytes(artifact_path.read_bytes()) != manifest["artifact_sha256"]:
        raise PackageBuildError("generated artifact digest does not match manifest")
    verify_signature(manifest_path, signature_path, public_key_path)
    try:
        with tarfile.open(artifact_path, mode="r:") as archive:
            members = archive.getmembers()
            names = [member.name for member in members]
            if names != sorted(names) or len(names) != len(set(names)):
                raise PackageBuildError("artifact entries must be unique and sorted")
            for member in members:
                if not member.isfile() or member.mtime != 0 or member.uid != 0 or member.gid != 0:
                    raise PackageBuildError("artifact metadata is not deterministic")
    except tarfile.TarError as error:
        raise PackageBuildError(f"generated artifact is not a tar archive: {error}") from error
    sbom = load_json(sbom_path, f"{spec.package_id} SBOM")
    if not isinstance(sbom, dict) or manifest["artifact_sha256"] not in json.dumps(sbom, sort_keys=True):
        raise PackageBuildError("generated SBOM does not bind the final artifact digest")


def build_package(spec: PackageSpec, version: str, output: Path, signer: Signer, goos: str, goarch: str) -> None:
    entries, contents = package_entries(spec, version, goos, goarch)
    # Archive order and bytes are fixed before any generated digest is inserted
    # into the signed manifest.
    artifact = deterministic_tar(entries)
    if len(artifact) > MAX_ARTIFACT_BYTES:
        raise PackageBuildError(f"{spec.package_id} artifact exceeds {MAX_ARTIFACT_BYTES} bytes")
    manifest = generated_manifest(spec, version, goos, goarch, artifact, contents)
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
    verify_output(output, spec, version)


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    scope = parser.add_mutually_exclusive_group(required=True)
    scope.add_argument("--all", action="store_true", help="Build every v4 package in the release-stage matrix.")
    scope.add_argument("--package", choices=[spec.package_id for spec in PACKAGE_SPECS], help="Build one package.")
    parser.add_argument("--version", required=True, help="Stable package version applied to every selected artifact.")
    parser.add_argument("--out", required=True, type=Path, help="Directory for .anxp, manifest, signature, and SBOM files.")
    parser.add_argument("--goos", default="linux", help="Agent target GOOS (default: linux).")
    parser.add_argument("--goarch", default="amd64", help="Agent target GOARCH (default: amd64).")
    parser.add_argument("--signing-key", type=Path, help="PEM Ed25519 key used for an official release signature.")
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    if not args.version or args.version.strip() != args.version:
        raise PackageBuildError("version is required")
    selected = PACKAGE_SPECS if args.all else tuple(spec for spec in PACKAGE_SPECS if spec.package_id == args.package)
    try:
        with signer_for_build(args.signing_key) as signer:
            for spec in selected:
                build_package(spec, args.version, args.out, signer, args.goos, args.goarch)
    except PackageBuildError as error:
        print(f"build_package.py: error: {error}", file=os.sys.stderr)
        return 1
    print(f"built {len(selected)} signed package artifacts and {len(selected)} SBOM files in {args.out}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
