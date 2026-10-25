package config

import (
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"

	"github.com/AnixOps/anix-control/v5/internal/requestorigin"
)

// TLS modes (tls.mode): who terminates TLS for the web side.
const (
	// TLSModeACME: Control obtains and renews a publicly trusted
	// certificate for domain over ACME and serves https itself.
	TLSModeACME = "acme"
	// TLSModeFiles: Control serves https itself with the certificate files
	// tls.cert_file and tls.key_file, reloaded when they change.
	TLSModeFiles = "files"
	// TLSModeExternal: a CDN or a reverse proxy terminates TLS and sends
	// plain http to listen.
	TLSModeExternal = "external"
)

// TLSModes lists the tls.mode values.
var TLSModes = []string{TLSModeACME, TLSModeFiles, TLSModeExternal}

// Where the gRPC listener's certificate comes from.
const (
	// GRPCCertPanel: the panel's own certificate, the gRPC name being domain.
	GRPCCertPanel = "panel"
	// GRPCCertACME: a certificate of its own for the gRPC name, over ACME.
	GRPCCertACME = "acme"
	// GRPCCertFiles: certificate files (grpc.tls_cert_file, or tls.cert_file
	// in tls.mode external), reloaded when they change.
	GRPCCertFiles = "files"
	// GRPCCertNone: no TLS. Only without any name to certify (no domain, no
	// grpc.public_name, no certificate files): development and tests. Agents
	// cannot connect to such a listener.
	GRPCCertNone = "none"
	// GRPCCertEdge: a reverse proxy named by grpc.edge_proxies terminates the
	// nodes' TLS with its own certificate; Control holds none for gRPC.
	GRPCCertEdge = "edge"
)

// Listener addresses Control owns in tls.mode acme and files, and the
// default plain address.
const (
	DefaultHTTPSAddress  = ":443"
	DefaultHTTPAddress   = ":80"
	DefaultListenAddress = "127.0.0.1:8080"
	DefaultGRPCPort      = 50051
)

// PanelSettings is the resolved listener and certificate layout of a
// Control: which addresses it binds, which names it holds certificates for
// and where they come from. Everything is derived from the configuration;
// nothing here touches the network.
type PanelSettings struct {
	// Mode is the effective tls.mode; ModeExplicit tells whether tls.mode
	// was set or derived (acme with domain, external without).
	Mode         string
	ModeExplicit bool
	// Domain is the panel's name (empty only in external mode).
	Domain string
	// HTTPSAddress and HTTPAddress are the https listener and the http one
	// (the ACME HTTP-01 answers and the redirect to https) of acme and files.
	HTTPSAddress string
	HTTPAddress  string
	// PlainAddresses are the plain http listeners of external mode: listen,
	// or the legacy server.host:server.port (and :frontend.port).
	PlainAddresses []string
	// ChallengeHTTPAddress and ChallengeTLSAddress are external mode's ACME
	// challenge listeners when Control obtains a certificate there (the
	// gRPC name's): port 80 for HTTP-01 and 443 for TLS-ALPN-01, bound when
	// free. Empty when a plain address already takes the port (it answers
	// HTTP-01 itself) or nothing is obtained over ACME.
	ChallengeHTTPAddress string
	ChallengeTLSAddress  string
	// WebCertFile and WebKeyFile are the web certificate of files mode.
	WebCertFile string
	WebKeyFile  string
	// GRPCEnabled tells whether the gRPC listener runs (grpc.enabled, always
	// with domain); GRPCAddress is where it binds.
	GRPCEnabled bool
	GRPCAddress string
	GRPCPort    int
	// GRPCName is the name nodes dial (grpc.public_name, else domain); empty
	// when there is none.
	GRPCName string
	// GRPCCertSource is one of the GRPCCert* values; GRPCCertFile and
	// GRPCKeyFile are the files of GRPCCertFiles.
	GRPCCertSource string
	GRPCCertFile   string
	GRPCKeyFile    string
	// GRPCEdge tells whether a reverse proxy terminates the nodes' TLS in
	// front of the gRPC listener (grpc.edge_proxies); GRPCEdgeProxies are
	// the proxy's addresses, the only peers the listener admits.
	GRPCEdge        bool
	GRPCEdgeProxies []string
	// PublicURL is the https address nodes and browsers use:
	// agent_install.public_url, else https://<domain>; empty when unknown
	// (then the request's origin is used).
	PublicURL string
	// GRPCTarget is the host:port nodes dial: agent_install.grpc_target,
	// else <GRPCName>:<grpc.port>; empty when unknown.
	GRPCTarget string
	// ACMENames are the names Control obtains certificates for over ACME,
	// in order (the panel's first).
	ACMENames []string
}

