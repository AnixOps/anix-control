#!/usr/bin/env python3
"""Verify an NAT Egress release signature with an Ed25519 public key."""

from __future__ import annotations

import argparse
import base64
import binascii
import hashlib
import json
import re
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path


ED25519_PUBLIC_KEY_BYTES = 32
ED25519_SIGNATURE_BYTES = 64
ED25519_SPKI_PREFIX = bytes.fromhex("302a300506032b6570032100")


class SignatureError(RuntimeError):
    """Raised when public-key signature verification cannot be completed."""


def decode_base64(data: bytes, context: str) -> bytes:
    compact = b"".join(data.split())
    try:
        return base64.b64decode(compact, validate=True)
    except (ValueError, binascii.Error) as exc:
        raise SignatureError(f"{context} must be valid Base64") from exc


def public_key_pem(data: bytes) -> bytes:
    stripped = data.strip()
    if stripped.startswith(b"-----BEGIN PUBLIC KEY-----"):
        if b"-----END PUBLIC KEY-----" not in stripped:
            raise SignatureError("public key PEM is incomplete")
        return stripped + b"\n"

    raw = decode_base64(stripped, "public key")
    if len(raw) != ED25519_PUBLIC_KEY_BYTES:
        raise SignatureError("raw Ed25519 public key must decode to 32 bytes")
    der = ED25519_SPKI_PREFIX + raw
    encoded = base64.b64encode(der).decode("ascii")
    lines = [encoded[index : index + 64] for index in range(0, len(encoded), 64)]
    return (
        "-----BEGIN PUBLIC KEY-----\n"
        + "\n".join(lines)
        + "\n-----END PUBLIC KEY-----\n"
    ).encode("ascii")


def signature_bytes(data: bytes) -> bytes:
    signature = decode_base64(data, "signature")
    if len(signature) != ED25519_SIGNATURE_BYTES:
        raise SignatureError("Ed25519 signature must decode to 64 bytes")
    return signature


def verify_release_signature(
    manifest_path: Path,
    artifact_path: Path,
    signature_path: Path,
    public_key_path: Path,
    openssl: str = "openssl",
) -> None:
    if shutil.which(openssl) is None:
        raise SignatureError(f"OpenSSL executable not found: {openssl}")
    try:
        manifest = manifest_path.read_bytes()
        artifact = artifact_path.read_bytes()
        signature = signature_bytes(signature_path.read_bytes())
        public_key = public_key_pem(public_key_path.read_bytes())
    except OSError as exc:
        raise SignatureError(f"read signature input: {exc}") from exc
    if not manifest:
        raise SignatureError("manifest must not be empty")
    try:
        manifest_value = json.loads(manifest)
    except (UnicodeDecodeError, json.JSONDecodeError) as exc:
        raise SignatureError(f"manifest must be valid JSON: {exc}") from exc
    expected_artifact = manifest_value.get("artifact_sha256") if isinstance(manifest_value, dict) else None
    if not isinstance(expected_artifact, str) or not re.fullmatch(r"[0-9a-f]{64}", expected_artifact):
        raise SignatureError("manifest artifact_sha256 is invalid")
    if hashlib.sha256(artifact).hexdigest() != expected_artifact:
        raise SignatureError("package artifact SHA-256 does not match the signed manifest")

    with tempfile.TemporaryDirectory(prefix="anixops-plugin-signature-") as temporary:
        root = Path(temporary)
        normalized_public_key = root / "public-key.pem"
        raw_signature = root / "signature.bin"
        normalized_public_key.write_bytes(public_key)
        raw_signature.write_bytes(signature)
        try:
            result = subprocess.run(
                [
                    openssl,
                    "pkeyutl",
                    "-verify",
                    "-pubin",
                    "-inkey",
                    str(normalized_public_key),
                    "-rawin",
                    "-in",
                    str(manifest_path),
                    "-sigfile",
                    str(raw_signature),
                ],
                check=False,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True,
            )
        except OSError as exc:
            raise SignatureError(f"run OpenSSL verifier: {exc}") from exc
    if result.returncode != 0:
        detail = (result.stderr or result.stdout).strip()
        if detail:
            raise SignatureError(f"Ed25519 signature verification failed: {detail}")
        raise SignatureError("Ed25519 signature verification failed")


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--manifest", required=True, type=Path, help="Canonical manifest.json signed by the release key.")
    parser.add_argument("--artifact", required=True, type=Path, help="Package artifact bound by manifest artifact_sha256.")
    parser.add_argument("--signature", required=True, type=Path, help="Base64-encoded Ed25519 signature file.")
    parser.add_argument(
        "--public-key",
        required=True,
        type=Path,
        help="Ed25519 public key as PEM or the same raw Base64 value used by plugins.official_public_key.",
    )
    parser.add_argument("--openssl", default="openssl", help="OpenSSL executable (default: openssl).")
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    try:
        verify_release_signature(args.manifest, args.artifact, args.signature, args.public_key, args.openssl)
    except SignatureError as exc:
        print(f"{Path(__file__).name}: error: {exc}", file=sys.stderr)
        return 2
    print(f"verified signature for {args.manifest}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
