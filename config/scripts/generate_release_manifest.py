#!/usr/bin/env python3
"""Generate the machine-readable GitHub Actions release manifest."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import tempfile
from pathlib import Path
from typing import Iterable


MANIFEST_NAME = "RELEASE_MANIFEST.json"
CHECKSUM_NAME = "SHA256SUMS.txt"


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def release_files(release_dir: Path, output: Path) -> Iterable[Path]:
    excluded = {CHECKSUM_NAME, MANIFEST_NAME, output.name}
    for path in sorted(release_dir.iterdir(), key=lambda item: item.name):
        if path.is_file() and path.name not in excluded:
            yield path


def build_manifest(
    *,
    release_dir: Path,
    output: Path,
    project: str,
    build_source: str,
    manual_deployment_required: bool,
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
    }


def write_manifest(manifest: dict[str, object], output: Path) -> None:
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(manifest, indent=2, sort_keys=True) + "\n", encoding="utf-8")


def run_self_test() -> None:
    with tempfile.TemporaryDirectory() as tmp:
        release_dir = Path(tmp) / "release"
        release_dir.mkdir()
        (release_dir / "b.txt").write_text("second\n", encoding="utf-8")
        (release_dir / "a.bin").write_bytes(b"first")
        (release_dir / CHECKSUM_NAME).write_text("old checksum\n", encoding="utf-8")
        (release_dir / MANIFEST_NAME).write_text("old manifest\n", encoding="utf-8")
        (release_dir / "ignored-dir").mkdir()

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
        )
        write_manifest(manifest, output)
        loaded = json.loads(output.read_text(encoding="utf-8"))

        names = [artifact["name"] for artifact in loaded["artifacts"]]
        assert names == ["a.bin", "b.txt"], names
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
    )
    write_manifest(manifest, output)
    print(f"wrote {output}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
