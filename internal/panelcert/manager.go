// Package panelcert holds the certificates Control serves itself: the
// panel's (web, API, subscriptions, the install script) and the node gRPC
// listener's. A certificate comes from ACME (golang.org/x/crypto/acme/
// autocert: TLS-ALPN-01 on 443 first, HTTP-01 on 80 second) or from
// certificate files reloaded when they change. Handshakes never wait for
// an issuance: until a certificate is there, and whenever it is not usable,
// a self-signed fallback is served and the status says why.
//
// ACME state (the account key, the certificates and their keys, the
// challenge answers in flight) lives in the database table v5_acme_cache,
// the entries with a private key sealed with the root key
// (module_runtime.ca_kek), so that a restore or a move to another machine
// does not request the certificates again.
//
// Issuance is Control's own loop: proactively at start, renewal a third of
// the lifetime before the end (at most 30 days), retries after 1, 5 and 15
// minutes, then hourly three times, then every 6 hours, and never more than
// three requests per name within an hour (manual retries included). Each
// request runs a fresh autocert.Manager whose cache misses the name, so
// that it requests a new certificate; a long-lived autocert.Manager answers
// the challenges from the shared cache. A new certificate replaces the
// served one for the next handshake of every listener (hot reload).
package panelcert

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
	"gorm.io/gorm"
)

// Spec is one certificate Control serves.
type Spec struct {
	// Name is the DNS name it is for.
	Name string
	// Roles are the listeners that serve it: RoleWeb, RoleGRPC or both.
	Roles []string
	// Source is SourceACME or SourceFiles; CertFile and KeyFile are the PEM
	// files of SourceFiles.
	Source   string
	CertFile string
	KeyFile  string
}

// Options configures a Manager.
type Options struct {
	// DB holds v5_acme_cache; KEK (32 bytes, module_runtime.ca_kek) seals
	// the entries with a key. Without either, those entries stay in memory.
	DB  *gorm.DB
	KEK []byte
	// Mode, Domain and GRPCName describe the layout for the status.
	Mode     string
	Domain   string
	GRPCName string
	// Certificates are the certificates to serve.
	Certificates []Spec
	// DirectoryURL is the ACME directory; empty: the public CA
	// (autocert.DefaultACMEDirectory).
	DirectoryURL string
	// HTTPClient reaches the ACME directory; nil: a client on
	// http.DefaultTransport.
	HTTPClient *http.Client
	// Roots verifies served certificates for PubliclyTrusted; nil: the
	// system roots.
	Roots *x509.CertPool
	// Now defaults to time.Now.
	Now func() time.Time
	// FilePollInterval is how often certificate files are checked for a
	// change (default 60s).
	FilePollInterval time.Duration
	// RetryDelays are the waits after consecutive failed requests; the
	// last repeats (default 1m, 5m, 15m, 1h, 1h, 1h, 6h).
	RetryDelays []time.Duration
	// MaxAttemptsPerHour caps the requests per name within any hour
	// (default 3).
	MaxAttemptsPerHour int
	// AttemptTimeout bounds one request (default 5m, autocert's own
	// bound).
	AttemptTimeout time.Duration
	// LocalAddresses lists this machine's addresses for the fallback and
	// the DNS diagnosis (default: the interface addresses).
	LocalAddresses func() []net.IP
	// Resolver resolves names for the diagnosis of a failure (default
	// net.DefaultResolver).
	Resolver Resolver
	// Logf logs (default log.Printf).
	Logf func(format string, args ...any)
}

// Defaults.
const (
	defaultFilePollInterval = time.Minute
	defaultMaxAttempts      = 3
	defaultAttemptTimeout   = 5 * time.Minute
	// maxRenewBefore caps how early an ACME certificate is renewed.
	maxRenewBefore = 30 * 24 * time.Hour
	// statusRefresh is the longest a loop sleeps before it looks again
	// (the status's clock-driven parts, the fallback's age).
	statusRefresh = time.Minute
	// retryPollInterval is how often the command line's retry request is
	// read from the database.
	retryPollInterval = 5 * time.Second
	// inertRenewBefore makes the renewal timer of an issuance's own
	// autocert.Manager fire only at the certificate's end; by then this
	// package renewed it, and the timer finds the newer certificate in the
	// cache and goes back to sleep. It never reaches the CA: the issuance's
	// transport is closed. (autocert has no way to stop that timer.)
	inertRenewBefore = 10 * time.Nanosecond
	// transportGrace lets an ended issuance deactivate its pending
	// authorizations before its transport closes.
	transportGrace = 30 * time.Second
)

