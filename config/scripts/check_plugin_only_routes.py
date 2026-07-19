#!/usr/bin/env python3
from __future__ import annotations

import argparse
import json
import sys
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Iterable

from check_v2_package_route_catalog import (
    ALLOWED_ENVELOPES,
    ALLOWED_METHODS,
    ALLOWED_TRANSPORTS,
    CatalogError,
    CatalogRoute,
    InventoryRoute,
    inventory_from_go,
    load_catalog,
    validate_catalog_against_inventory,
)


REPO_ROOT = Path(__file__).resolve().parents[2]
COMPATIBILITY_ROOT_FIELDS = frozenset({"api_version", "package_id", "routes"})
COMPATIBILITY_ROUTE_FIELDS = frozenset({"method", "legacy_path", "package_route", "envelope", "transport"})
HTTP_GATEWAY_HANDLER = "v2PackageGateway.Serve"
WEBSOCKET_GATEWAY_HANDLER = "v2WebSocketGateway.Serve"
EXPECTED_V4_ROUTE_COUNT = 292


class PluginOnlyRouteError(CatalogError):
    pass


@dataclass(frozen=True)
class PackageRouteDeclaration:
    owner: str
    method: str
    path: str
    route_id: str
    envelope: str
    transport: str

    @property
    def key(self) -> tuple[str, str]:
        return self.method, self.path


def require_exact_fields(value: dict[str, Any], expected: frozenset[str], context: str) -> None:
    actual = frozenset(value)
    missing = sorted(expected - actual)
    extra = sorted(actual - expected)
    if not missing and not extra:
        return
    details: list[str] = []
    if missing:
        details.append("missing " + ", ".join(missing))
    if extra:
        details.append("unexpected " + ", ".join(extra))
    raise PluginOnlyRouteError(f"{context} has invalid schema: {'; '.join(details)}")


def require_string(value: dict[str, Any], field: str, context: str) -> str:
    candidate = value.get(field)
    if not isinstance(candidate, str) or not candidate:
        raise PluginOnlyRouteError(f"{context} field {field} must be a non-empty string")
    return candidate


def load_json_object(path: Path, label: str) -> dict[str, Any]:
    try:
        value = json.loads(path.read_text(encoding="utf-8"))
    except OSError as error:
        raise PluginOnlyRouteError(f"cannot read {label} {path}: {error}") from error
    except json.JSONDecodeError as error:
        raise PluginOnlyRouteError(f"invalid JSON in {label} {path}: {error}") from error
    if not isinstance(value, dict):
        raise PluginOnlyRouteError(f"{label} {path} must be a JSON object")
    return value


def validate_package_host(packages_root: Path, owner: str, manifest: dict[str, Any]) -> None:
    package_root = packages_root / owner
    manifest_id = manifest.get("id")
    if manifest_id != owner:
        raise PluginOnlyRouteError(f"declaration owner mismatch: manifest for {owner!r} declares {manifest_id!r}")
    targets = manifest.get("targets")
    if not isinstance(targets, list) or "control" not in targets:
        raise PluginOnlyRouteError(f"missing host target control for package {owner!r}")
    entrypoint = manifest.get("control_entrypoint")
    if not isinstance(entrypoint, dict) or entrypoint.get("path") != "bin/control-host":
        raise PluginOnlyRouteError(f"missing host entrypoint for package {owner!r}")
    compatibility_routes = manifest.get("compatibility_routes")
    if not isinstance(compatibility_routes, dict) or compatibility_routes.get("path") != "compat/v2-routes.json":
        raise PluginOnlyRouteError(f"missing route declaration manifest entry for package {owner!r}")
    migrations = manifest.get("migrations")
    if not isinstance(migrations, dict) or migrations.get("index") != "migrations/index.json":
        raise PluginOnlyRouteError(f"missing migration index for package {owner!r}")
    if not (package_root / "migrations" / "index.json").is_file():
        raise PluginOnlyRouteError(f"missing migration index file for package {owner!r}")
    package_host = package_root / "control" / "main.go"
    generic_host = packages_root / "shared" / "controlhost" / "main.go"
    if not package_host.is_file() and not generic_host.is_file():
        raise PluginOnlyRouteError(f"missing host for package {owner!r}")


