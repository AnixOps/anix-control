package agentpki_test

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"net"
	"net/url"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v4/internal/agentpki"
	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// negotiate records that node's Agent negotiated forward.v1 (or did not),
// as kernelforward.RecordHello does.
func (f *fixture) negotiate(t *testing.T, node agentcontrol.AgentNode, negotiated bool) {
	t.Helper()
	row := model.KernelForwardNode{NodeRef: node.String(), NodeKind: node.Kind, NodeID: uint64(node.ID), Negotiated: negotiated,
		CreatedAt: f.clock.Now(), UpdatedAt: f.clock.Now()}
	require.NoError(t, f.db.Save(&row).Error)
}

// agentCertificate enrolls node's Agent and returns its certificate and key.
func (f *fixture) agentCertificate(t *testing.T, node agentcontrol.AgentNode) (*x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()
	key, csr := newCSR(t)
	bootstrap := agentpki.Bootstrap{Method: model.AgentEnrollmentMethodForwardToken, Node: node, Secret: f.forwardToken}
	if node.Kind == agentcontrol.NodeKindProxy {
		bootstrap = agentpki.Bootstrap{Method: model.AgentEnrollmentMethodNodeAPIKey, Node: node, Secret: f.proxyKey}
	}
	issued, err := f.pki.Enroll(t.Context(), agentpki.EnrollRequest{Bootstrap: bootstrap, CSRDER: csr})
	require.NoError(t, err)
	return leafOf(t, issued), key
}

// linkCSR is a request for a fresh key naming the given SANs.
func linkCSR(t *testing.T, template *x509.CertificateRequest) (crypto.Signer, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	return key, csrFor(t, template, key)
}

func csrFor(t *testing.T, template *x509.CertificateRequest, key crypto.Signer) []byte {
	t.Helper()
	if template == nil {
		template = &x509.CertificateRequest{}
	}
	der, err := x509.CreateCertificateRequest(rand.Reader, template, key)
	require.NoError(t, err)
	return der
}

func spiffeURL(t *testing.T, node agentcontrol.AgentNode) *url.URL {
	t.Helper()
	identity, err := agentcontrol.NewAgentIdentity(testCluster, node)
	require.NoError(t, err)
	return identity.URL()
}

func pool(certificates ...[]byte) *x509.CertPool {
	out := x509.NewCertPool()
	for _, der := range certificates {
		certificate, err := x509.ParseCertificate(der)
		if err == nil {
			out.AddCert(certificate)
		}
	}
	return out
}

