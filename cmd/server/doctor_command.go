package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/config"
	"github.com/AnixOps/anix-control/v5/internal/modulepki"
	"github.com/AnixOps/anix-control/v5/internal/panelcert"
	"gorm.io/gorm"
)

const doctorCommandUsage = `usage:
  anix-control doctor [-json]

Checks what a working panel needs and prints a fix for each problem: the
configuration, the root key, the certificates the running Control serves,
the DNS records of the panel's and the nodes' names compared with this
machine's addresses (a name proxied by a CDN shows here), and the ports.
It changes nothing. The exit status is 1 when a check fails.`

// Doctor check levels.
const (
	doctorOK   = "ok"
	doctorWarn = "warn"
	doctorFail = "fail"
	doctorInfo = "info"
)

// errDoctorFailed: at least one check failed.
var errDoctorFailed = errors.New("doctor found problems")

// doctorCheck is one finding.
type doctorCheck struct {
	ID      string `json:"id"`
	Level   string `json:"level"`
	Message string `json:"message"`
	Fix     string `json:"fix,omitempty"`
}

// doctorEnvironment is what the checks read besides the configuration and
// the database; tests replace it.
type doctorEnvironment struct {
	resolver       panelcert.Resolver
	localAddresses func() []net.IP
	checkPort      func(address string) (string, string)
	// probeTLS dials address with the server name and returns the served
	// leaf's names (Control answers with its certificate).
	probeTLS func(address, serverName string) ([]string, error)
}

func defaultDoctorEnvironment() doctorEnvironment {
	return doctorEnvironment{
		resolver: net.DefaultResolver, localAddresses: panelcert.LocalAddresses, checkPort: panelcert.CheckPort,
		probeTLS: func(address, serverName string) ([]string, error) {
			dialer := &net.Dialer{Timeout: 3 * time.Second}
			// #nosec G402 -- the probe reads which certificate is served; it sends nothing.
			conn, err := tls.DialWithDialer(dialer, "tcp", address, &tls.Config{ServerName: serverName, InsecureSkipVerify: true, MinVersion: tls.VersionTLS12})
			if err != nil {
				return nil, err
			}
			defer func() { _ = conn.Close() }()
			leaf := conn.ConnectionState().PeerCertificates[0]
			return leaf.DNSNames, nil
		},
	}
}

// runDoctorCommand runs "doctor".
func runDoctorCommand(ctx context.Context, cfg *config.Config, db *gorm.DB, arguments []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("doctor", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print the checks as JSON")
	if err := flags.Parse(arguments); err != nil || flags.NArg() != 0 {
		return fmt.Errorf("invalid doctor command\n%s", doctorCommandUsage)
	}
	checks := doctorChecks(ctx, cfg, db, defaultDoctorEnvironment())
	if *asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(map[string]any{"checks": checks}); err != nil {
			return err
		}
	} else {
		for _, check := range checks {
			_, _ = fmt.Fprintf(stdout, "[%-4s] %s\n", check.Level, check.Message)
			if check.Fix != "" {
				_, _ = fmt.Fprintf(stdout, "       fix: %s\n", check.Fix)
			}
		}
	}
	for _, check := range checks {
		if check.Level == doctorFail {
			return errDoctorFailed
		}
	}
	return nil
}

