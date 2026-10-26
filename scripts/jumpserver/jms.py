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
    jms.py control --method GET --path /api/v2/admin/nodes [--body JSON]
                                         call Control's administrator API
    jms.py install-agent --hosts host --node proxy-12
                                         install and enroll the Agent of a
                                         Control node on a JumpServer host

In GitHub Actions (the repository and its logs are public) host addresses
and names are masked and `run` prints each host's output only with --show-output.

A command may name a secret as @@NAME@@ (NAME one of SECRET_PLACEHOLDERS).
`run` replaces it with the environment variable NAME just before sending the
job, so the secret never appears in the workflow inputs or the printed command.

`control` and `install-agent` reach Control at CONTROL_URL with the admin API
token CONTROL_ADMIN_TOKEN (docs/reference/admin-api-tokens.md). A `control`
body may hold generated secrets, filled in on the runner and masked:
@@NEW_REALITY_KEY@@ (an X25519 private key; its public key is printed, and
@@NEW_REALITY_PUBLIC_KEY@@ in the same body becomes it) and
@@NEW_SS2022_KEY_16@@ / @@NEW_SS2022_KEY_32@@ (a Shadowsocks 2022 key). Secret
fields of Control's answer are masked, and IP addresses in it replaced with
a short hash, before it is printed.

