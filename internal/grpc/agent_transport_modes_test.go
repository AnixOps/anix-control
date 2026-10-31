package grpc

import (
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	pb "github.com/AnixOps/anix-control/v5/api/grpc/v2boardpb"
	"github.com/AnixOps/anix-control/v5/internal/agentpki"
	"github.com/AnixOps/anix-control/v5/internal/agenttransport"
	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const controlStreamMethod = agentv1pb.AgentControlService_ControlStream_FullMethodName

// transportRow returns the inventory row of a node on a transport.
func transportRow(t *testing.T, kind string, id uint, transport string) (model.AgentTransport, bool) {
	t.Helper()
	var rows []model.AgentTransport
	require.NoError(t, database.Get().Where("node_kind = ? AND node_id = ? AND transport = ?", kind, id, transport).Find(&rows).Error)
	if len(rows) == 0 {
		return model.AgentTransport{}, false
	}
	return rows[0], true
}

// TestAgentListenerRefusesUncertifiedStreams: a stream without a client
// certificate is refused whatever credentials it carries.
func TestAgentListenerRefusesUncertifiedStreams(t *testing.T) {
	t.Run("the 4.2 default without a CA", func(t *testing.T) {
		// Without the built-in CA the kernel still starts (with a startup
		// warning): API-key credentials are refused, and enrollment is
		// unavailable rather than failing open.
		l := startAgentListener(t, false)
		ctx := testContext(t)
		anonymous := l.dial(t, nil)
		assert.Nil(t, l.server.agentAuthenticator().pki())
		refused := agenttransport.RefusedRequests(controlStreamMethod)
		stream, err := agentv1pb.NewAgentControlServiceClient(anonymous).ControlStream(legacyCredentials(ctx, l.proxy.ID, l.proxyKey))
		require.NoError(t, err)
		_ = stream.Send(validAgentHello(uint32(l.proxy.ID)))
		_, err = stream.Recv()
		require.Equal(t, codes.Unauthenticated, status.Code(err))
		assert.Equal(t, []string{agentcontrol.ErrorCodeMTLSRequired}, stream.Trailer().Get(agentcontrol.MetadataErrorCode))
		assert.Equal(t, refused+1, agenttransport.RefusedRequests(controlStreamMethod))
		_, _, err = enrollForTest(t, ctx, anonymous, "anixagt_unused")
		assert.Equal(t, codes.FailedPrecondition, status.Code(err), "no enrollment without the CA: %v", err)
	})

	t.Run("required", func(t *testing.T) {
		l := startAgentListener(t, true)
		ctx := testContext(t)
		anonymous := l.dial(t, nil)
		refused := agenttransport.RefusedRequests(controlStreamMethod)
		stream, err := agentv1pb.NewAgentControlServiceClient(anonymous).ControlStream(legacyCredentials(ctx, l.proxy.ID, l.proxyKey))
		require.NoError(t, err)
		_ = stream.Send(validAgentHello(uint32(l.proxy.ID)))
		_, err = stream.Recv()
		require.Equal(t, codes.Unauthenticated, status.Code(err))
		assert.Contains(t, status.Convert(err).Message(), agentcontrol.ErrorCodeMTLSRequired)
		assert.Equal(t, []string{agentcontrol.ErrorCodeMTLSRequired}, stream.Trailer().Get(agentcontrol.MetadataErrorCode))
		assert.Equal(t, refused+1, agenttransport.RefusedRequests(controlStreamMethod))
		_, ok := transportRow(t, agentcontrol.NodeKindProxy, l.proxy.ID, model.AgentTransportAPIKeyStream)
		assert.False(t, ok, "a refused stream is not a sighting")

		// Enrolling with the API key is refused with the same code.
		var trailer metadata.MD
		enrollRefused := agenttransport.RefusedRequests(agentv1pb.AgentEnrollment_Enroll_FullMethodName)
		_, err = agentv1pb.NewAgentEnrollmentClient(anonymous).Enroll(nodeCredentials(ctx, l.proxyNode(), l.proxyKey),
			&agentv1pb.EnrollAgentRequest{CsrDer: []byte("csr")}, grpc.Trailer(&trailer))
		assert.Equal(t, codes.Unauthenticated, status.Code(err))
		assert.Equal(t, []string{agentcontrol.ErrorCodeMTLSRequired}, trailer.Get(agentcontrol.MetadataErrorCode))
		assert.Equal(t, enrollRefused+1, agenttransport.RefusedRequests(agentv1pb.AgentEnrollment_Enroll_FullMethodName))

		// UniProxy's gRPC sibling, the v2board services, stay open to the
		// legacy credential (third-party node software) and are recorded.
		_, err = pb.NewNodeServiceClient(anonymous).GetConfig(legacyCredentials(ctx, l.proxy.ID, l.proxyKey), &pb.NodeConfigRequest{NodeId: uint32(l.proxy.ID)})
		assert.NotContains(t, []codes.Code{codes.Unauthenticated, codes.PermissionDenied}, status.Code(err))
		row, ok := transportRow(t, agentcontrol.NodeKindProxy, l.proxy.ID, model.AgentTransportV2boardGRPC)
		require.True(t, ok)
		assert.Equal(t, "api-key", row.Identity)

		// A certificate opens the stream; its v2board calls are recorded
		// with the certificate's identity.
		credential, _, err := l.pki.CreateEnrollmentToken(ctx, agentpki.TokenRequest{Node: l.proxyNode(), TTL: time.Hour})
		require.NoError(t, err)
		certificate, issued, err := enrollForTest(t, ctx, anonymous, credential)
		require.NoError(t, err)
		conn := l.dial(t, certificate)
		_, helloAck, err := openStream(t, ctx, conn, uint32(l.proxy.ID))
		require.NoError(t, err)
		assert.NotEmpty(t, helloAck.SessionId)
		_, ok = transportRow(t, agentcontrol.NodeKindProxy, l.proxy.ID, model.AgentTransportMTLSStream)
		assert.True(t, ok)
		_, err = pb.NewNodeServiceClient(conn).GetConfig(ctx, &pb.NodeConfigRequest{NodeId: uint32(l.proxy.ID)})
		assert.NotContains(t, []codes.Code{codes.Unauthenticated, codes.PermissionDenied}, status.Code(err))
		row, _ = transportRow(t, agentcontrol.NodeKindProxy, l.proxy.ID, model.AgentTransportV2boardGRPC)
		assert.Equal(t, issued.GetSpiffeId(), row.Identity, "a new identity is written at once, past the throttle")
	})
}

// TestAgentAuthenticatorDefaults: a stream authenticated by certificate is
// recorded as mtls-stream.
func TestAgentAuthenticatorDefaults(t *testing.T) {
	certificate := agentPrincipal{Node: agentcontrol.AgentNode{Kind: agentcontrol.NodeKindForward, ID: 3}, Certificate: true,
		Serial: "01", NotAfter: time.Now().Add(time.Hour), SPIFFEID: "spiffe://anixops/test/agent/forward-3"}
	sighting := certificate.sighting("2.0.0")
	assert.Equal(t, model.AgentTransportMTLSStream, sighting.Transport)
	assert.Equal(t, "01", sighting.CertSerial)
	assert.NotNil(t, sighting.CertNotAfter)
	legacy := agentPrincipal{Node: agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: 3}}.sighting("1.0.0")
	assert.Equal(t, model.AgentTransportAPIKeyStream, legacy.Transport)
	assert.Equal(t, "api-key", legacy.Identity)
	assert.Nil(t, legacy.CertNotAfter)
}
