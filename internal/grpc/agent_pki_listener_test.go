package grpc

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	pb "github.com/AnixOps/anix-control/v4/api/grpc/v2boardpb"
	"github.com/AnixOps/anix-control/v4/internal/agentpki"
	"github.com/AnixOps/anix-control/v4/internal/agenttransport"
	"github.com/AnixOps/anix-control/v4/internal/cache"
	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/modulepki"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// agentListener is the real node-facing listener (Server.Start) with TLS,
// the agent PKI and one proxy node and one forward node of the same id.
type agentListener struct {
	server       *Server
	addr         string
	serverRoots  *x509.CertPool
	authority    *modulepki.Authority
	pki          *agentpki.Service
	proxy        model.Node
	proxyKey     string
	forward      model.ForwardNode
	forwardToken string
}

func startAgentListener(t *testing.T, mode string, withPKI bool) *agentListener {
	t.Helper()
	return startAgentListenerWith(t, mode, withPKI, nil)
}

// startAgentListenerWith is startAgentListener with configure applied to
// the server configuration before the listener starts.
func startAgentListenerWith(t *testing.T, mode string, withPKI bool, configure func(*ServerConfig)) *agentListener {
	t.Helper()
	cache.InitMemory()
	requireInMemoryDatabase(t)
	requireAutoMigrate(t, &model.Node{}, &model.NodeProtocol{}, &model.AuthorizedKey{}, &model.ForwardNode{},
		&model.ServiceCA{}, &model.AgentEnrollment{}, &model.AgentCertificate{}, &model.OperationLog{},
		&model.NodeServiceAssignment{}, &model.PluginTelemetryState{}, &model.NodePluginObservedState{}, &model.AgentTransport{},
		&model.NodeOperationRevision{})
	db := database.Get()
	// A fresh transport recorder: its throttle must not carry sightings of
	// an earlier test's node with the same id.
	t.Cleanup(agenttransport.SetDefault(agenttransport.NewRecorder(database.Get)))

	l := &agentListener{proxyKey: "agent-pki-proxy-key", forwardToken: "agent-pki-forward-token"}
	l.proxy = model.Node{Name: "proxy", Host: "127.0.0.1", APIKeyHash: apiKeyHashForTest(l.proxyKey), Status: model.NodeStatusOnline}
	require.NoError(t, db.Create(&l.proxy).Error)
	l.forward = model.ForwardNode{ID: l.proxy.ID, Name: "forward", Host: "127.0.0.1", Port: 8443, APIToken: l.forwardToken, Enabled: true}
	require.NoError(t, db.Create(&l.forward).Error)

	cfg := DefaultServerConfig()
	cfg.Host, cfg.Port, cfg.AgentMTLS = "127.0.0.1", 0, mode
	cfg.TLSCertFile, cfg.TLSKeyFile, l.serverRoots = writeServerCertificate(t)
	if withPKI {
		kek := make([]byte, 32)
		_, err := rand.Read(kek)
		require.NoError(t, err)
		l.authority, err = modulepki.New(modulepki.Options{DB: db, Cluster: "test", KEK: kek})
		require.NoError(t, err)
		require.NoError(t, l.authority.Ensure(t.Context()))
		l.pki, err = agentpki.New(agentpki.Options{DB: db, Authority: l.authority})
		require.NoError(t, err)
		cfg.AgentPKI = l.pki
	}
	if configure != nil {
		configure(cfg)
	}
	l.server = NewServer(cfg)
	require.NoError(t, l.server.Start())
	l.addr = l.server.listener.Addr().String()
	t.Cleanup(func() {
		l.server.Stop()
		requireDatabaseClosed(t)
	})
	return l
}

// writeServerCertificate writes a throwaway self-signed server certificate
// for 127.0.0.1, the listener's public TLS certificate (grpc.tls_cert_file).
func writeServerCertificate(t *testing.T) (string, string, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "control.test"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses: []net.IP{net.IPv4(127, 0, 0, 1)}, BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	dir := t.TempDir()
	certFile, keyFile := filepath.Join(dir, "server.crt"), filepath.Join(dir, "server.key")
	require.NoError(t, os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600))
	require.NoError(t, os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), 0o600))
	leaf, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	roots := x509.NewCertPool()
	roots.AddCert(leaf)
	return certFile, keyFile, roots
}

