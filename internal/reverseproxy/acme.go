package reverseproxy

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/forwardddns"
	"github.com/AnixOps/anix-control/v5/internal/kernelforward"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
	"gorm.io/gorm"
)

// The issuer's defaults.
const (
	// DefaultInterval is how often the issuer looks for due proxies.
	DefaultInterval = time.Minute
	// DefaultRenewBefore renews a certificate this long before it expires.
	DefaultRenewBefore = 30 * 24 * time.Hour
	// DefaultRetryAfter is the wait after a failed attempt, which keeps
	// the issuer well inside the public CA's failed-validation limit.
	DefaultRetryAfter = time.Hour
	// DefaultAttemptTimeout bounds one issuance.
	DefaultAttemptTimeout = 10 * time.Minute
	// challengeTTL is the TTL of the challenge TXT records.
	challengeTTL = 60
	// AuditActionCertificate records a certificate the issuer stored.
	AuditActionCertificate = "reverse_proxy_certificate"
	// accountAdditionalData binds a sealed ACME account key to its use.
	accountAdditionalData = "anixops/reverse-proxy/acme-account"
	maxCertificateError   = 1000
)

// errStale: the proxy changed while its certificate was being issued.
var errStale = errors.New("reverse proxy: changed while its certificate was issued")

// Issuer obtains and renews the certificates of the acme-mode reverse
// proxies: an ACME order for the sites' hosts, its DNS-01 challenges
// answered with TXT records written through the proxy's DNS provider, the
// certificate and its sealed key stored on the proxy, and the devices
// replanned. It runs in the lease holder only (cmd/server).
type Issuer struct {
	Service *Service
	// Directory is the ACME directory; empty is the public CA's
	// (autocert.DefaultACMEDirectory).
	Directory string
	// HTTPClient talks to the directory; nil is http.DefaultClient.
	HTTPClient *http.Client
	// DNS builds the client of a stored DNS provider; nil uses the
	// forwarding DNS providers (kernelforward).
	DNS func(db *gorm.DB, id uint64) (forwardddns.Provider, error)
	// Propagated waits until the TXT records are visible; nil looks them
	// up with the system resolver for up to two minutes.
	Propagated func(ctx context.Context, records map[string][]string) error
	// Interval, RenewBefore, RetryAfter and AttemptTimeout default to the
	// constants above.
	Interval, RenewBefore, RetryAfter, AttemptTimeout time.Duration
	// Logf defaults to log.Printf.
	Logf func(format string, args ...any)
}

func (i *Issuer) logf(format string, args ...any) {
	if i.Logf != nil {
		i.Logf(format, args...)
		return
	}
	log.Printf(format, args...)
}

func orDuration(value, fallback time.Duration) time.Duration {
	if value > 0 {
		return value
	}
	return fallback
}

func (i *Issuer) directory() string {
	if d := strings.TrimSpace(i.Directory); d != "" {
		return d
	}
	return autocert.DefaultACMEDirectory
}

