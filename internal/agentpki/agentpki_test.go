package agentpki_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"errors"
	"math/big"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v4/internal/agentpki"
	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/modulepki"
	"github.com/AnixOps/anix-control/v4/internal/nodesecrets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (f *fixture) proxyNode() agentcontrol.AgentNode {
	return agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: uint32(f.proxy.ID)}
}

func (f *fixture) forwardNode() agentcontrol.AgentNode {
	return agentcontrol.AgentNode{Kind: agentcontrol.NodeKindForward, ID: uint32(f.forward.ID)}
}

func (f *fixture) enroll(t *testing.T, bootstrap agentpki.Bootstrap) (agentpki.Issued, error) {
	t.Helper()
	_, csr := newCSR(t)
	return f.pki.Enroll(t.Context(), agentpki.EnrollRequest{
		Bootstrap: bootstrap, CSRDER: csr, AgentVersion: "1.2.0", InstanceID: "instance-1", RemoteAddr: "198.51.100.10",
	})
}

func (f *fixture) token(t *testing.T, node agentcontrol.AgentNode, ttl time.Duration) string {
	t.Helper()
	credential, row, err := f.pki.CreateEnrollmentToken(t.Context(), agentpki.TokenRequest{Node: node, TTL: ttl, CreatedBy: 1, Actor: "admin@example.com"})
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(credential, agentcontrol.EnrollmentCredentialPrefix))
	require.NotNil(t, row.CredentialHash)
	require.NotEqual(t, credential, *row.CredentialHash)
	return credential
}

func leafOf(t *testing.T, issued agentpki.Issued) *x509.Certificate {
	t.Helper()
	leaf, err := x509.ParseCertificate(issued.CertificateDER)
	require.NoError(t, err)
	return leaf
}

// requireNodeCertificate checks that a certificate names exactly node: one
// URI SAN, nothing from the CSR, and the agent lifetime.
func requireNodeCertificate(t *testing.T, f *fixture, issued agentpki.Issued, node agentcontrol.AgentNode) *x509.Certificate {
	t.Helper()
	leaf := leafOf(t, issued)
	want := "spiffe://anixops/" + testCluster + "/agent/" + node.String()
	require.Len(t, leaf.URIs, 1)
	assert.Equal(t, want, leaf.URIs[0].String())
	assert.Equal(t, want, issued.Identity.String())
	assert.Empty(t, leaf.DNSNames, "the CSR's DNS SAN must be ignored")
	assert.Equal(t, want, leaf.Subject.CommonName, "the CSR's subject must be ignored")
	assert.Empty(t, leaf.Subject.Organization)
	assert.Equal(t, f.clock.Now().Add(agentpki.DefaultCertificateLifetime), leaf.NotAfter)
	identity, verified, err := f.pki.VerifyPeer(t.Context(), [][]byte{issued.CertificateDER})
	require.NoError(t, err)
	assert.Equal(t, node, identity.Node)
	assert.Equal(t, leaf.SerialNumber, verified.SerialNumber)
	var record model.AgentCertificate
	require.NoError(t, f.db.First(&record, "serial = ?", issued.Serial).Error)
	assert.Equal(t, node.Kind, record.NodeKind)
	assert.Equal(t, uint(node.ID), record.NodeID)
	assert.Equal(t, issued.EnrollmentID, record.EnrollmentID)
	assert.Nil(t, record.RevokedAt)
	return leaf
}

