package grpc

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	pb "github.com/AnixOps/anix-control/v4/api/grpc/v2boardpb"
	"github.com/AnixOps/anix-control/v4/internal/agentpki"
	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

func TestAgentRefusalCarriesItsCode(t *testing.T) {
	err := refuseAgent(agentcontrol.ErrorCodeCertRevoked, codes.Unauthenticated, "revoked")
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
	assert.Equal(t, agentcontrol.ErrorCodeCertRevoked+": revoked", status.Convert(err).Message())
	assert.Equal(t, agentcontrol.ErrorCodeCertRevoked, refusalCode(fmt.Errorf("wrapped: %w", err)))
	assert.Empty(t, refusalCode(status.Error(codes.Unavailable, "down")))

	var trailers []metadata.MD
	collect := func(md metadata.MD) { trailers = append(trailers, md) }
	setRefusalTrailer(status.Error(codes.Unavailable, "down"), collect)
	setRefusalTrailer(nil, collect)
	assert.Empty(t, trailers, "only refusals set a trailer")

	refusal := refuseAgent(agentcontrol.ErrorCodeMTLSRequired, codes.Unauthenticated, "required").(*agentRefusal)
	refusal.trailer = metadata.Pairs(agentcontrol.MetadataAuthDeprecated, "yes", agentcontrol.MetadataErrorCode, "stale")
	setRefusalTrailer(refusal, collect)
	require.Len(t, trailers, 1)
	assert.Equal(t, []string{agentcontrol.ErrorCodeMTLSRequired}, trailers[0].Get(agentcontrol.MetadataErrorCode), "the code is written once")
	assert.Equal(t, []string{"yes"}, trailers[0].Get(agentcontrol.MetadataAuthDeprecated))
	assert.Equal(t, []string{"stale"}, refusal.trailer.Get(agentcontrol.MetadataErrorCode), "the refusal's own metadata is not changed")
}

func TestCertificateRefusalMapsVerificationErrors(t *testing.T) {
	for err, want := range map[error]string{
		agentpki.ErrCertificateRevoked:                                 agentcontrol.ErrorCodeCertRevoked,
		fmt.Errorf("%w (not_after x)", agentpki.ErrCertificateExpired): agentcontrol.ErrorCodeCertExpired,
		fmt.Errorf("%w: x", agentpki.ErrCertificateWrongCluster):       agentcontrol.ErrorCodeCertWrongCluster,
		fmt.Errorf("%w: bad", agentpki.ErrInvalidCertificate):          agentcontrol.ErrorCodeCertInvalid,
	} {
		refusal := certificateRefusal(err)
		assert.Equal(t, want, refusalCode(refusal), "%v", err)
		assert.Equal(t, codes.Unauthenticated, status.Code(refusal))
	}
	assert.Nil(t, certificateRefusal(nil))
	transient := errors.New("database is down")
	assert.Same(t, transient, certificateRefusal(transient))
}

// refusedTrailer reads a refused stream to its end and returns the code
// and the error code trailer.
func refusedTrailer(t *testing.T, stream agentv1pb.AgentControlService_ControlStreamClient) (codes.Code, []string) {
	t.Helper()
	var err error
	for err == nil {
		_, err = stream.Recv()
	}
	return status.Code(err), stream.Trailer().Get(agentcontrol.MetadataErrorCode)
}

// openRefusedStream opens a stream on conn with ctx, sends a Hello for
// nodeID and returns the refusal's code and error code trailer.
func openRefusedStream(t *testing.T, ctx context.Context, conn *grpc.ClientConn, nodeID uint32) (codes.Code, []string) {
	t.Helper()
	stream, err := agentv1pb.NewAgentControlServiceClient(conn).ControlStream(ctx)
	require.NoError(t, err)
	_ = stream.Send(validAgentHello(nodeID))
	return refusedTrailer(t, stream)
}

