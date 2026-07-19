#!/usr/bin/env python3
"""Run formal v4 release gates and render their evidence bundle."""

from __future__ import annotations

import argparse
import base64
import hashlib
import json
import os
import subprocess
import sys
import tempfile
from pathlib import Path
from typing import Any

from verify_v4_evidence import (
    EVIDENCE_SCHEMA,
    PACKAGE_IDS,
    REQUIRED_TAG,
    EvidenceError,
    absolute_path,
    canonical_json,
    has_symlink_component,
    is_regular_file,
    load_and_validate_approval,
    load_trusted_official_public_key,
    release_subject_digest,
    sha256_file,
    sign_detached_signature,
    validate_v4_evidence,
    verify_evidence_signature,
)


REPO_ROOT = Path(__file__).resolve().parents[2]
REHEARSAL_RESULTS_SCHEMA = "anixops.v4.rehearsal-results/v1"
RESULT_NAMES = (
    "official_root_verification",
    "release_stage_gate",
    "v2_route_catalog_gate",
    "plugin_only_route_gate",
    "websocket_relay_tests",
    "v2_compatibility_tests",
    "sqlite_rehearsal",
    "postgres_rehearsal",
)


class RenderError(EvidenceError):
    pass


def relative_evidence_path(output: Path, path: Path, label: str) -> str:
    root = output.parent.resolve()
    candidate = path.resolve()
    try:
        return candidate.relative_to(root).as_posix()
    except ValueError as error:
        raise RenderError(f"{label} must be located under {root}") from error


def file_evidence(output: Path, path: Path, label: str) -> dict[str, str]:
    if not is_regular_file(path) or path.stat().st_size == 0:
        raise RenderError(f"{label} is missing or empty: {path}")
    return {"path": relative_evidence_path(output, path, label), "sha256": sha256_file(path)}


def write_bytes_atomically(path: Path, contents: bytes) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    descriptor, temporary_name = tempfile.mkstemp(prefix=f".{path.name}.", dir=path.parent)
    temporary = Path(temporary_name)
    try:
        with os.fdopen(descriptor, "wb") as handle:
            handle.write(contents)
            handle.flush()
            os.fsync(handle.fileno())
        os.replace(temporary, path)
    finally:
        if temporary.exists():
            temporary.unlink()


def normalize_approval(
    *,
    output: Path,
    source: Path,
    kind: str,
    release_subject_sha256: str,
    trusted_official_public_key: bytes,
) -> dict[str, str]:
    approval = load_and_validate_approval(
        source,
        kind=kind,
        release_subject_sha256=release_subject_sha256,
        trusted_official_public_key=trusted_official_public_key,
        label=f"{kind} approval",
    )
    target = output.parent / f"{kind}-approval.json"
    write_bytes_atomically(target, canonical_json(approval) + b"\n")
    return file_evidence(output, target, f"{kind} approval")


def normalize_official_public_key(output: Path, trusted_official_public_key: bytes) -> dict[str, str]:
    target = output.parent / "official-public-key.raw"
    write_bytes_atomically(target, base64.b64encode(trusted_official_public_key) + b"\n")
    return file_evidence(output, target, "official public key")


def package_artifacts(output: Path, packages_dir: Path, tag: str) -> list[dict[str, str]]:
    if has_symlink_component(packages_dir) or not packages_dir.is_dir():
        raise RenderError(f"formal package directory must be a regular directory: {packages_dir}")
    version = tag.removeprefix("v")
    entries: list[dict[str, str]] = []
    for package_id in PACKAGE_IDS:
        stem = f"{package_id}-{version}"
        files = {
            "artifact": packages_dir / f"{stem}.anxp",
            "manifest": packages_dir / f"{stem}.manifest.json",
            "signature": packages_dir / f"{stem}.manifest.sig",
            "public_key": packages_dir / f"{stem}.public-key.pem",
            "sbom": packages_dir / f"{stem}.sbom.spdx.json",
        }
        entry = {"id": package_id}
        for field, path in files.items():
            if not is_regular_file(path) or path.stat().st_size == 0:
                raise RenderError(f"missing signed package {package_id} {field}: {path}")
            entry[field] = relative_evidence_path(output, path, f"package {package_id} {field}")
        entries.append(entry)
    return entries


def require_successful_results(results: dict[str, dict[str, object]]) -> None:
    if set(results) != set(RESULT_NAMES):
        missing = sorted(set(RESULT_NAMES) - set(results))
        extra = sorted(set(results) - set(RESULT_NAMES))
        raise RenderError(f"release evidence results are incomplete: missing={missing}, extra={extra}")
    for name in RESULT_NAMES:
        result = results[name]
        if not isinstance(result, dict) or result.get("passed") is not True:
            raise RenderError(f"{name} did not pass")
        transcript = result.get("transcript")
        if not isinstance(transcript, str) or not transcript:
            raise RenderError(f"{name} transcript is missing")
        digest = result.get("transcript_sha256")
        if not isinstance(digest, str) or len(digest) != 64:
            raise RenderError(f"{name} transcript digest is invalid")


