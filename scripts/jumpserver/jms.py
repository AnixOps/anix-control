#!/usr/bin/env python3
"""Small JumpServer API client for the node fleet.

It reads the JumpServer address and Access Key from the environment
(JMS_URL, JMS_KEY_ID, JMS_SECRET; JMS_ORG defaults to the default
organization), signs every request the way JumpServer expects (HTTP
Signature, hmac-sha256 over "(request-target) accept date") and offers
three commands:

    jms.py check                         who the key belongs to, how many hosts
    jms.py hosts                         list the hosts
    jms.py run --cmd 'uptime' [--hosts a,b] [--runas root] [--timeout 60]
                                         run a shell command on the hosts with
                                         JumpServer's job center (Ansible) and
                                         wait for the result

In GitHub Actions (the repository and its logs are public) host addresses
and names are masked and `run` prints each host's output only with --show-output.

It needs `requests` (`pip install requests`). The
GitHub Actions workflow `.github/workflows/jumpserver.yml` runs it with the
`production` environment's secrets (docs/guide/jumpserver.md).
"""

import argparse
import base64
import hashlib
import hmac
import json
import os
import sys
import time
from email.utils import formatdate
from urllib.parse import urlsplit

import requests

DEFAULT_ORG = "00000000-0000-0000-0000-000000000002"
PAGE_SIZE = 100
RESULT_LIMIT = 20000


class JumpServerError(Exception):
    pass


class SignatureAuth(requests.auth.AuthBase):
    """HTTP Signature (draft-cavage) with hmac-sha256, as JumpServer verifies it."""

    HEADERS = ("(request-target)", "accept", "date")

    def __init__(self, key_id, secret):
        self.key_id = key_id
        self.secret = secret.encode()

    def __call__(self, request):
        url = urlsplit(request.url)
        target = url.path + (f"?{url.query}" if url.query else "")
        lines = [
            f"(request-target): {request.method.lower()} {target}",
            f"accept: {request.headers['Accept']}",
            f"date: {request.headers['Date']}",
        ]
        digest = hmac.new(self.secret, "\n".join(lines).encode(), hashlib.sha256).digest()
        request.headers["Authorization"] = (
            f'Signature keyId="{self.key_id}",algorithm="hmac-sha256",'
            f'headers="{" ".join(self.HEADERS)}",'
            f'signature="{base64.b64encode(digest).decode()}"'
        )
        return request


class Client:
    def __init__(self, url, key_id, secret, org=DEFAULT_ORG, timeout=30):
        self.url = url.rstrip("/")
        self.org = org
        self.timeout = timeout
        self.auth = SignatureAuth(key_id, secret)

    def request(self, method, path, body=None):
        headers = {
            "Accept": "application/json",
            "X-JMS-ORG": self.org,
            "Date": formatdate(usegmt=True),
        }
        response = requests.request(
            method, self.url + path, json=body, headers=headers,
            auth=self.auth, timeout=self.timeout,
        )
        if response.status_code >= 400:
            raise JumpServerError(
                f"{method} {path}: HTTP {response.status_code}: {response.text[:500]}"
            )
        if not response.content:
            return None
        return response.json()

    def get(self, path):
        return self.request("GET", path)

    def post(self, path, body):
        return self.request("POST", path, body)

    def hosts(self):
        found, offset = [], 0
        while True:
            page = self.get(f"/api/v1/assets/hosts/?limit={PAGE_SIZE}&offset={offset}")
            if isinstance(page, list):  # an unpaginated answer
                return mask_addresses(page)
            found.extend(page.get("results", []))
            offset += PAGE_SIZE
            if not page.get("next") or offset >= page.get("count", 0):
                return mask_addresses(found)


def mask_addresses(hosts):
    """In GitHub Actions, have the runner mask every host address and name.

    The repository is public, so its workflow logs are too.
    """
    if os.environ.get("GITHUB_ACTIONS") == "true":
        for h in hosts:
            for value in (h.get("address"), h.get("name")):
                if value:
                    print(f"::add-mask::{value}", flush=True)
    return hosts


def label(value):
    """Return a choice field's label: JumpServer sends {"value", "label"}."""
    if isinstance(value, dict):
        return value.get("name") or value.get("label") or value.get("value") or ""
    return "" if value is None else str(value)


def select_hosts(hosts, names):
    active = [h for h in hosts if h.get("is_active", True)]
    if not names:
        return active
    wanted = {n.strip() for n in names.split(",") if n.strip()}
    chosen = [h for h in active if h.get("name") in wanted or h.get("address") in wanted]
    missing = wanted - {h.get("name") for h in chosen} - {h.get("address") for h in chosen}
    if missing:
        raise JumpServerError("unknown or inactive hosts: " + ", ".join(sorted(missing)))
    return chosen


