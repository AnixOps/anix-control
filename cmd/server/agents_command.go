package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/agenttransport"
	"github.com/AnixOps/anix-control/v5/internal/config"
	"gorm.io/gorm"
)

const agentsCommandUsage = `usage:
  anix-control agents transports [--json]
  anix-control agents transports --check-required [--json]

transports lists every proxy and forward node with the transport its agent
was last seen on, the agent version, its newest valid agent certificate and
when it was last seen:
  mtls-stream    the Agent Control stream with a client certificate

A node's status is mtls (seen on the stream) or unseen.

--check-required lists every enabled node that never enrolled (never seen
and no valid agent certificate) and exits with status 3 when there is one,
0 when there is none (2 on any other error).

Sightings are written at most once a minute per node and transport, so a
node may show the previous minute's state.

The config file comes from ANIX_CONTROL_CONFIG or config/config.yaml.`

// errAgentsNotReady is --check-required's answer when an enabled node never
// enrolled; main exits with status 3 on it.
var errAgentsNotReady = errors.New("enabled nodes never enrolled")

func agentsUsageError() error {
	return fmt.Errorf("invalid agents command\n%s", agentsCommandUsage)
}

// runAgentsCommand prints the agent transport inventory: the pre-v4.2
// upgrade checklist.
func runAgentsCommand(ctx context.Context, cfg *config.Config, db *gorm.DB, arguments []string, stdout io.Writer) error {
	if len(arguments) == 0 || arguments[0] != "transports" {
		return agentsUsageError()
	}
	flags := flag.NewFlagSet("agents transports", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print JSON")
	checkRequired := flags.Bool("check-required", false, "exit 3 when an enabled node has never enrolled")
	if err := flags.Parse(arguments[1:]); err != nil || flags.NArg() != 0 {
		return agentsUsageError()
	}
	inventory, err := agenttransport.Build(ctx, db, agenttransport.Options{})
	if err != nil {
		return err
	}
	if *checkRequired {
		return checkRequiredReadiness(stdout, inventory, *asJSON)
	}
	if *asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(inventory)
	}
	return printAgentTransports(stdout, inventory)
}

// checkRequiredReadiness prints the enabled nodes that never enrolled and
// fails with errAgentsNotReady when there is one.
func checkRequiredReadiness(stdout io.Writer, inventory agenttransport.Inventory, asJSON bool) error {
	summary := inventory.Summary
	if asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(struct {
			ReadyForRequired bool                             `json:"ready_for_required"`
			Reasons          []string                         `json:"required_reasons"`
			Blockers         []agenttransport.RequiredBlocker `json:"required_blockers"`
			UpgradeGuide     string                           `json:"upgrade_guide"`
		}{summary.ReadyForRequired, summary.RequiredReasons, summary.RequiredBlockers, inventory.UpgradeGuide}); err != nil {
			return err
		}
	} else {
		if summary.ReadyForRequired {
			_, _ = fmt.Fprintln(stdout, "Every enabled node has enrolled.")
			return nil
		}
		writer := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
		_, _ = fmt.Fprintln(writer, "NODE\tNAME\tREASON\tTRANSPORT\tLAST SEEN")
		for _, blocker := range summary.RequiredBlockers {
			lastSeen := "-"
			if blocker.LastSeenAt != nil {
				lastSeen = blocker.LastSeenAt.Format("2006-01-02 15:04:05")
			}
			_, _ = fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\n", blocker.Node, blocker.Name, blocker.Reason, dash(blocker.Transport), lastSeen)
		}
		if err := writer.Flush(); err != nil {
			return err
		}
		_, _ = fmt.Fprintln(stdout, "Not every enabled node has enrolled:")
		for _, reason := range summary.RequiredReasons {
			_, _ = fmt.Fprintf(stdout, "  - %s\n", reason)
		}
		_, _ = fmt.Fprintf(stdout, "See %s\n", inventory.UpgradeGuide)
	}
	if summary.ReadyForRequired {
		return nil
	}
	return errAgentsNotReady
}

func printAgentTransports(stdout io.Writer, inventory agenttransport.Inventory) error {
	writer := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(writer, "NODE\tNAME\tSTATUS\tTRANSPORT\tVERSION\tCERTIFICATE\tLAST SEEN\tSEEN ON")
	for _, node := range inventory.Nodes {
		name := node.Name
		if !node.Enabled {
			name += " (disabled)"
		}
		certificate := "-"
		if node.Certificate != nil {
			certificate = node.Certificate.Serial + " until " + node.Certificate.NotAfter.Format(time.DateOnly)
		}
		lastSeen := "-"
		if node.LastSeenAt != nil {
			lastSeen = node.LastSeenAt.Format("2006-01-02 15:04:05")
		}
		transports := make([]string, 0, len(node.Transports))
		for _, transport := range node.Transports {
			transports = append(transports, transport.Transport)
		}
		_, _ = fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", node.Node, name, node.Status,
			dash(node.Transport), dash(node.AgentVersion), certificate, lastSeen, dash(strings.Join(transports, ",")))
	}
	if err := writer.Flush(); err != nil {
		return err
	}
	summary := inventory.Summary
	_, _ = fmt.Fprintf(stdout, "%d node(s): %d mtls, %d unseen.\n",
		summary.Total, summary.MTLS, summary.Unseen)
	return nil
}

func dash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

// requiredReadinessWarningLimit caps the nodes the startup warning names.
const requiredReadinessWarningLimit = 10

// logRequiredReadiness warns at startup about the enabled nodes that never
// enrolled, from the transport inventory. It never stops the start. A failure to read the inventory is logged and ignored.
func logRequiredReadiness(ctx context.Context, cfg *config.Config, db *gorm.DB, logf func(string, ...any)) {
	if cfg == nil || db == nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	inventory, err := agenttransport.Build(ctx, db, agenttransport.Options{})
	if err != nil {
		logf("Agent transports: the enrollment check could not read the transport inventory: %v", err)
		return
	}
	if line := requiredReadinessWarning(inventory); line != "" {
		logf("%s", line)
	}
}

// requiredReadinessWarning is the startup warning of an inventory: empty when
// every enabled node has enrolled.
func requiredReadinessWarning(inventory agenttransport.Inventory) string {
	summary := inventory.Summary
	if summary.ReadyForRequired {
		return ""
	}
	named := make([]string, 0, len(summary.RequiredBlockers))
	for _, blocker := range summary.RequiredBlockers {
		named = append(named, blocker.Node)
	}
	more := ""
	if len(named) > requiredReadinessWarningLimit {
		more = fmt.Sprintf(", and %d more", len(named)-requiredReadinessWarningLimit)
		named = named[:requiredReadinessWarningLimit]
	}
	return fmt.Sprintf("WARNING: Agent transports: %d enabled node(s) never enrolled: %s%s. "+
		"Inspect with `anix-control agents transports --check-required` or GET /api/v4/kernel/agents/transports (%s).",
		len(summary.RequiredBlockers), strings.Join(named, ", "), more, inventory.UpgradeGuide)
}