// Run issues due certificates until ctx ends.
func (i *Issuer) Run(ctx context.Context) {
	ticker := time.NewTicker(orDuration(i.Interval, DefaultInterval))
	defer ticker.Stop()
	for {
		if err := i.Step(ctx); err != nil && ctx.Err() == nil {
			i.logf("reverse proxy certificates: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Step issues the certificate of every due acme proxy, one at a time.
func (i *Issuer) Step(ctx context.Context) error {
	db := i.Service.DB.WithContext(ctx)
	if !db.Migrator().HasTable(&model.ReverseProxy{}) {
		return nil
	}
	var rows []model.ReverseProxy
	if err := db.Where("enabled = ? AND tls_mode = ?", true, TLSACME).Order("id").Find(&rows).Error; err != nil {
		return err
	}
	now := i.Service.now()
	for _, row := range rows {
		if !i.due(row, now) {
			continue
		}
		if err := i.Issue(ctx, row.ID); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			i.logf("reverse proxy %q: certificate: %v", row.Name, err)
		}
	}
	return nil
}

// due reports whether row needs a certificate now: it has none, or its
// own expires within RenewBefore; after a failed attempt it waits
// RetryAfter.
func (i *Issuer) due(row model.ReverseProxy, now time.Time) bool {
	if row.CertificateError != "" && row.CertificateAttemptAt != nil &&
		now.Sub(*row.CertificateAttemptAt) < orDuration(i.RetryAfter, DefaultRetryAfter) {
		return false
	}
	if row.CertificatePEM == "" || row.SealedPrivateKey == "" || row.CertificateNotAfter == nil {
		return true
	}
	return row.CertificateNotAfter.Sub(now) < orDuration(i.RenewBefore, DefaultRenewBefore)
}

// Issue obtains a certificate for proxy id now and stores it; a failure
// is stored on the proxy as its certificate_error.
func (i *Issuer) Issue(ctx context.Context, id uint) error {
	db := i.Service.DB.WithContext(ctx)
	var row model.ReverseProxy
	if err := db.First(&row, id).Error; err != nil {
		return err
	}
	if row.TLSMode != TLSACME {
		return fmt.Errorf("reverse proxy %d is not in tls_mode acme", id)
	}
	attempt := i.Service.now()
	if err := db.Model(&model.ReverseProxy{}).Where("id = ?", id).Update("certificate_attempt_at", attempt).Error; err != nil {
		return err
	}
	attemptCtx, cancel := context.WithTimeout(ctx, orDuration(i.AttemptTimeout, DefaultAttemptTimeout))
	defer cancel()
	certPEM, keyPEM, err := i.obtain(attemptCtx, row)
	if err == nil {
		err = i.store(ctx, row, certPEM, keyPEM)
	}
	if err != nil && !errors.Is(err, errStale) {
		message := truncate(err.Error(), maxCertificateError)
		if updateErr := i.Service.DB.WithContext(context.WithoutCancel(ctx)).Model(&model.ReverseProxy{}).
			Where("id = ?", id).Update("certificate_error", message).Error; updateErr != nil {
			return errors.Join(err, updateErr)
		}
	}
	return err
}

// acmeNames are the hosts of the sites, lowercased, without duplicates.
func acmeNames(sitesJSON string) ([]string, error) {
	var sites []Site
	if err := json.Unmarshal([]byte(sitesJSON), &sites); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var names []string
	for _, site := range sites {
		for _, host := range site.Hosts {
			host = strings.ToLower(host)
			if host == "*" || seen[host] {
				continue
			}
			seen[host] = true
			names = append(names, host)
		}
	}
	if len(names) == 0 {
		return nil, errors.New("the sites name no host")
	}
	return names, nil
}

// obtain runs one ACME order for row's hosts and answers the chain and
// the private key as PEM.
func (i *Issuer) obtain(ctx context.Context, row model.ReverseProxy) (string, string, error) {
	names, err := acmeNames(row.SitesJSON)
	if err != nil {
		return "", "", err
	}
	provider, err := i.provider(ctx, row.DNSProviderID)
	if err != nil {
		return "", "", fmt.Errorf("DNS provider %d: %w", row.DNSProviderID, err)
	}
	client, err := i.client(ctx)
	if err != nil {
		return "", "", fmt.Errorf("ACME account: %w", err)
	}
	order, err := client.AuthorizeOrder(ctx, acme.DomainIDs(names...))
	if err != nil {
		return "", "", fmt.Errorf("ACME order: %w", err)
	}
	records := map[string][]string{}
	var challenges []*acme.Challenge
	var pending []string
	for _, url := range order.AuthzURLs {
		authz, err := client.GetAuthorization(ctx, url)
		if err != nil {
			return "", "", fmt.Errorf("ACME authorization: %w", err)
		}
		if authz.Status == acme.StatusValid {
			continue
		}
		var challenge *acme.Challenge
		for _, c := range authz.Challenges {
			if c.Type == "dns-01" {
				challenge = c
				break
			}
		}
		if challenge == nil {
			return "", "", fmt.Errorf("the CA offers no dns-01 challenge for %s", authz.Identifier.Value)
		}
		value, err := client.DNS01ChallengeRecord(challenge.Token)
		if err != nil {
			return "", "", err
		}
		name := "_acme-challenge." + strings.TrimPrefix(strings.ToLower(authz.Identifier.Value), "*.")
		records[name] = append(records[name], value)
		challenges = append(challenges, challenge)
		pending = append(pending, url)
	}
	defer i.clean(ctx, provider, row.DNSZone, records)
	for _, name := range sortedNames(records) {
		set := forwardddns.RecordSet{Zone: row.DNSZone, Name: name, Type: forwardddns.TypeTXT, Values: records[name], TTL: challengeTTL}
		if err := provider.SetRecords(ctx, set); err != nil {
			return "", "", fmt.Errorf("write TXT %s: %w", name, err)
		}
	}
	if len(records) > 0 {
		if err := i.propagated(ctx, records); err != nil {
			return "", "", err
		}
	}
	for _, challenge := range challenges {
		if _, err := client.Accept(ctx, challenge); err != nil {
			return "", "", fmt.Errorf("ACME challenge: %w", err)
		}
	}
	for _, url := range pending {
		if _, err := client.WaitAuthorization(ctx, url); err != nil {
			return "", "", fmt.Errorf("ACME validation: %w", err)
		}
	}
	// A polled order carries no Location: keep the order's URL.
	orderURL := order.URI
	if order, err = client.WaitOrder(ctx, orderURL); err != nil {
		return "", "", fmt.Errorf("ACME order: %w", err)
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", err
	}
	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{DNSNames: names}, key)
	if err != nil {
		return "", "", err
	}
	chain, _, err := client.CreateOrderCert(ctx, order.FinalizeURL, csr, true)
	if err != nil {
		// A CA still processing the finalized order answers it without a
		// Location, which CreateOrderCert needs to poll: poll the order's
		// own URL and fetch the certificate it names.
		processed, waitErr := client.WaitOrder(ctx, orderURL)
		if waitErr != nil || processed.Status != acme.StatusValid || processed.CertURL == "" {
			return "", "", fmt.Errorf("ACME certificate: %w", err)
		}
		if chain, err = client.FetchCert(ctx, processed.CertURL, true); err != nil {
			return "", "", fmt.Errorf("ACME certificate: %w", err)
		}
	}
	var certPEM strings.Builder
	for _, der := range chain {
		_ = pem.Encode(&certPEM, &pem.Block{Type: "CERTIFICATE", Bytes: der})
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return "", "", err
	}
	return certPEM.String(), string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})), nil
}

