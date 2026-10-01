package kernelnodeops

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/v4/internal/agentpki"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/nodesecrets"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

const linksTable = "v2_subscription_group_node_protocols"

func retireNodeSpec(kind kernelnodeopsv1.NodeKind, id uint64) *kernelnodeopsv1.OperationSpec {
	return &kernelnodeopsv1.OperationSpec{Operation: &kernelnodeopsv1.OperationSpec_RetireNode{RetireNode: &kernelnodeopsv1.RetireNode{Node: nodeRef(kind, id)}}}
}

func retireProtocolSpec(id uint64) *kernelnodeopsv1.OperationSpec {
	return &kernelnodeopsv1.OperationSpec{Operation: &kernelnodeopsv1.OperationSpec_RetireProtocol{RetireProtocol: &kernelnodeopsv1.RetireProtocol{ProtocolId: id}}}
}

// seedCascade gives proxy node 1 a second protocol, each protocol a
// subscription group link, a WireGuard peer and a secret, the node a raw
// configuration secret, and nodes 1 and 10 an agent certificate. Every
// split row is dual-written.
func seedCascade(t *testing.T, db *gorm.DB) {
	t.Helper()
	reality := `{"dest":"www.example.test:443","private_key":"reality-private-5","public_key":"pub"}`
	settings := `{"password":"ss-password-6"}`
	raw := `{"type":"vless","tls_settings":{"private_key":"raw-private-1"}}`
	require.NoError(t, db.Model(&model.NodeProtocol{}).Where("id = ?", 5).Update("reality_settings", reality).Error)
	require.NoError(t, db.Create(&model.NodeProtocol{ID: 6, NodeID: 1, Name: "ss", Type: model.ProtocolShadowsocks, Port: 8388, Settings: &settings}).Error)
	require.NoError(t, db.Model(&model.Node{}).Where("id = ?", 1).Update("raw_config", raw).Error)
	group := &model.SubscriptionGroup{Name: "group", Enable: 1}
	require.NoError(t, db.Create(group).Error)
	require.NoError(t, db.Exec("INSERT INTO "+linksTable+" (subscription_group_id, node_protocol_id) VALUES (?, ?), (?, ?)", group.ID, 5, group.ID, 6).Error)
	require.NoError(t, db.Create(&[]model.WireGuardPeer{
		{NodeProtocolID: 5, UserID: 1, PeerIP: "10.9.0.2", PrivateKey: "peer-private-5", PublicKey: "pub"},
		{NodeProtocolID: 6, UserID: 1, PeerIP: "10.9.0.3", PrivateKey: "peer-private-6", PublicKey: "pub"},
	}).Error)
	require.NoError(t, db.Create(&[]model.AgentCertificate{
		{Serial: "cert-proxy-1", NodeKind: "proxy", NodeID: 1, Cluster: "test", EnrollmentID: "e", IssuerKeyID: "k", NotAfter: time.Now().Add(time.Hour)},
		{Serial: "cert-forward-10", NodeKind: "forward", NodeID: 10, Cluster: "test", EnrollmentID: "e", IssuerKeyID: "k", NotAfter: time.Now().Add(time.Hour)},
	}).Error)
	require.NoError(t, db.Model(&model.ForwardNode{}).Where("id = ?", 10).Update("api_token", "forward-token-10").Error)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		for _, write := range []struct {
			table string
			ids   []uint
		}{
			{nodesecrets.TableNode, []uint{1}}, {nodesecrets.TableNodeProtocol, []uint{5, 6}},
			{nodesecrets.TableWireGuardPeer, []uint{1, 2}}, {nodesecrets.TableForwardNode, []uint{10}},
		} {
			if err := nodesecrets.Sync(tx, write.table, write.ids...); err != nil {
				return err
			}
		}
		return nil
	}))
}

func count(t *testing.T, db *gorm.DB, value any, query string, args ...any) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Model(value).Where(query, args...).Count(&n).Error)
	return n
}

func countTable(t *testing.T, db *gorm.DB, table, query string, args ...any) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Table(table).Where(query, args...).Count(&n).Error)
	return n
}

