#!/usr/bin/env python3
"""Verify GitHub Release artifact manifest and checksum consistency."""

from __future__ import annotations

import argparse
import base64
import binascii
import fnmatch
import hashlib
import json
import subprocess
import sys
import tempfile
from pathlib import Path
from typing import Any

sys.path.insert(0, str(Path(__file__).resolve().parent))
from generate_release_manifest import (  # noqa: E402
    CHECKSUM_SIGNATURE_NAME,
    builtin_package_entries,
    write_fixture_builtin_set,
)


MANIFEST_NAME = "RELEASE_MANIFEST.json"
CHECKSUM_NAME = "SHA256SUMS.txt"
ED25519_SPKI_PREFIX = bytes.fromhex("302a300506032b6570032100")


class VerificationError(RuntimeError):
    """Raised when release artifact verification fails."""


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def release_file_names(release_dir: Path, *, exclude_checksums: bool) -> set[str]:
    # The checksums and their signature list neither each other nor
    # themselves: the signature is written after the checksums.
    excluded = {CHECKSUM_NAME, CHECKSUM_SIGNATURE_NAME} if exclude_checksums else set()
    return {path.name for path in release_dir.iterdir() if path.is_file() and path.name not in excluded}


def validate_flat_name(name: str, context: str) -> None:
    if not name or Path(name).name != name or name in {".", ".."}:
        raise VerificationError(f"{context} must be a flat release file name: {name!r}")


def load_manifest(path: Path) -> dict[str, Any]:
    try:
        data = json.loads(path.read_text(encoding="utf-8"))
    except json.JSONDecodeError as exc:
        raise VerificationError(f"invalid release manifest JSON: {exc}") from exc
    if not isinstance(data, dict):
        raise VerificationError("release manifest must be a JSON object")
    return data


def read_checksums(path: Path) -> dict[str, str]:
    entries: dict[str, str] = {}
    for line_no, line in enumerate(path.read_text(encoding="utf-8").splitlines(), start=1):
        if not line.strip():
            continue
        parts = line.split(maxsplit=1)
        if len(parts) != 2:
            raise VerificationError(f"invalid checksum line {line_no}: {line!r}")
        digest, name = parts
        name = name.lstrip("*")
        validate_flat_name(name, f"checksum line {line_no}")
        if len(digest) != 64 or any(char not in "0123456789abcdefABCDEF" for char in digest):
            raise VerificationError(f"invalid SHA-256 digest on line {line_no}: {digest!r}")
        if name in entries:
            raise VerificationError(f"duplicate checksum entry: {name}")
        entries[name] = digest.lower()
    return entries


def verify_manifest(release_dir: Path, manifest_path: Path, builtin_dir: Path | None = None) -> set[str]:
    manifest = load_manifest(manifest_path)
    if manifest.get("build_source") != "github-actions":
        raise VerificationError("manifest build_source must be github-actions")
    if manifest.get("manual_deployment_required") is not True:
        raise VerificationError("manifest manual_deployment_required must be true")

    artifacts = manifest.get("artifacts")
    if not isinstance(artifacts, list) or not artifacts:
        raise VerificationError("manifest artifacts must be a non-empty list")

    manifest_names: set[str] = set()
    for index, artifact in enumerate(artifacts):
        if not isinstance(artifact, dict):
            raise VerificationError(f"manifest artifact {index} must be an object")

        name = artifact.get("name")
        if not isinstance(name, str):
            raise VerificationError(f"manifest artifact {index} has invalid name")
        validate_flat_name(name, f"manifest artifact {index}")
        if name in {MANIFEST_NAME, CHECKSUM_NAME, CHECKSUM_SIGNATURE_NAME}:
            raise VerificationError(f"manifest must not list generated metadata file: {name}")
        if name in manifest_names:
            raise VerificationError(f"duplicate manifest artifact: {name}")
        manifest_names.add(name)

        path = release_dir / name
        if not path.is_file():
            raise VerificationError(f"manifest artifact missing from release directory: {name}")
        if artifact.get("size") != path.stat().st_size:
            raise VerificationError(f"manifest size mismatch for {name}")
        if artifact.get("sha256") != sha256_file(path):
            raise VerificationError(f"manifest SHA-256 mismatch for {name}")

    expected_names = release_file_names(release_dir, exclude_checksums=True) - {MANIFEST_NAME}
    if manifest_names != expected_names:
        missing = sorted(expected_names - manifest_names)
        extra = sorted(manifest_names - expected_names)
        raise VerificationError(f"manifest artifact set mismatch; missing={missing}, extra={extra}")

    verify_package_listing(manifest, builtin_dir)
    return manifest_names


