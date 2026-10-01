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
EXTRACTION_FORMAT = "anixops.package-extraction/v1"
EXTRACTION_ROOT_FIELDS = frozenset({"format", "routes"})
EXTRACTION_ROUTE_FIELDS = frozenset({"method", "path", "package_id", "route_id", "mode", "legacy"})
# bridged: only the legacy implementation exists, until a kernel contract lets
# the package serve the route. kernel-owned: only the legacy implementation
# exists, by design: the route stays in the kernel, and its row carries a
# reason. native-flagged: the host has a native implementation and the legacy
# one stays for runtime route modes. native: the legacy handler is deleted and
# the host answers alone.
EXTRACTION_MODES = frozenset({"bridged", "kernel-owned", "native-flagged", "native"})
# Modes whose route the package host only relays to the kernel's legacy
# handler: it is listed in the host's bridgedRoutes and has no native handler.
RELAY_ONLY_MODES = frozenset({"bridged", "kernel-owned"})
EXTRACTION_REASON_FIELD = "reason"
EXTRACTION_REASON_MAX_LENGTH = 160
# The map literal through which a package host relays its relay-only routes.
BRIDGED_ROUTES_DECLARATION = "var bridgedRoutes = map[string]struct{}{"
# router: the legacy gin handler is registered through registeredPackageRoute.
# identity-bridge: it lives in the kernel's identity bridge allowlist and the
# router binds the bare gateway. none: there is no legacy handler.
EXTRACTION_LEGACY_SOURCES = frozenset({"router", "identity-bridge", "none"})


# config/node-secret-fields.json: the kernel-owned list of the routes and
# fields whose node secrets the v2 gateway seals (node-ops-service.md section
# 3.7). The kernel embeds and parses it (internal/sealedsecrets); this gate
# ties it to the extraction map.
NODE_SECRET_FIELDS_FORMAT = "anixops.node-secret-fields/v1"
NODE_SECRET_FIELDS_ROOT_FIELDS = frozenset({"format", "routes"})
NODE_SECRET_ROUTE_FIELDS = frozenset({"route_id", "target", "request", "answer"})
NODE_SECRET_TARGET_KINDS = frozenset({"proxy", "forward", "protocol", "clean_agent", "registration_key", "none"})
NODE_SECRET_REQUEST_KINDS = frozenset({"value", "document"})


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


@dataclass(frozen=True)
class ExtractionRoute:
    method: str
    path: str
    package_id: str
    route_id: str
    mode: str
    legacy: str
    reason: str | None = None

    @property
    def key(self) -> tuple[str, str]:
        return self.method, self.path


def load_extraction(path: Path) -> dict[tuple[str, str], ExtractionRoute]:
    document = load_json_object(path, "package extraction map")
    require_exact_fields(document, EXTRACTION_ROOT_FIELDS, "package extraction map")
    if document["format"] != EXTRACTION_FORMAT:
        raise PluginOnlyRouteError(f"package extraction map format must be {EXTRACTION_FORMAT}")
    rows = document["routes"]
    if not isinstance(rows, list):
        raise PluginOnlyRouteError("package extraction map field routes must be an array")
    routes: dict[tuple[str, str], ExtractionRoute] = {}
    for index, row in enumerate(rows):
        context = f"package extraction row {index + 1}"
        if not isinstance(row, dict):
            raise PluginOnlyRouteError(f"{context} must be an object")
        # Only a kernel-owned row says why it stays in the kernel.
        kernel_owned = row.get("mode") == "kernel-owned"
        expected_fields = EXTRACTION_ROUTE_FIELDS | {EXTRACTION_REASON_FIELD} if kernel_owned else EXTRACTION_ROUTE_FIELDS
        require_exact_fields(row, expected_fields, context)
        route = ExtractionRoute(
            method=require_string(row, "method", context),
            path=require_string(row, "path", context),
            package_id=require_string(row, "package_id", context),
            route_id=require_string(row, "route_id", context),
            mode=require_string(row, "mode", context),
            legacy=require_string(row, "legacy", context),
            reason=require_string(row, EXTRACTION_REASON_FIELD, context) if kernel_owned else None,
        )
        if route.reason is not None and (
            route.reason != route.reason.strip()
            or "\n" in route.reason
            or len(route.reason) > EXTRACTION_REASON_MAX_LENGTH
        ):
            raise PluginOnlyRouteError(
                f"{context} reason must be one trimmed line of at most {EXTRACTION_REASON_MAX_LENGTH} characters"
            )
        if route.mode not in EXTRACTION_MODES:
            raise PluginOnlyRouteError(f"{context} has unsupported mode {route.mode!r}")
        if route.legacy not in EXTRACTION_LEGACY_SOURCES:
            raise PluginOnlyRouteError(f"{context} has unsupported legacy source {route.legacy!r}")
        if (route.mode == "native") != (route.legacy == "none"):
            raise PluginOnlyRouteError(
                f"{context} mode {route.mode!r} does not match legacy source {route.legacy!r}: "
                "only native routes have no legacy handler"
            )
        if route.key in routes:
            raise PluginOnlyRouteError(f"duplicate package extraction row: {route.method} {route.path}")
        routes[route.key] = route
    return routes


