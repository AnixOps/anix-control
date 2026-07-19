#!/usr/bin/env python3
"""Verify complete formal v4.0.0 package-only release evidence."""

from __future__ import annotations

import argparse
import base64
import gzip
import hashlib
import json
import os
import shutil
import stat
import subprocess
import sys
import tarfile
import tempfile
from pathlib import Path, PurePosixPath
from typing import Any


REPO_ROOT = Path(__file__).resolve().parents[2]
EVIDENCE_SCHEMA = "anixops.v4.release-evidence/v1"
APPROVAL_SCHEMA = "anixops.v4.release-approval/v1"
REQUIRED_TAG = "v4.0.0"
PACKAGE_IDS = (
    "identity-platform",
    "subscription",
    "proxy-node",
    "plan",
    "order",
    "payment",
    "forward",
    "ticket",
    "notification",
    "knowledge",
    "machine-telemetry",
    "nftables-forward",
    "gost-mesh",
    "nat-egress",
    "wireguard",
    "protocol-runtime",
)
EVIDENCE_FIELDS = frozenset(
    {
        "schema",
        "tag",
        "package_artifacts",
        "official_root_verification",
        "release_stage_gate",
        "v2_route_catalog_gate",
        "plugin_only_route_gate",
        "websocket_relay_tests",
        "v2_compatibility_tests",
        "sqlite_rehearsal",
        "postgres_rehearsal",
        "canary_evidence",
        "support_evidence",
    }
)
PACKAGE_FIELDS = frozenset({"id", "artifact", "manifest", "signature", "public_key", "sbom"})
RESULT_FIELDS = frozenset({"passed", "transcript", "transcript_sha256"})
OFFICIAL_ROOT_RESULT_FIELDS = RESULT_FIELDS | {"public_key"}
FILE_EVIDENCE_FIELDS = frozenset({"path", "sha256"})
APPROVAL_FIELDS = frozenset({"schema", "kind", "tag", "release_subject_sha256", "approved", "signature"})
ED25519_PUBLIC_KEY_BYTES = 32
ED25519_SPKI_PREFIX = bytes.fromhex("302a300506032b6570032100")
MAX_ARCHIVE_MEMBERS = 512
# Sixteen formal artifacts may each reach the builder's 64 MiB limit. The
# remaining 64 MiB is reserved for signed manifests, SBOMs, transcripts, and
# approvals while keeping archive expansion bounded.
MAX_ARCHIVE_UNPACKED_BYTES = (len(PACKAGE_IDS) * (64 << 20)) + (64 << 20)
MAX_ARCHIVE_COMPRESSED_BYTES = MAX_ARCHIVE_UNPACKED_BYTES


class EvidenceError(ValueError):
    pass


class BoundedArchiveReader:
    """Cap bytes exposed to tarfile, including hidden PAX/GNU metadata."""

    def __init__(self, source: Any, limit: int) -> None:
        self._source = source
        self._limit = limit
        self._read_bytes = 0

    def read(self, size: int = -1) -> bytes:
        if size == 0:
            return b""
        remaining = self._limit - self._read_bytes
        requested = remaining + 1 if size < 0 else min(size, remaining + 1)
        data = self._source.read(requested)
        self._read_bytes += len(data)
        if self._read_bytes > self._limit:
            raise EvidenceError("evidence archive exceeds the unpacked size limit")
        return data

    def __getattr__(self, name: str) -> Any:
        return getattr(self._source, name)


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def canonical_json(value: Any) -> bytes:
    return json.dumps(value, ensure_ascii=True, separators=(",", ":"), sort_keys=True).encode("utf-8")


def absolute_path(path: Path) -> Path:
    return Path(os.path.abspath(path))


def has_symlink_component(path: Path) -> bool:
    absolute = absolute_path(path)
    current = Path(absolute.anchor)
    for part in absolute.parts[1:]:
        current /= part
        if current.is_symlink():
            return True
    return False


def is_regular_file(path: Path) -> bool:
    return not has_symlink_component(path) and path.is_file()


def require_exact_fields(value: Any, fields: frozenset[str], context: str) -> dict[str, Any]:
    if not isinstance(value, dict):
        raise EvidenceError(f"{context} must be an object")
    actual = frozenset(value)
    missing = sorted(fields - actual)
    extra = sorted(actual - fields)
    if missing or extra:
        details: list[str] = []
        if missing:
            details.append("missing " + ", ".join(missing))
        if extra:
            details.append("unexpected " + ", ".join(extra))
        raise EvidenceError(f"{context} has invalid schema: {'; '.join(details)}")
    return value


