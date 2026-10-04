package grpc

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net/url"
	"testing"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	"github.com/AnixOps/anix-control/v4/internal/agentpki"
	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/kernelforward"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/modulepki"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// linkListener is the agent listener with the agent PKI and the forward link
// CA.
type linkListener struct {
	*agentListener
	link *agentpki.LinkAuthority
}

func startLinkListener(t *testing.T, mode string, withLink bool) *linkListener {
	t.Helper()
	out := &linkListener{}
	var authority *modulepki.Authority
	var pki *agentpki.Service
	l := startAgentListenerWith(t, mode, false, func(cfg *ServerConfig) {
		requireAutoMigrate(t, append(model.KernelForwardModels(), &model.ForwardLinkCA{}, &model.ForwardLinkCertificate{})...)
		db := database.Get()
		kek := make([]byte, 32)
		_, err := rand.Read(kek)
		require.NoError(t, err)
		authority, err = modulepki.New(modulepki.Options{DB: db, Cluster: "test", KEK: kek})
		require.NoError(t, err)
		require.NoError(t, authority.Ensure(t.Context()))
		options := agentpki.Options{DB: db, Authority: authority}
		if withLink {
			out.link, err = agentpki.NewLinkAuthority(agentpki.LinkAuthorityOptions{DB: db, Cluster: "test", KEK: kek})
			require.NoError(t, err)
			require.NoError(t, out.link.Ensure(t.Context()))
			options.Link = out.link
		}
		pki, err = agentpki.New(options)
		require.NoError(t, err)
		cfg.AgentPKI = pki
	})
	l.authority, l.pki = authority, pki
	out.agentListener = l
	return out
}

// negotiateForward records a forward.v1 Hello of node, as the control
// stream does (kernelforward.RecordHello).
func negotiateForward(t *testing.T, node agentcontrol.AgentNode, negotiated bool) {
	t.Helper()
	caps := forwardNodeCapabilities()
	if !negotiated {
		caps = nil
	}
	_, _, err := kernelforward.New(database.Get()).RecordHello(t.Context(), node, caps, "4.2.0")
	require.NoError(t, err)
}

// enrolledConn enrolls node with its credential and dials with the
// certificate; it returns the connection and the Agent's key.
func (l *linkListener) enrolledConn(t *testing.T, ctx context.Context, node agentcontrol.AgentNode) (*grpc.ClientConn, *tls.Certificate) {
	t.Helper()
	secret := l.proxyKey
	if node.Kind == agentcontrol.NodeKindForward {
		secret = l.forwardToken
	}
	certificate, _, err := enrollForTest(t, nodeCredentials(ctx, node, secret), l.dial(t, nil), "")
	require.NoError(t, err)
	return l.dial(t, certificate), certificate
}

func linkRequest(t *testing.T, template *x509.CertificateRequest) *agentv1pb.IssueLinkCertificateRequest {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	if template == nil {
		template = &x509.CertificateRequest{}
	}
	csr, err := x509.CreateCertificateRequest(rand.Reader, template, key)
	require.NoError(t, err)
	return &agentv1pb.IssueLinkCertificateRequest{CsrDer: csr}
}

// requireRefusal checks a refused call's status and its error code trailer.
func requireRefusal(t *testing.T, err error, trailer metadata.MD, want codes.Code, code string) {
	t.Helper()
	require.Error(t, err)
	assert.Equal(t, want, status.Code(err), err)
	assert.Equal(t, []string{code}, trailer.Get(agentcontrol.MetadataErrorCode), err)
}