def validate_extraction(
    catalog: Iterable[CatalogRoute], extraction: dict[tuple[str, str], ExtractionRoute], packages_root: Path
) -> None:
    catalog_keys = set()
    for route in catalog:
        catalog_keys.add(route.key)
        entry = extraction.get(route.key)
        if entry is None:
            raise PluginOnlyRouteError(f"missing package extraction row for {route.method} {route.path}")
        if entry.package_id != route.owner or entry.route_id != route.route_id:
            raise PluginOnlyRouteError(
                f"package extraction mismatch for {route.method} {route.path}: "
                f"catalog=({route.owner!r}, {route.route_id!r}) extraction=({entry.package_id!r}, {entry.route_id!r})"
            )
    for key in sorted(set(extraction) - catalog_keys):
        raise PluginOnlyRouteError(f"package extraction row without catalog row: {key[0]} {key[1]}")
    hosts: dict[str, PackageHostRoutes] = {}
    for entry in extraction.values():
        if entry.package_id not in hosts:
            hosts[entry.package_id] = load_package_host_routes(packages_root, entry.package_id)
        host = hosts[entry.package_id]
        quoted = f'"{entry.route_id}"'
        if entry.mode in RELAY_ONLY_MODES:
            # The generic host only relays; a package's own host relays the
            # routes in its bridgedRoutes, and nothing else in the package
            # may name them, so none has a native handler.
            if not host.has_host:
                continue
            if quoted not in host.relayed:
                raise PluginOnlyRouteError(
                    f"{entry.mode} route {entry.method} {entry.path} is not relayed: "
                    f"packages/{entry.package_id}/control does not list {entry.route_id} in bridgedRoutes"
                )
            if quoted in host.package_sources:
                raise PluginOnlyRouteError(
                    f"{entry.mode} route {entry.method} {entry.path} has a native handler: "
                    f"packages/{entry.package_id} names {entry.route_id} outside bridgedRoutes"
                )
            continue
        # A native implementation lives in the package's own host; the
        # generic host only relays to legacy handlers.
        if quoted not in host.host_sources:
            raise PluginOnlyRouteError(
                f"{entry.mode} route {entry.method} {entry.path} has no native implementation: "
                f"packages/{entry.package_id}/control does not name {entry.route_id}"
            )
        if quoted in host.relayed:
            raise PluginOnlyRouteError(
                f"{entry.mode} route {entry.method} {entry.path} is relayed: "
                f"packages/{entry.package_id}/control lists {entry.route_id} in bridgedRoutes"
            )


@dataclass(frozen=True)
class PackageHostRoutes:
    """A package's Go sources, split around its host's bridgedRoutes."""

    has_host: bool
    # The body of the bridgedRoutes map literal ("" without one).
    relayed: str
    # The host's sources (packages/<id>/control) without that literal.
    host_sources: str
    # Every Go source of the package without that literal.
    package_sources: str


def go_sources(root: Path, *, recursive: bool) -> list[Path]:
    if not root.is_dir():
        return []
    sources = root.rglob("*.go") if recursive else root.glob("*.go")
    return sorted(source for source in sources if not source.name.endswith("_test.go"))