// dial connects over TLS, presenting certificate when it is not nil.
func (l *agentListener) dial(t *testing.T, certificate *tls.Certificate) *grpc.ClientConn {
	t.Helper()
	config := &tls.Config{RootCAs: l.serverRoots, MinVersion: tls.VersionTLS12}
	if certificate != nil {
		config.Certificates = []tls.Certificate{*certificate}
	}
	conn, err := grpc.NewClient(l.addr, grpc.WithTransportCredentials(credentials.NewTLS(config)))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func (l *agentListener) proxyNode() agentcontrol.AgentNode {
	return agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: uint32(l.proxy.ID)}
}

func (l *agentListener) forwardNode() agentcontrol.AgentNode {
	return agentcontrol.AgentNode{Kind: agentcontrol.NodeKindForward, ID: uint32(l.forward.ID)}
}

func nodeCredentials(ctx context.Context, node agentcontrol.AgentNode, secret string) context.Context {
	return metadata.AppendToOutgoingContext(ctx, agentcontrol.MetadataNodeID, strconv.FormatUint(uint64(node.ID), 10),
		agentcontrol.MetadataAPIKey, secret, agentcontrol.MetadataNodeKind, node.Kind)
}

func legacyCredentials(ctx context.Context, nodeID uint, apiKey string) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "x-node-id", strconv.FormatUint(uint64(nodeID), 10), "x-api-key", apiKey)
}

// enroll calls Enroll on conn and returns the agent's TLS certificate.
func enrollForTest(t *testing.T, ctx context.Context, conn *grpc.ClientConn, credential string) (*tls.Certificate, *agentv1pb.AgentCertificate, error) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: "proxy-999"}}, key)
	require.NoError(t, err)
	response, err := agentv1pb.NewAgentEnrollmentClient(conn).Enroll(ctx, &agentv1pb.EnrollAgentRequest{
		CsrDer: csr, EnrollmentCredential: credential, AgentVersion: "2.0.0", InstanceId: "instance",
	})
	if err != nil {
		return nil, nil, err
	}
	issued := response.GetCertificate()
	return &tls.Certificate{Certificate: [][]byte{issued.GetCertificateDer()}, PrivateKey: key}, issued, nil
}

// openStream sends Hello for nodeID and returns the stream and HelloAck.
func openStream(t *testing.T, ctx context.Context, conn *grpc.ClientConn, nodeID uint32) (agentv1pb.AgentControlService_ControlStreamClient, *agentv1pb.HelloAck, error) {
	t.Helper()
	stream, err := agentv1pb.NewAgentControlServiceClient(conn).ControlStream(ctx)
	require.NoError(t, err)
	if err := stream.Send(validAgentHello(nodeID)); err != nil {
		_, err = stream.Recv()
		return stream, nil, err
	}
	message, err := stream.Recv()
	if err != nil {
		return stream, nil, err
	}
	return stream, message.GetHelloAck(), nil
}