def require_digest(value: Any, context: str) -> str:
    if not isinstance(value, str) or len(value) != 64 or any(character not in "0123456789abcdef" for character in value):
        raise EvidenceError(f"{context} must be a lowercase SHA-256 digest")
    return value


def decode_raw_public_key(value: bytes, context: str) -> bytes:
    try:
        public_key = base64.b64decode(b"".join(value.split()), validate=True)
    except ValueError as error:
        raise EvidenceError(f"{context} must be Base64-encoded raw Ed25519 bytes") from error
    if len(public_key) != ED25519_PUBLIC_KEY_BYTES:
        raise EvidenceError(f"{context} must contain 32 raw Ed25519 bytes")
    return public_key


def load_trusted_official_public_key(path: Path) -> bytes:
    if not is_regular_file(path):
        raise EvidenceError("trusted official public key must be a regular file")
    try:
        return decode_raw_public_key(path.read_bytes(), "trusted official public key")
    except OSError as error:
        raise EvidenceError(f"cannot read trusted official public key: {error}") from error


def official_public_key_pem(public_key: bytes) -> bytes:
    if len(public_key) != ED25519_PUBLIC_KEY_BYTES:
        raise EvidenceError("trusted official public key must contain 32 raw Ed25519 bytes")
    encoded = base64.b64encode(ED25519_SPKI_PREFIX + public_key).decode("ascii")
    lines = [encoded[index : index + 64] for index in range(0, len(encoded), 64)]
    return ("-----BEGIN PUBLIC KEY-----\n" + "\n".join(lines) + "\n-----END PUBLIC KEY-----\n").encode("ascii")


def verify_detached_signature(payload: bytes, encoded_signature: Any, public_key: bytes, context: str) -> None:
    if not isinstance(encoded_signature, str) or not encoded_signature:
        raise EvidenceError(f"{context} signature is required")
    try:
        signature = base64.b64decode(b"".join(encoded_signature.encode("ascii").split()), validate=True)
    except (UnicodeEncodeError, ValueError) as error:
        raise EvidenceError(f"{context} signature must be Base64") from error
    if not signature:
        raise EvidenceError(f"{context} signature is empty")
    with tempfile.TemporaryDirectory(prefix="anixops-v4-evidence-signature-") as temporary:
        root = Path(temporary)
        public_key_path = root / "official-public-key.pem"
        payload_path = root / "payload.json"
        signature_path = root / "payload.sig"
        public_key_path.write_bytes(official_public_key_pem(public_key))
        payload_path.write_bytes(payload)
        signature_path.write_bytes(signature)
        try:
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
                    str(payload_path),
                    "-sigfile",
                    str(signature_path),
                ],
                check=False,
                text=True,
                capture_output=True,
            )
        except OSError as error:
            raise EvidenceError(f"{context} signature verification cannot run: {error}") from error
    if result.returncode != 0:
        detail = (result.stderr or result.stdout).strip()
        raise EvidenceError(f"{context} signature verification failed: {detail}")


def sign_detached_signature(payload: bytes, signing_key: Path, context: str) -> bytes:
    if not is_regular_file(signing_key):
        raise EvidenceError(f"{context} signing key must be a regular file")
    if stat.S_IMODE(signing_key.stat().st_mode) & 0o077:
        raise EvidenceError(f"{context} signing key must not be readable by group or others")
    with tempfile.TemporaryDirectory(prefix="anixops-v4-evidence-sign-") as temporary:
        root = Path(temporary)
        payload_path = root / "payload.json"
        signature_path = root / "payload.sig"
        payload_path.write_bytes(payload)
        try:
            result = subprocess.run(
                [
                    "openssl",
                    "pkeyutl",
                    "-sign",
                    "-inkey",
                    str(signing_key),
                    "-rawin",
                    "-in",
                    str(payload_path),
                    "-out",
                    str(signature_path),
                ],
                check=False,
                text=True,
                capture_output=True,
            )
        except OSError as error:
            raise EvidenceError(f"{context} signing cannot run: {error}") from error
        if result.returncode != 0:
            detail = (result.stderr or result.stdout).strip()
            raise EvidenceError(f"{context} signing failed: {detail}")
        try:
            signature = signature_path.read_bytes()
        except OSError as error:
            raise EvidenceError(f"{context} signature output is unavailable: {error}") from error
    if not signature:
        raise EvidenceError(f"{context} signing returned an empty signature")
    return base64.b64encode(signature) + b"\n"


