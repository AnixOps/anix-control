package agentpki_test

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v4/internal/agentpki"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// holdAt pauses the next insert into table, in whatever transaction makes
// it, until the returned release is called; reached closes when it is held.
func holdAt(t *testing.T, db *gorm.DB, table string) (reached <-chan struct{}, release func()) {
	t.Helper()
	var armed atomic.Bool
	armed.Store(true)
	held := make(chan struct{})
	go_ := make(chan struct{})
	name := "test:hold_" + table
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Table == table && armed.CompareAndSwap(true, false) {
			close(held)
			<-go_
		}
	}))
	t.Cleanup(func() { _ = db.Callback().Create().Remove(name) })
	return held, func() { close(go_) }
}

// raceRevocation runs inFlight, holds it just before it records a
// certificate or enrollment, runs the revocation of the node meanwhile, then
// lets inFlight finish. Whatever the interleaving, the revocation must be
// complete once both are done: PostgreSQL gives every statement its own
// snapshot, so an issuance that began before the revocation committed would
// otherwise record a live credential after the revocation passed over the
// existing ones.
func raceRevocation(t *testing.T, f *fixture, table string, node agentcontrol.AgentNode, inFlight func() error) {
	t.Helper()
	reached, release := holdAt(t, f.db, table)
	finished := make(chan error, 1)
	go func() { finished <- inFlight() }()
	select {
	case <-reached:
	case <-time.After(10 * time.Second):
		t.Fatal("the issuance never reached its insert")
	}
	revoked := make(chan error, 1)
	go func() {
		revoked <- agentpki.RevokeNode(t.Context(), f.db, node, agentpki.RevokeReasonCredentialsRevoked)
	}()
	var revokeErr error
	revokeDone := false
	select {
	case revokeErr = <-revoked:
		revokeDone = true
	case <-time.After(500 * time.Millisecond):
		// The revocation waits for the issuance, which is what we want.
	}
	release()
	if !revokeDone {
		revokeErr = <-revoked
	}
	require.NoError(t, revokeErr)
	<-finished

	for model, query := range map[string]*gorm.DB{
		"certificate":      f.db.Model(&model.AgentCertificate{}).Where("node_kind = ? AND node_id = ? AND revoked_at IS NULL", node.Kind, node.ID),
		"enrollment":       f.db.Model(&model.AgentEnrollment{}).Where("node_kind = ? AND node_id = ? AND revoked_at IS NULL", node.Kind, node.ID),
		"link certificate": f.db.Model(&model.ForwardLinkCertificate{}).Where("node_kind = ? AND node_id = ? AND revoked_at IS NULL", node.Kind, node.ID),
	} {
		var live int64
		require.NoError(t, query.Count(&live).Error)
		require.Zero(t, live, "a revoked node has no live %s, whatever interleaving the issuance had", model)
	}
}

func TestIssuanceInFlightDuringRevocationLeavesNothingLive(t *testing.T) {
	newFixtureOnPostgres := func(t *testing.T) *fixture { return newFixture(t, openPostgres(t)) }

	t.Run("renewal", func(t *testing.T) {
		f := newFixtureOnPostgres(t)
		node := f.proxyNode()
		peer, _ := f.agentCertificate(t, node)
		_, csr := newCSR(t)
		raceRevocation(t, f, (model.AgentCertificate{}).TableName(), node, func() error {
			_, err := f.pki.Renew(t.Context(), peer, csr)
			return err
		})
	})

	t.Run("enrollment with the node key", func(t *testing.T) {
		f := newFixtureOnPostgres(t)
		node := f.proxyNode()
		_, csr := newCSR(t)
		raceRevocation(t, f, (model.AgentCertificate{}).TableName(), node, func() error {
			_, err := f.pki.Enroll(t.Context(), agentpki.EnrollRequest{
				Bootstrap: agentpki.Bootstrap{Method: model.AgentEnrollmentMethodNodeAPIKey, Node: node, Secret: f.proxyKey}, CSRDER: csr,
			})
			return err
		})
	})

	t.Run("link certificate", func(t *testing.T) {
		f := newFixtureOnPostgres(t)
		node := f.forwardNode()
		peer, _ := f.agentCertificate(t, node)
		f.negotiate(t, node, true)
		spiffe, err := url.Parse("spiffe://anixops/" + testCluster + "/agent/" + node.String())
		require.NoError(t, err)
		_, csr := linkCSR(t, &x509.CertificateRequest{Subject: pkix.Name{CommonName: "ignored"}, DNSNames: []string{node.String()}, URIs: []*url.URL{spiffe}})
		raceRevocation(t, f, (model.ForwardLinkCertificate{}).TableName(), node, func() error {
			_, err := f.pki.IssueLinkCertificate(t.Context(), peer, csr, "198.51.100.20")
			return err
		})
	})
}