var defaultRetryDelays = []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute, time.Hour, time.Hour, time.Hour, 6 * time.Hour}

// Manager serves and renews the panel's certificates.
type Manager struct {
	opts      Options
	store     *Store
	directory string
	responder *autocert.Manager
	challenge http.Handler
	slots     []*slot
	acmeNames map[string]bool

	issueMu sync.Mutex
	started atomic.Bool

	mu            sync.Mutex
	listeners     []ListenerStatus
	lastPublished string
	publishedAt   time.Time
}

// slot is one served certificate.
type slot struct {
	spec Spec
	wake chan struct{}

	served   atomic.Pointer[servedCertificate]
	fallback atomic.Pointer[tls.Certificate]

	mu       sync.Mutex
	attempts []time.Time
	failures int
	retry    bool
	// next is when the loop may make its next request.
	next          time.Time
	lastAttempt   time.Time
	lastSuccess   time.Time
	lastError     string
	problem       string
	attemptFailed bool
	// attempting is set while a request to the CA runs.
	attempting bool
	// files: the files' last seen stamp.
	stamp     fileStamp
	loadedAny bool
}

// servedCertificate is a certificate from ACME or the files.
type servedCertificate struct {
	certificate *tls.Certificate
	serving     string
	trusted     bool
	trustError  string
}

// New checks opts and returns a Manager; Start loads what is stored.
func New(opts Options) (*Manager, error) {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.FilePollInterval <= 0 {
		opts.FilePollInterval = defaultFilePollInterval
	}
	if len(opts.RetryDelays) == 0 {
		opts.RetryDelays = defaultRetryDelays
	}
	if opts.MaxAttemptsPerHour <= 0 {
		opts.MaxAttemptsPerHour = defaultMaxAttempts
	}
	if opts.AttemptTimeout <= 0 {
		opts.AttemptTimeout = defaultAttemptTimeout
	}
	if opts.LocalAddresses == nil {
		opts.LocalAddresses = LocalAddresses
	}
	if opts.Logf == nil {
		opts.Logf = log.Printf
	}
	store, err := NewStore(opts.DB, opts.KEK, opts.Now)
	if err != nil {
		return nil, err
	}
	m := &Manager{opts: opts, store: store, directory: strings.TrimSpace(opts.DirectoryURL), acmeNames: map[string]bool{}}
	if m.directory == "" {
		m.directory = autocert.DefaultACMEDirectory
	}
	seen := map[string]bool{}
	for _, spec := range opts.Certificates {
		spec.Name = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(spec.Name)), ".")
		// Only certificate files may come without a name (a gRPC listener
		// of a Control without domain): the files name it.
		if (spec.Name == "" && spec.Source != SourceFiles) || seen[spec.Name] {
			return nil, fmt.Errorf("panel certificate: missing or repeated name %q", spec.Name)
		}
		seen[spec.Name] = true
		switch spec.Source {
		case SourceACME:
			m.acmeNames[spec.Name] = true
		case SourceFiles:
			if spec.CertFile == "" || spec.KeyFile == "" {
				return nil, fmt.Errorf("panel certificate %s: the certificate and key files are required", spec.Name)
			}
		default:
			return nil, fmt.Errorf("panel certificate %s: unknown source %q", spec.Name, spec.Source)
		}
		m.slots = append(m.slots, &slot{spec: spec, wake: make(chan struct{}, 1)})
	}
	if len(m.acmeNames) > 0 {
		// The responder answers the challenges of every issuance from the
		// shared cache; it never requests a certificate itself.
		m.responder = &autocert.Manager{Prompt: autocert.AcceptTOS, Cache: newDirectoryCache(store, m.directory), HostPolicy: m.hostPolicy}
		m.challenge = m.responder.HTTPHandler(http.NotFoundHandler())
	}
	return m, nil
}

