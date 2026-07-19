from __future__ import annotations

import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parents[2]
CHECKER = REPO_ROOT / "config" / "scripts" / "check_v2_package_route_catalog.py"
INVENTORY = REPO_ROOT / "config" / "scripts" / "v2_route_inventory.go"
CATALOG = REPO_ROOT / "config" / "v2-package-route-catalog.json"
ROUTER = REPO_ROOT / "internal" / "router" / "router.go"
FIXTURES = REPO_ROOT / "config" / "scripts" / "testdata"
SAMPLE_ROUTER = FIXTURES / "v2-router-ast-fixture.go"
FIXTURE_INVENTORY = FIXTURES / "v2-route-inventory-fixture.json"
VALID_FIXTURE_CATALOG = FIXTURES / "v2-route-catalog-valid.json"


class RouteCatalogTest(unittest.TestCase):
    def run_inventory(self, router: Path) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            ["go", "run", str(INVENTORY), "--router", str(router), "--format", "json"],
            cwd=REPO_ROOT,
            check=False,
            text=True,
            capture_output=True,
        )

    def run_checker(self, catalog: Path, *, inventory: Path | None = None, router: Path | None = None) -> subprocess.CompletedProcess[str]:
        command = [sys.executable, str(CHECKER), "--catalog", str(catalog)]
        if inventory is not None:
            command.extend(["--inventory", str(inventory)])
        if router is not None:
            command.extend(["--router", str(router)])
        return subprocess.run(
            command,
            cwd=REPO_ROOT,
            check=False,
            text=True,
            capture_output=True,
        )

    def test_inventory_expands_nested_groups_any_and_handler_identifiers(self) -> None:
        result = self.run_inventory(SAMPLE_ROUTER)
        self.assertEqual(0, result.returncode, result.stderr or result.stdout)

        rows = json.loads(result.stdout)
        actual = {
            (row["method"], row["path"], row["handler"], row["middleware_group"], row["transport"])
            for row in rows
        }
        expected = {
            ("GET", "/api/v2/login", "authHandler.Login", "public", "http"),
            ("POST", "/api/v2/private/item/:id", "itemHandler.Create", "user", "http"),
            ("DELETE", "/api/v2/admin/users/:id", "adminHandler.Delete", "admin", "http"),
            ("PUT", "/api/v2/direct/:id", "directHandler.Update", "public", "http"),
            ("GET", "/api/v2/inherited-node-auth", "inheritedHandler.Get", "node-api", "http"),
            ("GET", "/api/v2/inherited-inline-node-auth", "inheritedHandler.Inline", "node-api", "http"),
            ("PATCH", "/api/v2/patch", "patchHandler.Update", "public", "http"),
            ("GET", "/api/v2/inline-auth", "inlineHandler.Get", "user", "http"),
            ("GET", "/api/v2/chained-use", "chainedHandler.Get", "user", "http"),
            ("PATCH", "/api/v2/handled", "handledHandler.Patch", "user", "http"),
            ("PATCH", "/api/v2/matched", "matchedHandler.Patch", "user", "http"),
            ("DELETE", "/api/v2/matched", "matchedHandler.Patch", "user", "http"),
        }
        any_methods = {"GET", "POST", "PUT", "PATCH", "HEAD", "OPTIONS", "DELETE", "CONNECT", "TRACE"}
        expected.update(
            (method, "/api/v2/fanout", "fanoutHandler.Handle", "public", "http") for method in any_methods
        )
        self.assertEqual(expected, actual)

    def test_inventory_uses_the_mutated_router_ast_instead_of_a_static_or_regex_route_list(self) -> None:
        source = SAMPLE_ROUTER.read_text(encoding="utf-8")
        source = source.replace('"/login", authHandler.Login', '"/login/:user_id", authHandler.LoginForUser')
        with tempfile.TemporaryDirectory() as temporary:
            router = Path(temporary) / "router.go"
            router.write_text(source, encoding="utf-8")
            result = self.run_inventory(router)

        self.assertEqual(0, result.returncode, result.stderr or result.stdout)
        rows = json.loads(result.stdout)
        routes = {(row["method"], row["path"], row["handler"]) for row in rows}
        self.assertIn(("GET", "/api/v2/login/:user_id", "authHandler.LoginForUser"), routes)
        self.assertNotIn(("GET", "/api/v2/login", "authHandler.Login"), routes)
        self.assertNotIn(("GET", "/api/v2/comment-only", "decoyHandler.Get"), routes)
        self.assertNotIn(("POST", "/api/v2/string-only", "decoyHandler.Post"), routes)

    def test_inventory_resolves_local_literal_group_and_route_paths(self) -> None:
        source = SAMPLE_ROUTER.read_text(encoding="utf-8")
        source += """
func LiteralAliases(r *gin.Engine) {
	v2Prefix := "/api/v2"
	v2 := r.Group(v2Prefix)
	v2.GET("/literal-group", literalHandler.Group)
	directPath := "/api/v2/literal-direct"
	r.GET(directPath, literalHandler.Direct)
}
"""
        with tempfile.TemporaryDirectory() as temporary:
            router = Path(temporary) / "router.go"
            router.write_text(source, encoding="utf-8")
            result = self.run_inventory(router)

        self.assertEqual(0, result.returncode, result.stderr or result.stdout)
        routes = {(row["method"], row["path"], row["handler"]) for row in json.loads(result.stdout)}
        self.assertIn(("GET", "/api/v2/literal-group", "literalHandler.Group"), routes)
        self.assertIn(("GET", "/api/v2/literal-direct", "literalHandler.Direct"), routes)

    def test_inventory_rejects_unresolved_local_group_paths(self) -> None:
        source = SAMPLE_ROUTER.read_text(encoding="utf-8")
        source += """
func UnresolvedGroup(r *gin.Engine) {
	prefix := cfg.DynamicPrefix
	dynamic := r.Group(prefix)
	dynamic.GET("/hidden", hiddenHandler.Get)
}
"""
        with tempfile.TemporaryDirectory() as temporary:
            router = Path(temporary) / "router.go"
            router.write_text(source, encoding="utf-8")
            result = self.run_inventory(router)

        self.assertNotEqual(0, result.returncode)
        self.assertIn("Gin Group path must be a string literal", result.stderr)

    def test_inventory_fails_closed_for_unowned_v2_registration_forms(self) -> None:
        cases = {
            "static": (
                """
func StaticRoute(r *gin.Engine) {
	v2 := r.Group("/api/v2")
	v2.Static("/assets", "./assets")
}
""",
                "unsupported Gin selector Static on /api/v2 group",
            ),
            "unknown": (
                """
func UnknownRoute(r *gin.Engine) {
	v2 := r.Group("/api/v2")
	v2.Unrecognized("/hidden", hiddenHandler.Get)
}
""",
                "unsupported Gin selector Unrecognized on /api/v2 group",
            ),
            "chain": (
                """
func ChainedRegistration(r *gin.Engine) {
	v2 := r.Group("/api/v2")
	v2.GET("/outer", chainedHandler.Get).POST("/inner", chainedHandler.Post)
}
""",
                "unsupported Gin registration expression GET on /api/v2 group",
            ),
            "assignment": (
                """
func AssignmentBypass(r *gin.Engine) {
	v2 := r.Group("/api/v2")
	_ = v2.StaticFile("/asset", "./asset")
}
""",
                "unsupported Gin selector StaticFile on /api/v2 group",
            ),
            "declaration": (
                """
func DeclarationBypass(r *gin.Engine) {
	v2 := r.Group("/api/v2")
	var _ = v2.StaticFS("/asset", filesystem)
}
""",
                "unsupported Gin selector StaticFS on /api/v2 group",
            ),
            "root_dynamic": (
                """
func RootDynamicRoute(r *gin.Engine) {
	path := cfg.DynamicPath
	r.GET(path, dynamicHandler.Get)
}
""",
                "root Gin Engine path must be statically resolvable",
            ),
        }
        source = SAMPLE_ROUTER.read_text(encoding="utf-8")
        with tempfile.TemporaryDirectory() as temporary:
            temporary_root = Path(temporary)
            for name, (addition, expected_error) in cases.items():
                router = temporary_root / f"{name}.go"
                router.write_text(source + addition, encoding="utf-8")
                result = self.run_inventory(router)
                self.assertNotEqual(0, result.returncode, name)
                self.assertIn(expected_error, result.stderr, name)

    def test_inventory_marks_exact_existing_v2_websocket_routes(self) -> None:
        result = self.run_inventory(ROUTER)
        self.assertEqual(0, result.returncode, result.stderr or result.stdout)

        rows = json.loads(result.stdout)
        websocket_routes = {(row["method"], row["path"]) for row in rows if row["transport"] == "websocket"}
        self.assertEqual(
            {
                ("GET", "/api/v2/admin/ws/monitor"),
                ("GET", "/api/v2/node/ws"),
                ("GET", "/api/v2/agent/ws"),
            },
            websocket_routes,
        )

    def test_checker_rejects_duplicate_catalog_fixture(self) -> None:
        result = self.run_checker(FIXTURES / "v2-route-catalog-duplicate.json", inventory=FIXTURE_INVENTORY)
        self.assertNotEqual(0, result.returncode)
        self.assertIn("duplicate method/path", result.stdout + result.stderr)

    def test_checker_rejects_unowned_catalog_fixture(self) -> None:
        result = self.run_checker(FIXTURES / "v2-route-catalog-unowned.json", inventory=FIXTURE_INVENTORY)
        self.assertNotEqual(0, result.returncode)
        self.assertIn("unowned v2 route", result.stdout + result.stderr)

    def test_checker_rejects_mutated_catalog_schema_values(self) -> None:
        valid = json.loads(VALID_FIXTURE_CATALOG.read_text(encoding="utf-8"))
        mutations = {
            "owner": ("owner", "not-a-package", "unsupported owner"),
            "envelope": ("envelope", "not-an-envelope", "unsupported envelope"),
            "middleware": ("middleware_group", "not-a-middleware-group", "unsupported middleware group"),
            "transport": ("transport", "not-a-transport", "unsupported transport"),
            "wildcard": ("path", "/api/v2/users/*", "wildcard"),
        }
        with tempfile.TemporaryDirectory() as temporary:
            temporary_root = Path(temporary)
            for name, (field, value, expected_error) in mutations.items():
                mutated = [dict(row) for row in valid]
                mutated[0][field] = value
                catalog = temporary_root / f"{name}.json"
                catalog.write_text(json.dumps(mutated), encoding="utf-8")
                result = self.run_checker(catalog, inventory=FIXTURE_INVENTORY)
                self.assertNotEqual(0, result.returncode, name)
                self.assertIn(expected_error, result.stdout + result.stderr, name)

    def test_checker_allows_known_topology_owners_without_current_v2_rows(self) -> None:
        valid = json.loads(VALID_FIXTURE_CATALOG.read_text(encoding="utf-8"))
        with tempfile.TemporaryDirectory() as temporary:
            temporary_root = Path(temporary)
            for owner in ("nftables-forward", "nat-egress"):
                catalog = [dict(row) for row in valid]
                catalog[0]["owner"] = owner
                catalog[0]["route_id"] = "nftables.fixture.route" if owner == "nftables-forward" else "nat.fixture.route"
                catalog_path = temporary_root / f"{owner}.json"
                catalog_path.write_text(json.dumps(catalog), encoding="utf-8")
                result = self.run_checker(catalog_path, inventory=FIXTURE_INVENTORY)
                self.assertEqual(0, result.returncode, result.stderr or result.stdout)

    def test_catalog_records_resolved_ownership_boundaries(self) -> None:
        self.assertTrue(CATALOG.is_file(), "v2 package route catalog is missing")
        catalog = json.loads(CATALOG.read_text(encoding="utf-8"))
        rows = {(row["method"], row["path"]): row for row in catalog}

        expected = {
            ("GET", "/api/v2/admin/loadbalancers"): ("proxy-node", "proxy.loadbalancer."),
            ("POST", "/api/v2/speed-limit/create"): ("plan", "plan.speed_limit."),
            ("GET", "/api/v2/admin/nodes/:id/protocols"): ("protocol-runtime", "protocol."),
            ("GET", "/api/v2/admin/protocol-templates"): ("protocol-runtime", "protocol."),
            ("POST", "/api/v2/node/register"): ("proxy-node", "proxy."),
            ("GET", "/api/v2/server/UniProxy/config"): ("proxy-node", "proxy."),
            ("POST", "/api/v2/admin/forward/create"): ("forward", "forward."),
            ("GET", "/api/v2/admin/forward/nodex/status"): ("gost-mesh", "gost."),
            ("POST", "/api/v2/admin/forward/test-connection"): ("gost-mesh", "gost."),
            ("POST", "/api/v2/admin/wireguard/keypair"): ("wireguard", "wireguard."),
        }
        for key, (owner, route_id_prefix) in expected.items():
            with self.subTest(method=key[0], path=key[1]):
                self.assertIn(key, rows)
                self.assertEqual(owner, rows[key]["owner"])
                self.assertTrue(rows[key]["route_id"].startswith(route_id_prefix))

    def test_catalog_preserves_legacy_envelope_kinds(self) -> None:
        self.assertTrue(CATALOG.is_file(), "v2 package route catalog is missing")
        catalog = json.loads(CATALOG.read_text(encoding="utf-8"))
        envelopes = {(row["method"], row["path"]): row["envelope"] for row in catalog}
        expected = {
            ("POST", "/api/v2/login"): "data",
            ("POST", "/api/v2/payment/callback/:type"): "raw",
            ("POST", "/api/v2/payment/stripe/webhook"): "raw",
            ("GET", "/api/v2/server/UniProxy/config"): "raw",
            ("POST", "/api/v2/agent/heartbeat"): "raw",
            ("GET", "/api/v2/forward-agent/install.sh"): "raw",
            ("GET", "/api/v2/admin/ws/monitor"): "websocket",
        }
        for key, envelope in expected.items():
            with self.subTest(method=key[0], path=key[1]):
                self.assertEqual(envelope, envelopes[key])

    def test_catalog_equals_router_v2_inventory(self) -> None:
        result = self.run_checker(CATALOG, router=ROUTER)
        self.assertEqual(0, result.returncode, result.stderr or result.stdout)
        self.assertIn("v2 package route catalog passed", result.stdout)


if __name__ == "__main__":
    unittest.main()
