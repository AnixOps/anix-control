package grpc

import (
	"crypto/rand"
	"crypto/tls"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	agentcontrol "github.com/AnixOps/anix-control/sdk/agentcontrol"
	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	"github.com/AnixOps/anix-control/v5/internal/agentpki"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/modulepki"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"gorm.io/gorm"
)

// CrossRepositoryCluster is the SPIFFE cluster of the cross-repository
// listener's agent PKI.
const CrossRepositoryCluster = "cross-repository"

// CrossRepositoryAgentListener is the node-facing listener for the
// cross-repository Agent fixtures: AgentControlService, AgentEnrollment
// and AgentArtifacts over TLS that requests a client certificate, as
// Server.Start serves them, with the agent PKI, the given manager and a
// one-second heartbeat that keeps the release gates fast.
type CrossRepositoryAgentListener struct {
	Address string
	caFile  string
	pki     *agentpki.Service
}

// StartCrossRepositoryAgentListener migrates the agent PKI tables into db
// and starts the listener; the test's cleanup stops it.
func StartCrossRepositoryAgentListener(t *testing.T, db *gorm.DB, manager *AgentControlManager) *CrossRepositoryAgentListener {
	t.Helper()
	require.NoError(t, db.AutoMigrate(&model.ServiceCA{}, &model.AgentEnrollment{}, &model.AgentCertificate{}, &model.OperationLog{}, &model.AgentTransport{}))
	kek := make([]byte, 32)
	_, err := rand.Read(kek)
	require.NoError(t, err)
	authority, err := modulepki.New(modulepki.Options{DB: db, Cluster: CrossRepositoryCluster, KEK: kek})
	require.NoError(t, err)
	require.NoError(t, authority.Ensure(t.Context()))
	pki, err := agentpki.New(agentpki.Options{DB: db, Authority: authority})
	require.NoError(t, err)

	certFile, keyFile, _ := writeServerCertificate(t)
	certificate, err := tls.LoadX509KeyPair(certFile, keyFile)
	require.NoError(t, err)
	server := grpc.NewServer(grpc.Creds(credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{certificate}, ClientAuth: tls.RequestClientCert, MinVersion: tls.VersionTLS12,
	})))
	agents := &AgentAuthenticator{PKI: pki}
	control := NewAgentControlGRPCServer(manager).WithAuthenticator(agents)
	control.heartbeatIntervalSeconds = 1
	agentv1pb.RegisterAgentControlServiceServer(server, control)
	agentv1pb.RegisterAgentEnrollmentServer(server, NewAgentEnrollmentGRPCServer(agents))
	agentv1pb.RegisterAgentArtifactsServer(server, NewAgentArtifactsGRPCServer(agents))
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	served := make(chan error, 1)
	go func() { served <- server.Serve(listener) }()
	t.Cleanup(func() {
		server.Stop()
		if err := <-served; err != nil && err != grpc.ErrServerStopped {
			t.Errorf("cross-repository listener: %v", err)
		}
	})
	return &CrossRepositoryAgentListener{Address: listener.Addr().String(), caFile: certFile, pki: pki}
}

// FixtureFlags are the Agent fixture flags that enroll proxy node nodeID
// with a fresh one-time credential and stream by its certificate.
func (l *CrossRepositoryAgentListener) FixtureFlags(t *testing.T, nodeID uint) []string {
	t.Helper()
	node := agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: uint32(nodeID)}
	credential, _, err := l.pki.CreateEnrollmentToken(t.Context(), agentpki.TokenRequest{Node: node, TTL: time.Hour, Actor: "cross-repository"})
	require.NoError(t, err)
	dir := t.TempDir()
	credentialFile := filepath.Join(dir, "enroll.credential")
	require.NoError(t, os.WriteFile(credentialFile, []byte(credential+"\n"), 0o600))
	return []string{
		"--target", l.Address, "--node-id", strconv.FormatUint(uint64(nodeID), 10),
		"--tls-ca-file", l.caFile, "--server-name", "127.0.0.1",
		"--pki-dir", filepath.Join(dir, "pki"), "--enroll-credential-file", credentialFile,
		"--cluster", CrossRepositoryCluster,
	}
}