// TestLinkCertificateProfile: the node's identity name as CN and only DNS
// name, its SPIFFE ID as only URI, serverAuth and clientAuth, the Agent
// lifetime, renewal at two thirds, signed by the link CA and nothing else.
func TestLinkCertificateProfile(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, f *fixture) {
		for _, node := range []agentcontrol.AgentNode{f.forwardNode(), f.proxyNode()} {
			t.Run(node.Kind, func(t *testing.T) {
				peer, _ := f.agentCertificate(t, node)
				f.negotiate(t, node, true)
				_, csr := linkCSR(t, &x509.CertificateRequest{
					Subject: pkix.Name{CommonName: "ignored"}, DNSNames: []string{node.String()}, URIs: []*url.URL{spiffeURL(t, node)},
				})
				issued, err := f.pki.IssueLinkCertificate(t.Context(), peer, csr, "198.51.100.20")
				require.NoError(t, err)
				leaf, err := x509.ParseCertificate(issued.CertificateDER)
				require.NoError(t, err)
				assert.Equal(t, node.String(), leaf.Subject.CommonName)
				assert.Equal(t, []string{node.String()}, leaf.DNSNames)
				require.Len(t, leaf.URIs, 1)
				assert.Equal(t, spiffeURL(t, node).String(), leaf.URIs[0].String())
				assert.Empty(t, leaf.IPAddresses)
				assert.ElementsMatch(t, []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}, leaf.ExtKeyUsage)
				assert.False(t, leaf.IsCA)
				now := f.clock.Now()
				assert.Equal(t, now.Add(agentpki.DefaultLinkCertificateLifetime), leaf.NotAfter)
				assert.Equal(t, now.Add(agentpki.DefaultLinkCertificateLifetime*2/3), issued.RenewAfter)
				assert.Equal(t, node.String(), issued.DNSName)
				assert.Equal(t, node, issued.Identity.Node)

				// The link certificate chains to the link bundle, for both
				// usages and the node's name; never to the Agent CA.
				for _, usage := range []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth} {
					_, err = leaf.Verify(x509.VerifyOptions{Roots: pool(issued.TrustBundleDER...), DNSName: node.String(),
						CurrentTime: now, KeyUsages: []x509.ExtKeyUsage{usage}})
					require.NoError(t, err)
				}
				agentRoots, err := f.pki.Roots(t.Context())
				require.NoError(t, err)
				_, err = leaf.Verify(x509.VerifyOptions{Roots: agentRoots, CurrentTime: now, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}})
				require.Error(t, err, "the link CA is a separate root")
				_, _, err = f.pki.VerifyPeer(t.Context(), [][]byte{issued.CertificateDER})
				require.ErrorIs(t, err, agentpki.ErrInvalidCertificate, "a link certificate never authenticates an Agent")
				_, err = peer.Verify(x509.VerifyOptions{Roots: pool(issued.TrustBundleDER...), CurrentTime: now,
					KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}})
				require.Error(t, err, "an Agent certificate never authenticates a link")

				var record model.ForwardLinkCertificate
				require.NoError(t, f.db.First(&record, "serial = ?", issued.Serial).Error)
				assert.Equal(t, node.Kind, record.NodeKind)
				assert.Equal(t, uint(node.ID), record.NodeID)
				assert.Equal(t, node.String(), record.DNSName)
				assert.Equal(t, spiffeURL(t, node).String(), record.SPIFFEID)
				assert.Equal(t, issued.IssuerKeyID, record.IssuerKeyID)
				assert.NotEmpty(t, record.AgentSerial)
				assert.Nil(t, record.RevokedAt)
				var audit model.OperationLog
				require.NoError(t, f.db.Where("action = ? AND target_id = ?", agentpki.AuditActionLinkIssue, node.ID).Last(&audit).Error)
				assert.Contains(t, audit.Content, issued.Serial)
			})
		}
	})
}

// TestLinkCertificateRefusesBadRequests: only the node's DNS name and
// SPIFFE ID may be requested, never with the Agent's key.
func TestLinkCertificateRefusesBadRequests(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, f *fixture) {
		node := f.forwardNode()
		peer, agentKey := f.agentCertificate(t, node)
		f.negotiate(t, node, true)
		other := agentcontrol.AgentNode{Kind: agentcontrol.NodeKindForward, ID: node.ID + 1}
		for name, template := range map[string]*x509.CertificateRequest{
			"another DNS name":  {DNSNames: []string{"forward-999"}},
			"an extra DNS name": {DNSNames: []string{node.String(), "control.example"}},
			"another URI":       {URIs: []*url.URL{spiffeURL(t, other)}},
			"an IP address":     {IPAddresses: []net.IP{net.ParseIP("192.0.2.1")}},
			"an e-mail address": {EmailAddresses: []string{"ops@example.com"}},
		} {
			_, csr := linkCSR(t, template)
			_, err := f.pki.IssueLinkCertificate(t.Context(), peer, csr, "")
			require.ErrorIs(t, err, agentpki.ErrInvalidLinkRequest, name)
		}
		_, err := f.pki.IssueLinkCertificate(t.Context(), peer, csrFor(t, nil, agentKey), "")
		require.ErrorIs(t, err, agentpki.ErrInvalidLinkRequest, "the Agent's own key")
		_, err = f.pki.IssueLinkCertificate(t.Context(), peer, []byte("not a csr"), "")
		require.ErrorIs(t, err, agentpki.ErrInvalidLinkRequest)
		weak, err := rsa.GenerateKey(rand.Reader, 1024)
		require.NoError(t, err)
		_, err = f.pki.IssueLinkCertificate(t.Context(), peer, csrFor(t, nil, weak), "")
		require.ErrorIs(t, err, agentpki.ErrInvalidLinkRequest, "RSA under 2048 bits")
		p224, err := ecdsa.GenerateKey(elliptic.P224(), rand.Reader)
		require.NoError(t, err)
		_, err = f.pki.IssueLinkCertificate(t.Context(), peer, csrFor(t, nil, p224), "")
		require.ErrorIs(t, err, agentpki.ErrInvalidLinkRequest, "P-224")

		_, csr := linkCSR(t, nil)
		_, err = f.pki.IssueLinkCertificate(t.Context(), peer, csr, "")
		require.NoError(t, err, "a request without SANs gets the node's names")
		var count int64
		require.NoError(t, f.db.Model(&model.ForwardLinkCertificate{}).Count(&count).Error)
		assert.Equal(t, int64(1), count, "refused requests record nothing")
	})
}