// doctorChecks runs every check.
func doctorChecks(ctx context.Context, cfg *config.Config, db *gorm.DB, env doctorEnvironment) []doctorCheck {
	var checks []doctorCheck
	add := func(id, level, message, fix string) {
		checks = append(checks, doctorCheck{ID: id, Level: level, Message: message, Fix: fix})
	}
	panel := cfg.Panel()

	// Configuration.
	if err := cfg.ValidateForServer(); err != nil {
		add("config", doctorFail, "The configuration does not start a server: "+err.Error(), "Fix the key named above in the config file (or its ANIX_CONTROL_* variable).")
	} else {
		add("config", doctorOK, describeLayout(panel), "")
	}
	if panel.Mode == config.TLSModeExternal && panel.Domain == "" {
		add("domain", doctorWarn, "domain is not set: Control does not know its own name, so nodes get no install address and the gRPC listener has no certificate.",
			"Set domain to the panel's name (one line in the config file: domain: panel.example.com).")
	}

	// The root key.
	if raw := strings.TrimSpace(cfg.ModuleRuntime.CAKEK); raw == "" {
		add("root_key", doctorFail, "module_runtime.ca_kek is not set: there is no CA for node certificates, node group tokens cannot be sealed and ACME keys cannot be stored.",
			"Set module_runtime.ca_kek to a 32-byte key (openssl rand -base64 32) and restart Control.")
	} else if _, err := modulepki.ParseKEK(raw); err != nil {
		add("root_key", doctorFail, "module_runtime.ca_kek cannot be used: "+err.Error(), "Set module_runtime.ca_kek to 32 bytes, base64 or hex.")
	} else {
		add("root_key", doctorOK, "module_runtime.ca_kek is set.", "")
	}

	// Certificates, as the running Control last stored them.
	status, err := panelcert.LoadStatus(ctx, db)
	switch {
	case errors.Is(err, panelcert.ErrNoStatus):
		add("certificates", doctorWarn, "Control has not stored a certificate status in this database: it has not run with this configuration yet.",
			"Start Control (systemctl start anix-control), then run anix-control doctor again.")
	case err != nil:
		add("certificates", doctorFail, "The certificate status cannot be read: "+err.Error(), "Check the database.")
	default:
		if status.Mode != panel.Mode || status.Domain != panel.Domain {
			add("certificates", doctorWarn, fmt.Sprintf("The running Control was started with tls.mode %s for %q; the configuration says %s for %q.", status.Mode, status.Domain, panel.Mode, panel.Domain),
				"Restart Control to apply the configuration.")
		}
		if len(status.Certificates) == 0 {
			add("certificates", doctorInfo, "Control manages no certificate (TLS is terminated in front of it, and the gRPC listener has none).", "")
		}
		for _, certificate := range status.Certificates {
			id := "certificate:" + certificate.Name
			label := fmt.Sprintf("The certificate for %s (%s, from %s)", certificate.Name, strings.Join(certificate.Roles, ", "), certificate.Source)
			switch {
			case certificate.State == panelcert.StateValid && certificate.PubliclyTrusted && !certificate.RenewalFailing:
				add(id, doctorOK, fmt.Sprintf("%s is valid until %s, issued by %s.", label, formatTime(certificate.NotAfter), certificate.Issuer), "")
			case certificate.State == panelcert.StateValid && certificate.RenewalFailing:
				add(id, doctorWarn, fmt.Sprintf("%s is valid until %s, but its renewal fails: %s", label, formatTime(certificate.NotAfter), certificate.LastError), certificate.Fix)
			case certificate.State == panelcert.StateValid:
				add(id, doctorFail, fmt.Sprintf("%s is valid but does not verify against the system CAs, which nodes and browsers use: %s", label, certificate.LastError), certificate.Fix)
			case certificate.State == panelcert.StatePending:
				add(id, doctorWarn, label+" is being requested; Control serves a self-signed certificate until then.", "Wait a minute, then run anix-control tls status.")
			default:
				add(id, doctorFail, fmt.Sprintf("%s is not usable (Control serves a self-signed one, which nodes refuse): %s", label, certificate.LastError), certificate.Fix)
			}
		}
		for _, listener := range status.Listeners {
			add("listener:"+listener.Address, doctorWarn, fmt.Sprintf("The %s on %s is not bound: %s.", listener.Purpose, listener.Address, listener.Error),
				fmt.Sprintf("ACME challenges for %s must reach Control another way: forward /.well-known/acme-challenge/ for it to Control's listen address, or free the port.", panel.GRPCName))
		}
	}

	// DNS.
	local := env.localAddresses()
	// cdnOK: the name may resolve elsewhere (the panel's name in external
	// mode sits behind a CDN or a proxy); a name nodes dial may not.
	type dnsName struct {
		id, name string
		cdnOK    bool
	}
	var names []dnsName
	if panel.Domain != "" {
		names = append(names, dnsName{"dns:panel", panel.Domain, panel.Mode == config.TLSModeExternal && panel.GRPCName != panel.Domain})
	}
	if panel.GRPCEnabled && panel.GRPCName != "" && panel.GRPCName != panel.Domain {
		names = append(names, dnsName{"dns:grpc", panel.GRPCName, false})
	}
	for _, entry := range names {
		report := panelcert.DiagnoseDNS(ctx, env.resolver, entry.name, local)
		role := "the panel's name"
		if entry.id == "dns:grpc" {
			role = "the name nodes dial for gRPC (grpc.public_name)"
		} else if panel.GRPCEnabled && panel.GRPCName == panel.Domain {
			role = "the panel's name, which nodes also dial for gRPC"
		}
		switch report.Verdict {
		case panelcert.DNSOK:
			add(entry.id, doctorOK, fmt.Sprintf("%s (%s): %s", entry.name, role, report.Hint), "")
		case panelcert.DNSUnresolved:
			add(entry.id, doctorFail, fmt.Sprintf("%s (%s) does not resolve.", entry.name, role), report.Hint)
		case panelcert.DNSElsewhere:
			if entry.cdnOK {
				add(entry.id, doctorInfo, fmt.Sprintf("%s (%s) resolves to %s, not this machine: expected with tls.mode external behind a CDN or a proxy on another machine.", entry.name, role, strings.Join(report.Addresses, ", ")), "")
				continue
			}
			add(entry.id, doctorFail, fmt.Sprintf("%s (%s): %s", entry.name, role, report.Hint),
				"Nodes connect straight to this machine and verify its certificate, which a CDN cannot pass through: point the name here as DNS only. With the panel behind a CDN, set tls.mode external and grpc.public_name to a DNS-only name.")
		default:
			add(entry.id, doctorWarn, fmt.Sprintf("%s (%s): %s", entry.name, role, report.Hint), "")
		}
	}

	// The CDN mode: the install script asks /install/ip through the CDN.
	if panel.Mode == config.TLSModeExternal && panel.GRPCName != "" && panel.GRPCName != panel.Domain && panel.Domain != "" {
		add("cdn:client_address", doctorInfo, "With the panel behind a CDN, nodes read their own address from https://"+panel.Domain+"/install/ip through the CDN: Control takes the address from X-Forwarded-For only when the connecting peer is in server.trusted_proxies.",
			"List the local proxy and the CDN's published address ranges in server.trusted_proxies; otherwise /install/ip answers the CDN's address, which the join records for the node.")
	}

	// Ports.
	type portCheck struct{ purpose, address, probeName string }
	var ports []portCheck
	switch panel.Mode {
	case config.TLSModeACME, config.TLSModeFiles:
		ports = append(ports, portCheck{"https", panel.HTTPSAddress, panel.Domain}, portCheck{"http (ACME HTTP-01, redirect)", panel.HTTPAddress, ""})
	default:
		for _, address := range panel.PlainAddresses {
			ports = append(ports, portCheck{"plain http (listen)", address, ""})
		}
		if panel.ChallengeHTTPAddress != "" {
			ports = append(ports, portCheck{"ACME HTTP-01", panel.ChallengeHTTPAddress, ""})
		}
	}
	if panel.GRPCEnabled {
		ports = append(ports, portCheck{"node gRPC", panel.GRPCAddress, panel.GRPCName})
	}
	for _, port := range ports {
		state, detail := env.checkPort(port.address)
		id := "port:" + port.address
		switch state {
		case panelcert.PortInUse:
			message := fmt.Sprintf("%s on %s is in use (by Control when it runs).", port.purpose, port.address)
			if port.probeName != "" && env.probeTLS != nil {
				probe := loopbackAddress(port.address)
				if names, err := env.probeTLS(probe, port.probeName); err != nil {
					add(id, doctorWarn, fmt.Sprintf("%s on %s is in use, but a TLS handshake for %s fails there: %v. Another program may hold the port.", port.purpose, port.address, port.probeName, err),
						fmt.Sprintf("Find the holder with: ss -ltnp 'sport = :%s'", portOf(port.address)))
					continue
				} else {
					message = fmt.Sprintf("%s on %s answers with a certificate for %s.", port.purpose, port.address, strings.Join(names, ", "))
				}
			}
			add(id, doctorOK, message, "")
		case panelcert.PortFree:
			add(id, doctorWarn, fmt.Sprintf("Nothing listens on %s (%s): Control is not running.", port.address, port.purpose),
				"Start Control: systemctl start anix-control (journalctl -u anix-control says why it stopped).")
		default:
			add(id, doctorWarn, fmt.Sprintf("%s on %s cannot be checked: %s.", port.purpose, port.address, detail),
				fmt.Sprintf("Run doctor as root, or check with: ss -ltnp 'sport = :%s'", portOf(port.address)))
		}
	}
	if panel.ServesWebTLS() || panel.ChallengeHTTPAddress != "" || panel.NeedsACME() {
		add("firewall", doctorInfo, "Whether ports 80 and 443 (and the gRPC port) are open from the internet cannot be seen from this machine.",
			"Check the firewall and the provider's security group; the CA reaches port 80 or 443 of the names above from outside.")
	}
	return checks
}

