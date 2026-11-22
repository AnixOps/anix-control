#!/usr/bin/env python3
"""Build deterministic signed AnixOps v2 package artifacts.

The packages are the ones Control embeds (internal/builtinpkg). A package
holds no binary: the signed manifest's control_entrypoint is
{"builtin": "<id>"}, the host linked into the Control binary
(packages/builtin), it targets only "control", and index.json lists the set.
"""

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
import re
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
ED25519_PUBLIC_KEY_BYTES = 32
ED25519_SPKI_PREFIX = bytes.fromhex("302a300506032b6570032100")
SAFE_SEGMENT_CHARACTERS = frozenset("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789._+-")
MIGRATION_VERSION_TOKEN = "__ANIXOPS_PACKAGE_VERSION__"
MIGRATION_ID_PATTERN = re.compile(r"[0-9a-z][0-9a-z_]{0,63}")
BUILTIN_INDEX_FORMAT = "anixops.builtin-packages/v1"
BUILTIN_INDEX_NAME = "index.json"
BUILTIN_PUBLIC_KEY_NAME = "official-public-key.pem"
BUILTIN_TARGETS = ("control",)
SUPPORTED_PLATFORMS = (("linux", "amd64"), ("linux", "arm64"))


@dataclass(frozen=True)
class PackageSpec:
    package_id: str
    # targets are the ones the package's manifest template declares; the
    # built package targets only "control" (BUILTIN_TARGETS).
    targets: tuple[str, ...]


_ALL_PACKAGE_SPECS = (
    PackageSpec("identity-platform", ("control",)),
    PackageSpec("platform", ("control",)),
    PackageSpec("affiliate", ("control",)),
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
    PackageSpec("wireguard", ("control",)),
    PackageSpec("protocol-runtime", ("control",)),
)

# The commercial packages (config/commercial-boundary.json) live in the
# private anix-control-commercial repository and are overlaid onto this tree
# for the commercial build. A community checkout has no directory for them, so
# a package is specified only when its manifest template is present.
COMMERCIAL_PACKAGE_IDS = frozenset({"affiliate", "order", "payment"})

PACKAGE_SPECS = tuple(
    spec
    for spec in _ALL_PACKAGE_SPECS
    if spec.package_id not in COMMERCIAL_PACKAGE_IDS
    or (PACKAGES_ROOT / spec.package_id / "manifest.template.json").is_file()
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
    "control_entrypoint",
    "agent_entrypoint",
    "migrations",
    "compatibility_routes",
    "route_contract_digest",
    "runtime_api_version",
)
# No package ships an administrator WebUI: every manifest keeps
# `frontend_sha256` empty and has no `webui` key. The kernel and the Agent
# encode that manifest as `"frontend_sha256":""`, the byte form this builder
# signs.


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


def manifest_template(package_id: str) -> dict[str, Any]:
    template = load_json(package_root(package_id) / "manifest.template.json", f"{package_id} manifest template")
    if not isinstance(template, dict):
        raise PackageBuildError(f"{package_id} manifest template must be a JSON object")
    return template


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


def source_compatibility_routes(package_id: str) -> bytes:
    source = package_root(package_id) / "compat" / "v2-routes.json"
    if not source.is_file():
        return generated_compatibility_routes(package_id)
    routes = load_json(source, f"{package_id} compatibility routes")
    if not isinstance(routes, dict) or routes.get("api_version") != "v2" or routes.get("package_id") != package_id or not isinstance(routes.get("routes"), list):
        raise PackageBuildError(f"{package_id} compatibility routes are invalid")
    return read_source(source, f"{package_id} compatibility routes")


