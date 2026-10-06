import contextlib
import io
import json
import threading
import unittest
from http.server import BaseHTTPRequestHandler, HTTPServer

import rollback_installations as rb


class FakePanel:
    """A 4.1.0 Control: refuses every PUT while forward is not on 4.1.0, like release_invalid."""

    def __init__(self, rows, old="4.1.0"):
        self.rows = {(r["plugin_id"], r["target"]): dict(r, health="healthy", enabled=r.get("enabled", True)) for r in rows}
        self.old = old
        self.puts = []
        panel = self

        class Handler(BaseHTTPRequestHandler):
            def log_message(self, *args):
                pass

            def _send(self, status, payload):
                body = json.dumps(payload).encode()
                self.send_response(status)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(body)))
                self.end_headers()
                self.wfile.write(body)

            def do_GET(self):
                if self.headers.get("Authorization") != "Bearer t":
                    return self._send(401, {"error": {"code": "unauthorized", "message": "no"}})
                self._send(200, {"data": list(panel.rows.values())})

            def do_PUT(self):
                body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
                panel.puts.append(body)
                forward = panel.rows.get(("forward", "control"))
                if forward and forward["desired_version"] != panel.old and body["plugin_id"] != "forward":
                    return self._send(409, {"error": {"code": "release_invalid", "message": 'unknown kernel capability "kernel.forward.v1"'}})
                row = panel.rows[(body["plugin_id"], body["target"])]
                row["desired_version"] = body["desired_version"]
                self._send(200, {"data": row})

        self.server = HTTPServer(("127.0.0.1", 0), Handler)
        self.url = f"http://127.0.0.1:{self.server.server_port}"
        threading.Thread(target=self.server.serve_forever, daemon=True).start()

    def close(self):
        self.server.shutdown()
        self.server.server_close()


def rows(version="4.2.0"):
    return [
        {"plugin_id": plugin, "target": "control", "desired_version": version}
        for plugin in ("plan", "identity-platform", "ticket", "forward", "subscription")
    ] + [{"plugin_id": "machine-telemetry", "target": "agent", "desired_version": version, "enabled": False}]


class RollbackTest(unittest.TestCase):
    def setUp(self):
        self.panel = FakePanel(rows())
        self.addCleanup(self.panel.close)

    def run_rb(self, apply):
        out = io.StringIO()
        code = rb.run(self.panel.url, "t", "4.1.0", apply, 5, out=out)
        return code, out.getvalue()

    def test_plan_puts_forward_then_identity_then_the_rest(self):
        order = [r["plugin_id"] for r in rb.plan(rows(), "4.1.0")]
        self.assertEqual(order[:2], ["forward", "identity-platform"])
        self.assertEqual(order[2:], sorted(order[2:]))

    def test_dry_run_changes_nothing(self):
        code, text = self.run_rb(False)
        self.assertEqual(code, 0)
        self.assertEqual(self.panel.puts, [])
        self.assertIn("dry run", text)
        self.assertIn("forward/control: 4.2.0 -> 4.1.0", text)

    def test_apply_moves_everything_in_order_and_keeps_enabled(self):
        code, _ = self.run_rb(True)
        self.assertEqual(code, 0)
        self.assertEqual([p["plugin_id"] for p in self.panel.puts[:2]], ["forward", "identity-platform"])
        self.assertEqual(len(self.panel.puts), 6)
        self.assertTrue(all(r["desired_version"] == "4.1.0" for r in self.panel.rows.values()))
        agent = next(p for p in self.panel.puts if p["plugin_id"] == "machine-telemetry")
        self.assertEqual((agent["target"], agent["enabled"]), ("agent", False))

    def test_a_second_run_has_nothing_to_do(self):
        self.run_rb(True)
        count = len(self.panel.puts)
        code, text = self.run_rb(True)
        self.assertEqual(code, 0)
        self.assertIn("nothing to do", text)
        self.assertEqual(len(self.panel.puts), count)

    def test_stops_at_the_first_refusal(self):
        # the panel keeps refusing after forward moved: the run stops at identity-platform
        self.panel.old = "never"
        with self.assertRaises(rb.RollbackError) as caught:
            self.run_rb(True)
        self.assertIn("409", str(caught.exception))
        self.assertIn("release_invalid", str(caught.exception))
        self.assertEqual([p["plugin_id"] for p in self.panel.puts], ["forward", "identity-platform"])

    def test_a_bad_token_is_reported(self):
        with self.assertRaises(rb.RollbackError) as caught:
            rb.run(self.panel.url, "wrong", "4.1.0", False, 1)
        self.assertIn("401", str(caught.exception))

    def test_main_needs_a_token_and_a_sane_version(self):
        import os

        os.environ.pop("ANIX_CONTROL_TOKEN", None)
        with contextlib.redirect_stderr(io.StringIO()):
            self.assertEqual(rb.main(["--panel", self.panel.url, "--version", "4.1.0"]), 1)
        os.environ["ANIX_CONTROL_TOKEN"] = "t"
        self.addCleanup(os.environ.pop, "ANIX_CONTROL_TOKEN", None)
        with contextlib.redirect_stderr(io.StringIO()):
            self.assertEqual(rb.main(["--panel", self.panel.url, "--version", "4.1.0; rm"]), 1)
        with contextlib.redirect_stdout(io.StringIO()):
            self.assertEqual(rb.main(["--panel", self.panel.url, "--version", "4.1.0"]), 0)


if __name__ == "__main__":
    unittest.main()
