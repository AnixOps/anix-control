package grpc

import (
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	pb "github.com/AnixOps/anix-control/v4/api/grpc/v2boardpb"
	"github.com/AnixOps/anix-control/v4/internal/agentpki"
	"github.com/AnixOps/anix-control/v4/internal/agenttransport"
	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
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

// TestAgentListenerTransitionModes walks the four agent_control.mtls modes
// on the stream's API key authentication (A2-6, decision H5).
func TestAgentListenerTransitionModes(t *testing.T) {
	t.Run("off", func(t *testing.T) {
		l := startAgentListener(t, config.AgentMTLSOff, true)
		ctx := testContext(t)
		anonymous := l.dial(t, nil)
		served := agenttransport.LegacyRequests(controlStreamMethod)

		stream, err := agentv1pb.NewAgentControlServiceClient(anonymous).ControlStream(legacyCredentials(ctx, l.proxy.ID, l.proxyKey))
		require.NoError(t, err)
		require.NoError(t, stream.Send(validAgentHello(uint32(l.proxy.ID))))
		_, err = stream.Recv()
		require.NoError(t, err, "legacy credentials work")
		header, err := stream.Header()
		require.NoError(t, err)
		assert.Empty(t, header.Get(agentcontrol.MetadataAuthDeprecated), "off is silent")
		assert.Equal(t, served+1, agenttransport.LegacyRequests(controlStreamMethod), "counted in every mode")

		// Enrollment is off although the CA is there, and the listener asks
		// for no client certificate.
		_, _, err = enrollForTest(t, nodeCredentials(ctx, l.proxyNode(), l.proxyKey), anonymous, "")
		assert.Equal(t, codes.FailedPrecondition, status.Code(err))
		assert.Nil(t, l.server.agentAuthenticator().pki())
		row, ok := transportRow(t, agentcontrol.NodeKindProxy, l.proxy.ID, model.AgentTransportAPIKeyStream)
		require.True(t, ok)
		assert.Equal(t, "api-key", row.Identity)
	})

	t.Run("optional", func(t *testing.T) {
		l := startAgentListener(t, config.AgentMTLSOptional, true)
		ctx := testContext(t)
		anonymous := l.dial(t, nil)
		served := agenttransport.LegacyRequests(controlStreamMethod)
		stream, err := agentv1pb.NewAgentControlServiceClient(anonymous).ControlStream(legacyCredentials(ctx, l.proxy.ID, l.proxyKey))
		require.NoError(t, err)
		require.NoError(t, stream.Send(validAgentHello(uint32(l.proxy.ID))))
		_, err = stream.Recv()
		require.NoError(t, err)
		header, err := stream.Header()
		require.NoError(t, err)
		assert.Empty(t, header.Get(agentcontrol.MetadataAuthDeprecated), "optional is silent")
		assert.Equal(t, served+1, agenttransport.LegacyRequests(controlStreamMethod))

		// An enrolled agent is recorded on the mTLS stream with its
		// certificate.
		certificate, issued, err := enrollForTest(t, nodeCredentials(ctx, l.proxyNode(), l.proxyKey), anonymous, "")
		require.NoError(t, err)
		_, _, err = openStream(t, ctx, l.dial(t, certificate), uint32(l.proxy.ID))
		require.NoError(t, err)
		assert.Equal(t, served+1, agenttransport.LegacyRequests(controlStreamMethod), "a certificate is not legacy")
		row, ok := transportRow(t, agentcontrol.NodeKindProxy, l.proxy.ID, model.AgentTransportMTLSStream)
		require.True(t, ok)
		assert.Equal(t, issued.GetSerial(), row.CertSerial)
		assert.Equal(t, issued.GetSpiffeId(), row.Identity)
		assert.NotEmpty(t, row.AgentVersion)
		require.NotNil(t, row.CertNotAfter)
	})

	t.Run("preferred", func(t *testing.T) {
		l := startAgentListenerWith(t, config.AgentMTLSPreferred, true, func(cfg *ServerConfig) {
			cfg.AgentLegacySunset = time.Date(2027, 3, 31, 0, 0, 0, 0, time.UTC)
		})
		ctx := testContext(t)
		anonymous := l.dial(t, nil)
		served := agenttransport.LegacyRequests(controlStreamMethod)
		stream, err := agentv1pb.NewAgentControlServiceClient(anonymous).ControlStream(legacyCredentials(ctx, l.proxy.ID, l.proxyKey))
		require.NoError(t, err)
		require.NoError(t, stream.Send(validAgentHello(uint32(l.proxy.ID))))
		_, err = stream.Recv()
		require.NoError(t, err)
		header, err := stream.Header()
		require.NoError(t, err)
		assert.NotEmpty(t, header.Get(agentcontrol.MetadataAuthDeprecated))
		assert.Equal(t, []string{agenttransport.UpgradeGuideURL}, header.Get(agentcontrol.MetadataAuthDeprecationLink))
		assert.Equal(t, []string{"Wed, 31 Mar 2027 00:00:00 GMT"}, header.Get(agentcontrol.MetadataAuthSunset))
		assert.Equal(t, served+1, agenttransport.LegacyRequests(controlStreamMethod))
		require.NoError(t, stream.CloseSend())
		for {
			if _, err := stream.Recv(); err != nil {
				break
			}
		}
		assert.NotEmpty(t, stream.Trailer().Get(agentcontrol.MetadataAuthDeprecated), "the trailer repeats it")
		_, ok := transportRow(t, agentcontrol.NodeKindProxy, l.proxy.ID, model.AgentTransportAPIKeyStream)
		assert.True(t, ok)
	})

	t.Run("the 4.2 default without a CA", func(t *testing.T) {
		// agent_control.mtls left empty is required. Without the built-in
		// CA the kernel still starts (config validation lets the default
		// through, with a startup warning): legacy credentials are refused,
		// and enrollment is unavailable rather than failing open.
		l := startAgentListener(t, "", false)
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
		_, _, err = enrollForTest(t, nodeCredentials(ctx, l.proxyNode(), l.proxyKey), anonymous, "")
		assert.Equal(t, codes.FailedPrecondition, status.Code(err), "no enrollment without the CA: %v", err)
	})

	t.Run("required", func(t *testing.T) {
		l := startAgentListener(t, config.AgentMTLSRequired, true)
		ctx := testContext(t)
		anonymous := l.dial(t, nil)
		served, refused := agenttransport.LegacyRequests(controlStreamMethod), agenttransport.RefusedRequests(controlStreamMethod)
		stream, err := agentv1pb.NewAgentControlServiceClient(anonymous).ControlStream(legacyCredentials(ctx, l.proxy.ID, l.proxyKey))
		require.NoError(t, err)
		_ = stream.Send(validAgentHello(uint32(l.proxy.ID)))
		_, err = stream.Recv()
		require.Equal(t, codes.Unauthenticated, status.Code(err))
		assert.Contains(t, status.Convert(err).Message(), agentcontrol.ErrorCodeMTLSRequired)
		assert.Equal(t, []string{agentcontrol.ErrorCodeMTLSRequired}, stream.Trailer().Get(agentcontrol.MetadataErrorCode))
		assert.NotEmpty(t, stream.Trailer().Get(agentcontrol.MetadataAuthDeprecationLink))
		assert.Equal(t, served, agenttransport.LegacyRequests(controlStreamMethod))
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

// TestAgentAuthenticatorDefaults: a nil authenticator is optional, an
// empty mode is the configuration default (required from 4.2), and a stream
// authenticated by certificate is recorded as mtls-stream.
func TestAgentAuthenticatorDefaults(t *testing.T) {
	var none *AgentAuthenticator
	assert.Equal(t, config.AgentMTLSOptional, none.mode())
	assert.Nil(t, none.deprecationHeader())
	assert.Equal(t, config.AgentMTLSRequired, (&AgentAuthenticator{}).mode())
	assert.Equal(t, config.AgentMTLSPreferred, (&AgentAuthenticator{Mode: config.AgentMTLSPreferred}).mode(), "preferred stays selectable")
	assert.Nil(t, (&AgentAuthenticator{}).deprecationHeader(), "required refuses legacy credentials rather than signalling them")
	preferred := &AgentAuthenticator{Mode: config.AgentMTLSPreferred}
	assert.NotNil(t, preferred.deprecationHeader())
	assert.Empty(t, preferred.deprecationHeader().Get(agentcontrol.MetadataAuthSunset), "no sunset unless configured")

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