def verify_evidence_signature(evidence_path: Path, signature_path: Path, trusted_official_public_key: bytes) -> None:
    if not is_regular_file(evidence_path) or evidence_path.stat().st_size == 0:
        raise EvidenceError("v4 release evidence is missing or empty")
    if not is_regular_file(signature_path) or signature_path.stat().st_size == 0:
        raise EvidenceError("v4 release evidence signature is missing or empty")
    try:
        verify_detached_signature(
            evidence_path.read_bytes(),
            signature_path.read_text(encoding="ascii"),
            trusted_official_public_key,
            "v4 release evidence",
        )
    except (OSError, UnicodeError) as error:
        raise EvidenceError(f"v4 release evidence signature cannot be read: {error}") from error


def archive_member_path(name: str) -> Path | None:
    if not name or "\\" in name:
        raise EvidenceError("evidence archive member has an unsafe path")
    member_path = PurePosixPath(name)
    if member_path.is_absolute() or ".." in member_path.parts:
        raise EvidenceError("evidence archive member has an unsafe path")
    parts = tuple(part for part in member_path.parts if part not in ("", "."))
    if not parts:
        return None
    return Path(*parts)


def safe_extract_evidence_archive(archive_path: Path, destination: Path) -> None:
    if not is_regular_file(archive_path) or archive_path.stat().st_size == 0:
        raise EvidenceError("v4 release evidence archive is missing or empty")
    if archive_path.stat().st_size > MAX_ARCHIVE_COMPRESSED_BYTES:
        raise EvidenceError("v4 release evidence archive exceeds the compressed size limit")
    if has_symlink_component(destination) or (destination.exists() and not destination.is_dir()):
        raise EvidenceError("evidence archive destination must not be a symlink")
    if destination.exists() and any(destination.iterdir()):
        raise EvidenceError("evidence archive destination must be empty")
    destination.mkdir(parents=True, exist_ok=True)
    root = destination.resolve()
    try:
        with archive_path.open("rb") as compressed, gzip.GzipFile(fileobj=compressed, mode="rb") as expanded:
            limited = BoundedArchiveReader(expanded, MAX_ARCHIVE_UNPACKED_BYTES)
            with tarfile.open(fileobj=limited, mode="r|") as archive:
                seen: set[Path] = set()
                unpacked_bytes = 0
                member_count = 0
                for member in archive:
                    member_count += 1
                    if member_count > MAX_ARCHIVE_MEMBERS:
                        raise EvidenceError("evidence archive has an invalid member count")
                    relative = archive_member_path(member.name)
                    if relative is None:
                        if not member.isdir():
                            raise EvidenceError("evidence archive member has an unsafe path")
                        continue
                    if relative in seen:
                        raise EvidenceError("evidence archive contains duplicate members")
                    seen.add(relative)
                    if not (member.isdir() or member.isfile()):
                        raise EvidenceError("evidence archive contains a non-regular member")
                    if member.isfile():
                        unpacked_bytes += member.size
                        if unpacked_bytes > MAX_ARCHIVE_UNPACKED_BYTES:
                            raise EvidenceError("evidence archive exceeds the unpacked size limit")
                    target = (root / relative).resolve()
                    try:
                        target.relative_to(root)
                    except ValueError as error:
                        raise EvidenceError("evidence archive member escapes the destination") from error
                    if member.isdir():
                        target.mkdir(parents=True, exist_ok=True)
                        continue
                    target.parent.mkdir(parents=True, exist_ok=True)
                    source = archive.extractfile(member)
                    if source is None:
                        raise EvidenceError("evidence archive member cannot be read")
                    with source, target.open("xb") as output:
                        shutil.copyfileobj(source, output, length=1024 * 1024)
                    target.chmod(0o600)
                if member_count == 0:
                    raise EvidenceError("evidence archive has an invalid member count")
    except (EOFError, OSError, tarfile.TarError) as error:
        raise EvidenceError(f"cannot safely unpack v4 release evidence archive: {error}") from error


