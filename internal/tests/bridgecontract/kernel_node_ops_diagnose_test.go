package bridgecontract

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/v4/internal/kernelnodeops"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
)

// seedDiagnosis adds forward nodes 10 and 11, a two-hop tunnel 30 between
// them and forward 40 on it, with a public and a loopback target.
func seedDiagnosis(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.AutoMigrate(&model.ForwardNode{}, &model.ForwardTunnel{}, &model.Forward{}, &model.SystemConfig{}))
	exit := uint(11)
	require.NoError(t, db.Create(&[]model.ForwardNode{
		{ID: 10, Name: "relay", Type: model.ForwardNodeTypeRelay, Host: "198.51.100.10", Port: 443, APIPort: 18080, APIToken: "relay-token"},
		{ID: 11, Name: "exit", Type: model.ForwardNodeTypeExit, Host: "198.51.100.11", Port: 443, APIPort: 18080, APIToken: "exit-token"},
	}).Error)
	require.NoError(t, db.Create(&model.ForwardTunnel{ID: 30, Name: "tunnel", InNodeID: 10, OutNodeID: &exit, Type: 2}).Error)
	require.NoError(t, db.Create(&model.Forward{ID: 40, UserID: 1, Name: "web", TunnelID: 30, InPort: 1000, RemoteAddr: "203.0.113.1:80,127.0.0.1:5432"}).Error)
}

// diagnosisDial is the kernel's network in these tests: node 10 and the
// public target connect, everything else is refused.
func diagnosisDial(_ context.Context, _, address string, _ time.Duration) (net.Conn, error) {
	switch address {
	case "198.51.100.10:443", "203.0.113.1:80":
		client, server := net.Pipe()
		_ = server.Close()
		return client, nil
	}
	return nil, fmt.Errorf("dial tcp %s: connect: connection refused", address)
}

