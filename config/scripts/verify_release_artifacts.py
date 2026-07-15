#!/usr/bin/env python3
"""Verify GitHub Release artifact manifest and checksum consistency."""

from __future__ import annotations

import argparse
import fnmatch
import hashlib
import json
import tempfile
from pathlib import Path
from typing import Any


MANIFEST_NAME = "RELEASE_MANIFEST.json"
CHECKSUM_NAME = "SHA256SUMS.txt"


class VerificationError(RuntimeError):
    """Raised when release artifact verification fails."""


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def release_file_names(release_dir: Path, *, exclude_checksums: bool) -> set[str]:
    excluded = {CHECKSUM_NAME} if exclude_checksums else set()
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


def verify_manifest(release_dir: Path, manifest_path: Path) -> set[str]:
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
        if name in {MANIFEST_NAME, CHECKSUM_NAME}:
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

    return manifest_names


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
) -> None:
    if not release_dir.is_dir():
        raise VerificationError(f"release directory not found: {release_dir}")
    if not manifest_path.is_file():
        raise VerificationError(f"release manifest not found: {manifest_path}")
    if not checksum_path.is_file():
        raise VerificationError(f"release checksums not found: {checksum_path}")

    verify_manifest(release_dir, manifest_path)
    verify_checksums(release_dir, checksum_path)
    verify_required_assets(release_dir, required, required_globs)


def write_fixture_file(path: Path, content: bytes) -> None:
    path.write_bytes(content)


def write_fixture_manifest(release_dir: Path) -> None:
    artifacts = []
    for path in sorted(release_dir.iterdir(), key=lambda item: item.name):
        if path.is_file() and path.name not in {MANIFEST_NAME, CHECKSUM_NAME}:
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
    }
    (release_dir / MANIFEST_NAME).write_text(json.dumps(manifest, indent=2, sort_keys=True) + "\n", encoding="utf-8")


def write_fixture_checksums(release_dir: Path) -> None:
    lines = []
    for path in sorted(release_dir.iterdir(), key=lambda item: item.name):
        if path.is_file() and path.name != CHECKSUM_NAME:
            lines.append(f"{sha256_file(path)}  {path.name}")
    (release_dir / CHECKSUM_NAME).write_text("\n".join(lines) + "\n", encoding="utf-8")


def expect_failure(label: str, func: Any) -> None:
    try:
        func()
    except VerificationError:
        return
    raise AssertionError(f"expected verification failure: {label}")


def run_self_test() -> None:
    required = [
        "OPERATOR_DEPLOYMENT.md",
        "UPGRADE.md",
        "RELEASE_NOTES.md",
        "RELEASE_MANIFEST.json",
        "SHA256SUMS.txt",
        "docker-image.txt",
        "migration-dry-run.txt",
        "anix-control-source.sbom.spdx.json",
        "anix-control-frontend.tar.gz",
        "anix-control-frontend.zip",
        "anix-control-linux-amd64.tar.gz",
        "anix-control-linux-arm64.tar.gz",
        "anix-control-windows-amd64.exe.zip",
        "anix-control-windows-arm64.exe.zip",
        "anix-control-darwin-amd64.tar.gz",
        "anix-control-darwin-arm64.tar.gz",
    ]

    with tempfile.TemporaryDirectory() as tmp:
        release_dir = Path(tmp) / "release"
        release_dir.mkdir()
        for name in required:
            if name not in {MANIFEST_NAME, CHECKSUM_NAME}:
                write_fixture_file(release_dir / name, f"{name}\n".encode("utf-8"))
        write_fixture_manifest(release_dir)
        write_fixture_checksums(release_dir)

        verify_release_artifacts(
            release_dir=release_dir,
            manifest_path=release_dir / MANIFEST_NAME,
            checksum_path=release_dir / CHECKSUM_NAME,
            required=required,
            required_globs=["anix-control-linux-*.tar.gz", "anix-control-windows-*.zip"],
        )

        corrupt_dir = Path(tmp) / "corrupt"
        corrupt_dir.mkdir()
        for path in release_dir.iterdir():
            if path.is_file():
                write_fixture_file(corrupt_dir / path.name, path.read_bytes())
        (corrupt_dir / "docker-image.txt").write_text("tampered\n", encoding="utf-8")
        expect_failure(
            "tampered artifact",
            lambda: verify_release_artifacts(
                release_dir=corrupt_dir,
                manifest_path=corrupt_dir / MANIFEST_NAME,
                checksum_path=corrupt_dir / CHECKSUM_NAME,
                required=required,
                required_globs=[],
            ),
        )

        missing_checksum_dir = Path(tmp) / "missing-checksum"
        missing_checksum_dir.mkdir()
        for path in release_dir.iterdir():
            if path.is_file():
                write_fixture_file(missing_checksum_dir / path.name, path.read_bytes())
        lines = (missing_checksum_dir / CHECKSUM_NAME).read_text(encoding="utf-8").splitlines()
        (missing_checksum_dir / CHECKSUM_NAME).write_text("\n".join(lines[:-1]) + "\n", encoding="utf-8")
        expect_failure(
            "missing checksum entry",
            lambda: verify_release_artifacts(
                release_dir=missing_checksum_dir,
                manifest_path=missing_checksum_dir / MANIFEST_NAME,
                checksum_path=missing_checksum_dir / CHECKSUM_NAME,
                required=required,
                required_globs=[],
            ),
        )

        expect_failure(
            "missing required asset",
            lambda: verify_release_artifacts(
                release_dir=release_dir,
                manifest_path=release_dir / MANIFEST_NAME,
                checksum_path=release_dir / CHECKSUM_NAME,
                required=required + ["missing-required.txt"],
                required_globs=[],
            ),
        )

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
    )
    print("release artifacts verified")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