func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func TestAgentListenerLegacyAgentWithoutCertificateInOptionalMode(t *testing.T) {
	l := startAgentListener(t, config.AgentMTLSOptional, true)
	conn := l.dial(t, nil)
	ctx := testContext(t)

	var header metadata.MD
	stream, err := agentv1pb.NewAgentControlServiceClient(conn).ControlStream(legacyCredentials(ctx, l.proxy.ID, l.proxyKey))
	require.NoError(t, err)
	require.NoError(t, stream.Send(validAgentHello(uint32(l.proxy.ID))))
	message, err := stream.Recv()
	require.NoError(t, err)
	require.NotNil(t, message.GetHelloAck())
	header, err = stream.Header()
	require.NoError(t, err)
	assert.Empty(t, header.Get(agentcontrol.MetadataAuthDeprecated), "optional mode sends no deprecation header")

	// The v2board services keep the legacy credential too.
	_, err = pb.NewNodeServiceClient(conn).GetConfig(legacyCredentials(ctx, l.proxy.ID, l.proxyKey), &pb.NodeConfigRequest{NodeId: uint32(l.proxy.ID)})
	assert.NotContains(t, []codes.Code{codes.Unauthenticated, codes.PermissionDenied}, status.Code(err))

	// A wrong key is still refused.
	_, _, err = openStream(t, legacyCredentials(ctx, l.proxy.ID, "wrong"), conn, uint32(l.proxy.ID))
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestAgentListenerEnrollsWithEachBootstrapAndStreamsByCertificate(t *testing.T) {
	l := startAgentListener(t, config.AgentMTLSOptional, true)
	anonymous := l.dial(t, nil)
	ctx := testContext(t)

	// 1. An agent in the field: its proxy node's API key.
	proxyCertificate, issued, err := enrollForTest(t, nodeCredentials(ctx, l.proxyNode(), l.proxyKey), anonymous, "")
	require.NoError(t, err)
	assert.Equal(t, "spiffe://anixops/test/agent/"+l.proxyNode().String(), issued.GetSpiffeId())
	assert.Equal(t, l.proxyNode().String(), issued.GetNode())
	assert.Equal(t, int64((agentpki.DefaultCertificateLifetime / 3).Seconds()), issued.GetNotAfterUnix()-issued.GetRenewAfterUnix(),
		"renew after two thirds of seven days")

	conn := l.dial(t, proxyCertificate)
	_, helloAck, err := openStream(t, ctx, conn, uint32(l.proxy.ID))
	require.NoError(t, err)
	require.NotNil(t, helloAck)
	snapshot, connected := GetAgentControlManager().Connection(uint32(l.proxy.ID))
	require.True(t, connected)
	assert.Equal(t, helloAck.SessionId, snapshot.SessionID)

	// Renew and GetTrustBundle need the certificate.
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{}, key)
	require.NoError(t, err)
	renewed, err := agentv1pb.NewAgentEnrollmentClient(conn).Renew(ctx, &agentv1pb.RenewAgentCertificateRequest{CsrDer: csr})
	require.NoError(t, err)
	assert.Equal(t, issued.GetSpiffeId(), renewed.GetCertificate().GetSpiffeId())
	assert.NotEqual(t, issued.GetSerial(), renewed.GetCertificate().GetSerial())
	bundle, err := agentv1pb.NewAgentEnrollmentClient(conn).GetTrustBundle(ctx, &agentv1pb.GetAgentTrustBundleRequest{})
	require.NoError(t, err)
	assert.Len(t, bundle.GetTrustBundleDer(), 1)
	_, err = agentv1pb.NewAgentEnrollmentClient(anonymous).Renew(ctx, &agentv1pb.RenewAgentCertificateRequest{CsrDer: csr})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
	_, err = agentv1pb.NewAgentEnrollmentClient(anonymous).GetTrustBundle(ctx, &agentv1pb.GetAgentTrustBundleRequest{})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))

	// 2. A forward node: its token, with x-node-kind forward. It streams in
	// its own manager: forward and proxy ids overlap.
	forwardCertificate, issued, err := enrollForTest(t, nodeCredentials(ctx, l.forwardNode(), l.forwardToken), anonymous, "")
	require.NoError(t, err)
	assert.Equal(t, "spiffe://anixops/test/agent/"+l.forwardNode().String(), issued.GetSpiffeId())
	forwardConn := l.dial(t, forwardCertificate)
	_, helloAck, err = openStream(t, ctx, forwardConn, uint32(l.forward.ID))
	require.NoError(t, err)
	forwardSnapshot, connected := GetForwardAgentControlManager().Connection(uint32(l.forward.ID))
	require.True(t, connected)
	assert.Equal(t, helloAck.SessionId, forwardSnapshot.SessionID)
	proxySnapshot, _ := GetAgentControlManager().Connection(uint32(l.proxy.ID))
	assert.Equal(t, snapshot.SessionID, proxySnapshot.SessionID, "the forward stream must not replace the proxy stream")
	var forward model.ForwardNode
	require.NoError(t, database.Get().First(&forward, l.forward.ID).Error)
	assert.Equal(t, model.ForwardNodeStatusOnline, forward.Status)

	// 3. A new node: a one-time enrollment credential, without node
	// metadata.
	credential, _, err := l.pki.CreateEnrollmentToken(ctx, agentpki.TokenRequest{Node: l.proxyNode(), TTL: time.Hour})
	require.NoError(t, err)
	_, issued, err = enrollForTest(t, ctx, anonymous, credential)
	require.NoError(t, err)
	assert.Equal(t, l.proxyNode().String(), issued.GetNode())
	_, _, err = enrollForTest(t, ctx, anonymous, credential)
	assert.Equal(t, codes.Unauthenticated, status.Code(err), "a credential enrolls once")

	// 4. A registration key mints a node credential, which then enrolls.
	authKey := "agent-pki-registration-key"
	digest := sha256.Sum256([]byte(authKey))
	require.NoError(t, database.Get().Create(&model.AuthorizedKey{Name: "fleet", Key: authKey, KeyHash: hex.EncodeToString(digest[:])}).Error)
	registered, err := pb.NewNodeServiceClient(anonymous).Register(ctx, &pb.NodeRegisterRequest{AuthKey: authKey, Name: "registered", Host: "127.0.0.1", Port: 443})
	require.NoError(t, err)
	registeredNode := agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: registered.GetNodeId()}
	registeredCertificate, issued, err := enrollForTest(t, nodeCredentials(ctx, registeredNode, registered.GetApiKey()), anonymous, "")
	require.NoError(t, err)
	assert.Equal(t, registeredNode.String(), issued.GetNode())
	_, helloAck, err = openStream(t, ctx, l.dial(t, registeredCertificate), registered.GetNodeId())
	require.NoError(t, err)
	assert.NotEmpty(t, helloAck.SessionId)

	// Wrong bootstraps are refused alike.
	for name, enrollCtx := range map[string]context.Context{
		"no credential":            ctx,
		"wrong API key":            nodeCredentials(ctx, l.proxyNode(), "wrong"),
		"token as proxy key":       nodeCredentials(ctx, l.proxyNode(), l.forwardToken),
		"API key as forward token": nodeCredentials(ctx, l.forwardNode(), l.proxyKey),
	} {
		_, _, err := enrollForTest(t, enrollCtx, anonymous, "")
		assert.Equal(t, codes.Unauthenticated, status.Code(err), name)
	}

	var enrollments []model.OperationLog
	require.NoError(t, database.Get().Where("action = ?", agentpki.AuditActionEnroll).Find(&enrollments).Error)
	assert.Len(t, enrollments, 4)
}

