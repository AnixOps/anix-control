package main

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"gorm.io/gorm"
)

const routesCommandUsage = `usage:
  anix-control routes list [--package <id>] [--json]
  anix-control routes set --package <id> [--route <route-id> ...] --mode <legacy|shadow|native> [--reason <text>] [--yes] [--json]
  anix-control routes rollback --package <id> [--reason <text>] [--json]
  anix-control routes history [--package <id>] [--limit 100] [--json]

list prints each v2 Control package route with its configured and effective
mode and the modes it may switch to. set switches the named routes, or
without --route every route of the package that may switch to the mode;
switching to native needs --reason and --yes. rollback returns the whole
package to legacy in one change (identity group A excepted: it moves only
with the identity cutover and rollback). history prints the latest switches.

Every switch is written to the audit log and the revision history as
system/cli. Package hosts apply it at their next configuration poll (about
5 seconds).

The config file comes from ANIX_CONTROL_CONFIG or config/config.yaml.`

func routesUsageError() error {
	return fmt.Errorf("invalid routes command\n%s", routesCommandUsage)
}

// routeList collects repeated --route flags; a value may also hold several
// comma-separated route ids.
type routeList []string

func (l *routeList) String() string { return strings.Join(*l, ",") }

func (l *routeList) Set(value string) error {
	for _, route := range strings.Split(value, ",") {
		if route = strings.TrimSpace(route); route != "" {
			*l = append(*l, route)
		}
	}
	return nil
}

var cliRouteModeActor = service.RouteModeActor{Name: service.RouteModeCLIActor}

func routesPublicKey(cfg *config.Config) (ed25519.PublicKey, error) {
	if cfg == nil || strings.TrimSpace(cfg.Plugins.OfficialPublicKey) == "" {
		return nil, service.ErrPluginTrustRootRequired
	}
	return service.ParseOfficialPluginPublicKey(cfg.Plugins.OfficialPublicKey)
}

// runRoutesCommand lists and switches package route modes from the command
// line.
func runRoutesCommand(ctx context.Context, cfg *config.Config, db *gorm.DB, arguments []string, stdout io.Writer) error {
	if len(arguments) == 0 {
		return routesUsageError()
	}
	flags := flag.NewFlagSet("routes "+arguments[0], flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	packageID := flags.String("package", "", "package id")
	asJSON := flags.Bool("json", false, "print JSON")
	var routes routeList
	var mode, reason *string
	var yes *bool
	limit := new(int)
	switch arguments[0] {
	case "list":
	case "set":
		flags.Var(&routes, "route", "route id (repeatable)")
		mode = flags.String("mode", "", "legacy, shadow or native")
		reason = flags.String("reason", "", "why the routes switch")
		yes = flags.Bool("yes", false, "confirm a switch to native")
	case "rollback":
		reason = flags.String("reason", "", "why the package rolls back")
	case "history":
		limit = flags.Int("limit", 100, "at most this many revisions")
	default:
		return routesUsageError()
	}
	if err := flags.Parse(arguments[1:]); err != nil || flags.NArg() != 0 {
		return routesUsageError()
	}
	*packageID = strings.TrimSpace(*packageID)
	admin := &service.RouteModeAdmin{DB: db}
	publicKey, keyErr := routesPublicKey(cfg)
	if keyErr == nil {
		admin.PublicKey = publicKey
	}
	switch arguments[0] {
	case "list":
		packages, err := admin.List(ctx, *packageID, nil)
		if err != nil {
			return err
		}
		if *asJSON {
			return encodeRoutesJSON(stdout, packages)
		}
		return printRouteModes(stdout, packages)
	case "history":
		revisions, err := admin.Revisions(ctx, *packageID, *limit)
		if err != nil {
			return err
		}
		if *asJSON {
			return encodeRoutesJSON(stdout, revisions)
		}
		writer := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
		_, _ = fmt.Fprintln(writer, "TIME\tPACKAGE\tROUTE\tACTION\tFROM\tTO\tACTOR\tREVISION\tREASON")
		for _, revision := range revisions {
			_, _ = fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%d\t%s\n",
				revision.CreatedAt.UTC().Format("2006-01-02 15:04:05"), revision.PackageID, revision.RouteID, revision.Action,
				revision.FromMode, revision.ToMode, revision.Actor, revision.ConfigRevision, revision.Reason)
		}
		return writer.Flush()
	}
	if *packageID == "" {
		return fmt.Errorf("--package is required\n%s", routesCommandUsage)
	}
	if keyErr != nil {
		return keyErr
	}
	var result service.RouteModeResult
	var err error
	if arguments[0] == "set" {
		*mode = strings.TrimSpace(*mode)
		if *mode == "" {
			return fmt.Errorf("--mode is required\n%s", routesCommandUsage)
		}
		result, err = admin.Set(ctx, service.RouteModeChangeRequest{
			PackageID: *packageID, Routes: routes, Mode: *mode, Reason: *reason, Confirm: *yes,
		}, cliRouteModeActor)
		if errors.Is(err, service.ErrRouteModeConfirmationRequired) {
			return errors.New("switching to native needs --reason and --yes")
		}
	} else {
		result, err = admin.Rollback(ctx, *packageID, *reason, cliRouteModeActor)
	}
	if err != nil {
		return err
	}
	if *asJSON {
		return encodeRoutesJSON(stdout, result)
	}
	if len(result.Changes) == 0 {
		_, _ = fmt.Fprintf(stdout, "%s: no route changed (configuration revision %d)\n", result.PackageID, result.ConfigRevision)
	} else {
		_, _ = fmt.Fprintf(stdout, "%s: %d route(s) changed, configuration revision %d, group %s\n",
			result.PackageID, len(result.Changes), result.ConfigRevision, result.GroupID)
	}
	for _, change := range result.Changes {
		_, _ = fmt.Fprintf(stdout, "  %s: %s -> %s\n", change.RouteID, change.From, change.To)
	}
	for _, skip := range result.Skipped {
		_, _ = fmt.Fprintf(stdout, "  skipped %s: %s\n", skip.RouteID, skip.Reason)
	}
	return nil
}

func encodeRoutesJSON(stdout io.Writer, value any) error {
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func printRouteModes(stdout io.Writer, packages []service.PackageRouteModes) error {
	writer := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(writer, "PACKAGE\tROUTE\tMETHOD\tCATALOG\tCONFIGURED\tEFFECTIVE\tALLOWED")
	for _, pkg := range packages {
		if pkg.Error != "" {
			_, _ = fmt.Fprintf(writer, "%s\t(error: %s)\t\t\t\t\t\n", pkg.PackageID, pkg.Error)
			continue
		}
		for _, route := range pkg.Routes {
			allowed := strings.Join(route.AllowedModes, ",")
			if route.Locked != "" {
				allowed = "locked: " + route.Locked
			}
			catalog := route.Catalog
			if catalog == "" {
				catalog = "-"
			}
			_, _ = fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				pkg.PackageID, route.RouteID, route.Method, catalog, route.Configured, route.Effective, allowed)
		}
	}
	return writer.Flush()
}