// TestLinkCertificateNeedsForwardV1: a node whose Agent never negotiated
// forward.v1, or whose last Hello dropped it, gets no link certificate.
func TestLinkCertificateNeedsForwardV1(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, f *fixture) {
		node := f.forwardNode()
		peer, _ := f.agentCertificate(t, node)
		_, csr := linkCSR(t, nil)
		_, err := f.pki.IssueLinkCertificate(t.Context(), peer, csr, "")
		require.ErrorIs(t, err, agentpki.ErrLinkNotNegotiated, "no inventory row")
		f.negotiate(t, node, false)
		_, err = f.pki.IssueLinkCertificate(t.Context(), peer, csr, "")
		require.ErrorIs(t, err, agentpki.ErrLinkNotNegotiated, "the last Hello did not list forward.v1")
		f.negotiate(t, f.proxyNode(), true)
		_, err = f.pki.IssueLinkCertificate(t.Context(), peer, csr, "")
		require.ErrorIs(t, err, agentpki.ErrLinkNotNegotiated, "another node's flag (same id, other kind) does not count")
		f.negotiate(t, node, true)
		_, err = f.pki.IssueLinkCertificate(t.Context(), peer, csr, "")
		require.NoError(t, err)

		require.NoError(t, f.db.Migrator().DropTable(&model.KernelForwardNode{}))
		_, err = f.pki.IssueLinkCertificate(t.Context(), peer, csr, "")
		require.ErrorIs(t, err, agentpki.ErrLinkNotNegotiated, "a database without the forwarding tables")
	})
}

// TestLinkCertificateRenewal: renewing is asking again with a new key; the
// old certificate stays recorded and valid until it expires.
func TestLinkCertificateRenewal(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, f *fixture) {
		node := f.forwardNode()
		peer, _ := f.agentCertificate(t, node)
		f.negotiate(t, node, true)
		_, csr := linkCSR(t, nil)
		first, err := f.pki.IssueLinkCertificate(t.Context(), peer, csr, "")
		require.NoError(t, err)
		f.clock.Advance(first.RenewAfter.Sub(f.clock.Now()))
		_, csr = linkCSR(t, nil)
		second, err := f.pki.IssueLinkCertificate(t.Context(), peer, csr, "")
		require.NoError(t, err)
		assert.NotEqual(t, first.Serial, second.Serial)
		assert.True(t, second.NotAfter.After(first.NotAfter))
		var rows []model.ForwardLinkCertificate
		require.NoError(t, f.db.Order("created_at").Find(&rows, "node_kind = ? AND node_id = ?", node.Kind, node.ID).Error)
		require.Len(t, rows, 2)
		for _, row := range rows {
			assert.Nil(t, row.RevokedAt)
		}
	})
}