func TestAgentListenerBindsEnvelopesAndRequestsToTheCertificate(t *testing.T) {
	l := startAgentListener(t, config.AgentMTLSOptional, true)
	ctx := testContext(t)
	anonymous := l.dial(t, nil)
	proxyCertificate, _, err := enrollForTest(t, nodeCredentials(ctx, l.proxyNode(), l.proxyKey), anonymous, "")
	require.NoError(t, err)
	conn := l.dial(t, proxyCertificate)

	// Hello naming another node.
	_, _, err = openStream(t, ctx, conn, uint32(l.proxy.ID)+1)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))

	// A later envelope naming another node.
	stream, helloAck, err := openStream(t, ctx, conn, uint32(l.proxy.ID))
	require.NoError(t, err)
	require.NoError(t, stream.Send(&agentv1pb.AgentToControl{
		RequestId: "heartbeat", NodeId: uint32(l.proxy.ID) + 1, SentAtUnixMs: time.Now().UnixMilli(),
		Payload: &agentv1pb.AgentToControl_Heartbeat{Heartbeat: &agentv1pb.Heartbeat{SessionId: helloAck.SessionId}},
	}))
	_, err = stream.Recv()
	assert.Equal(t, codes.PermissionDenied, status.Code(err))

	// Metadata naming another node than the certificate.
	_, _, err = openStream(t, legacyCredentials(ctx, l.proxy.ID+1, l.proxyKey), conn, uint32(l.proxy.ID))
	assert.Equal(t, codes.Unauthenticated, status.Code(err))

	// v2board requests must name the certificate's node.
	nodes := pb.NewNodeServiceClient(conn)
	_, err = nodes.GetConfig(ctx, &pb.NodeConfigRequest{NodeId: uint32(l.proxy.ID) + 1})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
	_, err = nodes.GetConfig(ctx, &pb.NodeConfigRequest{NodeId: uint32(l.proxy.ID)})
	assert.NotContains(t, []codes.Code{codes.Unauthenticated, codes.PermissionDenied}, status.Code(err))

	// A forward node's certificate is for the Agent services only.
	forwardCertificate, _, err := enrollForTest(t, nodeCredentials(ctx, l.forwardNode(), l.forwardToken), anonymous, "")
	require.NoError(t, err)
	_, err = pb.NewNodeServiceClient(l.dial(t, forwardCertificate)).GetConfig(ctx, &pb.NodeConfigRequest{NodeId: uint32(l.forward.ID)})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestAgentListenerModes(t *testing.T) {
	t.Run("preferred", func(t *testing.T) {
		l := startAgentListener(t, config.AgentMTLSPreferred, true)
		ctx := testContext(t)
		anonymous := l.dial(t, nil)
		stream, err := agentv1pb.NewAgentControlServiceClient(anonymous).ControlStream(legacyCredentials(ctx, l.proxy.ID, l.proxyKey))
		require.NoError(t, err)
		require.NoError(t, stream.Send(validAgentHello(uint32(l.proxy.ID))))
		_, err = stream.Recv()
		require.NoError(t, err, "legacy credentials still work")
		header, err := stream.Header()
		require.NoError(t, err)
		assert.NotEmpty(t, header.Get(agentcontrol.MetadataAuthDeprecated))

		certificate, _, err := enrollForTest(t, nodeCredentials(ctx, l.proxyNode(), l.proxyKey), anonymous, "")
		require.NoError(t, err, "agents in the field still enroll with their key")
		stream, err = agentv1pb.NewAgentControlServiceClient(l.dial(t, certificate)).ControlStream(ctx)
		require.NoError(t, err)
		require.NoError(t, stream.Send(validAgentHello(uint32(l.proxy.ID))))
		_, err = stream.Recv()
		require.NoError(t, err)
		header, err = stream.Header()
		require.NoError(t, err)
		assert.Empty(t, header.Get(agentcontrol.MetadataAuthDeprecated))
	})

	t.Run("required", func(t *testing.T) {
		l := startAgentListener(t, config.AgentMTLSRequired, true)
		ctx := testContext(t)
		anonymous := l.dial(t, nil)
		_, _, err := openStream(t, legacyCredentials(ctx, l.proxy.ID, l.proxyKey), anonymous, uint32(l.proxy.ID))
		assert.Equal(t, codes.Unauthenticated, status.Code(err), "legacy credentials no longer open the stream")
		_, _, err = enrollForTest(t, nodeCredentials(ctx, l.proxyNode(), l.proxyKey), anonymous, "")
		assert.Equal(t, codes.Unauthenticated, status.Code(err), "API keys no longer enroll")

		credential, _, err := l.pki.CreateEnrollmentToken(ctx, agentpki.TokenRequest{Node: l.proxyNode(), TTL: time.Hour})
		require.NoError(t, err)
		certificate, _, err := enrollForTest(t, ctx, anonymous, credential)
		require.NoError(t, err)
		_, helloAck, err := openStream(t, ctx, l.dial(t, certificate), uint32(l.proxy.ID))
		require.NoError(t, err)
		assert.NotEmpty(t, helloAck.SessionId)

		// Third-party node software keeps the v2board services.
		_, err = pb.NewNodeServiceClient(anonymous).GetConfig(legacyCredentials(ctx, l.proxy.ID, l.proxyKey), &pb.NodeConfigRequest{NodeId: uint32(l.proxy.ID)})
		assert.NotContains(t, []codes.Code{codes.Unauthenticated, codes.PermissionDenied}, status.Code(err))
	})

	t.Run("without the agent PKI", func(t *testing.T) {
		l := startAgentListener(t, config.AgentMTLSOptional, false)
		ctx := testContext(t)
		anonymous := l.dial(t, nil)
		_, _, err := enrollForTest(t, nodeCredentials(ctx, l.proxyNode(), l.proxyKey), anonymous, "")
		assert.Equal(t, codes.FailedPrecondition, status.Code(err))
		_, helloAck, err := openStream(t, legacyCredentials(ctx, l.proxy.ID, l.proxyKey), anonymous, uint32(l.proxy.ID))
		require.NoError(t, err)
		assert.NotEmpty(t, helloAck.SessionId)
	})
}