func TestAgentLinkCertificateIssuance(t *testing.T) {
	l := startLinkListener(t, config.AgentMTLSOptional, true)
	ctx := testContext(t)
	node := l.forwardNode()
	conn, agentCertificate := l.enrolledConn(t, ctx, node)
	client := agentv1pb.NewAgentEnrollmentClient(conn)
	negotiateForward(t, node, true)

	identity := "spiffe://anixops/test/agent/" + node.String()
	uri, err := url.Parse(identity)
	require.NoError(t, err)
	response, err := client.IssueLinkCertificate(ctx, linkRequest(t, &x509.CertificateRequest{DNSNames: []string{node.String()}, URIs: []*url.URL{uri}}))
	require.NoError(t, err)
	issued := response.GetCertificate()
	assert.Equal(t, identity, issued.GetSpiffeId())
	assert.Equal(t, node.String(), issued.GetNode())
	assert.Equal(t, node.String(), issued.GetDnsName())
	assert.NotEmpty(t, issued.GetSerial())
	assert.Equal(t, int64((agentpki.DefaultLinkCertificateLifetime / 3).Seconds()), issued.GetNotAfterUnix()-issued.GetRenewAfterUnix())
	require.Len(t, issued.GetTrustBundleDer(), 1)

	leaf, err := x509.ParseCertificate(issued.GetCertificateDer())
	require.NoError(t, err)
	assert.Equal(t, []string{node.String()}, leaf.DNSNames)
	roots := x509.NewCertPool()
	for _, der := range issued.GetTrustBundleDer() {
		root, err := x509.ParseCertificate(der)
		require.NoError(t, err)
		roots.AddCert(root)
	}
	_, err = leaf.Verify(x509.VerifyOptions{Roots: roots, DNSName: node.String(), KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}})
	require.NoError(t, err)

	// The bundle RPC answers the same CAs, and the next one after a rotation.
	bundle, err := client.GetLinkTrustBundle(ctx, &agentv1pb.GetLinkTrustBundleRequest{})
	require.NoError(t, err)
	assert.Equal(t, issued.GetTrustBundleDer(), bundle.GetTrustBundleDer())
	_, err = l.link.Rotate(ctx)
	require.NoError(t, err)
	bundle, err = client.GetLinkTrustBundle(ctx, &agentv1pb.GetLinkTrustBundleRequest{})
	require.NoError(t, err)
	assert.Len(t, bundle.GetTrustBundleDer(), 2, "current and next")

	// Renewal: the same call with a new key, a new serial.
	renewed, err := client.IssueLinkCertificate(ctx, linkRequest(t, nil))
	require.NoError(t, err)
	assert.NotEqual(t, issued.GetSerial(), renewed.GetCertificate().GetSerial())
	assert.Len(t, renewed.GetCertificate().GetTrustBundleDer(), 2)

	// The Agent's own key is refused.
	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{}, agentCertificate.PrivateKey)
	require.NoError(t, err)
	var trailer metadata.MD
	_, err = client.IssueLinkCertificate(ctx, &agentv1pb.IssueLinkCertificateRequest{CsrDer: csr}, grpc.Trailer(&trailer))
	requireRefusal(t, err, trailer, codes.InvalidArgument, agentcontrol.ErrorCodeLinkRequestInvalid)

	// A link certificate never authenticates on the agent listener
	// (tls.RequestClientCert, verified by the handler): its CA is not the
	// agent CA.
	linkKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	linkCSR, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{}, linkKey)
	require.NoError(t, err)
	linkIssued, err := client.IssueLinkCertificate(ctx, &agentv1pb.IssueLinkCertificateRequest{CsrDer: linkCSR})
	require.NoError(t, err)
	linkConn := l.dial(t, &tls.Certificate{Certificate: [][]byte{linkIssued.GetCertificate().GetCertificateDer()}, PrivateKey: linkKey})
	trailer = nil
	_, err = agentv1pb.NewAgentEnrollmentClient(linkConn).IssueLinkCertificate(ctx, linkRequest(t, nil), grpc.Trailer(&trailer))
	requireRefusal(t, err, trailer, codes.Unauthenticated, agentcontrol.ErrorCodeCertInvalid)

	var records int64
	require.NoError(t, database.Get().Model(&model.ForwardLinkCertificate{}).Where("node_kind = ? AND node_id = ?", node.Kind, node.ID).Count(&records).Error)
	assert.Equal(t, int64(3), records)
}