// Every refusal of an agent certificate, on the stream, on the enrollment
// service and on the v2board services, names its reason in the
// x-anix-error-code trailer.
func TestAgentListenerCertificateRefusalCodes(t *testing.T) {
	l := startAgentListener(t, config.AgentMTLSOptional, true)
	ctx := testContext(t)
	anonymous := l.dial(t, nil)
	proxyCertificate, _, err := enrollForTest(t, nodeCredentials(ctx, l.proxyNode(), l.proxyKey), anonymous, "")
	require.NoError(t, err)
	proxyConn := l.dial(t, proxyCertificate)

	// The certificate names another node than the metadata or the Hello.
	code, trailer := openRefusedStream(t, legacyCredentials(ctx, l.proxy.ID+1, l.proxyKey), proxyConn, uint32(l.proxy.ID))
	assert.Equal(t, codes.Unauthenticated, code)
	assert.Equal(t, []string{agentcontrol.ErrorCodeCertWrongNode}, trailer)
	code, trailer = openRefusedStream(t, ctx, proxyConn, uint32(l.proxy.ID)+1)
	assert.Equal(t, codes.PermissionDenied, code)
	assert.Equal(t, []string{agentcontrol.ErrorCodeCertWrongNode}, trailer)
	// An API key session naming another node is refused without a
	// certificate code.
	code, trailer = openRefusedStream(t, legacyCredentials(ctx, l.proxy.ID, l.proxyKey), anonymous, uint32(l.proxy.ID)+1)
	assert.Equal(t, codes.PermissionDenied, code)
	assert.Empty(t, trailer)

	// A forward node's certificate on the proxy-only v2board services.
	forwardCertificate, _, err := enrollForTest(t, nodeCredentials(ctx, l.forwardNode(), l.forwardToken), anonymous, "")
	require.NoError(t, err)
	var unaryTrailer metadata.MD
	_, err = pb.NewNodeServiceClient(l.dial(t, forwardCertificate)).GetConfig(ctx, &pb.NodeConfigRequest{NodeId: uint32(l.forward.ID)}, grpc.Trailer(&unaryTrailer))
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
	assert.Equal(t, []string{agentcontrol.ErrorCodeCertWrongNode}, unaryTrailer.Get(agentcontrol.MetadataErrorCode))
	statusStream, err := pb.NewNodeServiceClient(l.dial(t, forwardCertificate)).StatusStream(ctx)
	require.NoError(t, err)
	_, err = statusStream.Recv()
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
	assert.Equal(t, []string{agentcontrol.ErrorCodeCertWrongNode}, statusStream.Trailer().Get(agentcontrol.MetadataErrorCode))

	// A certificate that is no agent's: the kernel's own.
	source, err := l.authority.KernelTLS(ctx, time.Minute)
	require.NoError(t, err)
	kernelCertificate, err := source.Certificate()
	require.NoError(t, err)
	code, trailer = openRefusedStream(t, ctx, l.dial(t, kernelCertificate), uint32(l.proxy.ID))
	assert.Equal(t, codes.Unauthenticated, code)
	assert.Equal(t, []string{agentcontrol.ErrorCodeCertInvalid}, trailer)

	// Renew and GetTrustBundle without a certificate.
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{}, key)
	require.NoError(t, err)
	unaryTrailer = nil
	_, err = agentv1pb.NewAgentEnrollmentClient(anonymous).Renew(ctx, &agentv1pb.RenewAgentCertificateRequest{CsrDer: csr}, grpc.Trailer(&unaryTrailer))
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
	assert.Equal(t, []string{agentcontrol.ErrorCodeCertInvalid}, unaryTrailer.Get(agentcontrol.MetadataErrorCode))
	unaryTrailer = nil
	_, err = agentv1pb.NewAgentEnrollmentClient(anonymous).GetTrustBundle(ctx, &agentv1pb.GetAgentTrustBundleRequest{}, grpc.Trailer(&unaryTrailer))
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
	assert.Equal(t, []string{agentcontrol.ErrorCodeCertInvalid}, unaryTrailer.Get(agentcontrol.MetadataErrorCode))

	// An unusable bootstrap credential.
	for name, enrollCtx := range map[string]context.Context{
		"wrong API key": nodeCredentials(ctx, l.proxyNode(), "wrong"),
		"no credential": ctx,
		"bad node id":   metadata.AppendToOutgoingContext(ctx, agentcontrol.MetadataNodeID, "x", agentcontrol.MetadataAPIKey, l.proxyKey),
		"bad node kind": metadata.AppendToOutgoingContext(ctx, agentcontrol.MetadataNodeID, "1", agentcontrol.MetadataNodeKind, "edge", agentcontrol.MetadataAPIKey, l.proxyKey),
	} {
		unaryTrailer = nil
		_, err = agentv1pb.NewAgentEnrollmentClient(anonymous).Enroll(enrollCtx, &agentv1pb.EnrollAgentRequest{CsrDer: csr}, grpc.Trailer(&unaryTrailer))
		assert.Equal(t, codes.Unauthenticated, status.Code(err), name)
		assert.Equal(t, []string{agentcontrol.ErrorCodeEnrollmentRejected}, unaryTrailer.Get(agentcontrol.MetadataErrorCode), name)
	}
	credential, _, err := l.pki.CreateEnrollmentToken(ctx, agentpki.TokenRequest{Node: l.proxyNode(), TTL: time.Hour})
	require.NoError(t, err)
	_, _, err = enrollForTest(t, ctx, anonymous, credential)
	require.NoError(t, err)
	unaryTrailer = nil
	_, err = agentv1pb.NewAgentEnrollmentClient(anonymous).Enroll(ctx, &agentv1pb.EnrollAgentRequest{CsrDer: csr, EnrollmentCredential: credential}, grpc.Trailer(&unaryTrailer))
	assert.Equal(t, codes.Unauthenticated, status.Code(err), "a credential enrolls once")
	assert.Equal(t, []string{agentcontrol.ErrorCodeEnrollmentRejected}, unaryTrailer.Get(agentcontrol.MetadataErrorCode))

	// A certificate revoked while its stream is open ends the stream at
	// the next heartbeat; then new streams, renewal, the trust bundle and
	// the v2board services refuse it as revoked.
	stream, helloAck, err := openStream(t, ctx, proxyConn, uint32(l.proxy.ID))
	require.NoError(t, err)
	require.NoError(t, service.NewNodeService().UpdateNode(l.proxy.ID, map[string]any{"status": model.NodeStatusDisabled}))
	require.NoError(t, stream.Send(heartbeatMessage("heartbeat", uint32(l.proxy.ID), helloAck.SessionId)))
	code, trailer = refusedTrailer(t, stream)
	assert.Equal(t, codes.Unauthenticated, code)
	assert.Equal(t, []string{agentcontrol.ErrorCodeCertRevoked}, trailer)
	code, trailer = openRefusedStream(t, ctx, proxyConn, uint32(l.proxy.ID))
	assert.Equal(t, codes.Unauthenticated, code)
	assert.Equal(t, []string{agentcontrol.ErrorCodeCertRevoked}, trailer)
	unaryTrailer = nil
	_, err = agentv1pb.NewAgentEnrollmentClient(proxyConn).Renew(ctx, &agentv1pb.RenewAgentCertificateRequest{CsrDer: csr}, grpc.Trailer(&unaryTrailer))
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
	assert.Equal(t, []string{agentcontrol.ErrorCodeCertRevoked}, unaryTrailer.Get(agentcontrol.MetadataErrorCode))
	unaryTrailer = nil
	_, err = agentv1pb.NewAgentEnrollmentClient(proxyConn).GetTrustBundle(ctx, &agentv1pb.GetAgentTrustBundleRequest{}, grpc.Trailer(&unaryTrailer))
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
	assert.Equal(t, []string{agentcontrol.ErrorCodeCertRevoked}, unaryTrailer.Get(agentcontrol.MetadataErrorCode))
	unaryTrailer = nil
	_, err = pb.NewNodeServiceClient(proxyConn).GetConfig(ctx, &pb.NodeConfigRequest{NodeId: uint32(l.proxy.ID)}, grpc.Trailer(&unaryTrailer))
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
	assert.Equal(t, []string{agentcontrol.ErrorCodeCertRevoked}, unaryTrailer.Get(agentcontrol.MetadataErrorCode))
}