// NeedsACME tells whether Control obtains any certificate over ACME.
func (p PanelSettings) NeedsACME() bool { return len(p.ACMENames) > 0 }

// ServesWebTLS tells whether Control terminates https for the web side
// itself (acme and files modes).
func (p PanelSettings) ServesWebTLS() bool {
	return p.Mode == TLSModeACME || p.Mode == TLSModeFiles
}

// GRPCTLS tells whether the gRPC listener serves TLS.
func (p PanelSettings) GRPCTLS() bool {
	return p.GRPCEnabled && p.GRPCCertSource != GRPCCertNone
}

// normalizeHostName lower-cases a DNS name and drops a trailing dot.
func normalizeHostName(value string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(value)), ".")
}

// TLSModeOrDefault returns the effective tls.mode: the configured one, else
// acme when domain is set, external otherwise.
func (c *Config) TLSModeOrDefault() string {
	if mode := strings.ToLower(strings.TrimSpace(c.TLS.Mode)); mode != "" {
		return mode
	}
	if c.PanelDomain() != "" {
		return TLSModeACME
	}
	return TLSModeExternal
}

// PanelDomain returns domain, normalized. The older tls.domain is not read:
// it was only ever logged, and taking it now would move an existing
// configuration behind a reverse proxy to tls.mode acme on its own.
func (c *Config) PanelDomain() string {
	return normalizeHostName(c.Domain)
}

