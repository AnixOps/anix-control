package bridgecontract

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	modulepkiv1 "github.com/AnixOps/anix-control/sdk/api/modulepki/v1"
	"github.com/AnixOps/anix-control/sdk/moduletls"
	"github.com/AnixOps/anix-control/sdk/packagebridgesdk"
	"github.com/AnixOps/anix-control/v4/internal/kernelnodeops"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/modulepki"
	"github.com/AnixOps/anix-control/v4/internal/moduleruntime"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var nodeOpsGorm = &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), DisableForeignKeyConstraintWhenMigrating: true}

// nodeOpsDatabases runs body on SQLite and, with ANIX_TEST_POSTGRES_DSN, on
// a throwaway PostgreSQL schema, each holding the ledger and proxy nodes 1
// and 2.
func nodeOpsDatabases(t *testing.T, body func(t *testing.T, db *gorm.DB)) {
	t.Helper()
	t.Run("sqlite", func(t *testing.T) {
		db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "kernel.db")+"?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)"), nodeOpsGorm)
		require.NoError(t, err)
		sqlDB, err := db.DB()
		require.NoError(t, err)
		t.Cleanup(func() { _ = sqlDB.Close() })
		seedNodeOps(t, db)
		body(t, db)
	})
	t.Run("postgres", func(t *testing.T) {
		base := strings.TrimSpace(os.Getenv("ANIX_TEST_POSTGRES_DSN"))
		if base == "" {
			t.Skip("ANIX_TEST_POSTGRES_DSN is not set")
		}
		admin, err := gorm.Open(postgres.Open(base), nodeOpsGorm)
		require.NoError(t, err)
		adminDB, err := admin.DB()
		require.NoError(t, err)
		t.Cleanup(func() { _ = adminDB.Close() })
		var databaseName string
		require.NoError(t, admin.Raw("SELECT current_database()").Scan(&databaseName).Error)
		if !strings.Contains(strings.ToLower(databaseName), "test") && os.Getenv("ANIX_TEST_POSTGRES_ALLOW_UNSAFE") != "1" {
			t.Skipf("refusing to run destructive postgres test against database %q", databaseName)
		}
		suffix := make([]byte, 4)
		_, err = rand.Read(suffix)
		require.NoError(t, err)
		schema := "bridge_nodeops_" + hex.EncodeToString(suffix)
		require.NoError(t, admin.Exec(`CREATE SCHEMA "`+schema+`"`).Error)
		t.Cleanup(func() { _ = admin.Exec(`DROP SCHEMA IF EXISTS "` + schema + `" CASCADE`).Error })
		db, err := gorm.Open(postgres.Open(base+" search_path="+schema), nodeOpsGorm)
		require.NoError(t, err)
		sqlDB, err := db.DB()
		require.NoError(t, err)
		t.Cleanup(func() { _ = sqlDB.Close() })
		seedNodeOps(t, db)
		body(t, db)
	})
}

func seedNodeOps(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.AutoMigrate(append(model.KernelNodeOperationModels(), &model.Node{})...))
	require.NoError(t, db.Create(&[]model.Node{{ID: 1, Name: "edge-1", APIKey: "key-1"}, {ID: 2, Name: "edge-2", APIKey: "key-2"}}).Error)
}

// nodeOpsGrants authorizes hosts by package: the families each holds.
type nodeOpsGrants map[string][]string

func (g nodeOpsGrants) AuthorizeCapability(_ context.Context, host packagebridge.HostIdentity, capability string) error {
	for _, held := range g[host.PackageID] {
		if held == capability {
			return nil
		}
	}
	return service.ErrCapabilityNotAuthorized
}

// nodeOpsKernel is the real KernelNodeOps server on db, with a node.sync
// executor that waits for its release and an engine running its dispatcher.
type nodeOpsKernel struct {
	server  *kernelnodeops.Server
	release chan struct{}
}

