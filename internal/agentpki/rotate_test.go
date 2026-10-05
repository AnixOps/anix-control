package agentpki_test

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v4/internal/agentpki"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/nodesecrets"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func (f *fixture) rotate(t *testing.T, node agentcontrol.AgentNode, mutate func(*agentpki.RotateRequest)) (agentpki.Rotation, error) {
	t.Helper()
	request := agentpki.RotateRequest{
		Node: node, TTL: time.Hour, CreatedBy: 7, Actor: "root@example.com", IP: "203.0.113.9", Reason: "key left on a build host",
	}
	if mutate != nil {
		mutate(&request)
	}
	return f.pki.RotateCredentials(t.Context(), request)
}

func (f *fixture) enrollWith(t *testing.T, credential string) (agentpki.Issued, error) {
	t.Helper()
	return f.enroll(t, agentpki.Bootstrap{Method: model.AgentEnrollmentMethodCredential, Secret: credential})
}

// TestRotateCredentialsRevokesAndReissues: the node's Agent certificate,
// its enrollments (an unused credential included) and its link
// certificates stop working, a fresh one-time credential enrolls, and the
// node of the same id but the other kind is left alone.
func TestRotateCredentialsRevokesAndReissues(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, f *fixture) {
		proxy, forward := f.proxyNode(), f.forwardNode()
		issued, err := f.enroll(t, agentpki.Bootstrap{Method: model.AgentEnrollmentMethodNodeAPIKey, Node: proxy, Secret: f.proxyKey})
		require.NoError(t, err)
		peer := leafOf(t, issued)
		f.negotiate(t, proxy, true)
		_, csr := linkCSR(t, &x509.CertificateRequest{Subject: pkix.Name{CommonName: "x"}, URIs: []*url.URL{spiffeURL(t, proxy)}})
		link, err := f.pki.IssueLinkCertificate(t.Context(), peer, csr, "198.51.100.10")
		require.NoError(t, err)
		unused := f.token(t, proxy, time.Hour)
		forwardIssued, err := f.enroll(t, agentpki.Bootstrap{Method: model.AgentEnrollmentMethodForwardToken, Node: forward, Secret: f.forwardToken})
		require.NoError(t, err)

		rotation, err := f.rotate(t, proxy, nil)
		require.NoError(t, err)
		assert.Equal(t, agentpki.ActiveCredentials{Certificates: 1, Enrollments: 2, LinkCertificates: 1}, rotation.Revoked)
		assert.False(t, rotation.APIKeyRotated)
		require.NotEmpty(t, rotation.Credential)
		assert.Contains(t, rotation.Credential, agentcontrol.EnrollmentCredentialPrefix)
		require.NotNil(t, rotation.Enrollment.CredentialHash)
		assert.NotEqual(t, rotation.Credential, *rotation.Enrollment.CredentialHash, "only the hash is stored")
		require.NotNil(t, rotation.Enrollment.ExpiresAt)
		assert.Equal(t, f.clock.Now().Add(time.Hour), *rotation.Enrollment.ExpiresAt)
		assert.Equal(t, uint(7), rotation.Enrollment.CreatedBy)

		// The old identity: refused on connect, on renewal and on enrollment.
		_, _, err = f.pki.VerifyPeer(t.Context(), [][]byte{issued.CertificateDER})
		require.ErrorIs(t, err, agentpki.ErrCertificateRevoked)
		_, csr = newCSR(t)
		_, err = f.pki.Renew(t.Context(), peer, csr)
		require.ErrorIs(t, err, agentpki.ErrCertificateRevoked)
		_, err = f.enrollWith(t, unused)
		require.ErrorIs(t, err, agentpki.ErrEnrollmentRejected)
		var linkRecord model.ForwardLinkCertificate
		require.NoError(t, f.db.First(&linkRecord, "serial = ?", link.Serial).Error)
		require.NotNil(t, linkRecord.RevokedAt)
		assert.Equal(t, agentpki.RevokeReasonCredentialsRotated, linkRecord.RevokeReason)
		var certificate model.AgentCertificate
		require.NoError(t, f.db.First(&certificate, "serial = ?", issued.Serial).Error)
		assert.Equal(t, agentpki.RevokeReasonCredentialsRotated, certificate.RevokeReason)

		// The other kind of the same id is untouched.
		_, _, err = f.pki.VerifyPeer(t.Context(), [][]byte{forwardIssued.CertificateDER})
		require.NoError(t, err)

		// The new credential enrolls, once.
		fresh, err := f.enrollWith(t, rotation.Credential)
		require.NoError(t, err)
		requireNodeCertificate(t, f, fresh, proxy)
		_, err = f.enrollWith(t, rotation.Credential)
		require.ErrorIs(t, err, agentpki.ErrEnrollmentRejected)
	})
}

