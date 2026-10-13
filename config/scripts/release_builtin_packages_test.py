"""The built-in package set of a release: signed, listed in the release
manifest, and covered by the signed checksums."""

from __future__ import annotations

import base64
import hashlib
import json
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parents[2]
BUILDER = REPO_ROOT / "packages" / "shared" / "build_package.py"
sys.path.insert(0, str(Path(__file__).resolve().parent))
from generate_release_manifest import builtin_package_entries  # noqa: E402
from verify_release_artifacts import VerificationError, verify_checksums_signature  # noqa: E402

VERSION = "5.0.0-rc.1"
PACKAGES = ("identity-platform", "order")
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


class ReleaseBuiltinPackagesTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.temporary = tempfile.TemporaryDirectory()
        root = Path(cls.temporary.name)
        cls.root = root
        cls.key, cls.official = throwaway_key(root, "official")
        cls.dist = root / "dist"
        arguments = []
        for package in PACKAGES:
            arguments += ["--package", package]
        result = run(
            *arguments, "--builtin", "--version", VERSION, "--out", str(cls.dist),
            "--formal-release", "--signing-key", str(cls.key), "--official-public-key", str(cls.official),
        )  # fmt: skip
        if result.returncode != 0:
            raise AssertionError(result.stderr or result.stdout)

    @classmethod
    def tearDownClass(cls) -> None:
        cls.temporary.cleanup()

    def test_release_binaries_embed_a_verified_set(self) -> None:
        arguments = []
        for package in PACKAGES:
            arguments += ["--package", package]
        verified = run(*arguments, "--builtin", "--version", VERSION, "--out", str(self.dist), "--verify-release", "--official-public-key", str(self.official))
        self.assertEqual(0, verified.returncode, verified.stderr or verified.stdout)
        _, other = throwaway_key(self.root, "other")
        refused = run(*arguments, "--builtin", "--version", VERSION, "--out", str(self.dist), "--verify-release", "--official-public-key", str(other))
        self.assertNotEqual(0, refused.returncode)

    def test_release_manifest_lists_each_built_in_package(self) -> None:
        entries = builtin_package_entries(self.dist)
        self.assertEqual(list(PACKAGES), [entry["id"] for entry in entries])
        for entry in entries:
            stem = f"{entry['id']}-{VERSION}"
            manifest_bytes = (self.dist / f"{stem}.manifest.json").read_bytes()
            artifact = (self.dist / f"{stem}.anxp").read_bytes()
            self.assertEqual(VERSION, entry["version"])
            self.assertEqual(json.loads(manifest_bytes)["artifact_sha256"], entry["sha256"])
            self.assertEqual(hashlib.sha256(artifact).hexdigest(), entry["sha256"])
            self.assertEqual(len(artifact), entry["size"])
            self.assertEqual(hashlib.sha256(manifest_bytes).hexdigest(), entry["manifest_sha256"])

    def test_checksums_are_signed_with_the_package_key(self) -> None:
        # The release job signs SHA256SUMS.txt with the same key and openssl
        # step as every package manifest.
        checksums = self.root / "SHA256SUMS.txt"
        checksums.write_text("0" * 64 + "  anix-control-linux-amd64.tar.gz\n", encoding="utf-8")
        signature_bin = self.root / "checksums.bin"
        subprocess.run(
            ["openssl", "pkeyutl", "-sign", "-inkey", str(self.key), "-rawin", "-in", str(checksums), "-out", str(signature_bin)],
            check=True, capture_output=True,
        )  # fmt: skip
        signature = self.root / "SHA256SUMS.txt.sig"
        signature.write_bytes(base64.b64encode(signature_bin.read_bytes()) + b"\n")
        verify_checksums_signature(checksums, signature, self.official)
        _, other = throwaway_key(self.root, "checksums-other")
        with self.assertRaises(VerificationError):
            verify_checksums_signature(checksums, signature, other)


if __name__ == "__main__":
    unittest.main()