func TestAgentLinkCertificateAuthorization(t *testing.T) {
	l := startLinkListener(t, config.AgentMTLSOptional, true)
	ctx := testContext(t)
	anonymous := agentv1pb.NewAgentEnrollmentClient(l.dial(t, nil))

	// No certificate, or the node credential: refused.
	for _, callCtx := range []context.Context{
		ctx, // no credential
		legacyCredentials(ctx, l.proxy.ID, l.proxyKey),        // a proxy node's API key session
		nodeCredentials(ctx, l.forwardNode(), l.forwardToken), // a forward node's token
		nodeCredentials(ctx, l.proxyNode(), l.proxyKey),       // the enrollment bootstrap metadata
	} {
		var trailer metadata.MD
		_, err := anonymous.IssueLinkCertificate(callCtx, linkRequest(t, nil), grpc.Trailer(&trailer))
		requireRefusal(t, err, trailer, codes.Unauthenticated, agentcontrol.ErrorCodeCertInvalid)
		trailer = nil
		_, err = anonymous.GetLinkTrustBundle(callCtx, &agentv1pb.GetLinkTrustBundleRequest{}, grpc.Trailer(&trailer))
		requireRefusal(t, err, trailer, codes.Unauthenticated, agentcontrol.ErrorCodeCertInvalid)
	}

	// A certificate of a node that did not negotiate forward.v1.
	node := l.proxyNode()
	conn, _ := l.enrolledConn(t, ctx, node)
	client := agentv1pb.NewAgentEnrollmentClient(conn)
	var trailer metadata.MD
	_, err := client.IssueLinkCertificate(ctx, linkRequest(t, nil), grpc.Trailer(&trailer))
	requireRefusal(t, err, trailer, codes.FailedPrecondition, agentcontrol.ErrorCodeLinkNotNegotiated)
	negotiateForward(t, node, true)
	_, err = client.IssueLinkCertificate(ctx, linkRequest(t, nil))
	require.NoError(t, err, "a proxy node that negotiated forward.v1 gets one")
	negotiateForward(t, node, false)
	trailer = nil
	_, err = client.IssueLinkCertificate(ctx, linkRequest(t, nil), grpc.Trailer(&trailer))
	requireRefusal(t, err, trailer, codes.FailedPrecondition, agentcontrol.ErrorCodeLinkNotNegotiated)
	negotiateForward(t, node, true)

	// Wrong SANs.
	other, err := url.Parse("spiffe://anixops/test/agent/forward-999")
	require.NoError(t, err)
	for _, template := range []*x509.CertificateRequest{
		{DNSNames: []string{"forward-999"}}, {URIs: []*url.URL{other}}, {DNSNames: []string{node.String(), "cdn.example.com"}},
	} {
		trailer = nil
		_, err = client.IssueLinkCertificate(ctx, linkRequest(t, template), grpc.Trailer(&trailer))
		requireRefusal(t, err, trailer, codes.InvalidArgument, agentcontrol.ErrorCodeLinkRequestInvalid)
	}

	// RetireNode, disabling or deleting the node revokes the Agent
	// certificate and the link certificates; the session gets no more.
	require.NoError(t, agentpki.RevokeNode(ctx, database.Get(), node, agentpki.RevokeReasonNodeDeleted))
	var revoked int64
	require.NoError(t, database.Get().Model(&model.ForwardLinkCertificate{}).
		Where("node_kind = ? AND node_id = ? AND revoked_at IS NOT NULL", node.Kind, node.ID).Count(&revoked).Error)
	assert.Equal(t, int64(1), revoked)
	trailer = nil
	_, err = client.IssueLinkCertificate(ctx, linkRequest(t, nil), grpc.Trailer(&trailer))
	requireRefusal(t, err, trailer, codes.Unauthenticated, agentcontrol.ErrorCodeCertRevoked)
}

func TestAgentLinkCertificateUnavailable(t *testing.T) {
	t.Run("without the link CA", func(t *testing.T) {
		l := startLinkListener(t, config.AgentMTLSOptional, false)
		ctx := testContext(t)
		conn, _ := l.enrolledConn(t, ctx, l.forwardNode())
		client := agentv1pb.NewAgentEnrollmentClient(conn)
		var trailer metadata.MD
		_, err := client.IssueLinkCertificate(ctx, linkRequest(t, nil), grpc.Trailer(&trailer))
		requireRefusal(t, err, trailer, codes.FailedPrecondition, agentcontrol.ErrorCodeLinkUnavailable)
		trailer = nil
		_, err = client.GetLinkTrustBundle(ctx, &agentv1pb.GetLinkTrustBundleRequest{}, grpc.Trailer(&trailer))
		requireRefusal(t, err, trailer, codes.FailedPrecondition, agentcontrol.ErrorCodeLinkUnavailable)
	})
	t.Run("before the link CA exists", func(t *testing.T) {
		l := startLinkListener(t, config.AgentMTLSOptional, true)
		ctx := testContext(t)
		node := l.forwardNode()
		conn, _ := l.enrolledConn(t, ctx, node)
		negotiateForward(t, node, true)
		require.NoError(t, database.Get().Where("1 = 1").Delete(&model.ForwardLinkCA{}).Error)
		client := agentv1pb.NewAgentEnrollmentClient(conn)
		var trailer metadata.MD
		_, err := client.IssueLinkCertificate(ctx, linkRequest(t, nil), grpc.Trailer(&trailer))
		requireRefusal(t, err, trailer, codes.FailedPrecondition, agentcontrol.ErrorCodeLinkUnavailable)
		trailer = nil
		_, err = client.GetLinkTrustBundle(ctx, &agentv1pb.GetLinkTrustBundleRequest{}, grpc.Trailer(&trailer))
		requireRefusal(t, err, trailer, codes.FailedPrecondition, agentcontrol.ErrorCodeLinkUnavailable)
	})
	t.Run("agent_control.mtls off", func(t *testing.T) {
		l := startLinkListener(t, config.AgentMTLSOff, true)
		ctx := testContext(t)
		var trailer metadata.MD
		_, err := agentv1pb.NewAgentEnrollmentClient(l.dial(t, nil)).IssueLinkCertificate(ctx, linkRequest(t, nil), grpc.Trailer(&trailer))
		requireRefusal(t, err, trailer, codes.FailedPrecondition, agentcontrol.ErrorCodeLinkUnavailable)
	})
}