// Panel resolves the listener and certificate layout. It assumes a
// configuration that passed validation (load and ValidateForServer).
func (c *Config) Panel() PanelSettings {
	settings := PanelSettings{
		Mode:         c.TLSModeOrDefault(),
		ModeExplicit: strings.TrimSpace(c.TLS.Mode) != "",
		Domain:       c.PanelDomain(),
		GRPCPort:     c.GRPC.Port,
	}
	if settings.GRPCPort <= 0 {
		settings.GRPCPort = DefaultGRPCPort
	}
	switch settings.Mode {
	case TLSModeACME, TLSModeFiles:
		settings.HTTPSAddress, settings.HTTPAddress = DefaultHTTPSAddress, DefaultHTTPAddress
	default:
		settings.PlainAddresses = c.plainAddresses()
	}
	if settings.Mode == TLSModeFiles {
		settings.WebCertFile, settings.WebKeyFile = strings.TrimSpace(c.TLS.CertFile), strings.TrimSpace(c.TLS.KeyFile)
	}
	if settings.Mode == TLSModeACME {
		settings.ACMENames = append(settings.ACMENames, settings.Domain)
	}

	settings.GRPCEnabled = c.GRPC.Enable || settings.Domain != ""
	if len(c.GRPC.EdgeProxies) > 0 {
		settings.GRPCEnabled = true
		settings.GRPCEdge = true
		settings.GRPCEdgeProxies = append([]string(nil), c.GRPC.EdgeProxies...)
	}
	grpcHost := strings.TrimSpace(c.GRPC.Host)
	if grpcHost == "" {
		grpcHost = "0.0.0.0"
	}
	settings.GRPCAddress = net.JoinHostPort(strings.Trim(grpcHost, "[]"), strconv.Itoa(settings.GRPCPort))
	settings.GRPCName = normalizeHostName(c.GRPC.PublicName)
	if settings.GRPCName == "" {
		settings.GRPCName = settings.Domain
	}
	grpcCert, grpcKey := strings.TrimSpace(c.GRPC.TLSCertFile), strings.TrimSpace(c.GRPC.TLSKeyFile)
	webCert, webKey := strings.TrimSpace(c.TLS.CertFile), strings.TrimSpace(c.TLS.KeyFile)
	switch {
	case settings.GRPCEdge:
		settings.GRPCCertSource = GRPCCertEdge
	case grpcCert != "" && grpcKey != "":
		settings.GRPCCertSource, settings.GRPCCertFile, settings.GRPCKeyFile = GRPCCertFiles, grpcCert, grpcKey
	case settings.GRPCName == "":
		settings.GRPCCertSource = GRPCCertNone
	case settings.GRPCName == settings.Domain && settings.ServesWebTLS():
		settings.GRPCCertSource = GRPCCertPanel
	case settings.Mode == TLSModeExternal && webCert != "" && webKey != "":
		settings.GRPCCertSource, settings.GRPCCertFile, settings.GRPCKeyFile = GRPCCertFiles, webCert, webKey
	default:
		settings.GRPCCertSource = GRPCCertACME
		if settings.GRPCEnabled {
			settings.ACMENames = append(settings.ACMENames, settings.GRPCName)
		}
	}

	if settings.Mode == TLSModeExternal && settings.NeedsACME() {
		taken := map[string]bool{}
		for _, address := range settings.PlainAddresses {
			if _, port, err := net.SplitHostPort(address); err == nil {
				taken[port] = true
			}
		}
		if !taken["80"] {
			settings.ChallengeHTTPAddress = DefaultHTTPAddress
		}
		if !taken["443"] {
			settings.ChallengeTLSAddress = DefaultHTTPSAddress
		}
	}

	settings.PublicURL = strings.TrimRight(strings.TrimSpace(c.AgentInstall.PublicURL), "/")
	if settings.PublicURL == "" && settings.Domain != "" {
		settings.PublicURL = "https://" + settings.Domain
	}
	settings.GRPCTarget = strings.TrimSpace(c.AgentInstall.GRPCTarget)
	if settings.GRPCTarget == "" && settings.GRPCName != "" {
		settings.GRPCTarget = net.JoinHostPort(settings.GRPCName, strconv.Itoa(settings.GRPCPort))
	}
	return settings
}

// plainAddresses are external mode's plain http listeners: listen, or the
// legacy pair server.host:server.port and server.host:frontend.port (one
// handler on both; the second only with frontend.enable). An empty
// server.host is loopback: a plain listener on every address is a choice.
func (c *Config) plainAddresses() []string {
	if listen := strings.TrimSpace(c.Listen); listen != "" {
		return []string{listen}
	}
	host := strings.Trim(strings.TrimSpace(c.Server.Host), "[]")
	if host == "" {
		host = "127.0.0.1"
	}
	port := c.Server.Port
	if port <= 0 {
		port = 8080
	}
	addresses := []string{net.JoinHostPort(host, strconv.Itoa(port))}
	if c.Frontend.Enable && c.Frontend.Port > 0 && c.Frontend.Port != port {
		addresses = append(addresses, net.JoinHostPort(host, strconv.Itoa(c.Frontend.Port)))
	}
	return addresses
}

// validDNSName reports whether name is a DNS host name ACME can certify: at
// least two labels of letters, digits and dashes, no address.
func validDNSName(name string) bool {
	if name == "" || len(name) > 253 || net.ParseIP(name) != nil || !strings.Contains(name, ".") {
		return false
	}
	for _, label := range strings.Split(name, ".") {
		if label == "" || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return false
		}
		for _, r := range label {
			if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
				return false
			}
		}
	}
	return true
}