func TestEnrollWithEachBootstrap(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, f *fixture) {
		cases := []struct {
			name      string
			bootstrap func() agentpki.Bootstrap
			node      agentcontrol.AgentNode
			method    string
		}{
			{name: "proxy node API key", node: f.proxyNode(), method: model.AgentEnrollmentMethodNodeAPIKey, bootstrap: func() agentpki.Bootstrap {
				return agentpki.Bootstrap{Method: model.AgentEnrollmentMethodNodeAPIKey, Node: f.proxyNode(), Secret: f.proxyKey}
			}},
			{name: "forward node token", node: f.forwardNode(), method: model.AgentEnrollmentMethodForwardToken, bootstrap: func() agentpki.Bootstrap {
				return agentpki.Bootstrap{Method: model.AgentEnrollmentMethodForwardToken, Node: f.forwardNode(), Secret: f.forwardToken}
			}},
			{name: "enrollment credential for a proxy node", node: f.proxyNode(), method: model.AgentEnrollmentMethodCredential, bootstrap: func() agentpki.Bootstrap {
				return agentpki.Bootstrap{Method: model.AgentEnrollmentMethodCredential, Secret: f.token(t, f.proxyNode(), time.Hour)}
			}},
			{name: "enrollment credential for a forward node", node: f.forwardNode(), method: model.AgentEnrollmentMethodCredential, bootstrap: func() agentpki.Bootstrap {
				return agentpki.Bootstrap{Method: model.AgentEnrollmentMethodCredential, Node: f.forwardNode(), Secret: f.token(t, f.forwardNode(), time.Hour)}
			}},
		}
		for _, test := range cases {
			t.Run(test.name, func(t *testing.T) {
				issued, err := f.enroll(t, test.bootstrap())
				require.NoError(t, err)
				requireNodeCertificate(t, f, issued, test.node)
				var enrollment model.AgentEnrollment
				require.NoError(t, f.db.First(&enrollment, "id = ?", issued.EnrollmentID).Error)
				assert.Equal(t, test.method, enrollment.Method)
				assert.Equal(t, test.node.Kind, enrollment.NodeKind)
				assert.NotNil(t, enrollment.UsedAt)
				assert.Equal(t, "1.2.0", enrollment.AgentVersion)
				assert.Equal(t, "instance-1", enrollment.InstanceID)
			})
		}
	})
}

func TestEnrollBindsKindAndID(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, f *fixture) {
		other := model.Node{Name: "other", APIKeyHash: sha256Hex("other-key"), Status: model.NodeStatusOnline}
		require.NoError(t, f.db.Create(&other).Error)
		otherNode := agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: uint32(other.ID)}
		credential := f.token(t, f.proxyNode(), time.Hour)
		rejected := map[string]agentpki.Bootstrap{
			"proxy key claiming another node":       {Method: model.AgentEnrollmentMethodNodeAPIKey, Node: otherNode, Secret: f.proxyKey},
			"proxy key claiming the forward node":   {Method: model.AgentEnrollmentMethodNodeAPIKey, Node: f.forwardNode(), Secret: f.proxyKey},
			"forward token claiming the proxy node": {Method: model.AgentEnrollmentMethodForwardToken, Node: f.proxyNode(), Secret: f.forwardToken},
			"proxy key as a forward token":          {Method: model.AgentEnrollmentMethodForwardToken, Node: f.forwardNode(), Secret: f.proxyKey},
			"wrong proxy key":                       {Method: model.AgentEnrollmentMethodNodeAPIKey, Node: f.proxyNode(), Secret: "wrong"},
			"empty secret":                          {Method: model.AgentEnrollmentMethodNodeAPIKey, Node: f.proxyNode()},
			"credential claiming another node":      {Method: model.AgentEnrollmentMethodCredential, Node: otherNode, Secret: credential},
			"credential claiming the other kind":    {Method: model.AgentEnrollmentMethodCredential, Node: f.forwardNode(), Secret: credential},
			"unknown credential":                    {Method: model.AgentEnrollmentMethodCredential, Secret: agentcontrol.EnrollmentCredentialPrefix + "unknown"},
			"credential without its prefix":         {Method: model.AgentEnrollmentMethodCredential, Secret: strings.TrimPrefix(credential, agentcontrol.EnrollmentCredentialPrefix)},
			"unknown method":                        {Method: "registration_key", Node: f.proxyNode(), Secret: f.proxyKey},
		}
		for name, bootstrap := range rejected {
			_, err := f.enroll(t, bootstrap)
			assert.ErrorIs(t, err, agentpki.ErrEnrollmentRejected, name)
		}
		// The rejected claims did not consume the credential.
		issued, err := f.enroll(t, agentpki.Bootstrap{Method: model.AgentEnrollmentMethodCredential, Node: f.proxyNode(), Secret: credential})
		require.NoError(t, err)
		requireNodeCertificate(t, f, issued, f.proxyNode())

		// A disabled node cannot enroll, with any bootstrap.
		require.NoError(t, f.db.Model(&model.Node{}).Where("id = ?", f.proxy.ID).Update("status", model.NodeStatusDisabled).Error)
		_, err = f.enroll(t, agentpki.Bootstrap{Method: model.AgentEnrollmentMethodNodeAPIKey, Node: f.proxyNode(), Secret: f.proxyKey})
		assert.ErrorIs(t, err, agentpki.ErrEnrollmentRejected)
		require.NoError(t, f.db.Model(&model.ForwardNode{}).Where("id = ?", f.forward.ID).Update("enabled", false).Error)
		_, err = f.enroll(t, agentpki.Bootstrap{Method: model.AgentEnrollmentMethodForwardToken, Node: f.forwardNode(), Secret: f.forwardToken})
		assert.ErrorIs(t, err, agentpki.ErrEnrollmentRejected)
		_, _, err = f.pki.CreateEnrollmentToken(t.Context(), agentpki.TokenRequest{Node: f.forwardNode()})
		assert.ErrorIs(t, err, agentpki.ErrInvalidNode)
		_, _, err = f.pki.CreateEnrollmentToken(t.Context(), agentpki.TokenRequest{Node: agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: 4242}})
		assert.ErrorIs(t, err, agentpki.ErrInvalidNode)
	})
}