// TestLinkCertificatesAreRevokedWithTheNode: RevokeNode (disable, credential
// replacement, deletion, RetireNode) revokes the node's link certificates
// and stops their renewal; other nodes keep theirs.
func TestLinkCertificatesAreRevokedWithTheNode(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, f *fixture) {
		forward, proxy := f.forwardNode(), f.proxyNode()
		forwardPeer, _ := f.agentCertificate(t, forward)
		proxyPeer, _ := f.agentCertificate(t, proxy)
		f.negotiate(t, forward, true)
		f.negotiate(t, proxy, true)
		_, csr := linkCSR(t, nil)
		forwardLink, err := f.pki.IssueLinkCertificate(t.Context(), forwardPeer, csr, "")
		require.NoError(t, err)
		_, csr = linkCSR(t, nil)
		proxyLink, err := f.pki.IssueLinkCertificate(t.Context(), proxyPeer, csr, "")
		require.NoError(t, err)

		require.NoError(t, agentpki.RevokeNode(t.Context(), f.db, forward, agentpki.RevokeReasonNodeDeleted))
		var record model.ForwardLinkCertificate
		require.NoError(t, f.db.First(&record, "serial = ?", forwardLink.Serial).Error)
		require.NotNil(t, record.RevokedAt)
		assert.Equal(t, agentpki.RevokeReasonNodeDeleted, record.RevokeReason)
		var proxyRecord model.ForwardLinkCertificate
		require.NoError(t, f.db.First(&proxyRecord, "serial = ?", proxyLink.Serial).Error)
		assert.Nil(t, proxyRecord.RevokedAt, "kind and id together name the node")

		_, csr = linkCSR(t, nil)
		_, err = f.pki.IssueLinkCertificate(t.Context(), forwardPeer, csr, "")
		require.ErrorIs(t, err, agentpki.ErrCertificateRevoked, "the revoked Agent certificate gets no link certificate")

		// A disabled node gets none either, even before its revocation ran.
		require.NoError(t, f.db.Model(&model.Node{}).Where("id = ?", f.proxy.ID).Update("status", model.NodeStatusDisabled).Error)
		_, err = f.pki.IssueLinkCertificate(t.Context(), proxyPeer, csr, "")
		require.ErrorIs(t, err, agentpki.ErrCertificateRevoked)

		// Without the link table RevokeNode still revokes the Agent's.
		require.NoError(t, f.db.Migrator().DropTable(&model.ForwardLinkCertificate{}))
		require.NoError(t, agentpki.RevokeNode(t.Context(), f.db, proxy, agentpki.RevokeReasonNodeDisabled))
	})
}