It needs `requests` and `websocket-client` (`pip install requests websocket-client`). The
GitHub Actions workflow `.github/workflows/jumpserver.yml` runs it with the
`production` environment's secrets (docs/guide/jumpserver.md).
"""

import argparse
import base64
import hashlib
import hmac
import ipaddress
import json
import os
import re
import secrets
import shlex
import sys
import time
from email.utils import formatdate
from urllib.parse import urlsplit

import requests

DEFAULT_ORG = "00000000-0000-0000-0000-000000000002"
PAGE_SIZE = 100
RESULT_LIMIT = 20000
# Secrets a command may name as @@NAME@@; the workflow passes them as env.
SECRET_PLACEHOLDERS = ("CLOUDFLARE_API_TOKEN",)


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

    def headers(self):
        return {
            "Accept": "application/json",
            "X-JMS-ORG": self.org,
            "Date": formatdate(usegmt=True),
        }

    def signed_headers(self, method, path):
        """Headers with the signature, for a request requests does not send."""
        request = requests.Request(method, self.url + path, headers=self.headers()).prepare()
        return dict(self.auth(request).headers)

    def request(self, method, path, body=None):
        headers = self.headers()
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


def ansible_raw(cmd):
    """Keep Ansible from templating the command: it would choke on {{ }}."""
    if any(t in cmd for t in ("{{", "{%", "{#")):
        return "{% raw %}" + cmd + "{% endraw %}"
    return cmd


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
    shown = getattr(args, "describe", None) or repr(args.cmd)
    print(f"Running {shown} as {args.runas} on {len(hosts)} hosts: "
          + ", ".join(h.get("name", "") for h in hosts))
    job = client.post("/api/v1/ops/jobs/", {
        "instant": True,
        "is_periodic": False,  # the serializer defaults it to true and then wants a schedule
        "type": "adhoc",
        "module": "shell",
        "args": ansible_raw(fill_secrets(args.cmd)),
        "assets": [h["id"] for h in hosts],
        "nodes": [],
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

    print(f"Status:  {label(detail.get('status'))} in {detail.get('time_cost')}s")
    print("Summary: " + json.dumps(detail.get("summary"), ensure_ascii=False, indent=2))
    ok = 0 if detail.get("is_success") else 1
    if not args.show_output:
        print("Output:  hidden (--show-output prints the job log; workflow logs are public)")
        return ok
    try:
        output = job_log(client, task_id)
    except JumpServerError as exc:
        print(f"Output:  unavailable ({str(exc)[:200]})")
        return ok
    if len(output) > RESULT_LIMIT:
        output = output[:RESULT_LIMIT] + "\n... (truncated)"
    print("Output:\n" + output)
    return ok


def mask(value):
    """Have the GitHub runner mask value in the log from here on."""
    if value and os.environ.get("GITHUB_ACTIONS") == "true":
        print(f"::add-mask::{value}", flush=True)


# Fields of a Control answer whose values are secrets.
SECRET_FIELD = re.compile(
    r"(^|_)(api_key|secret|credential|token|password|private_key|privatekey|server_key|psk|preshared_key)$",
    re.IGNORECASE,
)


# IPv4 and IPv6 address candidates in a string; scrub_addresses checks each.
ADDRESS_CANDIDATE = re.compile(r"\b\d{1,3}(?:\.\d{1,3}){3}\b|[0-9A-Fa-f:]*:[0-9A-Fa-f:]*:[0-9A-Fa-f:.]*")


def scrub_addresses(text):
    """Replace each IP address in text with ip:<first 6 of its sha1>.

    The logs are public and node hosts are often bare addresses; the hash
    still tells two addresses apart.
    """
    def scrub(match):
        candidate = match.group(0)
        try:
            ipaddress.ip_address(candidate)
        except ValueError:
            return candidate
        return "ip:" + hashlib.sha1(candidate.encode()).hexdigest()[:6]
    return ADDRESS_CANDIDATE.sub(scrub, text)


def redact(value, key=""):
    """Mask and replace the secret fields of a decoded JSON answer, and
    scrub the IP addresses of every other string."""
    if isinstance(value, dict):
        return {k: redact(v, k) for k, v in value.items()}
    if isinstance(value, list):
        return [redact(v, key) for v in value]
    if isinstance(value, str) and value and SECRET_FIELD.search(key):
        mask(value)
        return "***"
    if isinstance(value, str):
        return scrub_addresses(value)
    return value


X25519_P = 2**255 - 19


def x25519(scalar, u):
    """RFC 7748 X25519 on 32-byte little-endian strings."""
    k = bytearray(scalar)
    k[0] &= 248
    k[31] &= 127
    k[31] |= 64
    k = int.from_bytes(k, "little")
    x1 = int.from_bytes(u, "little") & ((1 << 255) - 1)
    x2, z2, x3, z3, swap = 1, 0, x1, 1, 0
    for t in reversed(range(255)):
        bit = (k >> t) & 1
        swap ^= bit
        if swap:
            x2, x3, z2, z3 = x3, x2, z3, z2
        swap = bit
        a, b = x2 + z2, x2 - z2
        aa, bb = a * a % X25519_P, b * b % X25519_P
        e = aa - bb
        c, d = x3 + z3, x3 - z3
        da, cb = d * a % X25519_P, c * b % X25519_P
        x3, z3 = (da + cb) ** 2 % X25519_P, x1 * (da - cb) ** 2 % X25519_P
        x2, z2 = aa * bb % X25519_P, e * (aa + 121665 * e) % X25519_P
    if swap:
        x2, z2 = x3, z3
    return (x2 * pow(z2, X25519_P - 2, X25519_P) % X25519_P).to_bytes(32, "little")


def b64url(raw):
    return base64.urlsafe_b64encode(raw).decode().rstrip("=")


def fill_generated(body):
    """Replace the @@NEW_...@@ placeholders of a control body with new secrets.

    @@NEW_REALITY_PUBLIC_KEY@@ is the public key of the body's
    @@NEW_REALITY_KEY@@, which is generated once per body.
    """
    reality = {}

    def reality_pair():
        if not reality:
            private = secrets.token_bytes(32)
            reality["private"] = b64url(private)
            reality["public"] = b64url(x25519(private, (9).to_bytes(32, "little")))
            mask(reality["private"])
            print(f"Generated Reality key pair, public key {reality['public']}")
        return reality

    def generated(match):
        name = match.group(1)
        if name == "NEW_REALITY_KEY":
            return reality_pair()["private"]
        if name == "NEW_REALITY_PUBLIC_KEY":
            return reality_pair()["public"]
        if name in ("NEW_SS2022_KEY_16", "NEW_SS2022_KEY_32"):
            value = base64.b64encode(secrets.token_bytes(int(name[-2:]))).decode()
            mask(value)
            return value
        raise JumpServerError(f"@@{name}@@ is not a known generated secret")
    return re.sub(r"@@(NEW_[A-Z0-9_]+)@@", generated, body)


class Control:
    """Control's administrator API with an admin API token."""

    def __init__(self, url, token, timeout=30):
        self.url = url.rstrip("/")
        self.token = token
        self.timeout = timeout

    def request(self, method, path, body=None):
        if not path.startswith("/api/"):
            raise JumpServerError("the path must start with /api/")
        response = requests.request(
            method, self.url + path, json=body, timeout=self.timeout,
            headers={"Authorization": f"Bearer {self.token}", "Accept": "application/json"},
        )
        try:
            answer = response.json() if response.content else None
        except ValueError:
            answer = response.text[:500]
        return response.status_code, answer