func TestEnrollRejectsBadCSR(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, f *fixture) {
		credential := f.token(t, f.proxyNode(), time.Hour)
		_, err := f.pki.Enroll(t.Context(), agentpki.EnrollRequest{
			Bootstrap: agentpki.Bootstrap{Method: model.AgentEnrollmentMethodCredential, Secret: credential}, CSRDER: []byte("not a csr"),
		})
		require.ErrorIs(t, err, modulepki.ErrInvalidRequest)
		// A failed issuance leaves the credential unused.
		_, err = f.enroll(t, agentpki.Bootstrap{Method: model.AgentEnrollmentMethodCredential, Secret: credential})
		require.NoError(t, err)
	})
}

func TestEnrollmentCredentialIsOneTime(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, f *fixture) {
		credential := f.token(t, f.proxyNode(), time.Hour)
		bootstrap := agentpki.Bootstrap{Method: model.AgentEnrollmentMethodCredential, Secret: credential}
		_, err := f.enroll(t, bootstrap)
		require.NoError(t, err)
		_, err = f.enroll(t, bootstrap)
		require.ErrorIs(t, err, agentpki.ErrEnrollmentRejected)

		// Concurrent use: exactly one enrollment wins.
		credential = f.token(t, f.forwardNode(), time.Hour)
		bootstrap = agentpki.Bootstrap{Method: model.AgentEnrollmentMethodCredential, Secret: credential}
		const racers = 8
		csrs := make([][]byte, racers)
		for i := range csrs {
			_, csrs[i] = newCSR(t)
		}
		var (
			wg        sync.WaitGroup
			mu        sync.Mutex
			succeeded int
			failures  []error
		)
		start := make(chan struct{})
		for i := 0; i < racers; i++ {
			wg.Add(1)
			go func(csr []byte) {
				defer wg.Done()
				<-start
				_, err := f.pki.Enroll(t.Context(), agentpki.EnrollRequest{Bootstrap: bootstrap, CSRDER: csr})
				mu.Lock()
				defer mu.Unlock()
				if err == nil {
					succeeded++
				} else {
					failures = append(failures, err)
				}
			}(csrs[i])
		}
		close(start)
		wg.Wait()
		assert.Equal(t, 1, succeeded)
		for _, err := range failures {
			assert.ErrorIs(t, err, agentpki.ErrEnrollmentRejected)
		}
		var certificates int64
		require.NoError(t, f.db.Model(&model.AgentCertificate{}).Where("node_kind = ?", agentcontrol.NodeKindForward).Count(&certificates).Error)
		assert.Equal(t, int64(1), certificates)
	})
}

