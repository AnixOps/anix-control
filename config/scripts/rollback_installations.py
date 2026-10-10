#!/usr/bin/env python3
"""Move package installations back to an older release, in the order 4.1.0 needs.

After rolling Control back from 4.2 to 4.1.0 on a database whose packages were
already imported, the installations still point at the 4.2 releases, which the
old root does not verify. 4.1.0 refuses every installation change while the
`forward` package declares a capability it does not know, so the order matters:
`forward` first, then `identity-platform` (login returns), then the rest.

The script only calls the administrator API the old Control already has
(`GET /api/v3/plugin-releases`, `GET` and `PUT /api/v3/plugin-installations`),
with the session token taken before the upgrade. It is a dry run unless --apply
is given and is safe to run again.

An installation whose package has no release of --version on the panel (a
package that never had a 4.1.0 build) cannot be moved: the script lists those up
front, skips them and moves the rest. Any other refusal (a network or
authorization error, a 5xx, the `409` that means `forward` did not move) stops
the run. The run ends with a summary of what was rolled back, skipped and failed.

Exit status: 0 everything was moved and is healthy (or there was nothing to do),
1 an error stopped the run, 2 moved installations are not healthy within --wait,
3 some installations were skipped (--allow-skipped makes that 0). A dry run
exits with the status the same run with --apply would end with.

    python3 config/scripts/rollback_installations.py --panel https://panel.example.com --version 4.1.0
    python3 config/scripts/rollback_installations.py --panel https://panel.example.com --version 4.1.0 --apply

The token comes from ANIX_CONTROL_TOKEN (never from the command line, where the
process list would show it).
"""

from __future__ import annotations

import argparse
import http.client
import json
import os
import re
import sys
import time
import urllib.error
import urllib.request
from typing import Any

INSTALLATIONS = "/api/v3/plugin-installations"
RELEASES = "/api/v3/plugin-releases"
SKIPPED_EXIT = 3
FIRST = ("forward", "identity-platform")
VERSION = re.compile(r"^[0-9A-Za-z][0-9A-Za-z.+-]{0,63}$")


class RollbackError(Exception):
    """A refusal or failure; `status` and `code` are the panel's HTTP status and error code, if it answered."""

    def __init__(self, message: str, status: int | None = None, code: str = "") -> None:
        super().__init__(message)
        self.status = status
        self.code = code


def call(panel: str, token: str, method: str, body: dict[str, Any] | None = None, path: str = INSTALLATIONS) -> Any:
    data = None if body is None else json.dumps(body).encode()
    request = urllib.request.Request(
        panel.rstrip("/") + path,
        data=data,
        method=method,
        headers={"Authorization": "Bearer " + token, "Content-Type": "application/json"},
    )
    try:
        with urllib.request.urlopen(request, timeout=30) as response:  # nosec B310 - the operator's own panel URL
            answer = response.read()
    except urllib.error.HTTPError as err:
        detail = err.read().decode(errors="replace")
        code = ""
        try:
            error = json.loads(detail).get("error") or {}
            code = str(error.get("code", ""))
            detail = f"{code}: {error.get('message', '')}".strip(": ")
        except (ValueError, AttributeError):
            pass
        raise RollbackError(f"{method} {path} answered {err.code}: {detail}", err.code, code) from err
    except urllib.error.URLError as err:
        raise RollbackError(f"{method} {path} failed: {err.reason}") from err
    except (OSError, http.client.HTTPException) as err:  # a connection dropped or timed out while the answer was read
        raise RollbackError(f"{method} {path} failed: {err}") from err
    try:
        return json.loads(answer or b"{}").get("data")
    except (ValueError, AttributeError) as err:  # the answer is not the JSON envelope
        raise RollbackError(f"{method} {path} answered something that is not the expected JSON: {err}") from err


def plan(installations: list[dict[str, Any]], version: str) -> list[dict[str, Any]]:
    """The installations that are not on version yet, forward and identity-platform first."""

    def rank(row: dict[str, Any]) -> tuple[int, str, str]:
        plugin = row["plugin_id"]
        return (FIRST.index(plugin) if plugin in FIRST else len(FIRST), plugin, row["target"])

    return sorted((row for row in installations if row.get("desired_version") != version), key=rank)


def wait_until_on(panel: str, token: str, moved: list[dict[str, Any]], version: str, wait: int) -> list[str]:
    """Poll until every moved installation is on version and healthy; return what is still not."""
    deadline = time.monotonic() + wait
    while True:
        rows = {(r["plugin_id"], r["target"]): r for r in call(panel, token, "GET") or []}
        pending = [
            f"{key[0]}/{key[1]} ({row.get('desired_version')}, {row.get('health') or row.get('state')})"
            for key in ((m["plugin_id"], m["target"]) for m in moved)
            for row in [rows.get(key, {})]
            if row.get("desired_version") != version or row.get("health") != "healthy"
        ]
        if not pending or time.monotonic() >= deadline:
            return pending
        time.sleep(2)


def key_of(row: dict[str, Any]) -> str:
    return f"{row['plugin_id']}/{row['target']}"


def registered_releases(panel: str, token: str) -> dict[str, list[str]]:
    """The release versions the panel holds, by plugin id."""
    found: dict[str, list[str]] = {}
    for release in call(panel, token, "GET", path=RELEASES) or []:
        found.setdefault(str(release.get("plugin_id")), []).append(str(release.get("version")))
    return {plugin: sorted(versions) for plugin, versions in found.items()}


