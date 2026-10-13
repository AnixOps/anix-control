#!/usr/bin/env python3
"""Generate the machine-readable GitHub Actions release manifest."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import tempfile
from pathlib import Path, PurePosixPath
from typing import Iterable


MANIFEST_NAME = "RELEASE_MANIFEST.json"
CHECKSUM_NAME = "SHA256SUMS.txt"
# SHA256SUMS.txt.sig signs SHA256SUMS.txt with the official package key; it
# is written after the checksums and, like them, listed in neither file.
CHECKSUM_SIGNATURE_NAME = "SHA256SUMS.txt.sig"
# packages/shared/build_package.py --builtin writes this index next to the
# packages the release binaries embed.
BUILTIN_INDEX_NAME = "index.json"
BUILTIN_INDEX_FORMAT = "anixops.builtin-packages/v1"


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def release_files(release_dir: Path, output: Path) -> Iterable[Path]:
    excluded = {CHECKSUM_NAME, CHECKSUM_SIGNATURE_NAME, MANIFEST_NAME, output.name}
    for path in sorted(release_dir.iterdir(), key=lambda item: item.name):
        if path.is_file() and path.name not in excluded:
            yield path


def builtin_package_entries(builtin_dir: Path) -> list[dict[str, object]]:
    """List the official packages the release binaries embed.

    They are not release assets: the Control binary carries them
    (internal/builtinpkg). The manifest keeps one entry per package: its id
    and version, and the size and SHA-256 of its slim artifact and of its
    signed manifest.
    """
    index = json.loads((builtin_dir / BUILTIN_INDEX_NAME).read_text(encoding="utf-8"))
    if not isinstance(index, dict) or index.get("format") != BUILTIN_INDEX_FORMAT or not isinstance(index.get("packages"), list):
        raise ValueError(f"{builtin_dir / BUILTIN_INDEX_NAME} is not a built-in package index")
    entries = []
    for package in index["packages"]:
        manifest_bytes = (builtin_dir / PurePosixPath(package["manifest"]).name).read_bytes()
        manifest = json.loads(manifest_bytes)
        artifact_path = builtin_dir / PurePosixPath(package["artifact"]).name
        artifact_sha256 = sha256_file(artifact_path)
        if manifest.get("id") != package["id"] or manifest.get("version") != package["version"]:
            raise ValueError(f"{package['manifest']} does not name {package['id']} {package['version']}")
        if manifest.get("artifact_sha256") != artifact_sha256:
            raise ValueError(f"{package['artifact']} does not match its manifest artifact_sha256")
        entries.append(
            {
                "id": package["id"],
                "version": package["version"],
                "artifact": artifact_path.name,
                "size": artifact_path.stat().st_size,
                "sha256": artifact_sha256,
                "manifest_sha256": hashlib.sha256(manifest_bytes).hexdigest(),
            }
        )
    return sorted(entries, key=lambda entry: str(entry["id"]))


def build_manifest(
    *,
    release_dir: Path,
    output: Path,
    project: str,
    build_source: str,
    manual_deployment_required: bool,
    builtin_dir: Path | None = None,
) -> dict[str, object]:
    artifacts = [
        {
            "name": path.name,
            "size": path.stat().st_size,
            "sha256": sha256_file(path),
        }
        for path in release_files(release_dir, output)
    ]

    return {
        "project": project,
        "tag": os.environ.get("GITHUB_REF_NAME", ""),
        "commit": os.environ.get("GITHUB_SHA", ""),
        "run_id": os.environ.get("GITHUB_RUN_ID", ""),
        "run_number": os.environ.get("GITHUB_RUN_NUMBER", ""),
        "run_attempt": os.environ.get("GITHUB_RUN_ATTEMPT", ""),
        "build_source": build_source,
        "manual_deployment_required": manual_deployment_required,
        "artifacts": artifacts,
        "packages": builtin_package_entries(builtin_dir) if builtin_dir is not None else [],
    }


def write_manifest(manifest: dict[str, object], output: Path) -> None:
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(manifest, indent=2, sort_keys=True) + "\n", encoding="utf-8")


def write_fixture_builtin_set(builtin_dir: Path) -> tuple[bytes, bytes]:
    """A built-in index with one package, as build_package.py --builtin writes it."""
    builtin_dir.mkdir()
    anxp = b"package bytes"
    package_manifest = json.dumps(
        {"id": "demo", "version": "2.4.0", "artifact_sha256": hashlib.sha256(anxp).hexdigest()}
    ).encode("utf-8")
    (builtin_dir / "demo-2.4.0.anxp").write_bytes(anxp)
    (builtin_dir / "demo-2.4.0.manifest.json").write_bytes(package_manifest)
    (builtin_dir / "demo-2.4.0.manifest.sig").write_bytes(b"signature\n")
    (builtin_dir / BUILTIN_INDEX_NAME).write_text(
        json.dumps(
            {
                "format": BUILTIN_INDEX_FORMAT,
                "packages": [
                    {
                        "artifact": "demo-2.4.0.anxp",
                        "id": "demo",
                        "manifest": "demo-2.4.0.manifest.json",
                        "signature": "demo-2.4.0.manifest.sig",
                        "version": "2.4.0",
                    }
                ],
                "public_key": "official-public-key.pem",
            }
        ),
        encoding="utf-8",
    )
    return anxp, package_manifest


def run_self_test() -> None:
    with tempfile.TemporaryDirectory() as tmp:
        release_dir = Path(tmp) / "release"
        release_dir.mkdir()
        (release_dir / "b.txt").write_text("second\n", encoding="utf-8")
        (release_dir / "a.bin").write_bytes(b"first")
        (release_dir / CHECKSUM_NAME).write_text("old checksum\n", encoding="utf-8")
        (release_dir / CHECKSUM_SIGNATURE_NAME).write_text("old signature\n", encoding="utf-8")
        (release_dir / MANIFEST_NAME).write_text("old manifest\n", encoding="utf-8")
        (release_dir / "ignored-dir").mkdir()
        anxp, package_manifest = write_fixture_builtin_set(Path(tmp) / "builtin")

        output = release_dir / MANIFEST_NAME
        os.environ.update(
            {
                "GITHUB_REF_NAME": "v2.4.0",
                "GITHUB_SHA": "abc123",
                "GITHUB_RUN_ID": "100",
                "GITHUB_RUN_NUMBER": "7",
                "GITHUB_RUN_ATTEMPT": "2",
            }
        )

        manifest = build_manifest(
            release_dir=release_dir,
            output=output,
            project="test-project",
            build_source="github-actions",
            manual_deployment_required=True,
            builtin_dir=Path(tmp) / "builtin",
        )
        write_manifest(manifest, output)
        loaded = json.loads(output.read_text(encoding="utf-8"))

        names = [artifact["name"] for artifact in loaded["artifacts"]]
        assert names == ["a.bin", "b.txt"], names
        assert loaded["packages"] == [
            {
                "id": "demo",
                "version": "2.4.0",
                "artifact": "demo-2.4.0.anxp",
                "size": len(anxp),
                "sha256": hashlib.sha256(anxp).hexdigest(),
                "manifest_sha256": hashlib.sha256(package_manifest).hexdigest(),
            }
        ], loaded["packages"]
        assert loaded["project"] == "test-project"
        assert loaded["tag"] == "v2.4.0"
        assert loaded["commit"] == "abc123"
        assert loaded["run_id"] == "100"
        assert loaded["run_number"] == "7"
        assert loaded["run_attempt"] == "2"
        assert loaded["build_source"] == "github-actions"
        assert loaded["manual_deployment_required"] is True
        assert loaded["artifacts"][0]["sha256"] == hashlib.sha256(b"first").hexdigest()
        assert loaded["artifacts"][1]["sha256"] == hashlib.sha256(b"second\n").hexdigest()
        assert output.read_text(encoding="utf-8").endswith("\n")

        (Path(tmp) / "builtin" / "demo-2.4.0.anxp").write_bytes(b"tampered")
        try:
            builtin_package_entries(Path(tmp) / "builtin")
        except ValueError:
            pass
        else:
            raise AssertionError("a tampered built-in artifact must be refused")

    print("release manifest self-test passed")


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--release-dir", default="release", help="Directory containing release assets.")
    parser.add_argument(
        "--output",
        default=None,
        help="Manifest output path. Defaults to RELEASE_MANIFEST.json inside --release-dir.",
    )
    parser.add_argument("--project", default="anix-control", help="Project name recorded in the manifest.")
    parser.add_argument(
        "--build-source",
        default="github-actions",
        help="Build source marker recorded in the manifest.",
    )
    parser.add_argument(
        "--manual-deployment-required",
        choices=("true", "false"),
        default="true",
        help="Whether production deployment requires manual operator approval.",
    )
    parser.add_argument(
        "--builtin-packages",
        type=Path,
        default=None,
        help="The verified built-in package directory the release binaries embed (build_package.py --builtin).",
    )
    parser.add_argument("--self-test", action="store_true", help="Run the built-in manifest generation self-test.")
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    if args.self_test:
        run_self_test()
        return 0

    release_dir = Path(args.release_dir)
    output = Path(args.output) if args.output else release_dir / MANIFEST_NAME
    if not release_dir.is_dir():
        raise SystemExit(f"release directory not found: {release_dir}")

    manifest = build_manifest(
        release_dir=release_dir,
        output=output,
        project=args.project,
        build_source=args.build_source,
        manual_deployment_required=args.manual_deployment_required == "true",
        builtin_dir=args.builtin_packages,
    )
    write_manifest(manifest, output)
    print(f"wrote {output}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
