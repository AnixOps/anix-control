#!/usr/bin/env python3
"""Run the shared GOST Mesh semantic contract against a real Agent binary."""

from __future__ import annotations

import argparse
import copy
import json
import os
import subprocess
import tempfile
from pathlib import Path
from typing import Any


FORMAT = "anixops.gost-mesh.semantic-cases/v1"
CA_FILE = "/run/anixops/secrets/mesh-ca.pem"
CLIENT_CERT_FILE = "/run/anixops/secrets/mesh-client.pem"
CLIENT_KEY_FILE = "/run/anixops/secrets/mesh-client-key.pem"
SERVER_CERT_FILE = "/run/anixops/secrets/mesh-server.pem"
SERVER_KEY_FILE = "/run/anixops/secrets/mesh-server-key.pem"


class ContractError(RuntimeError):
    """Raised when the shared semantic contract or Agent result is invalid."""


def health(enabled: bool, source_address: str = "") -> dict[str, Any]:
    return {
        "enabled": enabled,
        "target": "1.1.1.1:443" if enabled else "",
        "source_address": source_address if enabled else "",
        "interval_seconds": 15,
        "timeout_seconds": 3,
        "failure_threshold": 3,
        "restart_delay_seconds": 3,
        "restart_limit": 10,
    }


def entry_tunnel(index: int, transport: str) -> dict[str, Any]:
    source_network = f"10.0.{index}.0/24"
    tunnel = {
        "id": f"mesh-entry-{index:03d}",
        "role": "entry",
        "transport": transport,
        "remote": {"host": "exit.example.com", "port": 443},
        "tun": {
            "name": f"anxg{index:03d}",
            "address": f"172.16.{index}.2/30",
            "peer_address": f"172.16.{index}.1",
            "port": 18000 + index,
            "mtu": 1280,
        },
        "routing": {
            "source_cidrs": [source_network],
            "route_cidrs": [],
            "table": index + 1,
            "priority": 10000 + index,
        },
        "tls": {
            "server_name": "exit.example.com",
            "ca_file": CA_FILE,
            "cert_file": CLIENT_CERT_FILE,
            "key_file": CLIENT_KEY_FILE,
        },
        "wss_path": "/anixops-mesh" if transport == "wss" else "",
        "health": health(True, f"10.0.{index}.1"),
    }
    return tunnel


def exit_tunnel(transport: str, explicit_zeroes: bool) -> dict[str, Any]:
    routing: dict[str, Any] = {
        "source_cidrs": [],
        "route_cidrs": ["10.66.0.0/24"],
    }
    if explicit_zeroes:
        routing.update({"table": 0, "priority": 0})
    return {
        "id": f"mesh-exit-{transport}",
        "role": "exit",
        "transport": transport,
        "listen": {"address": "0.0.0.0", "port": 443},
        "tun": {
            "name": f"anx{transport}x",
            "address": "172.31.66.1/30",
            "peer_address": "172.31.66.2",
            "port": 18421,
            "mtu": 1280,
        },
        "routing": routing,
        "tls": {
            "server_name": "",
            "ca_file": CA_FILE,
            "cert_file": SERVER_CERT_FILE,
            "key_file": SERVER_KEY_FILE,
        },
        "wss_path": "/anixops-mesh" if transport == "wss" else "",
        "health": health(False),
    }


def config(tunnels: list[dict[str, Any]]) -> dict[str, Any]:
    return {
        "api_version": "anixops.gost-mesh/v1",
        "apply": True,
        "rollback_on_exit": True,
        "tunnels": tunnels,
    }


def apply_pointer(value: Any, pointer: str, replacement: Any) -> None:
    if not pointer.startswith("/"):
        raise ContractError(f"JSON pointer must be absolute: {pointer!r}")
    parts = [part.replace("~1", "/").replace("~0", "~") for part in pointer[1:].split("/")]
    current = value
    for part in parts[:-1]:
        current = current[int(part)] if isinstance(current, list) else current[part]
    final = parts[-1]
    if isinstance(current, list):
        current[int(final)] = replacement
    else:
        current[final] = replacement