// RetireNode removes everything the kernel holds for a proxy node, as the
// legacy deletion does, and leaves the node row to the package. A retired
// forward node loses its token's split row and its certificates.
func TestRetireNode(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		h := newCredentialHarness(t, db)
		seedCascade(t, db)
		client := h.client(proxyHost, allFamilies())
		require.EqualValues(t, 5, count(t, db, &model.ProtocolSecret{}, "1 = 1"), "two protocol secrets, the raw configuration's and the two peers' keys")

		_, err := client.SubmitOperation(context.Background(), &kernelnodeopsv1.SubmitOperationRequest{RequestId: "node.retire:proxy-9", Operation: retireNodeSpec(proxyKind, 9)})
		require.Equal(t, codes.NotFound, status.Code(err))
		_, err = client.SubmitOperation(context.Background(), &kernelnodeopsv1.SubmitOperationRequest{RequestId: "node.retire:clean-agent-20",
			Operation: retireNodeSpec(kernelnodeopsv1.NodeKind_NODE_KIND_CLEAN_AGENT, 20)})
		require.Equal(t, codes.PermissionDenied, status.Code(err), "a node kind the operation does not take")

		operation, applied := submitBound(t, client, "node.retire:proxy-1", retireNodeSpec(proxyKind, 1), nil)
		require.True(t, applied)
		require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_SUCCEEDED, operation.GetState(), operation.GetError())
		require.Equal(t, kernelnodeopsv1.Channel_CHANNEL_KERNEL, operation.GetChannel())
		retired := operation.GetResult().GetRetire()
		require.EqualValues(t, 2, retired.GetProtocolsDeleted())
		require.EqualValues(t, 2, retired.GetWireguardPeersDeleted())
		require.EqualValues(t, 2, retired.GetSubscriptionLinksDeleted())
		require.EqualValues(t, 3, retired.GetSecretsDeleted(), "the protocols' secrets and the raw configuration's; the peers' keys went with the peers")
		require.EqualValues(t, 1, retired.GetCredentialsRevoked(), "the node's API key row")
		require.EqualValues(t, 1, count(t, db, &model.Node{}, "id = ?", 1), "the node row is the package's")
		require.Zero(t, count(t, db, &model.NodeProtocol{}, "node_id = ?", 1))
		require.Zero(t, count(t, db, &model.WireGuardPeer{}, "node_protocol_id IN ?", []uint{5, 6}))
		require.Zero(t, countTable(t, db, linksTable, "node_protocol_id IN ?", []uint{5, 6}))
		require.Zero(t, count(t, db, &model.ProtocolSecret{}, "1 = 1"))
		require.Zero(t, count(t, db, &model.NodeCredential{}, "subject_kind = ? AND subject_id = ?", nodesecrets.SubjectProxy, 1))
		var certificate model.AgentCertificate
		require.NoError(t, db.First(&certificate, "serial = ?", "cert-proxy-1").Error)
		require.NotNil(t, certificate.RevokedAt)
		require.Equal(t, agentpki.RevokeReasonNodeDeleted, certificate.RevokeReason)
		requireNoSecret(t, db, operation, "reality-private-5", "ss-password-6", "raw-private-1", "peer-private-5", "peer-private-6", "node-key-1")

		// A repeat is the first operation; a new retirement of what is
		// gone succeeds with nothing counted.
		again, applied := submitBound(t, client, "node.retire:proxy-1", retireNodeSpec(proxyKind, 1), nil)
		require.False(t, applied)
		require.Equal(t, operation.GetOperationId(), again.GetOperationId())
		require.NoError(t, db.Delete(&model.Node{}, 1).Error)
		operation, _ = submitBound(t, client, "node.retire:proxy-2", retireNodeSpec(proxyKind, 2), nil)
		require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_SUCCEEDED, operation.GetState(), operation.GetError())
		require.Zero(t, operation.GetResult().GetRetire().GetProtocolsDeleted())

		// A forward node.
		forward := h.client(forwardHost, allFamilies())
		operation, _ = submitBound(t, forward, "node.retire:forward-10", retireNodeSpec(forwardKind, 10), nil)
		require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_SUCCEEDED, operation.GetState(), operation.GetError())
		require.EqualValues(t, 1, operation.GetResult().GetRetire().GetCredentialsRevoked())
		require.Zero(t, count(t, db, &model.NodeCredential{}, "subject_kind = ? AND subject_id = ?", nodesecrets.SubjectForward, 10))
		require.EqualValues(t, 1, count(t, db, &model.ForwardNode{}, "id = ?", 10))
		var forwardCertificate model.AgentCertificate
		require.NoError(t, db.First(&forwardCertificate, "serial = ?", "cert-forward-10").Error)
		require.NotNil(t, forwardCertificate.RevokedAt)
		requireNoSecret(t, db, operation, "forward-token-10")
	})
}

