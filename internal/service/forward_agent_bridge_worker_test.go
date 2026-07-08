package service

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

// bridgeStubNodeXClient is a configurable NodeX executor stub for bridge worker tests.
type bridgeStubNodeXClient struct {
	translateCalls int
	translateFn    func(ctx context.Context, sourceJobID uint, nodeID uint, payload nodeXForwardExecuteRequest) (*nodeXBridgeAgentTask, error)
}

func (c *bridgeStubNodeXClient) Execute(ctx context.Context, req nodeXForwardExecuteRequest) (*nodeXForwardExecuteResult, error) {
	return &nodeXForwardExecuteResult{Status: model.ForwardRuntimeJobStatusSuccess}, nil
}

func (c *bridgeStubNodeXClient) Translate(ctx context.Context, sourceJobID uint, nodeID uint, payload nodeXForwardExecuteRequest) (*nodeXBridgeAgentTask, error) {
	c.translateCalls++
	if c.translateFn != nil {
		return c.translateFn(ctx, sourceJobID, nodeID, payload)
	}
	params := map[string]any{
		"source_job_id":   sourceJobID,
		"runtime_backend": model.ForwardRuntimeBackendCleanAgent,
	}
	return &nodeXBridgeAgentTask{
		TaskID: bridgeTaskID(sourceJobID),
		NodeID: nodeID,
		Type:   "forward",
		Action: payload.Action,
		Params: params,
	}, nil
}

func bridgeTaskID(jobID uint) string {
	return "forward-runtime-job-" + strconv.FormatUint(uint64(jobID), 10)
}

type ForwardAgentBridgeWorkerTestSuite struct {
	ServiceTestSuite
	stub   *bridgeStubNodeXClient
	worker *ForwardAgentBridgeWorker
	svc    *ForwardAgentBridgeService
}

func (s *ForwardAgentBridgeWorkerTestSuite) SetupSuite() {
	s.ServiceTestSuite.SetupSuite()
	s.Require().NoError(database.AutoMigrate(
		&model.User{},
		&model.ForwardNode{},
		&model.ForwardTunnel{},
		&model.ForwardUserTunnel{},
		&model.Forward{},
		&model.ForwardPortBinding{},
		&model.ForwardTrafficCursor{},
		&model.ForwardRuntimeJob{},
		&model.ForwardAgentBridgeTask{},
	))
}

func (s *ForwardAgentBridgeWorkerTestSuite) SetupTest() {
	s.ServiceTestSuite.SetupTest()
	db := database.Get()
	db.Exec("DELETE FROM v2_forward_agent_bridge_task")
	db.Exec("DELETE FROM v2_forward_runtime_job")
	db.Exec("DELETE FROM v2_forward_port_binding")
	db.Exec("DELETE FROM v2_forward_traffic_cursor")
	db.Exec("DELETE FROM v2_forward")
	db.Exec("DELETE FROM v2_forward_user_tunnel")
	db.Exec("DELETE FROM v2_forward_tunnel")
	db.Exec("DELETE FROM v2_forward_node")
	db.Exec("DELETE FROM v2_user")

	s.stub = &bridgeStubNodeXClient{}
	s.worker = &ForwardAgentBridgeWorker{
		db:               db,
		nodex:            s.stub,
		pollInterval:     defaultForwardRuntimeJobPollInterval,
		idlePollInterval: defaultForwardRuntimeJobIdlePollInterval,
		batchSize:        defaultForwardRuntimeJobBatchSize,
		actionTimeout:    defaultForwardCleanAgentActionTimeout,
		errorLogger:      newForwardBackgroundErrorLogger(defaultForwardRuntimeJobErrorLogInterval),
	}
	s.svc = NewForwardAgentBridgeService(db)
}

