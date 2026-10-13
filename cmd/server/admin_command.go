package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/adminclaim"
	"github.com/AnixOps/anix-control/v5/internal/config"
	"github.com/AnixOps/anix-control/v5/internal/panelcert"
	"gorm.io/gorm"
)

const adminCommandUsage = `usage:
  anix-control admin claim-link [-ttl 24h]

Prints the one-time link that creates the panel's first administrator,
https://<domain>/claim#anixclaim_... Open it, choose the administrator's
email and password, and sign in. The link works once and for -ttl (at most
24 hours); running the command again replaces it. It is printed here only,
never logged. Once the panel has an administrator the command refuses with
exit status 1.

The config file comes from ANIX_CONTROL_CONFIG or config/config.yaml.`

// administratorNotice is the line a start without an administrator logs.
const administratorNotice = "No administrator yet. Run `sudo anix-control admin claim-link` to get the claim link."

// claimLinkEnvironment is what claim-link reads besides the configuration
// and the database; tests replace it.
type claimLinkEnvironment struct {
	now func() time.Time
	// loadStatus reads the certificate status the running Control stored
	// (what GET /api/v5/panel/status answers).
	loadStatus     func(ctx context.Context, db *gorm.DB) (panelcert.Status, error)
	localAddresses func() []net.IP
}

func defaultClaimLinkEnvironment() claimLinkEnvironment {
	return claimLinkEnvironment{now: time.Now, loadStatus: panelcert.LoadStatus, localAddresses: panelcert.LocalAddresses}
}

// runAdminCommand's "admin" family.
func runAdminClaimCommand(ctx context.Context, cfg *config.Config, db *gorm.DB, arguments []string, stdout io.Writer) error {
	return runAdminClaimCommandWith(ctx, cfg, db, arguments, stdout, defaultClaimLinkEnvironment())
}

func runAdminClaimCommandWith(ctx context.Context, cfg *config.Config, db *gorm.DB, arguments []string, stdout io.Writer, env claimLinkEnvironment) error {
	if len(arguments) == 0 || arguments[0] != "claim-link" {
		return fmt.Errorf("invalid admin command\n%s", adminCommandUsage)
	}
	flags := flag.NewFlagSet("admin claim-link", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	ttl := flags.Duration("ttl", adminclaim.DefaultTTL, "how long the link works")
	if err := flags.Parse(arguments[1:]); err != nil || flags.NArg() != 0 {
		return fmt.Errorf("invalid admin claim-link command\n%s", adminCommandUsage)
	}
	panel := cfg.Panel()
	base, known := panelBaseURL(panel)
	issued, err := adminclaim.Issue(ctx, db, *ttl, env.now())
	if errors.Is(err, adminclaim.ErrAdministratorExists) {
		return fmt.Errorf("%w: a claim link only creates the first one. Sign in at %s/login; an administrator adds others under Users", err, base)
	}
	if err != nil {
		return err
	}
	fragment := "/claim#" + issued.Token
	var out strings.Builder
	fmt.Fprintf(&out, "Open this link within %s to create the panel's first administrator. It works once.\n\n", describeTTL(*ttl))
	if known {
		fmt.Fprintf(&out, "  %s%s\n\n", base, fragment)
	} else {
		fmt.Fprintf(&out, "  <the panel's address>%s\n\n", fragment)
		fmt.Fprintf(&out, "domain is not set, so Control does not know the panel's address: put the address your\nreverse proxy serves (https://panel.example.com) in front of /claim. On this machine it is:\n\n")
		fmt.Fprintf(&out, "  %s%s\n\n", base, fragment)
	}
	if fallback := untrustedFallback(ctx, db, panel, env); fallback != "" {
		fmt.Fprintf(&out, "The panel's certificate is not publicly trusted yet, so the browser warns. Until it is,\nthis address works too:\n\n")
		fmt.Fprintf(&out, "  %s%s\n\n", fallback, fragment)
	}
	fmt.Fprintf(&out, "The link expires at %s. Running `anix-control admin claim-link` again replaces it.\n", issued.ExpiresAt.UTC().Format("2006-01-02 15:04 UTC"))
	_, err = io.WriteString(stdout, out.String())
	return err
}

// panelBaseURL is the panel's address without a trailing slash, and
// whether it is the public one: https://<domain> (or
// agent_install.public_url), else the first plain listener of external mode
// as this machine reaches it.
func panelBaseURL(panel config.PanelSettings) (string, bool) {
	if public := strings.TrimRight(panel.PublicURL, "/"); public != "" {
		return public, true
	}
	if len(panel.PlainAddresses) > 0 {
		host, port, err := net.SplitHostPort(panel.PlainAddresses[0])
		if err == nil {
			if ip := net.ParseIP(host); host == "" || (ip != nil && ip.IsUnspecified()) {
				host = "127.0.0.1"
			}
			return "http://" + net.JoinHostPort(host, port), false
		}
	}
	return "http://127.0.0.1:8080", false
}

// untrustedFallback is https://<this machine's first public address> while
// Control serves the panel's TLS itself and its web certificate is not
// publicly trusted (not obtained yet, or a self-signed fallback, which
// names the machine's addresses); "" otherwise or when no public address is
// known.
func untrustedFallback(ctx context.Context, db *gorm.DB, panel config.PanelSettings, env claimLinkEnvironment) string {
	if !panel.ServesWebTLS() {
		return ""
	}
	if status, err := env.loadStatus(ctx, db); err == nil {
		if web, ok := status.Certificate(panelcert.RoleWeb); ok && web.PubliclyTrusted {
			return ""
		}
	}
	address := firstPublicAddress(env.localAddresses())
	if address == nil {
		return ""
	}
	return (&url.URL{Scheme: "https", Host: hostLiteral(address)}).String()
}

// firstPublicAddress prefers IPv4; private and shared ranges do not count.
func firstPublicAddress(addresses []net.IP) net.IP {
	var v6 net.IP
	for _, address := range addresses {
		if !address.IsGlobalUnicast() || address.IsPrivate() || sharedAddressSpace.Contains(address) {
			continue
		}
		if address.To4() != nil {
			return address
		}
		if v6 == nil {
			v6 = address
		}
	}
	return v6
}

// sharedAddressSpace is 100.64.0.0/10 (carrier-grade NAT), which
// net.IP.IsPrivate does not cover.
var sharedAddressSpace = &net.IPNet{IP: net.IPv4(100, 64, 0, 0), Mask: net.CIDRMask(10, 32)}

func hostLiteral(address net.IP) string {
	if address.To4() != nil {
		return address.String()
	}
	return "[" + address.String() + "]"
}

// describeTTL is the link's lifetime in words.
func describeTTL(ttl time.Duration) string {
	plural := func(n int64, unit string) string {
		if n == 1 {
			return "1 " + unit
		}
		return fmt.Sprintf("%d %ss", n, unit)
	}
	switch {
	case ttl%time.Hour == 0:
		return plural(int64(ttl/time.Hour), "hour")
	case ttl%time.Minute == 0:
		return plural(int64(ttl/time.Minute), "minute")
	}
	return ttl.String()
}

// noteMissingAdministrator logs, once per start, that the panel has no
// administrator yet and how to get the claim link. It never issues one.
func noteMissingAdministrator(ctx context.Context, db *gorm.DB, logf func(string, ...any)) {
	exists, err := adminclaim.HasAdministrator(ctx, db)
	if err != nil {
		logf("Could not check whether the panel has an administrator: %v", err)
		return
	}
	if !exists {
		logf("%s", administratorNotice)
	}
}