func (m *Manager) now() time.Time { return m.opts.Now() }

// hostPolicy admits the ACME names only, with or without a port.
func (m *Manager) hostPolicy(_ context.Context, host string) error {
	if name, _, err := net.SplitHostPort(host); err == nil {
		host = name
	}
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if m.acmeNames[host] {
		return nil
	}
	return fmt.Errorf("panel certificate: %q is not a name of this Control", host)
}

// Start loads the fallback, the stored ACME certificates and the files, so
// that the listeners serve the right certificate from their first
// handshake. It makes no ACME request; Run does.
func (m *Manager) Start(ctx context.Context) error {
	if !m.started.CompareAndSwap(false, true) {
		return errors.New("panel certificate manager already started")
	}
	now := m.now()
	addresses := m.opts.LocalAddresses()
	for _, s := range m.slots {
		fallback, err := loadSelfSigned(ctx, m.store, s.fallbackName(), addresses, now)
		if fallback == nil {
			return fmt.Errorf("panel certificate %s: self-signed fallback: %w", s.spec.Name, err)
		}
		if err != nil {
			m.opts.Logf("Panel certificate %s: the self-signed fallback was not stored: %v", s.spec.Name, err)
		}
		s.fallback.Store(fallback)
		switch s.spec.Source {
		case SourceACME:
			m.loadStoredACME(ctx, s)
		case SourceFiles:
			m.loadFiles(s, true)
		}
	}
	m.publish(ctx, true)
	return nil
}

// Run renews and reloads the certificates until ctx ends.
func (m *Manager) Run(ctx context.Context) {
	if !m.started.Load() {
		if err := m.Start(ctx); err != nil {
			m.opts.Logf("Panel certificates: %v", err)
			return
		}
	}
	var wg sync.WaitGroup
	for _, s := range m.slots {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.loop(ctx, s)
		}()
	}
	if m.opts.DB != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.pollRetryRequests(ctx)
		}()
	}
	wg.Wait()
}

func (m *Manager) loop(ctx context.Context, s *slot) {
	for {
		wait := m.step(ctx, s)
		if ctx.Err() != nil {
			return
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		case <-s.wake:
			timer.Stop()
		}
	}
}

// step does what is due for s and returns how long to wait.
func (m *Manager) step(ctx context.Context, s *slot) time.Duration {
	m.refreshFallback(ctx, s)
	wait := statusRefresh
	switch s.spec.Source {
	case SourceFiles:
		s.mu.Lock()
		force := s.retry
		s.retry = false
		s.mu.Unlock()
		m.loadFiles(s, force)
		wait = min(wait, m.opts.FilePollInterval)
	case SourceACME:
		wait = min(wait, m.stepACME(ctx, s))
	}
	m.publish(ctx, false)
	return max(wait, time.Second)
}

// refreshFallback replaces a fallback close to its end.
func (m *Manager) refreshFallback(ctx context.Context, s *slot) {
	current := s.fallback.Load()
	now := m.now()
	if current != nil && current.Leaf != nil && current.Leaf.NotAfter.Sub(now) > selfSignedRenewBefore {
		return
	}
	if fallback, err := loadSelfSigned(ctx, m.store, s.fallbackName(), m.opts.LocalAddresses(), now); fallback != nil {
		s.fallback.Store(fallback)
		if err != nil {
			m.opts.Logf("Panel certificate %s: the self-signed fallback was not stored: %v", s.spec.Name, err)
		}
	}
}

// fallbackName is the name of s's self-signed fallback.
func (s *slot) fallbackName() string {
	if s.spec.Name == "" {
		return "localhost"
	}
	return s.spec.Name
}

// certificateFor returns what a handshake for s gets now.
func (s *slot) certificateFor(now time.Time) *tls.Certificate {
	if served := s.served.Load(); served != nil && served.certificate.Leaf != nil && now.Before(served.certificate.Leaf.NotAfter) {
		return served.certificate
	}
	return s.fallback.Load()
}