// RetireProtocol removes a protocol's peers, links and secrets, and leaves
// the row to the package.
func TestRetireProtocol(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		h := newCredentialHarness(t, db)
		seedCascade(t, db)
		client := h.client(protocolHost, allFamilies())
		_, err := client.SubmitOperation(context.Background(), &kernelnodeopsv1.SubmitOperationRequest{RequestId: "protocol.retire:9", Operation: retireProtocolSpec(9)})
		require.Equal(t, codes.NotFound, status.Code(err))

		operation, _ := submitBound(t, client, "protocol.retire:5", retireProtocolSpec(5), nil)
		require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_SUCCEEDED, operation.GetState(), operation.GetError())
		retired := operation.GetResult().GetRetire()
		require.EqualValues(t, 1, retired.GetWireguardPeersDeleted())
		require.EqualValues(t, 1, retired.GetSubscriptionLinksDeleted())
		require.EqualValues(t, 1, retired.GetSecretsDeleted())
		require.Zero(t, retired.GetProtocolsDeleted(), "the row is the package's")
		require.EqualValues(t, 1, count(t, db, &model.NodeProtocol{}, "id = ?", 5))
		require.Zero(t, count(t, db, &model.WireGuardPeer{}, "node_protocol_id = ?", 5))
		require.EqualValues(t, 1, count(t, db, &model.WireGuardPeer{}, "node_protocol_id = ?", 6), "the other protocol keeps its peer")
		require.Zero(t, countTable(t, db, linksTable, "node_protocol_id = ?", 5))
		require.EqualValues(t, 1, countTable(t, db, linksTable, "node_protocol_id = ?", 6))
		require.Zero(t, count(t, db, &model.ProtocolSecret{}, "scope = ? AND owner_id = ?", nodesecrets.ScopeNodeProtocol, 5))
		require.EqualValues(t, 1, count(t, db, &model.ProtocolSecret{}, "scope = ? AND owner_id = ?", nodesecrets.ScopeNodeProtocol, 6))
		requireNoSecret(t, db, operation, "reality-private-5", "peer-private-5")

		again, applied := submitBound(t, client, "protocol.retire:5", retireProtocolSpec(5), nil)
		require.False(t, applied)
		require.Equal(t, operation.GetOperationId(), again.GetOperationId())
	})
}

// A cascade completes or rolls back as one: when its last step fails,
// nothing of it is applied, and the operation fails retryable.
func TestRetirementCascadesRollBackAsOne(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		h := newCredentialHarness(t, db)
		seedCascade(t, db)
		client := h.client(proxyHost, allFamilies())
		failing := errors.New("the certificate table is unavailable")
		require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test:fail-certificates", func(d *gorm.DB) {
			if d.Statement.Schema != nil && d.Statement.Schema.Table == (model.AgentCertificate{}).TableName() {
				_ = d.AddError(failing)
			}
		}))
		t.Cleanup(func() { _ = db.Callback().Update().Remove("test:fail-certificates") })

		operation, _ := submitBound(t, client, "node.retire:proxy-1:fails", retireNodeSpec(proxyKind, 1), nil)
		require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_FAILED, operation.GetState())
		require.Equal(t, kernelnodeopsv1.ErrorCode_ERROR_CODE_INTERNAL, operation.GetError().GetCode())
		require.True(t, operation.GetError().GetRetryable())
		require.True(t, strings.Contains(operation.GetError().GetMessage(), "certificate table"), operation.GetError().GetMessage())
		require.EqualValues(t, 2, count(t, db, &model.NodeProtocol{}, "node_id = ?", 1), "the protocols are back")
		require.EqualValues(t, 2, count(t, db, &model.WireGuardPeer{}, "node_protocol_id IN ?", []uint{5, 6}))
		require.EqualValues(t, 2, countTable(t, db, linksTable, "node_protocol_id IN ?", []uint{5, 6}))
		require.EqualValues(t, 5, count(t, db, &model.ProtocolSecret{}, "1 = 1"))
		require.EqualValues(t, 1, count(t, db, &model.NodeCredential{}, "subject_kind = ? AND subject_id = ?", nodesecrets.SubjectProxy, 1))
		var certificate model.AgentCertificate
		require.NoError(t, db.First(&certificate, "serial = ?", "cert-proxy-1").Error)
		require.Nil(t, certificate.RevokedAt)

		require.NoError(t, db.Callback().Update().Remove("test:fail-certificates"))
		operation, _ = submitBound(t, client, "node.retire:proxy-1:again", retireNodeSpec(proxyKind, 1), nil)
		require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_SUCCEEDED, operation.GetState(), operation.GetError())
		require.Zero(t, count(t, db, &model.NodeProtocol{}, "node_id = ?", 1))
	})
}