func TestExpiredEnrollmentCredentialIsRefused(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, f *fixture) {
		credential := f.token(t, f.proxyNode(), time.Hour)
		f.clock.Advance(time.Hour)
		_, err := f.enroll(t, agentpki.Bootstrap{Method: model.AgentEnrollmentMethodCredential, Secret: credential})
		require.ErrorIs(t, err, agentpki.ErrEnrollmentRejected)

		for _, ttl := range []time.Duration{-time.Second, agentpki.MaxEnrollmentCredentialTTL + time.Second} {
			_, _, err := f.pki.CreateEnrollmentToken(t.Context(), agentpki.TokenRequest{Node: f.proxyNode(), TTL: ttl})
			assert.Error(t, err, ttl)
		}
		_, row, err := f.pki.CreateEnrollmentToken(t.Context(), agentpki.TokenRequest{Node: f.proxyNode()})
		require.NoError(t, err)
		assert.Equal(t, f.clock.Now().Add(agentpki.DefaultEnrollmentCredentialTTL), row.ExpiresAt.UTC())
		_, row, err = f.pki.CreateEnrollmentToken(t.Context(), agentpki.TokenRequest{Node: f.proxyNode(), TTL: agentpki.MaxEnrollmentCredentialTTL})
		require.NoError(t, err)
		assert.Equal(t, f.clock.Now().Add(7*24*time.Hour), row.ExpiresAt.UTC())
	})
}

func TestRenewAtTwoThirdsOfTheLifetime(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, f *fixture) {
		issued, err := f.enroll(t, agentpki.Bootstrap{Method: model.AgentEnrollmentMethodNodeAPIKey, Node: f.proxyNode(), Secret: f.proxyKey})
		require.NoError(t, err)
		lifetime := agentpki.DefaultCertificateLifetime
		assert.Equal(t, 7*24*time.Hour, lifetime)
		assert.Equal(t, f.clock.Now().Add(lifetime*2/3), issued.RenewAfter)
		// The recorded issue and expiry times give the same instant: the
		// transport inventory derives renew_after from the record.
		requireRecordedRenewAfter(t, f, issued)

		f.clock.Advance(lifetime * 2 / 3)
		leaf := leafOf(t, issued)
		_, csr := newCSR(t)
		renewed, err := f.pki.Renew(t.Context(), leaf, csr)
		require.NoError(t, err)
		requireNodeCertificate(t, f, renewed, f.proxyNode())
		assert.Equal(t, issued.EnrollmentID, renewed.EnrollmentID)
		assert.NotEqual(t, issued.Serial, renewed.Serial)
		assert.Equal(t, f.clock.Now().Add(lifetime*2/3), renewed.RenewAfter)
		requireRecordedRenewAfter(t, f, renewed)

		// The old certificate is still valid until it expires, then refused.
		_, _, err = f.pki.VerifyPeer(t.Context(), [][]byte{issued.CertificateDER})
		require.NoError(t, err)
		f.clock.Advance(lifetime/3 + time.Minute)
		_, _, err = f.pki.VerifyPeer(t.Context(), [][]byte{issued.CertificateDER})
		require.ErrorIs(t, err, agentpki.ErrInvalidCertificate)
		require.ErrorIs(t, err, agentpki.ErrCertificateExpired, "an expired certificate of this CA is told apart")

		// A revoked certificate cannot renew.
		require.NoError(t, agentpki.RevokeNode(t.Context(), f.db, f.proxyNode(), agentpki.RevokeReasonCredentialsRevoked))
		_, csr = newCSR(t)
		_, err = f.pki.Renew(t.Context(), leafOf(t, renewed), csr)
		require.ErrorIs(t, err, agentpki.ErrCertificateRevoked)
	})
}