def resolve_evidence_file(evidence_path: Path, value: Any, context: str) -> Path:
    if not isinstance(value, str) or not value:
        raise EvidenceError(f"{context} path is required")
    relative = Path(value)
    if relative.is_absolute() or ".." in relative.parts or "\\" in value:
        raise EvidenceError(f"{context} path must be relative to the evidence file")
    root = evidence_path.parent.resolve()
    unresolved = root / relative
    if has_symlink_component(unresolved):
        raise EvidenceError(f"{context} path must not contain a symlink")
    candidate = unresolved.resolve()
    try:
        candidate.relative_to(root)
    except ValueError as error:
        raise EvidenceError(f"{context} path escapes the evidence directory") from error
    if not candidate.is_file() or candidate.stat().st_size == 0:
        raise EvidenceError(f"{context} file is missing or empty: {relative}")
    return candidate


def validate_result(value: Any, evidence_path: Path, label: str) -> dict[str, Any]:
    result = require_exact_fields(value, RESULT_FIELDS, label)
    if result["passed"] is not True:
        raise EvidenceError(f"{label} must have passed")
    transcript = resolve_evidence_file(evidence_path, result["transcript"], f"{label} transcript")
    if sha256_file(transcript) != require_digest(result["transcript_sha256"], f"{label} transcript"):
        raise EvidenceError(f"{label} transcript digest does not match {result['transcript']}")
    return result


def validate_file_evidence(value: Any, evidence_path: Path, label: str) -> Path:
    entry = require_exact_fields(value, FILE_EVIDENCE_FIELDS, label)
    path = resolve_evidence_file(evidence_path, entry["path"], label)
    if sha256_file(path) != require_digest(entry["sha256"], f"{label} digest"):
        raise EvidenceError(f"{label} digest does not match {entry['path']}")
    return path


def release_subject_digest(packages: list[Any], evidence_path: Path) -> str:
    records: list[dict[str, str]] = []
    for entry in packages:
        if not isinstance(entry, dict):
            raise EvidenceError("package artifacts must contain objects")
        package_id = entry.get("id")
        if not isinstance(package_id, str):
            raise EvidenceError("package artifact id must be a string")
        artifact = resolve_evidence_file(evidence_path, entry.get("artifact"), f"package {package_id} artifact")
        records.append({"id": package_id, "artifact_sha256": sha256_file(artifact)})
    return release_subject_digest_from_records(records)


def release_subject_digest_from_records(records: list[dict[str, str]]) -> str:
    return hashlib.sha256(canonical_json({"tag": REQUIRED_TAG, "packages": records})).hexdigest()


def release_subject_digest_from_package_directory(packages_dir: Path) -> str:
    if has_symlink_component(packages_dir) or not packages_dir.is_dir():
        raise EvidenceError("formal package directory must be a regular directory")
    version = REQUIRED_TAG.removeprefix("v")
    records: list[dict[str, str]] = []
    for package_id in PACKAGE_IDS:
        artifact = packages_dir / f"{package_id}-{version}.anxp"
        if not is_regular_file(artifact) or artifact.stat().st_size == 0:
            raise EvidenceError(f"formal package artifact is missing or empty: {artifact}")
        records.append({"id": package_id, "artifact_sha256": sha256_file(artifact)})
    return release_subject_digest_from_records(records)


def approval_payload(kind: str, release_subject_sha256: str) -> dict[str, Any]:
    return {
        "schema": APPROVAL_SCHEMA,
        "kind": kind,
        "tag": REQUIRED_TAG,
        "release_subject_sha256": release_subject_sha256,
        "approved": True,
    }


