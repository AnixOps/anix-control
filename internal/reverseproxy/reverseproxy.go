// Package reverseproxy is the panel's HTTP(S) reverse proxies: an
// administrator names the sites (host names and path routes to upstream
// servers), the entry devices that answer them and, optionally, the
// devices the upstreams are dialed from; Control turns each into a
// NetworkPlan reverse proxy on every entry (networkmodel.Forward with a
// ReverseProxySpec), linking the devices as it links a forward route's
// hops. The entries run it in anixcore (networkcore inbound-reverse-proxy),
// terminating TLS with the proxy's certificate.
package reverseproxy

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	networkv1 "github.com/AnixOps/anix-control/sdk/api/network/v1"
	"github.com/AnixOps/anix-control/sdk/network"
	"github.com/AnixOps/anix-control/v5/internal/config"
	"github.com/AnixOps/anix-control/v5/internal/forwardddns"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/modulepki"
	"github.com/AnixOps/anix-control/v5/internal/networkmodel"
	"google.golang.org/protobuf/encoding/protojson"
	"gorm.io/gorm"
)

func init() { networkmodel.RegisterForwardSource("reverse_proxy", Forwards) }

var (
	// ErrNotFound: no reverse proxy with that id.
	ErrNotFound = errors.New("reverse proxy not found")
	// ErrInvalid: the reverse proxy is invalid; the error says why.
	ErrInvalid = errors.New("invalid reverse proxy")
	// ErrNoKEK: certificate keys are sealed with module_runtime.ca_kek.
	ErrNoKEK = errors.New("set module_runtime.ca_kek: reverse proxy private keys are sealed with Control's key-encryption key")
)

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

// TLS modes.
const (
	TLSNone   = "none"
	TLSManual = "manual"
	TLSACME   = "acme"
)

// Limits and audit names.
const (
	MaxProxies        = 256
	MaxEntries        = 8
	MaxVia            = 4
	maxNameLength     = 100
	auditModule       = "reverse_proxy"
	auditTarget       = "reverse_proxy"
	AuditActionCreate = "reverse_proxy_create"
	AuditActionUpdate = "reverse_proxy_update"
	AuditActionDelete = "reverse_proxy_delete"
	// IDPrefix starts a reverse proxy's id in the plans; HTTPSuffix ends
	// the id of its plain redirect listener.
	IDPrefix   = "rp-"
	HTTPSuffix = "-http"
	// keyAdditionalData binds a sealed private key to its use.
	keyAdditionalData = "anixops/reverse-proxy/private-key"
)

// Upstream is one server a route sends requests to.
type Upstream struct {
	Host     string `json:"host"`
	Port     uint32 `json:"port"`
	Weight   uint32 `json:"weight,omitempty"`
	Priority uint32 `json:"priority,omitempty"`
}

// Health is a route's active health check of its upstreams (TCP connect
// through the route; defaults 5000 and 2000 ms).
type Health struct {
	IntervalMs uint32 `json:"interval_ms,omitempty"`
	TimeoutMs  uint32 `json:"timeout_ms,omitempty"`
	Disabled   bool   `json:"disabled,omitempty"`
}

// Route sends requests under Path to its upstreams.
type Route struct {
	// Path is "/" or a prefix without a trailing slash.
	Path        string `json:"path"`
	StripPrefix bool   `json:"strip_prefix,omitempty"`
	// HostHeader is "client" (default) or "upstream"; Host, when set, is
	// sent instead.
	HostHeader string            `json:"host_header,omitempty"`
	Host       string            `json:"host,omitempty"`
	Headers    map[string]string `json:"headers,omitempty"`
	Upstreams  []Upstream        `json:"upstreams"`
	// Balance is round_robin (default), random, ip_hash, least_conn or
	// failover.
	Balance string  `json:"balance,omitempty"`
	Health  *Health `json:"health,omitempty"`
}

// Site is the names a reverse proxy answers and their routes.
type Site struct {
	// Hosts are "example.com", "*.example.com" or "*".
	Hosts  []string `json:"hosts"`
	Routes []Route  `json:"routes"`
}