def verify_package_listing(manifest: dict[str, Any], builtin_dir: Path | None) -> None:
    """The manifest lists exactly the packages the release binaries embed."""
    if builtin_dir is None:
        return
    try:
        expected = builtin_package_entries(builtin_dir)
    except (OSError, ValueError, KeyError) as exc:
        raise VerificationError(f"cannot read the built-in packages: {exc}") from exc
    if not expected:
        raise VerificationError("the release binaries embed no packages")
    if manifest.get("packages", []) != expected:
        raise VerificationError("manifest packages do not match the built-in packages")


def verify_checksums_signature(checksum_path: Path, signature_path: Path, official_public_key: Path) -> None:
    """SHA256SUMS.txt.sig: Base64 raw Ed25519 over SHA256SUMS.txt's exact bytes,
    made with the official package key (the raw Base64 root of
    plugins.official_public_key)."""
    try:
        raw_key = base64.b64decode(b"".join(official_public_key.read_bytes().split()), validate=True)
        signature = base64.b64decode(b"".join(signature_path.read_bytes().split()), validate=True)
    except (OSError, ValueError, binascii.Error) as exc:
        raise VerificationError(f"cannot read the checksum signature or the official key: {exc}") from exc
    if len(raw_key) != 32:
        raise VerificationError("the official public key must be 32 raw Ed25519 bytes")
    with tempfile.TemporaryDirectory(prefix="anixops-checksums-") as temporary:
        key_path = Path(temporary) / "official.der"
        key_path.write_bytes(ED25519_SPKI_PREFIX + raw_key)
        signature_bin = Path(temporary) / "signature.bin"
        signature_bin.write_bytes(signature)
        result = subprocess.run(
            ["openssl", "pkeyutl", "-verify", "-pubin", "-keyform", "DER", "-inkey", str(key_path), "-rawin",
             "-in", str(checksum_path), "-sigfile", str(signature_bin)],
            check=False, capture_output=True, text=True,
        )
    if result.returncode != 0:
        raise VerificationError(f"{signature_path.name} does not verify against the official public key")


def verify_checksums(release_dir: Path, checksum_path: Path) -> None:
    entries = read_checksums(checksum_path)
    expected_names = release_file_names(release_dir, exclude_checksums=True)
    if set(entries) != expected_names:
        missing = sorted(expected_names - set(entries))
        extra = sorted(set(entries) - expected_names)
        raise VerificationError(f"checksum file set mismatch; missing={missing}, extra={extra}")
    for name, digest in entries.items():
        actual = sha256_file(release_dir / name)
        if digest != actual:
            raise VerificationError(f"checksum SHA-256 mismatch for {name}")


def verify_required_assets(release_dir: Path, required: list[str], required_globs: list[str]) -> None:
    names = release_file_names(release_dir, exclude_checksums=False)
    for name in required:
        validate_flat_name(name, "required asset")
        if name not in names:
            raise VerificationError(f"required release asset missing: {name}")
    for pattern in required_globs:
        if not any(fnmatch.fnmatch(name, pattern) for name in names):
            raise VerificationError(f"required release asset pattern has no matches: {pattern}")


def verify_release_artifacts(
    *,
    release_dir: Path,
    manifest_path: Path,
    checksum_path: Path,
    required: list[str],
    required_globs: list[str],
    builtin_dir: Path | None = None,
    official_public_key: Path | None = None,
) -> None:
    if not release_dir.is_dir():
        raise VerificationError(f"release directory not found: {release_dir}")
    if not manifest_path.is_file():
        raise VerificationError(f"release manifest not found: {manifest_path}")
    if not checksum_path.is_file():
        raise VerificationError(f"release checksums not found: {checksum_path}")

    verify_manifest(release_dir, manifest_path, builtin_dir)
    verify_checksums(release_dir, checksum_path)
    verify_required_assets(release_dir, required, required_globs)
    if official_public_key is not None:
        signature_path = checksum_path.with_name(checksum_path.name + ".sig")
        if not signature_path.is_file():
            raise VerificationError(f"checksum signature missing: {signature_path.name}")
        verify_checksums_signature(checksum_path, signature_path, official_public_key)


def write_fixture_file(path: Path, content: bytes) -> None:
    path.write_bytes(content)


