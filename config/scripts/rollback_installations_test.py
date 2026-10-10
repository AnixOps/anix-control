import contextlib
import io
import json
import os
import threading
import unittest
from unittest import mock
from http.server import BaseHTTPRequestHandler, HTTPServer

import rollback_installations as rb


class FakePanel:
    """A 4.1.0 Control: refuses every PUT while forward is not on 4.1.0, like release_invalid.

    It holds one release per installation (the one it is on) plus `old` for every plugin not in
    `without`, answers release_not_found for a PUT to a release it does not hold, and lists its
    releases on GET /api/v3/plugin-releases. `refuse` maps (plugin_id, target) to (status, code), or
    to "drop" to close the connection without an answer, for a PUT of that installation.
    """

    def __init__(self, rows, old="4.1.0", without=()):
        self.rows = {(r["plugin_id"], r["target"]): dict(r, health="healthy", enabled=r.get("enabled", True)) for r in rows}
        self.old = old
        self.releases = {(r["plugin_id"], r["desired_version"]) for r in rows}
        self.releases |= {(r["plugin_id"], old) for r in rows if r["plugin_id"] not in without}
        self.releases_status = 200
        self.releases_body = None  # raw bytes to answer the release listing with instead of the JSON
        self.refuse = {}
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
                if self.path == "/api/v3/plugin-releases":
                    if panel.releases_body is not None:
                        self.send_response(200)
                        self.send_header("Content-Length", str(len(panel.releases_body)))
                        self.end_headers()
                        return self.wfile.write(panel.releases_body)
                    if panel.releases_status != 200:
                        return self._send(panel.releases_status, {"error": {"code": "internal_error", "message": "boom"}})
                    return self._send(200, {"data": [{"plugin_id": p, "version": v} for p, v in sorted(panel.releases)]})
                if self.path == "/api/v3/plugin-installations":
                    return self._send(200, {"data": list(panel.rows.values())})
                self._send(404, {"error": {"code": "not_found", "message": self.path}})

            def do_PUT(self):
                body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
                panel.puts.append(body)
                refusal = panel.refuse.get((body["plugin_id"], body["target"]))
                if refusal == "drop":
                    self.close_connection = True
                    return
                if refusal:
                    return self._send(refusal[0], {"error": {"code": refusal[1], "message": "refused by the test"}})
                if (body["plugin_id"], body["desired_version"]) not in panel.releases:
                    return self._send(400, {"error": {"code": "release_not_found", "message": "verified plugin release not found"}})
                forward = panel.rows.get(("forward", "control"))
                if forward and forward["desired_version"] != panel.old and body["plugin_id"] != "forward":
                    return self._send(409, {"error": {"code": "release_invalid", "message": 'unknown kernel capability "kernel.forward.v1"'}})
                row = panel.rows[(body["plugin_id"], body["target"])]
                row["desired_version"] = body["desired_version"]
                self._send(200, {"data": row})

        self.server = HTTPServer(("127.0.0.1", 0), Handler)
        self.url = f"http://127.0.0.1:{self.server.server_port}"
        threading.Thread(target=self.server.serve_forever, kwargs={"poll_interval": 0.01}, daemon=True).start()

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


def commercial(version="4.0.3"):
    """Installations of packages that never had a 4.1.0 release: they sit on their 4.0 build."""
    return [{"plugin_id": plugin, "target": "control", "desired_version": version} for plugin in ("order", "payment")]


