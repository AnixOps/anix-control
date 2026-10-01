package bridgecontract

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/v4/internal/kernelnodeops"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// seedForward adds forward node 10 with an API token, tunnel 30 on it and
// forward 40, with the runtime on the gost backend through nodex.
func seedForward(t *testing.T, db *gorm.DB, nodex string) {
	t.Helper()
	require.NoError(t, db.AutoMigrate(&model.ForwardNode{}, &model.ForwardTunnel{}, &model.Forward{}, &model.ForwardRuntimeJob{}, &model.SystemConfig{}, &model.ForwardUserTunnel{}, &model.SpeedLimit{}))
	require.NoError(t, service.EnsureForwardRuntimeJobSchema(db))
	require.NoError(t, db.Create(&model.ForwardNode{ID: 10, Name: "relay", Type: model.ForwardNodeTypeRelay, Host: "198.51.100.10", Port: 443, APIPort: 18080, APIToken: "relay-token", Enabled: true}).Error)
	require.NoError(t, db.Create(&model.ForwardTunnel{ID: 30, Name: "tunnel", InNodeID: 10, Type: 1}).Error)
	require.NoError(t, db.Create(&model.Forward{ID: 40, UserID: 1, Name: "web", TunnelID: 30, InPort: 1000, RemoteAddr: "203.0.113.1:80"}).Error)
	configs := service.NewSystemConfigService(db)
	for key, value := range map[string]string{
		"forward.runtime_backend": model.ForwardRuntimeBackendGost, "forward.runtime.nodex.base_url": nodex, "forward.runtime.nodex.token": "nodex-shared-token",
	} {
		require.NoError(t, configs.Set(key, value, "string", "forward", "bridge test"))
	}
}

// A package reaches the forward kinds through its bridge: GetCapabilities
// lists them, a forward.apply submitted with a terminal wait answers the
// NodeX result, a repeat answers the receipt, and the job it recorded holds
// no token, on SQLite and PostgreSQL.
func TestKernelNodeOpsForwardRoundTrip(t *testing.T) {
	var mu sync.Mutex
	var bodies []string
	nodex := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(body))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"backend":"gost","status":2,"message":"nodex applied","result":"applied"}}`))
	}))
	t.Cleanup(nodex.Close)

	nodeOpsDatabases(t, func(t *testing.T, db *gorm.DB) {
		seedForward(t, db, nodex.URL)
		registry := kernelnodeops.NewRegistry()
		require.NoError(t, (&kernelnodeops.Forward{}).Register(registry))
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
		server := &kernelnodeops.Server{Engine: engine, Authorizer: nodeOpsGrants{"plan": {service.CapabilityNodeOpsForward}}}
		client := kernelnodeopsv1.NewKernelNodeOpsClient(dialPlanSession(t, packagebridge.SessionOptions{KernelNodeOps: server.For}).Conn())
		background := context.Background()

		capabilities, err := client.GetCapabilities(background, &kernelnodeopsv1.GetCapabilitiesRequest{})
		require.NoError(t, err)
		require.Equal(t, []string{kernelnodeops.KindForwardApply, kernelnodeops.KindForwardLegacyRule, kernelnodeops.KindForwardSyncBackend, kernelnodeops.KindForwardTunnel}, capabilities.GetKinds())
		require.Equal(t, []kernelnodeopsv1.OperationFamily{kernelnodeopsv1.OperationFamily_OPERATION_FAMILY_FORWARD}, capabilities.GetGrantedFamilies())

		apply := &kernelnodeopsv1.OperationSpec{Operation: &kernelnodeopsv1.OperationSpec_ApplyForward{ApplyForward: &kernelnodeopsv1.ApplyForward{
			ForwardId: 40, Action: kernelnodeopsv1.ForwardAction_FORWARD_ACTION_CREATE,
		}}}
		submitted, err := client.SubmitOperation(background, &kernelnodeopsv1.SubmitOperationRequest{
			RequestId: "forward.apply:40:create:bridge", Operation: apply, Wait: kernelnodeopsv1.WaitMode_WAIT_MODE_TERMINAL, WaitTimeoutMs: 20000,
		})
		require.NoError(t, err)
		require.True(t, submitted.GetApplied())
		operation := submitted.GetOperation()
		require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_SUCCEEDED, operation.GetState(), "%v", operation.GetError())
		require.Equal(t, kernelnodeops.KindForwardApply, operation.GetKind())
		require.Equal(t, kernelnodeopsv1.Channel_CHANNEL_NODEX, operation.GetChannel())
		result := operation.GetResult().GetForwardApply()
		require.Equal(t, "nodex applied", result.GetMessage())
		require.EqualValues(t, model.ForwardRuntimeJobStatusSuccess, result.GetRuntimeStatus())
		require.NotZero(t, result.GetRuntimeJobId())

		var job model.ForwardRuntimeJob
		require.NoError(t, db.First(&job, result.GetRuntimeJobId()).Error)
		require.Equal(t, model.ForwardRuntimeJobStatusSuccess, job.Status)
		require.NotContains(t, job.Payload, "relay-token", "the stored payload carries no token")
		mu.Lock()
		sent := append([]string(nil), bodies...)
		bodies = nil
		mu.Unlock()
		require.Len(t, sent, 1)
		require.True(t, strings.Contains(sent[0], `"apiToken":"relay-token"`), "NodeX got the token with the request")

		repeat, err := client.SubmitOperation(background, &kernelnodeopsv1.SubmitOperationRequest{RequestId: "forward.apply:40:create:bridge", Operation: apply})
		require.NoError(t, err)
		require.False(t, repeat.GetApplied())
		require.Equal(t, operation.GetOperationId(), repeat.GetOperation().GetOperationId())
		mu.Lock()
		require.Empty(t, bodies, "a repeat applies nothing")
		mu.Unlock()
	})
}
