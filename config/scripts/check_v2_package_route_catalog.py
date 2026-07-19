from __future__ import annotations

import argparse
import json
import posixpath
import subprocess
import sys
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Iterable


REPO_ROOT = Path(__file__).resolve().parents[2]

ALLOWED_METHODS = frozenset({"GET", "POST", "PUT", "PATCH", "HEAD", "OPTIONS", "DELETE", "CONNECT", "TRACE"})
ALLOWED_OWNERS = frozenset(
    {
        "identity-platform",
        "machine-telemetry",
        "subscription",
        "knowledge",
        "ticket",
        "plan",
        "order",
        "payment",
        "notification",
        "proxy-node",
        "protocol-runtime",
        "wireguard",
        "forward",
        "nftables-forward",
        "gost-mesh",
        "nat-egress",
    }
)
OWNER_ROUTE_PREFIXES = {
    "identity-platform": ("identity.",),
    "machine-telemetry": ("telemetry.",),
    "subscription": ("subscription.",),
    "knowledge": ("knowledge.",),
    "ticket": ("ticket.",),
    "plan": ("plan.",),
    "order": ("order.",),
    "payment": ("payment.",),
    "notification": ("notification.",),
    "proxy-node": ("proxy.",),
    "protocol-runtime": ("protocol.",),
    "wireguard": ("wireguard.",),
    "forward": ("forward.",),
    "nftables-forward": ("nftables.",),
    "gost-mesh": ("gost.",),
    "nat-egress": ("nat.",),
}
ALLOWED_ENVELOPES = frozenset({"data", "empty", "panel", "raw", "websocket"})
ALLOWED_MIDDLEWARE_GROUPS = frozenset({"public", "user", "admin", "agent", "node-api", "internal"})
ALLOWED_TRANSPORTS = frozenset({"http", "websocket"})
CATALOG_FIELDS = frozenset({"method", "path", "owner", "route_id", "envelope", "middleware_group", "transport"})
INVENTORY_FIELDS = frozenset({"method", "path", "handler", "middleware_group", "transport"})
INVENTORY_OPTIONAL_FIELDS = frozenset({"binding", "package_id", "route_id"})


class CatalogError(ValueError):
    pass


@dataclass(frozen=True)
class CatalogRoute:
    method: str
    path: str
    owner: str
    route_id: str
    envelope: str
    middleware_group: str
    transport: str

    @property
    def key(self) -> tuple[str, str]:
        return self.method, self.path


@dataclass(frozen=True)
class InventoryRoute:
    method: str
    path: str
    handler: str
    middleware_group: str
    transport: str
    binding: str = ""
    package_id: str = ""
    route_id: str = ""

    @property
    def key(self) -> tuple[str, str]:
        return self.method, self.path


def load_catalog(path: Path) -> tuple[CatalogRoute, ...]:
    rows = load_json_array(path, "catalog")
    catalog: list[CatalogRoute] = []
    seen: set[tuple[str, str]] = set()
    for index, row in enumerate(rows):
        context = f"catalog row {index + 1}"
        validate_exact_fields(row, CATALOG_FIELDS, context)
        route = CatalogRoute(
            method=require_string(row, "method", context),
            path=require_string(row, "path", context),
            owner=require_string(row, "owner", context, allow_empty=True),
            route_id=require_string(row, "route_id", context),
            envelope=require_string(row, "envelope", context),
            middleware_group=require_string(row, "middleware_group", context),
            transport=require_string(row, "transport", context),
        )
        validate_catalog_route(route, context)
        if route.key in seen:
            raise CatalogError(f"duplicate method/path in catalog: {route.method} {route.path}")
        seen.add(route.key)
        catalog.append(route)
    return tuple(catalog)


def load_inventory(path: Path | None, raw_inventory: str | None = None) -> tuple[InventoryRoute, ...]:
    if raw_inventory is not None:
        rows = decode_json_array(raw_inventory, "inventory stdin")
    elif path is not None:
        rows = load_json_array(path, "inventory")
    else:
        raise CatalogError("inventory source is missing")

    inventory: list[InventoryRoute] = []
    seen: set[tuple[str, str]] = set()
    for index, row in enumerate(rows):
        context = f"inventory row {index + 1}"
        validate_inventory_fields(row, context)
        route = InventoryRoute(
            method=require_string(row, "method", context),
            path=require_string(row, "path", context),
            handler=require_string(row, "handler", context),
            middleware_group=require_string(row, "middleware_group", context),
            transport=require_string(row, "transport", context),
            binding=optional_string(row, "binding", context),
            package_id=optional_string(row, "package_id", context),
            route_id=optional_string(row, "route_id", context),
        )
        validate_inventory_route(route, context)
        if route.key in seen:
            raise CatalogError(f"duplicate method/path in inventory: {route.method} {route.path}")
        seen.add(route.key)
        inventory.append(route)
    return tuple(inventory)


