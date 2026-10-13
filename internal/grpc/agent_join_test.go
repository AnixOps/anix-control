package grpc

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	"github.com/AnixOps/anix-control/v5/internal/agentpki"
	"github.com/AnixOps/anix-control/v5/internal/agentstreams"
	"github.com/AnixOps/anix-control/v5/internal/config"
	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/kernelnodeops"
	"github.com/AnixOps/anix-control/v5/internal/middleware"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/nodegroup"
	"github.com/AnixOps/anix-control/v5/internal/nodesecrets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// joinListener is the agent listener in agent_control.mtls: required (the
// only mode of 5.0) with the node group tables and one proxy group.
type joinListener struct {
	*agentListener
	groups *nodegroup.Service
	group  nodegroup.GroupView
	token  string
	joined chan nodegroup.Joined
}

func startJoinListener(t *testing.T) *joinListener {
	t.Helper()
	joined := make(chan nodegroup.Joined, 8)
	l := startAgentListenerWith(t, config.AgentMTLSRequired, true, func(cfg *ServerConfig) {
		cfg.OnJoin = func(j nodegroup.Joined) { joined <- j }
	})
	requireAutoMigrate(t, append(model.KernelModels(), &model.SubscriptionGroup{}, &model.NodeProtocol{})...)
	require.NoError(t, nodesecrets.MigrateForTest(database.Get()))
	kek := make([]byte, 32)
	_, err := rand.Read(kek)
	require.NoError(t, err)
	groups := &nodegroup.Service{DB: database.Get(), KEK: func() string { return base64.StdEncoding.EncodeToString(kek) }, PKI: l.pki}
	group, err := groups.Create(t.Context(), nodegroup.GroupInput{
		Name: "HK", Kind: "proxy", ProtocolTemplate: json.RawMessage(`[{"type":"vmess","port":8080}]`),
	}, nodegroup.Actor{Name: "test"})
	require.NoError(t, err)
	revealed, err := groups.RevealToken(t.Context(), group.ID, nodegroup.Actor{Name: "test"})
	require.NoError(t, err)
	return &joinListener{agentListener: l, groups: groups, group: group, token: revealed.Token, joined: joined}
}

// joinForTest calls Join on conn with a new key and returns the Agent's TLS
// certificate, the answer and the trailer.
func joinForTest(t *testing.T, ctx context.Context, conn *grpc.ClientConn, request *agentv1pb.JoinRequest) (*tls.Certificate, *agentv1pb.JoinResponse, metadata.MD, error) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: "anything"}}, key)
	require.NoError(t, err)
	request.CsrDer = csr
	var trailer metadata.MD
	response, err := agentv1pb.NewAgentEnrollmentClient(conn).Join(ctx, request, grpc.Trailer(&trailer))
	if err != nil {
		return nil, nil, trailer, err
	}
	return &tls.Certificate{Certificate: [][]byte{response.GetCertificate().GetCertificateDer()}, PrivateKey: key}, response, trailer, nil
}

