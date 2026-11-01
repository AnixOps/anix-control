package reverseproxy

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/network"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/networkmodel"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const testKEK = "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8="

var testNow = time.Date(2026, 10, 31, 8, 0, 0, 0, time.UTC)

func newService(t *testing.T) *Service {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "rp.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(append([]any{&model.Node{}, &model.ForwardNode{}, &model.OperationLog{}, &model.ReverseProxy{}},
		model.NetworkModels()...)...))
	for _, id := range []uint{1, 2} {
		require.NoError(t, db.Create(&model.Node{ID: id, Name: "n", Host: "h.example.com"}).Error)
	}
	kek := func() string { return testKEK }
	now := func() time.Time { return testNow }
	s := &Service{DB: db, KEK: kek, Now: now}
	s.Network = func(db *gorm.DB) *networkmodel.Service {
		return &networkmodel.Service{DB: db, KEK: kek, Now: now, Forwards: s.Forwards}
	}
	return s
}

func selfSigned(t *testing.T, names ...string) (string, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: names[0]}, DNSNames: names,
		NotBefore: testNow.Add(-time.Hour), NotAfter: testNow.Add(90 * 24 * time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
		string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}))
}

func blogInput(cert, key string) Input {
	return Input{
		Name: "blog", Entries: []string{"proxy-1"}, ListenPort: 443, HTTPPort: 80, TLSMode: TLSManual,
		CertificatePEM: cert, PrivateKeyPEM: key,
		Sites: []Site{{Hosts: []string{"Blog.Example.com", "*.blog.example.com"}, Routes: []Route{
			{Path: "/api", StripPrefix: true, Upstreams: []Upstream{{Host: "10.0.0.5", Port: 8080}}, Balance: "least_conn"},
			{Path: "/", Upstreams: []Upstream{{Host: "10.0.0.6", Port: 80}}},
		}}},
	}
}

