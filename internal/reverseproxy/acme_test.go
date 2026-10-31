package reverseproxy

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/forwardddns"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func acmeService(t *testing.T) *Service {
	t.Helper()
	s := newService(t)
	require.NoError(t, s.DB.AutoMigrate(&model.KernelForwardDNSProvider{}, &model.ReverseProxyACMEAccount{}))
	require.NoError(t, s.DB.Create(&model.KernelForwardDNSProvider{ID: 7, Name: "cf", Kind: forwardddns.KindCloudflare,
		CreatedAt: testNow, UpdatedAt: testNow}).Error)
	return s
}

func acmeInput() Input {
	input := blogInput("", "")
	input.TLSMode, input.DNSProviderID, input.DNSZone = TLSACME, 7, "Example.com."
	return input
}

// An acme proxy names a DNS provider and a zone that holds its hosts; it
// waits for its certificate and is not planned before it has one.
func TestACMEProxiesWaitForTheirCertificate(t *testing.T) {
	s := acmeService(t)
	ctx := context.Background()
	for name, change := range map[string]func(*Input){
		"no provider":      func(in *Input) { in.DNSProviderID = 0 },
		"unknown provider": func(in *Input) { in.DNSProviderID = 9 },
		"no zone":          func(in *Input) { in.DNSZone = "" },
		"host outside":     func(in *Input) { in.DNSZone = "example.net" },
		"catch-all host":   func(in *Input) { in.Sites[0].Hosts = append(in.Sites[0].Hosts, "*") },
		"a certificate":    func(in *Input) { in.CertificatePEM, in.PrivateKeyPEM = selfSigned(t, "blog.example.com") },
	} {
		input := acmeInput()
		change(&input)
		_, err := s.Create(ctx, input, Actor{Name: "admin"})
		require.ErrorIs(t, err, ErrInvalid, name)
	}
	view, err := s.Create(ctx, acmeInput(), Actor{Name: "admin"})
	require.NoError(t, err)
	require.Equal(t, "example.com", view.DNSZone)
	require.False(t, view.HasCertificate)
	require.Contains(t, view.Pending, "waiting for a certificate")
	forwards, err := s.Forwards(s.DB)
	require.NoError(t, err)
	require.Empty(t, forwards)

	issuer := &Issuer{Service: s}
	var row model.ReverseProxy
	require.NoError(t, s.DB.First(&row, view.ID).Error)
	require.True(t, issuer.due(row, testNow))
	attempt := testNow.Add(-time.Minute)
	row.CertificateError, row.CertificateAttemptAt = "rate limited", &attempt
	require.False(t, issuer.due(row, testNow), "a failure waits an hour")
	require.True(t, issuer.due(row, testNow.Add(time.Hour)))
	notAfter := testNow.Add(60 * 24 * time.Hour)
	row.CertificateError, row.CertificatePEM, row.SealedPrivateKey, row.CertificateNotAfter = "", "x", "y", &notAfter
	require.False(t, issuer.due(row, testNow))
	require.True(t, issuer.due(row, testNow.Add(31*24*time.Hour)), "renewed 30 days before it expires")

	names, err := acmeNames(row.SitesJSON)
	require.NoError(t, err)
	require.Equal(t, []string{"blog.example.com", "*.blog.example.com"}, names)
}

// challtestsrv writes the challenge TXT records into Pebble's test DNS
// server through its management API.
type challtestsrv struct {
	url string
	mu  sync.Mutex
	set map[string][]string
}

func (c *challtestsrv) Kind() string { return "challtestsrv" }

func (c *challtestsrv) post(ctx context.Context, path string, body any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url+path, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: %s", path, response.Status)
	}
	return nil
}

func (c *challtestsrv) SetRecords(ctx context.Context, set forwardddns.RecordSet) error {
	if err := forwardddns.CheckSet(set, false); err != nil {
		return err
	}
	for _, value := range set.Values {
		if err := c.post(ctx, "/set-txt", map[string]string{"host": set.Name + ".", "value": value}); err != nil {
			return err
		}
	}
	c.mu.Lock()
	c.set[set.Name] = set.Values
	c.mu.Unlock()
	return nil
}

func (c *challtestsrv) DeleteRecords(ctx context.Context, _, name, _ string) error {
	c.mu.Lock()
	delete(c.set, name)
	c.mu.Unlock()
	return c.post(ctx, "/clear-txt", map[string]string{"host": name + "."})
}

// The issuer against a real ACME server: Pebble, validating DNS-01
// through pebble-challtestsrv (the CI job that runs
// TestPebbleIssuesOverBothChallenges starts both). An apex and a
// wildcard name share one TXT name with two values; the certificate is
// stored, the records removed and the proxy planned.
func TestPebbleIssuesDNS01(t *testing.T) {
	directory, mgmt := os.Getenv("ANIX_CONTROL_TEST_PEBBLE_DIRECTORY"), os.Getenv("ANIX_CONTROL_TEST_PEBBLE_CHALLTESTSRV")
	if directory == "" || mgmt == "" {
		t.Skip("ANIX_CONTROL_TEST_PEBBLE_DIRECTORY and ANIX_CONTROL_TEST_PEBBLE_CHALLTESTSRV are not set: no Pebble to test against")
	}
	caPEM, err := os.ReadFile(os.Getenv("ANIX_CONTROL_TEST_PEBBLE_CA_FILE"))
	require.NoError(t, err)
	roots := x509.NewCertPool()
	require.True(t, roots.AppendCertsFromPEM(caPEM))
	client := &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}}}

	s := acmeService(t)
	s.Now = time.Now
	ctx := context.Background()
	input := acmeInput()
	input.Sites[0].Hosts = []string{"blog.example.com", "*.blog.example.com"}
	view, err := s.Create(ctx, input, Actor{Name: "admin"})
	require.NoError(t, err)

	dns := &challtestsrv{url: strings.TrimSuffix(mgmt, "/"), set: map[string][]string{}}
	var written map[string][]string
	issuer := &Issuer{
		Service: s, Directory: directory, HTTPClient: client, Logf: t.Logf,
		DNS: func(*gorm.DB, uint64) (forwardddns.Provider, error) { return dns, nil },
		Propagated: func(_ context.Context, records map[string][]string) error {
			written = records
			return nil
		},
	}
	require.NoError(t, issuer.Step(ctx))
	require.Len(t, written["_acme-challenge.blog.example.com"], 2, "the apex and the wildcard share one TXT name")
	require.Empty(t, dns.set, "the challenge records are removed")

	got, err := s.Get(ctx, view.ID)
	require.NoError(t, err)
	require.True(t, got.HasCertificate, got.CertificateError)
	require.Empty(t, got.CertificateError)
	require.ElementsMatch(t, []string{"blog.example.com", "*.blog.example.com"}, got.CertificateNames)
	forwards, err := s.Forwards(s.DB)
	require.NoError(t, err)
	require.Len(t, forwards, 2, "the proxy and its redirect listener")
	_, err = tls.X509KeyPair([]byte(forwards[0].ReverseProxy.CertificatePEM), []byte(forwards[0].ReverseProxy.PrivateKeyPEM))
	require.NoError(t, err)

	// The account is kept: a second issuance reuses it.
	var accounts int64
	require.NoError(t, s.DB.Model(&model.ReverseProxyACMEAccount{}).Count(&accounts).Error)
	require.EqualValues(t, 1, accounts)
	require.NoError(t, issuer.Issue(ctx, view.ID))
	require.NoError(t, s.DB.Model(&model.ReverseProxyACMEAccount{}).Count(&accounts).Error)
	require.EqualValues(t, 1, accounts)
}
