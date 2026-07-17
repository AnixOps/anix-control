#!/usr/bin/env python3
"""Require a release tag to match every shipped Control version surface."""

from __future__ import annotations

import argparse
import json
import re
import tempfile
from pathlib import Path


TAG_PATTERN = re.compile(r"^v(?P<version>[0-9]+\.[0-9]+\.[0-9]+(?:-(?:alpha|beta|rc)(?:\.[0-9]+)?)?)$")


def require_pattern(path: Path, pattern: str, expected: str, label: str) -> None:
    content = path.read_text(encoding="utf-8")
    match = re.search(pattern, content, re.MULTILINE)
    actual = match.group(1) if match else "<missing>"
    if actual != expected:
        raise ValueError(f"{label} version mismatch: expected {expected}, found {actual}")


def require_all_patterns(
    path: Path,
    pattern: str,
    expected: str,
    label: str,
    expected_count: int | None = None,
) -> None:
    content = path.read_text(encoding="utf-8")
    matches = re.findall(pattern, content, re.MULTILINE)
    if not matches:
        raise ValueError(f"{label} version is missing")
    if expected_count is not None and len(matches) != expected_count:
        raise ValueError(f"{label} version field count mismatch: expected {expected_count}, found {len(matches)}")
    mismatches = [actual for actual in matches if actual != expected]
    if mismatches:
        raise ValueError(f"{label} version mismatch: expected {expected}, found {mismatches[0]}")


def require_json_version(path: Path, keys: tuple[str, ...], expected: str, label: str) -> None:
    value: object = json.loads(path.read_text(encoding="utf-8"))
    for key in keys:
        if not isinstance(value, dict) or key not in value:
            raise ValueError(f"{label} version field is missing: {'.'.join(keys)}")
        value = value[key]
    if value != expected:
        raise ValueError(f"{label} version mismatch: expected {expected}, found {value}")


def check_release_version(repo_root: Path, tag: str) -> str:
    match = TAG_PATTERN.fullmatch(tag)
    if match is None:
        raise ValueError(f"unsupported release tag: {tag}")
    version = match.group("version")

    require_pattern(
        repo_root / "internal/branding/branding.go",
        r'^\s*DefaultVersion\s*=\s*"([^"]+)"',
        version,
        "Control branding",
    )
    require_pattern(repo_root / "Makefile", r"^VERSION\s*:=\s*(\S+)", version, "Makefile")
    require_pattern(
        repo_root / "cmd/server/main.go",
        r"^//\s*@version\s+(\S+)",
        version,
        "Swagger annotation",
    )
    require_json_version(repo_root / "web/package.json", ("version",), version, "frontend package")
    require_json_version(repo_root / "web/package-lock.json", ("version",), version, "frontend lockfile")
    require_json_version(
        repo_root / "web/package-lock.json",
        ("packages", "", "version"),
        version,
        "frontend lockfile root package",
    )
    require_json_version(repo_root / "docs/swagger.json", ("info", "version"), version, "Swagger output")
    require_pattern(
        repo_root / "config/config.yaml.example",
        r'^\s{2}version:\s*"([^"]+)"',
        version,
        "default configuration",
    )
    for config_name in ("config/config.prod.yaml", "config/config.dev.yaml.example"):
        require_pattern(
            repo_root / config_name,
            r'^\s{2}version:\s*"([^"]+)"',
            version,
            config_name,
        )
    require_all_patterns(
        repo_root / "install.sh",
        r'^\s{2}version:\s*"([^"]+)"',
        version,
        "installer configuration",
        expected_count=2,
    )
    require_pattern(
        repo_root / "docs/docs.go",
        r'^\s*Version:\s*"([^"]+)"',
        version,
        "generated Swagger Go output",
    )
    require_pattern(
        repo_root / "docs/swagger.yaml",
        r'^\s{2}version:\s*([^\s]+)',
        version,
        "generated Swagger YAML output",
    )
    require_pattern(
        repo_root / "CHANGELOG.md",
        r"^##\s+([^\s]+)\s+-\s+\d{4}-\d{2}-\d{2}$",
        version,
        "changelog",
    )
    require_pattern(
        repo_root / "README.md",
        r"^- Current preview:\s+`v([^`]+)`",
        version,
        "README preview",
    )
    return version


def write_fixture(root: Path, version: str) -> None:
    files = {
        "internal/branding/branding.go": f'package branding\nconst (\n\tDefaultVersion = "{version}"\n)\n',
        "Makefile": f"VERSION := {version}\n",
        "cmd/server/main.go": f"// @version {version}\npackage main\n",
        "web/package.json": json.dumps({"version": version}),
        "web/package-lock.json": json.dumps({"version": version, "packages": {"": {"version": version}}}),
        "docs/swagger.json": json.dumps({"info": {"version": version}}),
        "config/config.yaml.example": f'app:\n  version: "{version}"\n',
        "config/config.prod.yaml": f'app:\n  version: "{version}"\n',
        "config/config.dev.yaml.example": f'app:\n  version: "{version}"\n',
        "install.sh": f'  version: "{version}"\n  version: "{version}"\n',
        "docs/docs.go": f'\tVersion: "{version}",\n',
        "docs/swagger.yaml": f'  version: {version}\n',
        "CHANGELOG.md": f"# Changelog\n\n## {version} - 2026-07-17\n",
        "README.md": f"- Current preview: `v{version}`\n",
    }
    for relative, content in files.items():
        path = root / relative
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(content, encoding="utf-8")


def self_test() -> None:
    version = "4.0.0-alpha.3"
    with tempfile.TemporaryDirectory(prefix="anix-release-version-") as tmp:
        root = Path(tmp)

        def expect_failure(fragment: str) -> None:
            try:
                check_release_version(root, f"v{version}")
            except ValueError as error:
                assert fragment in str(error)
            else:
                raise AssertionError(f"version mismatch was accepted: {fragment}")

        write_fixture(root, version)
        assert check_release_version(root, f"v{version}") == version

        package = root / "web/package.json"
        package.write_text(json.dumps({"version": "4.0.0-alpha.4"}), encoding="utf-8")
        expect_failure("frontend package version mismatch")

        write_fixture(root, version)
        installer = root / "install.sh"
        installer.write_text(
            f'  version: "{version}"\n  version: "4.0.0-alpha.4"\n',
            encoding="utf-8",
        )
        expect_failure("installer configuration version mismatch")

        write_fixture(root, version)
        installer.write_text(
            f'  version: "{version}"\n  version: {version}\n',
            encoding="utf-8",
        )
        expect_failure("installer configuration version field count mismatch")

        try:
            check_release_version(root, "v4")
        except ValueError as error:
            assert "unsupported release tag" in str(error)
        else:
            raise AssertionError("invalid release tag was accepted")
    print("release version self-test passed")


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--tag", help="Release tag, including the leading v")
    parser.add_argument("--repo-root", type=Path, default=Path(__file__).resolve().parents[2])
    parser.add_argument("--self-test", action="store_true")
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    if args.self_test:
        self_test()
        return 0
    if not args.tag:
        raise SystemExit("--tag is required")
    version = check_release_version(args.repo_root.resolve(), args.tag)
    print(f"release version surfaces match {version}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