func TestRevokedSerialsAreRefusedWithinTheCacheWindow(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, f *fixture) {
		issued, err := f.enroll(t, agentpki.Bootstrap{Method: model.AgentEnrollmentMethodForwardToken, Node: f.forwardNode(), Secret: f.forwardToken})
		require.NoError(t, err)
		_, _, err = f.pki.VerifyPeer(t.Context(), [][]byte{issued.CertificateDER})
		require.NoError(t, err)

		// Revoked by another process: this one still trusts its cached
		// answer, for at most the cache lifetime.
		require.NoError(t, f.db.Model(&model.AgentCertificate{}).Where("serial = ?", issued.Serial).Update("revoked_at", f.clock.Now()).Error)
		f.clock.Advance(agentpki.RevocationCacheTTL - time.Second)
		_, _, err = f.pki.VerifyPeer(t.Context(), [][]byte{issued.CertificateDER})
		require.NoError(t, err, "within the cache window")
		f.clock.Advance(time.Second)
		_, _, err = f.pki.VerifyPeer(t.Context(), [][]byte{issued.CertificateDER})
		require.ErrorIs(t, err, agentpki.ErrCertificateRevoked)

		// Revoked in this process: refused at once.
		second, err := f.enroll(t, agentpki.Bootstrap{Method: model.AgentEnrollmentMethodForwardToken, Node: f.forwardNode(), Secret: f.forwardToken})
		require.NoError(t, err)
		_, _, err = f.pki.VerifyPeer(t.Context(), [][]byte{second.CertificateDER})
		require.NoError(t, err)
		credential := f.token(t, f.forwardNode(), time.Hour)
		require.NoError(t, agentpki.RevokeNode(t.Context(), f.db, f.forwardNode(), agentpki.RevokeReasonCredentialsReplaced))
		_, _, err = f.pki.VerifyPeer(t.Context(), [][]byte{second.CertificateDER})
		require.ErrorIs(t, err, agentpki.ErrCertificateRevoked)
		// Revoking the node also voids its unused enrollment credentials,
		// and leaves the proxy node of the same id alone.
		_, err = f.enroll(t, agentpki.Bootstrap{Method: model.AgentEnrollmentMethodCredential, Secret: credential})
		require.ErrorIs(t, err, agentpki.ErrEnrollmentRejected)
		var record model.AgentCertificate
		require.NoError(t, f.db.First(&record, "serial = ?", second.Serial).Error)
		assert.Equal(t, agentpki.RevokeReasonCredentialsReplaced, record.RevokeReason)

		proxy, err := f.enroll(t, agentpki.Bootstrap{Method: model.AgentEnrollmentMethodNodeAPIKey, Node: f.proxyNode(), Secret: f.proxyKey})
		require.NoError(t, err)
		_, _, err = f.pki.VerifyPeer(t.Context(), [][]byte{proxy.CertificateDER})
		require.NoError(t, err)
	})
}

