package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/AnixOps/anix-control/v4/internal/agentinstall"
	"github.com/AnixOps/anix-control/v4/internal/agentupgrade"
	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/handler"
	"gorm.io/gorm"
)

// runAgentUpgradeCommand runs "agent upgrade <command>" against the
// database; the running Control's singleton worker drives the campaign.
func runAgentUpgradeCommand(ctx context.Context, cfg *config.Config, db *gorm.DB, command string, arguments []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("agent upgrade "+command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	id := flags.String("id", "", "the campaign id")
	version := flags.String("version", "", "the Agent release tag (default this Control's)")
	exclude := flags.String("exclude", "", "nodes to leave out: proxy-<id>,forward-<id>,...")
	excludeTags := flags.String("exclude-tag", "", "node tags to leave out, comma separated")
	reason := flags.String("reason", "", "why (recorded in the audit log)")
	controlURL := flags.String("control", "", "the Control address nodes reach (default agent_install.public_url)")
	rollback := flags.Bool("rollback", false, "abort: roll the current batch back first")
	if err := flags.Parse(arguments); err != nil {
		return fmt.Errorf("%w\n%s", err, agentCommandUsage)
	}
	if flags.NArg() != 0 {
		return agentUsageError()
	}
	service := &agentupgrade.Service{DB: db}
	actor := agentupgrade.Actor{Name: "cli"}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	show := func(campaignID string) error {
		view, err := service.Get(ctx, campaignID)
		if err != nil {
			return err
		}
		return encoder.Encode(view)
	}
	needID := func() error {
		if strings.TrimSpace(*id) == "" {
			return fmt.Errorf("-id is required\n%s", agentCommandUsage)
		}
		return nil
	}
	switch command {
	case "start":
		install := cfg.AgentInstall
		address := *controlURL
		if address == "" {
			address = install.PublicURL
		}
		settings, err := agentinstall.ResolveSettings(agentinstall.SettingsInput{
			ControlURL: address, GRPCTarget: install.GRPCTarget, GRPCPort: cfg.GRPC.Port,
			AgentVersion: install.AgentVersion, ReleaseVersion: handler.ReleaseVersion, ArtifactDir: install.ArtifactDir, CNMirrorURL: install.CNMirrorURL,
		})
		if err != nil {
			return fmt.Errorf("%w (or pass -control https://<control>)", err)
		}
		target := strings.TrimSpace(*version)
		if target == "" {
			target = settings.AgentVersion
		}
		artifacts, err := agentinstall.UpgradeArtifacts(install.ArtifactDir, target, cfg.Plugins.OfficialPublicKey, settings.ControlURL)
		if err != nil {
			return err
		}
		campaign, err := service.Start(ctx, agentupgrade.StartRequest{
			TargetVersion: target, ControlVersion: handler.ReleaseVersion, Artifacts: artifacts,
			Exclude: agentupgrade.Exclude{Nodes: splitList(*exclude), Tags: splitList(*excludeTags)}, Reason: *reason, Actor: actor,
		})
		if err != nil {
			return err
		}
		return show(campaign.ID)
	case "status":
		if *id != "" {
			return show(*id)
		}
		active, ok, err := service.Active(ctx)
		if err != nil {
			return err
		}
		if ok {
			return show(active.ID)
		}
		campaigns, err := service.List(ctx, 1)
		if err != nil {
			return err
		}
		if len(campaigns) == 0 {
			return errors.New("no Agent upgrade campaign yet: anix-control agent upgrade start")
		}
		return show(campaigns[0].ID)
	case "pause":
		if err := needID(); err != nil {
			return err
		}
		if _, err := service.Pause(ctx, *id, actor); err != nil {
			return err
		}
		return show(*id)
	case "resume":
		if err := needID(); err != nil {
			return err
		}
		if _, err := service.Resume(ctx, *id, actor); err != nil {
			return err
		}
		return show(*id)
	case "abort":
		if err := needID(); err != nil {
			return err
		}
		if _, err := service.Abort(ctx, *id, actor, *rollback); err != nil {
			return err
		}
		return show(*id)
	default:
		return agentUsageError()
	}
}

func splitList(value string) []string {
	var out []string
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}
