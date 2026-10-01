package service

import (
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v4/internal/agentpki"
	"github.com/AnixOps/anix-control/v4/internal/cache"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func agentPKIHookDB(t *testing.T) *gorm.DB {
	t.Helper()
	cache.InitMemory()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&model.Node{}, &model.NodeProtocol{}, &model.ForwardNode{},
		&model.AgentEnrollment{}, &model.AgentCertificate{}))
	return db
}

// seedAgentCertificate records an unrevoked certificate, with its
// enrollment, of a node.
func seedAgentCertificate(t *testing.T, db *gorm.DB, kind string, nodeID uint, serial string) {
	t.Helper()
	enrollment := model.AgentEnrollment{
		ID: serial + "-enrollment", NodeKind: kind, NodeID: nodeID, Cluster: "default",
		Method: model.AgentEnrollmentMethodNodeAPIKey, CreatedAt: time.Now(),
	}
	require.NoError(t, db.Create(&enrollment).Error)
	require.NoError(t, db.Create(&model.AgentCertificate{
		Serial: serial, NodeKind: kind, NodeID: nodeID, Cluster: "default", EnrollmentID: enrollment.ID,
		IssuerKeyID: "issuer", NotAfter: time.Now().Add(time.Hour), CreatedAt: time.Now(),
	}).Error)
}

func agentCertificateRevocation(t *testing.T, db *gorm.DB, serial string) string {
	t.Helper()
	var record model.AgentCertificate
	require.NoError(t, db.First(&record, "serial = ?", serial).Error)
	if record.RevokedAt == nil {
		return ""
	}
	return record.RevokeReason
}

func TestProxyNodeChangesRevokeAgentCertificates(t *testing.T) {
	db := agentPKIHookDB(t)
	nodes := &NodeService{db: db}
	node := model.Node{Name: "proxy", APIKeyHash: strings.Repeat("c", 64), Status: model.NodeStatusOnline}
	require.NoError(t, db.Create(&node).Error)
	seedAgentCertificate(t, db, agentcontrol.NodeKindProxy, node.ID, "proxy-cert")
	// A forward node with the same id is another node.
	seedAgentCertificate(t, db, agentcontrol.NodeKindForward, node.ID, "forward-cert")

	require.NoError(t, nodes.UpdateNode(node.ID, map[string]any{"name": "renamed"}))
	assert.Empty(t, agentCertificateRevocation(t, db, "proxy-cert"))
	require.NoError(t, nodes.UpdateNode(node.ID, map[string]any{"status": float64(model.NodeStatusOffline)}))
	assert.Empty(t, agentCertificateRevocation(t, db, "proxy-cert"))

	require.NoError(t, nodes.UpdateNode(node.ID, map[string]any{"status": float64(model.NodeStatusDisabled)}))
	assert.Equal(t, agentpki.RevokeReasonNodeDisabled, agentCertificateRevocation(t, db, "proxy-cert"))
	var enrollment model.AgentEnrollment
	require.NoError(t, db.First(&enrollment, "id = ?", "proxy-cert-enrollment").Error)
	assert.NotNil(t, enrollment.RevokedAt)
	assert.Empty(t, agentCertificateRevocation(t, db, "forward-cert"))

	seedAgentCertificate(t, db, agentcontrol.NodeKindProxy, node.ID, "proxy-cert-2")
	require.NoError(t, nodes.DeleteNode(node.ID))
	assert.Equal(t, agentpki.RevokeReasonNodeDeleted, agentCertificateRevocation(t, db, "proxy-cert-2"))
	assert.Empty(t, agentCertificateRevocation(t, db, "forward-cert"))
}

func TestForwardNodeChangesRevokeAgentCertificates(t *testing.T) {
	db := agentPKIHookDB(t)
	forwards := NewForwardNodeService(db)
	node := &model.ForwardNode{Name: "forward", Host: "198.51.100.30", Port: 8443, APIToken: "forward-token", Enabled: true}
	require.NoError(t, forwards.Create(node))
	seedAgentCertificate(t, db, agentcontrol.NodeKindForward, node.ID, "forward-cert")

	node.Name = "renamed"
	require.NoError(t, forwards.Update(node))
	assert.Empty(t, agentCertificateRevocation(t, db, "forward-cert"), "an unrelated change keeps the certificates")

	node.APIToken = "replaced-token"
	require.NoError(t, forwards.Update(node))
	assert.Equal(t, agentpki.RevokeReasonCredentialsReplaced, agentCertificateRevocation(t, db, "forward-cert"))

	seedAgentCertificate(t, db, agentcontrol.NodeKindForward, node.ID, "forward-cert-2")
	node.Enabled = false
	require.NoError(t, forwards.Update(node))
	assert.Equal(t, agentpki.RevokeReasonNodeDisabled, agentCertificateRevocation(t, db, "forward-cert-2"))

	seedAgentCertificate(t, db, agentcontrol.NodeKindForward, node.ID, "forward-cert-3")
	require.NoError(t, forwards.Delete(node.ID))
	assert.Equal(t, agentpki.RevokeReasonNodeDeleted, agentCertificateRevocation(t, db, "forward-cert-3"))
}

func TestNodeWritersWorkWithoutAgentPKITables(t *testing.T) {
	cache.InitMemory()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Node{}, &model.NodeProtocol{}, &model.ForwardNode{}))
	node := model.Node{Name: "proxy", APIKeyHash: strings.Repeat("d", 64)}
	require.NoError(t, db.Create(&node).Error)
	nodes := &NodeService{db: db}
	require.NoError(t, nodes.UpdateNode(node.ID, map[string]any{"status": float64(model.NodeStatusDisabled)}))
	require.NoError(t, nodes.DeleteNode(node.ID))
	forwards := NewForwardNodeService(db)
	forward := &model.ForwardNode{Name: "forward", Host: "198.51.100.31", Port: 1, APIToken: "a", Enabled: true}
	require.NoError(t, forwards.Create(forward))
	forward.APIToken = "b"
	require.NoError(t, forwards.Update(forward))
	require.NoError(t, forwards.Delete(forward.ID))
}

func TestAgentPKITablesAreProtected(t *testing.T) {
	for _, table := range []string{"v4_kernel_agent_enrollment", "v4_kernel_agent_certificate"} {
		assert.True(t, protectedKernelTable(table), table)
		assert.Error(t, validateManifestCapabilities([]string{CapabilityStorage, "kernel.storage.adopt:" + table}), table)
	}
}