def load_package_declarations(packages_root: Path, owners: Iterable[str]) -> tuple[PackageRouteDeclaration, ...]:
    declarations: list[PackageRouteDeclaration] = []
    seen: set[tuple[str, str]] = set()
    for owner in sorted(set(owners)):
        package_root = packages_root / owner
        manifest_path = package_root / "manifest.template.json"
        if not manifest_path.is_file():
            raise PluginOnlyRouteError(f"missing host manifest for package {owner!r}")
        manifest = load_json_object(manifest_path, "package manifest")
        validate_package_host(packages_root, owner, manifest)

        routes_path = package_root / "compat" / "v2-routes.json"
        routes_document = load_json_object(routes_path, "package route declaration")
        require_exact_fields(routes_document, COMPATIBILITY_ROOT_FIELDS, f"package route declaration {owner!r}")
        if routes_document["api_version"] != "v2":
            raise PluginOnlyRouteError(f"package route declaration {owner!r} must use api_version v2")
        declared_owner = require_string(routes_document, "package_id", f"package route declaration {owner!r}")
        if declared_owner != owner:
            raise PluginOnlyRouteError(
                f"declaration owner mismatch: package directory {owner!r} declares {declared_owner!r}"
            )
        routes = routes_document["routes"]
        if not isinstance(routes, list):
            raise PluginOnlyRouteError(f"package route declaration {owner!r} field routes must be an array")
        for index, route_value in enumerate(routes):
            context = f"package route declaration {owner!r} row {index + 1}"
            if not isinstance(route_value, dict):
                raise PluginOnlyRouteError(f"{context} must be an object")
            require_exact_fields(route_value, COMPATIBILITY_ROUTE_FIELDS, context)
            method = require_string(route_value, "method", context)
            path = require_string(route_value, "legacy_path", context)
            route_id = require_string(route_value, "package_route", context)
            envelope = require_string(route_value, "envelope", context)
            transport = require_string(route_value, "transport", context)
            if method not in ALLOWED_METHODS:
                raise PluginOnlyRouteError(f"{context} has unsupported method {method!r}")
            if not path.startswith("/api/v2/"):
                raise PluginOnlyRouteError(f"{context} path must be a concrete /api/v2 route: {path!r}")
            if envelope not in ALLOWED_ENVELOPES:
                raise PluginOnlyRouteError(f"{context} has unsupported envelope {envelope!r}")
            if transport not in ALLOWED_TRANSPORTS:
                raise PluginOnlyRouteError(f"{context} has unsupported transport {transport!r}")
            if transport == "websocket" and envelope != "websocket":
                raise PluginOnlyRouteError(f"{context} websocket transport must use websocket envelope")
            if transport == "http" and envelope == "websocket":
                raise PluginOnlyRouteError(f"{context} HTTP transport cannot use websocket envelope")
            declaration = PackageRouteDeclaration(owner, method, path, route_id, envelope, transport)
            if declaration.key in seen:
                raise PluginOnlyRouteError(
                    f"duplicate package route declaration: {declaration.method} {declaration.path}"
                )
            seen.add(declaration.key)
            declarations.append(declaration)
    return tuple(declarations)