def render_evidence(
    *,
    tag: str,
    packages_dir: Path,
    official_public_key: Path,
    canary_evidence: Path,
    support_evidence: Path,
    output: Path,
    results: dict[str, dict[str, object]],
) -> dict[str, object]:
    if tag != REQUIRED_TAG:
        raise RenderError(f"v4 release evidence tag must be {REQUIRED_TAG}")
    output.parent.mkdir(parents=True, exist_ok=True)
    require_successful_results(results)
    trusted_official_public_key = load_trusted_official_public_key(official_public_key)
    artifacts = package_artifacts(output, packages_dir, tag)
    release_subject_sha256 = release_subject_digest(artifacts, output)
    official = normalize_official_public_key(output, trusted_official_public_key)
    evidence: dict[str, object] = {
        "schema": EVIDENCE_SCHEMA,
        "tag": tag,
        "package_artifacts": artifacts,
        "official_root_verification": {
            **results["official_root_verification"],
            "public_key": official["path"],
        },
        "release_stage_gate": results["release_stage_gate"],
        "v2_route_catalog_gate": results["v2_route_catalog_gate"],
        "plugin_only_route_gate": results["plugin_only_route_gate"],
        "websocket_relay_tests": results["websocket_relay_tests"],
        "v2_compatibility_tests": results["v2_compatibility_tests"],
        "sqlite_rehearsal": results["sqlite_rehearsal"],
        "postgres_rehearsal": results["postgres_rehearsal"],
        "canary_evidence": normalize_approval(
            output=output,
            source=canary_evidence,
            kind="canary",
            release_subject_sha256=release_subject_sha256,
            trusted_official_public_key=trusted_official_public_key,
        ),
        "support_evidence": normalize_approval(
            output=output,
            source=support_evidence,
            kind="support",
            release_subject_sha256=release_subject_sha256,
            trusted_official_public_key=trusted_official_public_key,
        ),
    }
    try:
        validate_v4_evidence(evidence, output, trusted_official_public_key)
    except EvidenceError as error:
        raise RenderError(str(error)) from error
    return evidence


def write_evidence(
    evidence: dict[str, object], output: Path, signing_key: Path, trusted_official_public_key: bytes
) -> Path:
    encoded = (json_dumps(evidence) + "\n").encode("utf-8")
    write_bytes_atomically(output, encoded)
    signature_path = output.with_suffix(output.suffix + ".sig")
    write_bytes_atomically(signature_path, sign_detached_signature(encoded, signing_key, "v4 release evidence"))
    verify_evidence_signature(output, signature_path, trusted_official_public_key)
    return signature_path


def write_rehearsal_results(tag: str, results: dict[str, dict[str, object]], output: Path) -> None:
    if tag != REQUIRED_TAG:
        raise RenderError(f"v4 rehearsal results tag must be {REQUIRED_TAG}")
    require_successful_results(results)
    document = {
        "schema": REHEARSAL_RESULTS_SCHEMA,
        "tag": tag,
        "results": results,
    }
    write_bytes_atomically(output, (json_dumps(document) + "\n").encode("utf-8"))


def load_rehearsal_results(path: Path, tag: str) -> dict[str, dict[str, object]]:
    if not is_regular_file(path) or path.stat().st_size == 0:
        raise RenderError(f"v4 rehearsal results are missing or empty: {path}")
    try:
        document = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, UnicodeError, json.JSONDecodeError) as error:
        raise RenderError(f"v4 rehearsal results are invalid JSON: {path}") from error
    if not isinstance(document, dict) or frozenset(document) != frozenset({"schema", "tag", "results"}):
        raise RenderError("v4 rehearsal results have an invalid schema")
    if document["schema"] != REHEARSAL_RESULTS_SCHEMA:
        raise RenderError(f"v4 rehearsal results have unsupported schema: {document['schema']!r}")
    if document["tag"] != tag or tag != REQUIRED_TAG:
        raise RenderError(f"v4 rehearsal results tag must be {REQUIRED_TAG}")
    results = document["results"]
    if not isinstance(results, dict):
        raise RenderError("v4 rehearsal results must contain an object")
    require_successful_results(results)
    return results


def finalize_evidence(
    *,
    tag: str,
    packages_dir: Path,
    official_public_key: Path,
    canary_evidence: Path,
    support_evidence: Path,
    signing_key: Path,
    output: Path,
    results: dict[str, dict[str, object]],
) -> Path:
    evidence = render_evidence(
        tag=tag,
        packages_dir=packages_dir,
        official_public_key=official_public_key,
        canary_evidence=canary_evidence,
        support_evidence=support_evidence,
        output=output,
        results=results,
    )
    return write_evidence(evidence, output, signing_key, load_trusted_official_public_key(official_public_key))