def split_unavailable(
    todo: list[dict[str, Any]], releases: dict[str, list[str]], version: str
) -> tuple[list[dict[str, Any]], list[tuple[dict[str, Any], str]]]:
    """Split the plan into what can move to version and what cannot, with the reason for each."""
    movable: list[dict[str, Any]] = []
    unavailable: list[tuple[dict[str, Any], str]] = []
    for row in todo:
        held = releases.get(row["plugin_id"], [])
        if version in held:
            movable.append(row)
            continue
        have = ", ".join(held) if held else "none"
        why = f"no {version} release of {row['plugin_id']} on this panel (it has: {have}); stays on {row.get('desired_version')}"
        unavailable.append((row, why))
    return movable, unavailable


def print_summary(
    out,
    version: str,
    moved: list[dict[str, Any]],
    skipped: list[tuple[dict[str, Any], str]],
    failed: list[tuple[dict[str, Any], str]] | None = None,
    not_attempted: list[dict[str, Any]] | None = None,
    not_healthy: int = 0,
) -> None:
    failed = failed or []
    counts = [f"{len(moved)} rolled back to {version}", f"{len(skipped)} skipped", f"{len(failed)} failed"]
    if not_attempted:
        counts.append(f"{len(not_attempted)} not attempted")
    if not_healthy:
        counts.append(f"{not_healthy} not healthy yet")
    print("summary: " + ", ".join(counts), file=out)
    if skipped:
        print(f"  skipped (no {version} release): " + ", ".join(key_of(row) for row, _ in skipped), file=out)
    for row, why in failed:
        print(f"  failed: {key_of(row)}: {why}", file=out)
    if not_attempted:
        print("  not attempted: " + ", ".join(key_of(row) for row in not_attempted), file=out)


def run(panel: str, token: str, version: str, apply: bool, wait: int, out=None, allow_skipped: bool = False) -> int:
    out = sys.stdout if out is None else out  # looked up per call, so a redirected stdout is honoured
    rows = call(panel, token, "GET")
    if not rows:
        raise RollbackError("the panel lists no installations; is the token an administrator session from before the upgrade?")
    todo = plan(rows, version)
    if not todo:
        print(f"every installation is already on {version}; nothing to do", file=out)
        return 0
    movable, skipped = split_unavailable(todo, registered_releases(panel, token), version)
    if skipped:
        print(f"{len(skipped)} installations cannot be moved to {version} and are skipped:", file=out)
        for row, why in skipped:
            print(f"  {key_of(row)}: {why}", file=out)
    if not movable:
        print(f"no installation can be moved to {version}", file=out)
    else:
        print(f"{len(movable)} of {len(rows)} installations to move to {version}, in this order:", file=out)
        for row in movable:
            print(f"  {key_of(row)}: {row.get('desired_version')} -> {version}", file=out)
    if not apply:
        print("dry run: nothing was changed" + ("; add --apply to move them" if movable else ""), file=out)
        return SKIPPED_EXIT if skipped and not allow_skipped else 0
    moved: list[dict[str, Any]] = []
    for index, row in enumerate(movable):
        body = {
            "plugin_id": row["plugin_id"],
            "target": row["target"],
            "desired_version": version,
            "enabled": bool(row.get("enabled")),
        }
        try:
            call(panel, token, "PUT", body)
        except RollbackError as err:
            if err.code != "release_not_found":
                print_summary(out, version, moved, skipped, [(row, str(err))], movable[index + 1 :])
                raise
            # the listing and the PUT disagree (the release went away in between): this one cannot move either
            skipped.append((row, str(err)))
            print(f"skipped {key_of(row)}: {err}", file=out)
            continue
        moved.append(row)
        print(f"moved {key_of(row)}", file=out)
    try:
        pending = wait_until_on(panel, token, moved, version, wait) if moved else []
    except RollbackError:
        print_summary(out, version, moved, skipped)
        raise
    if pending:
        print("not healthy on " + version + " yet: " + ", ".join(pending), file=out)
    elif moved:
        print(f"all {len(moved)} installations are on {version} and healthy", file=out)
    print_summary(out, version, moved, skipped, not_healthy=len(pending))
    if pending:
        return 2
    return SKIPPED_EXIT if skipped and not allow_skipped else 0


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--panel", required=True, help="base URL of the rolled-back Control, for example https://panel.example.com")
    parser.add_argument("--version", required=True, help="the release to move every installation to, for example 4.1.0")
    parser.add_argument("--apply", action="store_true", help="make the changes (the default is a dry run)")
    parser.add_argument("--wait", type=int, default=120, help="seconds to wait for the moved installations to be healthy (default 120)")
    parser.add_argument(
        "--allow-skipped",
        action="store_true",
        help=f"exit 0 although some installations were skipped because the panel has no release of --version for them (default: exit {SKIPPED_EXIT})",
    )
    args = parser.parse_args(argv)
    token = os.environ.get("ANIX_CONTROL_TOKEN", "").strip()
    try:
        if not VERSION.match(args.version):
            raise RollbackError(f"{args.version!r} is not a release version")
        if not token:
            raise RollbackError("set ANIX_CONTROL_TOKEN to the administrator session token taken before the upgrade")
        return run(args.panel, token, args.version, args.apply, args.wait, allow_skipped=args.allow_skipped)
    except RollbackError as err:
        print(f"error: {err}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