def control_client():
    missing = [k for k in ("CONTROL_URL", "CONTROL_ADMIN_TOKEN") if not os.environ.get(k)]
    if missing:
        raise JumpServerError("missing environment: " + ", ".join(missing))
    mask(os.environ["CONTROL_ADMIN_TOKEN"])
    return Control(os.environ["CONTROL_URL"], os.environ["CONTROL_ADMIN_TOKEN"])


def cmd_control(_client, args):
    control = control_client()
    body = None
    if args.body:
        try:
            body = json.loads(fill_generated(args.body))
        except ValueError as exc:
            raise JumpServerError(f"the body is not JSON: {exc}") from exc
    method = args.method.upper()
    status, answer = control.request(method, args.path, body)
    print(f"{method} {args.path}: HTTP {status}")
    print(json.dumps(redact(answer), ensure_ascii=False, indent=2)[:RESULT_LIMIT])
    return 0 if status < 400 else 1


def cmd_install_agent(client, args):
    """Install and enroll a node's Agent on one host.

    Control issues a one-hour install token for the node; the host runs
    Control's install command under systemd-run (it outlives the job
    timeout), logging to /root/anix-agent-install.log. Read the log with a
    later `run`.
    """
    hosts = select_hosts(client.hosts(), args.hosts)
    if len(hosts) != 1:
        raise JumpServerError("install-agent needs exactly one host")
    control = control_client()
    status, answer = control.request("POST", "/api/v4/kernel/agents/install-tokens",
                                     {"node": args.node, "ttl_seconds": 3600})
    if status >= 400 or not isinstance(answer, dict):
        print(json.dumps(redact(answer), ensure_ascii=False, indent=2)[:2000])
        raise JumpServerError(f"install token for {args.node}: HTTP {status}")
    data = answer.get("data") or {}
    mask(data.get("credential"))
    commands = {c.get("mirror"): c for c in data.get("commands") or []}
    chosen = commands.get(args.mirror) or {}
    if not chosen.get("command"):
        raise JumpServerError(f"Control offers no {args.mirror} install command")
    if not chosen.get("available", True):
        print(f"Note: {chosen.get('note')}")
    print(f"Install token for {data.get('node')}, Agent {data.get('agent_version')}, "
          f"expires {data.get('expires_at')}")
    # The job already runs as root, and a host need not have sudo.
    command = chosen["command"].replace("| sudo bash", "| bash")
    script = "#!/bin/bash\nset -o pipefail\n" + command + "\n"
    encoded = base64.b64encode(script.encode()).decode()
    mask(encoded)
    unit = "anix-agent-install-" + re.sub(r"[^a-z0-9-]", "-", args.node)
    remote = (
        f"echo {encoded} | base64 -d > /root/anix-agent-install.sh && chmod 700 /root/anix-agent-install.sh && "
        f"systemd-run --unit={unit}-$(date +%s) --collect "
        f"bash -c {shlex.quote('bash /root/anix-agent-install.sh > /root/anix-agent-install.log 2>&1')} && echo launched"
    )
    args.cmd, args.timeout = remote, 60
    args.describe = f"the anix-agent installer for {args.node}"
    return cmd_run(client, args)