def source_migrations(package_id: str, version: str) -> tuple[bytes, list[ArchiveEntry]]:
    """Materialize the migration index for one release.

    The source index names the package and lists ordered steps; its version is
    the __ANIXOPS_PACKAGE_VERSION__ token (or the build version itself). The
    packaged index records the build version and the SHA-256 of every step
    script, so the kernel can verify each step it runs.
    """
    source_root = package_root(package_id)
    index_path = source_root / "migrations" / "index.json"
    if not index_path.is_file():
        return generated_migrations_index(package_id, version), []
    index = load_json(index_path, f"{package_id} migrations index")
    if (
        not isinstance(index, dict)
        or set(index) != {"format", "migrations", "package_id", "version"}
        or index.get("format") != "anixops.migrations/v1"
        or index.get("package_id") != package_id
        or index.get("version") not in (MIGRATION_VERSION_TOKEN, version)
        or not isinstance(index.get("migrations"), list)
    ):
        raise PackageBuildError(f"{package_id} migrations index is invalid")
    entries: list[ArchiveEntry] = []
    packaged: list[dict[str, str]] = []
    seen_paths: set[str] = set()
    seen_ids: set[str] = set()
    for migration in index["migrations"]:
        if not isinstance(migration, dict) or set(migration) != {"id", "path"} or not isinstance(migration.get("id"), str):
            raise PackageBuildError(f"{package_id} migration entry is invalid")
        migration_id = migration["id"]
        if not MIGRATION_ID_PATTERN.fullmatch(migration_id) or migration_id in seen_ids:
            raise PackageBuildError(f"{package_id} migration id {migration_id!r} is invalid or duplicated")
        relative_path = require_relative_path(migration.get("path"), f"{package_id} migration")
        if not relative_path.startswith("migrations/") or relative_path == "migrations/index.json" or relative_path in seen_paths:
            raise PackageBuildError(f"{package_id} migration path is invalid")
        seen_ids.add(migration_id)
        seen_paths.add(relative_path)
        script = read_source(source_root / relative_path, f"{package_id} {relative_path}")
        entries.append(ArchiveEntry(relative_path, script, 0o644))
        packaged.append({"id": migration_id, "path": relative_path, "sha256": hashlib.sha256(script).hexdigest()})
    materialized = pretty_json(
        {
            "format": "anixops.migrations/v1",
            "migrations": packaged,
            "package_id": package_id,
            "version": version,
        }
    )
    return materialized, entries


def package_entries(spec: PackageSpec, version: str) -> tuple[list[ArchiveEntry], dict[str, Any]]:
    migrations, migration_entries = source_migrations(spec.package_id, version)
    routes = source_compatibility_routes(spec.package_id)
    entries = [
        ArchiveEntry("compat/v2-routes.json", routes, 0o644),
        ArchiveEntry("migrations/index.json", migrations, 0o644),
    ]
    entries.extend(migration_entries)
    source_root = package_root(spec.package_id)
    for relative_path in ("config.schema.json", "config.defaults.json"):
        source = source_root / relative_path
        if source.is_file():
            entries.append(ArchiveEntry(relative_path, read_source(source, f"{spec.package_id} {relative_path}"), 0o644))

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
        "targets": list(BUILTIN_TARGETS),
        "version": version,
    }
    entries.append(ArchiveEntry("package.json", pretty_json(package_index), 0o644))
    entries.sort(key=lambda entry: entry.path)
    return entries, {"migrations": migrations, "routes": routes}


def ordered_manifest(template: Any) -> dict[str, Any]:
    if not isinstance(template, dict):
        raise PackageBuildError("manifest template must be a JSON object")
    missing = [field for field in MANIFEST_FIELDS if field not in template]
    unknown = sorted(set(template) - set(MANIFEST_FIELDS))
    if missing or unknown:
        raise PackageBuildError(f"manifest template fields are invalid: missing={missing}, unknown={unknown}")
    return {field: template[field] for field in MANIFEST_FIELDS}


# A Control package declares no Agent runtime metadata.
EMITTED_MANIFEST_FIELDS = tuple(field for field in MANIFEST_FIELDS if field not in {"agent_entrypoint", "runtime_api_version"})


