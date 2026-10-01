from __future__ import annotations

import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parents[2]
CHECKER = REPO_ROOT / "config" / "scripts" / "check_plugin_only_routes.py"


class PluginOnlyRoutesTest(unittest.TestCase):
    def write_fixture(
        self,
        root: Path,
        *,
        owner: str = "knowledge",
        method: str = "GET",
        path: str = "/api/v2/user/knowledge",
        route_id: str = "knowledge.article.list",
        envelope: str = "data",
        middleware_group: str = "user",
        transport: str = "http",
        handler: str | None = None,
        declaration_owner: str | None = None,
        declaration_routes: list[dict[str, str]] | None = None,
        include_generic_host: bool = True,
        mode: str = "bridged",
        legacy: str = "router",
        reason: str | None = None,
        package_host_source: str | None = None,
        identity_bridge_source: str = "",
    ) -> tuple[Path, Path, Path]:
        packages_root = root / "packages"
        package_root = packages_root / owner
        compat_dir = package_root / "compat"
        migration_dir = package_root / "migrations"
        compat_dir.mkdir(parents=True)
        migration_dir.mkdir()
        if include_generic_host:
            generic_host = packages_root / "shared" / "controlhost" / "main.go"
            generic_host.parent.mkdir(parents=True)
            generic_host.write_text("package main\n", encoding="utf-8")

        manifest = {
            "id": owner,
            "targets": ["control"],
            "control_entrypoint": {"path": "bin/control-host"},
            "compatibility_routes": {"path": "compat/v2-routes.json"},
            "migrations": {"index": "migrations/index.json"},
        }
        (package_root / "manifest.template.json").write_text(json.dumps(manifest), encoding="utf-8")
        (migration_dir / "index.json").write_text(
            json.dumps({"format": "anixops.migrations/v1", "migrations": []}), encoding="utf-8"
        )
        if declaration_routes is None:
            declaration_routes = [
                {
                    "method": method,
                    "legacy_path": path,
                    "package_route": route_id,
                    "envelope": envelope,
                    "transport": transport,
                }
            ]
        declaration = {
            "api_version": "v2",
            "package_id": declaration_owner or owner,
            "routes": declaration_routes,
        }
        (compat_dir / "v2-routes.json").write_text(json.dumps(declaration), encoding="utf-8")

        catalog = root / "catalog.json"
        catalog.write_text(
            json.dumps(
                [
                    {
                        "method": method,
                        "path": path,
                        "owner": owner,
                        "route_id": route_id,
                        "envelope": envelope,
                        "middleware_group": middleware_group,
                        "transport": transport,
                    }
                ]
            ),
            encoding="utf-8",
        )
        row = {"method": method, "path": path, "package_id": owner, "route_id": route_id, "mode": mode, "legacy": legacy}
        if reason is not None:
            row["reason"] = reason
        (root / "extraction.json").write_text(
            json.dumps({"format": "anixops.package-extraction/v1", "routes": [row]}),
            encoding="utf-8",
        )
        if package_host_source is not None:
            (package_root / "control").mkdir()
            (package_root / "control" / "main.go").write_text(package_host_source, encoding="utf-8")
        (root / "identitybridge").mkdir()
        (root / "identitybridge" / "bridge.go").write_text(identity_bridge_source, encoding="utf-8")
        (root / "node-secret-fields.json").write_text(
            json.dumps({"format": "anixops.node-secret-fields/v1", "routes": []}), encoding="utf-8"
        )
        if handler is None:
            handler = (
                f'registeredPackageWebSocketRoute(v2WebSocketGateway.Serve, "{owner}", "{route_id}", legacyHandler, nil)'
                if transport == "websocket"
                else f'registeredPackageRoute(v2PackageGateway.Serve, "{owner}", "{route_id}", legacyHandler)'
            )
        if middleware_group == "admin":
            group_source = """
	admin := v2.Group("/admin")
	admin.Use(middleware.JWTAuth())
	admin.Use(middleware.AdminAuth())
	admin.GET("/ws/monitor", HANDLER)
"""
        else:
            group_source = """
	user := v2.Group("")
	user.Use(middleware.JWTAuth())
	user.GET("/user/knowledge", HANDLER)
"""
        router = root / "router.go"
        router.write_text(
            """package router

import "github.com/gin-gonic/gin"

func Setup(r *gin.Engine) {
	v2 := r.Group("/api/v2")
GROUP
}
""".replace("GROUP", group_source.replace("HANDLER", handler)),
            encoding="utf-8",
        )
        return catalog, router, packages_root

    def run_gate(
        self, catalog: Path, router: Path, packages_root: Path, *, expected_route_count: int = 1
    ) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            [
                sys.executable,
                str(CHECKER),
                "--catalog",
                str(catalog),
                "--router",
                str(router),
                "--packages-root",
                str(packages_root),
                "--expected-route-count",
                str(expected_route_count),
                "--extraction",
                str(catalog.parent / "extraction.json"),
                "--identity-bridge",
                str(catalog.parent / "identitybridge"),
                "--node-secret-fields",
                str(catalog.parent / "node-secret-fields.json"),
            ],
            cwd=REPO_ROOT,
            check=False,
            text=True,
            capture_output=True,
        )

    def test_plugin_only_route_gate_accepts_a_catalogued_package_gateway(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            catalog, router, packages_root = self.write_fixture(Path(temporary))
            result = self.run_gate(catalog, router, packages_root)

        self.assertEqual(0, result.returncode, result.stderr or result.stdout)
        self.assertIn("plugin-only v2 route gate passed", result.stdout)

    def test_plugin_only_route_gate_rejects_direct_catalogued_handler(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            catalog, router, packages_root = self.write_fixture(
                Path(temporary), handler="knowledgeHandler.GetArticles"
            )
            result = self.run_gate(catalog, router, packages_root)

        self.assertNotEqual(0, result.returncode)
        self.assertIn("direct legacy handler", result.stderr)

    def test_plugin_only_route_gate_rejects_a_mismatched_wrapper_binding(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            catalog, router, packages_root = self.write_fixture(
                Path(temporary),
                handler='registeredPackageRoute(v2PackageGateway.Serve, "ticket", "knowledge.article.list", legacyHandler)',
            )
            result = self.run_gate(catalog, router, packages_root)

        self.assertNotEqual(0, result.returncode)
        self.assertIn("package bridge binding mismatch", result.stderr)

    def test_plugin_only_route_gate_rejects_a_missing_package_declaration(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            catalog, router, packages_root = self.write_fixture(Path(temporary), declaration_routes=[])
            result = self.run_gate(catalog, router, packages_root)

        self.assertNotEqual(0, result.returncode)
        self.assertIn("missing route declaration", result.stderr)

    def test_plugin_only_route_gate_rejects_a_declaration_owner_mismatch(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            catalog, router, packages_root = self.write_fixture(Path(temporary), declaration_owner="ticket")
            result = self.run_gate(catalog, router, packages_root)

        self.assertNotEqual(0, result.returncode)
        self.assertIn("declaration owner mismatch", result.stderr)

    def test_plugin_only_route_gate_rejects_a_missing_control_host(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            catalog, router, packages_root = self.write_fixture(Path(temporary), include_generic_host=False)
            result = self.run_gate(catalog, router, packages_root)

        self.assertNotEqual(0, result.returncode)
        self.assertIn("missing host", result.stderr)

    def test_plugin_only_route_gate_rejects_a_legacy_websocket_binding(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            catalog, router, packages_root = self.write_fixture(
                Path(temporary),
                owner="machine-telemetry",
                path="/api/v2/admin/ws/monitor",
                route_id="telemetry.admin.ws.monitor.get",
                envelope="websocket",
                middleware_group="admin",
                transport="websocket",
                handler="monitorWSHandler.HandleMonitorWS",
            )
            result = self.run_gate(catalog, router, packages_root)

        self.assertNotEqual(0, result.returncode)
        self.assertIn("legacy WebSocket binding", result.stderr)

    def test_plugin_only_route_gate_rejects_a_catalog_below_the_required_v4_cardinality(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            catalog, router, packages_root = self.write_fixture(Path(temporary))
            result = self.run_gate(catalog, router, packages_root, expected_route_count=292)

        self.assertNotEqual(0, result.returncode)
        self.assertIn("expected 292 routes", result.stderr)

    def test_extraction_map_expresses_the_identity_bridge_exception(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            catalog, router, packages_root = self.write_fixture(
                Path(temporary), handler="v2PackageGateway.Serve", legacy="identity-bridge",
                identity_bridge_source='package identitybridge\nvar ids = []string{"knowledge.article.list"}\n',
            )
            accepted = self.run_gate(catalog, router, packages_root)
        with tempfile.TemporaryDirectory() as temporary:
            catalog, router, packages_root = self.write_fixture(
                Path(temporary), handler="v2PackageGateway.Serve", legacy="identity-bridge"
            )
            missing = self.run_gate(catalog, router, packages_root)
        with tempfile.TemporaryDirectory() as temporary:
            catalog, router, packages_root = self.write_fixture(Path(temporary), handler="v2PackageGateway.Serve")
            undeclared = self.run_gate(catalog, router, packages_root)

        self.assertEqual(0, accepted.returncode, accepted.stderr)
        self.assertIn("identity bridge has no legacy handler", missing.stderr)
        self.assertIn("package bridge binding mismatch", undeclared.stderr)

    def test_native_route_must_drop_its_legacy_handler(self) -> None:
        host = 'package main\nconst route = "knowledge.article.list"\n'
        with tempfile.TemporaryDirectory() as temporary:
            catalog, router, packages_root = self.write_fixture(
                Path(temporary), handler="v2PackageGateway.Serve", mode="native", legacy="none", package_host_source=host
            )
            accepted = self.run_gate(catalog, router, packages_root)
        with tempfile.TemporaryDirectory() as temporary:
            catalog, router, packages_root = self.write_fixture(
                Path(temporary), mode="native", legacy="none", package_host_source=host
            )
            kept_legacy = self.run_gate(catalog, router, packages_root)
        with tempfile.TemporaryDirectory() as temporary:
            catalog, router, packages_root = self.write_fixture(
                Path(temporary), handler="v2PackageGateway.Serve", mode="native", legacy="none", package_host_source=host,
                identity_bridge_source='package identitybridge\nvar ids = []string{"knowledge.article.list"}\n',
            )
            kept_bridge = self.run_gate(catalog, router, packages_root)

        self.assertEqual(0, accepted.returncode, accepted.stderr)
        self.assertIn("native route GET /api/v2/user/knowledge registers legacy handler legacyHandler", kept_legacy.stderr)
        self.assertIn("still has an identity bridge handler", kept_bridge.stderr)

    def test_native_modes_need_a_package_host_that_implements_the_route(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            catalog, router, packages_root = self.write_fixture(Path(temporary), mode="native-flagged")
            generic_only = self.run_gate(catalog, router, packages_root)
        with tempfile.TemporaryDirectory() as temporary:
            catalog, router, packages_root = self.write_fixture(
                Path(temporary), mode="native-flagged", package_host_source='package main\nconst route = "knowledge.article.list"\n'
            )
            flagged = self.run_gate(catalog, router, packages_root)

        self.assertIn("has no native implementation", generic_only.stderr)
        self.assertEqual(0, flagged.returncode, flagged.stderr)
        self.assertIn("1 native-flagged", flagged.stdout)

    def test_native_route_must_not_be_relayed(self) -> None:
        relayed_only = 'package main\nvar bridgedRoutes = map[string]struct{}{\n\t"knowledge.article.list": {},\n}\n'
        relayed_too = 'package main\nconst route = "knowledge.article.list"\n' + relayed_only
        with tempfile.TemporaryDirectory() as temporary:
            catalog, router, packages_root = self.write_fixture(
                Path(temporary), mode="native-flagged", package_host_source=relayed_only
            )
            only_relayed = self.run_gate(catalog, router, packages_root)
        with tempfile.TemporaryDirectory() as temporary:
            catalog, router, packages_root = self.write_fixture(
                Path(temporary), mode="native-flagged", package_host_source=relayed_too
            )
            also_relayed = self.run_gate(catalog, router, packages_root)

        self.assertIn("has no native implementation", only_relayed.stderr)
        self.assertIn("lists knowledge.article.list in bridgedRoutes", also_relayed.stderr)

    def test_kernel_owned_route_is_relayed_like_a_bridged_one(self) -> None:
        reason = "the kernel binary's own build metadata"
        relayed = 'package main\nvar bridgedRoutes = map[string]struct{}{\n\t"knowledge.article.list": {},\n}\n'
        outcomes: dict[str, subprocess.CompletedProcess[str]] = {}
        cases = {
            "generic host": {},
            "own host": {"package_host_source": relayed},
            "identity bridge": {
                "handler": "v2PackageGateway.Serve",
                "legacy": "identity-bridge",
                "identity_bridge_source": 'package identitybridge\nvar ids = []string{"knowledge.article.list"}\n',
            },
            "direct handler": {"handler": "knowledgeHandler.GetArticles"},
            "bare gateway": {"handler": "v2PackageGateway.Serve"},
            "not relayed": {"package_host_source": "package main\nvar bridgedRoutes = map[string]struct{}{}\n"},
            "no relay map": {"package_host_source": "package main\n"},
            "native host handler": {
                "package_host_source": relayed + 'var nativeRoutes = map[string]struct{}{"knowledge.article.list": {}}\n'
            },
        }
        for name, overrides in cases.items():
            with tempfile.TemporaryDirectory() as temporary:
                catalog, router, packages_root = self.write_fixture(
                    Path(temporary), mode="kernel-owned", reason=reason, **overrides
                )
                outcomes[name] = self.run_gate(catalog, router, packages_root)
        with tempfile.TemporaryDirectory() as temporary:
            catalog, router, packages_root = self.write_fixture(
                Path(temporary), mode="kernel-owned", reason=reason, package_host_source=relayed
            )
            native_dir = packages_root / "knowledge" / "native"
            native_dir.mkdir()
            (native_dir / "handlers.go").write_text(
                'package native\nconst RouteID = "knowledge.article.list"\n', encoding="utf-8"
            )
            outcomes["native package handler"] = self.run_gate(catalog, router, packages_root)

        for name in ("generic host", "own host", "identity bridge"):
            self.assertEqual(0, outcomes[name].returncode, f"{name}: {outcomes[name].stderr}")
        self.assertIn("1 kernel-owned", outcomes["own host"].stdout)
        self.assertIn("direct legacy handler", outcomes["direct handler"].stderr)
        self.assertIn("package bridge binding mismatch", outcomes["bare gateway"].stderr)
        self.assertIn("does not list knowledge.article.list in bridgedRoutes", outcomes["not relayed"].stderr)
        self.assertIn("does not list knowledge.article.list in bridgedRoutes", outcomes["no relay map"].stderr)
        self.assertIn("has a native handler", outcomes["native host handler"].stderr)
        self.assertIn("has a native handler", outcomes["native package handler"].stderr)

    def test_bridged_route_must_be_relayed_by_its_own_host(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            catalog, router, packages_root = self.write_fixture(
                Path(temporary), package_host_source='package main\nconst route = "knowledge.article.list"\n'
            )
            result = self.run_gate(catalog, router, packages_root)

        self.assertIn("bridged route GET /api/v2/user/knowledge is not relayed", result.stderr)

    def test_only_kernel_owned_rows_carry_a_reason(self) -> None:
        cases = {
            "kernel-owned without reason": ({"mode": "kernel-owned"}, "invalid schema: missing reason"),
            "kernel-owned with empty reason": (
                {"mode": "kernel-owned", "reason": ""},
                "field reason must be a non-empty string",
            ),
            "kernel-owned with long reason": (
                {"mode": "kernel-owned", "reason": "x" * 161},
                "reason must be one trimmed line of at most 160 characters",
            ),
            "kernel-owned with multi-line reason": (
                {"mode": "kernel-owned", "reason": "two\nlines"},
                "reason must be one trimmed line",
            ),
            "kernel-owned without legacy": (
                {"mode": "kernel-owned", "legacy": "none", "reason": "kernel disk"},
                "does not match legacy source",
            ),
            "bridged with reason": ({"mode": "bridged", "reason": "later"}, "invalid schema: unexpected reason"),
        }
        for name, (row, message) in cases.items():
            with self.subTest(name), tempfile.TemporaryDirectory() as temporary:
                catalog, router, packages_root = self.write_fixture(Path(temporary), **row)
                result = self.run_gate(catalog, router, packages_root)
                self.assertNotEqual(0, result.returncode)
                self.assertIn(message, result.stderr)

    def test_extraction_map_rejects_inconsistent_rows(self) -> None:
        cases = {
            "native with legacy": ({"mode": "native", "legacy": "router"}, "does not match legacy source"),
            "bridged without legacy": ({"mode": "bridged", "legacy": "none"}, "does not match legacy source"),
            "unknown mode": ({"mode": "shadow", "legacy": "router"}, "unsupported mode"),
        }
        for name, (row, message) in cases.items():
            with self.subTest(name), tempfile.TemporaryDirectory() as temporary:
                catalog, router, packages_root = self.write_fixture(Path(temporary), **row)
                result = self.run_gate(catalog, router, packages_root)
                self.assertIn(message, result.stderr)
        with tempfile.TemporaryDirectory() as temporary:
            catalog, router, packages_root = self.write_fixture(Path(temporary))
            extraction = catalog.parent / "extraction.json"
            document = json.loads(extraction.read_text(encoding="utf-8"))
            document["routes"][0]["route_id"] = "knowledge.other"
            extraction.write_text(json.dumps(document), encoding="utf-8")
            mismatch = self.run_gate(catalog, router, packages_root)
            document["routes"] = []
            extraction.write_text(json.dumps(document), encoding="utf-8")
            missing = self.run_gate(catalog, router, packages_root)
        self.assertIn("package extraction mismatch", mismatch.stderr)
        self.assertIn("missing package extraction row", missing.stderr)

    def test_node_secret_fields_name_routes_of_the_extraction_map(self) -> None:
        listed = {
            "route_id": "knowledge.article.list",
            "target": {"kind": "proxy", "new": True},
            "request": [{"pointer": "/raw_config", "kind": "document"}],
            "answer": [{"pointer": "/data/api_key", "name": "api_key"}],
        }
        cases = {
            "accepted": (listed, None),
            "unknown route": ({**listed, "route_id": "knowledge.other"}, "is not a route of the package extraction map"),
            "unknown target": ({**listed, "target": {"kind": "user", "new": True}}, "target kind 'user' is unknown"),
            "both targets": ({**listed, "target": {"kind": "proxy", "new": True, "path_param": "id"}}, "not both"),
            "missing parameter": ({**listed, "target": {"kind": "proxy", "path_param": "id"}}, "is not in /api/v2/user/knowledge"),
            "no fields": ({"route_id": "knowledge.article.list", "target": {"kind": "none"}}, "lists request or answer fields"),
            "bad pointer": ({**listed, "request": [{"pointer": "raw_config", "kind": "document"}]}, "must be a JSON pointer"),
            "spelled twice": (
                {**listed, "request": [{"pointer": "/raw_config", "kind": "document"}, {"pointer": "/RawConfig", "kind": "value"}]},
                "is listed twice",
            ),
            "field kind": ({**listed, "request": [{"pointer": "/raw_config", "kind": "blob"}]}, "kind 'blob' is unknown"),
            "answer name": ({**listed, "answer": [{"pointer": "/data/a", "name": ""}]}, "needs a name of its own"),
            "unknown member": ({**listed, "extra": 1}, "must have route_id, target"),
        }
        for name, (row, message) in cases.items():
            with self.subTest(name), tempfile.TemporaryDirectory() as temporary:
                catalog, router, packages_root = self.write_fixture(Path(temporary))
                (catalog.parent / "node-secret-fields.json").write_text(
                    json.dumps({"format": "anixops.node-secret-fields/v1", "routes": [row]}), encoding="utf-8"
                )
                result = self.run_gate(catalog, router, packages_root)
                if message is None:
                    self.assertEqual(0, result.returncode, result.stderr)
                    self.assertIn("1 with node secret fields", result.stdout)
                else:
                    self.assertNotEqual(0, result.returncode)
                    self.assertIn(message, result.stderr)
        with tempfile.TemporaryDirectory() as temporary:
            catalog, router, packages_root = self.write_fixture(Path(temporary))
            (catalog.parent / "node-secret-fields.json").write_text(
                json.dumps({"format": "anixops.node-secret-fields/v1", "routes": [listed, listed]}), encoding="utf-8"
            )
            twice = self.run_gate(catalog, router, packages_root)
            (catalog.parent / "node-secret-fields.json").write_text(json.dumps({"format": "other", "routes": []}), encoding="utf-8")
            wrong_format = self.run_gate(catalog, router, packages_root)
        self.assertIn("is listed twice", twice.stderr)
        self.assertIn("format must be anixops.node-secret-fields/v1", wrong_format.stderr)


if __name__ == "__main__":
    unittest.main()