def validate_gateway_handlers(catalog: Iterable[CatalogRoute], inventory: Iterable[InventoryRoute]) -> None:
    inventory_by_key = {route.key: route for route in inventory}
    for route in catalog:
        registered = inventory_by_key[route.key]
        expected = WEBSOCKET_GATEWAY_HANDLER if route.transport == "websocket" else HTTP_GATEWAY_HANDLER
        if registered.handler != expected:
            if route.transport == "websocket":
                raise PluginOnlyRouteError(
                    f"legacy WebSocket binding for {route.method} {route.path}: "
                    f"expected {expected}, found {registered.handler}"
                )
            raise PluginOnlyRouteError(
                f"direct legacy handler for {route.method} {route.path}: expected {expected}, found {registered.handler}"
            )
        if route.transport == "websocket":
            expected_binding = "package-websocket"
        else:
            expected_binding = "package-http"
        if registered.binding == "direct":
            if route.transport == "http" and route.owner == "identity-platform":
                continue
            raise PluginOnlyRouteError(
                f"package bridge binding mismatch for {route.method} {route.path}: "
                f"catalog requires {expected_binding}, found direct"
            )
        if registered.binding != expected_binding:
            raise PluginOnlyRouteError(
                f"package bridge binding mismatch for {route.method} {route.path}: "
                f"catalog requires {expected_binding}, found {registered.binding or 'missing'}"
            )
        if registered.package_id != route.owner or registered.route_id != route.route_id:
            raise PluginOnlyRouteError(
                f"package bridge binding mismatch for {route.method} {route.path}: "
                f"catalog=({route.owner!r}, {route.route_id!r}) "
                f"router=({registered.package_id!r}, {registered.route_id!r})"
            )


def validate_declarations(catalog: Iterable[CatalogRoute], declarations: Iterable[PackageRouteDeclaration]) -> None:
    catalog_by_key = {route.key: route for route in catalog}
    declarations_by_key = {route.key: route for route in declarations}
    for route in catalog:
        declaration = declarations_by_key.get(route.key)
        if declaration is None:
            raise PluginOnlyRouteError(f"missing route declaration for {route.method} {route.path}")
        if declaration.owner != route.owner:
            raise PluginOnlyRouteError(
                f"declaration owner mismatch for {route.method} {route.path}: "
                f"catalog={route.owner!r} declaration={declaration.owner!r}"
            )
        if (
            declaration.route_id != route.route_id
            or declaration.envelope != route.envelope
            or declaration.transport != route.transport
        ):
            raise PluginOnlyRouteError(
                f"route declaration mismatch for {route.method} {route.path}: "
                f"catalog=({route.route_id!r}, {route.envelope!r}, {route.transport!r}) "
                f"declaration=({declaration.route_id!r}, {declaration.envelope!r}, {declaration.transport!r})"
            )
    for declaration in declarations:
        catalog_route = catalog_by_key.get(declaration.key)
        if catalog_route is None:
            raise PluginOnlyRouteError(
                f"package route declaration without catalog row: {declaration.method} {declaration.path}"
            )
        if catalog_route.owner != declaration.owner:
            raise PluginOnlyRouteError(
                f"declaration owner mismatch for {declaration.method} {declaration.path}: "
                f"catalog={catalog_route.owner!r} declaration={declaration.owner!r}"
            )


def parse_arguments() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="enforce signed package-only ownership for every /api/v2 route")
    parser.add_argument("--catalog", type=Path, default=REPO_ROOT / "config" / "v2-package-route-catalog.json")
    parser.add_argument("--router", type=Path, default=REPO_ROOT / "internal" / "router" / "router.go")
    parser.add_argument("--packages-root", type=Path, default=REPO_ROOT / "packages")
    parser.add_argument("--expected-route-count", type=int, default=EXPECTED_V4_ROUTE_COUNT)
    return parser.parse_args()


def main() -> int:
    arguments = parse_arguments()
    try:
        catalog = load_catalog(arguments.catalog)
        if arguments.expected_route_count < 1:
            raise PluginOnlyRouteError("expected route count must be positive")
        if len(catalog) != arguments.expected_route_count:
            raise PluginOnlyRouteError(
                f"expected {arguments.expected_route_count} routes, found {len(catalog)}"
            )
        inventory = inventory_from_go(arguments.router)
        validate_catalog_against_inventory(catalog, inventory)
        validate_gateway_handlers(catalog, inventory)
        declarations = load_package_declarations(arguments.packages_root, (route.owner for route in catalog))
        validate_declarations(catalog, declarations)
    except CatalogError as error:
        print(f"plugin-only v2 route gate: {error}", file=sys.stderr)
        return 1

    print(f"plugin-only v2 route gate passed ({len(catalog)} routes)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