// A reverse proxy is planned on its entry with its certificate and a
// plain redirect listener; the API never answers the key, an update
// without one keeps it, and a deleted proxy leaves the plan.
func TestReverseProxyLifecycle(t *testing.T) {
	s := newService(t)
	ctx := context.Background()
	cert, key := selfSigned(t, "blog.example.com", "*.blog.example.com")

	view, err := s.Create(ctx, blogInput(cert, key), Actor{UserID: 1, Name: "admin"})
	require.NoError(t, err)
	require.Equal(t, "blog", view.Name)
	require.True(t, view.HasCertificate)
	require.Equal(t, []string{"blog.example.com", "*.blog.example.com"}, view.CertificateNames)
	require.Equal(t, "blog.example.com", view.Sites[0].Hosts[0])

	plan, found, err := s.network(s.DB).DevicePlan(ctx, "proxy-1")
	require.NoError(t, err)
	require.True(t, found)
	require.NoError(t, network.Check(plan))
	require.Len(t, plan.GetReverseProxies(), 2)
	tls, redirect := plan.GetReverseProxies()[0], plan.GetReverseProxies()[1]
	require.Equal(t, "rp-1", tls.GetId())
	require.Equal(t, strings.TrimSpace(key), strings.TrimSpace(tls.GetPrivateKeyPem()))
	require.Equal(t, uint32(443), tls.GetListen().GetPort())
	require.NotEmpty(t, tls.GetEgressId())
	require.Equal(t, "least_conn", tls.GetSites()[0].GetRoutes()[0].GetPolicy().GetBalance())
	require.Equal(t, "rp-1-http", redirect.GetId())
	require.True(t, redirect.GetSites()[0].GetRedirectHttps())
	require.Equal(t, "redacted", networkmodel.Redacted(plan).GetReverseProxies()[0].GetPrivateKeyPem())

	// The stored key is sealed.
	var row model.ReverseProxy
	require.NoError(t, s.DB.First(&row, view.ID).Error)
	require.NotContains(t, row.SealedPrivateKey, "PRIVATE KEY")

	// Update without the certificate keeps it.
	input := blogInput("", "")
	input.Name = "blog 2"
	view, err = s.Update(ctx, view.ID, input, Actor{})
	require.NoError(t, err)
	require.Equal(t, "blog 2", view.Name)
	require.True(t, view.HasCertificate)

	var audits int64
	require.NoError(t, s.DB.Model(&model.OperationLog{}).Where("module = ?", auditModule).Count(&audits).Error)
	require.Equal(t, int64(2), audits)

	require.NoError(t, s.Delete(ctx, view.ID, Actor{}))
	plan, _, err = s.network(s.DB).DevicePlan(ctx, "proxy-1")
	require.NoError(t, err)
	require.Empty(t, plan.GetReverseProxies())
	_, err = s.Get(ctx, view.ID)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestReverseProxyRefuses(t *testing.T) {
	cert, key := selfSigned(t, "blog.example.com", "*.blog.example.com")
	other, otherKey := selfSigned(t, "other.example.com")
	cases := map[string]struct {
		edit func(in *Input)
		want string
	}{
		"no name":          {func(in *Input) { in.Name = " " }, "name"},
		"no entry":         {func(in *Input) { in.Entries = nil }, "entries"},
		"unknown node":     {func(in *Input) { in.Entries = []string{"proxy-9"} }, "does not exist"},
		"forward ref":      {func(in *Input) { in.Entries = []string{"forward-1"} }, "proxy-<id>"},
		"entry is via":     {func(in *Input) { in.Via = []string{"proxy-1"} }, "entry and a via"},
		"no port":          {func(in *Input) { in.ListenPort = 0 }, "listen_port"},
		"same http port":   {func(in *Input) { in.HTTPPort = 443 }, "http_port"},
		"http without tls": {func(in *Input) { in.TLSMode = TLSNone }, "needs TLS"},
		"acme":             {func(in *Input) { in.TLSMode = TLSACME }, "acme"},
		"bad mode":         {func(in *Input) { in.TLSMode = "x" }, "tls_mode"},
		"no key":           {func(in *Input) { in.PrivateKeyPEM = "" }, "needs certificate_pem"},
		"not a pair":       {func(in *Input) { in.PrivateKeyPEM = otherKey }, "pair"},
		"not covered":      {func(in *Input) { in.CertificatePEM, in.PrivateKeyPEM = other, otherKey }, "does not cover blog.example.com"},
		"bad host":         {func(in *Input) { in.Sites[0].Hosts = []string{"a..b"} }, "not a name"},
		"framing header": {func(in *Input) {
			in.Sites[0].Routes[0].Headers = map[string]string{"Content-Length": "1"}
		}, "set by the proxy"},
		"no upstreams": {func(in *Input) { in.Sites[0].Routes[1].Upstreams = nil }, "upstreams are required"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			s := newService(t)
			input := blogInput(cert, key)
			c.edit(&input)
			_, err := s.Create(context.Background(), input, Actor{})
			require.ErrorIs(t, err, ErrInvalid)
			require.Contains(t, err.Error(), c.want)
		})
	}

	// Without a key-encryption key a certificate cannot be stored.
	s := newService(t)
	s.KEK = func() string { return "" }
	_, err := s.Create(context.Background(), blogInput(cert, key), Actor{})
	require.ErrorIs(t, err, ErrNoKEK)

	// A plain proxy needs none, and a disabled one is not planned.
	plain := blogInput("", "")
	plain.TLSMode, plain.HTTPPort, plain.ListenPort = TLSNone, 0, 80
	disabled := false
	plain.Enabled = &disabled
	view, err := s.Create(context.Background(), plain, Actor{})
	require.NoError(t, err)
	require.False(t, view.Enabled)
	forwards, err := s.Forwards(s.DB)
	require.NoError(t, err)
	require.Empty(t, forwards)
}