def generated_manifest(spec: PackageSpec, version: str, artifact: bytes, contents: dict[str, Any]) -> dict[str, Any]:
    manifest = ordered_manifest(manifest_template(spec.package_id))
    if manifest["id"] != spec.package_id:
        raise PackageBuildError(f"{spec.package_id} manifest template identity is invalid")
    if tuple(manifest["targets"]) != spec.targets:
        raise PackageBuildError(f"{spec.package_id} manifest targets do not match the package matrix")

    if manifest["frontend_sha256"] != "":
        raise PackageBuildError(f"{spec.package_id} manifest must keep frontend_sha256 empty")

    manifest["version"] = version
    manifest["api_version"] = MANIFEST_API_VERSION
    # The host is the Control binary, released for these platforms; the
    # package itself holds no binary.
    manifest["targets"] = list(BUILTIN_TARGETS)
    manifest["architectures"] = [platform_name(platform) for platform in SUPPORTED_PLATFORMS]
    manifest["artifact_sha256"] = sha256_bytes(artifact)
    manifest["config_schema"] = sorted_json_value(manifest["config_schema"])
    manifest["entrypoints"] = {}
    manifest["migration_version"] = 0
    manifest["control_entrypoint"] = {"builtin": spec.package_id}
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
    del manifest["agent_entrypoint"]
    del manifest["runtime_api_version"]
    validate_manifest_value(manifest, spec)
    return manifest


def valid_digest(value: Any) -> bool:
    return isinstance(value, str) and len(value) == 64 and all(char in "0123456789abcdef" for char in value)


def validate_manifest_value(manifest: dict[str, Any], spec: PackageSpec) -> None:
    if tuple(manifest) != EMITTED_MANIFEST_FIELDS:
        raise PackageBuildError("generated manifest field order is not canonical")
    if manifest.get("frontend_sha256") != "":
        raise PackageBuildError("generated manifest must keep frontend_sha256 empty")
    if manifest.get("api_version") != MANIFEST_API_VERSION:
        raise PackageBuildError("generated manifest API version is invalid")
    if manifest.get("publisher") != "AnixOps":
        raise PackageBuildError("generated manifest publisher is invalid")
    if tuple(manifest.get("targets", [])) != BUILTIN_TARGETS:
        raise PackageBuildError("generated manifest must target only control")
    if manifest.get("architectures") != [platform_name(platform) for platform in SUPPORTED_PLATFORMS]:
        raise PackageBuildError("generated manifest must declare linux/amd64 and linux/arm64")
    if manifest.get("entrypoints") != {}:
        raise PackageBuildError("generated manifest runtime entrypoints are invalid")
    if manifest.get("control_entrypoint") != {"builtin": spec.package_id}:
        raise PackageBuildError("generated manifest must name its built-in host")
    digests = [
        manifest.get("artifact_sha256"),
        manifest.get("route_contract_digest"),
        manifest.get("migrations", {}).get("sha256") if isinstance(manifest.get("migrations"), dict) else None,
        manifest.get("compatibility_routes", {}).get("sha256") if isinstance(manifest.get("compatibility_routes"), dict) else None,
    ]
    for value in digests:
        if not valid_digest(value):
            raise PackageBuildError("generated manifest digest is invalid")
    for key, field in (("migrations", "index"), ("compatibility_routes", "path")):
        value = manifest.get(key)
        if not isinstance(value, dict):
            raise PackageBuildError(f"generated manifest {key} is required")
        require_relative_path(value.get(field), f"generated manifest {key}")


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
            manifest_path = Path(temporary) / "manifest.json"
            manifest_path.write_bytes(manifest)
            return self.sign_file(manifest_path, "canonical manifest")

    def sign_file(self, path: Path, label: str) -> bytes:
        """Return the raw Ed25519 signature over the file's exact bytes."""
        with tempfile.TemporaryDirectory(prefix="anixops-package-sign-") as temporary:
            signature_path = Path(temporary) / "signature.bin"
            result = subprocess.run(
                [
                    "openssl",
                    "pkeyutl",
                    "-sign",
                    "-inkey",
                    str(self.private_key),
                    "-rawin",
                    "-in",
                    str(path),
                    "-out",
                    str(signature_path),
                ],
                check=False,
                text=True,
                capture_output=True,
            )
            if result.returncode != 0:
                raise PackageBuildError(f"sign {label}: {result.stderr.strip() or result.stdout.strip()}")
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