def json_dumps(value: object) -> str:
    import json

    return json.dumps(value, ensure_ascii=True, indent=2, sort_keys=True)


def command_result(
    name: str,
    command: list[str],
    *,
    repo_root: Path,
    environment: dict[str, str],
    output: Path,
    transcript_name: str,
    reject_skip: bool = False,
) -> dict[str, object]:
    result = subprocess.run(
        command,
        cwd=repo_root,
        env=environment,
        check=False,
        text=True,
        capture_output=True,
    )
    transcript = (result.stdout + result.stderr).encode("utf-8", errors="replace")
    if result.returncode != 0:
        detail = (result.stderr or result.stdout).strip()
        raise RenderError(f"{name} failed: {detail}")
    if reject_skip and "--- SKIP:" in result.stdout:
        raise RenderError(f"{name} skipped despite an explicit PostgreSQL DSN")
    transcript_path = output.parent / "transcripts" / f"{transcript_name}.log"
    transcript_path.parent.mkdir(parents=True, exist_ok=True)
    descriptor, temporary_name = tempfile.mkstemp(prefix=f".{transcript_path.name}.", dir=transcript_path.parent)
    temporary = Path(temporary_name)
    try:
        with os.fdopen(descriptor, "wb") as handle:
            handle.write(transcript)
            handle.flush()
            os.fsync(handle.fileno())
        os.replace(temporary, transcript_path)
    finally:
        if temporary.exists():
            temporary.unlink()
    return {
        "passed": True,
        "transcript": relative_evidence_path(output, transcript_path, f"{name} transcript"),
        "transcript_sha256": hashlib.sha256(transcript).hexdigest(),
    }


def rehearsal_environment(postgres_dsn: str) -> dict[str, str]:
    environment = {key: value for key, value in os.environ.items() if not key.startswith("ANIXOPS_")}
    environment.update({"GOWORK": "off", "ANIX_TEST_POSTGRES_DSN": postgres_dsn})
    return environment


def run_rehearsal(
    *, repo_root: Path, tag: str, packages_dir: Path, official_public_key: Path, postgres_dsn: str, output: Path
) -> dict[str, dict[str, object]]:
    if not postgres_dsn.strip():
        raise RenderError("PostgreSQL rehearsal requires --postgres-dsn")
    version = tag.removeprefix("v")
    environment = rehearsal_environment(postgres_dsn)
    builder = repo_root / "packages" / "shared" / "build_package.py"
    results = {
        "official_root_verification": command_result(
            "official root verification",
            [
                sys.executable,
                str(builder),
                "--all",
                "--version",
                version,
                "--out",
                str(packages_dir),
                "--verify-release",
                "--official-public-key",
                str(official_public_key),
            ],
            repo_root=repo_root,
            environment=environment,
            output=output,
            transcript_name="official-root-verification",
        ),
        "release_stage_gate": command_result(
            "release stage gate",
            [sys.executable, str(repo_root / "config" / "scripts" / "check_release_stage.py"), "--tag", tag],
            repo_root=repo_root,
            environment=environment,
            output=output,
            transcript_name="release-stage-gate",
        ),
        "v2_route_catalog_gate": command_result(
            "v2 route catalog gate",
            [sys.executable, str(repo_root / "config" / "scripts" / "check_v2_package_route_catalog.py")],
            repo_root=repo_root,
            environment=environment,
            output=output,
            transcript_name="v2-route-catalog-gate",
        ),
        "plugin_only_route_gate": command_result(
            "plugin-only route gate",
            [sys.executable, str(repo_root / "config" / "scripts" / "check_plugin_only_routes.py")],
            repo_root=repo_root,
            environment=environment,
            output=output,
            transcript_name="plugin-only-route-gate",
        ),
        "websocket_relay_tests": command_result(
            "WebSocket relay tests",
            [
                "go",
                "test",
                "./internal/packagebridge",
                "./pkg/packagebridgesdk",
                "./packages/shared/controlhost",
                "./internal/pluginhost",
                "./pkg/pluginhostsdk",
                "./internal/identitybridge",
                "-count=1",
            ],
            repo_root=repo_root,
            environment=environment,
            output=output,
            transcript_name="websocket-relay-tests",
        ),
        "v2_compatibility_tests": command_result(
            "v2 compatibility tests",
            ["go", "test", "./internal/compat/v2", "./internal/router", "./internal/handler", "./internal/tests/e2e", "-count=1"],
            repo_root=repo_root,
            environment=environment,
            output=output,
            transcript_name="v2-compatibility-tests",
        ),
        "sqlite_rehearsal": command_result(
            "SQLite rehearsal",
            ["go", "test", "./internal/tests/integration", "-run", "TestPackageMigrationCohortRollbackSQLite", "-count=1"],
            repo_root=repo_root,
            environment=environment,
            output=output,
            transcript_name="sqlite-rehearsal",
        ),
        "postgres_rehearsal": command_result(
            "PostgreSQL rehearsal",
            [
                "go",
                "test",
                "./internal/tests/integration",
                "-run",
                "TestPostgresPackageMigrationRollback|TestPostgresPackageMigrationSerializesRootRouteGeneration",
                "-count=1",
            ],
            repo_root=repo_root,
            environment=environment,
            output=output,
            transcript_name="postgres-rehearsal",
            reject_skip=True,
        ),
    }
    return results