// renewAt is when an ACME certificate is renewed: a third of its lifetime
// before its end, at most maxRenewBefore (autocert's rule).
func renewAt(leaf *x509.Certificate) time.Time {
	lifetime := leaf.NotAfter.Sub(leaf.NotBefore)
	return leaf.NotAfter.Add(-min(lifetime/3, maxRenewBefore))
}

// stepACME requests a certificate when one is due and allowed, and
// returns how long until the next look.
func (m *Manager) stepACME(ctx context.Context, s *slot) time.Duration {
	now := m.now()
	due := now
	if served := s.served.Load(); served != nil && served.certificate.Leaf != nil {
		due = renewAt(served.certificate.Leaf)
	}
	s.mu.Lock()
	manual := s.retry
	s.retry = false
	if !manual && now.Before(due) {
		s.mu.Unlock()
		return due.Sub(now)
	}
	if !manual && now.Before(s.next) {
		wait := s.next.Sub(now)
		s.mu.Unlock()
		return wait
	}
	if blockedUntil, blocked := s.quotaBlocked(now, m.opts.MaxAttemptsPerHour); blocked {
		// The status keeps the last failure; NextAttemptAt says when.
		s.next = blockedUntil
		s.mu.Unlock()
		return blockedUntil.Sub(now)
	}
	s.attempts = append(s.attempts, now)
	s.lastAttempt = now
	s.attempting = true
	s.mu.Unlock()
	m.publish(ctx, true)

	certificate, err := m.issue(ctx, s)
	s.mu.Lock()
	s.attempting = false
	s.mu.Unlock()
	if ctx.Err() != nil {
		return time.Second
	}
	now = m.now()
	if err == nil {
		served := m.servedFrom(certificate, ServingACME, s.spec.Name, now)
		s.served.Store(served)
		s.mu.Lock()
		s.failures, s.attemptFailed = 0, false
		s.lastSuccess, s.lastError, s.problem = now, "", ""
		s.next = time.Time{}
		s.mu.Unlock()
		m.opts.Logf("Panel certificate %s: issued by %s, valid until %s", s.spec.Name, issuerName(certificate.Leaf), certificate.Leaf.NotAfter.UTC().Format(time.RFC3339))
		return min(renewAt(certificate.Leaf).Sub(now), statusRefresh)
	}
	message := err.Error()
	var problems []caProblem
	var issuance *issuanceError
	if errors.As(err, &issuance) {
		problems = issuance.problems
	}
	problem := classify(err, problems)
	if problem != ProblemRateLimited && problem != ProblemCAUnreachable {
		// The local DNS view sharpens the CA's answer (no record, a record
		// that points elsewhere).
		report := DiagnoseDNS(ctx, m.opts.Resolver, s.spec.Name, m.opts.LocalAddresses())
		switch report.Verdict {
		case DNSUnresolved:
			problem = ProblemDNSUnresolved
		case DNSElsewhere:
			problem = ProblemDNSElsewhere
		}
		if report.Verdict != DNSOK {
			message += " (DNS: " + report.Hint + ")"
		}
	}
	s.mu.Lock()
	delay := m.opts.RetryDelays[min(s.failures, len(m.opts.RetryDelays)-1)]
	s.failures++
	s.attemptFailed = true
	s.lastError, s.problem = message, problem
	s.next = now.Add(delay)
	next := s.next
	s.mu.Unlock()
	m.opts.Logf("Panel certificate %s: the ACME request failed (%s): %s; next try at %s", s.spec.Name, problem, message, next.UTC().Format(time.RFC3339))
	return delay
}

// quotaBlocked tells whether max requests were made within the hour
// before now, and until when.
func (s *slot) quotaBlocked(now time.Time, maxAttempts int) (time.Time, bool) {
	recent := s.attempts[:0]
	for _, at := range s.attempts {
		if now.Sub(at) < time.Hour {
			recent = append(recent, at)
		}
	}
	s.attempts = recent
	if len(recent) < maxAttempts {
		return time.Time{}, false
	}
	return recent[0].Add(time.Hour), true
}

