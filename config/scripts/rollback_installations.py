#!/usr/bin/env python3
"""Move package installations back to an older release, in the order 4.1.0 needs.

After rolling Control back from 4.2 to 4.1.0 on a database whose packages were
already imported, the installations still point at the 4.2 releases, which the
old root does not verify. 4.1.0 refuses every installation change while the
`forward` package declares a capability it does not know, so the order matters:
`forward` first, then `identity-platform` (login returns), then the rest.

The script only calls the administrator API the old Control already has
(`GET` and `PUT /api/v3/plugin-installations`), with the session token taken
before the upgrade. It is a dry run unless --apply is given, stops at the first
refusal, and is safe to run again.

    python3 config/scripts/rollback_installations.py --panel https://panel.example.com --version 4.1.0
    python3 config/scripts/rollback_installations.py --panel https://panel.example.com --version 4.1.0 --apply

The token comes from ANIX_CONTROL_TOKEN (never from the command line, where the
process list would show it).
"""

from __future__ import annotations

import argparse
import json
import os
import re
import sys
import time
import urllib.error
import urllib.request
from typing import Any

INSTALLATIONS = "/api/v3/plugin-installations"
FIRST = ("forward", "identity-platform")
VERSION = re.compile(r"^[0-9A-Za-z][0-9A-Za-z.+-]{0,63}$")


class RollbackError(Exception):
    pass


def call(panel: str, token: str, method: str, body: dict[str, Any] | None = None) -> Any:
    data = None if body is None else json.dumps(body).encode()
    request = urllib.request.Request(
        panel.rstrip("/") + INSTALLATIONS,
        data=data,
        method=method,
        headers={"Authorization": "Bearer " + token, "Content-Type": "application/json"},
    )
    try:
        with urllib.request.urlopen(request, timeout=30) as response:  # nosec B310 - the operator's own panel URL
            return json.loads(response.read() or b"{}").get("data")
    except urllib.error.HTTPError as err:
        detail = err.read().decode(errors="replace")
        try:
            error = json.loads(detail).get("error") or {}
            detail = f"{error.get('code', '')}: {error.get('message', '')}".strip(": ")
        except (ValueError, AttributeError):
            pass
        raise RollbackError(f"{method} {INSTALLATIONS} answered {err.code}: {detail}") from err
    except urllib.error.URLError as err:
        raise RollbackError(f"{method} {INSTALLATIONS} failed: {err.reason}") from err


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


def run(panel: str, token: str, version: str, apply: bool, wait: int, out=sys.stdout) -> int:
    rows = call(panel, token, "GET")
    if not rows:
        raise RollbackError("the panel lists no installations; is the token an administrator session from before the upgrade?")
    todo = plan(rows, version)
    if not todo:
        print(f"every installation is already on {version}; nothing to do", file=out)
        return 0
    print(f"{len(todo)} of {len(rows)} installations to move to {version}, in this order:", file=out)
    for row in todo:
        print(f"  {row['plugin_id']}/{row['target']}: {row.get('desired_version')} -> {version}", file=out)
    if not apply:
        print("dry run: nothing was changed; add --apply to move them", file=out)
        return 0
    for row in todo:
        body = {
            "plugin_id": row["plugin_id"],
            "target": row["target"],
            "desired_version": version,
            "enabled": bool(row.get("enabled")),
        }
        call(panel, token, "PUT", body)
        print(f"moved {row['plugin_id']}/{row['target']}", file=out)
    pending = wait_until_on(panel, token, todo, version, wait)
    if pending:
        print("not healthy on " + version + " yet: " + ", ".join(pending), file=out)
        return 2
    print(f"all {len(todo)} installations are on {version} and healthy", file=out)
    return 0


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--panel", required=True, help="base URL of the rolled-back Control, for example https://panel.example.com")
    parser.add_argument("--version", required=True, help="the release to move every installation to, for example 4.1.0")
    parser.add_argument("--apply", action="store_true", help="make the changes (the default is a dry run)")
    parser.add_argument("--wait", type=int, default=120, help="seconds to wait for the moved installations to be healthy (default 120)")
    args = parser.parse_args(argv)
    token = os.environ.get("ANIX_CONTROL_TOKEN", "").strip()
    try:
        if not VERSION.match(args.version):
            raise RollbackError(f"{args.version!r} is not a release version")
        if not token:
            raise RollbackError("set ANIX_CONTROL_TOKEN to the administrator session token taken before the upgrade")
        return run(args.panel, token, args.version, args.apply, args.wait)
    except RollbackError as err:
        print(f"error: {err}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