def parse_arguments() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--tag", default=REQUIRED_TAG)
    parser.add_argument("--packages-dir", type=Path)
    parser.add_argument("--official-public-key", type=Path)
    parser.add_argument("--canary-evidence", type=Path)
    parser.add_argument("--support-evidence", type=Path)
    parser.add_argument("--signing-key", type=Path)
    parser.add_argument("--output", type=Path)
    parser.add_argument(
        "--results-output",
        type=Path,
        help="Write public, unsigned rehearsal results without accepting signing or approval inputs",
    )
    parser.add_argument(
        "--results",
        type=Path,
        help="Use precollected public rehearsal results and do not run gate subprocesses",
    )
    parser.add_argument("--postgres-dsn")
    parser.add_argument("--repo-root", type=Path, default=REPO_ROOT)
    parser.add_argument("--self-test", action="store_true")
    return parser.parse_args()


def main() -> int:
    arguments = parse_arguments()
    try:
        if arguments.self_test:
            if len(PACKAGE_IDS) != 16 or "identity-platform" not in PACKAGE_IDS:
                raise RenderError("v4 release evidence package matrix is incomplete")
            print("v4 release evidence renderer self-test passed")
            return 0
        repo_root = arguments.repo_root.resolve()
        if arguments.results_output is not None and arguments.results is not None:
            raise RenderError("--results-output and --results cannot be used together")
        if arguments.results_output is not None:
            if any(value is not None for value in (arguments.canary_evidence, arguments.support_evidence, arguments.signing_key)):
                raise RenderError("public rehearsal collection must not receive signing or approval inputs")
            required = {
                "--packages-dir": arguments.packages_dir,
                "--official-public-key": arguments.official_public_key,
                "--postgres-dsn": arguments.postgres_dsn,
                "--results-output": arguments.results_output,
            }
            missing = [name for name, value in required.items() if value is None or value == ""]
            if missing:
                raise RenderError("missing required arguments: " + ", ".join(missing))
            results_output = absolute_path(arguments.results_output)
            results = run_rehearsal(
                repo_root=repo_root,
                tag=arguments.tag,
                packages_dir=absolute_path(arguments.packages_dir),
                official_public_key=absolute_path(arguments.official_public_key),
                postgres_dsn=arguments.postgres_dsn,
                output=results_output,
            )
            write_rehearsal_results(arguments.tag, results, results_output)
            print(f"wrote {results_output}")
            return 0

        required = {
            "--packages-dir": arguments.packages_dir,
            "--official-public-key": arguments.official_public_key,
            "--canary-evidence": arguments.canary_evidence,
            "--support-evidence": arguments.support_evidence,
            "--signing-key": arguments.signing_key,
            "--output": arguments.output,
        }
        if arguments.results is None:
            required["--postgres-dsn"] = arguments.postgres_dsn
        missing = [name for name, value in required.items() if value is None or value == ""]
        if missing:
            raise RenderError("missing required arguments: " + ", ".join(missing))
        output = absolute_path(arguments.output)
        packages_dir = absolute_path(arguments.packages_dir)
        official_public_key = absolute_path(arguments.official_public_key)
        canary_evidence = absolute_path(arguments.canary_evidence)
        support_evidence = absolute_path(arguments.support_evidence)
        signing_key = absolute_path(arguments.signing_key)
        if arguments.results is None:
            results = run_rehearsal(
                repo_root=repo_root,
                tag=arguments.tag,
                packages_dir=packages_dir,
                official_public_key=official_public_key,
                postgres_dsn=arguments.postgres_dsn,
                output=output,
            )
        else:
            results = load_rehearsal_results(absolute_path(arguments.results), arguments.tag)
        finalize_evidence(
            tag=arguments.tag,
            packages_dir=packages_dir,
            official_public_key=official_public_key,
            canary_evidence=canary_evidence,
            support_evidence=support_evidence,
            signing_key=signing_key,
            output=output,
            results=results,
        )
    except (EvidenceError, OSError) as error:
        print(f"v4 release evidence renderer: {error}", file=sys.stderr)
        return 1
    print(f"wrote {output}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
