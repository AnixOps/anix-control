"""The release packages archive: build, sign, verify and list in the manifest."""

from __future__ import annotations

import base64
import hashlib
import json
import os
import subprocess
import sys
import tarfile
import tempfile
import unittest
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parents[2]
BUILDER = REPO_ROOT / "packages" / "shared" / "build_package.py"
sys.path.insert(0, str(Path(__file__).resolve().parent))
from generate_release_manifest import release_package_entries  # noqa: E402

VERSION = "4.1.0-rc.3"
PACKAGE = "identity-platform"
ARCHIVE = f"anix-control-packages-{VERSION}.tar.gz"
ED25519_SPKI_PREFIX = bytes.fromhex("302a300506032b6570032100")


def run(*arguments: str) -> subprocess.CompletedProcess[str]:
    env = dict(os.environ, GOWORK="off")
    return subprocess.run(
        [sys.executable, str(BUILDER), *arguments], cwd=REPO_ROOT, env=env, check=False, text=True, capture_output=True
    )


def throwaway_key(root: Path, name: str) -> tuple[Path, Path]:
    private_key = root / f"{name}.pem"
    subprocess.run(["openssl", "genpkey", "-algorithm", "ED25519", "-out", str(private_key)], check=True, capture_output=True)
    os.chmod(private_key, 0o600)
    der = subprocess.run(
        ["openssl", "pkey", "-in", str(private_key), "-pubout", "-outform", "DER"], check=True, capture_output=True
    ).stdout
    official = root / f"{name}.raw"
    official.write_bytes(base64.b64encode(der[len(ED25519_SPKI_PREFIX) :]) + b"\n")
    return private_key, official


class ReleasePackagesArchiveTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.temporary = tempfile.TemporaryDirectory()
        root = Path(cls.temporary.name)
        cls.root = root
        cls.key, cls.official = throwaway_key(root, "official")
        cls.packages = root / "packages"
        result = run("--package", PACKAGE, "--version", VERSION, "--out", str(cls.packages), "--signing-key", str(cls.key))
        if result.returncode != 0:
            raise AssertionError(result.stderr or result.stdout)

    @classmethod
    def tearDownClass(cls) -> None:
        cls.temporary.cleanup()

    def archive(self, name: str) -> Path:
        release = self.root / name
        result = run(
            "--package", PACKAGE, "--version", VERSION, "--out", str(self.packages),
            "--release-archive", str(release), "--signing-key", str(self.key), "--official-public-key", str(self.official),
        )  # fmt: skip
        self.assertEqual(0, result.returncode, result.stderr or result.stdout)
        return release / ARCHIVE

    def verify(self, archive: Path, official: Path | None = None) -> subprocess.CompletedProcess[str]:
        return run(
            "--package", PACKAGE, "--version", VERSION,
            "--verify-release-archive", str(archive), "--official-public-key", str(official or self.official),
        )  # fmt: skip

    def test_archive_holds_each_package_once_without_per_package_keys(self) -> None:
        archive = self.archive("release")
        self.assertEqual(sorted(path.name for path in archive.parent.iterdir()), [ARCHIVE, f"{ARCHIVE}.sig"])
        with tarfile.open(archive, mode="r:gz") as handle:
            names = handle.getnames()
        top = f"anix-control-packages-{VERSION}"
        self.assertEqual(
            [
                f"{top}/{PACKAGE}-{VERSION}.anxp",
                f"{top}/{PACKAGE}-{VERSION}.manifest.json",
                f"{top}/{PACKAGE}-{VERSION}.manifest.sig",
                f"{top}/{PACKAGE}-{VERSION}.sbom.spdx.json",
                f"{top}/official-public-key.pem",
            ],
            names,
        )
        result = self.verify(archive)
        self.assertEqual(0, result.returncode, result.stderr or result.stdout)

    def test_archive_is_reproducible(self) -> None:
        first = self.archive("first").read_bytes()
        second = self.archive("second").read_bytes()
        self.assertEqual(hashlib.sha256(first).hexdigest(), hashlib.sha256(second).hexdigest())

    def test_signature_is_checked_against_the_official_root(self) -> None:
        archive = self.archive("other-root")
        _, other = throwaway_key(self.root, "other")
        result = self.verify(archive, other)
        self.assertNotEqual(0, result.returncode)
        self.assertIn("release archive signature", result.stderr)

    def test_tampered_archive_fails(self) -> None:
        archive = self.archive("tampered")
        data = bytearray(archive.read_bytes())
        data[-1] ^= 0xFF
        archive.write_bytes(bytes(data))
        self.assertNotEqual(0, self.verify(archive).returncode)

    def test_signing_key_must_match_the_official_root(self) -> None:
        other_key, _ = throwaway_key(self.root, "mismatch")
        result = run(
            "--package", PACKAGE, "--version", VERSION, "--out", str(self.packages),
            "--release-archive", str(self.root / "mismatch"), "--signing-key", str(other_key),
            "--official-public-key", str(self.official),
        )  # fmt: skip
        self.assertNotEqual(0, result.returncode)
        self.assertFalse((self.root / "mismatch" / ARCHIVE).exists())

    def test_release_manifest_lists_each_package(self) -> None:
        archive = self.archive("manifest")
        manifest = json.loads((self.packages / f"{PACKAGE}-{VERSION}.manifest.json").read_text(encoding="utf-8"))
        anxp = (self.packages / f"{PACKAGE}-{VERSION}.anxp").read_bytes()
        entries = release_package_entries(archive.parent)
        self.assertEqual(
            [
                {
                    "id": PACKAGE,
                    "version": VERSION,
                    "archive": ARCHIVE,
                    "artifact": f"{PACKAGE}-{VERSION}.anxp",
                    "size": len(anxp),
                    "sha256": manifest["artifact_sha256"],
                    "manifest_sha256": hashlib.sha256(
                        (self.packages / f"{PACKAGE}-{VERSION}.manifest.json").read_bytes()
                    ).hexdigest(),
                }
            ],
            entries,
        )


if __name__ == "__main__":
    unittest.main()
