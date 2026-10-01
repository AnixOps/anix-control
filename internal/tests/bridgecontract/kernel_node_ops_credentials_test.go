package bridgecontract

import (
	"context"
	"testing"
	"time"

	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/v4/internal/kernelnodeops"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/nodesecrets"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

// seedRetirement gives node 2 a protocol with a secret, a subscription
// group link and a WireGuard peer, dual-written, and a registration key.
func seedRetirement(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.AutoMigrate(&model.NodeProtocol{}, &model.SubscriptionGroup{}, &model.WireGuardPeer{}, &model.AuthorizedKey{},
		&model.AgentEnrollment{}, &model.AgentCertificate{}, &model.NodeCredential{}, &model.ProtocolSecret{}, &model.NodeSecretSplit{}))
	require.NoError(t, nodesecrets.EnsureSchema(db))
	reality := `{"dest":"www.example.test:443","private_key":"reality-private-7"}`
	require.NoError(t, db.Create(&model.NodeProtocol{ID: 7, NodeID: 2, Name: "reality", Type: model.ProtocolVLESS, Port: 443, RealitySettings: &reality}).Error)
	group := &model.SubscriptionGroup{Name: "group", Enable: 1}
	require.NoError(t, db.Create(group).Error)
	require.NoError(t, db.Exec("INSERT INTO v2_subscription_group_node_protocols (subscription_group_id, node_protocol_id) VALUES (?, ?)", group.ID, 7).Error)
	require.NoError(t, db.Create(&model.WireGuardPeer{NodeProtocolID: 7, UserID: 1, PeerIP: "10.9.0.2", PrivateKey: "peer-private-7", PublicKey: "pub"}).Error)
	require.NoError(t, db.Create(&model.AuthorizedKey{ID: 60, Name: "bootstrap", Key: "registration-key-60", KeyHash: "h"}).Error)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		if err := nodesecrets.Sync(tx, nodesecrets.TableNodeProtocol, 7); err != nil {
			return err
		}
		if err := nodesecrets.Sync(tx, nodesecrets.TableWireGuardPeer, 1); err != nil {
			return err
		}
		return nodesecrets.Sync(tx, nodesecrets.TableAuthorizedKey, 60)
	}))
}