func TestVerifyPeerRefusesForeignCertificates(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, f *fixture) {
		// The kernel's own certificate chains to the same CA but is not an
		// agent identity.
		source, err := f.authority.KernelTLS(t.Context(), time.Minute)
		require.NoError(t, err)
		kernel, err := source.Certificate()
		require.NoError(t, err)
		_, _, err = f.pki.VerifyPeer(t.Context(), kernel.Certificate)
		require.ErrorIs(t, err, agentpki.ErrInvalidCertificate)

		// A certificate the CA signed for an agent identity but never
		// recorded counts as revoked.
		_, csr := newCSR(t)
		identity, err := agentcontrol.NewAgentIdentity(testCluster, f.proxyNode())
		require.NoError(t, err)
		leaf, err := f.authority.SignLeaf(t.Context(), f.db, identity.URL(), time.Hour, csr)
		require.NoError(t, err)
		_, _, err = f.pki.VerifyPeer(t.Context(), [][]byte{leaf.CertificateDER})
		require.ErrorIs(t, err, agentpki.ErrCertificateRevoked)

		// Another cluster's identity is refused.
		other, err := agentcontrol.NewAgentIdentity("other", f.proxyNode())
		require.NoError(t, err)
		leaf, err = f.authority.SignLeaf(t.Context(), f.db, other.URL(), time.Hour, csr)
		require.NoError(t, err)
		_, _, err = f.pki.VerifyPeer(t.Context(), [][]byte{leaf.CertificateDER})
		require.ErrorIs(t, err, agentpki.ErrInvalidCertificate)
		require.ErrorIs(t, err, agentpki.ErrCertificateWrongCluster)

		// Neither the kernel's certificate nor an expired one of another
		// CA is reported as an expired agent certificate.
		f.clock.Advance(2 * time.Hour)
		_, _, err = f.pki.VerifyPeer(t.Context(), kernel.Certificate)
		require.ErrorIs(t, err, agentpki.ErrInvalidCertificate)
		require.NotErrorIs(t, err, agentpki.ErrCertificateExpired)
		otherKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)
		foreignTemplate := &x509.Certificate{
			SerialNumber: big.NewInt(9), NotBefore: f.clock.Now().Add(-3 * time.Hour), NotAfter: f.clock.Now().Add(-time.Hour),
			ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, URIs: []*url.URL{identity.URL()},
		}
		foreign, err := x509.CreateCertificate(rand.Reader, foreignTemplate, foreignTemplate, &otherKey.PublicKey, otherKey)
		require.NoError(t, err)
		_, _, err = f.pki.VerifyPeer(t.Context(), [][]byte{foreign})
		require.ErrorIs(t, err, agentpki.ErrInvalidCertificate)
		require.NotErrorIs(t, err, agentpki.ErrCertificateExpired)

		_, _, err = f.pki.VerifyPeer(t.Context(), [][]byte{[]byte("garbage")})
		require.ErrorIs(t, err, agentpki.ErrInvalidCertificate)
		_, _, err = f.pki.VerifyPeer(t.Context(), nil)
		require.ErrorIs(t, err, agentpki.ErrInvalidCertificate)
	})
}

func TestAgentPKIAuditsTokensAndEnrollmentsWithoutSecrets(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, f *fixture) {
		credential := f.token(t, f.proxyNode(), time.Hour)
		issued, err := f.enroll(t, agentpki.Bootstrap{Method: model.AgentEnrollmentMethodCredential, Secret: credential})
		require.NoError(t, err)
		_, err = f.enroll(t, agentpki.Bootstrap{Method: model.AgentEnrollmentMethodForwardToken, Node: f.forwardNode(), Secret: f.forwardToken})
		require.NoError(t, err)

		var entries []model.OperationLog
		require.NoError(t, f.db.Order("id").Find(&entries).Error)
		require.Len(t, entries, 3)
		assert.Equal(t, agentpki.AuditActionTokenIssue, entries[0].Action)
		assert.Equal(t, "admin@example.com", entries[0].Username)
		require.NotNil(t, entries[0].UserID)
		assert.Equal(t, uint(1), *entries[0].UserID)
		assert.Equal(t, agentpki.AuditActionEnroll, entries[1].Action)
		assert.Equal(t, "agent:"+f.proxyNode().String(), entries[1].Username)
		assert.Contains(t, entries[1].Content, issued.Serial)
		assert.Contains(t, entries[1].Content, model.AgentEnrollmentMethodCredential)
		assert.Equal(t, "forward_node", entries[2].TargetType)
		for _, entry := range entries {
			assert.Equal(t, "agent_pki", entry.Module)
			for _, secret := range []string{credential, f.proxyKey, f.forwardToken} {
				assert.NotContains(t, entry.Content, secret)
			}
		}
	})
}

func TestPruneDropsExpiredRecords(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, f *fixture) {
		_, err := f.enroll(t, agentpki.Bootstrap{Method: model.AgentEnrollmentMethodNodeAPIKey, Node: f.proxyNode(), Secret: f.proxyKey})
		require.NoError(t, err)
		f.token(t, f.proxyNode(), time.Hour)
		require.NoError(t, f.pki.Prune(t.Context(), f.clock.Now()))
		var certificates, enrollments int64
		require.NoError(t, f.db.Model(&model.AgentCertificate{}).Count(&certificates).Error)
		require.NoError(t, f.db.Model(&model.AgentEnrollment{}).Count(&enrollments).Error)
		assert.Equal(t, int64(1), certificates)
		assert.Equal(t, int64(2), enrollments)

		cutoff := f.clock.Now().Add(agentpki.DefaultCertificateLifetime + time.Hour)
		require.NoError(t, f.pki.Prune(t.Context(), cutoff))
		require.NoError(t, f.db.Model(&model.AgentCertificate{}).Count(&certificates).Error)
		require.NoError(t, f.db.Model(&model.AgentEnrollment{}).Count(&enrollments).Error)
		assert.Equal(t, int64(0), certificates)
		assert.Equal(t, int64(1), enrollments, "the used enrollment stays; the unused expired credential goes")
	})
}

