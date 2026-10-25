package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/config"
	"github.com/AnixOps/anix-control/v5/internal/modulepki"
	"github.com/AnixOps/anix-control/v5/internal/panelcert"
	"github.com/AnixOps/anix-control/v5/internal/requestorigin"
	"github.com/AnixOps/anix-control/v5/internal/router"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// newPanelCertificates builds the certificate manager of the layout:
// the panel's certificate in tls.mode acme and files, and the gRPC
// listener's when it is not the panel's. It loads what is stored (and
// makes the self-signed fallback) but requests nothing yet.
func newPanelCertificates(ctx context.Context, cfg *config.Config, panel config.PanelSettings, db *gorm.DB) (*panelcert.Manager, error) {
	var specs []panelcert.Spec
	webRoles := []string{panelcert.RoleWeb}
	if panel.GRPCEnabled && panel.GRPCCertSource == config.GRPCCertPanel {
		webRoles = append(webRoles, panelcert.RoleGRPC)
	}
	switch panel.Mode {
	case config.TLSModeACME:
		specs = append(specs, panelcert.Spec{Name: panel.Domain, Roles: webRoles, Source: panelcert.SourceACME})
	case config.TLSModeFiles:
		specs = append(specs, panelcert.Spec{Name: panel.Domain, Roles: webRoles, Source: panelcert.SourceFiles,
			CertFile: panel.WebCertFile, KeyFile: panel.WebKeyFile})
	}
	if panel.GRPCEnabled {
		grpcRoles := []string{panelcert.RoleGRPC}
		switch panel.GRPCCertSource {
		case config.GRPCCertACME:
			specs = append(specs, panelcert.Spec{Name: panel.GRPCName, Roles: grpcRoles, Source: panelcert.SourceACME})
		case config.GRPCCertFiles:
			name := panel.GRPCName
			if name == panel.Domain && panel.ServesWebTLS() {
				// The panel's certificate serves the web side; the gRPC
				// files are another certificate for the same name.
				name = ""
			}
			specs = append(specs, panelcert.Spec{Name: name, Roles: grpcRoles, Source: panelcert.SourceFiles,
				CertFile: panel.GRPCCertFile, KeyFile: panel.GRPCKeyFile})
		}
	}
	opts := panelcert.Options{
		DB: db, Mode: panel.Mode, Domain: panel.Domain, GRPCName: panel.GRPCName, Certificates: specs,
		DirectoryURL: strings.TrimSpace(os.Getenv(config.ACMEDirectoryEnv)),
	}
	if raw := strings.TrimSpace(cfg.ModuleRuntime.CAKEK); raw != "" {
		kek, err := modulepki.ParseKEK(raw)
		if err != nil {
			return nil, fmt.Errorf("module_runtime.ca_kek: %w", err)
		}
		opts.KEK = kek
	}
	if file := strings.TrimSpace(os.Getenv(config.ACMECAFileEnv)); file != "" {
		client, err := acmeHTTPClient(file)
		if err != nil {
			return nil, err
		}
		opts.HTTPClient = client
	}
	if tunePanelCertificates != nil {
		tunePanelCertificates(&opts)
	}
	manager, err := panelcert.New(opts)
	if err != nil {
		return nil, err
	}
	if err := manager.Start(ctx); err != nil {
		return nil, err
	}
	return manager, nil
}

// tunePanelCertificates adjusts the certificate options in tests (a test
// CA's roots, faster file checks); nil in Control.
var tunePanelCertificates func(*panelcert.Options)

// acmeHTTPClient trusts the extra roots of file for the ACME directory
// (a test CA in CI) besides the system roots.
func acmeHTTPClient(file string) (*http.Client, error) {
	pem, err := os.ReadFile(filepath.Clean(file)) // #nosec G703 -- the path is the operator's own environment variable (a test CA in CI), never request data.
	if err != nil {
		return nil, fmt.Errorf("%s: %w", config.ACMECAFileEnv, err)
	}
	roots, err := x509.SystemCertPool()
	if err != nil || roots == nil {
		roots = x509.NewCertPool()
	}
	if !roots.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("%s: %s holds no PEM certificate", config.ACMECAFileEnv, file)
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
	return &http.Client{Transport: transport, Timeout: time.Minute}, nil
}