def load_package_host_routes(packages_root: Path, package_id: str) -> PackageHostRoutes:
    package_root = packages_root / package_id
    host_root = package_root / "control"
    host_files = go_sources(host_root, recursive=False)
    host_text = "\n".join(source.read_text(encoding="utf-8") for source in host_files)
    other_text = "\n".join(
        source.read_text(encoding="utf-8")
        for source in go_sources(package_root, recursive=True)
        if source.parent != host_root
    )
    relayed = ""
    start = host_text.find(BRIDGED_ROUTES_DECLARATION)
    if start >= 0:
        if host_text.find(BRIDGED_ROUTES_DECLARATION, start + 1) >= 0:
            raise PluginOnlyRouteError(f"packages/{package_id}/control declares bridgedRoutes more than once")
        body_start = start + len(BRIDGED_ROUTES_DECLARATION)
        depth, index = 1, body_start
        while depth and index < len(host_text):
            depth += {"{": 1, "}": -1}.get(host_text[index], 0)
            index += 1
        if depth:
            raise PluginOnlyRouteError(f"packages/{package_id}/control has an unterminated bridgedRoutes")
        relayed = host_text[body_start : index - 1]
        host_text = host_text[:start] + host_text[index:]
    return PackageHostRoutes(
        has_host=bool(host_files),
        relayed=relayed,
        host_sources=host_text,
        package_sources=host_text + "\n" + other_text,
    )


def identity_bridge_route_ids(source_root: Path) -> frozenset[str]:
    """Route ids quoted in the kernel's identity bridge sources."""
    identifiers: set[str] = set()
    if source_root.is_dir():
        for source in sorted(source_root.glob("*.go")):
            if source.name.endswith("_test.go"):
                continue
            text = source.read_text(encoding="utf-8")
            for piece in text.split('"')[1::2]:
                identifiers.add(piece)
    return frozenset(identifiers)