def load_and_validate_approval(
    path: Path,
    *,
    kind: str,
    release_subject_sha256: str,
    trusted_official_public_key: bytes,
    label: str,
) -> dict[str, Any]:
    if not is_regular_file(path) or path.stat().st_size == 0:
        raise EvidenceError(f"{label} is missing or empty: {path}")
    try:
        document = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, UnicodeError, json.JSONDecodeError) as error:
        raise EvidenceError(f"{label} is invalid JSON") from error
    approval = require_exact_fields(document, APPROVAL_FIELDS, label)
    if approval["schema"] != APPROVAL_SCHEMA:
        raise EvidenceError(f"{label} has unsupported schema: {approval['schema']!r}")
    if approval["kind"] != kind:
        raise EvidenceError(f"{label} kind must be {kind}")
    if approval["tag"] != REQUIRED_TAG:
        raise EvidenceError(f"{label} tag must be {REQUIRED_TAG}")
    if approval["approved"] is not True:
        raise EvidenceError(f"{label} must be approved")
    if approval["release_subject_sha256"] != require_digest(
        approval["release_subject_sha256"], f"{label} release subject"
    ):
        raise EvidenceError(f"{label} release subject is invalid")
    if approval["release_subject_sha256"] != release_subject_sha256:
        raise EvidenceError(f"{label} does not bind the formal v4 package artifacts")
    verify_detached_signature(
        canonical_json(approval_payload(kind, release_subject_sha256)),
        approval["signature"],
        trusted_official_public_key,
        label,
    )
    return approval


def validate_package_artifact(entry: Any, evidence_path: Path) -> Path:
    package = require_exact_fields(entry, PACKAGE_FIELDS, "package artifact")
    package_id = package["id"]
    if package_id not in PACKAGE_IDS:
        raise EvidenceError(f"unsupported package artifact id: {package_id!r}")
    version = REQUIRED_TAG.removeprefix("v")
    stem = f"{package_id}-{version}"
    files = {
        "artifact": f"{stem}.anxp",
        "manifest": f"{stem}.manifest.json",
        "signature": f"{stem}.manifest.sig",
        "public_key": f"{stem}.public-key.pem",
        "sbom": f"{stem}.sbom.spdx.json",
    }
    resolved: dict[str, Path] = {}
    for field, expected_name in files.items():
        path = resolve_evidence_file(evidence_path, package[field], f"package {package_id} {field}")
        if path.name != expected_name:
            raise EvidenceError(f"package {package_id} {field} must be named {expected_name}")
        resolved[field] = path
    try:
        manifest = json.loads(resolved["manifest"].read_text(encoding="utf-8"))
    except (OSError, UnicodeError, json.JSONDecodeError) as error:
        raise EvidenceError(f"package {package_id} manifest is invalid JSON") from error
    if not isinstance(manifest, dict) or manifest.get("id") != package_id or manifest.get("version") != version:
        raise EvidenceError(f"package {package_id} manifest identity does not match v4 evidence")
    artifact_digest = sha256_file(resolved["artifact"])
    if manifest.get("artifact_sha256") != artifact_digest:
        raise EvidenceError(f"package {package_id} manifest artifact digest does not match")
    if artifact_digest not in resolved["sbom"].read_text(encoding="utf-8", errors="replace"):
        raise EvidenceError(f"package {package_id} SBOM does not bind the artifact digest")
    return resolved["artifact"].parent