// Input is a reverse proxy as an administrator writes it. An empty
// private_key_pem on an update keeps the stored key (with the stored
// certificate when certificate_pem is empty too).
type Input struct {
	Name    string `json:"name"`
	Enabled *bool  `json:"enabled"`
	// Entries are the devices that answer (proxy-<id>); several are
	// entry HA, each running the same proxy.
	Entries []string `json:"entries"`
	// Via are devices the requests travel through before the last of them
	// dials the upstreams; empty dials from the entry.
	Via        []string `json:"via"`
	ListenPort uint32   `json:"listen_port"`
	// HTTPPort, with TLS, is a plain listener that redirects to https.
	HTTPPort uint32 `json:"http_port"`
	Sites    []Site `json:"sites"`
	// TLSMode is none, manual (certificate_pem and private_key_pem) or
	// acme: Control obtains and renews the certificate with an ACME DNS-01
	// challenge, writing the TXT records through dns_provider_id (a
	// forwarding DNS provider) in dns_zone, which holds every host.
	TLSMode        string `json:"tls_mode"`
	CertificatePEM string `json:"certificate_pem"`
	PrivateKeyPEM  string `json:"private_key_pem"`
	DNSProviderID  uint   `json:"dns_provider_id"`
	DNSZone        string `json:"dns_zone"`
}

// EntryStatus is what an entry device last reported of the proxy.
type EntryStatus struct {
	Node      string                           `json:"node"`
	ID        string                           `json:"id"`
	State     string                           `json:"state"`
	Detail    string                           `json:"detail,omitempty"`
	UpBytes   uint64                           `json:"up_bytes"`
	DownBytes uint64                           `json:"down_bytes"`
	Upstreams []*networkv1.ForwardTargetHealth `json:"upstreams,omitempty"`
}