func sortedNames(records map[string][]string) []string {
	names := make([]string, 0, len(records))
	for name := range records {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// clean removes the challenge records, also when ctx has ended.
func (i *Issuer) clean(ctx context.Context, provider forwardddns.Provider, zone string, records map[string][]string) {
	cleanCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
	defer cancel()
	for _, name := range sortedNames(records) {
		if err := provider.DeleteRecords(cleanCtx, zone, name, forwardddns.TypeTXT); err != nil {
			i.logf("reverse proxy certificates: remove TXT %s: %v", name, err)
		}
	}
}

func (i *Issuer) provider(ctx context.Context, id uint) (forwardddns.Provider, error) {
	db := i.Service.DB.WithContext(ctx)
	if i.DNS != nil {
		return i.DNS(db, uint64(id))
	}
	return kernelforward.New(db).DNSProvider(db, uint64(id))
}

// propagated waits until a resolver answers every challenge value, or
// two minutes, whichever is first: the CA may see them sooner than the
// local resolver does.
func (i *Issuer) propagated(ctx context.Context, records map[string][]string) error {
	if i.Propagated != nil {
		return i.Propagated(ctx, records)
	}
	deadline := time.Now().Add(2 * time.Minute)
	for {
		visible := true
		for name, values := range records {
			answer, err := net.DefaultResolver.LookupTXT(ctx, name)
			if err != nil || !containsAll(answer, values) {
				visible = false
				break
			}
		}
		if visible || time.Now().After(deadline) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
}

func containsAll(have, want []string) bool {
	set := map[string]bool{}
	for _, value := range have {
		set[value] = true
	}
	for _, value := range want {
		if !set[value] {
			return false
		}
	}
	return true
}

// client is the ACME client on Control's account at the directory,
// registered on first use; the account key is sealed under the
// key-encryption key.
func (i *Issuer) client(ctx context.Context) (*acme.Client, error) {
	directory := i.directory()
	db := i.Service.DB.WithContext(ctx)
	var rows []model.ReverseProxyACMEAccount
	if err := db.Where("directory = ?", directory).Limit(1).Find(&rows).Error; err != nil {
		return nil, err
	}
	client := &acme.Client{DirectoryURL: directory, HTTPClient: i.HTTPClient, UserAgent: "anix-control"}
	if len(rows) == 1 && rows[0].SealedKey != "" {
		keyPEM, err := i.Service.openWith(accountAdditionalData, rows[0].SealedKey)
		if err != nil {
			return nil, err
		}
		key, err := parseKey(keyPEM)
		if err != nil {
			return nil, err
		}
		client.Key = key
		if rows[0].AccountURL != "" {
			client.KID = acme.KeyID(rows[0].AccountURL)
			return client, nil
		}
	} else {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return nil, err
		}
		client.Key = key
	}
	account, err := client.Register(ctx, &acme.Account{}, acme.AcceptTOS)
	if errors.Is(err, acme.ErrAccountAlreadyExists) {
		account, err = client.GetReg(ctx, "")
	}
	if err != nil {
		return nil, err
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(client.Key)
	if err != nil {
		return nil, err
	}
	sealed, err := i.Service.sealWith(accountAdditionalData, string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})))
	if err != nil {
		return nil, err
	}
	now := i.Service.now()
	stored := model.ReverseProxyACMEAccount{Directory: directory, SealedKey: sealed, AccountURL: account.URI, CreatedAt: now, UpdatedAt: now}
	if len(rows) == 1 {
		stored.ID, stored.CreatedAt = rows[0].ID, rows[0].CreatedAt
	}
	if err := db.Save(&stored).Error; err != nil {
		return nil, err
	}
	client.KID = acme.KeyID(account.URI)
	return client, nil
}

