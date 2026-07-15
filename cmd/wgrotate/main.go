package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/AnixOps/anix-control/v3/internal/config"
	"github.com/AnixOps/anix-control/v3/internal/database"
	"github.com/AnixOps/anix-control/v3/internal/model"
	"github.com/AnixOps/anix-control/v3/internal/service"
)

const rotationConfirmation = "ROTATE-ALL-WIREGUARD-PEERS"

const (
	primaryControlConfigPath = "/opt/anixops/control/config/config.yaml"
	legacyControlConfigPath  = "/etc/v2board/config.yaml"
)

type options struct {
	configPath string
	protocolID uint
	dryRun     bool
	confirm    string
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "wireguard peer rotation failed:", err)
		os.Exit(1)
	}
}

func run(args []string, output io.Writer) error {
	opts, err := parseOptions(args)
	if err != nil {
		return err
	}
	if !opts.dryRun && opts.confirm != rotationConfirmation {
		return fmt.Errorf("destructive rotation requires -confirm %s", rotationConfirmation)
	}

	resolvedConfig, err := filepath.Abs(opts.configPath)
	if err != nil {
		return err
	}
	cfg, err := config.Load(resolvedConfig)
	if err != nil {
		return err
	}
	if driver := strings.ToLower(cfg.Database.Driver); driver == "" || driver == "sqlite" || driver == "sqlite3" {
		if !filepath.IsAbs(cfg.Database.Database) {
			cfg.Database.Database = filepath.Join(filepath.Dir(resolvedConfig), cfg.Database.Database)
		}
	}
	if err := database.Init(&cfg.Database); err != nil {
		return err
	}
	defer func() {
		_ = database.Close()
	}()

	db := database.Get()
	query := db.Model(&model.WireGuardPeer{})
	if opts.protocolID > 0 {
		query = query.Where("node_protocol_id = ?", opts.protocolID)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return err
	}
	if opts.dryRun {
		_, err := fmt.Fprintf(output, "dry-run: %d WireGuard peer keys would be rotated\n", count)
		return err
	}
	if count == 0 {
		return errors.New("no WireGuard peers matched the rotation scope")
	}

	result, err := service.RotateWireGuardPeerKeys(db, opts.protocolID)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(output, "rotated_peers=%d protocol_ids=%v\n", result.RotatedPeerCount, result.ProtocolIDs)
	return err
}

func parseOptions(args []string) (options, error) {
	var opts options
	flags := flag.NewFlagSet("wgrotate", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&opts.configPath, "config", defaultControlConfigPath(), "AnixOps Control configuration file")
	flags.UintVar(&opts.protocolID, "protocol-id", 0, "rotate only one node protocol; zero rotates all")
	flags.BoolVar(&opts.dryRun, "dry-run", true, "count matching peers without changing keys")
	flags.StringVar(&opts.confirm, "confirm", "", "required confirmation for a destructive rotation")
	if err := flags.Parse(args); err != nil {
		return options{}, err
	}
	if flags.NArg() != 0 {
		return options{}, errors.New("unexpected positional arguments")
	}
	return opts, nil
}

func defaultControlConfigPath() string {
	if _, err := os.Stat(primaryControlConfigPath); err == nil {
		return primaryControlConfigPath
	}
	if _, err := os.Stat(legacyControlConfigPath); err == nil {
		return legacyControlConfigPath
	}
	return primaryControlConfigPath
}