// TestRotateCredentialsIsRepeatable: each rotation voids the previous
// credential, so exactly the last one enrolls, and a rotation of a node
// whose Agent never enrolled works too (offline or new node).
func TestRotateCredentialsIsRepeatable(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, f *fixture) {
		node := f.proxyNode()
		first, err := f.rotate(t, node, nil)
		require.NoError(t, err)
		assert.Equal(t, agentpki.ActiveCredentials{}, first.Revoked)
		second, err := f.rotate(t, node, nil)
		require.NoError(t, err)
		assert.Equal(t, agentpki.ActiveCredentials{Enrollments: 1}, second.Revoked)
		third, err := f.rotate(t, node, nil)
		require.NoError(t, err)
		assert.NotEqual(t, first.Credential, second.Credential)

		for _, old := range []string{first.Credential, second.Credential} {
			_, err = f.enrollWith(t, old)
			require.ErrorIs(t, err, agentpki.ErrEnrollmentRejected)
		}
		issued, err := f.enrollWith(t, third.Credential)
		require.NoError(t, err)
		requireNodeCertificate(t, f, issued, node)

		var unused int64
		require.NoError(t, f.db.Model(&model.AgentEnrollment{}).
			Where("node_kind = ? AND node_id = ? AND method = ? AND used_at IS NULL AND revoked_at IS NULL", node.Kind, node.ID, model.AgentEnrollmentMethodCredential).
			Count(&unused).Error)
		assert.Zero(t, unused)
	})
}

// TestRotateCredentialsRefusesMissingAndDisabledNodes changes nothing.
func TestRotateCredentialsRefusesMissingAndDisabledNodes(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, f *fixture) {
		issued, err := f.enroll(t, agentpki.Bootstrap{Method: model.AgentEnrollmentMethodNodeAPIKey, Node: f.proxyNode(), Secret: f.proxyKey})
		require.NoError(t, err)

		_, err = f.rotate(t, agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: uint32(f.proxy.ID) + 100}, nil)
		require.ErrorIs(t, err, agentpki.ErrInvalidNode)
		_, err = f.rotate(t, agentcontrol.AgentNode{Kind: agentcontrol.NodeKindForward, ID: uint32(f.forward.ID) + 100}, nil)
		require.ErrorIs(t, err, agentpki.ErrInvalidNode)
		_, err = f.rotate(t, agentcontrol.AgentNode{Kind: "module", ID: 1}, nil)
		require.ErrorIs(t, err, agentcontrol.ErrInvalidAgentIdentity)

		require.NoError(t, f.db.Model(&model.Node{}).Where("id = ?", f.proxy.ID).Update("status", model.NodeStatusDisabled).Error)
		require.NoError(t, f.db.Model(&model.ForwardNode{}).Where("id = ?", f.forward.ID).Update("enabled", false).Error)
		_, err = f.rotate(t, f.proxyNode(), nil)
		require.ErrorIs(t, err, agentpki.ErrNodeDisabled)
		_, err = f.rotate(t, f.forwardNode(), nil)
		require.ErrorIs(t, err, agentpki.ErrNodeDisabled)

		require.NoError(t, f.db.Model(&model.Node{}).Where("id = ?", f.proxy.ID).Update("status", model.NodeStatusOnline).Error)
		var revoked, enrollments, audits int64
		require.NoError(t, f.db.Model(&model.AgentCertificate{}).Where("serial = ? AND revoked_at IS NOT NULL", issued.Serial).Count(&revoked).Error)
		require.NoError(t, f.db.Model(&model.AgentEnrollment{}).Count(&enrollments).Error)
		require.NoError(t, f.db.Model(&model.OperationLog{}).Where("action = ?", agentpki.AuditActionCredentialsRotate).Count(&audits).Error)
		assert.Zero(t, revoked, "a refused rotation revokes nothing")
		assert.EqualValues(t, 1, enrollments)
		assert.Zero(t, audits)
		_, _, err = f.pki.VerifyPeer(t.Context(), [][]byte{issued.CertificateDER})
		require.NoError(t, err)
	})
}

