package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/panelcert"
	"gorm.io/gorm"
)

const tlsCommandUsage = `usage:
  anix-control tls status [-json]
  anix-control tls retry [-wait 90s]

status prints the certificates the running Control serves (the status it
stores in the database): their source, state, expiry, the last error and
its fix. retry asks the running Control to request the failing ones again
now, within its limit of three requests per name and hour, and waits for
the outcome.

The config file comes from ANIX_CONTROL_CONFIG or config/config.yaml.`

// errTLSNotValid: tls retry ended with a certificate that is still not
// valid.
var errTLSNotValid = errors.New("a panel certificate is not valid")

func tlsUsageError() error {
	return fmt.Errorf("invalid tls command\n%s", tlsCommandUsage)
}

// runTLSCommand runs "tls status" and "tls retry".
func runTLSCommand(ctx context.Context, db *gorm.DB, arguments []string, stdout io.Writer) error {
	if len(arguments) == 0 {
		return tlsUsageError()
	}
	flags := flag.NewFlagSet("tls "+arguments[0], flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print the status as JSON")
	wait := flags.Duration("wait", 90*time.Second, "how long retry waits for the outcome")
	if err := flags.Parse(arguments[1:]); err != nil || flags.NArg() != 0 {
		return tlsUsageError()
	}
	switch arguments[0] {
	case "status":
		status, err := panelcert.LoadStatus(ctx, db)
		if err != nil {
			return err
		}
		if *asJSON {
			encoder := json.NewEncoder(stdout)
			encoder.SetIndent("", "  ")
			return encoder.Encode(status)
		}
		writeTLSStatus(stdout, status)
		return nil
	case "retry":
		return runTLSRetry(ctx, db, *wait, stdout)
	}
	return tlsUsageError()
}

// runTLSRetry stores a retry request, waits for the running Control to
// pick it up and for the certificates to settle, and prints the status.
func runTLSRetry(ctx context.Context, db *gorm.DB, wait time.Duration, stdout io.Writer) error {
	requested := time.Now().UTC()
	if err := panelcert.RequestRetry(ctx, db, requested); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(stdout, "Asked the running Control to request the failing certificates again.")
	deadline := time.Now().Add(wait)
	pickedUp := false
	for time.Now().Before(deadline) {
		if !pickedUp {
			pending, err := panelcert.RetryPending(ctx, db)
			if err != nil {
				return err
			}
			pickedUp = !pending
		}
		if pickedUp {
			status, err := panelcert.LoadStatus(ctx, db)
			if err == nil && settledSince(status, requested) {
				writeTLSStatus(stdout, status)
				if status.State != panelcert.StateValid && status.State != panelcert.StateNotConfigured {
					return errTLSNotValid
				}
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	if !pickedUp {
		_, _ = fmt.Fprintln(stdout, "Control did not pick the request up: it does not seem to run. The request waits for its next start (systemctl status anix-control).")
		return errTLSNotValid
	}
	_, _ = fmt.Fprintf(stdout, "No outcome within %s; the request goes on. Check again with anix-control tls status.\n", wait)
	if status, err := panelcert.LoadStatus(ctx, db); err == nil {
		writeTLSStatus(stdout, status)
	}
	return errTLSNotValid
}

// settledSince tells whether every certificate is valid, or was tried
// after since and is not being tried any more.
func settledSince(status panelcert.Status, since time.Time) bool {
	if status.UpdatedAt.Before(since) {
		return false
	}
	for _, certificate := range status.Certificates {
		if certificate.State == panelcert.StateValid && !certificate.RenewalFailing {
			continue
		}
		if certificate.LastAttemptAt == nil || certificate.LastAttemptAt.Before(since) {
			return false
		}
		if certificate.State == panelcert.StatePending {
			return false
		}
	}
	return true
}

func formatTime(value *time.Time) string {
	if value == nil {
		return "-"
	}
	return value.UTC().Format(time.RFC3339)
}

// writeTLSStatus prints the status for a person.
func writeTLSStatus(w io.Writer, status panelcert.Status) {
	stored := "kept in the database"
	if !status.Persistent {
		stored = "NOT kept: module_runtime.ca_kek is not set, the certificates are requested again at every start"
	}
	_, _ = fmt.Fprintf(w, "Panel certificates: %s (tls.mode %s; status of %s; %s)\n", status.State, status.Mode, status.UpdatedAt.UTC().Format(time.RFC3339), stored)
	if status.Directory != "" {
		_, _ = fmt.Fprintf(w, "  ACME directory: %s (not the public CA's)\n", status.Directory)
	}
	if len(status.Certificates) == 0 {
		_, _ = fmt.Fprintln(w, "  Control manages no certificate: TLS is terminated in front of it and the gRPC listener has none.")
	}
	for _, certificate := range status.Certificates {
		_, _ = fmt.Fprintf(w, "  %s (%s, from %s): %s, serving %s", certificate.Name, strings.Join(certificate.Roles, ", "), certificate.Source, certificate.State, certificate.Serving)
		if certificate.State == panelcert.StateValid {
			trusted := "publicly trusted"
			if !certificate.PubliclyTrusted {
				trusted = "NOT publicly trusted"
			}
			_, _ = fmt.Fprintf(w, ", %s, issued by %s, valid until %s", trusted, certificate.Issuer, formatTime(certificate.NotAfter))
			if certificate.RenewAt != nil {
				_, _ = fmt.Fprintf(w, ", renewal from %s", formatTime(certificate.RenewAt))
			}
		}
		_, _ = fmt.Fprintln(w)
		if certificate.RenewalFailing {
			_, _ = fmt.Fprintln(w, "    the renewal is failing")
		}
		if certificate.LastError != "" {
			_, _ = fmt.Fprintf(w, "    error: %s\n", certificate.LastError)
		}
		if certificate.Fix != "" {
			_, _ = fmt.Fprintf(w, "    fix:   %s\n", certificate.Fix)
		}
		if certificate.NextAttemptAt != nil && certificate.State != panelcert.StateValid {
			_, _ = fmt.Fprintf(w, "    next request: %s\n", formatTime(certificate.NextAttemptAt))
		}
	}
	for _, listener := range status.Listeners {
		_, _ = fmt.Fprintf(w, "  %s on %s is not bound: %s\n", listener.Purpose, listener.Address, listener.Error)
	}
}
