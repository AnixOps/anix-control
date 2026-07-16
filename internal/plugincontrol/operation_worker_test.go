package plugincontrol

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v3/internal/model"
	"github.com/AnixOps/anix-control/v3/internal/service"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type lifecycleTestExecutor struct {
	id        string
	version   string
	calls     []LifecycleRequest
	failKinds map[string]error
}

type blockingLifecycleExecutor struct {
	id      string
	version string
	started chan struct{}
	release chan struct{}
}

func (e *blockingLifecycleExecutor) PluginID() string { return e.id }
func (e *blockingLifecycleExecutor) Version() string  { return e.version }
func (e *blockingLifecycleExecutor) HandleRoute(context.Context, RouteRequest) (RouteResponse, error) {
	return RouteResponse{Status: http.StatusOK}, nil
}
func (e *blockingLifecycleExecutor) ExecuteLifecycle(ctx context.Context, _ LifecycleRequest) (json.RawMessage, error) {
	close(e.started)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-e.release:
		return json.RawMessage(`{"ok":true}`), nil
	}
}

func (e *lifecycleTestExecutor) PluginID() string { return e.id }
func (e *lifecycleTestExecutor) Version() string  { return e.version }
func (e *lifecycleTestExecutor) HandleRoute(context.Context, RouteRequest) (RouteResponse, error) {
	return RouteResponse{Status: http.StatusOK, Data: map[string]string{"version": e.version}}, nil
}
func (e *lifecycleTestExecutor) ExecuteLifecycle(_ context.Context, request LifecycleRequest) (json.RawMessage, error) {
	e.calls = append(e.calls, request)
	if err := e.failKinds[request.Kind]; err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{"kind": request.Kind, "version": e.version})
}

func newOperationWorkerDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, service.EnsureKernelSchema(db))
	return db
}

func seedControlPluginRelease(t *testing.T, db *gorm.DB, pluginID, version string) {
	t.Helper()
	plugin := model.Plugin{ID: pluginID, Name: pluginID, Publisher: "AnixOps", Official: true}
	require.NoError(t, db.Where("id = ?", pluginID).FirstOrCreate(&plugin).Error)
	manifest := service.PluginManifest{
		ID: pluginID, Name: pluginID, Version: version, APIVersion: "v1", Publisher: "AnixOps",
		Targets: []string{"control"}, ArtifactSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}
	canonical, err := service.CanonicalPluginManifest(manifest)
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.PluginRelease{
		PluginID: pluginID, Version: version, APIVersion: "v1", ManifestJSON: string(canonical),
		ArtifactSHA256: manifest.ArtifactSHA256, Signature: "test-signature", PublishedAt: time.Now(),
	}).Error)
}

func createControlOperation(t *testing.T, db *gorm.DB, pluginID, version, kind, key string, deadline time.Time) model.KernelOperation {
	t.Helper()
	operation, _, err := service.CreateKernelOperation(db, model.KernelOperation{
		ID: uuid.NewString(), IdempotencyKey: key, PluginID: pluginID, TargetVersion: version,
		Kind: kind, ConfigJSON: `{"interval_seconds":30}`, DeadlineAt: &deadline,
	})
	require.NoError(t, err)
	return *operation
}