// tlsPeerContext is the context of a call whose client presented
// certificate over TLS.
func tlsPeerContext(certificate *tls.Certificate) context.Context {
	leaf, _ := x509.ParseCertificate(certificate.Certificate[0])
	return peer.NewContext(context.Background(), &peer.Peer{AuthInfo: credentials.TLSInfo{
		State: tls.ConnectionState{PeerCertificates: []*x509.Certificate{leaf}},
	}})
}

// An expired certificate is refused as expired, at connection and on an
// open stream's heartbeat; one of another cluster as such; a certificate
// presented to a listener without the agent PKI as invalid.
func TestAgentCertificateExpiredAndWrongClusterCodes(t *testing.T) {
	l := startAgentListener(t, config.AgentMTLSOptional, true)
	ctx := testContext(t)
	certificate, issued, err := enrollForTest(t, nodeCredentials(ctx, l.proxyNode(), l.proxyKey), l.dial(t, nil), "")
	require.NoError(t, err)

	later, err := agentpki.New(agentpki.Options{DB: database.Get(), Authority: l.authority, Now: func() time.Time {
		return time.Unix(issued.GetNotAfterUnix(), 0).Add(time.Hour)
	}})
	require.NoError(t, err)
	_, _, err = (&AgentAuthenticator{PKI: later, Mode: config.AgentMTLSOptional}).certificatePrincipal(tlsPeerContext(certificate))
	assert.Equal(t, agentcontrol.ErrorCodeCertExpired, refusalCode(err))
	assert.Equal(t, codes.Unauthenticated, status.Code(err))

	principal, ok, err := (&AgentAuthenticator{PKI: l.pki, Mode: config.AgentMTLSOptional}).certificatePrincipal(tlsPeerContext(certificate))
	require.NoError(t, err)
	require.True(t, ok)
	principal.NotAfter = time.Now().Add(-time.Second)
	err = (&AgentAuthenticator{PKI: l.pki, Mode: config.AgentMTLSOptional}).recheck(ctx, principal)
	assert.Equal(t, agentcontrol.ErrorCodeCertExpired, refusalCode(err))

	// The module CA signs another cluster's agent identity only in a test.
	_, csr := func() (*ecdsa.PrivateKey, []byte) {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)
		csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{}, key)
		require.NoError(t, err)
		return key, csr
	}()
	other, err := agentcontrol.NewAgentIdentity("other", l.proxyNode())
	require.NoError(t, err)
	signed, err := l.authority.SignLeaf(ctx, database.Get(), other.URL(), time.Hour, csr)
	require.NoError(t, err)
	_, _, err = (&AgentAuthenticator{PKI: l.pki, Mode: config.AgentMTLSOptional}).certificatePrincipal(
		tlsPeerContext(&tls.Certificate{Certificate: [][]byte{signed.CertificateDER}}))
	assert.Equal(t, agentcontrol.ErrorCodeCertWrongCluster, refusalCode(err))

	_, _, err = (&AgentAuthenticator{Mode: config.AgentMTLSOptional}).certificatePrincipal(tlsPeerContext(certificate))
	assert.Equal(t, agentcontrol.ErrorCodeCertInvalid, refusalCode(err), "no agent PKI to verify it")
}
