package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/nodesecrets"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"gorm.io/gorm"
)

// #nosec G101 -- the usage text names tables and commands, not a credential.
const nodeSecretsCommandUsage = `usage:
  anix-control node-secrets status
  anix-control node-secrets backfill [-table <table>[,<table>...]] [-batch 500] [-restart]
  anix-control node-secrets verify [-table <table>[,<table>...]] [-samples 20]
  anix-control node-secrets phase [-by <name>] <table|all> <dual_read|dual_write>
  anix-control node-secrets validate

Tables: v2_node, v2_authorized_key, v2_forward_node, v2_forward_clean_agent,
v2_node_protocol, v2_wireguard_peer (default: all).

status prints each table's phase, backfill progress and last verify, and
the forward nodes that hold a token but no API port (id and name): their
token is pinned to no endpoint, so gost changes and legacy rules on them
fail until an administrator sets the port.

backfill copies the node credentials and protocol secrets of the legacy
tables into v4_kernel_node_credential and v4_kernel_protocol_secret. It is
idempotent and resumes an interrupted pass. verify compares the digests of
both forms and exits 3 when they differ.

phase moves a table's readers to dual_read (the new tables, falling back to
the legacy columns) or back to dual_write (the legacy columns). dual_read
needs the table's latest verify to have matched within the last hour; with
"all", one refusal refuses every table. Each change is audited; -by names who
made it (default: $USER). Running Control processes follow within 5 seconds.

validate checks the secrets of every node protocol and raw configuration as
the configuration builder uses them, and exits 3 when one fails. It reports
and excludes nothing.

No command prints a secret.

The config file comes from ANIX_CONTROL_CONFIG or config/config.yaml.`

// errNodeSecretsMismatch makes the process exit 3: verify ran, and the forms
// differ.
var errNodeSecretsMismatch = errors.New("the old and new forms of the node secrets differ")

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
// (docs/architecture/node-ops-service.md, section 4.3). It prints JSON with
// counts and digests only.
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
		unpinned, err := nodesecrets.ForwardNodesWithoutAPIPort(ctx, db)
		if err != nil {
			return err
		}
		return encoder.Encode(nodeSecretsStatus{
			Tables: rows, ForwardNodesWithoutAPIPort: nodeSecretsUnpinned{Count: len(unpinned), Nodes: unpinned},
		})
	case "backfill":
		flags := flag.NewFlagSet("node-secrets backfill", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		tables := flags.String("table", "", "comma-separated split tables (default: all)")
		batch := flags.Int("batch", nodesecrets.DefaultBatchSize, "legacy rows per transaction")
		restart := flags.Bool("restart", false, "start a new pass even where one was interrupted")
		if err := flags.Parse(arguments[1:]); err != nil || flags.NArg() != 0 || *batch <= 0 {
			return nodeSecretsUsageError()
		}
		results, err := nodesecrets.Backfill(ctx, db, nodesecrets.BackfillOptions{
			Tables: splitTables(*tables), BatchSize: *batch, Restart: *restart,
		})
		if encodeErr := encoder.Encode(results); encodeErr != nil && err == nil {
			err = encodeErr
		}
		return err
	case "verify":
		flags := flag.NewFlagSet("node-secrets verify", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		tables := flags.String("table", "", "comma-separated split tables (default: all)")
		samples := flags.Int("samples", nodesecrets.DefaultSamples, "mismatching secrets to name per table")
		if err := flags.Parse(arguments[1:]); err != nil || flags.NArg() != 0 || *samples <= 0 {
			return nodeSecretsUsageError()
		}
		results, err := nodesecrets.Verify(ctx, db, nodesecrets.VerifyOptions{Tables: splitTables(*tables), Samples: *samples})
		if encodeErr := encoder.Encode(results); encodeErr != nil && err == nil {
			err = encodeErr
		}
		if errors.Is(err, nodesecrets.ErrMismatch) {
			return errNodeSecretsMismatch
		}
		return err
	case "phase":
		flags := flag.NewFlagSet("node-secrets phase", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		by := flags.String("by", os.Getenv("USER"), "who changes the phase, for the audit entry")
		if err := flags.Parse(arguments[1:]); err != nil || flags.NArg() != 2 {
			return nodeSecretsUsageError()
		}
		var tables []string
		if table := flags.Arg(0); table != "all" {
			tables = splitTables(table)
			if len(tables) == 0 {
				return nodeSecretsUsageError()
			}
		}
		phase := flags.Arg(1)
		if phase != nodesecrets.PhaseDualRead && phase != nodesecrets.PhaseDualWrite {
			return nodeSecretsUsageError()
		}
		changes, err := nodesecrets.SetPhase(ctx, db, nodesecrets.PhaseOptions{Tables: tables, Phase: phase, Actor: *by})
		if changes != nil {
			if encodeErr := encoder.Encode(changes); encodeErr != nil && err == nil {
				err = encodeErr
			}
		}
		return err
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

// nodeSecretsStatus is what status prints: the split tables' state, and
// the forward nodes whose token is pinned to no endpoint because they have
// no API port (docs/UPGRADE.md), by id and name only.
type nodeSecretsStatus struct {
	Tables                     []model.NodeSecretSplit `json:"tables"`
	ForwardNodesWithoutAPIPort nodeSecretsUnpinned     `json:"forward_nodes_without_api_port"`
}

type nodeSecretsUnpinned struct {
	Count int                               `json:"count"`
	Nodes []nodesecrets.UnpinnedForwardNode `json:"nodes"`
}

func splitTables(value string) []string {
	var tables []string
	for _, table := range strings.Split(value, ",") {
		if table = strings.TrimSpace(table); table != "" {
			tables = append(tables, table)
		}
	}
	return tables
}