// describeLayout is the configuration line of doctor.
func describeLayout(panel config.PanelSettings) string {
	var parts []string
	switch panel.Mode {
	case config.TLSModeACME:
		parts = append(parts, fmt.Sprintf("tls.mode acme: Control obtains the certificate for %s and serves https on %s, http on %s", panel.Domain, panel.HTTPSAddress, panel.HTTPAddress))
	case config.TLSModeFiles:
		parts = append(parts, fmt.Sprintf("tls.mode files: Control serves https for %s on %s with %s", panel.Domain, panel.HTTPSAddress, panel.WebCertFile))
	default:
		parts = append(parts, fmt.Sprintf("tls.mode external: plain http on %s, TLS terminated in front of Control", strings.Join(panel.PlainAddresses, ", ")))
	}
	if panel.GRPCEnabled {
		parts = append(parts, fmt.Sprintf("nodes dial gRPC at %s (%s)", panel.GRPCTarget, describeGRPCTLS(panel)))
	} else {
		parts = append(parts, "the gRPC listener is off")
	}
	if panel.PublicURL != "" {
		parts = append(parts, "the install script is "+panel.PublicURL+"/install.sh")
	}
	return strings.Join(parts, "; ") + "."
}

// loopbackAddress is address with an unspecified host made loopback.
func loopbackAddress(address string) string {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return address
	}
	if ip := net.ParseIP(host); host == "" || (ip != nil && ip.IsUnspecified()) {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port)
}

func portOf(address string) string {
	_, port, err := net.SplitHostPort(address)
	if err != nil {
		return address
	}
	return port
}