def cmd_check(client, _args):
    profile = client.get("/api/v1/users/profile/")
    hosts = client.hosts()
    active = sum(1 for h in hosts if h.get("is_active", True))
    print(f"JumpServer: {client.url}")
    print(f"Key owner:  {profile.get('username')} ({profile.get('name')})")
    print(f"Hosts:      {len(hosts)} ({active} active)")
    return 0


def cmd_hosts(client, _args):
    hosts = client.hosts()
    print(f"{'NAME':<32} {'ADDRESS':<40} {'PLATFORM':<12} ACTIVE")
    for h in sorted(hosts, key=lambda h: h.get("name", "")):
        print(f"{h.get('name', ''):<32} {h.get('address', ''):<40} "
              f"{label(h.get('platform')):<12} {'yes' if h.get('is_active', True) else 'no'}")
    print(f"{len(hosts)} hosts")
    return 0


def cmd_run(client, args):
    hosts = select_hosts(client.hosts(), args.hosts)
    if not hosts:
        raise JumpServerError("no active hosts to run on")
    print(f"Running {args.cmd!r} as {args.runas} on {len(hosts)} hosts: "
          + ", ".join(h.get("name", "") for h in hosts))
    job = client.post("/api/v1/ops/jobs/", {
        "instant": True,
        # JumpServer's job serializer defaults is_periodic to true and then
        # rejects a job without a crontab or interval.
        "is_periodic": False,
        "type": "adhoc",
        "module": "shell",
        "args": args.cmd,
        "assets": [h["id"] for h in hosts],
        "runas": args.runas,
        "runas_policy": "privileged_first",
        "timeout": args.timeout,
    })
    task_id = (job or {}).get("task_id")
    if not task_id:
        raise JumpServerError(f"the job did not start: {json.dumps(job)[:500]}")
    print(f"Task {task_id}")

    deadline = time.monotonic() + args.timeout + 120
    while True:
        detail = client.get(f"/api/v1/ops/job-execution/task-detail/{task_id}/")
        if detail.get("is_finished"):
            break
        if time.monotonic() > deadline:
            raise JumpServerError(f"task {task_id} still running after {args.timeout + 120}s")
        time.sleep(3)

    result = client.get(f"/api/v1/ops/job-execution/result/{task_id}/")
    print(f"Status:  {label(detail.get('status'))} in {detail.get('time_cost')}s")
    print("Summary: " + json.dumps(detail.get("summary"), ensure_ascii=False, indent=2))
    if not args.show_output:
        print("Result:  hidden (--show-output prints each host's output; workflow logs are public)")
        return 0 if detail.get("is_success") else 1
    output = json.dumps((result or {}).get("result"), ensure_ascii=False, indent=2)
    if len(output) > RESULT_LIMIT:
        output = output[:RESULT_LIMIT] + "\n... (truncated)"
    print("Result:  " + output)
    return 0 if detail.get("is_success") else 1


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    sub = parser.add_subparsers(dest="command", required=True)
    sub.add_parser("check", help="check the key and count the hosts")
    sub.add_parser("hosts", help="list the hosts")
    run = sub.add_parser("run", help="run a shell command on the hosts")
    run.add_argument("--cmd", required=True, help="the shell command")
    run.add_argument("--hosts", default="", help="comma-separated host names or addresses; empty means all active hosts")
    run.add_argument("--runas", default="root", help="the account to run as (default root)")
    run.add_argument("--timeout", type=int, default=60, help="seconds the command may run (default 60)")
    run.add_argument("--show-output", action="store_true", help="print each host's output (the default outside GitHub Actions)")
    args = parser.parse_args(argv)
    if args.command == "run" and os.environ.get("GITHUB_ACTIONS") != "true":
        args.show_output = True

    missing = [k for k in ("JMS_URL", "JMS_KEY_ID", "JMS_SECRET") if not os.environ.get(k)]
    if missing:
        print("missing environment: " + ", ".join(missing), file=sys.stderr)
        return 2
    client = Client(
        os.environ["JMS_URL"], os.environ["JMS_KEY_ID"], os.environ["JMS_SECRET"],
        org=os.environ.get("JMS_ORG") or DEFAULT_ORG,
    )
    commands = {"check": cmd_check, "hosts": cmd_hosts, "run": cmd_run}
    try:
        return commands[args.command](client, args)
    except (JumpServerError, requests.RequestException) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