func (s *ForwardAgentBridgeWorkerTestSuite) createForwardFixture() (*model.ForwardNode, *model.Forward) {
	db := database.Get()

	node := &model.ForwardNode{
		Name:    "bridge-node",
		Type:    model.ForwardNodeTypeRelay,
		Host:    "203.0.113.20",
		Port:    22,
		Enabled: true,
	}
	assert.NoError(s.T(), db.Create(node).Error)

	tunnel := &model.ForwardTunnel{
		Name:     "bridge-tunnel",
		Type:     1,
		InNodeID: node.ID,
		InIP:     node.Host,
		OutIP:    node.Host,
		Status:   model.ForwardTunnelStatusActive,
	}
	assert.NoError(s.T(), db.Create(tunnel).Error)

	forward := &model.Forward{
		Name:           "bridge-forward",
		TunnelID:       tunnel.ID,
		InPort:         21001,
		RemoteAddr:     "127.0.0.1:8080",
		Status:         model.ForwardStatusActive,
		RuntimeBackend: model.ForwardRuntimeBackendCleanAgent,
	}
	assert.NoError(s.T(), db.Create(forward).Error)

	return node, forward
}

func (s *ForwardAgentBridgeWorkerTestSuite) enqueueCleanAgentJob(action string, forward *model.Forward, nodeID uint) *model.ForwardRuntimeJob {
	db := database.Get()
	payload := nodeXForwardExecuteRequest{
		ResourceType: nodeXForwardResourceTypePanelForward,
		Backend:      model.ForwardRuntimeBackendCleanAgent,
		Action:       action,
	}
	payloadJSON, err := json.Marshal(payload)
	assert.NoError(s.T(), err)

	fid := forward.ID
	nid := nodeID
	job := &model.ForwardRuntimeJob{
		Backend:      model.ForwardRuntimeBackendCleanAgent,
		Action:       action,
		ResourceType: nodeXForwardResourceTypePanelForward,
		ForwardID:    &fid,
		NodeID:       &nid,
		Status:       model.ForwardRuntimeJobStatusPending,
		Payload:      string(payloadJSON),
	}
	assert.NoError(s.T(), db.Create(job).Error)
	return job
}

func (s *ForwardAgentBridgeWorkerTestSuite) TestCleanAgentCreateSuccessEndToEnd() {
	db := database.Get()
	node, forward := s.createForwardFixture()
	job := s.enqueueCleanAgentJob(model.ForwardRuntimeJobActionCreate, forward, node.ID)

	assert.NoError(s.T(), s.worker.RunPendingJobs(context.Background()))
	assert.Equal(s.T(), 1, s.stub.translateCalls)

	// Job claimed → running (awaiting agent report), mapping persisted as pending.
	var reloaded model.ForwardRuntimeJob
	assert.NoError(s.T(), db.First(&reloaded, job.ID).Error)
	assert.Equal(s.T(), model.ForwardRuntimeJobStatusRunning, reloaded.Status)

	var runningForward model.Forward
	assert.NoError(s.T(), db.First(&runningForward, forward.ID).Error)
	assert.Equal(s.T(), model.ForwardRuntimeJobStatusRunning, runningForward.RuntimeStatus)
	assert.Equal(s.T(), "clean_agent runtime running", runningForward.RuntimeMessage)
	assert.NotNil(s.T(), runningForward.RuntimeLastSyncAt)

	var mapping model.ForwardAgentBridgeTask
	assert.NoError(s.T(), db.Where("runtime_job_id = ?", job.ID).First(&mapping).Error)
	assert.Equal(s.T(), bridgeTaskID(job.ID), mapping.TaskID)
	assert.Equal(s.T(), node.ID, mapping.NodeID)
	assert.Equal(s.T(), model.ForwardAgentBridgeTaskStatusPending, mapping.Status)

	// Agent reports success → job success, forward runtime success, mapping completed.
	done, err := s.svc.CompleteJob(mapping.TaskID, true, "applied", "")
	assert.NoError(s.T(), err)
	assert.False(s.T(), done)

	assert.NoError(s.T(), db.First(&reloaded, job.ID).Error)
	assert.Equal(s.T(), model.ForwardRuntimeJobStatusSuccess, reloaded.Status)
	assert.Equal(s.T(), "applied", reloaded.Result)

	var reloadedForward model.Forward
	assert.NoError(s.T(), db.First(&reloadedForward, forward.ID).Error)
	assert.Equal(s.T(), model.ForwardRuntimeJobStatusSuccess, reloadedForward.RuntimeStatus)
	assert.Equal(s.T(), model.ForwardStatusActive, reloadedForward.Status)

	assert.NoError(s.T(), db.Where("runtime_job_id = ?", job.ID).First(&mapping).Error)
	assert.Equal(s.T(), model.ForwardAgentBridgeTaskStatusCompleted, mapping.Status)
}