def write_fixture_manifest(release_dir: Path, builtin_dir: Path | None = None) -> None:
    artifacts = []
    for path in sorted(release_dir.iterdir(), key=lambda item: item.name):
        if path.is_file() and path.name not in {MANIFEST_NAME, CHECKSUM_NAME, CHECKSUM_SIGNATURE_NAME}:
            artifacts.append({"name": path.name, "size": path.stat().st_size, "sha256": sha256_file(path)})
    manifest = {
        "project": "anix-control",
        "tag": "v2.4.0",
        "commit": "abc123",
        "run_id": "100",
        "run_number": "7",
        "run_attempt": "2",
        "build_source": "github-actions",
        "manual_deployment_required": True,
        "artifacts": artifacts,
        "packages": builtin_package_entries(builtin_dir) if builtin_dir is not None else [],
    }
    (release_dir / MANIFEST_NAME).write_text(json.dumps(manifest, indent=2, sort_keys=True) + "\n", encoding="utf-8")


def copy_fixture_release(source: Path, target: Path) -> None:
    target.mkdir()
    for path in source.iterdir():
        if path.is_file():
            write_fixture_file(target / path.name, path.read_bytes())


def write_fixture_checksums(release_dir: Path) -> None:
    lines = []
    for path in sorted(release_dir.iterdir(), key=lambda item: item.name):
        if path.is_file() and path.name not in {CHECKSUM_NAME, CHECKSUM_SIGNATURE_NAME}:
            lines.append(f"{sha256_file(path)}  {path.name}")
    (release_dir / CHECKSUM_NAME).write_text("\n".join(lines) + "\n", encoding="utf-8")


def expect_failure(label: str, func: Any) -> None:
    try:
        func()
    except VerificationError:
        return
    raise AssertionError(f"expected verification failure: {label}")


def sign_fixture_checksums(release_dir: Path, private_key: Path) -> None:
    """Sign SHA256SUMS.txt the way the release job does."""
    with tempfile.TemporaryDirectory(prefix="anixops-checksums-") as temporary:
        signature_bin = Path(temporary) / "signature.bin"
        subprocess.run(
            ["openssl", "pkeyutl", "-sign", "-inkey", str(private_key), "-rawin",
             "-in", str(release_dir / CHECKSUM_NAME), "-out", str(signature_bin)],
            check=True, capture_output=True,
        )
        (release_dir / CHECKSUM_SIGNATURE_NAME).write_bytes(base64.b64encode(signature_bin.read_bytes()) + b"\n")


def fixture_key(root: Path, name: str) -> tuple[Path, Path]:
    private_key = root / f"{name}.pem"
    subprocess.run(["openssl", "genpkey", "-algorithm", "ED25519", "-out", str(private_key)], check=True, capture_output=True)
    der = subprocess.run(["openssl", "pkey", "-in", str(private_key), "-pubout", "-outform", "DER"], check=True, capture_output=True).stdout
    raw = root / f"{name}.raw"
    raw.write_bytes(base64.b64encode(der[len(ED25519_SPKI_PREFIX):]) + b"\n")
    return private_key, raw