// ecdsaHello asks autocert for an ECDSA P-256 certificate of name.
func ecdsaHello(name string) *tls.ClientHelloInfo {
	return &tls.ClientHelloInfo{
		ServerName:       name,
		SignatureSchemes: []tls.SignatureScheme{tls.ECDSAWithP256AndSHA256},
		SupportedCurves:  []tls.CurveID{tls.CurveP256},
		CipherSuites:     []uint16{tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256},
	}
}

// issue requests a new certificate for s from the CA with a fresh
// autocert.Manager. One issuance runs at a time: they share the account.
func (m *Manager) issue(ctx context.Context, s *slot) (*tls.Certificate, error) {
	m.issueMu.Lock()
	defer m.issueMu.Unlock()
	cache := newDirectoryCache(m.store, m.directory)
	cache.force(s.spec.Name)
	defer cache.release()
	var base http.RoundTripper
	timeout := 60 * time.Second
	if m.opts.HTTPClient != nil {
		base = m.opts.HTTPClient.Transport
		if m.opts.HTTPClient.Timeout > 0 {
			timeout = m.opts.HTTPClient.Timeout
		}
	}
	transport := newRecordingTransport(base)
	issuer := &autocert.Manager{
		Prompt:      autocert.AcceptTOS,
		Cache:       cache,
		HostPolicy:  m.hostPolicy,
		RenewBefore: inertRenewBefore,
		Client: &acme.Client{
			DirectoryURL: m.directory,
			HTTPClient:   &http.Client{Transport: transport, Timeout: timeout},
			UserAgent:    "anix-control",
		},
	}
	// Offer HTTP-01 after TLS-ALPN-01; the responder answers it.
	issuer.HTTPHandler(nil)

	type result struct {
		certificate *tls.Certificate
		err         error
	}
	done := make(chan result, 1)
	go func() {
		certificate, err := issuer.GetCertificate(ecdsaHello(s.spec.Name))
		done <- result{certificate, err}
	}()
	attemptCtx, cancel := context.WithTimeout(ctx, m.opts.AttemptTimeout)
	defer cancel()
	var outcome result
	select {
	case outcome = <-done:
		time.AfterFunc(transportGrace, transport.close)
	case <-attemptCtx.Done():
		transport.close()
		outcome = result{err: fmt.Errorf("the ACME request did not finish within %s: %w", m.opts.AttemptTimeout, attemptCtx.Err())}
	}
	if outcome.err != nil {
		return nil, &issuanceError{err: outcome.err, problems: transport.recorded()}
	}
	certificate := outcome.certificate
	if certificate.Leaf == nil {
		leaf, err := x509.ParseCertificate(certificate.Certificate[0])
		if err != nil {
			return nil, err
		}
		certificate.Leaf = leaf
	}
	return certificate, nil
}

// loadStoredACME serves the certificate a previous run stored for s when it
// is still valid for its name.
func (m *Manager) loadStoredACME(ctx context.Context, s *slot) {
	data, err := m.store.Get(ctx, certificateKey(m.directory, s.spec.Name))
	if err != nil {
		if !errors.Is(err, autocert.ErrCacheMiss) {
			m.opts.Logf("Panel certificate %s: the stored certificate cannot be read: %v", s.spec.Name, err)
		}
		return
	}
	certificate, err := parseKeyAndChain(data)
	now := m.now()
	if err != nil || certificate.Leaf.VerifyHostname(s.spec.Name) != nil || !now.Before(certificate.Leaf.NotAfter) {
		return
	}
	s.served.Store(m.servedFrom(certificate, ServingACME, s.spec.Name, now))
}

// servedFrom wraps a certificate with whether it verifies for name.
func (m *Manager) servedFrom(certificate *tls.Certificate, serving, name string, now time.Time) *servedCertificate {
	served := &servedCertificate{certificate: certificate, serving: serving}
	intermediates := x509.NewCertPool()
	for _, der := range certificate.Certificate[1:] {
		if parsed, err := x509.ParseCertificate(der); err == nil {
			intermediates.AddCert(parsed)
		}
	}
	_, err := certificate.Leaf.Verify(x509.VerifyOptions{
		DNSName: name, Roots: m.opts.Roots, Intermediates: intermediates, CurrentTime: now,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	})
	served.trusted = err == nil
	if err != nil {
		served.trustError = err.Error()
	}
	return served
}