// TestLinkCARotationOverlap: the next CA is in the bundle at once, signs only
// after one link certificate lifetime, and the retired CA stays in the bundle
// until its last link certificate has expired.
func TestLinkCARotationOverlap(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, f *fixture) {
		node := f.forwardNode()
		f.negotiate(t, node, true)
		issue := func() agentpki.LinkIssued {
			t.Helper()
			peer, _ := f.agentCertificate(t, node)
			_, csr := linkCSR(t, nil)
			issued, err := f.pki.IssueLinkCertificate(t.Context(), peer, csr, "")
			require.NoError(t, err)
			return issued
		}
		bundle := func() []*x509.Certificate {
			t.Helper()
			certificates, err := f.pki.LinkTrustBundle(t.Context())
			require.NoError(t, err)
			return certificates
		}
		old := issue()
		require.Len(t, bundle(), 1)
		next, err := f.link.Rotate(t.Context())
		require.NoError(t, err)
		again, err := f.link.Rotate(t.Context())
		require.NoError(t, err)
		assert.Equal(t, next.KeyID, again.KeyID, "rotating while a next CA exists returns it")
		require.Len(t, bundle(), 2, "the next CA is trusted at once")
		duringOverlap := issue()
		assert.Equal(t, old.IssuerKeyID, duringOverlap.IssuerKeyID, "the current CA keeps signing")
		assert.Len(t, duringOverlap.TrustBundleDER, 2, "issuance delivers the next CA")

		f.clock.Advance(agentpki.DefaultLinkCertificateLifetime - time.Minute)
		require.NoError(t, f.link.Maintain(t.Context()))
		assert.Equal(t, old.IssuerKeyID, issue().IssuerKeyID, "not promoted before one link lifetime")

		f.clock.Advance(time.Minute)
		require.NoError(t, f.link.Maintain(t.Context()))
		promoted := issue()
		assert.Equal(t, next.KeyID, promoted.IssuerKeyID)
		roots := bundle()
		require.Len(t, roots, 2, "the retired CA stays while its link certificates live")
		oldLeaf, err := x509.ParseCertificate(duringOverlap.CertificateDER)
		require.NoError(t, err)
		newLeaf, err := x509.ParseCertificate(promoted.CertificateDER)
		require.NoError(t, err)
		rootPool := x509.NewCertPool()
		for _, root := range roots {
			rootPool.AddCert(root)
		}
		for _, leaf := range []*x509.Certificate{oldLeaf, newLeaf} {
			_, err = leaf.Verify(x509.VerifyOptions{Roots: rootPool, DNSName: node.String(), CurrentTime: f.clock.Now()})
			require.NoError(t, err, "peers holding the bundle accept both generations")
		}

		f.clock.Advance(agentpki.DefaultLinkCertificateLifetime + 2*time.Minute)
		require.Len(t, bundle(), 1, "the retired CA leaves once its certificates expired")
		cas, err := f.link.CAs(t.Context())
		require.NoError(t, err)
		require.Len(t, cas, 2)
		assert.Equal(t, model.ForwardLinkCAStateRetired, cas[0].State)
		assert.Equal(t, model.ForwardLinkCAStateCurrent, cas[1].State)
	})
}

// TestLinkAuthorityKeys: the CA key is sealed with the KEK under the link
// PKI's own additional data, and the link CA is P-256, constrained to
// AnixOps SPIFFE URIs.
func TestLinkAuthorityKeys(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, f *fixture) {
		cas, err := f.link.CAs(t.Context())
		require.NoError(t, err)
		require.Len(t, cas, 1)
		root, err := f.pki.LinkTrustBundle(t.Context())
		require.NoError(t, err)
		assert.True(t, root[0].IsCA)
		assert.Equal(t, []string{"anixops"}, root[0].PermittedURIDomains)
		key, ok := root[0].PublicKey.(*ecdsa.PublicKey)
		require.True(t, ok)
		assert.Equal(t, elliptic.P256(), key.Curve)
		assert.NotContains(t, cas[0].SealedKey, "PRIVATE KEY")

		var moduleCA model.ServiceCA
		require.NoError(t, f.db.First(&moduleCA).Error)
		assert.NotEqual(t, moduleCA.CertificatePEM, cas[0].CertificatePEM, "a root of its own")

		wrongKEK := make([]byte, 32)
		wrong, err := agentpki.NewLinkAuthority(agentpki.LinkAuthorityOptions{DB: f.db, Cluster: testCluster, KEK: wrongKEK, Now: f.clock.Now})
		require.NoError(t, err)
		pki, err := agentpki.New(agentpki.Options{DB: f.db, Authority: f.authority, Now: f.clock.Now, Link: wrong})
		require.NoError(t, err)
		node := f.forwardNode()
		peer, _ := f.agentCertificate(t, node)
		f.negotiate(t, node, true)
		_, csr := linkCSR(t, nil)
		_, err = pki.IssueLinkCertificate(t.Context(), peer, csr, "")
		require.ErrorContains(t, err, "wrong key-encryption key")
	})
}