// A package reaches the diagnosis kinds through its bridge: GetCapabilities
// lists them, a CheckEndpoints submitted without waiting is followed on the
// watch stream to its end and read back, and a forward diagnosis answers
// its typed result, on SQLite and PostgreSQL.
func TestKernelNodeOpsDiagnosisRoundTrip(t *testing.T) {
	nodeOpsDatabases(t, func(t *testing.T, db *gorm.DB) {
		seedDiagnosis(t, db)
		registry := kernelnodeops.NewRegistry()
		require.NoError(t, (&kernelnodeops.Diagnosis{Probes: service.DiagnosisProbes{Dial: diagnosisDial}}).Register(registry))
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
		server := &kernelnodeops.Server{Engine: engine, Authorizer: nodeOpsGrants{"plan": {service.CapabilityNodeOpsDiagnose}}}
		client := kernelnodeopsv1.NewKernelNodeOpsClient(dialPlanSession(t, packagebridge.SessionOptions{KernelNodeOps: server.For}).Conn())
		background := context.Background()

		capabilities, err := client.GetCapabilities(background, &kernelnodeopsv1.GetCapabilitiesRequest{})
		require.NoError(t, err)
		require.Equal(t, []string{kernelnodeops.KindDiagnoseEndpoints, kernelnodeops.KindDiagnoseForward, kernelnodeops.KindDiagnoseNodeStats, kernelnodeops.KindDiagnoseTunnel}, capabilities.GetKinds())
		require.Equal(t, []kernelnodeopsv1.OperationFamily{kernelnodeopsv1.OperationFamily_OPERATION_FAMILY_DIAGNOSE}, capabilities.GetGrantedFamilies())

		watchCtx, stopWatch := context.WithCancel(background)
		defer stopWatch()
		stream, err := client.WatchOperations(watchCtx, &kernelnodeopsv1.WatchOperationsRequest{})
		require.NoError(t, err)

		check := &kernelnodeopsv1.OperationSpec{Operation: &kernelnodeopsv1.OperationSpec_CheckEndpoints{CheckEndpoints: &kernelnodeopsv1.CheckEndpoints{
			Nodes:        []*kernelnodeopsv1.NodeRef{{Kind: kernelnodeopsv1.NodeKind_NODE_KIND_FORWARD, Id: 10}, {Kind: kernelnodeopsv1.NodeKind_NODE_KIND_FORWARD, Id: 11}},
			RecordStatus: true,
		}}}
		submitted, err := client.SubmitOperation(background, &kernelnodeopsv1.SubmitOperationRequest{RequestId: "diagnose.endpoints:forward-10,11:bridge", Operation: check})
		require.NoError(t, err)
		require.True(t, submitted.GetApplied())
		require.Equal(t, kernelnodeops.KindDiagnoseEndpoints, submitted.GetOperation().GetKind())

		var watched *kernelnodeopsv1.Operation
		for watched == nil {
			event, err := stream.Recv()
			require.NoError(t, err)
			require.Equal(t, kernelnodeopsv1.OperationEventKind_OPERATION_EVENT_KIND_CHANGED, event.GetKind())
			if operation := event.GetOperation(); operation.GetOperationId() == submitted.GetOperation().GetOperationId() &&
				operation.GetState() >= kernelnodeopsv1.OperationState_OPERATION_STATE_SUCCEEDED {
				watched = operation
			}
		}
		require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_SUCCEEDED, watched.GetState(), "%v", watched.GetError())
		require.Equal(t, kernelnodeopsv1.Channel_CHANNEL_CONTROL_DIAL, watched.GetChannel())

		got, err := client.GetOperation(background, &kernelnodeopsv1.GetOperationRequest{
			Selector: &kernelnodeopsv1.GetOperationRequest_RequestId{RequestId: "diagnose.endpoints:forward-10,11:bridge"},
		})
		require.NoError(t, err)
		require.True(t, proto.Equal(watched, got.GetOperation()), "Get answers what the watch delivered")
		checks := got.GetOperation().GetResult().GetEndpointCheck().GetChecks()
		require.Len(t, checks, 2)
		require.True(t, checks[0].GetReachable())
		require.False(t, checks[1].GetReachable())
		require.Equal(t, "dial tcp 198.51.100.11:443: connect: connection refused", checks[1].GetError())
		require.Equal(t, kernelnodeopsv1.Vantage_VANTAGE_CONTROL, checks[0].GetVantage().GetUsed())
		var recorded model.ForwardNode
		require.NoError(t, db.First(&recorded, 11).Error)
		require.Equal(t, model.ForwardNodeStatusOffline, recorded.Status)

		repeat, err := client.SubmitOperation(background, &kernelnodeopsv1.SubmitOperationRequest{RequestId: "diagnose.endpoints:forward-10,11:bridge", Operation: check})
		require.NoError(t, err)
		require.False(t, repeat.GetApplied())
		require.Equal(t, got.GetOperation().GetOperationId(), repeat.GetOperation().GetOperationId())

		diagnosed, err := client.SubmitOperation(background, &kernelnodeopsv1.SubmitOperationRequest{
			RequestId: "diagnose.forward:40:bridge", Wait: kernelnodeopsv1.WaitMode_WAIT_MODE_TERMINAL,
			Operation: &kernelnodeopsv1.OperationSpec{Operation: &kernelnodeopsv1.OperationSpec_DiagnoseForward{DiagnoseForward: &kernelnodeopsv1.DiagnoseForward{ForwardId: 40}}},
		})
		require.NoError(t, err)
		require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_SUCCEEDED, diagnosed.GetOperation().GetState())
		outcomes := diagnosed.GetOperation().GetResult().GetDiagnosis().GetOutcomes()
		require.Len(t, outcomes, 2)
		require.True(t, outcomes[0].GetSuccess())
		require.Equal(t, "203.0.113.1", outcomes[0].GetTargetIp())
		require.False(t, outcomes[1].GetSuccess())
		require.Equal(t, "不能诊断内网或本机地址", outcomes[1].GetMessage(), "the kernel dials no loopback target for a package")
	})
}
