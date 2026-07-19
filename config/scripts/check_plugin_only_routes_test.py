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


if __name__ == "__main__":
    unittest.main()