def generate_case(case: dict[str, Any], max_config_bytes: int) -> bytes:
    generator = case.get("generator")
    if generator == "single":
        role = case.get("role")
        transport = case.get("transport")
        if role == "entry":
            value = config([entry_tunnel(66, transport)])
        elif role == "exit":
            value = config([exit_tunnel(transport, bool(case.get("exit_route_zeroes")))])
        else:
            raise ContractError(f"unsupported role in {case.get('name')}: {role!r}")
    elif generator == "entry-set":
        count = case.get("count")
        if not isinstance(count, int) or count < 1 or count > 128:
            raise ContractError(f"invalid entry-set count in {case.get('name')}: {count!r}")
        value = config([entry_tunnel(index, "quic" if index % 2 == 0 else "wss") for index in range(count)])
    elif generator == "duplicate-object":
        tunnel = entry_tunnel(66, "quic")
        value = config([tunnel, copy.deepcopy(tunnel)])
    elif generator == "oversized-document":
        tunnels = [entry_tunnel(index, "quic") for index in range(128)]
        padding = "a" * 768
        for index, tunnel in enumerate(tunnels):
            tunnel["tls"] = {
                "server_name": "exit.example.com",
                "ca_file": f"/run/anixops/secrets/ca-{index:03d}-{padding}.pem",
                "cert_file": f"/run/anixops/secrets/cert-{index:03d}-{padding}.pem",
                "key_file": f"/run/anixops/secrets/key-{index:03d}-{padding}.pem",
            }
        value = config(tunnels)
    else:
        raise ContractError(f"unsupported generator in {case.get('name')}: {generator!r}")

    if "pointer" in case:
        apply_pointer(value, case["pointer"], case.get("value"))
    encoded = json.dumps(value, ensure_ascii=True, separators=(",", ":")).encode("utf-8")
    if generator == "oversized-document":
        if len(encoded) <= max_config_bytes:
            raise ContractError("oversized-document generator did not exceed the canonical JSON limit")
    return encoded


def load_contract(path: Path) -> dict[str, Any]:
    try:
        contract = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, UnicodeError, json.JSONDecodeError) as exc:
        raise ContractError(f"read semantic contract: {exc}") from exc
    if contract.get("format") != FORMAT:
        raise ContractError("semantic contract format is invalid")
    if contract.get("max_config_bytes") != 256 << 10:
        raise ContractError("semantic contract max_config_bytes must remain 262144")
    if not contract.get("valid_cases") or not contract.get("invalid_cases"):
        raise ContractError("semantic contract must contain valid and invalid cases")
    return contract


def run_case(agent: Path, path: Path) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        [str(agent), "--anixops-validate", "--anixops-config", str(path.resolve())],
        check=False,
        capture_output=True,
        text=True,
        timeout=30,
    )


def run_contract(agent: Path, contract_path: Path) -> tuple[int, int]:
    contract = load_contract(contract_path)
    max_config_bytes = contract["max_config_bytes"]
    valid_count = 0
    invalid_count = 0
    with tempfile.TemporaryDirectory(prefix="anixops-gost-mesh-contract-") as temporary_root:
        root = Path(temporary_root)
        for expected_valid, cases in ((True, contract["valid_cases"]), (False, contract["invalid_cases"])):
            for case in cases:
                name = case.get("name")
                if not isinstance(name, str) or not name:
                    raise ContractError("semantic case name is required")
                contents = generate_case(case, max_config_bytes)
                path = root / f"{name}.json"
                path.write_bytes(contents)
                os.chmod(path, 0o600)
                result = run_case(agent, path)
                output = (result.stdout + result.stderr).strip()
                if expected_valid:
                    if result.returncode != 0:
                        raise ContractError(f"valid case {name!r} failed: {output}")
                    valid_count += 1
                    continue
                if result.returncode == 0:
                    raise ContractError(f"invalid case {name!r} unexpectedly succeeded")
                expected_error = case.get("expected_error")
                if expected_error and expected_error.lower() not in output.lower():
                    raise ContractError(
                        f"invalid case {name!r} did not report {expected_error!r}: {output}"
                    )
                invalid_count += 1
    return valid_count, invalid_count


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--agent", required=True, type=Path)
    parser.add_argument(
        "--cases",
        type=Path,
        default=Path(__file__).with_name("semantic_contract_cases.json"),
    )
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    if not args.agent.is_file() or args.agent.is_symlink() or not os.access(args.agent, os.X_OK):
        raise ContractError("--agent must be a real executable file")
    valid_count, invalid_count = run_contract(args.agent.resolve(), args.cases.resolve())
    print(f"semantic_contract_valid={valid_count}")
    print(f"semantic_contract_invalid={invalid_count}")
    print("semantic_contract_network_started=false")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except ContractError as exc:
        raise SystemExit(f"semantic contract failed: {exc}") from exc