def fill_secrets(cmd):
    """Replace each @@NAME@@ in cmd with the environment variable NAME."""
    def secret(match):
        name = match.group(1)
        if name not in SECRET_PLACEHOLDERS:
            raise JumpServerError(f"@@{name}@@ is not a known secret placeholder")
        value = os.environ.get(name, "")
        if not value:
            raise JumpServerError(f"@@{name}@@ is used but {name} is not set")
        if os.environ.get("GITHUB_ACTIONS") == "true":
            print(f"::add-mask::{value}")
        return value
    return re.sub(r"@@([A-Z0-9_]+)@@", secret, cmd)


def job_log(client, task_id, timeout=60):
    """Return the Ansible log of a finished job, which holds each host's output.

    JumpServer serves it only over the task log websocket, which accepts the
    same signed headers as the REST API. The task id is the execution's id.
    """
    import websocket  # websocket-client

    path = "/ws/ops/tasks/log/"
    url = client.url.replace("https://", "wss://", 1).replace("http://", "ws://", 1) + path
    headers = client.signed_headers("GET", path)
    try:
        ws = websocket.create_connection(
            url, header=[f"{k}: {v}" for k, v in headers.items()],
            suppress_origin=True, timeout=timeout,
        )
    except (websocket.WebSocketException, OSError) as exc:
        raise JumpServerError(f"task log websocket: {exc}") from exc
    chunks = []
    try:
        ws.send(json.dumps({"task": task_id, "type": "ansible"}))
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            message = json.loads(ws.recv())
            if message.get("event") in ("end", "unavailable"):
                break
            chunks.append(message.get("message") or "")
    except (websocket.WebSocketException, OSError, ValueError) as exc:
        if not chunks:
            raise JumpServerError(f"task log websocket: {exc}") from exc
    finally:
        ws.close()
    # The log ends with NUL padding.
    return strip_ansi("".join(chunks)).replace("\r\n", "\n").rstrip("\x00\n")


def strip_ansi(text):
    return re.sub(r"\x1b\[[0-9;]*[A-Za-z]", "", text)


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
    control = sub.add_parser("control", help="call Control's administrator API")
    control.add_argument("--method", default="GET", help="the HTTP method (default GET)")
    control.add_argument("--path", required=True, help="the API path, for example /api/v2/admin/nodes")
    control.add_argument("--body", default="", help="a JSON body; may hold @@NEW_...@@ placeholders")
    install = sub.add_parser("install-agent", help="install and enroll a Control node's Agent on one host")
    install.add_argument("--hosts", required=True, help="the JumpServer host")
    install.add_argument("--node", required=True, help="the Control node, proxy-<id> or forward-<id>")
    install.add_argument("--mirror", default="control", choices=("control", "cn", "github"),
                         help="where the host downloads the Agent (default control)")
    install.add_argument("--runas", default="root", help="the account to run as (default root)")
    install.add_argument("--show-output", action="store_true", help="print the host's output")
    args = parser.parse_args(argv)
    if args.command in ("run", "install-agent") and os.environ.get("GITHUB_ACTIONS") != "true":
        args.show_output = True

    missing = [k for k in ("JMS_URL", "JMS_KEY_ID", "JMS_SECRET") if not os.environ.get(k)]
    if args.command == "control":
        missing = []  # Control only
    if missing:
        print("missing environment: " + ", ".join(missing), file=sys.stderr)
        return 2
    client = None if args.command == "control" else Client(
        os.environ["JMS_URL"], os.environ["JMS_KEY_ID"], os.environ["JMS_SECRET"],
        org=os.environ.get("JMS_ORG") or DEFAULT_ORG,
    )
    commands = {"check": cmd_check, "hosts": cmd_hosts, "run": cmd_run,
                "control": cmd_control, "install-agent": cmd_install_agent}
    try:
        return commands[args.command](client, args)
    except (JumpServerError, requests.RequestException) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