// A package reaches the NO-5 kinds through its bridge: GetCapabilities
// lists them, ValidateNodeConfig answers the validators, a RetireProtocol
// submitted with wait TERMINAL answers its typed counts, and a
// RevokeRegistrationKey removes the key, on SQLite and PostgreSQL.
func TestKernelNodeOpsCredentialRoundTrip(t *testing.T) {
	nodeOpsDatabases(t, func(t *testing.T, db *gorm.DB) {
		seedRetirement(t, db)
		registry := kernelnodeops.NewRegistry()
		require.NoError(t, (&kernelnodeops.Credentials{}).Register(registry))
		require.NoError(t, (&kernelnodeops.Retirements{}).Register(registry))
		require.NoError(t, (&kernelnodeops.SecretDocuments{}).Register(registry))
		engine := &kernelnodeops.Engine{DB: db, Executors: registry, PollInterval: 20 * time.Millisecond}
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() {
			engine.Run(ctx)
			close(done)
		}()
		t.Cleanup(func() {
			cancel()
			<-done
		})
		server := &kernelnodeops.Server{Engine: engine, Authorizer: nodeOpsGrants{"plan": {service.CapabilityNodeOpsNodeConfig, service.CapabilityNodeOpsCredentials}}}
		client := kernelnodeopsv1.NewKernelNodeOpsClient(dialPlanSession(t, packagebridge.SessionOptions{KernelNodeOps: server.For}).Conn())
		background := context.Background()

		capabilities, err := client.GetCapabilities(background, &kernelnodeopsv1.GetCapabilitiesRequest{})
		require.NoError(t, err)
		require.Equal(t, []string{
			kernelnodeops.KindCleanAgentIssue, kernelnodeops.KindCredentialIssue, kernelnodeops.KindCredentialRevoke, kernelnodeops.KindNodeRetire,
			kernelnodeops.KindProtocolRetire, kernelnodeops.KindRegKeyIssue, kernelnodeops.KindRegKeyRevoke, kernelnodeops.KindSecretsPut,
		}, capabilities.GetKinds())

		validated, err := client.ValidateNodeConfig(background, &kernelnodeopsv1.ValidateNodeConfigRequest{
			Kind: kernelnodeopsv1.NodeConfigKind_NODE_CONFIG_KIND_RAW_CONFIG, DocumentJson: []byte(`{"type":"vless"}`),
		})
		require.NoError(t, err)
		require.True(t, validated.GetValid())
		require.Len(t, validated.GetIssues(), 1, "the server_port warning")

		retired, err := client.SubmitOperation(background, &kernelnodeopsv1.SubmitOperationRequest{
			RequestId: "protocol.retire:7", Wait: kernelnodeopsv1.WaitMode_WAIT_MODE_TERMINAL, WaitTimeoutMs: 20000,
			Operation: &kernelnodeopsv1.OperationSpec{Operation: &kernelnodeopsv1.OperationSpec_RetireProtocol{RetireProtocol: &kernelnodeopsv1.RetireProtocol{ProtocolId: 7}}},
		})
		require.NoError(t, err)
		require.True(t, retired.GetApplied())
		operation := retired.GetOperation()
		require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_SUCCEEDED, operation.GetState(), operation.GetError())
		require.Equal(t, kernelnodeopsv1.Channel_CHANNEL_KERNEL, operation.GetChannel())
		require.Equal(t, kernelnodeops.KindProtocolRetire, operation.GetKind())
		result := operation.GetResult().GetRetire()
		require.EqualValues(t, 1, result.GetWireguardPeersDeleted())
		require.EqualValues(t, 1, result.GetSubscriptionLinksDeleted())
		require.EqualValues(t, 1, result.GetSecretsDeleted())
		var peers, links, secrets, protocols int64
		require.NoError(t, db.Model(&model.WireGuardPeer{}).Where("node_protocol_id = ?", 7).Count(&peers).Error)
		require.NoError(t, db.Table("v2_subscription_group_node_protocols").Where("node_protocol_id = ?", 7).Count(&links).Error)
		require.NoError(t, db.Model(&model.ProtocolSecret{}).Where("owner_id = ?", 7).Count(&secrets).Error)
		require.NoError(t, db.Model(&model.NodeProtocol{}).Where("id = ?", 7).Count(&protocols).Error)
		require.Zero(t, peers+links+secrets)
		require.EqualValues(t, 1, protocols, "the row is the package's to delete")

		read, err := client.GetOperation(background, &kernelnodeopsv1.GetOperationRequest{Selector: &kernelnodeopsv1.GetOperationRequest_RequestId{RequestId: "protocol.retire:7"}})
		require.NoError(t, err)
		require.Equal(t, operation.GetOperationId(), read.GetOperation().GetOperationId())

		revoked, err := client.SubmitOperation(background, &kernelnodeopsv1.SubmitOperationRequest{
			RequestId: "regkey.revoke:60", Wait: kernelnodeopsv1.WaitMode_WAIT_MODE_TERMINAL, WaitTimeoutMs: 20000,
			Operation: &kernelnodeopsv1.OperationSpec{Operation: &kernelnodeopsv1.OperationSpec_RevokeRegistrationKey{RevokeRegistrationKey: &kernelnodeopsv1.RevokeRegistrationKey{KeyId: 60}}},
		})
		require.NoError(t, err)
		require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_SUCCEEDED, revoked.GetOperation().GetState(), revoked.GetOperation().GetError())
		require.Equal(t, "bootstrap", revoked.GetOperation().GetResult().GetRegistrationKey().GetName())
		var keys int64
		require.NoError(t, db.Model(&model.AuthorizedKey{}).Where("id = ?", 60).Count(&keys).Error)
		require.Zero(t, keys)

		// Issuing needs the administrator's request: a package without a
		// binding is refused and nothing is recorded.
		_, err = client.SubmitOperation(background, &kernelnodeopsv1.SubmitOperationRequest{
			RequestId: "regkey.issue:unbound",
			Operation: &kernelnodeopsv1.OperationSpec{Operation: &kernelnodeopsv1.OperationSpec_IssueRegistrationKey{IssueRegistrationKey: &kernelnodeopsv1.IssueRegistrationKey{Name: "x"}}},
		})
		require.Equal(t, codes.PermissionDenied, status.Code(err))
		list, err := client.ListOperations(background, &kernelnodeopsv1.ListOperationsRequest{})
		require.NoError(t, err)
		require.Len(t, list.GetOperations(), 2)
	})
}