def load_json_array(path: Path, label: str) -> list[dict[str, Any]]:
    try:
        content = path.read_text(encoding="utf-8")
    except OSError as error:
        raise CatalogError(f"cannot read {label} {path}: {error}") from error
    return decode_json_array(content, f"{label} {path}")


def decode_json_array(content: str, label: str) -> list[dict[str, Any]]:
    try:
        value = json.loads(content)
    except json.JSONDecodeError as error:
        raise CatalogError(f"invalid JSON in {label}: {error}") from error
    if not isinstance(value, list):
        raise CatalogError(f"{label} must be a JSON array")
    if not all(isinstance(row, dict) for row in value):
        raise CatalogError(f"{label} must contain JSON objects")
    return value


def validate_exact_fields(row: dict[str, Any], expected: frozenset[str], context: str) -> None:
    actual = frozenset(row)
    missing = sorted(expected - actual)
    extra = sorted(actual - expected)
    if missing or extra:
        details: list[str] = []
        if missing:
            details.append(f"missing {', '.join(missing)}")
        if extra:
            details.append(f"unexpected {', '.join(extra)}")
        raise CatalogError(f"{context} has invalid schema: {'; '.join(details)}")


def validate_inventory_fields(row: dict[str, Any], context: str) -> None:
    actual = frozenset(row)
    missing = sorted(INVENTORY_FIELDS - actual)
    extra = sorted(actual - INVENTORY_FIELDS - INVENTORY_OPTIONAL_FIELDS)
    if missing or extra:
        details: list[str] = []
        if missing:
            details.append(f"missing {', '.join(missing)}")
        if extra:
            details.append(f"unexpected {', '.join(extra)}")
        raise CatalogError(f"{context} has invalid schema: {'; '.join(details)}")


def require_string(row: dict[str, Any], field: str, context: str, *, allow_empty: bool = False) -> str:
    value = row[field]
    if not isinstance(value, str):
        raise CatalogError(f"{context} field {field} must be a string")
    if not allow_empty and not value:
        raise CatalogError(f"{context} field {field} must not be empty")
    return value


def optional_string(row: dict[str, Any], field: str, context: str) -> str:
    if field not in row:
        return ""
    value = row[field]
    if not isinstance(value, str):
        raise CatalogError(f"{context} field {field} must be a string")
    return value


def validate_catalog_route(route: CatalogRoute, context: str) -> None:
    validate_method_and_path(route.method, route.path, context)
    if not route.owner:
        raise CatalogError(f"unowned v2 route: {route.method} {route.path}")
    if route.owner not in ALLOWED_OWNERS:
        raise CatalogError(f"unsupported owner {route.owner!r} in {context}")
    allowed_prefixes = OWNER_ROUTE_PREFIXES[route.owner]
    if not route.route_id.startswith(allowed_prefixes):
        raise CatalogError(f"route id {route.route_id!r} does not match owner {route.owner!r} in {context}")
    if route.envelope not in ALLOWED_ENVELOPES:
        raise CatalogError(f"unsupported envelope {route.envelope!r} in {context}")
    if route.middleware_group not in ALLOWED_MIDDLEWARE_GROUPS:
        raise CatalogError(f"unsupported middleware group {route.middleware_group!r} in {context}")
    if route.transport not in ALLOWED_TRANSPORTS:
        raise CatalogError(f"unsupported transport {route.transport!r} in {context}")
    if route.transport == "websocket" and route.envelope != "websocket":
        raise CatalogError(f"websocket route must use websocket envelope in {context}")
    if route.transport == "http" and route.envelope == "websocket":
        raise CatalogError(f"HTTP route cannot use websocket envelope in {context}")


def validate_inventory_route(route: InventoryRoute, context: str) -> None:
    validate_method_and_path(route.method, route.path, context)
    if route.middleware_group not in ALLOWED_MIDDLEWARE_GROUPS:
        raise CatalogError(f"unsupported middleware group {route.middleware_group!r} in {context}")
    if route.transport not in ALLOWED_TRANSPORTS:
        raise CatalogError(f"unsupported transport {route.transport!r} in {context}")
    metadata = (route.binding, route.package_id, route.route_id)
    if not any(metadata):
        return
    if route.binding not in {"direct", "package-http", "package-websocket"}:
        raise CatalogError(f"unsupported route binding {route.binding!r} in {context}")
    if route.binding == "direct":
        if route.package_id or route.route_id:
            raise CatalogError(f"direct route binding must not declare package metadata in {context}")
        return
    if not route.package_id or not route.route_id:
        raise CatalogError(f"package route binding must declare package_id and route_id in {context}")


