from __future__ import annotations

import base64
import importlib.util
import sys
import unittest
from pathlib import Path


PACKAGE_ROOT = Path(__file__).resolve().parents[1]
SPEC = importlib.util.spec_from_file_location(
    "gost_mesh_verify_signature",
    PACKAGE_ROOT / "verify_signature.py",
)
if SPEC is None or SPEC.loader is None:
    raise RuntimeError("cannot load gost-mesh signature verifier")
VERIFY = importlib.util.module_from_spec(SPEC)
sys.modules[SPEC.name] = VERIFY
SPEC.loader.exec_module(VERIFY)


class GostMeshSignatureVerifierTest(unittest.TestCase):
    def test_raw_public_key_is_wrapped_as_ed25519_spki(self) -> None:
        raw = bytes(range(VERIFY.ED25519_PUBLIC_KEY_BYTES))
        pem = VERIFY.public_key_pem(base64.b64encode(raw))
        encoded = b"".join(
            line
            for line in pem.splitlines()
            if not line.startswith(b"-----")
        )
        self.assertEqual(
            VERIFY.ED25519_SPKI_PREFIX + raw,
            base64.b64decode(encoded, validate=True),
        )

    def test_signature_must_be_base64_and_exact_length(self) -> None:
        raw = bytes(range(VERIFY.ED25519_SIGNATURE_BYTES))
        self.assertEqual(raw, VERIFY.signature_bytes(base64.b64encode(raw)))
        with self.assertRaisesRegex(VERIFY.SignatureError, "valid Base64"):
            VERIFY.signature_bytes(b"not-base64!")
        with self.assertRaisesRegex(VERIFY.SignatureError, "64 bytes"):
            VERIFY.signature_bytes(base64.b64encode(b"short"))


if __name__ == "__main__":
    unittest.main()
