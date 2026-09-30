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
	"github.com/AnixOps/anix-control/v4/internal/modulepki"
	"gorm.io/gorm"
)

const moduleCommandUsage = `usage:
  anix-control module token create -package <id> [-ttl 1h] [-reusable]
  anix-control module token list
  anix-control module token revoke <enrollment-id>
  anix-control module ca rotate

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
	authority, err := modulepki.FromConfig(cfg.ModuleRuntime, db)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
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