func (s *ForwardAgentBridgeWorkerTestSuite) TestCleanAgentDeleteSuccess() {
	db := database.Get()
	node, forward := s.createForwardFixture()
	assert.NoError(s.T(), db.Create(&model.ForwardPortBinding{
		ForwardID:  forward.ID,
		NodeID:     node.ID,
		Transport:  "tcp",
		ListenAddr: "0.0.0.0",
		InPort:     forward.InPort,
	}).Error)
	assert.NoError(s.T(), db.Create(&model.ForwardTrafficCursor{
		ForwardID:     forward.ID,
		Backend:       model.ForwardRuntimeBackendCleanAgent,
		UploadTotal:   10,
		DownloadTotal: 20,
	}).Error)
	job := s.enqueueCleanAgentJob(model.ForwardRuntimeJobActionDelete, forward, node.ID)

	assert.NoError(s.T(), s.worker.RunPendingJobs(context.Background()))

	done, err := s.svc.CompleteJob(bridgeTaskID(job.ID), true, "removed", "")
	assert.NoError(s.T(), err)
	assert.False(s.T(), done)

	var reloaded model.ForwardRuntimeJob
	assert.NoError(s.T(), db.First(&reloaded, job.ID).Error)
	assert.Equal(s.T(), model.ForwardRuntimeJobStatusSuccess, reloaded.Status)

	var count int64
	assert.NoError(s.T(), db.Model(&model.Forward{}).Where("id = ?", forward.ID).Count(&count).Error)
	assert.Zero(s.T(), count)
	assert.NoError(s.T(), db.Model(&model.ForwardPortBinding{}).Where("forward_id = ?", forward.ID).Count(&count).Error)
	assert.Zero(s.T(), count)
	assert.NoError(s.T(), db.Model(&model.ForwardTrafficCursor{}).Where("forward_id = ?", forward.ID).Count(&count).Error)
	assert.Zero(s.T(), count)
}

func (s *ForwardAgentBridgeWorkerTestSuite) TestAgentFailureResult() {
	db := database.Get()
	node, forward := s.createForwardFixture()
	job := s.enqueueCleanAgentJob(model.ForwardRuntimeJobActionCreate, forward, node.ID)

	assert.NoError(s.T(), s.worker.RunPendingJobs(context.Background()))

	done, err := s.svc.CompleteJob(bridgeTaskID(job.ID), false, "", "nft rule failed")
	assert.NoError(s.T(), err)
	assert.False(s.T(), done)

	var reloaded model.ForwardRuntimeJob
	assert.NoError(s.T(), db.First(&reloaded, job.ID).Error)
	assert.Equal(s.T(), model.ForwardRuntimeJobStatusFailed, reloaded.Status)
	assert.Equal(s.T(), "nft rule failed", reloaded.Error)

	var reloadedForward model.Forward
	assert.NoError(s.T(), db.First(&reloadedForward, forward.ID).Error)
	assert.Equal(s.T(), model.ForwardRuntimeJobStatusFailed, reloadedForward.RuntimeStatus)
	assert.Equal(s.T(), model.ForwardStatusError, reloadedForward.Status)
}

func (s *ForwardAgentBridgeWorkerTestSuite) TestDuplicateResultIsIdempotent() {
	db := database.Get()
	node, forward := s.createForwardFixture()
	job := s.enqueueCleanAgentJob(model.ForwardRuntimeJobActionCreate, forward, node.ID)
	assert.NoError(s.T(), s.worker.RunPendingJobs(context.Background()))

	done1, err := s.svc.CompleteJob(bridgeTaskID(job.ID), true, "applied", "")
	assert.NoError(s.T(), err)
	assert.False(s.T(), done1)

	// A second (failure) report for the same task must not regress the terminal state.
	done2, err := s.svc.CompleteJob(bridgeTaskID(job.ID), false, "", "late failure")
	assert.NoError(s.T(), err)
	assert.True(s.T(), done2)

	var reloaded model.ForwardRuntimeJob
	assert.NoError(s.T(), db.First(&reloaded, job.ID).Error)
	assert.Equal(s.T(), model.ForwardRuntimeJobStatusSuccess, reloaded.Status)
	assert.Equal(s.T(), "applied", reloaded.Result)
	assert.Empty(s.T(), reloaded.Error)
}