func TestAgentJoinOverTheListener(t *testing.T) {
	l := startJoinListener(t)
	previous := setJoinLimiter(middleware.NewRateLimiter(100, 100))
	t.Cleanup(func() { setJoinLimiter(previous) })
	anonymous := l.dial(t, nil)
	ctx := testContext(t)

	certificate, response, _, err := joinForTest(t, ctx, anonymous, &agentv1pb.JoinRequest{
		JoinToken: l.token, MachineFingerprint: "", Hostname: "hk-01", PublicAddresses: []string{"203.0.113.5"},
		Arch: "amd64", Os: "linux", AgentVersion: "v5.0.0", InstanceId: "instance",
	})
	require.NoError(t, err)
	node, err := agentcontrol.ParseAgentNode(response.GetNode())
	require.NoError(t, err)
	assert.Equal(t, agentcontrol.NodeKindProxy, node.Kind)
	assert.Equal(t, "hk-01", response.GetNodeName())
	assert.Equal(t, "HK", response.GetGroup())
	assert.True(t, response.GetCreated())
	assert.Equal(t, model.NodeApprovalApproved, response.GetApproval())
	assert.Equal(t, "spiffe://anixops/test/agent/"+response.GetNode(), response.GetCertificate().GetSpiffeId())
	assert.Equal(t, response.GetNode(), response.GetCertificate().GetNode())

	// The certificate opens the node's stream under mTLS required.
	_, helloAck, err := openStream(t, ctx, l.dial(t, certificate), node.ID)
	require.NoError(t, err)
	require.NotNil(t, helloAck)

	// The administrators hear of it after the commit.
	select {
	case joined := <-l.joined:
		assert.Equal(t, node, joined.Node)
	case <-time.After(5 * time.Second):
		t.Fatal("the join notifier was not called")
	}

	// An unusable token: one answer, PermissionDenied with join_rejected
	// and no detail.
	for name, token := range map[string]string{"unknown": "anixjoin_" + l.token[9:20] + "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx", "garbage": "nope", "empty": ""} {
		_, _, trailer, err := joinForTest(t, ctx, anonymous, &agentv1pb.JoinRequest{JoinToken: token, Hostname: "x", PublicAddresses: []string{"203.0.113.6"}})
		assert.Equal(t, codes.PermissionDenied, status.Code(err), name)
		assert.Equal(t, []string{agentcontrol.ErrorCodeJoinRejected}, trailer.Get(agentcontrol.MetadataErrorCode), name)
		assert.Equal(t, "join_rejected: the join token is not usable", status.Convert(err).Message(), name)
	}
	var refusals int64
	require.NoError(t, database.Get().Model(&model.OperationLog{}).Where("action = ?", nodegroup.AuditActionJoinRejected).Count(&refusals).Error)
	assert.Equal(t, int64(3), refusals, "every refusal is audited")

	// A malformed request is InvalidArgument.
	_, _, _, err = joinForTest(t, ctx, anonymous, &agentv1pb.JoinRequest{JoinToken: l.token, PublicAddresses: []string{"not-an-ip"}})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))

	// The join is rate limited per peer.
	setJoinLimiter(middleware.NewRateLimiter(0.001, 1))
	_, _, _, err = joinForTest(t, ctx, anonymous, &agentv1pb.JoinRequest{JoinToken: "nope"})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
	_, _, _, err = joinForTest(t, ctx, anonymous, &agentv1pb.JoinRequest{JoinToken: l.token, PublicAddresses: []string{"203.0.113.7"}})
	assert.Equal(t, codes.ResourceExhausted, status.Code(err))
}

// An anixagt_ enrollment credential enrolls its node without any node
// metadata, under agent_control.mtls: required: the installer's join
// command uses it so for an existing node.
func TestAgentEnrollCredentialWithoutNodeMetadataUnderRequiredMTLS(t *testing.T) {
	l := startJoinListener(t)
	ctx := testContext(t)
	credential, _, err := l.pki.CreateEnrollmentToken(ctx, agentpki.TokenRequest{Node: l.forwardNode(), TTL: time.Hour})
	require.NoError(t, err)
	certificate, issued, err := enrollForTest(t, ctx, l.dial(t, nil), credential)
	require.NoError(t, err)
	assert.Equal(t, l.forwardNode().String(), issued.GetNode(), "the credential names its node")
	_, helloAck, err := openStream(t, ctx, l.dial(t, certificate), uint32(l.forward.ID))
	require.NoError(t, err)
	require.NotNil(t, helloAck)
}