class SkippedInstallationsTest(unittest.TestCase):
    def setUp(self):
        self.panel = FakePanel(rows() + commercial(), without=("order", "payment"))
        self.addCleanup(self.panel.close)

    def go(self, apply=True, allow_skipped=False, wait=5):
        out = io.StringIO()
        code = rb.run(self.panel.url, "t", "4.1.0", apply, wait, out=out, allow_skipped=allow_skipped)
        return code, out.getvalue()

    def test_a_missing_release_does_not_leave_the_rest_on_the_old_version(self):
        # the defect: the PUT of order answered release_not_found and every installation after it stayed on 4.2.0
        try:
            rb.run(self.panel.url, "t", "4.1.0", True, 5, out=io.StringIO())
        except rb.RollbackError:
            pass
        left = sorted(plugin for (plugin, _), row in self.panel.rows.items() if row["desired_version"] == "4.2.0")
        self.assertEqual(left, [])

    def test_the_installations_without_the_release_are_listed_up_front_and_skipped(self):
        code, text = self.go()
        self.assertEqual(code, 3)
        listing = "2 installations cannot be moved to 4.1.0 and are skipped:"
        self.assertIn(listing, text)
        self.assertIn("order/control: no 4.1.0 release of order on this panel (it has: 4.0.3); stays on 4.0.3", text)
        self.assertIn("payment/control: no 4.1.0 release of payment on this panel (it has: 4.0.3); stays on 4.0.3", text)
        # the list comes before the first change, and the plan holds only what can move
        self.assertLess(text.index(listing), text.index("moved forward/control"))
        self.assertIn("6 of 8 installations to move to 4.1.0, in this order:", text)
        self.assertNotIn("order/control: 4.0.3 -> 4.1.0", text)

    def test_the_rest_is_moved_in_order_and_the_skipped_are_left_alone(self):
        self.go()
        puts = [p["plugin_id"] for p in self.panel.puts]
        self.assertEqual(puts[:2], ["forward", "identity-platform"])
        self.assertEqual(len(puts), 6)
        self.assertNotIn("order", puts)
        self.assertNotIn("payment", puts)
        for (plugin, _), row in self.panel.rows.items():
            self.assertEqual(row["desired_version"], "4.0.3" if plugin in ("order", "payment") else "4.1.0", plugin)

    def test_the_run_ends_with_a_summary(self):
        _, text = self.go()
        lines = text.splitlines()
        self.assertEqual(lines[-3], "all 6 installations are on 4.1.0 and healthy")
        self.assertEqual(lines[-2], "summary: 6 rolled back to 4.1.0, 2 skipped, 0 failed")
        self.assertEqual(lines[-1], "  skipped (no 4.1.0 release): order/control, payment/control")

    def test_the_exit_status_is_nonzero_unless_skipping_is_allowed(self):
        self.assertEqual(self.go(allow_skipped=False)[0], 3)
        self.assertEqual(self.go(allow_skipped=True)[0], 0)

    def test_a_dry_run_lists_the_skipped_and_ends_with_the_status_the_run_would(self):
        code, text = self.go(apply=False)
        self.assertEqual(code, 3)
        self.assertEqual(self.panel.puts, [])
        self.assertIn("order/control: no 4.1.0 release of order", text)
        self.assertIn("dry run: nothing was changed; add --apply to move them", text)
        self.assertEqual(self.go(apply=False, allow_skipped=True)[0], 0)
        self.assertEqual(self.panel.puts, [])

    def test_a_skipped_installation_is_not_waited_for(self):
        self.panel.rows[("order", "control")]["health"] = "unhealthy"
        code, text = self.go()
        self.assertEqual(code, 3)
        self.assertNotIn("not healthy", text)

    def test_a_moved_installation_that_is_not_healthy_is_still_exit_two(self):
        self.panel.rows[("ticket", "control")]["health"] = "unhealthy"
        code, text = self.go(wait=0)
        self.assertEqual(code, 2)
        self.assertIn("not healthy on 4.1.0 yet: ticket/control", text)
        self.assertIn("summary: 6 rolled back to 4.1.0, 2 skipped, 0 failed, 1 not healthy yet", text)

    def test_a_second_run_moves_nothing_and_skips_the_same_ones(self):
        self.go()
        count = len(self.panel.puts)
        code, text = self.go()
        self.assertEqual(code, 3)
        self.assertEqual(len(self.panel.puts), count)
        self.assertIn("2 installations cannot be moved to 4.1.0 and are skipped:", text)
        self.assertIn("no installation can be moved to 4.1.0", text)
        self.assertIn("summary: 0 rolled back to 4.1.0, 2 skipped, 0 failed", text)

    def test_nothing_movable_is_reported_and_changes_nothing(self):
        self.panel.rows = {k: v for k, v in self.panel.rows.items() if k[0] in ("order", "payment")}
        code, text = self.go()
        self.assertEqual(code, 3)
        self.assertEqual(self.panel.puts, [])
        self.assertIn("no installation can be moved to 4.1.0", text)
        self.assertEqual(self.go(allow_skipped=True)[0], 0)

    def test_a_plugin_with_no_release_at_all_says_so(self):
        movable, skipped = rb.split_unavailable(commercial("4.2.0"), {}, "4.1.0")
        self.assertEqual(movable, [])
        self.assertEqual(len(skipped), 2)
        self.assertIn("(it has: none); stays on 4.2.0", skipped[0][1])

    def test_split_unavailable_keeps_the_plan_order(self):
        todo = rb.plan(rows() + commercial(), "4.1.0")
        movable, skipped = rb.split_unavailable(todo, {r["plugin_id"]: ["4.1.0"] for r in rows()}, "4.1.0")
        self.assertEqual([r["plugin_id"] for r in movable], [r["plugin_id"] for r in todo if r["plugin_id"] not in ("order", "payment")])
        self.assertEqual([r["plugin_id"] for r, _ in skipped], ["order", "payment"])