func newNodeOpsKernel(t *testing.T, db *gorm.DB, packageID string) *nodeOpsKernel {
	t.Helper()
	registry := kernelnodeops.NewRegistry()
	kernel := &nodeOpsKernel{release: make(chan struct{}, 8)}
	require.NoError(t, registry.Register(kernelnodeops.KindNodeSync, kernelnodeops.ExecutorFunc(
		func(ctx context.Context, run *kernelnodeops.Run) kernelnodeops.Outcome {
			if err := run.Accept(ctx, kernelnodeops.Acceptance{Channel: kernelnodeopsv1.Channel_CHANNEL_AGENT_CONTROL, NodeRevision: 4}); err != nil {
				return kernelnodeops.Cancelled("")
			}
			select {
			case <-kernel.release:
			case <-ctx.Done():
				return kernelnodeops.Cancelled("")
			}
			run.UseSecret("node-key-secret")
			return kernelnodeops.Succeeded(&kernelnodeopsv1.OperationResult{Result: &kernelnodeopsv1.OperationResult_NodeSync{
				NodeSync: &kernelnodeopsv1.NodeSyncResult{Channel: kernelnodeopsv1.Channel_CHANNEL_AGENT_CONTROL, Revision: 4,
					ConfigHash: "sha256:abc", Changed: true, Ack: &kernelnodeopsv1.AgentAck{Accepted: true, Error: "echo node-key-secret"}},
			}})
		})))
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
	kernel.server = &kernelnodeops.Server{Engine: engine, Authorizer: nodeOpsGrants{packageID: {service.CapabilityNodeOpsNodeConfig}}}
	return kernel
}

func syncProxy(id uint64) *kernelnodeopsv1.OperationSpec {
	return &kernelnodeopsv1.OperationSpec{Operation: &kernelnodeopsv1.OperationSpec_SyncNode{SyncNode: &kernelnodeopsv1.SyncNode{
		Node: &kernelnodeopsv1.NodeRef{Kind: kernelnodeopsv1.NodeKind_NODE_KIND_PROXY, Id: id}, Force: true,
	}}}
}

// mintBinding mints a live bridge capability on a host's generation, as the
// kernel does for each dispatch: the request binding a native route passes.
func mintBinding(t *testing.T, minter interface {
	Mint(packagebridge.Request) ([]byte, error)
}, packageID string) []byte {
	t.Helper()
	capability, err := minter.Mint(packagebridge.Request{
		RequestID: "req-1", RouteID: "proxy.admin.nodes.id.sync", Method: "POST",
		PrincipalJSON: []byte(`{"actor_id":1,"admin":true,"package_id":"` + packageID + `"}`), Deadline: time.Now().Add(time.Minute),
	})
	require.NoError(t, err)
	return capability
}

