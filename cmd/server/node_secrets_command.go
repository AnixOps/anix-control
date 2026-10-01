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

	"github.com/AnixOps/anix-control/v4/internal/nodesecrets"
	"gorm.io/gorm"
)

// #nosec G101 -- the usage text names tables and commands, not a credential.
const nodeSecretsCommandUsage = `usage:
  anix-control node-secrets status
  anix-control node-secrets backfill [-table <table>[,<table>...]] [-batch 500] [-restart]
  anix-control node-secrets verify [-table <table>[,<table>...]] [-samples 20]

Tables: v2_node, v2_authorized_key, v2_forward_node, v2_forward_clean_agent,
v2_node_protocol, v2_wireguard_peer (default: all).

backfill copies the node credentials and protocol secrets of the legacy
tables into v4_kernel_node_credential and v4_kernel_protocol_secret. It is
idempotent and resumes an interrupted pass. verify compares the digests of
both forms and exits 3 when they differ. Neither prints a secret.

The config file comes from ANIX_CONTROL_CONFIG or config/config.yaml.`

// errNodeSecretsMismatch makes the process exit 3: verify ran, and the forms
// differ.
var errNodeSecretsMismatch = errors.New("the old and new forms of the node secrets differ")

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
		return encoder.Encode(rows)
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
	}
	return nodeSecretsUsageError()
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