func TestOperationWorkerPersistsLifecycleUpdateRollbackAndRestartReplay(t *testing.T) {
	db := newOperationWorkerDB(t)
	const pluginID = "worker-reference"
	seedControlPluginRelease(t, db, pluginID, "1.0.0")
	seedControlPluginRelease(t, db, pluginID, "2.0.0")
	v1 := &lifecycleTestExecutor{id: pluginID, version: "1.0.0", failKinds: map[string]error{}}
	v2 := &lifecycleTestExecutor{id: pluginID, version: "2.0.0", failKinds: map[string]error{}}
	registry, err := NewRegistry(v1, v2)
	require.NoError(t, err)
	worker, err := NewOperationWorker(db, registry)
	require.NoError(t, err)
	now := time.Unix(1_800_000_000, 0).UTC()
	worker.now = func() time.Time { return now }

	installation := model.PluginInstallation{
		PluginID: pluginID, Target: "control", DesiredVersion: "1.0.0", State: "pending", Enabled: true,
	}
	require.NoError(t, db.Create(&installation).Error)
	deadline := now.Add(time.Minute)
	enable := createControlOperation(t, db, pluginID, "1.0.0", "plugin.enable", "worker-enable-v1", deadline)

	count, err := worker.RunOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, count)
	require.NoError(t, db.First(&enable, "id = ?", enable.ID).Error)
	require.Equal(t, "succeeded", enable.State)
	require.NotNil(t, enable.DispatchedAt)
	require.NotNil(t, enable.AcknowledgedAt)
	require.NotNil(t, enable.ObservedAt)
	require.NoError(t, db.First(&installation, installation.ID).Error)
	require.Equal(t, "healthy", installation.State)
	require.Equal(t, "1.0.0", installation.ObservedVersion)
	require.Len(t, v1.calls, 1)

	count, err = worker.RunOnce(context.Background())
	require.NoError(t, err)
	require.Zero(t, count)
	require.Len(t, v1.calls, 1)

	require.NoError(t, db.Model(&installation).Updates(map[string]any{"desired_version": "2.0.0", "state": "pending"}).Error)
	update := createControlOperation(t, db, pluginID, "2.0.0", "plugin.update", "worker-update-v2", deadline)
	count, err = worker.RunOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, count)
	require.NoError(t, db.First(&update, "id = ?", update.ID).Error)
	require.Equal(t, "succeeded", update.State)
	require.NoError(t, db.First(&installation, installation.ID).Error)
	require.Equal(t, "2.0.0", installation.ObservedVersion)
	require.Equal(t, "1.0.0", installation.PreviousVersion)

	require.NoError(t, db.Model(&installation).Updates(map[string]any{"desired_version": "1.0.0", "state": "pending"}).Error)
	rollback := createControlOperation(t, db, pluginID, "1.0.0", "plugin.rollback", "worker-rollback-v1", deadline)
	count, err = worker.RunOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, count)
	require.NoError(t, db.First(&rollback, "id = ?", rollback.ID).Error)
	require.Equal(t, "succeeded", rollback.State)
	require.NoError(t, db.First(&installation, installation.ID).Error)
	require.Equal(t, "1.0.0", installation.DesiredVersion)
	require.Equal(t, "1.0.0", installation.ObservedVersion)
	require.Equal(t, "2.0.0", installation.PreviousVersion)

	queued, err := worker.QueueReconciliation("boot-two")
	require.NoError(t, err)
	require.Equal(t, 1, queued)
	queued, err = worker.QueueReconciliation("boot-two")
	require.NoError(t, err)
	require.Zero(t, queued)
	count, err = worker.RunOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, count)
	require.Equal(t, "plugin.enable", v1.calls[len(v1.calls)-1].Kind)

	stale := createControlOperation(t, db, pluginID, "1.0.0", "plugin.health", "worker-stale-health", deadline)
	require.NoError(t, db.Model(&model.KernelOperation{}).Where("id = ?", stale.ID).Updates(map[string]any{
		"state": "running", "updated_at": now.Add(-time.Minute),
	}).Error)
	count, err = worker.RunOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, count)
	require.NoError(t, db.First(&stale, "id = ?", stale.ID).Error)
	require.Equal(t, "succeeded", stale.State)
}