// A node pending approval connects, but gets neither its configuration nor
// its users until an administrator approves it; then both flow at once.
func TestAgentControlWithholdsConfigAndUsersFromAPendingNode(t *testing.T) {
	environment := newAgentUsersEnvironment(t, func(t *testing.T) { requireInMemoryDatabase(t) })
	requireAutoMigrate(t, append(model.KernelNodeOperationModels(), &model.OperationLog{}, &model.ForwardNode{})...)
	previousPoll := approvalPoll
	approvalPoll = 20 * time.Millisecond
	t.Cleanup(func() { approvalPoll = previousPoll })
	environment.seedUsers(t, 3)
	db := database.Get()
	require.NoError(t, db.Model(&model.Node{}).Where("id = ?", environment.node.ID).Update("approval", model.NodeApprovalPending).Error)
	node := agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: environment.nodeID()}

	capabilities := append(usersCapabilities(), &agentv1pb.Capability{Name: agentcontrol.CapabilityConfig, Version: agentcontrol.CapabilityVersionV1})
	stream, helloAck, _ := environment.openUsersSession(t, capabilities, 0)
	require.True(t, agentcontrol.Negotiated(capabilities, helloAck.ServerCapabilities, agentcontrol.CapabilityConfig))
	require.True(t, agentcontrol.Negotiated(capabilities, helloAck.ServerCapabilities, agentcontrol.CapabilityUsers))
	requireHeartbeatAckNext(t, stream, environment.nodeID(), helloAck.SessionId)

	// The configuration is built and stored, but a push sends nothing: a
	// push on a stream that cannot send does not even fail.
	row, _, err := kernelnodeops.RefreshDesiredConfig(context.Background(), db, node, time.Now())
	require.NoError(t, err)
	broken := &AgentControlConnection{NodeID: node.ID, stream: failingControlStream{}, configNegotiated: true}
	assert.NoError(t, pushDesiredConfig(context.Background(), broken, node, configTriggerRefresh))
	requireHeartbeatAckNext(t, stream, environment.nodeID(), helloAck.SessionId)

	before := snapshotsSent(configTriggerApproval)
	decision, err := nodegroup.New(db).Approve(context.Background(), node, nodegroup.Actor{Name: "admin"})
	require.NoError(t, err)
	assert.Equal(t, model.NodeApprovalApproved, decision.Approval)

	// The configuration comes at once, the users within a poll.
	sawConfig, users := false, []uint64{}
	deadline := time.Now().Add(5 * time.Second)
	for (!sawConfig || len(users) < 3) && time.Now().Before(deadline) {
		message, err := stream.Recv()
		require.NoError(t, err)
		switch {
		case message.GetConfig() != nil:
			sawConfig = true
			assert.Equal(t, row.Revision, message.GetConfig().GetConfigRevision())
		case message.GetUsers() != nil:
			for _, user := range message.GetUsers().GetUpserts() {
				users = append(users, user.GetUserId())
			}
		}
	}
	assert.True(t, sawConfig, "the configuration was not sent after the approval")
	assert.ElementsMatch(t, []uint64{1, 2, 3}, users)
	assert.Equal(t, before+1, snapshotsSent(configTriggerApproval))
	require.NoError(t, stream.CloseSend())
}

// A node sync of a pending node is refused with ErrPendingApproval, which
// the node operation retries.
func TestAgentStreamsPushConfigRefusesAPendingNode(t *testing.T) {
	env := newConfigTestEnvironment(t)
	row := env.desiredConfig(t)
	session := openConfigSession(t, env, row.Revision)
	session.heartbeat()
	_ = session.untilHeartbeatAck()
	require.NoError(t, database.Get().Model(&model.Node{}).Where("id = ?", env.node.ID).Update("approval", model.NodeApprovalPending).Error)
	streams := NewAgentStreams(env.manager, NewAgentControlManager())
	_, sent, err := streams.PushConfig(context.Background(), env.agentNode(), kernelnodeops.ConfigSnapshotOf(row))
	assert.ErrorIs(t, err, agentstreams.ErrPendingApproval)
	assert.False(t, sent)
	require.NoError(t, session.stream.CloseSend())
}
