// Command stagingctl drives the local staging rehearsal of route cutovers
// (legacy -> shadow -> native): it installs the signed packages, seeds
// synthetic data, switches a batch's read routes to shadow, replays traffic,
// reconciles write routes on two database copies and writes the batch
// report. scripts/staging/rehearse.sh runs it inside the staging Compose
// network; the runbook is docs/guide/staging-rehearsal.md.
//
// It never switches a route of the rehearsal instance to native: it prints
// the command for that, which an operator runs after sign-off (H7). Only
// the throwaway reconciliation twin "recon-native" runs a batch natively.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

const usage = `usage: stagingctl <command> [flags]

commands:
  wait       wait until Control instances answer /readyz
  install    register, upload and enable the signed packages
  seed       write the deterministic synthetic data set
  batches    print the batch -> package -> route assignment
  shadow     switch a batch's read routes to shadow (super administrator)
  replay     replay read traffic until the thresholds are met, then observe
  reconcile  replay write traffic on the two reconciliation twins and compare
  report     combine a batch's results into report.md and report.json
  rollback   return every package of a batch to legacy`

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "stagingctl:", err)
		os.Exit(1)
	}
}

type commonFlags struct {
	url           string
	adminEmail    string
	adminPassword string
	batch         int
	dir           string
}

func (f *commonFlags) register(flags *flag.FlagSet) {
	flags.StringVar(&f.url, "url", "http://control:8080", "Control base URL")
	flags.StringVar(&f.adminEmail, "admin-email", "root@staging.example.com", "super administrator e-mail")
	flags.IntVar(&f.batch, "batch", 0, "batch number (1-4)")
	flags.StringVar(&f.dir, "dir", "/work/reports", "report directory")
	f.adminPassword = os.Getenv("STAGING_ADMIN_PASSWORD")
}

func run(ctx context.Context, arguments []string) error {
	if len(arguments) == 0 {
		return errors.New(usage)
	}
	command, arguments := arguments[0], arguments[1:]
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	var common commonFlags
	common.register(flags)
	switch command {
	case "wait":
		urls := flags.String("urls", "http://control:8080", "comma-separated base URLs")
		timeout := flags.Duration("timeout", 3*time.Minute, "give up after")
		if err := flags.Parse(arguments); err != nil {
			return err
		}
		for _, base := range strings.Split(*urls, ",") {
			if err := newClient(base).WaitReady(ctx, *timeout); err != nil {
				return err
			}
		}
		return nil
	case "install":
		dir := flags.String("packages", "/work/packages", "directory of signed packages")
		timeout := flags.Duration("timeout", 5*time.Minute, "give up after")
		if err := flags.Parse(arguments); err != nil {
			return err
		}
		client := newClient(common.url)
		token, err := client.Login(ctx, common.adminEmail, common.adminPassword)
		if err != nil {
			return err
		}
		return installPackages(ctx, client, token, *dir, *timeout)
	case "seed":
		options := seedOptions{}
		flags.StringVar(&options.Host, "db-host", "postgres", "PostgreSQL host")
		flags.StringVar(&options.Database, "db", "staging", "database")
		flags.Int64Var(&options.Seed, "seed", envInt("STAGING_SEED", 20261002), "random seed")
		flags.IntVar(&options.Scale, "scale", 1, "data volume multiplier")
		if err := flags.Parse(arguments); err != nil {
			return err
		}
		options.User, options.Password = "staging", os.Getenv("STAGING_DB_PASSWORD")
		return seed(ctx, options)
	case "batches":
		parityOf := flags.Int("parity-of", 0, "print only the parity suites of this batch")
		if err := flags.Parse(arguments); err != nil {
			return err
		}
		if *parityOf != 0 {
			batch, err := batchByNumber(*parityOf)
			if err != nil {
				return err
			}
			fmt.Println(strings.Join(batch.Parity, " "))
			return nil
		}
		return printBatches()
	case "shadow":
		if err := flags.Parse(arguments); err != nil {
			return err
		}
		return switchShadow(ctx, common)
	case "rollback":
		if err := flags.Parse(arguments); err != nil {
			return err
		}
		return rollbackBatch(ctx, common)
	case "replay":
		options := replayOptions{}
		flags.IntVar(&options.MinRequests, "min-requests", 200, "replayed requests per read route")
		flags.DurationVar(&options.MinDuration, "min-shadow-duration", 2*time.Hour, "time each read route must spend in shadow")
		flags.Float64Var(&options.Rate, "rate", 5, "requests per second")
		flags.Int64Var(&options.Seed, "seed", envInt("STAGING_SEED", 20261002), "random seed")
		if err := flags.Parse(arguments); err != nil {
			return err
		}
		return replayReads(ctx, common, options)
	case "reconcile":
		options := reconcileOptions{}
		flags.StringVar(&options.LegacyURL, "legacy-url", "http://recon-legacy:8080", "legacy twin URL")
		flags.StringVar(&options.NativeURL, "native-url", "http://recon-native:8080", "native twin URL")
		flags.StringVar(&options.DBHost, "db-host", "postgres", "PostgreSQL host")
		flags.Int64Var(&options.Seed, "seed", envInt("STAGING_SEED", 20261002), "random seed")
		if err := flags.Parse(arguments); err != nil {
			return err
		}
		options.DBUser, options.DBPassword = "staging", os.Getenv("STAGING_DB_PASSWORD")
		return reconcileWrites(ctx, common, options)
	case "report":
		if err := flags.Parse(arguments); err != nil {
			return err
		}
		return writeReport(common)
	default:
		return fmt.Errorf("unknown command %q\n%s", command, usage)
	}
}

func envInt(name string, fallback int64) int64 {
	var value int64
	if _, err := fmt.Sscan(os.Getenv(name), &value); err == nil {
		return value
	}
	return fallback
}

func printBatches() error {
	if err := checkBatchCoverage(); err != nil {
		return err
	}
	for _, batch := range Batches {
		routes, err := batchRoutes(batch)
		if err != nil {
			return err
		}
		reads := 0
		for _, route := range routes {
			if route.Read() {
				reads++
			}
		}
		fmt.Printf("batch %d: %s\n  packages: %s\n  parity: %s\n  native-flagged routes: %d (%d read, %d write)\n",
			batch.Number, batch.Name, strings.Join(batch.Packages, ", "), strings.Join(batch.Parity, ", "),
			len(routes), reads, len(routes)-reads)
	}
	return nil
}
