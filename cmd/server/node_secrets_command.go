package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/nodesecrets"
	"github.com/AnixOps/anix-control/v5/internal/service"
	"gorm.io/gorm"
)

// #nosec G101 -- the usage text names tables and commands, not a credential.
const nodeSecretsCommandUsage = `usage:
  anix-control node-secrets status
  anix-control node-secrets validate

Tables: v2_node, v2_node_protocol, v2_wireguard_peer.

status prints each table's state. Every table is finalized: its secrets are
in v4_kernel_protocol_secret, and its legacy columns hold tombstones and
redacted documents.

validate checks the secrets of every node protocol and raw configuration as
the configuration builder uses them, and exits 3 when one fails. It reports
and excludes nothing.

No command prints a secret.

The config file comes from ANIX_CONTROL_CONFIG or config/config.yaml.`

// errNodeSecretsInvalid makes the process exit 3: validate ran, and a secret
// fails validation.
var errNodeSecretsInvalid = errors.New("node secrets fail validation")

func nodeSecretsUsageError() error {
	return fmt.Errorf("invalid node-secrets command\n%s", nodeSecretsCommandUsage)
}

// takeNodeSecretsCommand removes a leading "node-secrets" argument and
// returns the arguments after it; nil means the process is not running a
// node-secrets command.
func takeNodeSecretsCommand() []string {
	if len(os.Args) > 1 && os.Args[1] == "node-secrets" {
		arguments := append([]string{}, os.Args[2:]...)
		os.Args = os.Args[:1]
		return arguments
	}
	return nil
}

// runNodeSecretsCommand runs the node credential split commands
// (docs/architecture/node-ops-service.md, section 4.3). It prints JSON
// without any secret.
func runNodeSecretsCommand(ctx context.Context, db *gorm.DB, arguments []string, stdout io.Writer) error {
	if len(arguments) == 0 {
		return nodeSecretsUsageError()
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	switch arguments[0] {
	case "status":
		if len(arguments) != 1 {
			return nodeSecretsUsageError()
		}
		rows, err := nodesecrets.Status(ctx, db)
		if err != nil {
			return err
		}
		return encoder.Encode(nodeSecretsStatus{Tables: rows})
	case "validate":
		if len(arguments) != 1 {
			return nodeSecretsUsageError()
		}
		report, err := service.ScanNodeSecrets(ctx, db)
		if err != nil {
			return err
		}
		if err := encoder.Encode(report); err != nil {
			return err
		}
		if len(report.Findings) > 0 {
			return errNodeSecretsInvalid
		}
		return nil
	}
	return nodeSecretsUsageError()
}

// nodeSecretsStatus is what status prints: the split tables' state.
type nodeSecretsStatus struct {
	Tables []model.NodeSecretSplit `json:"tables"`
}