func TestAgentListenerRefusesRevokedAndForeignCertificates(t *testing.T) {
	l := startAgentListener(t, config.AgentMTLSOptional, true)
	ctx := testContext(t)
	anonymous := l.dial(t, nil)

	// Replacing a forward node's token revokes its certificates: an open
	// stream ends at its next heartbeat and new streams are refused.
	forwardCertificate, _, err := enrollForTest(t, nodeCredentials(ctx, l.forwardNode(), l.forwardToken), anonymous, "")
	require.NoError(t, err)
	forwardConn := l.dial(t, forwardCertificate)
	stream, helloAck, err := openStream(t, ctx, forwardConn, uint32(l.forward.ID))
	require.NoError(t, err)
	forward, err := service.NewForwardNodeService(database.Get()).GetByID(l.forward.ID)
	require.NoError(t, err)
	forward.APIToken = "replaced-forward-token"
	require.NoError(t, service.NewForwardNodeService(database.Get()).Update(forward))
	require.NoError(t, stream.Send(&agentv1pb.AgentToControl{
		RequestId: "heartbeat", NodeId: uint32(l.forward.ID), SentAtUnixMs: time.Now().UnixMilli(),
		Payload: &agentv1pb.AgentToControl_Heartbeat{Heartbeat: &agentv1pb.Heartbeat{SessionId: helloAck.SessionId}},
	}))
	_, err = stream.Recv()
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
	_, _, err = openStream(t, ctx, forwardConn, uint32(l.forward.ID))
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
	_, _, err = enrollForTest(t, nodeCredentials(ctx, l.forwardNode(), l.forwardToken), anonymous, "")
	assert.Equal(t, codes.Unauthenticated, status.Code(err), "the old token no longer enrolls")

	// Disabling a proxy node revokes its certificates.
	proxyCertificate, _, err := enrollForTest(t, nodeCredentials(ctx, l.proxyNode(), l.proxyKey), anonymous, "")
	require.NoError(t, err)
	proxyConn := l.dial(t, proxyCertificate)
	_, _, err = openStream(t, ctx, proxyConn, uint32(l.proxy.ID))
	require.NoError(t, err)
	require.NoError(t, service.NewNodeService().UpdateNode(l.proxy.ID, map[string]any{"status": model.NodeStatusDisabled}))
	_, _, err = openStream(t, ctx, proxyConn, uint32(l.proxy.ID))
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
	_, err = pb.NewNodeServiceClient(proxyConn).GetConfig(ctx, &pb.NodeConfigRequest{NodeId: uint32(l.proxy.ID)})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))

	// The kernel's own certificate chains to the same CA but is no agent.
	source, err := l.authority.KernelTLS(ctx, time.Minute)
	require.NoError(t, err)
	kernelCertificate, err := source.Certificate()
	require.NoError(t, err)
	_, _, err = openStream(t, ctx, l.dial(t, kernelCertificate), uint32(l.proxy.ID))
	assert.Equal(t, codes.Unauthenticated, status.Code(err))

	// A certificate from another CA is refused, never downgraded to the
	// legacy credential.
	otherKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(7), NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	identity, err := agentcontrol.NewAgentIdentity("test", agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: 1})
	require.NoError(t, err)
	template.URIs = append(template.URIs, identity.URL())
	der, err := x509.CreateCertificate(rand.Reader, template, template, &otherKey.PublicKey, otherKey)
	require.NoError(t, err)
	selfSigned := &tls.Certificate{Certificate: [][]byte{der}, PrivateKey: otherKey}
	_, _, err = openStream(t, legacyCredentials(ctx, l.proxy.ID, l.proxyKey), l.dial(t, selfSigned), uint32(l.proxy.ID))
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}