// View is a reverse proxy as the API answers it: never the private key.
type View struct {
	ID                  uint       `json:"id"`
	Name                string     `json:"name"`
	Enabled             bool       `json:"enabled"`
	Entries             []string   `json:"entries"`
	Via                 []string   `json:"via"`
	ListenPort          uint32     `json:"listen_port"`
	HTTPPort            uint32     `json:"http_port"`
	Sites               []Site     `json:"sites"`
	TLSMode             string     `json:"tls_mode"`
	DNSProviderID       uint       `json:"dns_provider_id"`
	DNSZone             string     `json:"dns_zone"`
	HasCertificate      bool       `json:"has_certificate"`
	CertificateNames    []string   `json:"certificate_names"`
	CertificateNotAfter *time.Time `json:"certificate_not_after"`
	CertificateError    string     `json:"certificate_error,omitempty"`
	// Pending are why entries do not run it now; Status what they report.
	Pending   []string      `json:"pending"`
	Status    []EntryStatus `json:"status"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
}

// Actor is who made an administrator's change, for the audit entry.
type Actor struct {
	UserID uint
	Name   string
	IP     string
}

// Service manages the reverse proxies on one database.
type Service struct {
	DB *gorm.DB
	// KEK answers module_runtime.ca_kek; nil reads the configuration.
	KEK func() string
	// Now defaults to time.Now.
	Now func() time.Time
	// Network is the network model the proxies are planned in; nil uses
	// networkmodel.New on the same database.
	Network func(db *gorm.DB) *networkmodel.Service
}

// New returns the service on db.
func New(db *gorm.DB) *Service { return &Service{DB: db} }

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s *Service) network(db *gorm.DB) *networkmodel.Service {
	if s.Network != nil {
		return s.Network(db)
	}
	return networkmodel.New(db)
}

// List answers every reverse proxy by name, with its pending reasons and
// its entries' reports.
func (s *Service) List(ctx context.Context) ([]View, error) {
	db := s.DB.WithContext(ctx)
	var rows []model.ReverseProxy
	if err := db.Order("name ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	pending := map[string][]string{}
	if db.Migrator().HasTable(&model.NetworkModel{}) {
		if view, err := s.network(db).Get(ctx); err == nil {
			for _, p := range view.Pending {
				if p.ReverseProxy != "" {
					pending[p.ReverseProxy] = append(pending[p.ReverseProxy], p.Reason)
				}
			}
		}
	}
	reports, err := entryReports(db)
	if err != nil {
		return nil, err
	}
	views := make([]View, 0, len(rows))
	for _, row := range rows {
		views = append(views, viewOf(row, pending, reports))
	}
	return views, nil
}

// Get answers one reverse proxy.
func (s *Service) Get(ctx context.Context, id uint) (View, error) {
	views, err := s.List(ctx)
	if err != nil {
		return View{}, err
	}
	for _, view := range views {
		if view.ID == id {
			return view, nil
		}
	}
	return View{}, ErrNotFound
}

// Create adds a reverse proxy, audited, and replans the devices.
func (s *Service) Create(ctx context.Context, input Input, actor Actor) (View, error) {
	var id uint
	err := s.write(ctx, actor, func(tx *gorm.DB) (string, *model.ReverseProxy, error) {
		var count int64
		if err := tx.Model(&model.ReverseProxy{}).Count(&count).Error; err != nil {
			return "", nil, err
		}
		if count >= MaxProxies {
			return "", nil, invalid("at most %d reverse proxies", MaxProxies)
		}
		row, err := s.validate(tx, input, nil)
		if err != nil {
			return "", nil, err
		}
		if err := requireUniqueName(tx, row.Name, 0); err != nil {
			return "", nil, err
		}
		row.CreatedAt, row.UpdatedAt = s.now(), s.now()
		enabled := row.Enabled
		if err := tx.Create(row).Error; err != nil {
			return "", nil, err
		}
		if !enabled {
			// A false Enabled takes the column's default on create.
			if err := tx.Model(row).Update("enabled", false).Error; err != nil {
				return "", nil, err
			}
			row.Enabled = false
		}
		id = row.ID
		return AuditActionCreate, row, nil
	})
	if err != nil {
		return View{}, err
	}
	return s.Get(ctx, id)
}

// Update changes a reverse proxy, audited, and replans the devices.
func (s *Service) Update(ctx context.Context, id uint, input Input, actor Actor) (View, error) {
	err := s.write(ctx, actor, func(tx *gorm.DB) (string, *model.ReverseProxy, error) {
		var current model.ReverseProxy
		if err := tx.First(&current, id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return "", nil, ErrNotFound
			}
			return "", nil, err
		}
		row, err := s.validate(tx, input, &current)
		if err != nil {
			return "", nil, err
		}
		if err := requireUniqueName(tx, row.Name, id); err != nil {
			return "", nil, err
		}
		row.ID, row.CreatedAt, row.UpdatedAt = id, current.CreatedAt, s.now()
		if err := tx.Model(&current).Select("*").Updates(row).Error; err != nil {
			return "", nil, err
		}
		return AuditActionUpdate, row, nil
	})
	if err != nil {
		return View{}, err
	}
	return s.Get(ctx, id)
}

// Delete removes a reverse proxy, audited, and replans the devices.
func (s *Service) Delete(ctx context.Context, id uint, actor Actor) error {
	return s.write(ctx, actor, func(tx *gorm.DB) (string, *model.ReverseProxy, error) {
		var current model.ReverseProxy
		if err := tx.First(&current, id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return "", nil, ErrNotFound
			}
			return "", nil, err
		}
		if err := tx.Delete(&model.ReverseProxy{}, id).Error; err != nil {
			return "", nil, err
		}
		return AuditActionDelete, &current, nil
	})
}

// write runs change in a transaction, audits it and replans the devices;
// the devices whose plan changed hear of it once the transaction commits.
func (s *Service) write(ctx context.Context, actor Actor, change func(tx *gorm.DB) (string, *model.ReverseProxy, error)) error {
	var notify func()
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		action, row, err := change(tx)
		if err != nil {
			return err
		}
		if err := writeAudit(tx, actor, action, row); err != nil {
			return err
		}
		if !tx.Migrator().HasTable(&model.NetworkModel{}) {
			return nil
		}
		notify, err = s.network(tx).ReplanTx(tx)
		if err != nil {
			var invalidModel *networkmodel.Invalid
			if errors.As(err, &invalidModel) {
				return invalid("%v", err)
			}
			return err
		}
		return nil
	})
	if err == nil && notify != nil {
		notify()
	}
	return err
}

func requireUniqueName(tx *gorm.DB, name string, except uint) error {
	var count int64
	if err := tx.Model(&model.ReverseProxy{}).Where("name = ? AND id <> ?", name, except).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return invalid("the name %q is used by another reverse proxy", name)
	}
	return nil
}

var namePattern = regexp.MustCompile(`^[^\x00-\x1f\x7f]+$`)

// validate checks input and answers the row to store; current is the
// stored row on an update.
func (s *Service) validate(tx *gorm.DB, input Input, current *model.ReverseProxy) (*model.ReverseProxy, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" || len(name) > maxNameLength || !namePattern.MatchString(name) {
		return nil, invalid("the name must be 1 to %d characters, without control characters", maxNameLength)
	}
	entries, err := nodeRefs(tx, input.Entries, "entries", 1, MaxEntries)
	if err != nil {
		return nil, err
	}
	via, err := nodeRefs(tx, input.Via, "via", 0, MaxVia)
	if err != nil {
		return nil, err
	}
	for _, ref := range via {
		for _, entry := range entries {
			if ref == entry {
				return nil, invalid("%s is an entry and a via device", ref)
			}
		}
	}
	if input.ListenPort == 0 || input.ListenPort > network.MaxPort {
		return nil, invalid("listen_port must be 1 to %d", network.MaxPort)
	}
	mode := input.TLSMode
	if mode == "" {
		mode = TLSNone
	}
	row := &model.ReverseProxy{
		Name: name, Enabled: input.Enabled == nil || *input.Enabled, ListenPort: input.ListenPort, TLSMode: mode,
	}
	switch mode {
	case TLSNone:
		if input.HTTPPort != 0 {
			return nil, invalid("http_port redirects to https; it needs TLS")
		}
	case TLSManual, TLSACME:
		if input.HTTPPort > network.MaxPort || input.HTTPPort == input.ListenPort {
			return nil, invalid("http_port must be 0 or another port from 1 to %d", network.MaxPort)
		}
		row.HTTPPort = input.HTTPPort
	default:
		return nil, invalid("tls_mode must be none, manual or acme")
	}
	sites, err := checkSites(input.Sites, input.ListenPort)
	if err != nil {
		return nil, err
	}
	for _, list := range []struct {
		field string
		value any
		out   *string
	}{{"entries", entries, &row.EntriesJSON}, {"via", via, &row.ViaJSON}, {"sites", sites, &row.SitesJSON}} {
		encoded, err := json.Marshal(list.value)
		if err != nil {
			return nil, err
		}
		*list.out = string(encoded)
	}
	switch mode {
	case TLSManual:
		if err := s.setCertificate(row, input, current, sites); err != nil {
			return nil, err
		}
	case TLSACME:
		if err := setACME(tx, row, input, current, sites); err != nil {
			return nil, err
		}
	}
	return row, nil
}

// setACME checks an acme proxy's DNS provider and zone and keeps the
// certificate it holds while that still names every host; otherwise the
// issuer obtains a new one.
func setACME(tx *gorm.DB, row *model.ReverseProxy, input Input, current *model.ReverseProxy, sites []Site) error {
	if strings.TrimSpace(input.CertificatePEM) != "" || strings.TrimSpace(input.PrivateKeyPEM) != "" {
		return invalid("tls_mode acme obtains its certificate; leave certificate_pem and private_key_pem empty")
	}
	if input.DNSProviderID == 0 {
		return invalid("tls_mode acme needs dns_provider_id, the DNS provider its challenges are written with")
	}
	var count int64
	if err := tx.Model(&model.KernelForwardDNSProvider{}).Where("id = ?", input.DNSProviderID).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return invalid("DNS provider %d does not exist", input.DNSProviderID)
	}
	zone := forwardddns.NormalizeName(input.DNSZone)
	if !forwardddns.ValidName(zone) || !strings.Contains(zone, ".") {
		return invalid("dns_zone %q is not a DNS zone", input.DNSZone)
	}
	for _, site := range sites {
		for _, host := range site.Hosts {
			if host == "*" {
				return invalid("tls_mode acme names its hosts; * has no certificate")
			}
			if !forwardddns.InZone(strings.TrimPrefix(host, "*."), zone) {
				return invalid("host %s is not in dns_zone %s", host, zone)
			}
		}
	}
	row.DNSProviderID, row.DNSZone = input.DNSProviderID, zone
	if current == nil || current.TLSMode != TLSACME || current.CertificatePEM == "" || current.SealedPrivateKey == "" {
		return nil
	}
	leaf, err := leafOf(current.CertificatePEM)
	if err != nil || covers(leaf, sites) != nil {
		return nil
	}
	row.CertificatePEM, row.SealedPrivateKey, row.CertificateNotAfter = current.CertificatePEM, current.SealedPrivateKey, current.CertificateNotAfter
	if current.DNSProviderID == row.DNSProviderID && current.DNSZone == row.DNSZone {
		row.CertificateError, row.CertificateAttemptAt = current.CertificateError, current.CertificateAttemptAt
	}
	return nil
}

// setCertificate puts the manual certificate on row: the input's, or the
// stored one when the input leaves both out.
func (s *Service) setCertificate(row *model.ReverseProxy, input Input, current *model.ReverseProxy, sites []Site) error {
	certPEM, keyPEM := strings.TrimSpace(input.CertificatePEM), strings.TrimSpace(input.PrivateKeyPEM)
	if certPEM == "" && keyPEM == "" && current != nil && current.CertificatePEM != "" && current.SealedPrivateKey != "" {
		row.CertificatePEM, row.SealedPrivateKey, row.CertificateNotAfter = current.CertificatePEM, current.SealedPrivateKey, current.CertificateNotAfter
		leaf, err := leafOf(current.CertificatePEM)
		if err != nil {
			return invalid("the stored certificate: %v", err)
		}
		return covers(leaf, sites)
	}
	if certPEM == "" || keyPEM == "" {
		return invalid("tls_mode manual needs certificate_pem and private_key_pem")
	}
	if len(certPEM) > network.MaxCertificateBytes || len(keyPEM) > network.MaxCertificateBytes {
		return invalid("certificate_pem and private_key_pem are at most %d bytes", network.MaxCertificateBytes)
	}
	pair, err := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
	if err != nil {
		return invalid("the certificate and private key do not make a pair: %v", err)
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return invalid("the certificate: %v", err)
	}
	if !leaf.NotAfter.After(s.now()) {
		return invalid("the certificate expired at %s", leaf.NotAfter.UTC().Format(time.RFC3339))
	}
	if err := covers(leaf, sites); err != nil {
		return err
	}
	sealed, err := s.seal(keyPEM)
	if err != nil {
		return err
	}
	notAfter := leaf.NotAfter.UTC()
	row.CertificatePEM, row.SealedPrivateKey, row.CertificateNotAfter = certPEM+"\n", sealed, &notAfter
	return nil
}

func leafOf(certPEM string) (*x509.Certificate, error) {
	block, _ := pem.Decode([]byte(certPEM))
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, errors.New("no certificate")
	}
	return x509.ParseCertificate(block.Bytes)
}

// covers checks that leaf names every exact and wildcard host.
func covers(leaf *x509.Certificate, sites []Site) error {
	for _, site := range sites {
		for _, host := range site.Hosts {
			if host == "*" {
				continue
			}
			probe := host
			if strings.HasPrefix(host, "*.") {
				probe = "anixops-probe" + host[1:]
			}
			if err := leaf.VerifyHostname(probe); err != nil {
				return invalid("the certificate does not cover %s", host)
			}
		}
	}
	return nil
}

// nodeRefs checks a list of proxy-<id> references to existing nodes.
func nodeRefs(tx *gorm.DB, refs []string, field string, least, most int) ([]string, error) {
	if len(refs) < least || len(refs) > most {
		return nil, invalid("%s takes %d to %d devices", field, least, most)
	}
	out := make([]string, 0, len(refs))
	seen := map[string]bool{}
	for _, ref := range refs {
		node, err := agentcontrol.ParseAgentNode(strings.TrimSpace(ref))
		if err != nil || node.ID == 0 || node.Kind != agentcontrol.NodeKindProxy {
			return nil, invalid("%s: %q is not a node reference (proxy-<id>)", field, ref)
		}
		if seen[node.String()] {
			return nil, invalid("%s: %s is listed twice", field, node.String())
		}
		seen[node.String()] = true
		var count int64
		if err := tx.Model(&model.Node{}).Where("id = ?", node.ID).Count(&count).Error; err != nil {
			return nil, err
		}
		if count == 0 {
			return nil, invalid("%s: node %d does not exist", field, node.ID)
		}
		out = append(out, node.String())
	}
	return out, nil
}

// checkSites normalizes sites and checks them as the SDK checks a plan's
// reverse proxy.
func checkSites(sites []Site, port uint32) ([]Site, error) {
	out := make([]Site, 0, len(sites))
	for _, site := range sites {
		normalized := Site{Routes: make([]Route, 0, len(site.Routes))}
		for _, host := range site.Hosts {
			normalized.Hosts = append(normalized.Hosts, strings.ToLower(strings.TrimSpace(host)))
		}
		for _, route := range site.Routes {
			route.Path = strings.TrimSpace(route.Path)
			if route.Path == "" {
				route.Path = "/"
			}
			route.Host = strings.TrimSpace(route.Host)
			for i := range route.Upstreams {
				route.Upstreams[i].Host = strings.TrimSpace(route.Upstreams[i].Host)
			}
			normalized.Routes = append(normalized.Routes, route)
		}
		out = append(out, normalized)
	}
	probe := &networkv1.ReverseProxy{Id: "rp-check", Listen: &networkv1.Endpoint{Address: "::", Port: port}, Sites: sitesProto(out, false)}
	if err := network.CheckReverseProxy(probe); err != nil {
		return nil, invalid("%s", strings.TrimPrefix(err.Error(), network.ErrInvalid.Error()+": "))
	}
	return out, nil
}

func sitesProto(sites []Site, redirect bool) []*networkv1.ReverseProxySite {
	out := make([]*networkv1.ReverseProxySite, 0, len(sites))
	for _, site := range sites {
		entry := &networkv1.ReverseProxySite{Hosts: append([]string(nil), site.Hosts...)}
		if redirect {
			entry.RedirectHttps = true
			out = append(out, entry)
			continue
		}
		for _, route := range site.Routes {
			r := &networkv1.ReverseProxyRoute{
				Path: route.Path, StripPrefix: route.StripPrefix, HostHeader: route.HostHeader, Host: route.Host, Headers: route.Headers,
			}
			for _, upstream := range route.Upstreams {
				r.Upstreams = append(r.Upstreams, &networkv1.ForwardTarget{
					Host: upstream.Host, Port: upstream.Port, Weight: upstream.Weight, Priority: upstream.Priority,
				})
			}
			if route.Balance != "" || route.Health != nil {
				r.Policy = &networkv1.ForwardPolicy{Balance: route.Balance}
				if h := route.Health; h != nil {
					r.Policy.Health = &networkv1.ForwardHealth{IntervalMs: h.IntervalMs, TimeoutMs: h.TimeoutMs, Disabled: h.Disabled}
				}
			}
			entry.Routes = append(entry.Routes, r)
		}
		out = append(out, entry)
	}
	return out
}

// Forwards answers the enabled reverse proxies as the network model's
// forwards (networkmodel.RegisterForwardSource): one per proxy, and one
// more for its plain redirect listener. A proxy whose private key does not
// unseal is left out.
func Forwards(db *gorm.DB) ([]networkmodel.Forward, error) {
	return (&Service{DB: db}).Forwards(db)
}

// Forwards is the package's Forwards with the service's key-encryption
// key.
func (s *Service) Forwards(db *gorm.DB) ([]networkmodel.Forward, error) {
	if !db.Migrator().HasTable(&model.ReverseProxy{}) {
		return nil, nil
	}
	var rows []model.ReverseProxy
	if err := db.Where("enabled = ?", true).Order("id").Find(&rows).Error; err != nil {
		return nil, err
	}
	var forwards []networkmodel.Forward
	for _, row := range rows {
		var entries, via []string
		var sites []Site
		if json.Unmarshal([]byte(row.EntriesJSON), &entries) != nil || json.Unmarshal([]byte(row.ViaJSON), &via) != nil ||
			json.Unmarshal([]byte(row.SitesJSON), &sites) != nil || len(entries) == 0 {
			continue
		}
		spec := &networkmodel.ReverseProxySpec{Sites: sitesProto(sites, false)}
		if row.TLSMode != TLSNone {
			if row.CertificatePEM == "" || row.SealedPrivateKey == "" {
				continue
			}
			key, err := s.open(row.SealedPrivateKey)
			if err != nil {
				continue
			}
			spec.CertificatePEM, spec.PrivateKeyPEM = row.CertificatePEM, key
		}
		base := networkmodel.Forward{
			ID: IDPrefix + strconv.FormatUint(uint64(row.ID), 10), Entries: entries,
			Listen: networkmodel.Endpoint{Address: "::", Port: row.ListenPort}, Via: via, Segments: make([]string, len(via)),
			ReverseProxy: spec,
		}
		forwards = append(forwards, base)
		if row.HTTPPort != 0 && spec.CertificatePEM != "" {
			redirect := base
			redirect.ID += HTTPSuffix
			redirect.Listen.Port = row.HTTPPort
			redirect.Segments = make([]string, len(via))
			redirect.ReverseProxy = &networkmodel.ReverseProxySpec{Sites: sitesProto(sites, true)}
			forwards = append(forwards, redirect)
		}
	}
	return forwards, nil
}

// entryReports answers what each device last reported of its reverse
// proxies, by plan id.
func entryReports(db *gorm.DB) (map[string][]EntryStatus, error) {
	reports := map[string][]EntryStatus{}
	if !db.Migrator().HasTable(&model.NetworkDevice{}) {
		return reports, nil
	}
	var rows []model.NetworkDevice
	if err := db.Select("node_ref", "status_json").Where("status_json <> ''").Order("node_ref").Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		var status networkv1.NetworkStatus
		if protojson.Unmarshal([]byte(row.StatusJSON), &status) != nil {
			continue
		}
		for _, proxy := range status.GetReverseProxies() {
			reports[proxy.GetId()] = append(reports[proxy.GetId()], EntryStatus{
				Node: row.NodeRef, ID: proxy.GetId(), State: strings.TrimPrefix(proxy.GetState().String(), "COMPONENT_STATE_"),
				Detail: proxy.GetDetail(), UpBytes: proxy.GetUpBytes(), DownBytes: proxy.GetDownBytes(), Upstreams: proxy.GetUpstreams(),
			})
		}
	}
	return reports, nil
}

func viewOf(row model.ReverseProxy, pending map[string][]string, reports map[string][]EntryStatus) View {
	view := View{
		ID: row.ID, Name: row.Name, Enabled: row.Enabled, ListenPort: row.ListenPort, HTTPPort: row.HTTPPort,
		TLSMode: row.TLSMode, DNSProviderID: row.DNSProviderID, DNSZone: row.DNSZone, HasCertificate: row.CertificatePEM != "" && row.SealedPrivateKey != "",
		CertificateNotAfter: row.CertificateNotAfter, CertificateError: row.CertificateError,
		Entries: []string{}, Via: []string{}, Sites: []Site{}, CertificateNames: []string{}, Pending: []string{}, Status: []EntryStatus{},
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
	_ = json.Unmarshal([]byte(row.EntriesJSON), &view.Entries)
	_ = json.Unmarshal([]byte(row.ViaJSON), &view.Via)
	_ = json.Unmarshal([]byte(row.SitesJSON), &view.Sites)
	if view.HasCertificate {
		if leaf, err := leafOf(row.CertificatePEM); err == nil {
			view.CertificateNames = append(view.CertificateNames, leaf.DNSNames...)
		}
	}
	id := IDPrefix + strconv.FormatUint(uint64(row.ID), 10)
	for _, planID := range []string{id, id + HTTPSuffix} {
		view.Pending = append(view.Pending, pending[planID]...)
		view.Status = append(view.Status, reports[planID]...)
	}
	if row.Enabled && row.TLSMode != TLSNone && !view.HasCertificate {
		view.Pending = append(view.Pending, "waiting for a certificate")
	}
	sort.SliceStable(view.Status, func(i, j int) bool { return view.Status[i].Node < view.Status[j].Node })
	return view
}

// writeAudit records the change without the certificate or key.
func writeAudit(tx *gorm.DB, actor Actor, action string, row *model.ReverseProxy) error {
	var hosts []string
	var sites []Site
	_ = json.Unmarshal([]byte(row.SitesJSON), &sites)
	for _, site := range sites {
		hosts = append(hosts, site.Hosts...)
	}
	encoded, err := json.Marshal(map[string]any{
		"name": row.Name, "enabled": row.Enabled, "entries": json.RawMessage(orArray(row.EntriesJSON)), "via": json.RawMessage(orArray(row.ViaJSON)),
		"listen_port": row.ListenPort, "http_port": row.HTTPPort, "tls_mode": row.TLSMode, "hosts": hosts,
	})
	if err != nil {
		return err
	}
	var userID *uint
	if actor.UserID != 0 {
		id := actor.UserID
		userID = &id
	}
	target := row.ID
	return tx.Create(&model.OperationLog{
		UserID: userID, Username: truncate(actor.Name, 100), Action: action, Module: auditModule,
		TargetType: auditTarget, TargetID: &target, Content: string(encoded), IP: truncate(actor.IP, 45), Status: 1,
	}).Error
}

func orArray(raw string) string {
	if raw == "" {
		return "[]"
	}
	return raw
}

func truncate(value string, limit int) string {
	if len(value) > limit {
		return value[:limit]
	}
	return value
}

func (s *Service) aead() (cipher.AEAD, error) {
	var raw string
	if s.KEK != nil {
		raw = s.KEK()
	} else if cfg := config.Get(); cfg != nil {
		raw = cfg.ModuleRuntime.CAKEK
	}
	if strings.TrimSpace(raw) == "" {
		return nil, ErrNoKEK
	}
	kek, err := modulepki.ParseKEK(raw)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(kek)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func (s *Service) seal(key string) (string, error) { return s.sealWith(keyAdditionalData, key) }

func (s *Service) open(sealed string) (string, error) { return s.openWith(keyAdditionalData, sealed) }

// sealWith seals plain under the key-encryption key, bound to ad.
func (s *Service) sealWith(ad, key string) (string, error) {
	aead, err := s.aead()
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(aead.Seal(nonce, nonce, []byte(key), []byte(ad))), nil
}

func (s *Service) openWith(ad, sealed string) (string, error) {
	aead, err := s.aead()
	if err != nil {
		return "", err
	}
	raw, err := base64.StdEncoding.DecodeString(sealed)
	if err != nil || len(raw) < aead.NonceSize() {
		return "", errors.New("reverse proxy: a sealed key is corrupt")
	}
	plain, err := aead.Open(nil, raw[:aead.NonceSize()], raw[aead.NonceSize():], []byte(ad))
	if err != nil {
		return "", errors.New("reverse proxy: a sealed key does not open with this key-encryption key")
	}
	return string(plain), nil
}