func parseKey(keyPEM string) (crypto.Signer, error) {
	block, _ := pem.Decode([]byte(keyPEM))
	if block == nil {
		return nil, errors.New("the ACME account key is not PEM")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	signer, ok := key.(crypto.Signer)
	if !ok {
		return nil, errors.New("the ACME account key cannot sign")
	}
	return signer, nil
}

// store puts the issued certificate on the proxy, audited, and replans
// the devices, unless the proxy changed meanwhile.
func (i *Issuer) store(ctx context.Context, issuedFor model.ReverseProxy, certPEM, keyPEM string) error {
	leaf, err := leafOf(certPEM)
	if err != nil {
		return err
	}
	sealed, err := i.Service.seal(keyPEM)
	if err != nil {
		return err
	}
	notAfter := leaf.NotAfter.UTC()
	actor := Actor{Name: "system:acme"}
	return i.Service.write(ctx, actor, func(tx *gorm.DB) (string, *model.ReverseProxy, error) {
		var current model.ReverseProxy
		if err := tx.First(&current, issuedFor.ID).Error; err != nil {
			return "", nil, err
		}
		if current.TLSMode != TLSACME || current.SitesJSON != issuedFor.SitesJSON ||
			current.DNSProviderID != issuedFor.DNSProviderID || current.DNSZone != issuedFor.DNSZone {
			return "", nil, errStale
		}
		if err := tx.Model(&current).Updates(map[string]any{
			"certificate_pem": certPEM, "sealed_private_key": sealed, "certificate_not_after": notAfter,
			"certificate_error": "", "updated_at": i.Service.now(),
		}).Error; err != nil {
			return "", nil, err
		}
		return AuditActionCertificate, &current, nil
	})
}