func TestOperationWorkerAutomaticallyRestoresPreviousVersionAfterFailedUpdate(t *testing.T) {
	db := newOperationWorkerDB(t)
	const pluginID = "worker-rollback"
	seedControlPluginRelease(t, db, pluginID, "1.0.0")
	seedControlPluginRelease(t, db, pluginID, "2.0.0")
	v1 := &lifecycleTestExecutor{id: pluginID, version: "1.0.0", failKinds: map[string]error{}}
	v2 := &lifecycleTestExecutor{id: pluginID, version: "2.0.0", failKinds: map[string]error{"plugin.update": errors.New("new version unhealthy")}}
	registry, err := NewRegistry(v1, v2)
	require.NoError(t, err)
	worker, err := NewOperationWorker(db, registry)
	require.NoError(t, err)
	now := time.Unix(1_800_000_000, 0).UTC()
	worker.now = func() time.Time { return now }

	installation := model.PluginInstallation{
		PluginID: pluginID, Target: "control", DesiredVersion: "2.0.0", ObservedVersion: "1.0.0",
		PreviousVersion: "1.0.0", State: "pending", Enabled: true,
	}
	require.NoError(t, db.Create(&installation).Error)
	deadline := now.Add(time.Minute)
	operation := createControlOperation(t, db, pluginID, "2.0.0", "plugin.update", "worker-failed-update", deadline)

	count, err := worker.RunOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, count)
	require.NoError(t, db.First(&operation, "id = ?", operation.ID).Error)
	require.Equal(t, "failed", operation.State)
	require.Contains(t, operation.LastError, "new version unhealthy")
	require.NoError(t, db.First(&installation, installation.ID).Error)
	require.Equal(t, "healthy", installation.State)
	require.Equal(t, "1.0.0", installation.ObservedVersion)
	require.Equal(t, "1.0.0", installation.DesiredVersion)
	require.Equal(t, "2.0.0", installation.PreviousVersion)
	require.NotEmpty(t, installation.LastError)
	require.Len(t, v1.calls, 1)
	require.Equal(t, "plugin.enable", v1.calls[0].Kind)
}

func TestOperationWorkerDoesNotStealAnUnexpiredLease(t *testing.T) {
	db := newOperationWorkerDB(t)
	const pluginID = "worker-lease"
	seedControlPluginRelease(t, db, pluginID, "1.0.0")
	executor := &lifecycleTestExecutor{id: pluginID, version: "1.0.0", failKinds: map[string]error{}}
	registry, err := NewRegistry(executor)
	require.NoError(t, err)
	worker, err := NewOperationWorker(db, registry)
	require.NoError(t, err)
	current := time.Unix(1_800_000_000, 0).UTC()
	worker.now = func() time.Time { return current }
	installation := model.PluginInstallation{PluginID: pluginID, Target: "control", DesiredVersion: "1.0.0", State: "pending", Enabled: true}
	require.NoError(t, db.Create(&installation).Error)
	deadline := current.Add(time.Minute)
	operation := createControlOperation(t, db, pluginID, "1.0.0", "plugin.enable", "worker-lease-enable", deadline)
	lease := current.Add(20 * time.Second)
	require.NoError(t, db.Model(&operation).Updates(map[string]any{
		"state": "running", "claimed_by": "other-control", "lease_expires_at": lease,
	}).Error)

	count, err := worker.RunOnce(context.Background())
	require.NoError(t, err)
	require.Zero(t, count)
	require.Empty(t, executor.calls)

	current = current.Add(21 * time.Second)
	count, err = worker.RunOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, count)
	require.Len(t, executor.calls, 1)
	require.NoError(t, db.First(&operation, "id = ?", operation.ID).Error)
	require.Equal(t, "succeeded", operation.State)
	require.Equal(t, 1, operation.Attempt)
}

func TestOperationWorkerSerializesReconciliationBehindExistingLease(t *testing.T) {
	db := newOperationWorkerDB(t)
	const pluginID = "worker-reconcile-order"
	seedControlPluginRelease(t, db, pluginID, "1.0.0")
	executor := &lifecycleTestExecutor{id: pluginID, version: "1.0.0", failKinds: map[string]error{}}
	registry, err := NewRegistry(executor)
	require.NoError(t, err)
	worker, err := NewOperationWorker(db, registry)
	require.NoError(t, err)
	current := time.Unix(1_800_000_000, 0).UTC()
	worker.now = func() time.Time { return current }
	installation := model.PluginInstallation{
		PluginID: pluginID, Target: "control", DesiredVersion: "1.0.0", State: "healthy", Enabled: true,
	}
	require.NoError(t, db.Create(&installation).Error)
	deadline := current.Add(time.Minute)
	oldOperation := createControlOperation(t, db, pluginID, "1.0.0", "plugin.enable", "worker-old-enable", deadline)
	lease := current.Add(20 * time.Second)
	require.NoError(t, db.Model(&oldOperation).Updates(map[string]any{
		"state": "running", "claimed_by": "previous-control", "lease_expires_at": lease,
	}).Error)

	queued, err := worker.QueueReconciliation("new-control-boot")
	require.NoError(t, err)
	require.Equal(t, 1, queued)
	count, err := worker.RunOnce(context.Background())
	require.NoError(t, err)
	require.Zero(t, count)
	require.Empty(t, executor.calls)

	current = current.Add(21 * time.Second)
	count, err = worker.RunOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, 2, count)
	require.Len(t, executor.calls, 2)
	require.Equal(t, oldOperation.ID, executor.calls[0].OperationID)
	require.NotEqual(t, executor.calls[0].OperationID, executor.calls[1].OperationID)
}