func TestLinkCertificateWithoutTheLinkCA(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, f *fixture) {
		pki, err := agentpki.New(agentpki.Options{DB: f.db, Authority: f.authority, Now: f.clock.Now})
		require.NoError(t, err)
		assert.Nil(t, pki.Link())
		node := f.forwardNode()
		peer, _ := f.agentCertificate(t, node)
		_, csr := linkCSR(t, nil)
		_, err = pki.IssueLinkCertificate(t.Context(), peer, csr, "")
		require.ErrorIs(t, err, agentpki.ErrLinkDisabled)
		_, err = pki.LinkTrustBundle(t.Context())
		require.ErrorIs(t, err, agentpki.ErrLinkDisabled)

		// A link authority whose cluster has no CA yet.
		require.NoError(t, f.db.Where("1 = 1").Delete(&model.ForwardLinkCA{}).Error)
		f.negotiate(t, node, true)
		_, err = f.pki.IssueLinkCertificate(t.Context(), peer, csr, "")
		require.ErrorIs(t, err, agentpki.ErrNoLinkAuthority)
		_, err = f.pki.LinkTrustBundle(t.Context())
		require.ErrorIs(t, err, agentpki.ErrNoLinkAuthority)
	})
}

func TestLinkCertificatesArePruned(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, f *fixture) {
		node := f.forwardNode()
		peer, _ := f.agentCertificate(t, node)
		f.negotiate(t, node, true)
		_, csr := linkCSR(t, nil)
		_, err := f.pki.IssueLinkCertificate(t.Context(), peer, csr, "")
		require.NoError(t, err)
		var count int64
		require.NoError(t, f.pki.Prune(t.Context(), f.clock.Now()))
		require.NoError(t, f.db.Model(&model.ForwardLinkCertificate{}).Count(&count).Error)
		assert.Equal(t, int64(1), count)
		require.NoError(t, f.pki.Prune(t.Context(), f.clock.Now().Add(agentpki.DefaultLinkCertificateLifetime+time.Hour)))
		require.NoError(t, f.db.Model(&model.ForwardLinkCertificate{}).Count(&count).Error)
		assert.Equal(t, int64(0), count)
	})
}

func TestLinkAuthorityOptionsAndConfig(t *testing.T) {
	db := openSQLiteForConfig(t)
	kek := make([]byte, 32)
	for name, opts := range map[string]agentpki.LinkAuthorityOptions{
		"no database":    {Cluster: testCluster, KEK: kek},
		"bad cluster":    {DB: db, Cluster: "Bad Cluster", KEK: kek},
		"short KEK":      {DB: db, Cluster: testCluster, KEK: kek[:16]},
		"short lifetime": {DB: db, Cluster: testCluster, KEK: kek, Lifetime: time.Minute},
	} {
		_, err := agentpki.NewLinkAuthority(opts)
		require.Error(t, err, name)
	}
	link, err := agentpki.NewLinkAuthority(agentpki.LinkAuthorityOptions{DB: db, Cluster: testCluster, KEK: kek})
	require.NoError(t, err)
	assert.Equal(t, testCluster, link.Cluster())
	assert.Equal(t, agentpki.DefaultLinkCertificateLifetime, link.Lifetime())

	cfg := &config.Config{ModuleRuntime: config.ModuleRuntimeConfig{CAKEK: "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=", Cluster: "edge"}}
	link, err = agentpki.LinkAuthorityFromConfig(cfg, db)
	require.NoError(t, err)
	assert.Equal(t, "edge", link.Cluster())
	pki, err := agentpki.FromConfig(cfg, db)
	require.NoError(t, err)
	require.NotNil(t, pki.Link(), "the agent PKI serves link certificates with the built-in CA")

	_, err = agentpki.LinkAuthorityFromConfig(nil, db)
	require.ErrorIs(t, err, agentpki.ErrLinkDisabled)
	_, err = agentpki.LinkAuthorityFromConfig(&config.Config{ModuleRuntime: config.ModuleRuntimeConfig{Enabled: true, PKI: config.ModulePKIExternal}}, db)
	require.ErrorIs(t, err, agentpki.ErrLinkDisabled, "an external PKI holds no key")
	_, err = agentpki.LinkAuthorityFromConfig(&config.Config{ModuleRuntime: config.ModuleRuntimeConfig{CAKEK: "short"}}, db)
	require.Error(t, err)
}