// leafName is the first name a certificate covers.
func leafName(leaf *x509.Certificate) string {
	if len(leaf.DNSNames) > 0 {
		return leaf.DNSNames[0]
	}
	if len(leaf.IPAddresses) > 0 {
		return leaf.IPAddresses[0].String()
	}
	return leaf.Subject.CommonName
}

// issuerName is a certificate issuer's display name.
func issuerName(leaf *x509.Certificate) string {
	if leaf == nil {
		return ""
	}
	if leaf.Issuer.CommonName != "" {
		return leaf.Issuer.CommonName
	}
	return strings.Join(leaf.Issuer.Organization, ", ")
}

// GetCertificate returns the tls.Config.GetCertificate of a listener of
// role (RoleWeb or RoleGRPC): the certificate whose name the client asked
// for (SNI), else the role's, from ACME or the files when usable and the
// self-signed fallback otherwise. TLS-ALPN-01 challenge handshakes get the
// challenge certificate. It never blocks on an issuance.
func (m *Manager) GetCertificate(role string) func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	return func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
		if wantsChallenge(hello) {
			return m.challengeCertificate(hello)
		}
		s := m.slotFor(role, hello.ServerName)
		if s == nil {
			return nil, fmt.Errorf("panel certificate: no certificate for the %s listener", role)
		}
		return s.certificateFor(m.now()), nil
	}
}

// ChallengeOnlyTLSConfig is the TLS configuration of a listener that only
// answers TLS-ALPN-01 challenges (port 443 in tls.mode external).
func (m *Manager) ChallengeOnlyTLSConfig() *tls.Config {
	return &tls.Config{
		MinVersion: tls.VersionTLS12,
		NextProtos: []string{acme.ALPNProto},
		GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			if wantsChallenge(hello) {
				return m.challengeCertificate(hello)
			}
			return nil, errors.New("this port answers ACME challenges only")
		},
	}
}

// WebTLSConfig is the https listener's TLS configuration: h2 and
// http/1.1, and acme-tls/1 for the TLS-ALPN-01 challenge.
func (m *Manager) WebTLSConfig() *tls.Config {
	return &tls.Config{
		MinVersion:     tls.VersionTLS12,
		NextProtos:     []string{"h2", "http/1.1", acme.ALPNProto},
		GetCertificate: m.GetCertificate(RoleWeb),
	}
}

func wantsChallenge(hello *tls.ClientHelloInfo) bool {
	return len(hello.SupportedProtos) == 1 && hello.SupportedProtos[0] == acme.ALPNProto
}

func (m *Manager) challengeCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	if m.responder == nil {
		return nil, errors.New("panel certificate: no ACME challenge in flight")
	}
	return m.responder.GetCertificate(hello)
}

// slotFor picks the certificate of a handshake.
func (m *Manager) slotFor(role, serverName string) *slot {
	name := strings.TrimSuffix(strings.ToLower(serverName), ".")
	var fallback *slot
	for _, s := range m.slots {
		if name != "" && s.spec.Name == name {
			return s
		}
		if fallback == nil && hasRole(s.spec.Roles, role) {
			fallback = s
		}
	}
	return fallback
}

func hasRole(roles []string, role string) bool {
	for _, r := range roles {
		if r == role {
			return true
		}
	}
	return false
}

// HasRole tells whether a certificate serves role.
func (m *Manager) HasRole(role string) bool {
	for _, s := range m.slots {
		if hasRole(s.spec.Roles, role) {
			return true
		}
	}
	return false
}

// HandlesChallenges tells whether any certificate comes from ACME.
func (m *Manager) HandlesChallenges() bool { return m.responder != nil }