func TestLinkPKIStatus(t *testing.T) {
	for err, want := range map[error]codes.Code{
		agentpki.ErrLinkNotNegotiated:                 codes.FailedPrecondition,
		agentpki.ErrInvalidLinkRequest:                codes.InvalidArgument,
		agentpki.ErrLinkDisabled:                      codes.FailedPrecondition,
		agentpki.ErrNoLinkAuthority:                   codes.FailedPrecondition,
		agentpki.ErrCertificateRevoked:                codes.Unauthenticated,
		status.Error(codes.Unavailable, "x"):          codes.Unavailable,
		errors.New("database is gone"):                codes.Internal,
		agentpki.ErrCertificateWrongCluster:           codes.Unauthenticated,
		errors.Join(agentpki.ErrInvalidCertificate):   codes.Unauthenticated,
		errors.Join(modulepki.ErrInvalidRequest, nil): codes.InvalidArgument,
	} {
		assert.Equal(t, want, status.Code(linkPKIStatus(err)), err)
	}
}

// TestAgentLinkCertificateAfterHelloAck: Control records the forward.v1
// flag before it sends the HelloAck, so an Agent may ask for its link
// certificate as soon as a HelloAck lists forward.v1; a later Hello without
// it withdraws the right.
func TestAgentLinkCertificateAfterHelloAck(t *testing.T) {
	l := startLinkListener(t, config.AgentMTLSOptional, true)
	requireAutoMigrate(t, &model.KernelNodeDesiredConfig{}, &model.KernelNodeConfigStatus{})
	ctx := testContext(t)
	node := l.forwardNode()
	conn, _ := l.enrolledConn(t, ctx, node)
	client := agentv1pb.NewAgentEnrollmentClient(conn)

	hello := func(capabilities []*agentv1pb.Capability) *agentv1pb.HelloAck {
		t.Helper()
		streamCtx, cancel := context.WithCancel(ctx)
		t.Cleanup(cancel)
		stream, err := agentv1pb.NewAgentControlServiceClient(conn).ControlStream(streamCtx)
		require.NoError(t, err)
		message := validAgentHello(node.ID)
		message.GetHello().Capabilities = capabilities
		require.NoError(t, stream.Send(message))
		reply, err := stream.Recv()
		require.NoError(t, err)
		require.NotNil(t, reply.GetHelloAck())
		return reply.GetHelloAck()
	}
	ack := hello(forwardHello(t))
	assert.True(t, agentcontrol.HasCapabilityVersion(ack.GetServerCapabilities(), agentcontrol.CapabilityForward, agentcontrol.CapabilityVersionV1))
	_, err := client.IssueLinkCertificate(ctx, linkRequest(t, nil))
	require.NoError(t, err, "right after the HelloAck")

	hello([]*agentv1pb.Capability{{Name: "agent.ping", Version: agentcontrol.CapabilityVersionV1}})
	var trailer metadata.MD
	_, err = client.IssueLinkCertificate(ctx, linkRequest(t, nil), grpc.Trailer(&trailer))
	requireRefusal(t, err, trailer, codes.FailedPrecondition, agentcontrol.ErrorCodeLinkNotNegotiated)
}
