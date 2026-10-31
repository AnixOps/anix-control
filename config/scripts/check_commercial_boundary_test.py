from __future__ import annotations

import tempfile
import unittest
from pathlib import Path

import check_commercial_boundary as gate

BOUNDARY = {
    "commercial": ["internal/service/order*.go"],
    "wiring": ["internal/router/**"],
    "symbols": ["OrderService", "model\\.(Coupon)"],
    "allowed_dependents": [],
}


def tree(files: dict[str, str]) -> Path:
    root = Path(tempfile.mkdtemp())
    for name, content in files.items():
        path = root / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(content, encoding="utf-8")
    return root


class CommercialBoundaryTest(unittest.TestCase):
    def test_the_repository_respects_its_boundary(self) -> None:
        boundary = gate.load_boundary(gate.REPO_ROOT / "config" / "commercial-boundary.json")
        self.assertEqual(gate.check(gate.REPO_ROOT, boundary), [])

    def test_commercial_and_wiring_files_may_use_the_set(self) -> None:
        root = tree({
            "internal/service/order_service.go": "var _ OrderService",
            "internal/router/router.go": "NewOrderService()",
        })
        self.assertEqual(gate.check(root, BOUNDARY), [])

    def test_a_new_dependent_fails(self) -> None:
        root = tree({"internal/service/stats.go": "x := model.Coupon{}"})
        problems = gate.check(root, BOUNDARY)
        self.assertEqual(len(problems), 1)
        self.assertIn("internal/service/stats.go", problems[0])

    def test_test_files_are_ignored(self) -> None:
        root = tree({"internal/service/stats_test.go": "model.Coupon{}"})
        self.assertEqual(gate.check(root, BOUNDARY), [])

    def test_a_listed_file_that_no_longer_depends_fails(self) -> None:
        root = tree({"internal/service/stats.go": "package service"})
        boundary = dict(BOUNDARY, allowed_dependents=["internal/service/stats.go"])
        problems = gate.check(root, boundary)
        self.assertEqual(len(problems), 1)
        self.assertIn("remove it", problems[0])

    def test_a_listed_dependent_passes(self) -> None:
        root = tree({"internal/service/stats.go": "OrderService"})
        boundary = dict(BOUNDARY, allowed_dependents=["internal/service/stats.go"])
        self.assertEqual(gate.check(root, boundary), [])


if __name__ == "__main__":
    unittest.main()