def validate_v4_evidence(evidence: Any, evidence_path: Path, trusted_official_public_key: bytes) -> None:
    if len(trusted_official_public_key) != ED25519_PUBLIC_KEY_BYTES:
        raise EvidenceError("trusted official public key must contain 32 raw Ed25519 bytes")
    if isinstance(evidence, dict) and "canary_evidence" not in evidence:
        raise EvidenceError("canary evidence is required")
    if isinstance(evidence, dict) and "support_evidence" not in evidence:
        raise EvidenceError("support evidence is required")
    document = require_exact_fields(evidence, EVIDENCE_FIELDS, "v4 release evidence")
    if document["schema"] != EVIDENCE_SCHEMA:
        raise EvidenceError(f"unsupported v4 release evidence schema: {document['schema']!r}")
    if document["tag"] != REQUIRED_TAG:
        raise EvidenceError(f"v4 release evidence tag must be {REQUIRED_TAG}")
    packages = document["package_artifacts"]
    if not isinstance(packages, list):
        raise EvidenceError("package artifacts must be an array")
    package_ids = [entry.get("id") if isinstance(entry, dict) else None for entry in packages]
    missing = [package_id for package_id in PACKAGE_IDS if package_id not in package_ids]
    extra = [package_id for package_id in package_ids if package_id not in PACKAGE_IDS]
    if missing or extra or len(package_ids) != len(PACKAGE_IDS) or len(set(package_ids)) != len(package_ids):
        details: list[str] = []
        if missing:
            details.append("missing required ids: " + ", ".join(missing))
        if extra:
            details.append("unexpected ids: " + ", ".join(str(package_id) for package_id in extra))
        if len(package_ids) != len(PACKAGE_IDS) or len(set(package_ids)) != len(package_ids):
            details.append(f"expected exactly {len(PACKAGE_IDS)} package artifacts")
        raise EvidenceError("package artifacts are incomplete: " + "; ".join(details))
    if package_ids != list(PACKAGE_IDS):
        raise EvidenceError("package artifacts must use the formal v4 release order")
    package_dirs = {validate_package_artifact(entry, evidence_path) for entry in packages}
    if len(package_dirs) != 1:
        raise EvidenceError("package artifacts must be colocated in one release directory")
    subject_digest = release_subject_digest(packages, evidence_path)

    official_root = require_exact_fields(document["official_root_verification"], OFFICIAL_ROOT_RESULT_FIELDS, "official root verification")
    validate_result(
        {field: official_root[field] for field in RESULT_FIELDS}, evidence_path, "official root verification"
    )
    root_file = resolve_evidence_file(evidence_path, official_root["public_key"], "official root verification")
    try:
        decoded_root = decode_raw_public_key(root_file.read_bytes(), "official root verification public key")
    except OSError as error:
        raise EvidenceError("official root verification public key is invalid") from error
    if decoded_root != trusted_official_public_key:
        raise EvidenceError("official root verification public key does not match the trusted official root")

    result_labels = {
        "release_stage_gate": "release stage gate",
        "v2_route_catalog_gate": "v2 route catalog gate",
        "plugin_only_route_gate": "plugin-only route gate",
        "websocket_relay_tests": "websocket relay tests",
        "v2_compatibility_tests": "v2 compatibility tests",
        "sqlite_rehearsal": "sqlite rehearsal",
        "postgres_rehearsal": "postgres rehearsal",
    }
    for field, label in result_labels.items():
        validate_result(document[field], evidence_path, label)
    canary_path = validate_file_evidence(document["canary_evidence"], evidence_path, "canary evidence")
    support_path = validate_file_evidence(document["support_evidence"], evidence_path, "support evidence")
    load_and_validate_approval(
        canary_path,
        kind="canary",
        release_subject_sha256=subject_digest,
        trusted_official_public_key=trusted_official_public_key,
        label="canary approval",
    )
    load_and_validate_approval(
        support_path,
        kind="support",
        release_subject_sha256=subject_digest,
        trusted_official_public_key=trusted_official_public_key,
        label="support approval",
    )


def verify_formal_artifacts(evidence: dict[str, Any], evidence_path: Path, trusted_official_public_key_path: Path) -> None:
    packages = evidence["package_artifacts"]
    package_dir = resolve_evidence_file(evidence_path, packages[0]["artifact"], "package artifact").parent
    command = [
        sys.executable,
        str(REPO_ROOT / "packages" / "shared" / "build_package.py"),
        "--all",
        "--version",
        REQUIRED_TAG.removeprefix("v"),
        "--out",
        str(package_dir),
        "--verify-release",
        "--official-public-key",
        str(trusted_official_public_key_path),
    ]
    result = subprocess.run(command, cwd=REPO_ROOT, check=False, text=True, capture_output=True)
    if result.returncode != 0:
        detail = (result.stderr or result.stdout).strip()
        raise EvidenceError(f"formal package artifact verification failed: {detail}")


def verify_matching_package_directory(evidence: dict[str, Any], evidence_path: Path, packages_dir: Path) -> None:
    if has_symlink_component(packages_dir) or not packages_dir.is_dir():
        raise EvidenceError("release package directory must be a regular directory")
    for entry in evidence["package_artifacts"]:
        package = require_exact_fields(entry, PACKAGE_FIELDS, "package artifact")
        package_id = package["id"]
        for field in ("artifact", "manifest", "signature", "public_key", "sbom"):
            evidence_file = resolve_evidence_file(evidence_path, package[field], f"package {package_id} {field}")
            release_file = packages_dir / evidence_file.name
            if not is_regular_file(release_file) or release_file.stat().st_size == 0:
                raise EvidenceError(f"release package {package_id} {field} is missing or empty")
            if sha256_file(release_file) != sha256_file(evidence_file):
                raise EvidenceError(f"release package {package_id} {field} does not match the verified evidence")
    evidence_root = resolve_evidence_file(
        evidence_path, evidence["official_root_verification"]["public_key"], "official root verification"
    )
    release_root = packages_dir / "official-public-key.raw"
    if not is_regular_file(release_root) or release_root.stat().st_size == 0:
        raise EvidenceError("release package official root is missing or empty")
    if sha256_file(release_root) != sha256_file(evidence_root):
        raise EvidenceError("release package official root does not match the verified evidence")