def verify_signature(
    manifest_path: Path, signature_path: Path, public_key_path: Path, label: str = "manifest signature"
) -> None:
    with tempfile.TemporaryDirectory(prefix="anixops-package-verify-") as temporary:
        raw_signature = Path(temporary) / "signature.bin"
        try:
            raw_signature.write_bytes(base64.b64decode(b"".join(signature_path.read_bytes().split()), validate=True))
        except (OSError, ValueError) as error:
            raise PackageBuildError(f"decode {label}: {error}") from error
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
            raise PackageBuildError(f"verify {label}: {result.stderr.strip() or result.stdout.strip()}")


def verify_signature_with_official_root(
    manifest_path: Path, signature_path: Path, official_public_key: bytes, label: str = "manifest signature"
) -> None:
    with tempfile.TemporaryDirectory(prefix="anixops-package-official-root-") as temporary:
        public_key_path = Path(temporary) / "official-public-key.pem"
        public_key_path.write_bytes(ed25519_public_key_pem(official_public_key))
        verify_signature(manifest_path, signature_path, public_key_path, label)


def verify_output(
    output: Path,
    spec: PackageSpec,
    version: str,
    official_public_key: bytes | None = None,
    formal_release: bool = False,
    require_public_key: bool = True,
) -> None:
    stem = f"{spec.package_id}-{version}"
    artifact_path = output / f"{stem}.anxp"
    manifest_path = output / f"{stem}.manifest.json"
    signature_path = output / f"{stem}.manifest.sig"
    public_key_path = output / f"{stem}.public-key.pem"
    sbom_path = output / f"{stem}.sbom.spdx.json"
    required = (artifact_path, manifest_path, signature_path, sbom_path)
    if require_public_key or official_public_key is None:
        required += (public_key_path,)
    if any(not path.is_file() for path in required):
        raise PackageBuildError(f"{spec.package_id} generated output is incomplete")
    manifest = load_json(manifest_path, f"{spec.package_id} generated manifest")
    if not isinstance(manifest, dict):
        raise PackageBuildError("generated manifest must be an object")
    validate_manifest_value(manifest, spec)
    if manifest.get("id") != spec.package_id or manifest.get("version") != version:
        raise PackageBuildError("generated manifest identity does not match the selected package")
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
        if public_key_path.is_file():
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
            binaries = [name for name in names if name.startswith(("bin/", "agent/", "runtime/"))]
            if binaries:
                raise PackageBuildError(f"{spec.package_id} artifact must hold no binary: {', '.join(binaries)}")
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
    official_public_key: bytes | None = None,
    formal_release: bool = False,
) -> None:
    entries, contents = package_entries(spec, version)
    # Archive order and bytes are fixed before any generated digest is inserted
    # into the signed manifest.
    artifact = deterministic_artifact(entries)
    if len(artifact) > MAX_ARTIFACT_BYTES:
        raise PackageBuildError(f"{spec.package_id} artifact exceeds {MAX_ARTIFACT_BYTES} bytes")
    manifest = generated_manifest(spec, version, artifact, contents)
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


# The built-in set: the slim packages Control embeds (internal/builtinpkg)
# and index.json, which lists them for the kernel's start-up bootstrap.
def builtin_index(selected: tuple[PackageSpec, ...], version: str) -> bytes:
    packages = [
        {
            "artifact": f"{spec.package_id}-{version}.anxp",
            "id": spec.package_id,
            "manifest": f"{spec.package_id}-{version}.manifest.json",
            "signature": f"{spec.package_id}-{version}.manifest.sig",
            "version": version,
        }
        for spec in sorted(selected, key=lambda item: item.package_id)
    ]
    return pretty_json({"format": BUILTIN_INDEX_FORMAT, "packages": packages, "public_key": BUILTIN_PUBLIC_KEY_NAME})