def validate_gateway_handlers(
    catalog: Iterable[CatalogRoute],
    inventory: Iterable[InventoryRoute],
    extraction: dict[tuple[str, str], ExtractionRoute],
    identity_bridge_ids: frozenset[str],
) -> None:
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
        entry = extraction[route.key]
        if registered.binding == "direct":
            if route.transport == "http" and entry.legacy == "identity-bridge":
                if route.route_id not in identity_bridge_ids:
                    raise PluginOnlyRouteError(
                        f"identity bridge has no legacy handler for {route.method} {route.path} ({route.route_id})"
                    )
                continue
            if route.transport == "http" and entry.legacy == "none":
                if route.route_id in identity_bridge_ids:
                    raise PluginOnlyRouteError(
                        f"native route {route.method} {route.path} still has an identity bridge handler"
                    )
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
        if entry.legacy != "router":
            raise PluginOnlyRouteError(
                f"{entry.mode} route {route.method} {route.path} registers legacy handler "
                f"{registered.legacy_handler or 'unknown'}; the extraction map says {entry.legacy!r}"
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


def _pointer_tokens(pointer: Any, context: str) -> list[str]:
    if not isinstance(pointer, str) or not pointer.startswith("/") or pointer == "/":
        raise PluginOnlyRouteError(f"{context} pointer must be a JSON pointer to a field")
    tokens = pointer[1:].split("/")
    for token in tokens:
        if token == "" or "~" in token.replace("~0", "").replace("~1", ""):
            raise PluginOnlyRouteError(f"{context} pointer {pointer!r} is not a valid JSON pointer")
    return tokens


def validate_node_secret_fields(path: Path, extraction: dict[tuple[str, str], ExtractionRoute]) -> int:
    """Check the node secret field list: its shape, and every route id
    against the extraction map. Returns the number of listed routes."""
    document = load_json_object(path, "node secret field list")
    require_exact_fields(document, NODE_SECRET_FIELDS_ROOT_FIELDS, "node secret field list")
    if document["format"] != NODE_SECRET_FIELDS_FORMAT:
        raise PluginOnlyRouteError(f"node secret field list format must be {NODE_SECRET_FIELDS_FORMAT}")
    rows = document["routes"]
    if not isinstance(rows, list):
        raise PluginOnlyRouteError("node secret field list field routes must be an array")
    by_route_id = {route.route_id: route for route in extraction.values()}
    seen: set[str] = set()
    for index, row in enumerate(rows):
        context = f"node secret field list row {index + 1}"
        if not isinstance(row, dict):
            raise PluginOnlyRouteError(f"{context} must be an object")
        unknown = set(row) - NODE_SECRET_ROUTE_FIELDS
        if unknown or "route_id" not in row or "target" not in row:
            raise PluginOnlyRouteError(f"{context} must have route_id, target and request or answer fields only")
        route_id = require_string(row, "route_id", context)
        context = f"node secret field list route {route_id!r}"
        if route_id in seen:
            raise PluginOnlyRouteError(f"{context} is listed twice")
        seen.add(route_id)
        route = by_route_id.get(route_id)
        if route is None:
            raise PluginOnlyRouteError(f"{context} is not a route of the package extraction map")
        target = row["target"]
        if not isinstance(target, dict) or set(target) - {"kind", "path_param", "new"}:
            raise PluginOnlyRouteError(f"{context} target must be an object of kind, path_param and new")
        kind = target.get("kind")
        if kind not in NODE_SECRET_TARGET_KINDS:
            raise PluginOnlyRouteError(f"{context} target kind {kind!r} is unknown")
        path_param = target.get("path_param", "")
        new = target.get("new", False)
        if not isinstance(path_param, str) or not isinstance(new, bool):
            raise PluginOnlyRouteError(f"{context} target path_param must be a string and new a boolean")
        if kind == "none":
            if path_param or new:
                raise PluginOnlyRouteError(f"{context} target of kind none has no path parameter")
        elif bool(path_param) == new:
            raise PluginOnlyRouteError(f"{context} target names a path parameter or is new, not both")
        if path_param and f":{path_param}" not in route.path.split("/"):
            raise PluginOnlyRouteError(f"{context} target path parameter {path_param!r} is not in {route.path}")
        request = row.get("request", [])
        answer = row.get("answer", [])
        if not isinstance(request, list) or not isinstance(answer, list) or not (request or answer):
            raise PluginOnlyRouteError(f"{context} lists request or answer fields")
        pointers: set[str] = set()
        for field in request:
            if not isinstance(field, dict) or set(field) != {"pointer", "kind"}:
                raise PluginOnlyRouteError(f"{context} request fields have a pointer and a kind")
            tokens = _pointer_tokens(field["pointer"], context)
            folded = "/".join(token.lower().replace("_", "").replace("-", "") for token in tokens)
            if folded in pointers:
                raise PluginOnlyRouteError(f"{context} request pointer {field['pointer']!r} is listed twice")
            pointers.add(folded)
            if field["kind"] not in NODE_SECRET_REQUEST_KINDS:
                raise PluginOnlyRouteError(f"{context} request field kind {field['kind']!r} is unknown")
        names: set[str] = set()
        for field in answer:
            if not isinstance(field, dict) or set(field) != {"pointer", "name"}:
                raise PluginOnlyRouteError(f"{context} answer fields have a pointer and a name")
            _pointer_tokens(field["pointer"], context)
            name = field["name"]
            if not isinstance(name, str) or not name.strip() or name in names:
                raise PluginOnlyRouteError(f"{context} answer field {field['pointer']!r} needs a name of its own")
            names.add(name)
    return len(seen)


def parse_arguments() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="enforce signed package-only ownership for every /api/v2 route")
    parser.add_argument("--catalog", type=Path, default=REPO_ROOT / "config" / "v2-package-route-catalog.json")
    parser.add_argument("--router", type=Path, default=REPO_ROOT / "internal" / "router" / "router.go")
    parser.add_argument("--packages-root", type=Path, default=REPO_ROOT / "packages")
    parser.add_argument("--expected-route-count", type=int, default=EXPECTED_V4_ROUTE_COUNT)
    parser.add_argument("--extraction", type=Path, default=REPO_ROOT / "config" / "package-extraction.json")
    parser.add_argument("--identity-bridge", type=Path, default=REPO_ROOT / "internal" / "identitybridge")
    parser.add_argument("--node-secret-fields", type=Path, default=REPO_ROOT / "config" / "node-secret-fields.json")
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
        extraction = load_extraction(arguments.extraction)
        validate_extraction(catalog, extraction, arguments.packages_root)
        sealed_routes = validate_node_secret_fields(arguments.node_secret_fields, extraction)
        inventory = inventory_from_go(arguments.router)
        validate_catalog_against_inventory(catalog, inventory)
        validate_gateway_handlers(catalog, inventory, extraction, identity_bridge_route_ids(arguments.identity_bridge))
        declarations = load_package_declarations(arguments.packages_root, (route.owner for route in catalog))
        validate_declarations(catalog, declarations)
    except CatalogError as error:
        print(f"plugin-only v2 route gate: {error}", file=sys.stderr)
        return 1

    modes = {mode: sum(1 for route in extraction.values() if route.mode == mode) for mode in sorted(EXTRACTION_MODES)}
    summary = ", ".join(f"{count} {mode}" for mode, count in modes.items())
    print(f"plugin-only v2 route gate passed ({len(catalog)} routes: {summary}; {sealed_routes} with node secret fields)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
