package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/agenttransport"
	"github.com/AnixOps/anix-control/v4/internal/config"
	"gorm.io/gorm"
)

const agentsCommandUsage = `usage:
  anix-control agents transports [--json] [--legacy-only]

transports lists every proxy and forward node with the transport its agent
was last seen on, the agent version, its newest valid agent certificate and
when it was last seen:
  mtls-stream    the Agent Control stream with a client certificate (ready)
  apikey-stream  the Agent Control stream with the node API key (legacy)
  http-legacy    /api/v2/agent/*, /api/v2/node/*, the forward agent rules (legacy)
  websocket      the agent or node WebSocket (legacy)
  clean-agent    a forward clean agent, /api/v2/forward-agent/* (legacy)
  uniproxy       UniProxy, shared with third-party node software (unaffected)
  v2board-grpc   the v2board gRPC services, likewise (unaffected)

A node's status is decided by its newest AnixOps Agent channel: mtls,
legacy, third-party (UniProxy or v2board gRPC only) or unseen. Before
upgrading to v4.2, where agent_control.mtls defaults to required, every
node listed by --legacy-only must run an Agent that has enrolled.

Sightings are written at most once a minute per node and transport, so a
node may show the previous minute's state.

The config file comes from ANIX_CONTROL_CONFIG or config/config.yaml.`

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
	legacyOnly := flags.Bool("legacy-only", false, "only the nodes agent_control.mtls: required would refuse")
	if err := flags.Parse(arguments[1:]); err != nil || flags.NArg() != 0 {
		return agentsUsageError()
	}
	agentControl := config.AgentControlConfig{}
	if cfg != nil {
		agentControl = cfg.AgentControl
	}
	inventory, err := agenttransport.Build(ctx, db, agenttransport.PolicyFrom(agentControl), agenttransport.Options{LegacyOnly: *legacyOnly})
	if err != nil {
		return err
	}
	if *asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(inventory)
	}
	return printAgentTransports(stdout, inventory, *legacyOnly)
}

func printAgentTransports(stdout io.Writer, inventory agenttransport.Inventory, legacyOnly bool) error {
	sunset := "none"
	if inventory.Sunset != nil {
		sunset = inventory.Sunset.Format(time.DateOnly)
	}
	_, _ = fmt.Fprintf(stdout, "agent_control.mtls: %s (legacy sunset: %s)\n", inventory.Mode, sunset)
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
	if legacyOnly {
		if summary.Legacy == 0 {
			_, _ = fmt.Fprintln(stdout, "No node uses a legacy AnixOps Agent channel: agent_control.mtls: required refuses none.")
			return nil
		}
		_, _ = fmt.Fprintf(stdout, "%d node(s) on a legacy AnixOps Agent channel: upgrade and enroll their agents before v4.2 (%s).\n",
			summary.Legacy, inventory.UpgradeGuide)
		return nil
	}
	_, _ = fmt.Fprintf(stdout, "%d node(s): %d mtls, %d legacy, %d third-party, %d unseen.\n",
		summary.Total, summary.MTLS, summary.Legacy, summary.ThirdParty, summary.Unseen)
	return nil
}

func dash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}