def validate_method_and_path(method: str, route_path: str, context: str) -> None:
    if method not in ALLOWED_METHODS:
        raise CatalogError(f"unsupported method {method!r} in {context}")
    if not route_path.startswith("/api/v2/"):
        raise CatalogError(f"{context} path must be a concrete /api/v2 route: {route_path!r}")
    if "*" in route_path:
        raise CatalogError(f"wildcard or prefix route mapping is forbidden in {context}: {route_path!r}")
    if "?" in route_path or "#" in route_path:
        raise CatalogError(f"{context} path cannot include a query or fragment: {route_path!r}")
    normalized = posixpath.normpath(route_path)
    if normalized != route_path or "//" in route_path:
        raise CatalogError(f"{context} path must be normalized: {route_path!r}")


def inventory_from_go(router: Path) -> tuple[InventoryRoute, ...]:
    command = [
        "go",
        "run",
        "./config/scripts/v2_route_inventory.go",
        "--router",
        str(router),
        "--format",
        "json",
    ]
    result = subprocess.run(command, cwd=REPO_ROOT, check=False, text=True, capture_output=True)
    if result.returncode != 0:
        detail = (result.stderr or result.stdout).strip()
        raise CatalogError(f"Go route inventory failed: {detail}")
    return load_inventory(None, raw_inventory=result.stdout)


def validate_catalog_against_inventory(catalog: Iterable[CatalogRoute], inventory: Iterable[InventoryRoute]) -> None:
    catalog_by_key = {route.key: route for route in catalog}
    inventory_by_key = {route.key: route for route in inventory}
    catalog_keys = set(catalog_by_key)
    inventory_keys = set(inventory_by_key)
    missing_catalog = sorted(inventory_keys - catalog_keys)
    extra_catalog = sorted(catalog_keys - inventory_keys)
    if missing_catalog or extra_catalog:
        details: list[str] = []
        if missing_catalog:
            details.append("missing catalog rows: " + format_keys(missing_catalog))
        if extra_catalog:
            details.append("catalog rows without router registration: " + format_keys(extra_catalog))
        raise CatalogError("inventory equality failed; " + "; ".join(details))

    mismatches: list[str] = []
    for key in sorted(inventory_keys):
        catalog_route = catalog_by_key[key]
        inventory_route = inventory_by_key[key]
        if catalog_route.middleware_group != inventory_route.middleware_group:
            mismatches.append(
                f"{catalog_route.method} {catalog_route.path} middleware group catalog={catalog_route.middleware_group!r} "
                f"inventory={inventory_route.middleware_group!r}"
            )
        if catalog_route.transport != inventory_route.transport:
            mismatches.append(
                f"{catalog_route.method} {catalog_route.path} transport catalog={catalog_route.transport!r} "
                f"inventory={inventory_route.transport!r}"
            )
    if mismatches:
        raise CatalogError("inventory metadata mismatch; " + "; ".join(mismatches))


def format_keys(keys: Iterable[tuple[str, str]]) -> str:
    return ", ".join(f"{method} {route_path}" for method, route_path in keys)


def parse_arguments() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="validate the complete /api/v2 package route ownership catalog")
    parser.add_argument("--catalog", type=Path, default=REPO_ROOT / "config" / "v2-package-route-catalog.json")
    parser.add_argument("--router", type=Path, default=REPO_ROOT / "internal" / "router" / "router.go")
    parser.add_argument(
        "--inventory",
        help="read inventory JSON from this file or stdin when the value is '-'; omit to invoke the Go AST inventory",
    )
    return parser.parse_args()


def main() -> int:
    arguments = parse_arguments()
    try:
        catalog = load_catalog(arguments.catalog)
        if arguments.inventory is None:
            inventory = inventory_from_go(arguments.router)
        elif arguments.inventory == "-":
            inventory = load_inventory(None, raw_inventory=sys.stdin.read())
        else:
            inventory = load_inventory(Path(arguments.inventory))
        validate_catalog_against_inventory(catalog, inventory)
    except CatalogError as error:
        print(f"v2 package route catalog: {error}", file=sys.stderr)
        return 1

    print(f"v2 package route catalog passed ({len(catalog)} routes)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
