from __future__ import annotations

import importlib.util
import json
import sys
import unittest
from pathlib import Path


PACKAGE_ROOT = Path(__file__).resolve().parents[1]
SPEC = importlib.util.spec_from_file_location(
    "gost_mesh_semantic_contract",
    PACKAGE_ROOT / "tests" / "run_semantic_contract.py",
)
if SPEC is None or SPEC.loader is None:
    raise RuntimeError("cannot load gost-mesh semantic contract runner")
CONTRACT = importlib.util.module_from_spec(SPEC)
sys.modules[SPEC.name] = CONTRACT
SPEC.loader.exec_module(CONTRACT)
CASES = PACKAGE_ROOT / "tests" / "semantic_contract_cases.json"


class GostMeshSemanticContractTest(unittest.TestCase):
    def test_corpus_generates_bounded_valid_and_explicit_invalid_documents(self) -> None:
        contract = CONTRACT.load_contract(CASES)
        maximum = contract["max_config_bytes"]
        names = [case["name"] for case in contract["valid_cases"] + contract["invalid_cases"]]
        self.assertEqual(len(names), len(set(names)))

        for case in contract["valid_cases"]:
            contents = CONTRACT.generate_case(case, maximum)
            self.assertLessEqual(len(contents), maximum, case["name"])
            json.loads(contents)

        for case in contract["invalid_cases"]:
            contents = CONTRACT.generate_case(case, maximum)
            json.loads(contents)
            if case["generator"] == "oversized-document":
                self.assertGreater(len(contents), maximum)
            else:
                self.assertLessEqual(len(contents), maximum, case["name"])
                self.assertTrue(case.get("expected_error"), case["name"])

    def test_128_tunnel_case_has_no_cross_tunnel_resource_conflicts(self) -> None:
        contract = CONTRACT.load_contract(CASES)
        case = next(case for case in contract["valid_cases"] if case["generator"] == "entry-set")
        value = json.loads(CONTRACT.generate_case(case, contract["max_config_bytes"]))
        tunnels = value["tunnels"]
        self.assertEqual(128, len(tunnels))
        for field in ("id",):
            self.assertEqual(128, len({tunnel[field] for tunnel in tunnels}))
        self.assertEqual(128, len({tunnel["tun"]["name"] for tunnel in tunnels}))
        self.assertEqual(128, len({tunnel["tun"]["address"] for tunnel in tunnels}))
        self.assertEqual(128, len({tunnel["routing"]["table"] for tunnel in tunnels}))
        self.assertEqual(128, len({tunnel["routing"]["priority"] for tunnel in tunnels}))
        self.assertEqual(128, len({tunnel["routing"]["source_cidrs"][0] for tunnel in tunnels}))

    def test_duplicate_object_case_is_an_exact_duplicate(self) -> None:
        contract = CONTRACT.load_contract(CASES)
        case = next(case for case in contract["invalid_cases"] if case["generator"] == "duplicate-object")
        value = json.loads(CONTRACT.generate_case(case, contract["max_config_bytes"]))
        self.assertEqual(2, len(value["tunnels"]))
        self.assertEqual(value["tunnels"][0], value["tunnels"][1])


if __name__ == "__main__":
    unittest.main()
