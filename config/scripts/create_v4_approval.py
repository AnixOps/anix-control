#!/usr/bin/env python3
"""Create a signed, public v4 canary or support approval declaration."""

from __future__ import annotations

import argparse
import os
import tempfile
from pathlib import Path

from verify_v4_evidence import (
    REQUIRED_TAG,
    EvidenceError,
    absolute_path,
    approval_payload,
    canonical_json,
    load_and_validate_approval,
    load_trusted_official_public_key,
    release_subject_digest_from_package_directory,
    sign_detached_signature,
)


def write_atomically(path: Path, contents: bytes) -> None:
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


def parse_arguments() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--kind", choices=("canary", "support"), required=True)
    parser.add_argument("--packages-dir", type=Path, required=True)
    parser.add_argument("--signing-key", type=Path, required=True)
    parser.add_argument("--trusted-official-public-key", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--tag", default=REQUIRED_TAG)
    return parser.parse_args()


def main() -> int:
    arguments = parse_arguments()
    try:
        if arguments.tag != REQUIRED_TAG:
            raise EvidenceError(f"v4 approval tag must be {REQUIRED_TAG}")
        packages_dir = absolute_path(arguments.packages_dir)
        signing_key = absolute_path(arguments.signing_key)
        trusted_root = load_trusted_official_public_key(absolute_path(arguments.trusted_official_public_key))
        subject = release_subject_digest_from_package_directory(packages_dir)
        approval = approval_payload(arguments.kind, subject)
        signature = sign_detached_signature(canonical_json(approval), signing_key, f"v4 {arguments.kind} approval")
        document = {**approval, "signature": signature.decode("ascii").strip()}
        output = absolute_path(arguments.output)
        write_atomically(output, canonical_json(document) + b"\n")
        load_and_validate_approval(
            output,
            kind=arguments.kind,
            release_subject_sha256=subject,
            trusted_official_public_key=trusted_root,
            label=f"{arguments.kind} approval",
        )
    except (EvidenceError, OSError) as error:
        print(f"v4 approval: {error}", file=os.sys.stderr)
        return 1
    print(f"wrote {output}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