// exerciseNodeOps runs the contract through an SDK client: capabilities, a
// submission bound to a live request that waits for its end, its repeat,
// polling, listing, the watch stream, cancellation and the refusals.
func exerciseNodeOps(t *testing.T, kernel *nodeOpsKernel, connection grpc.ClientConnInterface, packageID string, generation uint64, binding []byte) {
	t.Helper()
	client := kernelnodeopsv1.NewKernelNodeOpsClient(connection)
	ctx := context.Background()

	_, err := client.SubmitOperation(ctx, &kernelnodeopsv1.SubmitOperationRequest{
		RequestId: "node.sync:proxy-1:forged", Operation: syncProxy(1),
		Request: &kernelnodeopsv1.RequestBinding{BridgeCapability: make([]byte, 32)},
	})
	require.Equal(t, codes.PermissionDenied, status.Code(err), "a binding that names no live request of the package")

	capabilities, err := client.GetCapabilities(ctx, &kernelnodeopsv1.GetCapabilitiesRequest{})
	require.NoError(t, err)
	require.Equal(t, []kernelnodeopsv1.OperationFamily{kernelnodeopsv1.OperationFamily_OPERATION_FAMILY_NODE_CONFIG}, capabilities.GetGrantedFamilies())
	require.Equal(t, []string{kernelnodeops.KindNodeSync}, capabilities.GetKinds())
	require.Len(t, capabilities.GetTables(), 7)

	watchCtx, stopWatch := context.WithCancel(ctx)
	defer stopWatch()
	stream, err := client.WatchOperations(watchCtx, &kernelnodeopsv1.WatchOperationsRequest{})
	require.NoError(t, err)

	kernel.release <- struct{}{}
	submitted, err := client.SubmitOperation(ctx, &kernelnodeopsv1.SubmitOperationRequest{
		RequestId: "node.sync:proxy-1:bridge", Operation: syncProxy(1), Wait: kernelnodeopsv1.WaitMode_WAIT_MODE_TERMINAL,
		Reason: "protocol changed", Request: &kernelnodeopsv1.RequestBinding{BridgeCapability: binding},
	})
	require.NoError(t, err)
	require.True(t, submitted.GetApplied())
	operation := submitted.GetOperation()
	require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_SUCCEEDED, operation.GetState())
	require.Equal(t, packageID, operation.GetPackageId())
	require.Equal(t, generation, operation.GetPackageGeneration())
	require.Equal(t, kernelnodeopsv1.Channel_CHANNEL_AGENT_CONTROL, operation.GetChannel())
	require.EqualValues(t, 4, operation.GetNodeRevision())
	require.Equal(t, "sha256:abc", operation.GetResult().GetNodeSync().GetConfigHash())
	require.Equal(t, "echo ********", operation.GetResult().GetNodeSync().GetAck().GetError(), "the credential the operation used is scrubbed")

	repeat, err := client.SubmitOperation(ctx, &kernelnodeopsv1.SubmitOperationRequest{RequestId: "node.sync:proxy-1:bridge", Operation: syncProxy(1)})
	require.NoError(t, err)
	require.False(t, repeat.GetApplied())
	require.Equal(t, operation.GetOperationId(), repeat.GetOperation().GetOperationId())
	_, err = client.SubmitOperation(ctx, &kernelnodeopsv1.SubmitOperationRequest{RequestId: "node.sync:proxy-1:bridge", Operation: syncProxy(2)})
	require.Equal(t, codes.FailedPrecondition, status.Code(err))

	for {
		event, err := stream.Recv()
		require.NoError(t, err)
		require.Equal(t, kernelnodeopsv1.OperationEventKind_OPERATION_EVENT_KIND_CHANGED, event.GetKind())
		require.Equal(t, operation.GetOperationId(), event.GetOperation().GetOperationId())
		if event.GetOperation().GetState() == kernelnodeopsv1.OperationState_OPERATION_STATE_SUCCEEDED {
			break
		}
	}

	byRequest, err := client.GetOperation(ctx, &kernelnodeopsv1.GetOperationRequest{
		Selector: &kernelnodeopsv1.GetOperationRequest_RequestId{RequestId: "node.sync:proxy-1:bridge"},
	})
	require.NoError(t, err)
	require.Equal(t, operation.GetOperationId(), byRequest.GetOperation().GetOperationId())

	pending, err := client.SubmitOperation(ctx, &kernelnodeopsv1.SubmitOperationRequest{
		RequestId: "node.sync:proxy-2:bridge", Operation: syncProxy(2), Wait: kernelnodeopsv1.WaitMode_WAIT_MODE_ACCEPTED,
	})
	require.NoError(t, err)
	require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_RUNNING, pending.GetOperation().GetState())
	cancelled, err := client.CancelOperation(ctx, &kernelnodeopsv1.CancelOperationRequest{OperationId: pending.GetOperation().GetOperationId(), Reason: "node deleted"})
	require.NoError(t, err)
	require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_CANCELLED, cancelled.GetOperation().GetState())

	list, err := client.ListOperations(ctx, &kernelnodeopsv1.ListOperationsRequest{
		Target: &kernelnodeopsv1.NodeRef{Kind: kernelnodeopsv1.NodeKind_NODE_KIND_PROXY, Id: 2},
	})
	require.NoError(t, err)
	require.Len(t, list.GetOperations(), 1)
	require.Equal(t, "node.sync:proxy-2:bridge", list.GetOperations()[0].GetRequestId())

	_, err = client.SubmitOperation(ctx, &kernelnodeopsv1.SubmitOperationRequest{RequestId: "node.retire:proxy-2",
		Operation: &kernelnodeopsv1.OperationSpec{Operation: &kernelnodeopsv1.OperationSpec_RetireNode{RetireNode: &kernelnodeopsv1.RetireNode{
			Node: &kernelnodeopsv1.NodeRef{Kind: kernelnodeopsv1.NodeKind_NODE_KIND_PROXY, Id: 2},
		}}}})
	require.Equal(t, codes.Unimplemented, status.Code(err), "a kind this kernel does not execute")
	_, err = client.SubmitOperation(ctx, &kernelnodeopsv1.SubmitOperationRequest{RequestId: "forward.apply:1",
		Operation: &kernelnodeopsv1.OperationSpec{Operation: &kernelnodeopsv1.OperationSpec_ApplyForward{ApplyForward: &kernelnodeopsv1.ApplyForward{
			ForwardId: 1, Action: kernelnodeopsv1.ForwardAction_FORWARD_ACTION_SYNC,
		}}}})
	require.Equal(t, codes.PermissionDenied, status.Code(err), "a family the package does not hold")
	_, err = client.ValidateNodeConfig(ctx, &kernelnodeopsv1.ValidateNodeConfigRequest{})
	require.Equal(t, codes.Unimplemented, status.Code(err), "NO-5 serves the validators")
}