func (s *ForwardAgentBridgeWorkerTestSuite) TestRequeuePreservesDispatchedMapping() {
	db := database.Get()
	node, forward := s.createForwardFixture()
	job := s.enqueueCleanAgentJob(model.ForwardRuntimeJobActionCreate, forward, node.ID)
	assert.NoError(s.T(), s.worker.RunPendingJobs(context.Background()))

	// Simulate restart with a fresh worker instance.
	fresh := &ForwardAgentBridgeWorker{
		db:          db,
		nodex:       &bridgeStubNodeXClient{},
		batchSize:   defaultForwardRuntimeJobBatchSize,
		errorLogger: newForwardBackgroundErrorLogger(defaultForwardRuntimeJobErrorLogInterval),
	}
	assert.NoError(s.T(), fresh.requeueStaleJobs())

	// Job already has a mapping → stays running (awaiting agent report), not re-translated.
	var reloaded model.ForwardRuntimeJob
	assert.NoError(s.T(), db.First(&reloaded, job.ID).Error)
	assert.Equal(s.T(), model.ForwardRuntimeJobStatusRunning, reloaded.Status)

	var reloadedForward model.Forward
	assert.NoError(s.T(), db.First(&reloadedForward, forward.ID).Error)
	assert.Equal(s.T(), model.ForwardRuntimeJobStatusRunning, reloadedForward.RuntimeStatus)
	assert.Equal(s.T(), "clean_agent runtime running", reloadedForward.RuntimeMessage)

	// Durable mapping still resolves so a late agent report can complete the job.
	mapping, err := s.svc.LookupBridgeTask(bridgeTaskID(job.ID))
	assert.NoError(s.T(), err)
	assert.NotNil(s.T(), mapping)
}

func (s *ForwardAgentBridgeWorkerTestSuite) TestRequeueResetsRunningJobWithoutMapping() {
	db := database.Get()
	node, forward := s.createForwardFixture()
	// Job stuck running with NO mapping (crashed before/during translate).
	job := s.enqueueCleanAgentJob(model.ForwardRuntimeJobActionCreate, forward, node.ID)
	assert.NoError(s.T(), db.Model(&model.ForwardRuntimeJob{}).Where("id = ?", job.ID).
		Update("status", model.ForwardRuntimeJobStatusRunning).Error)
	assert.NoError(s.T(), db.Model(&model.Forward{}).Where("id = ?", forward.ID).Updates(map[string]any{
		"runtime_status":  model.ForwardRuntimeJobStatusRunning,
		"runtime_message": "clean_agent runtime running",
	}).Error)

	assert.NoError(s.T(), s.worker.requeueStaleJobs())

	var reloaded model.ForwardRuntimeJob
	assert.NoError(s.T(), db.First(&reloaded, job.ID).Error)
	assert.Equal(s.T(), model.ForwardRuntimeJobStatusPending, reloaded.Status)

	var reloadedForward model.Forward
	assert.NoError(s.T(), db.First(&reloadedForward, forward.ID).Error)
	assert.Equal(s.T(), model.ForwardRuntimeJobStatusPending, reloadedForward.RuntimeStatus)
	assert.Equal(s.T(), "clean_agent runtime requeued after worker restart", reloadedForward.RuntimeMessage)
}