// TestRotateCredentialsForwardNode rotates a forward node's Agent
// credentials; its token is not part of it.
func TestRotateCredentialsForwardNode(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, f *fixture) {
		node := f.forwardNode()
		issued, err := f.enroll(t, agentpki.Bootstrap{Method: model.AgentEnrollmentMethodForwardToken, Node: node, Secret: f.forwardToken})
		require.NoError(t, err)
		rotation, err := f.rotate(t, node, nil)
		require.NoError(t, err)
		assert.Equal(t, agentpki.ActiveCredentials{Certificates: 1, Enrollments: 1}, rotation.Revoked)
		_, _, err = f.pki.VerifyPeer(t.Context(), [][]byte{issued.CertificateDER})
		require.ErrorIs(t, err, agentpki.ErrCertificateRevoked)
		fresh, err := f.enrollWith(t, rotation.Credential)
		require.NoError(t, err)
		requireNodeCertificate(t, f, fresh, node)
		// The forward token still enrolls: it was not rotated.
		_, err = f.enroll(t, agentpki.Bootstrap{Method: model.AgentEnrollmentMethodForwardToken, Node: node, Secret: f.forwardToken})
		require.NoError(t, err)
		// The proxy node of the same id is untouched.
		assert.Zero(t, countRevokedEnrollments(t, f, f.proxyNode()))
	})
}

func countRevokedEnrollments(t *testing.T, f *fixture, node agentcontrol.AgentNode) int64 {
	t.Helper()
	var count int64
	require.NoError(t, f.db.Model(&model.AgentEnrollment{}).Where("node_kind = ? AND node_id = ? AND revoked_at IS NOT NULL", node.Kind, node.ID).Count(&count).Error)
	return count
}

// TestRotateCredentialsReplacesTheNodeKey: with ReplaceNodeKey the proxy
// node's old API key stops authenticating (here and as an enrollment
// bootstrap), the new one is stored and hashed, in the legacy columns and
// through the credential split, and a failing replacement rolls the whole
// rotation back.
func TestRotateCredentialsReplacesTheNodeKey(t *testing.T) {
	for _, phase := range []string{"legacy", nodesecrets.PhaseDualWrite, nodesecrets.PhaseDualRead} {
		t.Run("phase "+phase, func(t *testing.T) {
			forEachDatabase(t, func(t *testing.T, f *fixture) {
				if phase != "legacy" {
					require.NoError(t, nodesecrets.EnsureSchema(f.db))
					tables := []string{nodesecrets.TableNode}
					_, err := nodesecrets.Backfill(t.Context(), f.db, nodesecrets.BackfillOptions{Tables: tables})
					require.NoError(t, err)
					_, err = nodesecrets.Verify(t.Context(), f.db, nodesecrets.VerifyOptions{Tables: tables})
					require.NoError(t, err)
					_, err = nodesecrets.SetPhase(t.Context(), f.db, nodesecrets.PhaseOptions{Tables: tables, Phase: phase, Actor: "test"})
					require.NoError(t, err)
				}
				var oldKey string
				{
					var row model.Node
					require.NoError(t, f.db.First(&row, f.proxy.ID).Error)
					oldKey = nodesecrets.NodeAPIKey(f.db, &row)
				}
				_, err := nodesecrets.NodeByAPIKey(f.db, oldKey, true)
				require.NoError(t, err, "the old key works before the rotation")
				issued, err := f.enroll(t, agentpki.Bootstrap{Method: model.AgentEnrollmentMethodNodeAPIKey, Node: f.proxyNode(), Secret: oldKey})
				require.NoError(t, err)

				// A failing replacement leaves everything as it was.
				_, err = f.rotate(t, f.proxyNode(), func(r *agentpki.RotateRequest) {
					r.ReplaceNodeKey = func(*gorm.DB) error { return errors.New("replacement failed") }
				})
				require.EqualError(t, err, "replacement failed")
				_, _, err = f.pki.VerifyPeer(t.Context(), [][]byte{issued.CertificateDER})
				require.NoError(t, err, "a failed rotation revokes nothing")

				rotation, err := f.rotate(t, f.proxyNode(), func(r *agentpki.RotateRequest) {
					r.ReplaceNodeKey = func(tx *gorm.DB) error {
						credentials, err := service.GenerateProxyNodeCredentials(true, false)
						if err != nil {
							return err
						}
						return service.IssueProxyNodeCredentialsTx(tx, f.proxy.ID, credentials, false)
					}
				})
				require.NoError(t, err)
				assert.True(t, rotation.APIKeyRotated)
				assert.Equal(t, int64(1), rotation.Revoked.Certificates)

				_, err = nodesecrets.NodeByAPIKey(f.db, oldKey, true)
				require.Error(t, err, "the old key no longer authenticates")
				_, err = f.enroll(t, agentpki.Bootstrap{Method: model.AgentEnrollmentMethodNodeAPIKey, Node: f.proxyNode(), Secret: oldKey})
				require.ErrorIs(t, err, agentpki.ErrEnrollmentRejected)
				_, _, err = f.pki.VerifyPeer(t.Context(), [][]byte{issued.CertificateDER})
				require.ErrorIs(t, err, agentpki.ErrCertificateRevoked)

				var row model.Node
				require.NoError(t, f.db.First(&row, f.proxy.ID).Error)
				newKey := nodesecrets.NodeAPIKey(f.db, &row)
				require.NotEqual(t, oldKey, newKey)
				require.False(t, nodesecrets.Unusable(newKey))
				found, err := nodesecrets.NodeByAPIKey(f.db, newKey, true)
				require.NoError(t, err)
				assert.Equal(t, f.proxy.ID, found.ID)

				fresh, err := f.enrollWith(t, rotation.Credential)
				require.NoError(t, err)
				requireNodeCertificate(t, f, fresh, f.proxyNode())
			})
		})
	}
}