func TestRevokeNodeWithoutAgentTables(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, f *fixture) {
		require.NoError(t, f.db.Migrator().DropTable(&model.AgentCertificate{}))
		require.NoError(t, agentpki.RevokeNode(t.Context(), f.db, f.proxyNode(), agentpki.RevokeReasonNodeDeleted))
		require.NoError(t, agentpki.RevokeNode(t.Context(), nil, f.proxyNode(), agentpki.RevokeReasonNodeDeleted))
	})
}

func TestNewValidatesOptions(t *testing.T) {
	_, err := agentpki.New(agentpki.Options{})
	require.Error(t, err)
	_, err = agentpki.FromConfig(nil, nil)
	require.True(t, errors.Is(err, agentpki.ErrDisabled))
}

func TestFromConfigUsesTheCAAlone(t *testing.T) {
	db := openSQLiteForConfig(t)
	cfg := &config.Config{ModuleRuntime: config.ModuleRuntimeConfig{
		CAKEK: "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=", Cluster: "edge",
	}}
	pki, err := agentpki.FromConfig(cfg, db)
	require.NoError(t, err, "the module runtime need not be enabled")
	assert.Equal(t, "edge", pki.Cluster())
	assert.Equal(t, agentpki.DefaultCertificateLifetime, pki.Lifetime())

	external := &config.Config{ModuleRuntime: config.ModuleRuntimeConfig{Enabled: true, PKI: config.ModulePKIExternal}}
	_, err = agentpki.FromConfig(external, db)
	require.ErrorIs(t, err, agentpki.ErrExternalPKI)
	require.ErrorIs(t, err, agentpki.ErrDisabled)
	_, err = agentpki.FromConfig(&config.Config{}, db)
	require.ErrorIs(t, err, agentpki.ErrDisabled)
	require.NotErrorIs(t, err, agentpki.ErrExternalPKI)
}

