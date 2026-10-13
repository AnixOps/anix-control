package panelcert

import (
	"context"
	"errors"
	"fmt"
	"net"
	"slices"
	"strings"
	"syscall"
	"time"
)

// Resolver resolves a name's addresses (net.DefaultResolver).
type Resolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}

// DNS verdicts of DiagnoseDNS.
const (
	// DNSOK: the name resolves to an address of this machine.
	DNSOK = "ok"
	// DNSPartial: some of its addresses are this machine's, others are
	// not (often a stale AAAA record): a CA may use any of them.
	DNSPartial = "partial"
	// DNSUnresolved: no A or AAAA record.
	DNSUnresolved = "unresolved"
	// DNSElsewhere: no address is this machine's, and this machine has a
	// public address of its own: the record points elsewhere or a CDN
	// proxies the name.
	DNSElsewhere = "elsewhere"
	// DNSUnknown: no address is on this machine's interfaces, but every
	// interface address is private (NAT, as with most cloud providers):
	// whether the public address is this machine's cannot be told from
	// here without a network guess.
	DNSUnknown = "unknown"
)

// DNSReport is what the name's records say, compared with this machine's
// interface addresses. No network call besides the DNS lookup.
type DNSReport struct {
	Name      string   `json:"name"`
	Addresses []string `json:"addresses"`
	// Local are the addresses that are this machine's.
	Local   []string `json:"local"`
	Verdict string   `json:"verdict"`
	Error   string   `json:"error,omitempty"`
	// Hint says what to check, in English.
	Hint string `json:"hint"`
}

// publicAddress tells whether ip is reachable from the internet in
// principle: global unicast and not private, shared or link-local.
func publicAddress(ip net.IP) bool {
	if !ip.IsGlobalUnicast() || ip.IsPrivate() {
		return false
	}
	// 100.64.0.0/10, carrier-grade NAT.
	if v4 := ip.To4(); v4 != nil && v4[0] == 100 && v4[1]&0xc0 == 64 {
		return false
	}
	return true
}

// DiagnoseDNS looks name up and compares its addresses with local (this
// machine's interface addresses).
func DiagnoseDNS(ctx context.Context, resolver Resolver, name string, local []net.IP) DNSReport {
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	report := DNSReport{Name: name, Addresses: []string{}, Local: []string{}}
	lookupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	addresses, err := resolver.LookupIPAddr(lookupCtx, name)
	if err != nil || len(addresses) == 0 {
		report.Verdict = DNSUnresolved
		if err != nil {
			report.Error = err.Error()
		}
		report.Hint = fmt.Sprintf("%s has no A or AAAA record (as this machine's resolver sees it). Create an A record (and an AAAA record if this machine has IPv6) pointing at this machine, without a CDN proxy.", name)
		return report
	}
	hostPublic := slices.ContainsFunc(local, publicAddress)
	var elsewhere []string
	for _, address := range addresses {
		report.Addresses = append(report.Addresses, address.IP.String())
		if slices.ContainsFunc(local, address.IP.Equal) {
			report.Local = append(report.Local, address.IP.String())
		} else {
			elsewhere = append(elsewhere, address.IP.String())
		}
	}
	switch {
	case len(elsewhere) == 0:
		report.Verdict = DNSOK
		report.Hint = fmt.Sprintf("%s resolves to this machine (%s).", name, strings.Join(report.Local, ", "))
	case len(report.Local) > 0:
		report.Verdict = DNSPartial
		report.Hint = fmt.Sprintf("%s resolves to this machine (%s) and also to %s, which is not an address of this machine. A CA may validate over any of them (IPv6 first): fix or remove the other records.", name, strings.Join(report.Local, ", "), strings.Join(elsewhere, ", "))
	case hostPublic:
		report.Verdict = DNSElsewhere
		report.Hint = fmt.Sprintf("%s resolves to %s, which is not an address of this machine: the record points at another machine, or a CDN proxies the name (its addresses then belong to the CDN). Point it at this machine with the CDN proxy off; if the panel must stay behind the CDN, use tls.mode external and set grpc.public_name to a DNS-only name for the nodes.", name, strings.Join(elsewhere, ", "))
	default:
		report.Verdict = DNSUnknown
		report.Hint = fmt.Sprintf("%s resolves to %s. This machine has only private addresses (it sits behind NAT), so Control cannot tell from here whether that is its public address. Check in the provider's console that %s is this machine's public address, and that the name is DNS-only (a CDN proxy shows addresses that are not yours).", name, strings.Join(elsewhere, ", "), strings.Join(elsewhere, ", "))
	}
	return report
}

// Port states of CheckPort.
const (
	PortFree  = "free"
	PortInUse = "in_use"
	PortError = "error"
)

// CheckPort tries to bind address and releases it at once: free, in use
// (by Control itself while it runs, or by another program) or another
// error (a privileged port without the capability).
func CheckPort(address string) (string, string) {
	listener, err := net.Listen("tcp", address)
	if err == nil {
		_ = listener.Close()
		return PortFree, ""
	}
	if errors.Is(err, syscall.EADDRINUSE) {
		return PortInUse, err.Error()
	}
	return PortError, err.Error()
}

// fixFor is the English fix of a problem.
func fixFor(problem, name string, spec Spec, directory string) string {
	switch problem {
	case ProblemDNSUnresolved:
		return fmt.Sprintf("Create an A (and AAAA) record for %s pointing at this machine, with no CDN proxy, then run anix-control tls retry.", name)
	case ProblemDNSElsewhere:
		return fmt.Sprintf("%s does not resolve to this machine: point it here with the CDN proxy off. With the panel behind a CDN, use tls.mode external and set grpc.public_name to a DNS-only name that resolves here.", name)
	case ProblemUnreachable:
		return fmt.Sprintf("The CA could not connect to %s on port 443 (TLS-ALPN-01) nor on port 80 (HTTP-01): open both in the firewall and the provider's security group, and stop any other program that holds them.", name)
	case ProblemChallenge:
		return fmt.Sprintf("The CA reached %s but did not get Control's answer: the name points elsewhere, a CDN or another web server answers for it. Point it here with the proxy off, or forward /.well-known/acme-challenge/ for it to Control.", name)
	case ProblemRateLimited:
		return "The CA's rate limit was reached. Control tries again later by itself; do not reinstall (the certificate and the account are kept in the database)."
	case ProblemCAUnreachable:
		return fmt.Sprintf("Control cannot reach the ACME directory %s: check this machine's outbound https (port 443) and DNS.", directory)
	case ProblemQuota:
		return "Control made 3 requests for this name within the hour, the most it makes; it tries again when the hour has passed."
	case ProblemFileUnreadable:
		return fmt.Sprintf("Check that %s and %s exist, can be read by Control and hold a PEM certificate chain and its private key; Control reloads them when they change.", spec.CertFile, spec.KeyFile)
	case ProblemExpired:
		if spec.Source == SourceFiles {
			return fmt.Sprintf("The certificate in %s has ended: replace the files with a renewed certificate (Control reloads them).", spec.CertFile)
		}
		return "The certificate ended without being renewed; see the error above, then run anix-control tls retry."
	case ProblemNameMismatch:
		return fmt.Sprintf("The certificate in %s does not cover %s: use a certificate issued for that name.", spec.CertFile, name)
	case ProblemUntrusted:
		return fmt.Sprintf("The certificate for %s does not verify against the system CAs. Nodes (and browsers) accept only publicly trusted certificates: use one from a public CA.", name)
	}
	return "See the error; Control tries again by itself (anix-control tls retry tries now)."
}