func (s *ForwardAgentBridgeWorkerTestSuite) TestDispatchedBridgeTaskTimesOutAndLateReportIsIgnored() {
	db := database.Get()
	node, forward := s.createForwardFixture()
	job := s.enqueueCleanAgentJob(model.ForwardRuntimeJobActionCreate, forward, node.ID)
	assert.NoError(s.T(), s.worker.RunPendingJobs(context.Background()))

	now := time.Now()
	stale := now.Add(-2 * time.Minute)
	s.worker.actionTimeout = time.Minute
	assert.NoError(s.T(), db.Model(&model.ForwardAgentBridgeTask{}).
		Where("runtime_job_id = ?", job.ID).
		UpdateColumns(map[string]any{
			"status":     model.ForwardAgentBridgeTaskStatusDispatched,
			"dispatched": true,
			"updated_at": stale,
		}).Error)

	assert.NoError(s.T(), s.worker.expireTimedOutBridgeTasks(now))

	var reloadedJob model.ForwardRuntimeJob
	assert.NoError(s.T(), db.First(&reloadedJob, job.ID).Error)
	assert.Equal(s.T(), model.ForwardRuntimeJobStatusFailed, reloadedJob.Status)
	assert.Contains(s.T(), reloadedJob.Error, "clean_agent task timed out")
	assert.NotNil(s.T(), reloadedJob.CompletedAt)

	var reloadedForward model.Forward
	assert.NoError(s.T(), db.First(&reloadedForward, forward.ID).Error)
	assert.Equal(s.T(), model.ForwardRuntimeJobStatusFailed, reloadedForward.RuntimeStatus)
	assert.Equal(s.T(), model.ForwardStatusError, reloadedForward.Status)
	assert.Contains(s.T(), reloadedForward.RuntimeMessage, "clean_agent task timed out")

	var mapping model.ForwardAgentBridgeTask
	assert.NoError(s.T(), db.Where("runtime_job_id = ?", job.ID).First(&mapping).Error)
	assert.Equal(s.T(), model.ForwardAgentBridgeTaskStatusFailed, mapping.Status)

	done, err := s.svc.CompleteJob(mapping.TaskID, true, "late success", "")
	assert.NoError(s.T(), err)
	assert.True(s.T(), done)

	assert.NoError(s.T(), db.First(&reloadedJob, job.ID).Error)
	assert.Equal(s.T(), model.ForwardRuntimeJobStatusFailed, reloadedJob.Status)
	assert.NotContains(s.T(), reloadedJob.Result, "late success")
}

func (s *ForwardAgentBridgeWorkerTestSuite) TestFreshBridgeTaskDoesNotTimeOut() {
	db := database.Get()
	node, forward := s.createForwardFixture()
	job := s.enqueueCleanAgentJob(model.ForwardRuntimeJobActionCreate, forward, node.ID)
	assert.NoError(s.T(), s.worker.RunPendingJobs(context.Background()))

	now := time.Now()
	s.worker.actionTimeout = time.Minute
	assert.NoError(s.T(), s.worker.expireTimedOutBridgeTasks(now))

	var reloadedJob model.ForwardRuntimeJob
	assert.NoError(s.T(), db.First(&reloadedJob, job.ID).Error)
	assert.Equal(s.T(), model.ForwardRuntimeJobStatusRunning, reloadedJob.Status)

	var mapping model.ForwardAgentBridgeTask
	assert.NoError(s.T(), db.Where("runtime_job_id = ?", job.ID).First(&mapping).Error)
	assert.Equal(s.T(), model.ForwardAgentBridgeTaskStatusPending, mapping.Status)
}

func (s *ForwardAgentBridgeWorkerTestSuite) TestNftablesAnsibleNotBridged() {
	db := database.Get()
	_, forward := s.createForwardFixture()
	fid := forward.ID
	nid := uint(999)
	job := &model.ForwardRuntimeJob{
		Backend:      model.ForwardRuntimeBackendNftablesAnsible,
		Action:       model.ForwardRuntimeJobActionCreate,
		ResourceType: nodeXForwardResourceTypePanelForward,
		ForwardID:    &fid,
		NodeID:       &nid,
		Status:       model.ForwardRuntimeJobStatusPending,
		Payload:      "{}",
	}
	assert.NoError(s.T(), db.Create(job).Error)

	assert.NoError(s.T(), s.worker.RunPendingJobs(context.Background()))
	assert.Equal(s.T(), 0, s.stub.translateCalls)

	var reloaded model.ForwardRuntimeJob
	assert.NoError(s.T(), db.First(&reloaded, job.ID).Error)
	assert.Equal(s.T(), model.ForwardRuntimeJobStatusPending, reloaded.Status)
}