func TestOperationWorkerCancelsExecutorWhenLeaseOwnershipIsLost(t *testing.T) {
	db := newOperationWorkerDB(t)
	const pluginID = "worker-lease-loss"
	seedControlPluginRelease(t, db, pluginID, "1.0.0")
	executor := &blockingLifecycleExecutor{
		id: pluginID, version: "1.0.0", started: make(chan struct{}), release: make(chan struct{}),
	}
	registry, err := NewRegistry(executor)
	require.NoError(t, err)
	worker, err := NewOperationWorker(db, registry)
	require.NoError(t, err)
	worker.leaseDuration = 60 * time.Millisecond
	installation := model.PluginInstallation{PluginID: pluginID, Target: "control", DesiredVersion: "1.0.0", State: "pending", Enabled: true}
	require.NoError(t, db.Create(&installation).Error)
	deadline := time.Now().Add(time.Minute)
	operation := createControlOperation(t, db, pluginID, "1.0.0", "plugin.enable", "worker-lease-loss-enable", deadline)
	runResult := make(chan error, 1)
	go func() {
		_, runErr := worker.RunOnce(context.Background())
		runResult <- runErr
	}()
	select {
	case <-executor.started:
	case <-time.After(time.Second):
		t.Fatal("control plugin executor did not start")
	}
	require.NoError(t, db.Model(&model.KernelOperation{}).Where("id = ?", operation.ID).Update("claimed_by", "replacement-control").Error)
	select {
	case runErr := <-runResult:
		require.ErrorIs(t, runErr, errControlOperationLeaseLost)
	case <-time.After(time.Second):
		t.Fatal("lease loss did not cancel the control plugin executor")
	}
	require.NoError(t, db.First(&operation, "id = ?", operation.ID).Error)
	require.Equal(t, "running", operation.State)
	require.Equal(t, "replacement-control", operation.ClaimedBy)
}

func TestOperationWorkerCancellationCannotBeOverwrittenByLateSuccess(t *testing.T) {
	db := newOperationWorkerDB(t)
	const pluginID = "worker-cancel"
	seedControlPluginRelease(t, db, pluginID, "1.0.0")
	executor := &blockingLifecycleExecutor{
		id: pluginID, version: "1.0.0", started: make(chan struct{}), release: make(chan struct{}),
	}
	registry, err := NewRegistry(executor)
	require.NoError(t, err)
	worker, err := NewOperationWorker(db, registry)
	require.NoError(t, err)
	installation := model.PluginInstallation{PluginID: pluginID, Target: "control", DesiredVersion: "1.0.0", State: "pending", Enabled: true}
	require.NoError(t, db.Create(&installation).Error)
	deadline := time.Now().Add(time.Minute)
	operation := createControlOperation(t, db, pluginID, "1.0.0", "plugin.enable", "worker-cancel-enable", deadline)
	runResult := make(chan error, 1)
	go func() {
		_, runErr := worker.RunOnce(context.Background())
		runResult <- runErr
	}()
	select {
	case <-executor.started:
	case <-time.After(time.Second):
		t.Fatal("control plugin executor did not start")
	}
	cancelled, err := service.CancelKernelOperation(db, operation.ID, time.Now())
	require.NoError(t, err)
	require.Equal(t, "cancel_requested", cancelled.State)
	close(executor.release)
	require.NoError(t, <-runResult)

	require.NoError(t, db.First(&operation, "id = ?", operation.ID).Error)
	require.Equal(t, "cancelled", operation.State)
	require.NoError(t, db.First(&installation, installation.ID).Error)
	require.Equal(t, "pending", installation.State)
	require.Empty(t, installation.ObservedVersion)
}