// A local host reaches KernelNodeOps over its bridge connection and the
// kernel serves it as the session's host identity.
func TestKernelNodeOpsOverTheLocalBridge(t *testing.T) {
	nodeOpsDatabases(t, func(t *testing.T, db *gorm.DB) {
		kernel := newNodeOpsKernel(t, db, "plan")
		allowlist, err := packagebridge.NewAllowlistWithFallback(nil)
		require.NoError(t, err)
		session, child, err := packagebridge.NewSessionWithOptions(
			packagebridge.HostIdentity{PackageID: "plan", Version: "4.0.0", Generation: 3}, allowlist,
			packagebridge.SessionOptions{KernelNodeOps: kernel.server.For},
		)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, session.Close()) })
		client, err := packagebridgesdk.DialFile(context.Background(), child)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, client.Close()) })
		exerciseNodeOps(t, kernel, client.Conn(), "plan", 3, mintBinding(t, session, "plan"))
	})
}

func TestKernelNodeOpsIsAbsentWithoutAProvider(t *testing.T) {
	client := dialPlanSession(t, packagebridge.SessionOptions{})
	_, err := kernelnodeopsv1.NewKernelNodeOpsClient(client.Conn()).GetCapabilities(context.Background(), &kernelnodeopsv1.GetCapabilitiesRequest{})
	require.Equal(t, codes.Unimplemented, status.Code(err))
}

// nodeOpsBinder binds remote instances of one generation.
type nodeOpsBinder struct {
	mu         sync.Mutex
	generation *packagebridge.GenerationSession
}

func (b *nodeOpsBinder) BindInstance(_ context.Context, binding packagebridge.InstanceBinding) (*packagebridge.GenerationSession, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if identity := b.generation.Identity(); binding.PackageID != identity.PackageID || binding.Version != identity.Version {
		return nil, packagebridge.ErrNotRemote
	}
	return b.generation, nil
}