func (s *ForwardAgentBridgeWorkerTestSuite) TestGostNotBridged() {
	db := database.Get()
	_, forward := s.createForwardFixture()
	fid := forward.ID
	job := &model.ForwardRuntimeJob{
		Backend:      model.ForwardRuntimeBackendGost,
		Action:       model.ForwardRuntimeJobActionCreate,
		ResourceType: nodeXForwardResourceTypePanelForward,
		ForwardID:    &fid,
		Status:       model.ForwardRuntimeJobStatusPending,
		Payload:      "{}",
	}
	assert.NoError(s.T(), db.Create(job).Error)

	assert.NoError(s.T(), s.worker.RunPendingJobs(context.Background()))
	assert.Equal(s.T(), 0, s.stub.translateCalls)

	var reloaded model.ForwardRuntimeJob
	assert.NoError(s.T(), db.First(&reloaded, job.ID).Error)
	assert.Equal(s.T(), model.ForwardRuntimeJobStatusPending, reloaded.Status)
}

func (s *ForwardAgentBridgeWorkerTestSuite) TestTranslateFailureDoesNotFalseMarkSuccess() {
	db := database.Get()
	node, forward := s.createForwardFixture()
	job := s.enqueueCleanAgentJob(model.ForwardRuntimeJobActionCreate, forward, node.ID)

	s.stub.translateFn = func(ctx context.Context, sourceJobID uint, nodeID uint, payload nodeXForwardExecuteRequest) (*nodeXBridgeAgentTask, error) {
		return nil, errors.New("nodex down")
	}

	assert.NoError(s.T(), s.worker.RunPendingJobs(context.Background()))

	// Job failed, no mapping persisted, forward not marked success.
	var reloaded model.ForwardRuntimeJob
	assert.NoError(s.T(), db.First(&reloaded, job.ID).Error)
	assert.Equal(s.T(), model.ForwardRuntimeJobStatusFailed, reloaded.Status)
	assert.Contains(s.T(), reloaded.Error, "nodex down")

	var count int64
	assert.NoError(s.T(), db.Model(&model.ForwardAgentBridgeTask{}).Where("runtime_job_id = ?", job.ID).Count(&count).Error)
	assert.Equal(s.T(), int64(0), count)

	var reloadedForward model.Forward
	assert.NoError(s.T(), db.First(&reloadedForward, forward.ID).Error)
	assert.NotEqual(s.T(), model.ForwardRuntimeJobStatusSuccess, reloadedForward.RuntimeStatus)
}

func (s *ForwardAgentBridgeWorkerTestSuite) TestTranslateCancellationFailsJobAndDrains() {
	db := database.Get()
	node, forward := s.createForwardFixture()
	job := s.enqueueCleanAgentJob(model.ForwardRuntimeJobActionCreate, forward, node.ID)

	started := make(chan struct{})
	s.stub.translateFn = func(ctx context.Context, sourceJobID uint, nodeID uint, payload nodeXForwardExecuteRequest) (*nodeXBridgeAgentTask, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- s.worker.RunPendingJobs(ctx)
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		s.T().Fatal("bridge worker did not enter NodeX translate")
	}
	cancel()

	select {
	case err := <-done:
		assert.NoError(s.T(), err)
	case <-time.After(time.Second):
		s.T().Fatal("bridge worker did not drain after translate cancellation")
	}

	var reloaded model.ForwardRuntimeJob
	assert.NoError(s.T(), db.First(&reloaded, job.ID).Error)
	assert.Equal(s.T(), model.ForwardRuntimeJobStatusFailed, reloaded.Status)
	assert.Contains(s.T(), reloaded.Error, context.Canceled.Error())
	assert.NotNil(s.T(), reloaded.CompletedAt)

	var mappingCount int64
	assert.NoError(s.T(), db.Model(&model.ForwardAgentBridgeTask{}).Where("runtime_job_id = ?", job.ID).Count(&mappingCount).Error)
	assert.Zero(s.T(), mappingCount)

	var reloadedForward model.Forward
	assert.NoError(s.T(), db.First(&reloadedForward, forward.ID).Error)
	assert.Equal(s.T(), model.ForwardRuntimeJobStatusFailed, reloadedForward.RuntimeStatus)
	assert.Equal(s.T(), model.ForwardStatusError, reloadedForward.Status)
	assert.Contains(s.T(), reloadedForward.RuntimeMessage, context.Canceled.Error())
	assert.NotNil(s.T(), reloadedForward.RuntimeLastSyncAt)
}