// newPanelHandler is the panel's one HTTP handler: the API, the
// subscription and install paths, the health probes, /metrics and, with
// frontend.enable, the web app (frontend.path) with its SPA fallback. A
// path under /api/, /install/ or the subscription path that matches no
// route answers 404, never the web app's HTML.
func newPanelHandler(cfg *config.Config) (*gin.Engine, error) {
	r := gin.New()
	if err := applyTrustedProxies(r, cfg.Server.TrustedProxies); err != nil {
		return nil, fmt.Errorf("configure trusted proxies: %w", err)
	}
	router.Setup(r, cfg)
	frontendPath := ""
	if cfg.Frontend.Enable {
		frontendPath = cfg.Frontend.Path
		if frontendPath == "" {
			frontendPath = "web/public"
		}
		prepareFrontendDirectory(frontendPath)
		// Hash-named assets can be cached for good; the SPA entry below
		// stays uncached so a deploy never serves a stale bundle.
		assetHandler := serveAssetFiles(frontendPath)
		r.GET("/assets/*filepath", assetHandler)
		r.HEAD("/assets/*filepath", assetHandler)
		r.StaticFile("/favicon.ico", filepath.Join(frontendPath, "favicon.ico"))
	}
	reserved := []string{"/api/", "/install/", "/.well-known/", "/" + cfg.App.SubscribePath + "/"}
	r.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path
		if strings.HasPrefix(path, "/api/v2/") {
			router.WriteAPIV2NotFound(c)
			return
		}
		for _, prefix := range reserved {
			if strings.HasPrefix(path, prefix) {
				c.String(http.StatusNotFound, "404 page not found")
				return
			}
		}
		if frontendPath == "" || (c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead) {
			c.String(http.StatusNotFound, "404 page not found")
			return
		}
		setHTMLNoCacheHeaders(c)
		c.File(filepath.Join(frontendPath, "index.html"))
	})
	return r, nil
}

// prepareFrontendDirectory writes a placeholder index when the web app is
// missing; a read-only file system (containers) keeps the API working and
// the web app answers 404 until assets exist.
func prepareFrontendDirectory(frontendPath string) {
	if _, err := os.Stat(frontendPath); !os.IsNotExist(err) {
		return
	}
	log.Printf("Frontend directory '%s' not found, creating a placeholder...", frontendPath)
	if err := os.MkdirAll(frontendPath, 0o750); err != nil {
		log.Printf("Warning: cannot create frontend directory %q: %v", frontendPath, err)
	} else if err := createDefaultIndex(frontendPath); err != nil {
		log.Printf("Warning: cannot create default frontend index in %q: %v", frontendPath, err)
	}
}

// panelServer is one listener of the panel.
type panelServer struct {
	// purpose names the listener in logs and errors.
	purpose  string
	server   *http.Server
	listener net.Listener
}

// httpServer is an http.Server with the configured timeouts.
func httpServer(cfg *config.Config, handler http.Handler) *http.Server {
	readTimeout := time.Duration(cfg.Server.ReadTimeout) * time.Second
	if readTimeout == 0 {
		readTimeout = 30 * time.Second
	}
	writeTimeout := time.Duration(cfg.Server.WriteTimeout) * time.Second
	if writeTimeout == 0 {
		writeTimeout = 60 * time.Second
	}
	return &http.Server{
		Handler:           handler,
		ReadTimeout:       readTimeout,
		ReadHeaderTimeout: readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       120 * time.Second,
	}
}

// httpsRedirect sends every plain request to https on the same host:
// port 80 of tls.mode acme and files. Only GET and HEAD are redirected, and
// only to a name of this panel (names: the panel's domain and gRPC name): a
// request for any other Host is redirected to the panel's domain, so the
// Host header cannot make this listener an open redirect.
func httpsRedirect(domain string, names []string, httpsAddress string) http.Handler {
	_, httpsPort, _ := net.SplitHostPort(httpsAddress)
	own := make(map[string]struct{}, len(names)+1)
	for _, name := range append([]string{domain}, names...) {
		if name = strings.ToLower(strings.TrimSpace(name)); name != "" {
			own[name] = struct{}{}
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "Use https://"+domain, http.StatusBadRequest)
			return
		}
		host := domain
		if requested, ok := requestorigin.NormalizeHost(r.Host); ok {
			name := requested
			if h, _, err := net.SplitHostPort(requested); err == nil {
				name = h
			}
			name = strings.ToLower(strings.Trim(name, "[]"))
			if _, known := own[name]; known {
				host = name
			}
		}
		if httpsPort != "" && httpsPort != "443" && httpsPort != "0" {
			host = net.JoinHostPort(strings.Trim(host, "[]"), httpsPort)
		}
		target := url.URL{Scheme: "https", Host: host, Path: r.URL.Path, RawQuery: r.URL.RawQuery}
		http.Redirect(w, r, target.String(), http.StatusMovedPermanently)
	})
}

// panelOwnNames are the names a plain-http request may be redirected to:
// the panel's domain and the gRPC name.
func panelOwnNames(panel config.PanelSettings) []string {
	return []string{panel.Domain, panel.GRPCName}
}