// A network module reaches KernelNodeOps on the mTLS module listener through
// its SDK bridge client: each call runs as its bound instance's generation,
// and a fenced generation loses the contract.
func TestKernelNodeOpsOverTheModuleListener(t *testing.T) {
	nodeOpsDatabases(t, func(t *testing.T, db *gorm.DB) {
		kernel := newNodeOpsKernel(t, db, "proxy-node")
		pkiDB, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "pki.db")+"?_pragma=busy_timeout(10000)"), nodeOpsGorm)
		require.NoError(t, err)
		require.NoError(t, pkiDB.AutoMigrate(&model.ServiceCA{}, &model.ModuleEnrollment{}, &model.ModuleCertificate{}))
		kek := make([]byte, 32)
		_, err = rand.Read(kek)
		require.NoError(t, err)
		authority, err := modulepki.New(modulepki.Options{DB: pkiDB, Cluster: "prod", KEK: kek})
		require.NoError(t, err)
		require.NoError(t, authority.Ensure(context.Background()))
		kernelTLS, err := authority.KernelTLS(context.Background(), time.Minute)
		require.NoError(t, err)

		allowlist, err := packagebridge.NewAllowlistWithFallback(nil)
		require.NoError(t, err)
		generation, err := packagebridge.NewGenerationSession(packagebridge.HostIdentity{PackageID: "proxy-node", Version: "4.1.0", Generation: 9}, allowlist, packagebridge.SessionOptions{})
		require.NoError(t, err)
		bridge, err := packagebridge.NewModuleBridge(&nodeOpsBinder{generation: generation}, packagebridge.ModuleBridgeOptions{Cluster: "prod"})
		require.NoError(t, err)
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		ctx, cancel := context.WithCancel(context.Background())
		served := make(chan error, 1)
		go func() {
			served <- (&moduleruntime.Listener{TLS: kernelTLS, Cluster: "prod", PKI: authority, Bridge: bridge, KernelNodeOps: kernel.server.For}).Serve(ctx, listener)
		}()
		t.Cleanup(func() {
			cancel()
			require.NoError(t, <-served)
		})

		kernelID, err := moduletls.Kernel("prod")
		require.NoError(t, err)
		dial := func(certificate *tls.Certificate) *tls.Config {
			source := moduletls.Source{Roots: kernelTLS.Roots}
			if certificate != nil {
				source.Certificate = func() (*tls.Certificate, error) { return certificate, nil }
			}
			return source.ClientConfig(moduletls.AcceptExactly(kernelID))
		}
		credential, _, err := authority.CreateEnrollment(context.Background(), modulepki.EnrollmentRequest{PackageID: "proxy-node", TTL: time.Hour})
		require.NoError(t, err)
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)
		csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{}, key)
		require.NoError(t, err)
		anonymous, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(credentials.NewTLS(dial(nil))))
		require.NoError(t, err)
		t.Cleanup(func() { _ = anonymous.Close() })
		enrolled, err := modulepkiv1.NewModulePKIClient(anonymous).Enroll(context.Background(), &modulepkiv1.EnrollRequest{
			EnrollmentCredential: credential, PackageId: "proxy-node", Cluster: "prod", CsrDer: csr,
		})
		require.NoError(t, err)
		certificate := &tls.Certificate{Certificate: [][]byte{enrolled.GetCertificate().GetCertificateDer()}, PrivateKey: key}

		_, err = kernelnodeopsv1.NewKernelNodeOpsClient(anonymous).GetCapabilities(context.Background(), &kernelnodeopsv1.GetCapabilitiesRequest{})
		require.Equal(t, codes.Unauthenticated, status.Code(err), "no client certificate")

		module, err := packagebridgesdk.DialNetwork(packagebridgesdk.NetworkConfig{
			KernelAddr: listener.Addr().String(), TLS: dial(certificate), PackageID: "proxy-node", PackageVersion: "4.1.0",
			InstanceID: "pod-a", AdvertiseAddr: "10.0.0.7:7000", LeaseID: "lease-a",
		})
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, module.Close()) })
		require.NoError(t, module.Bind(context.Background()))

		exerciseNodeOps(t, kernel, module.Conn(), "proxy-node", 9, mintBinding(t, generation, "proxy-node"))

		require.NoError(t, generation.Close())
		fenced := kernelnodeopsv1.NewKernelNodeOpsClient(module.Conn())
		_, err = fenced.GetCapabilities(context.Background(), &kernelnodeopsv1.GetCapabilitiesRequest{})
		require.Equal(t, codes.PermissionDenied, status.Code(err), "a fenced generation loses KernelNodeOps")
		stream, err := fenced.WatchOperations(context.Background(), &kernelnodeopsv1.WatchOperationsRequest{})
		require.NoError(t, err)
		_, err = stream.Recv()
		require.Equal(t, codes.PermissionDenied, status.Code(err), "streaming calls too")
	})
}

// A binding is verified against the host the contract serves: a server for
// another generation than the session's refuses the session's bindings.
func TestKernelNodeOpsRefusesABindingOfAnotherGeneration(t *testing.T) {
	nodeOpsDatabases(t, func(t *testing.T, db *gorm.DB) {
		kernel := newNodeOpsKernel(t, db, "plan")
		allowlist, err := packagebridge.NewAllowlistWithFallback(nil)
		require.NoError(t, err)
		session, child, err := packagebridge.NewSessionWithOptions(
			packagebridge.HostIdentity{PackageID: "plan", Version: "4.0.0", Generation: 3}, allowlist,
			packagebridge.SessionOptions{KernelNodeOps: func(packagebridge.HostIdentity) kernelnodeopsv1.KernelNodeOpsServer {
				return kernel.server.For(packagebridge.HostIdentity{PackageID: "plan", Version: "4.0.0", Generation: 4})
			}},
		)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, session.Close()) })
		client, err := packagebridgesdk.DialFile(context.Background(), child)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, client.Close()) })
		_, err = kernelnodeopsv1.NewKernelNodeOpsClient(client.Conn()).SubmitOperation(context.Background(), &kernelnodeopsv1.SubmitOperationRequest{
			RequestId: "node.sync:proxy-1:other", Operation: syncProxy(1),
			Request: &kernelnodeopsv1.RequestBinding{BridgeCapability: mintBinding(t, session, "plan")},
		})
		require.Equal(t, codes.PermissionDenied, status.Code(err))
	})
}