// validatePanel checks domain, listen, tls.* and grpc.public_name.
func (c *Config) validatePanel() error {
	mode := strings.ToLower(strings.TrimSpace(c.TLS.Mode))
	switch mode {
	case "", TLSModeACME, TLSModeFiles, TLSModeExternal:
	default:
		return fmt.Errorf("invalid tls.mode %q: use %s", c.TLS.Mode, strings.Join(TLSModes, ", "))
	}
	for key, value := range map[string]string{"domain": c.Domain, "grpc.public_name": c.GRPC.PublicName} {
		if name := normalizeHostName(value); name != "" && !validDNSName(name) {
			return fmt.Errorf("invalid %s %q: use a DNS name such as panel.example.com (no scheme, port or path, not an address)", key, value)
		}
	}
	effective := c.TLSModeOrDefault()
	if (effective == TLSModeACME || effective == TLSModeFiles) && c.PanelDomain() == "" {
		return fmt.Errorf("tls.mode %s needs domain: the name the panel is served at", effective)
	}
	if (strings.TrimSpace(c.TLS.CertFile) == "") != (strings.TrimSpace(c.TLS.KeyFile) == "") {
		return fmt.Errorf("tls.cert_file and tls.key_file go together")
	}
	if effective == TLSModeFiles && strings.TrimSpace(c.TLS.CertFile) == "" {
		return fmt.Errorf("tls.mode files needs tls.cert_file and tls.key_file")
	}
	if (strings.TrimSpace(c.GRPC.TLSCertFile) == "") != (strings.TrimSpace(c.GRPC.TLSKeyFile) == "") {
		return fmt.Errorf("grpc.tls_cert_file and grpc.tls_key_file go together")
	}
	if len(c.GRPC.EdgeProxies) > 0 {
		if _, err := requestorigin.NewPolicy(c.GRPC.EdgeProxies); err != nil {
			return fmt.Errorf("grpc.edge_proxies: %w", err)
		}
		if effective != TLSModeExternal {
			return fmt.Errorf("grpc.edge_proxies needs tls.mode external: the proxy terminates TLS for the panel and the nodes, and Control serves plain http behind it")
		}
		if strings.TrimSpace(c.GRPC.TLSCertFile) != "" || strings.TrimSpace(c.TLS.CertFile) != "" {
			return fmt.Errorf("grpc.edge_proxies excludes grpc.tls_cert_file and tls.cert_file: the public certificate stays on the proxy")
		}
	}
	if listen := strings.TrimSpace(c.Listen); listen != "" {
		host, port, err := net.SplitHostPort(listen)
		number, portErr := strconv.Atoi(port)
		if err != nil || portErr != nil || number < 1 || number > 65535 {
			return fmt.Errorf("invalid listen %q: use host:port, for example 127.0.0.1:8080", listen)
		}
		if host != "" && net.ParseIP(strings.Trim(host, "[]")) == nil && host != "localhost" {
			return fmt.Errorf("invalid listen %q: the host must be an address (or empty for every address)", listen)
		}
		if effective != TLSModeExternal {
			log.Printf("listen %q is ignored: tls.mode %s serves https on %s and http on %s", listen, effective, DefaultHTTPSAddress, DefaultHTTPAddress)
		}
	}
	if c.TLS.Enable {
		// It used to serve https on the web port with tls.cert_file: refuse
		// rather than serve that port plain from now on.
		if strings.TrimSpace(c.TLS.Mode) == "" && strings.TrimSpace(c.TLS.CertFile) != "" {
			return fmt.Errorf("tls.enable is no longer read: to serve https with tls.cert_file, set domain and tls.mode: files (Control then serves https on 443 and http on 80); behind a reverse proxy or CDN, remove tls.enable")
		}
		log.Printf("tls.enable is ignored: the web side's TLS follows tls.mode (%s)", effective)
	}
	if strings.TrimSpace(c.TLS.Domain) != "" && c.PanelDomain() == "" {
		log.Printf("tls.domain is not read: set domain to the panel's name (Control then serves it over https with its own certificate, tls.mode acme)")
	}
	return nil
}