def load_evidence(path: Path) -> dict[str, Any]:
    try:
        document = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, UnicodeError, json.JSONDecodeError) as error:
        raise EvidenceError(f"cannot read v4 release evidence {path}: {error}") from error
    if not isinstance(document, dict):
        raise EvidenceError("v4 release evidence must be an object")
    return document


def verify_evidence_document(
    input_path: Path,
    signature_path: Path,
    trusted_root_path: Path,
    trusted_root: bytes,
    release_packages_dir: Path | None = None,
) -> None:
    verify_evidence_signature(input_path, signature_path, trusted_root)
    evidence = load_evidence(input_path)
    validate_v4_evidence(evidence, input_path, trusted_root)
    verify_formal_artifacts(evidence, input_path, trusted_root_path)
    if release_packages_dir is not None:
        verify_matching_package_directory(evidence, input_path, release_packages_dir)


def verify_evidence_archive(
    archive_path: Path, trusted_root_path: Path, trusted_root: bytes, release_packages_dir: Path | None = None
) -> None:
    with tempfile.TemporaryDirectory(prefix="anixops-v4-evidence-archive-") as temporary:
        root = Path(temporary)
        safe_extract_evidence_archive(archive_path, root)
        input_path = root / "v4-rehearsal-evidence.json"
        signature_path = root / "v4-rehearsal-evidence.json.sig"
        verify_evidence_document(input_path, signature_path, trusted_root_path, trusted_root, release_packages_dir)


def parse_arguments() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    source = parser.add_mutually_exclusive_group()
    source.add_argument("--input", type=Path, help="v4 evidence JSON file")
    source.add_argument("--archive", type=Path, help="v4 evidence tar.gz bundle")
    parser.add_argument(
        "--trusted-official-public-key",
        type=Path,
        help="Independent Base64 raw Ed25519 official root used to verify the evidence bundle",
    )
    parser.add_argument("--signature", type=Path, help="Detached Base64 evidence signature (defaults beside --input)")
    parser.add_argument(
        "--release-packages-dir",
        type=Path,
        help="Published package directory that must byte-match the verified evidence set",
    )
    parser.add_argument("--self-test", action="store_true", help="Validate the static v4 evidence contract")
    return parser.parse_args()


def main() -> int:
    arguments = parse_arguments()
    try:
        if arguments.self_test:
            if len(PACKAGE_IDS) != 16 or "identity-platform" not in PACKAGE_IDS:
                raise EvidenceError("v4 evidence package matrix is incomplete")
            print("v4 release evidence verifier self-test passed")
            return 0
        if arguments.input is None and arguments.archive is None:
            raise EvidenceError("--input or --archive is required unless --self-test is used")
        if arguments.trusted_official_public_key is None:
            raise EvidenceError("--trusted-official-public-key is required unless --self-test is used")
        trusted_root_path = absolute_path(arguments.trusted_official_public_key)
        trusted_root = load_trusted_official_public_key(trusted_root_path)
        release_packages_dir = absolute_path(arguments.release_packages_dir) if arguments.release_packages_dir else None
        if arguments.archive is not None:
            if arguments.signature is not None:
                raise EvidenceError("--signature can be used only with --input")
            verify_evidence_archive(absolute_path(arguments.archive), trusted_root_path, trusted_root, release_packages_dir)
        else:
            input_path = absolute_path(arguments.input)
            signature_path = absolute_path(arguments.signature) if arguments.signature else input_path.with_suffix(input_path.suffix + ".sig")
            verify_evidence_document(input_path, signature_path, trusted_root_path, trusted_root, release_packages_dir)
    except EvidenceError as error:
        print(f"v4 release evidence: {error}", file=sys.stderr)
        return 1
    print(f"v4 release evidence verified ({len(PACKAGE_IDS)} package artifacts)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
