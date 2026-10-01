from __future__ import annotations

import os
import subprocess
import tempfile
import unittest
from pathlib import Path


SCRIPT = Path(__file__).resolve().parent / "check_moved_columns.sh"
MODULE = "github.com/AnixOps/anix-control/v4"

# A stand-in for gorm: the gate recognizes a *gorm.DB by its package path.
FAKE_GORM = {
    "fakegorm/go.mod": "module gorm.io/gorm\n\ngo 1.22\n",
    "fakegorm/gorm.go": (
        "package gorm\n\n"
        "type DB struct{}\n\n"
        "func (db *DB) Model(value any) *DB { return db }\n"
        "func (db *DB) Table(name string) *DB { return db }\n"
        "func (db *DB) Where(query any, args ...any) *DB { return db }\n"
        "func (db *DB) Select(query any, args ...any) *DB { return db }\n"
        "func (db *DB) First(dest any, conds ...any) *DB { return db }\n"
        "func (db *DB) Update(column string, value any) *DB { return db }\n"
    ),
}

MODEL = (
    "package model\n\n"
    "type Node struct {\n\tID uint\n\tName string\n\tAPIKey string\n\tAPIKeyHash string\n\tSecret string\n}\n\n"
    "type ForwardNode struct {\n\tID uint\n\tAPIToken string\n}\n\n"
    "type ForwardCleanAgent struct {\n\tID uint\n\tToken string\n}\n\n"
    "type User struct {\n\tID uint\n\tToken string\n}\n"
)


class MovedColumnsGateTest(unittest.TestCase):
    def run_gate(self, files: dict[str, str]) -> subprocess.CompletedProcess[str]:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            (root / "go.mod").write_text(
                f"module {MODULE}\n\ngo 1.22\n\nrequire gorm.io/gorm v0.0.0\n\nreplace gorm.io/gorm => ./fakegorm\n",
                encoding="utf-8",
            )
            for name, content in {**FAKE_GORM, "internal/model/model.go": MODEL, **files}.items():
                path = root / name
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_text(content, encoding="utf-8")
            environment = dict(os.environ, GOWORK="off", GOFLAGS="-mod=mod")
            return subprocess.run(
                ["bash", str(SCRIPT), "-root", str(root), "-allowlist=false"],
                env=environment, check=False, text=True, capture_output=True,
            )

    def service(self, body: str) -> dict[str, str]:
        return {
            "internal/service/service.go": (
                "package service\n\n"
                f"import (\n\t\"gorm.io/gorm\"\n\n\t\"{MODULE}/internal/model\"\n)\n\n"
                "var _ = gorm.DB{}\nvar _ = model.Node{}\n\n" + body
            )
        }

    def test_passes_writes_and_unrelated_reads(self) -> None:
        result = self.run_gate(self.service(
            "func Write(db *gorm.DB, node *model.Node, user *model.User) string {\n"
            "\tnode.APIKey = \"generated\"\n"
            "\t_ = model.ForwardNode{APIToken: \"typed\"}\n"
            "\tdb.Model(&model.Node{}).Where(\"id = ?\", node.ID).Update(\"api_key\", node.Name)\n"
            "\tdb.Model(&model.User{}).Where(\"token = ?\", \"x\").First(user)\n"
            "\treturn node.Name + user.Token\n"
            "}\n"
        ))
        self.assertEqual(0, result.returncode, result.stderr)
        self.assertIn("moved column gate passed", result.stdout)

    def test_refuses_a_planted_field_read(self) -> None:
        result = self.run_gate(self.service(
            "func Authenticate(node *model.Node, presented string) bool {\n"
            "\treturn node.APIKey == presented\n"
            "}\n"
        ))
        self.assertNotEqual(0, result.returncode)
        self.assertIn("reads v2_node.api_key (model.Node.APIKey)", result.stderr)

    def test_refuses_a_moved_column_in_sql(self) -> None:
        result = self.run_gate(self.service(
            "func Inventory(db *gorm.DB) *gorm.DB {\n"
            "\treturn db.Model(&model.ForwardNode{}).Where(\"api_port = 0 AND api_token = ''\")\n"
            "}\n\n"
            "func Agent(db *gorm.DB, agent *model.ForwardCleanAgent) {\n"
            "\tdb.Where(\"token = ?\", \"t\").First(agent)\n"
            "}\n"
        ))
        self.assertNotEqual(0, result.returncode)
        self.assertIn("names the moved column api_token in the SQL of Where", result.stderr)
        self.assertIn("names the moved column token in the SQL of Where", result.stderr)

    def test_allows_internal_nodesecrets(self) -> None:
        result = self.run_gate({
            "internal/nodesecrets/read.go": (
                f"package nodesecrets\n\nimport \"{MODULE}/internal/model\"\n\n"
                "func Matches(node *model.Node, presented string) bool { return node.APIKey == presented }\n"
            )
        })
        self.assertEqual(0, result.returncode, result.stderr)


if __name__ == "__main__":
    unittest.main()