// HTTPHandler answers the ACME HTTP-01 challenges
// (/.well-known/acme-challenge/) of the ACME names and hands every other
// request to next.
func (m *Manager) HTTPHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if m.challenge != nil && strings.HasPrefix(r.URL.Path, "/.well-known/acme-challenge/") {
			m.challenge.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// NoteListener records a listener the layout asked for that could not
// bind (shown in the status).
func (m *Manager) NoteListener(address, purpose string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.listeners = append(m.listeners, ListenerStatus{Address: address, Purpose: purpose, Error: err.Error()})
}

// RetryResult says what a retry does.
type RetryResult struct {
	// Retrying are the names Control tries now; Waiting the names whose
	// hourly limit is reached (with when the next request may go), Healthy
	// the names that need nothing.
	Retrying []string             `json:"retrying"`
	Waiting  map[string]time.Time `json:"waiting,omitempty"`
	Healthy  []string             `json:"healthy"`
}

// Retry tries the certificates that are failing (no usable certificate, a
// renewal that is due or failed) again now, within the hourly limit;
// certificate files that are not usable or do not verify are read again.
func (m *Manager) Retry() RetryResult {
	result := RetryResult{Retrying: []string{}, Healthy: []string{}, Waiting: map[string]time.Time{}}
	now := m.now()
	for _, s := range m.slots {
		status := m.slotStatus(s, now)
		renewalDue := status.RenewAt != nil && !now.Before(*status.RenewAt)
		// A certificate the CA issued that does not verify (a staging
		// directory) is not requested again: the same CA would answer the
		// same; files are read again.
		reload := s.spec.Source == SourceFiles && !status.PubliclyTrusted
		needed := status.State != StateValid || status.RenewalFailing || renewalDue || reload
		if !needed {
			result.Healthy = append(result.Healthy, s.spec.Name)
			continue
		}
		s.mu.Lock()
		if s.attempting {
			// A request is under way; its outcome is the retry's.
			s.mu.Unlock()
			result.Retrying = append(result.Retrying, s.spec.Name)
			continue
		}
		if s.spec.Source == SourceACME {
			if until, blocked := s.quotaBlocked(now, m.opts.MaxAttemptsPerHour); blocked {
				result.Waiting[s.spec.Name] = until
				s.mu.Unlock()
				continue
			}
		}
		s.retry = true
		s.mu.Unlock()
		result.Retrying = append(result.Retrying, s.spec.Name)
		select {
		case s.wake <- struct{}{}:
		default:
		}
	}
	return result
}

// pollRetryRequests turns a retry request the command line stored into
// Retry.
func (m *Manager) pollRetryRequests(ctx context.Context) {
	ticker := time.NewTicker(retryPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		if _, err := m.store.Get(ctx, retryKey); err != nil {
			continue
		}
		if err := m.store.Delete(ctx, retryKey); err != nil {
			continue
		}
		result := m.Retry()
		m.opts.Logf("Panel certificates: retry requested from the command line: retrying %v, waiting %v", result.Retrying, result.Waiting)
	}
}

// slotStatus is s's status at now.
func (m *Manager) slotStatus(s *slot, now time.Time) CertificateStatus {
	status := CertificateStatus{
		Name: s.spec.Name, Names: []string{}, Roles: append([]string(nil), s.spec.Roles...), Source: s.spec.Source,
		CertFile: s.spec.CertFile, KeyFile: s.spec.KeyFile, Serving: ServingSelfSigned,
	}
	s.mu.Lock()
	status.LastAttemptAt = timeRef(s.lastAttempt)
	status.LastSuccessAt = timeRef(s.lastSuccess)
	status.NextAttemptAt = timeRef(s.next)
	status.LastError, status.Problem = s.lastError, s.problem
	status.Attempting = s.attempting
	attemptFailed, loadedAny := s.attemptFailed, s.loadedAny
	s.mu.Unlock()

	served := s.served.Load()
	if served != nil {
		leaf := served.certificate.Leaf
		if status.Name == "" {
			status.Name = leafName(leaf)
		}
		status.Issuer = issuerName(leaf)
		status.NotBefore, status.NotAfter = timeRef(leaf.NotBefore), timeRef(leaf.NotAfter)
		if s.spec.Source == SourceACME {
			status.RenewAt = timeRef(renewAt(leaf))
		}
	}
	switch {
	case served != nil && now.Before(served.certificate.Leaf.NotAfter):
		status.State, status.Serving = StateValid, served.serving
		status.Names = append(status.Names, served.certificate.Leaf.DNSNames...)
		for _, address := range served.certificate.Leaf.IPAddresses {
			status.Names = append(status.Names, address.String())
		}
		status.PubliclyTrusted = served.trusted
		if s.spec.Source == SourceACME && attemptFailed && !now.Before(renewAt(served.certificate.Leaf)) {
			status.RenewalFailing = true
		}
		if !served.trusted && status.Problem == "" {
			status.Problem = ProblemUntrusted
			if s.spec.Name != "" && served.certificate.Leaf.VerifyHostname(s.spec.Name) != nil {
				status.Problem = ProblemNameMismatch
			}
			status.LastError = "the served certificate does not verify: " + served.trustError
		}
	case served != nil:
		status.State = StateFailed
		if status.Problem == "" {
			status.Problem = ProblemExpired
			status.LastError = "the certificate ended at " + served.certificate.Leaf.NotAfter.UTC().Format(time.RFC3339)
		}
	case s.spec.Source == SourceACME && !attemptFailed:
		status.State = StatePending
	case s.spec.Source == SourceFiles && !loadedAny && status.LastError == "":
		status.State = StatePending
	default:
		status.State = StateFailed
	}
	if status.State == StateValid && !status.RenewalFailing && status.PubliclyTrusted {
		status.LastError, status.Problem = "", ""
	}
	if status.Problem != "" {
		status.Fix = fixFor(status.Problem, status.Name, s.spec, m.directory)
	}
	return status
}

func timeRef(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	utc := t.UTC()
	return &utc
}

// Status is the panel's certificate status now.
func (m *Manager) Status() Status {
	now := m.now()
	status := Status{
		Mode: m.opts.Mode, Domain: m.opts.Domain, GRPCName: m.opts.GRPCName,
		Persistent: m.store.Persistent(), Certificates: []CertificateStatus{}, UpdatedAt: now.UTC(),
	}
	if m.responder != nil && m.directory != autocert.DefaultACMEDirectory {
		status.Directory = m.directory
	}
	for _, s := range m.slots {
		status.Certificates = append(status.Certificates, m.slotStatus(s, now))
	}
	status.State = overall(status.Certificates)
	m.mu.Lock()
	status.Listeners = append([]ListenerStatus(nil), m.listeners...)
	m.mu.Unlock()
	return status
}

// publish stores the status for the command line and the alert scan when
// it changed (or force), at least every ten minutes.
func (m *Manager) publish(ctx context.Context, force bool) {
	status := m.Status()
	stamp := status.UpdatedAt
	status.UpdatedAt = time.Time{}
	comparable, err := json.Marshal(status)
	if err != nil {
		return
	}
	status.UpdatedAt = stamp
	m.mu.Lock()
	if !force && string(comparable) == m.lastPublished && stamp.Sub(m.publishedAt) < 10*time.Minute {
		m.mu.Unlock()
		return
	}
	m.lastPublished, m.publishedAt = string(comparable), stamp
	m.mu.Unlock()
	data, err := json.Marshal(status)
	if err != nil {
		return
	}
	if err := m.store.Put(ctx, statusKey, data); err != nil && ctx.Err() == nil {
		m.opts.Logf("Panel certificates: the status was not stored: %v", err)
	}
}

// GRPCCertificate is the status of the gRPC listener's certificate, false
// when it has none (plain gRPC).
func (m *Manager) GRPCCertificate() (CertificateStatus, bool) {
	return m.Status().Certificate(RoleGRPC)
}

var defaultManager atomic.Pointer[Manager]

// SetDefault makes m the process's manager, which the status route and the
// install readiness read; nil clears it.
func SetDefault(m *Manager) { defaultManager.Store(m) }

// Default returns the process's manager, nil when none runs (a command, a
// test).
func Default() *Manager { return defaultManager.Load() }
