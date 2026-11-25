// Command anix-relay is the region relay (docs/design/region-relay): it
// forwards the region's node connections to Control byte for byte and keeps
// a read-only cache of release files by SHA-256. It has no web interface,
// no API and no node state, and holds no node or Control key.
//
//	anix-relay -config /etc/anix-relay/relay.yaml
//	anix-relay -config /etc/anix-relay/relay.yaml -check
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/AnixOps/anix-control/v5/internal/regionrelay"
)

// version is set by the release build (-ldflags "-X main.version=...").
var version = "dev"

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	flags := flag.NewFlagSet("anix-relay", flag.ContinueOnError)
	configPath := flags.String("config", "/etc/anix-relay/relay.yaml", "the relay configuration file")
	check := flags.Bool("check", false, "validate the configuration and exit")
	showVersion := flags.Bool("version", false, "print the version and exit")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *showVersion {
		fmt.Println(version)
		return 0
	}
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil)).With("service", "anix-relay")
	config, err := regionrelay.LoadConfig(*configPath)
	if err != nil {
		logger.Error("invalid configuration", "path", *configPath, "error", err)
		return 1
	}
	if *check {
		fmt.Printf("%s: %d forward(s), cache %t\n", *configPath, len(config.Forwards), config.Cache != nil)
		return 0
	}
	relay, err := regionrelay.Start(config, logger)
	if err != nil {
		logger.Error("could not start", "error", err)
		return 1
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	logger.Info("relay started", "version", version)
	if err := relay.Serve(ctx); err != nil {
		logger.Error("relay stopped", "error", err)
		return 1
	}
	logger.Info("relay stopped")
	return 0
}