// TestEnrollReadsNodeCredentialsByPhase: the enrollment bootstrap reads node
// keys and forward tokens through the node credential split. In dual_read
// a key whose legacy column holds a tombstone but whose new row is valid
// enrolls; a valid legacy column without a new row falls back and counts
// the metric. In dual_write only the legacy columns count.
func TestEnrollReadsNodeCredentialsByPhase(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()
		// The fixture has the node tables only.
		tables := []string{nodesecrets.TableNode, nodesecrets.TableForwardNode}
		require.NoError(t, nodesecrets.EnsureSchema(f.db))
		_, err := nodesecrets.Backfill(ctx, f.db, nodesecrets.BackfillOptions{Tables: tables})
		require.NoError(t, err)
		_, err = nodesecrets.Verify(ctx, f.db, nodesecrets.VerifyOptions{Tables: tables})
		require.NoError(t, err)
		proxyBootstrap := agentpki.Bootstrap{Method: model.AgentEnrollmentMethodNodeAPIKey, Node: f.proxyNode(), Secret: f.proxyKey}
		forwardBootstrap := agentpki.Bootstrap{Method: model.AgentEnrollmentMethodForwardToken, Node: f.forwardNode(), Secret: f.forwardToken}
		setPhase := func(phase string) {
			_, err := nodesecrets.SetPhase(ctx, f.db, nodesecrets.PhaseOptions{Tables: tables, Phase: phase, Actor: "test"})
			require.NoError(t, err)
		}

		// Only the new tables hold the secrets, as after P3.
		require.NoError(t, f.db.Model(&model.Node{}).Where("id = ?", f.proxy.ID).
			UpdateColumns(map[string]any{"api_key": nodesecrets.Tombstone(uint64(f.proxy.ID)), "api_key_hash": ""}).Error)
		require.NoError(t, f.db.Model(&model.ForwardNode{}).Where("id = ?", f.forward.ID).UpdateColumn("api_token", "").Error)
		keyFallbacks := nodesecrets.FallbackCount(nodesecrets.TableNode, nodesecrets.KindNodeAPIKey, "")
		tokenFallbacks := nodesecrets.FallbackCount(nodesecrets.TableForwardNode, nodesecrets.KindForwardNodeToken, "")

		setPhase(nodesecrets.PhaseDualRead)
		issued, err := f.enroll(t, proxyBootstrap)
		require.NoError(t, err)
		requireNodeCertificate(t, f, issued, f.proxyNode())
		issued, err = f.enroll(t, forwardBootstrap)
		require.NoError(t, err)
		requireNodeCertificate(t, f, issued, f.forwardNode())
		require.Equal(t, keyFallbacks, nodesecrets.FallbackCount(nodesecrets.TableNode, nodesecrets.KindNodeAPIKey, ""))
		require.Equal(t, tokenFallbacks, nodesecrets.FallbackCount(nodesecrets.TableForwardNode, nodesecrets.KindForwardNodeToken, ""))
		// The tombstone itself never enrolls.
		_, err = f.enroll(t, agentpki.Bootstrap{Method: model.AgentEnrollmentMethodNodeAPIKey, Node: f.proxyNode(), Secret: nodesecrets.Tombstone(uint64(f.proxy.ID))})
		require.ErrorIs(t, err, agentpki.ErrEnrollmentRejected)

		// In dual_write the legacy columns are the only ones read.
		setPhase(nodesecrets.PhaseDualWrite)
		_, err = f.enroll(t, proxyBootstrap)
		require.ErrorIs(t, err, agentpki.ErrEnrollmentRejected)
		_, err = f.enroll(t, forwardBootstrap)
		require.ErrorIs(t, err, agentpki.ErrEnrollmentRejected)

		// The reverse: valid legacy columns and no new row fall back, and
		// count it.
		require.NoError(t, f.db.Model(&model.Node{}).Where("id = ?", f.proxy.ID).
			UpdateColumns(map[string]any{"api_key": f.proxyKey, "api_key_hash": sha256Hex(f.proxyKey)}).Error)
		require.NoError(t, f.db.Model(&model.ForwardNode{}).Where("id = ?", f.forward.ID).UpdateColumn("api_token", f.forwardToken).Error)
		require.NoError(t, f.db.Where("subject_kind IN ?", []string{nodesecrets.SubjectProxy, nodesecrets.SubjectForward}).Delete(&model.NodeCredential{}).Error)
		setPhase(nodesecrets.PhaseDualRead)
		issued, err = f.enroll(t, proxyBootstrap)
		require.NoError(t, err)
		requireNodeCertificate(t, f, issued, f.proxyNode())
		issued, err = f.enroll(t, forwardBootstrap)
		require.NoError(t, err)
		requireNodeCertificate(t, f, issued, f.forwardNode())
		require.Equal(t, keyFallbacks+1, nodesecrets.FallbackCount(nodesecrets.TableNode, nodesecrets.KindNodeAPIKey, nodesecrets.FallbackMissing))
		require.Equal(t, tokenFallbacks+1, nodesecrets.FallbackCount(nodesecrets.TableForwardNode, nodesecrets.KindForwardNodeToken, nodesecrets.FallbackMissing))
	})
}

// requireRecordedRenewAfter: the certificate's record (issued_at, not_after)
// yields the renew_after the agent was told.
func requireRecordedRenewAfter(t *testing.T, f *fixture, issued agentpki.Issued) {
	t.Helper()
	var record model.AgentCertificate
	require.NoError(t, f.db.Where("serial = ?", issued.Serial).First(&record).Error)
	assert.True(t, modulepki.RenewAfter(record.CreatedAt, record.NotAfter).Equal(issued.RenewAfter),
		"record %s %s, told %s", record.CreatedAt, record.NotAfter, issued.RenewAfter)
}