class RefusalTest(unittest.TestCase):
    def setUp(self):
        self.panel = FakePanel(rows())
        self.addCleanup(self.panel.close)

    def go(self, apply=True):
        out = io.StringIO()
        try:
            return rb.run(self.panel.url, "t", "4.1.0", apply, 5, out=out), out.getvalue()
        except rb.RollbackError as err:
            return err, out.getvalue()

    def test_release_not_found_from_the_put_is_skipped_and_the_run_continues(self):
        # the listing says the release exists, the PUT disagrees (it went away in between)
        self.panel.refuse[("plan", "control")] = (400, "release_not_found")
        code, text = self.go()
        self.assertEqual(code, 3)
        order = [p["plugin_id"] for p in self.panel.puts]
        self.assertEqual(order[-2:], ["subscription", "ticket"])
        self.assertIn("skipped plan/control: PUT /api/v3/plugin-installations answered 400: release_not_found", text)
        self.assertIn("summary: 5 rolled back to 4.1.0, 1 skipped, 0 failed", text)
        self.assertIn("  skipped (no 4.1.0 release): plan/control", text)

    def test_any_other_refusal_stops_and_the_summary_names_it(self):
        for status, code in ((401, "unauthorized"), (403, "forbidden"), (409, "plugin_conflict"), (500, "internal_error"), (502, "bad_gateway")):
            with self.subTest(status=status):
                self.setUp()
                self.panel.refuse[("plan", "control")] = (status, code)
                err, text = self.go()
                self.assertIsInstance(err, rb.RollbackError)
                self.assertIn(str(status), str(err))
                self.assertEqual((err.status, err.code), (status, code))
                # forward, identity-platform and machine-telemetry moved; plan failed; the rest was not attempted
                self.assertEqual([p["plugin_id"] for p in self.panel.puts], ["forward", "identity-platform", "machine-telemetry", "plan"])
                self.assertIn("summary: 3 rolled back to 4.1.0, 0 skipped, 1 failed, 2 not attempted", text)
                self.assertIn(f"  failed: plan/control: PUT /api/v3/plugin-installations answered {status}: {code}", text)
                self.assertIn("  not attempted: subscription/control, ticket/control", text)
                self.panel.close()

    def test_a_dropped_connection_stops_with_a_summary(self):
        self.panel.refuse[("plan", "control")] = "drop"
        err, text = self.go()
        self.assertIsInstance(err, rb.RollbackError)
        self.assertIn("PUT /api/v3/plugin-installations failed", str(err))
        self.assertIn("summary: 3 rolled back to 4.1.0, 0 skipped, 1 failed, 2 not attempted", text)

    def test_a_failing_release_listing_stops_before_anything_changes(self):
        self.panel.releases_status = 500
        err, _ = self.go()
        self.assertIsInstance(err, rb.RollbackError)
        self.assertIn("GET /api/v3/plugin-releases answered 500", str(err))
        self.assertEqual(self.panel.puts, [])

    def test_an_answer_that_is_not_json_stops_cleanly(self):
        self.panel.releases_body = b"<html>a proxy error page</html>"
        err, _ = self.go()
        self.assertIsInstance(err, rb.RollbackError)
        self.assertIn("GET /api/v3/plugin-releases answered something that is not the expected JSON", str(err))
        self.assertEqual(self.panel.puts, [])

    def test_a_bad_token_on_the_release_listing_is_reported(self):
        with self.assertRaises(rb.RollbackError) as caught:
            rb.call(self.panel.url, "wrong", "GET", path=rb.RELEASES)
        self.assertIn("GET /api/v3/plugin-releases answered 401", str(caught.exception))

    def test_main_maps_the_outcomes_to_exit_codes(self):
        self.panel.refuse[("plan", "control")] = (400, "release_not_found")
        base = ["--panel", self.panel.url, "--version", "4.1.0", "--apply", "--wait", "1"]
        with mock.patch.dict(os.environ, {"ANIX_CONTROL_TOKEN": "t"}):
            with contextlib.redirect_stdout(io.StringIO()):
                self.assertEqual(rb.main(base), 3)
                self.assertEqual(rb.main(base + ["--allow-skipped"]), 0)
            self.panel.refuse[("plan", "control")] = (500, "internal_error")
            stderr = io.StringIO()
            with contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(stderr):
                self.assertEqual(rb.main(base + ["--allow-skipped"]), 1)
            self.assertIn("answered 500", stderr.getvalue())


if __name__ == "__main__":
    unittest.main()
