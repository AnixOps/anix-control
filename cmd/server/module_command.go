package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/modulepki"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"gorm.io/gorm"
)

const moduleCommandUsage = `usage:
  anix-control module token create -package <id> [-ttl 1h] [-reusable]
  anix-control module token list
  anix-control module token revoke <enrollment-id>
  anix-control module ca rotate
  anix-control module ca bundle
  anix-control module runtime list
  anix-control module runtime set <plugin-id> <local|remote>

The config file comes from ANIX_CONTROL_CONFIG or config/config.yaml.`

func moduleUsageError() error {
	return fmt.Errorf("invalid module command\n%s", moduleCommandUsage)
}

// takeModuleCommand removes a leading "module" argument and returns the
// arguments after it; nil means the process is not running a module command.
func takeModuleCommand() []string {
	if len(os.Args) > 1 && os.Args[1] == "module" {
		arguments := append([]string{}, os.Args[2:]...)
		os.Args = os.Args[:1]
		return arguments
	}
	return nil
}

// runModuleCommand administers the built-in module PKI from the command line.
// It prints JSON so scripts can pick out the credential.
func runModuleCommand(ctx context.Context, cfg *config.Config, db *gorm.DB, arguments []string, stdout io.Writer) error {
	if len(arguments) < 2 {
		return moduleUsageError()
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	// Runtime selection needs no PKI: it only records the package runtime.
	switch arguments[0] + " " + arguments[1] {
	case "runtime list":
		var rows []model.PluginRuntime
		if err := db.WithContext(ctx).Order("plugin_id").Find(&rows).Error; err != nil {
			return err
		}
		return encoder.Encode(rows)
	case "runtime set":
		if len(arguments) != 4 {
			return moduleUsageError()
		}
		row, err := service.SetPluginRuntime(ctx, db, arguments[2], arguments[3], cfg.ModuleRuntime.Enabled, 0)
		if err != nil {
			return err
		}
		return encoder.Encode(row)
	}
	authority, err := modulepki.FromConfig(cfg.ModuleRuntime, db)
	if err != nil {
		return err
	}
	switch arguments[0] + " " + arguments[1] {
	case "token create":
		flags := flag.NewFlagSet("module token create", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		packageID := flags.String("package", "", "module package id")
		ttl := flags.Duration("ttl", time.Hour, "credential lifetime")
		reusable := flags.Bool("reusable", false, "allow repeated enrollment until expiry or revocation")
		if err := flags.Parse(arguments[2:]); err != nil {
			return fmt.Errorf("%w\n%s", err, moduleCommandUsage)
		}
		credential, row, err := authority.CreateEnrollment(ctx, modulepki.EnrollmentRequest{
			PackageID: *packageID, TTL: *ttl, Reusable: *reusable,
		})
		if err != nil {
			return err
		}
		return encoder.Encode(map[string]any{"enrollment": row, "credential": credential})
	case "token list":
		rows, err := authority.ListEnrollments(ctx)
		if err != nil {
			return err
		}
		return encoder.Encode(rows)
	case "token revoke":
		if len(arguments) != 3 {
			return moduleUsageError()
		}
		if err := authority.RevokeEnrollment(ctx, arguments[2]); err != nil {
			return err
		}
		return encoder.Encode(map[string]any{"revoked": arguments[2]})
	case "ca bundle":
		bundle, err := authority.TrustBundlePEM(ctx)
		if err != nil {
			return err
		}
		_, err = stdout.Write(bundle)
		return err
	case "ca rotate":
		next, err := authority.Rotate(ctx)
		if err != nil {
			return err
		}
		return encoder.Encode(next)
	default:
		return moduleUsageError()
	}
}