def write_builtin_index(output: Path, selected: tuple[PackageSpec, ...], version: str, signer: Signer) -> None:
    atomic_write(output / BUILTIN_PUBLIC_KEY_NAME, signer.public_key, 0o644)
    atomic_write(output / BUILTIN_INDEX_NAME, builtin_index(selected, version), 0o644)


def verify_builtin_set(
    output: Path,
    selected: tuple[PackageSpec, ...],
    version: str,
    official_public_key: bytes | None = None,
    formal_release: bool = False,
) -> None:
    """Check index.json against the selected packages and verify every one."""
    index_path = output / BUILTIN_INDEX_NAME
    if index_path.is_symlink() or not index_path.is_file():
        raise PackageBuildError(f"built-in package index is missing: {index_path}")
    if index_path.read_bytes() != builtin_index(selected, version):
        raise PackageBuildError("built-in package index does not list exactly the selected packages")
    for spec in selected:
        verify_output(output, spec, version, official_public_key, formal_release)
    public_key_path = output / BUILTIN_PUBLIC_KEY_NAME
    if public_key_path.is_symlink() or not public_key_path.is_file():
        raise PackageBuildError("built-in package set has no public key")
    if official_public_key is not None and not hmac.compare_digest(
        derive_ed25519_public_key_from_pem(public_key_path), official_public_key
    ):
        raise PackageBuildError("built-in package public key does not match the official public key")


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    scope = parser.add_mutually_exclusive_group(required=True)
    scope.add_argument("--all", action="store_true", help="Build every official package.")
    scope.add_argument(
        "--package",
        action="append",
        choices=[spec.package_id for spec in PACKAGE_SPECS],
        help="Build one package; repeat for several (the index lists them).",
    )
    parser.add_argument("--version", required=True, help="Stable package version applied to every selected artifact.")
    parser.add_argument(
        "--out",
        type=Path,
        required=True,
        help="Directory for .anxp, manifest, signature, and SBOM files and index.json.",
    )
    parser.add_argument(
        "--builtin",
        action="store_true",
        help="Accepted for existing invocations: every package is the built-in format.",
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
        if args.package and len(set(args.package)) != len(args.package):
            raise PackageBuildError("--package names a package twice")
        selected = PACKAGE_SPECS if args.all else tuple(spec for spec in PACKAGE_SPECS if spec.package_id in args.package)
        if args.official_public_key is not None and not (args.formal_release or args.verify_release):
            raise PackageBuildError("--official-public-key requires --formal-release or --verify-release")
        if args.verify_release:
            if args.signing_key is not None:
                raise PackageBuildError("release artifact verification does not accept --signing-key")
            official_public_key = load_official_public_key(args.official_public_key, "release artifact verification")
            verify_builtin_set(args.out, selected, version, official_public_key, formal_release=True)
            print(f"verified {len(selected)} built-in packages and their index against the configured official root in {args.out}")
            return 0

        official_public_key = None
        if args.formal_release:
            if args.signing_key is None:
                raise PackageBuildError("formal release requires --signing-key")
            official_public_key = load_official_public_key(args.official_public_key, "formal release")
        with signer_for_build(args.signing_key, official_public_key) as signer:
            for spec in selected:
                build_package(spec, version, args.out, signer, official_public_key, args.formal_release)
            write_builtin_index(args.out, selected, version, signer)
            verify_builtin_set(args.out, selected, version, official_public_key, args.formal_release)
    except PackageBuildError as error:
        print(f"build_package.py: error: {error}", file=os.sys.stderr)
        return 1
    print(f"built {len(selected)} signed built-in package artifacts, their index and {len(selected)} SBOM files in {args.out}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
