#!/usr/bin/env python3
import argparse
import base64
import json
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


def build_handler(token: str):
    services = {}
    limiters = {}
    expected_auth = "Basic " + base64.b64encode(f"admin:{token}".encode()).decode()

    class Handler(BaseHTTPRequestHandler):
        protocol_version = "HTTP/1.1"

        def _auth_ok(self) -> bool:
            return self.headers.get("Authorization", "").strip() == expected_auth

        def _read_json(self):
            raw_length = self.headers.get("Content-Length", "0")
            length = int(raw_length or 0)
            if length <= 0:
                return None
            payload = self.rfile.read(length)
            return json.loads(payload.decode("utf-8")) if payload else None

        def _send(self, status: int, payload=None):
            body = b""
            if payload is not None:
                body = json.dumps(payload).encode("utf-8")
            self.send_response(status)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            if body:
                self.wfile.write(body)

        def _require_auth(self) -> bool:
            if self._auth_ok():
                return True
            self._send(401, {"error": "unauthorized"})
            return False

        def do_GET(self):
            if not self._require_auth():
                return
            if self.path == "/api/config/services":
                self._send(200, list(services.values()))
                return
            if self.path == "/api/config/limiters":
                self._send(200, list(limiters.values()))
                return
            self._send(404, {"error": "not found"})

        def do_POST(self):
            if not self._require_auth():
                return
            if self.path == "/api/config/services":
                payload = self._read_json() or {}
                name = str(payload.get("name") or "").strip()
                if not name:
                    self._send(400, {"error": "name required"})
                    return
                services[name] = payload
                self._send(200, payload)
                return
            if self.path == "/api/config/limiters":
                payload = self._read_json() or {}
                name = str(payload.get("name") or "").strip()
                if not name:
                    self._send(400, {"error": "name required"})
                    return
                limiters[name] = payload
                self._send(200, payload)
                return
            self._send(404, {"error": "not found"})

        def do_PUT(self):
            if not self._require_auth():
                return
            if self.path.startswith("/api/config/limiters/"):
                name = self.path.rsplit("/", 1)[-1]
                payload = self._read_json() or {}
                payload["name"] = name
                limiters[name] = payload
                self._send(200, payload)
                return
            self._send(404, {"error": "not found"})

        def do_DELETE(self):
            if not self._require_auth():
                return
            if self.path.startswith("/api/config/services/"):
                name = self.path.rsplit("/", 1)[-1]
                services.pop(name, None)
                self._send(200, {"deleted": name})
                return
            if self.path.startswith("/api/config/limiters/"):
                name = self.path.rsplit("/", 1)[-1]
                limiters.pop(name, None)
                self._send(200, {"deleted": name})
                return
            self._send(404, {"error": "not found"})

        def log_message(self, fmt: str, *args):
            print(fmt % args, flush=True)

    return Handler


def main():
    parser = argparse.ArgumentParser(description="Mock gost relay API for local NodeX smoke tests")
    parser.add_argument("--host", default="127.0.0.1")
    parser.add_argument("--port", type=int, default=19080)
    parser.add_argument("--token", default="relay-token")
    args = parser.parse_args()

    server = ThreadingHTTPServer((args.host, args.port), build_handler(args.token))
    print(f"mock gost api listening on http://{args.host}:{args.port}", flush=True)
    server.serve_forever()


if __name__ == "__main__":
    main()