// TestRotateCredentialsAuditsWithoutSecrets: the rotation writes its own
// entry besides the enrollment token's, with the counts and the reason and
// no credential.
func TestRotateCredentialsAuditsWithoutSecrets(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, f *fixture) {
		_, err := f.enroll(t, agentpki.Bootstrap{Method: model.AgentEnrollmentMethodNodeAPIKey, Node: f.proxyNode(), Secret: f.proxyKey})
		require.NoError(t, err)
		rotation, err := f.rotate(t, f.proxyNode(), nil)
		require.NoError(t, err)

		var entries []model.OperationLog
		require.NoError(t, f.db.Where("action IN ?", []string{agentpki.AuditActionCredentialsRotate, agentpki.AuditActionTokenIssue}).Order("id").Find(&entries).Error)
		require.Len(t, entries, 2)
		assert.Equal(t, agentpki.AuditActionTokenIssue, entries[0].Action)
		rotate := entries[1]
		assert.Equal(t, agentpki.AuditActionCredentialsRotate, rotate.Action)
		assert.Equal(t, "agent_pki", rotate.Module)
		assert.Equal(t, "proxy_node", rotate.TargetType)
		assert.Equal(t, "root@example.com", rotate.Username)
		assert.Equal(t, "203.0.113.9", rotate.IP)
		require.NotNil(t, rotate.UserID)
		assert.Equal(t, uint(7), *rotate.UserID)
		assert.Contains(t, rotate.Content, `"reason":"key left on a build host"`)
		assert.Contains(t, rotate.Content, `"revoked_certificates":1`)
		assert.Contains(t, rotate.Content, `"api_key_rotated":false`)
		assert.Contains(t, rotate.Content, rotation.Enrollment.ID)
		for _, entry := range entries {
			for _, secret := range []string{rotation.Credential, f.proxyKey, f.forwardToken, *rotation.Enrollment.CredentialHash} {
				assert.NotContains(t, entry.Content, secret)
				assert.NotContains(t, entry.Username, secret)
			}
		}
	})
}

// TestConcurrentRotationsLeaveOneCredential: rotations of one node run one
// after the other (the node row is locked), so of N concurrent rotations
// exactly one credential is left unused and valid. SQLite has one
// connection in these tests; the check is the PostgreSQL run's.
func TestConcurrentRotationsLeaveOneCredential(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, f *fixture) {
		if f.db.Dialector.Name() != "postgres" {
			t.Skip("needs concurrent connections")
		}
		const callers = 6
		credentials := make([]string, callers)
		errs := make([]error, callers)
		var wg sync.WaitGroup
		for i := range callers {
			wg.Add(1)
			go func() {
				defer wg.Done()
				rotation, err := f.pki.RotateCredentials(t.Context(), agentpki.RotateRequest{Node: f.proxyNode(), TTL: time.Hour, CreatedBy: 7})
				credentials[i], errs[i] = rotation.Credential, err
			}()
		}
		wg.Wait()
		valid := 0
		for i := range callers {
			require.NoError(t, errs[i])
			if _, err := f.enrollWith(t, credentials[i]); err == nil {
				valid++
			}
		}
		assert.Equal(t, 1, valid, "exactly one credential survives")
	})
}
