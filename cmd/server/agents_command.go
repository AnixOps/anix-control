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

	"github.com/AnixOps/anix-control/v4/internal/agenttransport"
	"github.com/AnixOps/anix-control/v4/internal/config"
	"gorm.io/gorm"
)

const agentsCommandUsage = `usage:
  anix-control agents transports [--json] [--legacy-only]
  anix-control agents transports --check-required [--json]

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
legacy, third-party (UniProxy or v2board gRPC only) or unseen. From v4.2
agent_control.mtls defaults to required, which refuses legacy agents.

--check-required is the upgrade gate: it lists every enabled node required
would refuse (legacy: its newest AnixOps Agent channel is legacy;
never_enrolled: never seen and no valid agent certificate) and exits with
status 3 when there is one, 0 when there is none (2 on any other error).
Run it before upgrading to v4.2 or setting required yourself.

Sightings are written at most once a minute per node and transport, so a
node may show the previous minute's state.

The config file comes from ANIX_CONTROL_CONFIG or config/config.yaml.`

// errAgentsNotReady is --check-required's answer when required would
// refuse an enabled node; main exits with status 3 on it.
var errAgentsNotReady = errors.New("agent_control.mtls: required would refuse enabled nodes")

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
	legacyOnly := flags.Bool("legacy-only", false, "only the nodes on a legacy AnixOps Agent channel")
	checkRequired := flags.Bool("check-required", false, "exit 3 when agent_control.mtls: required would refuse an enabled node")
	if err := flags.Parse(arguments[1:]); err != nil || flags.NArg() != 0 || (*checkRequired && *legacyOnly) {
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
	if *checkRequired {
		return checkRequiredReadiness(stdout, inventory, *asJSON)
	}
	if *asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(inventory)
	}
	return printAgentTransports(stdout, inventory, *legacyOnly)
}

// checkRequiredReadiness prints the enabled nodes agent_control.mtls:
// required would refuse and fails with errAgentsNotReady when there is one.
func checkRequiredReadiness(stdout io.Writer, inventory agenttransport.Inventory, asJSON bool) error {
	summary := inventory.Summary
	if asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(struct {
			Mode             string                           `json:"mode"`
			ReadyForRequired bool                             `json:"ready_for_required"`
			Reasons          []string                         `json:"required_reasons"`
			Blockers         []agenttransport.RequiredBlocker `json:"required_blockers"`
			UpgradeGuide     string                           `json:"upgrade_guide"`
		}{inventory.Mode, summary.ReadyForRequired, summary.RequiredReasons, summary.RequiredBlockers, inventory.UpgradeGuide}); err != nil {
			return err
		}
	} else {
		_, _ = fmt.Fprintf(stdout, "agent_control.mtls: %s\n", inventory.Mode)
		if summary.ReadyForRequired {
			_, _ = fmt.Fprintln(stdout, "Ready for agent_control.mtls: required: it refuses no enabled node.")
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
		_, _ = fmt.Fprintln(stdout, "Not ready for agent_control.mtls: required:")
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

// requiredReadinessWarningLimit caps the nodes the startup warning names.
const requiredReadinessWarningLimit = 10

// logRequiredReadiness warns at startup, under agent_control.mtls:
// required, about the enabled nodes it refuses, from the transport
// inventory. It never stops the start: an operator may cut legacy nodes on
// purpose. A failure to read the inventory is logged and ignored.
func logRequiredReadiness(ctx context.Context, cfg *config.Config, db *gorm.DB, logf func(string, ...any)) {
	if cfg == nil || db == nil || cfg.AgentControl.MTLSOrDefault() != config.AgentMTLSRequired {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	inventory, err := agenttransport.Build(ctx, db, agenttransport.PolicyFrom(cfg.AgentControl), agenttransport.Options{})
	if err != nil {
		logf("Agent transports: the readiness check for agent_control.mtls=required could not read the transport inventory: %v", err)
		return
	}
	if line := requiredReadinessWarning(inventory); line != "" {
		logf("%s", line)
	}
}

// requiredReadinessWarning is the startup warning of an inventory under
// required: empty when it refuses no enabled node.
func requiredReadinessWarning(inventory agenttransport.Inventory) string {
	summary := inventory.Summary
	if summary.ReadyForRequired {
		return ""
	}
	legacy, recent, never := summary.RequiredBlockerCounts()
	// Recent legacy Agents first: they were in use until the upgrade.
	var named, rest []string
	for _, blocker := range summary.RequiredBlockers {
		if blocker.Reason == agenttransport.BlockerLegacy && blocker.Recent {
			named = append(named, blocker.Node+" ("+blocker.Transport+")")
		} else {
			rest = append(rest, blocker.Node+" ("+blocker.Reason+")")
		}
	}
	named = append(named, rest...)
	more := ""
	if len(named) > requiredReadinessWarningLimit {
		more = fmt.Sprintf(", and %d more", len(named)-requiredReadinessWarningLimit)
		named = named[:requiredReadinessWarningLimit]
	}
	days := int(agenttransport.RecentLegacyWindow / (24 * time.Hour))
	return fmt.Sprintf("WARNING: Agent transports: agent_control.mtls=required refuses %d enabled node(s): %d on a legacy AnixOps Agent channel, "+
		"%d of them seen within the last %d days (their Agents are cut off now), and %d never enrolled: %s%s. "+
		"Inspect with `anix-control agents transports --check-required` or GET /api/v4/kernel/agents/transports; "+
		"to serve legacy Agents while they migrate, set agent_control.mtls: preferred and restart (%s).",
		len(summary.RequiredBlockers), legacy, recent, days, never, strings.Join(named, ", "), more, inventory.UpgradeGuide)
}