def run_self_test() -> None:
    required = [
        "RELEASE_NOTES.md",
        "RELEASE_MANIFEST.json",
        "SHA256SUMS.txt",
        "SHA256SUMS.txt.sig",
        "docker-image.txt",
        "official-public-key.raw",
        "anix-control-source.sbom.spdx.json",
        "anix-control-frontend.tar.gz",
        "anix-control-linux-amd64.tar.gz",
        "anix-control-linux-arm64.tar.gz",
    ]

    with tempfile.TemporaryDirectory() as tmp:
        root = Path(tmp)
        private_key, official = fixture_key(root, "official")
        _, other = fixture_key(root, "other")
        builtin_dir = root / "builtin"
        write_fixture_builtin_set(builtin_dir)
        release_dir = root / "release"
        release_dir.mkdir()
        for name in required:
            if name not in {MANIFEST_NAME, CHECKSUM_NAME, CHECKSUM_SIGNATURE_NAME}:
                write_fixture_file(release_dir / name, f"{name}\n".encode("utf-8"))
        write_fixture_manifest(release_dir, builtin_dir)
        write_fixture_checksums(release_dir)
        sign_fixture_checksums(release_dir, private_key)

        def verify(directory: Path, **overrides: Any) -> None:
            arguments: dict[str, Any] = {
                "release_dir": directory,
                "manifest_path": directory / MANIFEST_NAME,
                "checksum_path": directory / CHECKSUM_NAME,
                "required": required,
                "required_globs": ["anix-control-linux-*.tar.gz"],
                "builtin_dir": builtin_dir,
                "official_public_key": official,
            }
            arguments.update(overrides)
            verify_release_artifacts(**arguments)

        verify(release_dir)

        corrupt_dir = root / "corrupt"
        copy_fixture_release(release_dir, corrupt_dir)
        (corrupt_dir / "docker-image.txt").write_text("tampered\n", encoding="utf-8")
        expect_failure("tampered artifact", lambda: verify(corrupt_dir))

        missing_checksum_dir = root / "missing-checksum"
        copy_fixture_release(release_dir, missing_checksum_dir)
        lines = (missing_checksum_dir / CHECKSUM_NAME).read_text(encoding="utf-8").splitlines()
        (missing_checksum_dir / CHECKSUM_NAME).write_text("\n".join(lines[:-1]) + "\n", encoding="utf-8")
        expect_failure("missing checksum entry", lambda: verify(missing_checksum_dir, official_public_key=None))

        expect_failure("missing required asset", lambda: verify(release_dir, required=required + ["missing-required.txt"]))

        # The checksums must be signed with the official key.
        expect_failure("checksums signed with another key", lambda: verify(release_dir, official_public_key=other))
        resigned_dir = root / "edited-checksums"
        copy_fixture_release(release_dir, resigned_dir)
        with (resigned_dir / CHECKSUM_NAME).open("a", encoding="utf-8") as handle:
            handle.write("\n")
        expect_failure("checksums edited after signing", lambda: verify(resigned_dir))
        unsigned_dir = root / "unsigned"
        copy_fixture_release(release_dir, unsigned_dir)
        (unsigned_dir / CHECKSUM_SIGNATURE_NAME).unlink()
        expect_failure("unsigned checksums", lambda: verify(unsigned_dir, required=[name for name in required if name != CHECKSUM_SIGNATURE_NAME]))

        # The manifest lists exactly the packages the binaries embed.
        other_builtin = root / "other-builtin"
        write_fixture_builtin_set(other_builtin)
        (other_builtin / "demo-2.4.0.manifest.json").write_text('{"id":"demo","version":"2.4.0","artifact_sha256":"0"}', encoding="utf-8")
        expect_failure("built-in package listing mismatch", lambda: verify(release_dir, builtin_dir=other_builtin))
        empty_builtin = root / "empty-builtin"
        empty_builtin.mkdir()
        (empty_builtin / "index.json").write_text('{"format":"anixops.builtin-packages/v1","packages":[]}', encoding="utf-8")
        expect_failure("binaries without packages", lambda: verify(release_dir, builtin_dir=empty_builtin))

    print("release artifact verification self-test passed")


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--release-dir", default="release", help="Directory containing release assets.")
    parser.add_argument(
        "--manifest",
        default=None,
        help="Manifest path. Defaults to RELEASE_MANIFEST.json inside --release-dir.",
    )
    parser.add_argument(
        "--checksums",
        default=None,
        help="Checksum path. Defaults to SHA256SUMS.txt inside --release-dir.",
    )
    parser.add_argument("--require", action="append", default=[], help="Required release asset file name.")
    parser.add_argument("--require-glob", action="append", default=[], help="Required release asset glob pattern.")
    parser.add_argument(
        "--builtin-packages",
        type=Path,
        default=None,
        help="The built-in package directory the release binaries embed; the manifest must list exactly its packages.",
    )
    parser.add_argument(
        "--official-public-key",
        type=Path,
        default=None,
        help="Raw Base64 Ed25519 official key: SHA256SUMS.txt.sig must verify against it.",
    )
    parser.add_argument("--self-test", action="store_true", help="Run the built-in verification self-test.")
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    if args.self_test:
        run_self_test()
        return 0

    release_dir = Path(args.release_dir)
    manifest_path = Path(args.manifest) if args.manifest else release_dir / MANIFEST_NAME
    checksum_path = Path(args.checksums) if args.checksums else release_dir / CHECKSUM_NAME
    verify_release_artifacts(
        release_dir=release_dir,
        manifest_path=manifest_path,
        checksum_path=checksum_path,
        required=args.require,
        required_globs=args.require_glob,
        builtin_dir=args.builtin_packages,
        official_public_key=args.official_public_key,
    )
    print("release artifacts verified")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