func (s *ForwardAgentBridgeWorkerTestSuite) TestRetryUpsertsExistingBridgeMapping() {
	db := database.Get()
	node, forward := s.createForwardFixture()
	job := s.enqueueCleanAgentJob(model.ForwardRuntimeJobActionUpdate, forward, node.ID)

	originalMapping := &model.ForwardAgentBridgeTask{
		TaskID:       "stale-forward-runtime-task",
		RuntimeJobID: job.ID,
		NodeID:       node.ID,
		ForwardID:    job.ForwardID,
		Action:       model.ForwardRuntimeJobActionCreate,
		Type:         "forward",
		Params:       `{"attempt":1}`,
		Status:       model.ForwardAgentBridgeTaskStatusFailed,
		Dispatched:   true,
	}
	assert.NoError(s.T(), db.Create(originalMapping).Error)

	s.stub.translateFn = func(ctx context.Context, sourceJobID uint, nodeID uint, payload nodeXForwardExecuteRequest) (*nodeXBridgeAgentTask, error) {
		return &nodeXBridgeAgentTask{
			TaskID: "retry-forward-runtime-task",
			NodeID: nodeID,
			Type:   "forward",
			Action: payload.Action,
			Params: map[string]any{
				"attempt":       2,
				"source_job_id": sourceJobID,
			},
		}, nil
	}

	assert.NoError(s.T(), s.worker.RunPendingJobs(context.Background()))
	assert.Equal(s.T(), 1, s.stub.translateCalls)

	var mappingCount int64
	assert.NoError(s.T(), db.Model(&model.ForwardAgentBridgeTask{}).Where("runtime_job_id = ?", job.ID).Count(&mappingCount).Error)
	assert.Equal(s.T(), int64(1), mappingCount)

	var mapping model.ForwardAgentBridgeTask
	assert.NoError(s.T(), db.Where("runtime_job_id = ?", job.ID).First(&mapping).Error)
	assert.Equal(s.T(), originalMapping.ID, mapping.ID)
	assert.Equal(s.T(), "retry-forward-runtime-task", mapping.TaskID)
	assert.Equal(s.T(), node.ID, mapping.NodeID)
	assert.Equal(s.T(), model.ForwardRuntimeJobActionUpdate, mapping.Action)
	assert.Equal(s.T(), model.ForwardAgentBridgeTaskStatusPending, mapping.Status)
	assert.False(s.T(), mapping.Dispatched)
	assert.JSONEq(s.T(), `{"attempt":2,"source_job_id":`+strconv.FormatUint(uint64(job.ID), 10)+`}`, mapping.Params)

	var reloaded model.ForwardRuntimeJob
	assert.NoError(s.T(), db.First(&reloaded, job.ID).Error)
	assert.Equal(s.T(), model.ForwardRuntimeJobStatusRunning, reloaded.Status)
	assert.NotNil(s.T(), reloaded.ClaimedAt)
}

func (s *ForwardAgentBridgeWorkerTestSuite) TestMissingExecutionNodeFailsJob() {
	db := database.Get()
	_, forward := s.createForwardFixture()
	fid := forward.ID
	payloadJSON, _ := json.Marshal(nodeXForwardExecuteRequest{
		Backend: model.ForwardRuntimeBackendCleanAgent,
		Action:  model.ForwardRuntimeJobActionCreate,
	})
	job := &model.ForwardRuntimeJob{
		Backend:      model.ForwardRuntimeBackendCleanAgent,
		Action:       model.ForwardRuntimeJobActionCreate,
		ResourceType: nodeXForwardResourceTypePanelForward,
		ForwardID:    &fid,
		NodeID:       nil,
		Status:       model.ForwardRuntimeJobStatusPending,
		Payload:      string(payloadJSON),
	}
	assert.NoError(s.T(), db.Create(job).Error)

	assert.NoError(s.T(), s.worker.RunPendingJobs(context.Background()))
	assert.Equal(s.T(), 0, s.stub.translateCalls)

	var reloaded model.ForwardRuntimeJob
	assert.NoError(s.T(), db.First(&reloaded, job.ID).Error)
	assert.Equal(s.T(), model.ForwardRuntimeJobStatusFailed, reloaded.Status)
	assert.Contains(s.T(), reloaded.Error, "missing execution node")
}

func TestForwardAgentBridgeWorker(t *testing.T) {
	suite.Run(t, new(ForwardAgentBridgeWorkerTestSuite))
}