// bindPanelServers binds every listener of the layout before anything
// serves, so that a port conflict fails the start: https and http (the
// ACME HTTP-01 answers and the redirect) in tls.mode acme and files; the
// plain listeners in external mode, plus the ACME challenge listeners when
// their ports are free (a challenge listener that cannot bind is noted in
// the certificate status, not fatal).
func bindPanelServers(cfg *config.Config, panel config.PanelSettings, certs *panelcert.Manager, handler http.Handler) ([]*panelServer, error) {
	var servers []*panelServer
	closeAll := func() {
		for _, server := range servers {
			_ = server.listener.Close()
		}
	}
	bind := func(purpose, address string, serve http.Handler, tlsConfig *tls.Config, optional bool) error {
		listener, err := net.Listen("tcp", address)
		if err != nil {
			if optional {
				log.Printf("Panel: %s on %s is not available (%v); ACME challenges for %s must reach this Control another way", purpose, address, err, panel.GRPCName)
				certs.NoteListener(address, purpose, err)
				return nil
			}
			return fmt.Errorf("%s on %s: %w", purpose, address, err)
		}
		server := httpServer(cfg, serve)
		if tlsConfig != nil {
			server.TLSConfig = tlsConfig
			listener = tls.NewListener(listener, tlsConfig)
		}
		servers = append(servers, &panelServer{purpose: purpose, server: server, listener: listener})
		return nil
	}
	var err error
	switch panel.Mode {
	case config.TLSModeACME, config.TLSModeFiles:
		if err = bind("https", panel.HTTPSAddress, handler, certs.WebTLSConfig(), false); err == nil {
			// The redirect names the port https got (443 is left out).
			httpsAddress := servers[len(servers)-1].listener.Addr().String()
			err = bind("http (redirect to https, ACME HTTP-01)", panel.HTTPAddress, certs.HTTPHandler(httpsRedirect(panel.Domain, panelOwnNames(panel), httpsAddress)), nil, false)
		}
	default:
		for _, address := range panel.PlainAddresses {
			if err = bind("plain http", address, certs.HTTPHandler(handler), nil, false); err != nil {
				break
			}
		}
		if err == nil && panel.ChallengeHTTPAddress != "" {
			err = bind("ACME HTTP-01 listener", panel.ChallengeHTTPAddress, certs.HTTPHandler(httpsRedirect(panel.Domain, panelOwnNames(panel), ":443")), nil, true)
		}
		if err == nil && panel.ChallengeTLSAddress != "" {
			err = bind("ACME TLS-ALPN-01 listener", panel.ChallengeTLSAddress, http.NotFoundHandler(), certs.ChallengeOnlyTLSConfig(), true)
		}
	}
	if err != nil {
		closeAll()
		return nil, err
	}
	return servers, nil
}

// describePanel is the startup log line of the layout.
func describePanel(panel config.PanelSettings, servers []*panelServer) string {
	addresses := make([]string, 0, len(servers))
	for _, server := range servers {
		addresses = append(addresses, server.purpose+" "+server.listener.Addr().String())
	}
	switch panel.Mode {
	case config.TLSModeACME:
		return fmt.Sprintf("Panel: tls.mode acme for %s (Control obtains and renews the certificate): %s", panel.Domain, strings.Join(addresses, ", "))
	case config.TLSModeFiles:
		return fmt.Sprintf("Panel: tls.mode files for %s (%s, reloaded when it changes): %s", panel.Domain, panel.WebCertFile, strings.Join(addresses, ", "))
	}
	origin := "listen"
	if strings.TrimSpace(panel.Domain) == "" {
		origin = "no domain"
	}
	return fmt.Sprintf("Panel: tls.mode external (%s; TLS is terminated in front of Control): %s", origin, strings.Join(addresses, ", "))
}

// serve runs a bound panel server until it is shut down.
func (s *panelServer) serve() error {
	err := s.server.Serve(s.listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// describeGRPCTLS says where the gRPC listener's certificate comes from.
func describeGRPCTLS(panel config.PanelSettings) string {
	switch panel.GRPCCertSource {
	case config.GRPCCertPanel:
		return "TLS with the panel's certificate for " + panel.GRPCName
	case config.GRPCCertACME:
		return "TLS with its own ACME certificate for " + panel.GRPCName
	case config.GRPCCertFiles:
		return "TLS with " + panel.GRPCCertFile + ", reloaded when it changes"
	case config.GRPCCertEdge:
		return "TLS at the edge proxy " + strings.Join(panel.GRPCEdgeProxies, ", ") + " (grpc.edge_proxies): plain HTTP/2 from it only"
	}
	return "plain: no domain, grpc.public_name or certificate files; Agents cannot connect"
}
