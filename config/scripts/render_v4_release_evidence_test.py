from __future__ import annotations

import base64
import hashlib
import importlib.util
import io
import json
import os
import shutil
import subprocess
import sys
import tarfile
import tempfile
import unittest
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parents[2]
SCRIPTS_ROOT = REPO_ROOT / "config" / "scripts"
RENDERER_PATH = SCRIPTS_ROOT / "render_v4_release_evidence.py"
VERIFIER_PATH = SCRIPTS_ROOT / "verify_v4_evidence.py"
APPROVAL_CREATOR_PATH = SCRIPTS_ROOT / "create_v4_approval.py"
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
ED25519_SPKI_PREFIX = bytes.fromhex("302a300506032b6570032100")


def load_module(name: str, path: Path):
    spec = importlib.util.spec_from_file_location(name, path)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"cannot load {path}")
    module = importlib.util.module_from_spec(spec)
    sys.modules[name] = module
    spec.loader.exec_module(module)
    return module


class V4ReleaseEvidenceTest(unittest.TestCase):
    def modules(self):
        if not RENDERER_PATH.is_file():
            self.fail("v4 release evidence renderer is missing")
        if not VERIFIER_PATH.is_file():
            self.fail("v4 release evidence verifier is missing")
        return load_module("v4_release_renderer", RENDERER_PATH), load_module("v4_evidence_verifier", VERIFIER_PATH)

    def write_fixture(self, root: Path) -> tuple[Path, Path, Path, Path, Path, Path]:
        packages = root / "v4-packages"
        packages.mkdir(exist_ok=True)
        artifact_digests: list[dict[str, str]] = []
        for package_id in PACKAGE_IDS:
            stem = f"{package_id}-4.0.0"
            artifact = packages / f"{stem}.anxp"
            artifact.write_bytes(f"artifact:{package_id}\n".encode("utf-8"))
            digest = hashlib.sha256(artifact.read_bytes()).hexdigest()
            artifact_digests.append({"id": package_id, "artifact_sha256": digest})
            (packages / f"{stem}.manifest.json").write_text(
                json.dumps({"id": package_id, "version": "4.0.0", "artifact_sha256": digest}), encoding="utf-8"
            )
            (packages / f"{stem}.manifest.sig").write_text("signature\n", encoding="utf-8")
            (packages / f"{stem}.public-key.pem").write_text("public-key\n", encoding="utf-8")
            (packages / f"{stem}.sbom.spdx.json").write_text(
                json.dumps({"artifact_sha256": digest}), encoding="utf-8"
            )
        signing_key = root / "official-signing-key.pem"
        subprocess.run(
            ["openssl", "genpkey", "-algorithm", "ED25519", "-out", str(signing_key)],
            check=True,
            capture_output=True,
        )
        signing_key.chmod(0o600)
        public_key = subprocess.run(
            ["openssl", "pkey", "-in", str(signing_key), "-pubout", "-outform", "DER"],
            check=True,
            capture_output=True,
        ).stdout
        self.assertEqual(ED25519_SPKI_PREFIX, public_key[:-32])
        official_root = packages / "official-public-key.raw"
        official_root.write_text(base64.b64encode(public_key[-32:]).decode("ascii") + "\n", encoding="utf-8")

        subject = hashlib.sha256(
            json.dumps(
                {"tag": "v4.0.0", "packages": artifact_digests},
                ensure_ascii=True,
                separators=(",", ":"),
                sort_keys=True,
            ).encode("utf-8")
        ).hexdigest()

        def write_approval(kind: str) -> Path:
            approval = {
                "schema": "anixops.v4.release-approval/v1",
                "kind": kind,
                "tag": "v4.0.0",
                "release_subject_sha256": subject,
                "approved": True,
            }
            payload = json.dumps(approval, ensure_ascii=True, separators=(",", ":"), sort_keys=True).encode("utf-8")
            payload_path = root / f"{kind}-approval.payload"
            signature_path = root / f"{kind}-approval.signature"
            payload_path.write_bytes(payload)
            subprocess.run(
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
                check=True,
                capture_output=True,
            )
            approval["signature"] = base64.b64encode(signature_path.read_bytes()).decode("ascii")
            path = root / f"{kind}.json"
            path.write_text(json.dumps(approval, ensure_ascii=True, separators=(",", ":"), sort_keys=True) + "\n", encoding="utf-8")
            return path

        canary = write_approval("canary")
        support = write_approval("support")
        output = root / "v4-rehearsal-evidence.json"
        return packages, official_root, canary, support, output, signing_key

    @staticmethod
    def successful_results(root: Path) -> dict[str, dict[str, object]]:
        transcripts = root / "transcripts"
        transcripts.mkdir(exist_ok=True)
        results: dict[str, dict[str, object]] = {}
        for name in (
            "official_root_verification",
            "release_stage_gate",
            "v2_route_catalog_gate",
            "plugin_only_route_gate",
            "websocket_relay_tests",
            "v2_compatibility_tests",
            "sqlite_rehearsal",
            "postgres_rehearsal",
        ):
            transcript = transcripts / f"{name}.log"
            transcript.write_text(f"{name} passed\n", encoding="utf-8")
            results[name] = {
                "passed": True,
                "transcript": f"transcripts/{transcript.name}",
                "transcript_sha256": hashlib.sha256(transcript.read_bytes()).hexdigest(),
            }
        return results

    def render_fixture_evidence(self, renderer, root: Path) -> dict[str, object]:
        packages, official_root, canary, support, output, _ = self.write_fixture(root)
        return renderer.render_evidence(
            tag="v4.0.0",
            packages_dir=packages,
            official_public_key=official_root,
            canary_evidence=canary,
            support_evidence=support,
            output=output,
            results=self.successful_results(root),
        )

    @staticmethod
    def trusted_public_key(verifier, root: Path) -> bytes:
        return verifier.load_trusted_official_public_key(root / "v4-packages" / "official-public-key.raw")

    def test_release_evidence_requires_identity_platform_and_v2_route_gate(self) -> None:
        renderer, verifier = self.modules()
        with tempfile.TemporaryDirectory() as temporary:
            evidence = self.render_fixture_evidence(renderer, Path(temporary))
            evidence["package_artifacts"] = [
                package for package in evidence["package_artifacts"] if package["id"] != "identity-platform"
            ]
            with self.assertRaisesRegex(verifier.EvidenceError, "identity-platform"):
                verifier.validate_v4_evidence(
                    evidence, Path(temporary) / "v4-rehearsal-evidence.json", self.trusted_public_key(verifier, Path(temporary))
                )

            evidence = self.render_fixture_evidence(renderer, Path(temporary))
            evidence["v2_route_catalog_gate"]["passed"] = False
            with self.assertRaisesRegex(verifier.EvidenceError, "v2 route catalog gate"):
                verifier.validate_v4_evidence(
                    evidence, Path(temporary) / "v4-rehearsal-evidence.json", self.trusted_public_key(verifier, Path(temporary))
                )

    def test_release_evidence_requires_postgres_canary_and_support_results(self) -> None:
        renderer, verifier = self.modules()
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            for field, expected_error in (
                ("postgres_rehearsal", "postgres rehearsal"),
                ("canary_evidence", "canary evidence"),
                ("support_evidence", "support evidence"),
            ):
                evidence = self.render_fixture_evidence(renderer, root)
                if field == "postgres_rehearsal":
                    evidence[field]["passed"] = False
                else:
                    evidence.pop(field)
                with self.subTest(field=field):
                    with self.assertRaisesRegex(verifier.EvidenceError, expected_error):
                        verifier.validate_v4_evidence(evidence, root / "v4-rehearsal-evidence.json", self.trusted_public_key(verifier, root))

    def test_release_evidence_requires_materialized_transcript_files(self) -> None:
        renderer, verifier = self.modules()
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            evidence = self.render_fixture_evidence(renderer, root)
            evidence["release_stage_gate"]["transcript"] = "transcripts/missing.log"

            with self.assertRaisesRegex(verifier.EvidenceError, "transcript"):
                verifier.validate_v4_evidence(evidence, root / "v4-rehearsal-evidence.json", self.trusted_public_key(verifier, root))

    def test_release_evidence_rejects_an_unbound_canary_record(self) -> None:
        renderer, verifier = self.modules()
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            evidence = self.render_fixture_evidence(renderer, root)
            unbound = root / "unbound-canary.json"
            unbound.write_text('{"status":"passed"}\n', encoding="utf-8")
            evidence["canary_evidence"] = {
                "path": unbound.name,
                "sha256": hashlib.sha256(unbound.read_bytes()).hexdigest(),
            }

            with self.assertRaisesRegex(verifier.EvidenceError, "canary approval"):
                verifier.validate_v4_evidence(evidence, root / "v4-rehearsal-evidence.json", self.trusted_public_key(verifier, root))

    def test_release_evidence_rejects_a_bundle_owned_root(self) -> None:
        renderer, verifier = self.modules()
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            evidence = self.render_fixture_evidence(renderer, root)

            with self.assertRaisesRegex(verifier.EvidenceError, "does not match the trusted official root"):
                verifier.validate_v4_evidence(evidence, root / "v4-rehearsal-evidence.json", bytes(range(32)))

    def test_verifier_rejects_a_symlinked_trust_root(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            root_file = root / "official-public-key.raw"
            root_file.write_text(base64.b64encode(bytes(range(32))).decode("ascii") + "\n", encoding="utf-8")
            symlink = root / "trusted-root-link"
            symlink.symlink_to(root_file)
            evidence = root / "v4-rehearsal-evidence.json"
            evidence.write_text("{}\n", encoding="utf-8")
            result = subprocess.run(
                [
                    sys.executable,
                    str(VERIFIER_PATH),
                    "--input",
                    str(evidence),
                    "--trusted-official-public-key",
                    str(symlink),
                ],
                check=False,
                capture_output=True,
                text=True,
            )

            self.assertNotEqual(0, result.returncode)
            self.assertIn("trusted official public key must be a regular file", result.stderr)

    def test_release_evidence_requires_a_detached_official_signature(self) -> None:
        renderer, verifier = self.modules()
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            output = root / "v4-rehearsal-evidence.json"
            output.write_text("{}\n", encoding="utf-8")

            with self.assertRaisesRegex(verifier.EvidenceError, "evidence signature"):
                verifier.verify_evidence_signature(
                    output,
                    output.with_suffix(output.suffix + ".sig"),
                    bytes(range(32)),
                )

    def test_evidence_archive_rejects_path_escape_before_unpacking(self) -> None:
        _, verifier = self.modules()
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            archive = root / "malicious-evidence.tar.gz"
            with tarfile.open(archive, "w:gz") as bundle:
                member = tarfile.TarInfo("../outside")
                member.size = 1
                bundle.addfile(member, io.BytesIO(b"x"))

            with self.assertRaisesRegex(verifier.EvidenceError, "archive member"):
                verifier.safe_extract_evidence_archive(archive, root / "unpacked")

    def test_evidence_archive_rejects_pax_and_gnu_metadata_before_materializing_it(self) -> None:
        _, verifier = self.modules()
        previous_limit = verifier.MAX_ARCHIVE_UNPACKED_BYTES
        verifier.MAX_ARCHIVE_UNPACKED_BYTES = 128
        try:
            with tempfile.TemporaryDirectory() as temporary:
                root = Path(temporary)
                for archive_format, name, pax_headers in (
                    (tarfile.PAX_FORMAT, "entry", {"comment": "x" * 1024}),
                    (tarfile.GNU_FORMAT, "/".join(["part"] * 80), {}),
                ):
                    with self.subTest(archive_format=archive_format):
                        archive = root / f"metadata-{archive_format}.tar.gz"
                        member = tarfile.TarInfo(name)
                        member.size = 1
                        member.pax_headers = pax_headers
                        with tarfile.open(archive, "w:gz", format=archive_format) as bundle:
                            bundle.addfile(member, io.BytesIO(b"x"))

                        with self.assertRaisesRegex(verifier.EvidenceError, "unpacked size limit"):
                            verifier.safe_extract_evidence_archive(archive, root / f"unpacked-{archive_format}")
        finally:
            verifier.MAX_ARCHIVE_UNPACKED_BYTES = previous_limit

    def test_approval_creator_signs_the_exact_package_cohort(self) -> None:
        _, verifier = self.modules()
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            packages, official_root, _, _, _, signing_key = self.write_fixture(root)
            output = root / "generated-canary-approval.json"
            subprocess.run(
                [
                    sys.executable,
                    str(APPROVAL_CREATOR_PATH),
                    "--kind",
                    "canary",
                    "--packages-dir",
                    str(packages),
                    "--signing-key",
                    str(signing_key),
                    "--trusted-official-public-key",
                    str(official_root),
                    "--output",
                    str(output),
                ],
                check=True,
                capture_output=True,
                text=True,
            )
            verifier.load_and_validate_approval(
                output,
                kind="canary",
                release_subject_sha256=verifier.release_subject_digest_from_package_directory(packages),
                trusted_official_public_key=self.trusted_public_key(verifier, root),
                label="canary approval",
            )

    def test_release_package_directory_must_match_the_verified_evidence_set(self) -> None:
        renderer, verifier = self.modules()
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            evidence = self.render_fixture_evidence(renderer, root)
            release = root / "release"
            release.mkdir()
            packages = root / "v4-packages"
            for package_id in PACKAGE_IDS:
                stem = f"{package_id}-4.0.0"
                for suffix in (".anxp", ".manifest.json", ".manifest.sig", ".public-key.pem", ".sbom.spdx.json"):
                    shutil.copy2(packages / f"{stem}{suffix}", release / f"{stem}{suffix}")
            shutil.copy2(packages / "official-public-key.raw", release / "official-public-key.raw")
            (release / "identity-platform-4.0.0.anxp").write_bytes(b"not-the-evidence-artifact\n")

            with self.assertRaisesRegex(verifier.EvidenceError, "does not match the verified evidence"):
                verifier.verify_matching_package_directory(evidence, root / "v4-rehearsal-evidence.json", release)

    def test_renderer_normalizes_an_external_trusted_root_into_the_evidence_directory(self) -> None:
        renderer, _ = self.modules()
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            packages, official_root, canary, support, _, _ = self.write_fixture(root)
            evidence_root = root / "evidence"
            shutil.copytree(packages, evidence_root / "v4-packages")
            external_root = root / "operator-trusted-root.raw"
            shutil.copy2(official_root, external_root)
            output = evidence_root / "v4-rehearsal-evidence.json"

            evidence = renderer.render_evidence(
                tag="v4.0.0",
                packages_dir=evidence_root / "v4-packages",
                official_public_key=external_root,
                canary_evidence=canary,
                support_evidence=support,
                output=output,
                results=self.successful_results(evidence_root),
            )

            self.assertEqual("official-public-key.raw", evidence["official_root_verification"]["public_key"])
            self.assertTrue((evidence_root / "official-public-key.raw").is_file())

    def test_rehearsal_environment_excludes_anixops_secrets(self) -> None:
        renderer, _ = self.modules()
        previous = os.environ.get("ANIXOPS_TEST_SECRET")
        os.environ["ANIXOPS_TEST_SECRET"] = "must-not-reach-a-gate"
        try:
            environment = renderer.rehearsal_environment("postgres-test-dsn")
        finally:
            if previous is None:
                os.environ.pop("ANIXOPS_TEST_SECRET", None)
            else:
                os.environ["ANIXOPS_TEST_SECRET"] = previous

        self.assertNotIn("ANIXOPS_TEST_SECRET", environment)
        self.assertEqual("postgres-test-dsn", environment["ANIX_TEST_POSTGRES_DSN"])

    def test_renderer_finalizes_precollected_public_results_without_running_gates(self) -> None:
        _, verifier = self.modules()
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            packages, official_root, canary, support, output, signing_key = self.write_fixture(root)
            results_path = root / "v4-rehearsal-results.json"
            results_path.write_text(
                json.dumps(
                    {
                        "schema": "anixops.v4.rehearsal-results/v1",
                        "tag": "v4.0.0",
                        "results": self.successful_results(root),
                    },
                    ensure_ascii=True,
                    sort_keys=True,
                )
                + "\n",
                encoding="utf-8",
            )

            result = subprocess.run(
                [
                    sys.executable,
                    str(RENDERER_PATH),
                    "--packages-dir",
                    str(packages),
                    "--official-public-key",
                    str(official_root),
                    "--canary-evidence",
                    str(canary),
                    "--support-evidence",
                    str(support),
                    "--signing-key",
                    str(signing_key),
                    "--results",
                    str(results_path),
                    "--output",
                    str(output),
                ],
                check=False,
                capture_output=True,
                text=True,
            )

            self.assertEqual(0, result.returncode, result.stderr or result.stdout)
            verifier.verify_evidence_signature(
                output,
                output.with_suffix(output.suffix + ".sig"),
                self.trusted_public_key(verifier, root),
            )

    def test_public_rehearsal_collection_rejects_secret_inputs(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            packages, official_root, _, _, _, signing_key = self.write_fixture(root)
            result = subprocess.run(
                [
                    sys.executable,
                    str(RENDERER_PATH),
                    "--packages-dir",
                    str(packages),
                    "--official-public-key",
                    str(official_root),
                    "--postgres-dsn",
                    "postgres-test-dsn",
                    "--results-output",
                    str(root / "v4-rehearsal-results.json"),
                    "--signing-key",
                    str(signing_key),
                ],
                check=False,
                capture_output=True,
                text=True,
            )

            self.assertNotEqual(0, result.returncode)
            self.assertIn("must not receive signing or approval inputs", result.stderr)

    def test_renderer_records_all_signed_package_evidence(self) -> None:
        renderer, verifier = self.modules()
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            evidence = self.render_fixture_evidence(renderer, root)
            output = root / "v4-rehearsal-evidence.json"
            signature = renderer.write_evidence(
                evidence,
                output,
                root / "official-signing-key.pem",
                self.trusted_public_key(verifier, root),
            )
            loaded = json.loads(output.read_text(encoding="utf-8"))
            verifier.verify_evidence_signature(output, signature, self.trusted_public_key(verifier, root))
            verifier.validate_v4_evidence(loaded, output, self.trusted_public_key(verifier, root))

        self.assertEqual(list(PACKAGE_IDS), [package["id"] for package in loaded["package_artifacts"]])


if __name__ == "__main__":
    unittest.main()
