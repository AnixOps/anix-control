package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	appconfig "github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"
)

type stubPanelForwardRuntimeCommandRunner struct {
	output      string
	err         error
	runFn       func(ctx context.Context, command string, args []string, workdir string, env map[string]string) (string, error)
	runs        int
	lastCommand string
	lastArgs    []string
	lastWorkdir string
	lastEnv     map[string]string
}

func (r *stubPanelForwardRuntimeCommandRunner) Run(ctx context.Context, command string, args []string, workdir string, env map[string]string) (string, error) {
	r.runs++
	r.lastCommand = command
	r.lastArgs = append([]string(nil), args...)
	r.lastWorkdir = workdir
	if env != nil {
		r.lastEnv = make(map[string]string, len(env))
		for key, value := range env {
			r.lastEnv[key] = value
		}
	} else {
		r.lastEnv = nil
	}
	if r.runFn != nil {
		return r.runFn(ctx, command, args, workdir, env)
	}
	return r.output, r.err
}

type PanelForwardRuntimeJobExecutorTestSuite struct {
	ServiceTestSuite
	executor *PanelForwardRuntimeJobExecutor
	runner   *stubPanelForwardRuntimeCommandRunner
}

func (s *PanelForwardRuntimeJobExecutorTestSuite) SetupSuite() {
	s.ServiceTestSuite.SetupSuite()
	s.Require().NoError(database.AutoMigrate(
		&model.Forward{},
		&model.ForwardPortBinding{},
		&model.ForwardTrafficCursor{},
		&model.ForwardRuntimeJob{},
	))
}

func (s *PanelForwardRuntimeJobExecutorTestSuite) SetupTest() {
	s.ServiceTestSuite.SetupTest()
	appconfig.Set(nil)

	db := database.Get()
	db.Exec("DELETE FROM v2_forward_runtime_job")
	db.Exec("DELETE FROM v2_forward_port_binding")
	db.Exec("DELETE FROM v2_forward_traffic_cursor")
	db.Exec("DELETE FROM v2_forward")

	s.runner = &stubPanelForwardRuntimeCommandRunner{}
	s.executor = NewPanelForwardRuntimeJobExecutor(db)
	s.executor.runner = s.runner
	s.executor.batchSize = 1
	s.executor.jobTimeout = 30 * time.Second
}

func (s *PanelForwardRuntimeJobExecutorTestSuite) TestRunPendingJobs_CompletesAnsibleJobAndUpdatesForward() {
	db := database.Get()

	forward := &model.Forward{
		UserID:         1,
		UserName:       "runtime-user",
		Name:           "Runtime Forward",
		TunnelID:       1,
		InPort:         10001,
		RemoteAddr:     "8.8.8.8:443",
		Status:         model.ForwardStatusActive,
		RuntimeBackend: model.ForwardRuntimeBackendIptablesAnsible,
		RuntimeStatus:  model.ForwardRuntimeJobStatusPending,
	}
	assert.NoError(s.T(), db.Create(forward).Error)

	payload := panelForwardAnsibleRuntimePayload{
		Action:        model.ForwardRuntimeJobActionCreate,
		Inventory:     "/etc/ansible/hosts",
		Playbook:      "/opt/ansible/apply.yml",
		Become:        true,
		Command:       "custom-ansible-playbook",
		WorkingDir:    "/opt/ansible",
		TargetPattern: "{{node.host}}",
		Environment: map[string]string{
			"ANSIBLE_CONFIG": "/etc/ansible/ansible.cfg",
		},
		TimeoutSeconds: 45,
		ExtraVars: map[string]any{
			"manage_with": "iptables",
		},
		Forward: panelForwardAnsibleForwardPayload{
			ID:            forward.ID,
			UserID:        forward.UserID,
			Name:          forward.Name,
			InPort:        forward.InPort,
			RemoteAddr:    forward.RemoteAddr,
			InterfaceName: "",
			Strategy:      "fifo",
			Status:        forward.Status,
		},
		Tunnel: panelForwardAnsibleTunnelPayload{
			ID:            1,
			Name:          "Tunnel A",
			InNodeID:      9,
			Protocol:      "tcp",
			TCPListenAddr: "0.0.0.0",
			UDPListenAddr: "0.0.0.0",
			InterfaceName: "eth0",
		},
		Node: panelForwardAnsibleNodePayload{
			ID:      9,
			Name:    "relay-a",
			Host:    "relay-a.example.com",
			Port:    22,
			APIPort: 9000,
		},
		Targets: []panelForwardAnsibleTargetPayload{
			{Name: "target-1", Addr: "8.8.8.8:443"},
		},
	}
	payloadJSON, err := json.Marshal(payload)
	assert.NoError(s.T(), err)

	job := &model.ForwardRuntimeJob{
		Backend:   model.ForwardRuntimeBackendIptablesAnsible,
		Action:    model.ForwardRuntimeJobActionCreate,
		ForwardID: uintPtr(forward.ID),
		Status:    model.ForwardRuntimeJobStatusPending,
		Payload:   string(payloadJSON),
	}
	assert.NoError(s.T(), db.Create(job).Error)

	s.runner.output = "PLAY RECAP relay-a.example.com : ok=3 changed=1 failed=0"

	err = s.executor.RunPendingJobs(context.Background())
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), 1, s.runner.runs)
	assert.Equal(s.T(), "custom-ansible-playbook", s.runner.lastCommand)
	assert.Equal(s.T(), resolveForwardRuntimeWorkingDir("/opt/ansible"), s.runner.lastWorkdir)
	assert.Equal(s.T(), resolveForwardRuntimeEnvPath("/opt/ansible", "/etc/ansible/ansible.cfg"), s.runner.lastEnv["ANSIBLE_CONFIG"])
	assert.Contains(s.T(), s.runner.lastArgs, "-i")
	assert.Contains(s.T(), s.runner.lastArgs, resolveForwardRuntimeFilePath("/opt/ansible", "/etc/ansible/hosts"))
	assert.Contains(s.T(), s.runner.lastArgs, "--limit")
	assert.Contains(s.T(), s.runner.lastArgs, "relay-a.example.com")
	assert.Contains(s.T(), s.runner.lastArgs, "--become")
	assert.Contains(s.T(), s.runner.lastArgs, resolveForwardRuntimeFilePath("/opt/ansible", "/opt/ansible/apply.yml"))

	var extraVarsJSON string
	for idx := 0; idx < len(s.runner.lastArgs)-1; idx++ {
		if s.runner.lastArgs[idx] == "--extra-vars" {
			extraVarsJSON = s.runner.lastArgs[idx+1]
			break
		}
	}
	assert.NotEmpty(s.T(), extraVarsJSON)

	var extraVars map[string]any
	assert.NoError(s.T(), json.Unmarshal([]byte(extraVarsJSON), &extraVars))
	assert.Equal(s.T(), model.ForwardRuntimeJobActionCreate, extraVars["runtimeAction"])
	assert.Equal(s.T(), "iptables", extraVars["manage_with"])
	assert.Equal(s.T(), "relay-a.example.com", extraVars["ansibleLimit"])

	var updatedJob model.ForwardRuntimeJob
	assert.NoError(s.T(), db.First(&updatedJob, job.ID).Error)
	assert.Equal(s.T(), model.ForwardRuntimeJobStatusSuccess, updatedJob.Status)
	assert.NotNil(s.T(), updatedJob.StartedAt)
	assert.NotNil(s.T(), updatedJob.CompletedAt)
	assert.Contains(s.T(), updatedJob.Result, "PLAY RECAP")
	assert.Empty(s.T(), updatedJob.Error)

	var updatedForward model.Forward
	assert.NoError(s.T(), db.First(&updatedForward, forward.ID).Error)
	assert.Equal(s.T(), model.ForwardRuntimeBackendIptablesAnsible, updatedForward.RuntimeBackend)
	assert.Equal(s.T(), model.ForwardRuntimeJobStatusSuccess, updatedForward.RuntimeStatus)
	assert.Equal(s.T(), "ansible runtime synchronized", updatedForward.RuntimeMessage)
	assert.Equal(s.T(), model.ForwardStatusActive, updatedForward.Status)
	assert.NotNil(s.T(), updatedForward.RuntimeLastSyncAt)
}

func (s *PanelForwardRuntimeJobExecutorTestSuite) TestFinishDeleteSuccessRemovesForwardRecords() {
	db := database.Get()

	forward := &model.Forward{
		UserID:         1,
		UserName:       "runtime-delete-user",
		Name:           "Runtime Delete Forward",
		TunnelID:       1,
		InPort:         10003,
		RemoteAddr:     "8.8.4.4:443",
		Status:         model.ForwardStatusPaused,
		RuntimeBackend: model.ForwardRuntimeBackendNftablesAnsible,
		RuntimeStatus:  model.ForwardRuntimeJobStatusPending,
	}
	assert.NoError(s.T(), db.Create(forward).Error)
	assert.NoError(s.T(), db.Create(&model.ForwardPortBinding{
		ForwardID:  forward.ID,
		NodeID:     1,
		Transport:  "tcp",
		ListenAddr: "0.0.0.0",
		InPort:     forward.InPort,
	}).Error)
	assert.NoError(s.T(), db.Create(&model.ForwardTrafficCursor{
		ForwardID:     forward.ID,
		Backend:       model.ForwardRuntimeBackendNftablesAnsible,
		UploadTotal:   10,
		DownloadTotal: 20,
	}).Error)

	job := &model.ForwardRuntimeJob{
		Backend:   model.ForwardRuntimeBackendNftablesAnsible,
		Action:    model.ForwardRuntimeJobActionDelete,
		ForwardID: uintPtr(forward.ID),
		Status:    model.ForwardRuntimeJobStatusRunning,
		Payload:   "{}",
	}
	assert.NoError(s.T(), db.Create(job).Error)

	err := s.executor.finishJobSuccess(job, &panelForwardAnsibleRuntimePayload{Action: model.ForwardRuntimeJobActionDelete}, "removed")
	assert.NoError(s.T(), err)

	var reloadedJob model.ForwardRuntimeJob
	assert.NoError(s.T(), db.First(&reloadedJob, job.ID).Error)
	assert.Equal(s.T(), model.ForwardRuntimeJobStatusSuccess, reloadedJob.Status)

	var count int64
	assert.NoError(s.T(), db.Model(&model.Forward{}).Where("id = ?", forward.ID).Count(&count).Error)
	assert.Zero(s.T(), count)
	assert.NoError(s.T(), db.Model(&model.ForwardPortBinding{}).Where("forward_id = ?", forward.ID).Count(&count).Error)
	assert.Zero(s.T(), count)
	assert.NoError(s.T(), db.Model(&model.ForwardTrafficCursor{}).Where("forward_id = ?", forward.ID).Count(&count).Error)
	assert.Zero(s.T(), count)
}

func (s *PanelForwardRuntimeJobExecutorTestSuite) TestAnsiblePayloadResolvesRelativePathsAgainstWorkingDir() {
	tempDir := s.T().TempDir()
	playbookDir := filepath.Join(tempDir, "playbooks")
	assert.NoError(s.T(), os.MkdirAll(playbookDir, 0o755))

	inventoryPath := filepath.Join(tempDir, "inventory.ini")
	playbookPath := filepath.Join(playbookDir, "apply.yml")
	ansibleConfigPath := filepath.Join(tempDir, "ansible.cfg")
	assert.NoError(s.T(), os.WriteFile(inventoryPath, []byte("[forward_nodes]\n"), 0o600))
	assert.NoError(s.T(), os.WriteFile(playbookPath, []byte("---\n- hosts: all\n"), 0o600))
	assert.NoError(s.T(), os.WriteFile(ansibleConfigPath, []byte("[defaults]\n"), 0o600))

	payload := panelForwardAnsibleRuntimePayload{
		Inventory:  "inventory.ini",
		Playbook:   filepath.Join("playbooks", "apply.yml"),
		WorkingDir: tempDir,
		Environment: map[string]string{
			"ANSIBLE_CONFIG": "ansible.cfg",
		},
	}

	args, err := payload.commandArgs()
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), tempDir, payload.workingDirectory())
	assert.Contains(s.T(), args, inventoryPath)
	assert.Contains(s.T(), args, playbookPath)
	assert.Equal(s.T(), ansibleConfigPath, payload.environment()["ANSIBLE_CONFIG"])
}

func (s *PanelForwardRuntimeJobExecutorTestSuite) TestRunPendingJobs_MarksForwardAsErrorWhenExecutionFails() {
	db := database.Get()

	forward := &model.Forward{
		UserID:         1,
		UserName:       "runtime-user",
		Name:           "Runtime Forward Failure",
		TunnelID:       1,
		InPort:         10002,
		RemoteAddr:     "1.1.1.1:443",
		Status:         model.ForwardStatusActive,
		RuntimeBackend: model.ForwardRuntimeBackendIptablesAnsible,
		RuntimeStatus:  model.ForwardRuntimeJobStatusPending,
	}
	assert.NoError(s.T(), db.Create(forward).Error)

	payload := panelForwardAnsibleRuntimePayload{
		Action:    model.ForwardRuntimeJobActionCreate,
		Inventory: "/etc/ansible/hosts",
		Playbook:  "/opt/ansible/apply.yml",
		Forward: panelForwardAnsibleForwardPayload{
			ID:         forward.ID,
			UserID:     forward.UserID,
			Name:       forward.Name,
			InPort:     forward.InPort,
			RemoteAddr: forward.RemoteAddr,
			Strategy:   "fifo",
			Status:     forward.Status,
		},
		Tunnel: panelForwardAnsibleTunnelPayload{
			ID:       1,
			Name:     "Tunnel A",
			InNodeID: 9,
			Protocol: "tcp",
		},
		Node: panelForwardAnsibleNodePayload{
			ID:   9,
			Name: "relay-a",
			Host: "relay-a.example.com",
		},
		Targets: []panelForwardAnsibleTargetPayload{
			{Name: "target-1", Addr: "1.1.1.1:443"},
		},
	}
	payloadJSON, err := json.Marshal(payload)
	assert.NoError(s.T(), err)

	job := &model.ForwardRuntimeJob{
		Backend:   model.ForwardRuntimeBackendIptablesAnsible,
		Action:    model.ForwardRuntimeJobActionCreate,
		ForwardID: uintPtr(forward.ID),
		Status:    model.ForwardRuntimeJobStatusPending,
		Payload:   string(payloadJSON),
	}
	assert.NoError(s.T(), db.Create(job).Error)

	s.runner.output = "fatal: play failed"
	s.runner.err = errors.New("exit status 2")

	err = s.executor.RunPendingJobs(context.Background())
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), 1, s.runner.runs)
	assert.Equal(s.T(), defaultAnsibleCommand, s.runner.lastCommand)

	var updatedJob model.ForwardRuntimeJob
	assert.NoError(s.T(), db.First(&updatedJob, job.ID).Error)
	assert.Equal(s.T(), model.ForwardRuntimeJobStatusFailed, updatedJob.Status)
	assert.NotNil(s.T(), updatedJob.StartedAt)
	assert.NotNil(s.T(), updatedJob.CompletedAt)
	assert.Contains(s.T(), updatedJob.Error, "exit status 2")
	assert.Contains(s.T(), updatedJob.Error, "fatal: play failed")

	var updatedForward model.Forward
	assert.NoError(s.T(), db.First(&updatedForward, forward.ID).Error)
	assert.Equal(s.T(), model.ForwardStatusError, updatedForward.Status)
	assert.Equal(s.T(), model.ForwardRuntimeJobStatusFailed, updatedForward.RuntimeStatus)
	assert.Contains(s.T(), updatedForward.RuntimeMessage, "exit status 2")
	assert.NotNil(s.T(), updatedForward.RuntimeLastSyncAt)
}

func (s *PanelForwardRuntimeJobExecutorTestSuite) TestClaimJobMarksForwardRuntimeRunning() {
	db := database.Get()

	forward := &model.Forward{
		UserID:         1,
		UserName:       "runtime-user",
		Name:           "Runtime Forward Running",
		TunnelID:       1,
		InPort:         10003,
		RemoteAddr:     "9.9.9.9:443",
		Status:         model.ForwardStatusActive,
		RuntimeBackend: model.ForwardRuntimeBackendNftablesAnsible,
		RuntimeStatus:  model.ForwardRuntimeJobStatusPending,
		RuntimeMessage: "ansible runtime queued for local executor",
	}
	assert.NoError(s.T(), db.Create(forward).Error)

	job := &model.ForwardRuntimeJob{
		Backend:   model.ForwardRuntimeBackendNftablesAnsible,
		Action:    model.ForwardRuntimeJobActionCreate,
		ForwardID: uintPtr(forward.ID),
		Status:    model.ForwardRuntimeJobStatusPending,
		Payload:   "{}",
	}
	assert.NoError(s.T(), db.Create(job).Error)

	claimed, err := s.executor.claimJob(job)
	assert.NoError(s.T(), err)
	assert.True(s.T(), claimed)

	var updatedJob model.ForwardRuntimeJob
	assert.NoError(s.T(), db.First(&updatedJob, job.ID).Error)
	assert.Equal(s.T(), model.ForwardRuntimeJobStatusRunning, updatedJob.Status)
	assert.NotNil(s.T(), updatedJob.StartedAt)

	var updatedForward model.Forward
	assert.NoError(s.T(), db.First(&updatedForward, forward.ID).Error)
	assert.Equal(s.T(), model.ForwardRuntimeBackendNftablesAnsible, updatedForward.RuntimeBackend)
	assert.Equal(s.T(), model.ForwardRuntimeJobStatusRunning, updatedForward.RuntimeStatus)
	assert.Equal(s.T(), "ansible runtime applying", updatedForward.RuntimeMessage)
	assert.Equal(s.T(), model.ForwardStatusActive, updatedForward.Status)
	assert.NotNil(s.T(), updatedForward.RuntimeLastSyncAt)
}

func (s *PanelForwardRuntimeJobExecutorTestSuite) TestRunPendingJobs_NoPendingJobsSkipsRunner() {
	err := s.executor.RunPendingJobs(context.Background())
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), 0, s.runner.runs)
}

func (s *PanelForwardRuntimeJobExecutorTestSuite) TestRunPendingJobs_CancelDrainsRunningJob() {
	db := database.Get()

	forward := &model.Forward{
		UserID:         1,
		UserName:       "runtime-cancel-user",
		Name:           "Runtime Cancel Forward",
		TunnelID:       1,
		InPort:         10004,
		RemoteAddr:     "203.0.113.10:443",
		Status:         model.ForwardStatusActive,
		RuntimeBackend: model.ForwardRuntimeBackendNftablesAnsible,
		RuntimeStatus:  model.ForwardRuntimeJobStatusPending,
		RuntimeMessage: "ansible runtime queued for local executor",
	}
	assert.NoError(s.T(), db.Create(forward).Error)

	payload := panelForwardAnsibleRuntimePayload{
		Action:        model.ForwardRuntimeJobActionCreate,
		Inventory:     "/etc/ansible/hosts",
		Playbook:      "/opt/ansible/apply.yml",
		TargetPattern: "{{node.host}}",
		Forward: panelForwardAnsibleForwardPayload{
			ID:         forward.ID,
			UserID:     forward.UserID,
			Name:       forward.Name,
			InPort:     forward.InPort,
			RemoteAddr: forward.RemoteAddr,
			Status:     forward.Status,
		},
		Tunnel: panelForwardAnsibleTunnelPayload{
			ID:       1,
			Name:     "Tunnel Cancel",
			InNodeID: 9,
			Protocol: "tcp",
		},
		Node: panelForwardAnsibleNodePayload{
			ID:   9,
			Name: "relay-cancel",
			Host: "relay-cancel.example.com",
			Port: 22,
		},
		Targets: []panelForwardAnsibleTargetPayload{
			{Name: "target-1", Addr: "203.0.113.10:443"},
		},
	}
	payloadJSON, err := json.Marshal(payload)
	assert.NoError(s.T(), err)

	job := &model.ForwardRuntimeJob{
		Backend:   model.ForwardRuntimeBackendNftablesAnsible,
		Action:    model.ForwardRuntimeJobActionCreate,
		ForwardID: uintPtr(forward.ID),
		Status:    model.ForwardRuntimeJobStatusPending,
		Payload:   string(payloadJSON),
	}
	assert.NoError(s.T(), db.Create(job).Error)

	started := make(chan struct{})
	s.runner.runFn = func(ctx context.Context, command string, args []string, workdir string, env map[string]string) (string, error) {
		close(started)
		<-ctx.Done()
		return "", ctx.Err()
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- s.executor.RunPendingJobs(ctx)
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		s.T().Fatal("runtime runner did not start")
	}
	cancel()

	select {
	case err := <-done:
		assert.NoError(s.T(), err)
	case <-time.After(time.Second):
		s.T().Fatal("runtime executor did not drain after cancellation")
	}
	assert.Equal(s.T(), 1, s.runner.runs)

	var updatedJob model.ForwardRuntimeJob
	assert.NoError(s.T(), db.First(&updatedJob, job.ID).Error)
	assert.Equal(s.T(), model.ForwardRuntimeJobStatusFailed, updatedJob.Status)
	assert.Contains(s.T(), updatedJob.Error, context.Canceled.Error())
	assert.NotNil(s.T(), updatedJob.CompletedAt)

	var updatedForward model.Forward
	assert.NoError(s.T(), db.First(&updatedForward, forward.ID).Error)
	assert.Equal(s.T(), model.ForwardRuntimeJobStatusFailed, updatedForward.RuntimeStatus)
	assert.Contains(s.T(), updatedForward.RuntimeMessage, context.Canceled.Error())
	assert.Equal(s.T(), model.ForwardStatusError, updatedForward.Status)
	assert.NotNil(s.T(), updatedForward.RuntimeLastSyncAt)
}

func TestPanelForwardRuntimeJobExecutor(t *testing.T) {
	suite.Run(t, new(PanelForwardRuntimeJobExecutorTestSuite))
}

func TestNewPanelForwardRuntimeJobExecutor_LoadsWorkerSettingsFromConfig(t *testing.T) {
	appconfig.Set(&appconfig.Config{
		ForwardRuntime: appconfig.ForwardRuntimeConfig{
			Jobs: appconfig.ForwardRuntimeJobsConfig{
				PollInterval:     "7s",
				IdlePollInterval: "2s",
				ErrorLogInterval: "90s",
				BatchSize:        4,
				TimeoutSeconds:   75,
			},
		},
	})
	defer appconfig.Set(nil)

	executor := NewPanelForwardRuntimeJobExecutor(&gorm.DB{})
	assert.Equal(t, 7*time.Second, executor.pollInterval)
	assert.Equal(t, 7*time.Second, executor.idlePollInterval)
	assert.Equal(t, 90*time.Second, executor.errorLogger.interval)
	assert.Equal(t, 4, executor.batchSize)
	assert.Equal(t, 75*time.Second, executor.jobTimeout)
}

func TestNewForwardAgentBridgeWorker_LoadsCleanAgentActionTimeoutFromConfig(t *testing.T) {
	appconfig.Set(&appconfig.Config{
		ForwardRuntime: appconfig.ForwardRuntimeConfig{
			CleanAgent: appconfig.ForwardRuntimeCleanAgentConfig{
				ActionTimeoutSeconds: 121,
			},
		},
	})
	defer appconfig.Set(nil)

	worker := NewForwardAgentBridgeWorker(&gorm.DB{})
	assert.Equal(t, 121*time.Second, worker.actionTimeout)
}

func TestForwardBackgroundErrorLogger_SuppressesRepeatedMessages(t *testing.T) {
	var buf bytes.Buffer
	originalWriter := log.Writer()
	originalFlags := log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(originalWriter)
		log.SetFlags(originalFlags)
	}()

	logger := newForwardBackgroundErrorLogger(time.Hour)
	logger.Logf("cycle", "forward runtime executor cycle failed: %v", errors.New("db unavailable"))
	logger.Logf("cycle", "forward runtime executor cycle failed: %v", errors.New("db unavailable"))
	logger.Logf("cycle", "forward runtime executor cycle failed: %v", errors.New("db unavailable again"))

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	assert.Len(t, lines, 2)
	assert.Contains(t, lines[0], "db unavailable")
	assert.Contains(t, lines[1], "db unavailable again")

	buf.Reset()
	logger.Clear("cycle")
	logger.Logf("cycle", "forward runtime executor cycle failed: %v", errors.New("db unavailable"))
	assert.Contains(t, strings.TrimSpace(buf.String()), "db unavailable")
}

func TestPanelForwardAnsibleRuntimePayload_CommandArgs_ValidatesInventoryAndPlaybook(t *testing.T) {
	payload := panelForwardAnsibleRuntimePayload{
		Action:    model.ForwardRuntimeJobActionCreate,
		Inventory: "",
		Playbook:  "/opt/ansible/apply.yml",
	}
	_, err := payload.commandArgs()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "inventory is required")

	payload.Inventory = "/etc/ansible/hosts"
	payload.Playbook = ""
	_, err = payload.commandArgs()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "playbook is required")
}

func TestPanelForwardAnsibleRuntimePayload_SuccessMessage(t *testing.T) {
	createPayload := panelForwardAnsibleRuntimePayload{Action: model.ForwardRuntimeJobActionCreate}
	assert.Equal(t, "ansible runtime synchronized", createPayload.successMessage())

	deletePayload := panelForwardAnsibleRuntimePayload{Action: model.ForwardRuntimeJobActionDelete}
	assert.Equal(t, "ansible runtime removed", deletePayload.successMessage())

	pausePayload := panelForwardAnsibleRuntimePayload{Action: model.ForwardRuntimeJobActionPause}
	assert.Equal(t, "ansible runtime removed", pausePayload.successMessage())

	updatePayload := panelForwardAnsibleRuntimePayload{Action: model.ForwardRuntimeJobActionUpdate}
	assert.Equal(t, "ansible runtime synchronized", updatePayload.successMessage())
}

func TestPanelForwardAnsibleRuntimePayload_BuildExtraVars_IncludesAllFields(t *testing.T) {
	payload := panelForwardAnsibleRuntimePayload{
		Action:         model.ForwardRuntimeJobActionCreate,
		Backend:        model.ForwardRuntimeBackendNftablesAnsible,
		FirewallDriver: "nftables",
		ExtraVars:      map[string]any{"custom_key": "custom_value"},
		Forward:        panelForwardAnsibleForwardPayload{ID: 1, Name: "fwd1"},
		Tunnel:         panelForwardAnsibleTunnelPayload{ID: 2, Name: "tun2"},
		Node:           panelForwardAnsibleNodePayload{ID: 3, Name: "node3"},
		Limiter:        &panelForwardLimiterPayload{SpeedID: 4, Speed: 100},
		Targets:        []panelForwardAnsibleTargetPayload{{Name: "target-1", Addr: "10.0.0.1:80"}},
	}

	extraVars := payload.buildExtraVars()
	assert.Equal(t, model.ForwardRuntimeJobActionCreate, extraVars["runtimeAction"])
	assert.Equal(t, model.ForwardRuntimeBackendNftablesAnsible, extraVars["runtimeBackend"])
	assert.Equal(t, "nftables", extraVars["firewallDriver"])
	assert.Equal(t, "custom_value", extraVars["custom_key"])
	assert.Equal(t, uint(1), extraVars["forwardId"])
	assert.Equal(t, uint(2), extraVars["tunnelId"])
	assert.Equal(t, uint(3), extraVars["nodeId"])
	assert.NotNil(t, extraVars["limiter"])
	assert.NotNil(t, extraVars["targets"])
}

func TestPanelForwardAnsibleRuntimePayload_LimitPattern_WithTemplate(t *testing.T) {
	payload := panelForwardAnsibleRuntimePayload{
		TargetPattern: "{{node.host}}:{{forward.id}}",
		Node:          panelForwardAnsibleNodePayload{Host: "node1.example.com", Name: "node1", ID: 5},
		Forward:       panelForwardAnsibleForwardPayload{ID: 10},
		Tunnel:        panelForwardAnsibleTunnelPayload{ID: 20},
	}
	limit := payload.limitPattern()
	assert.Equal(t, "node1.example.com:10", limit)
}

func TestPanelForwardAnsibleRuntimePayload_LimitPattern_FallsBackToNodeHost(t *testing.T) {
	payload := panelForwardAnsibleRuntimePayload{
		Node: panelForwardAnsibleNodePayload{Host: "node2.example.com"},
	}
	limit := payload.limitPattern()
	assert.Equal(t, "node2.example.com", limit)
}

func TestPanelForwardAnsibleRuntimePayload_LimitPattern_EmptyWhenNoTarget(t *testing.T) {
	payload := panelForwardAnsibleRuntimePayload{}
	assert.Empty(t, payload.limitPattern())
}

func TestNormalizeForwardIdlePollInterval(t *testing.T) {
	// When idleInterval > activeInterval, return idleInterval
	assert.Equal(t, 30*time.Second, normalizeForwardIdlePollInterval(10*time.Second, 30*time.Second))
	// When idleInterval < activeInterval, return activeInterval
	assert.Equal(t, 10*time.Second, normalizeForwardIdlePollInterval(10*time.Second, 5*time.Second))
	// When idleInterval is 0, return activeInterval
	assert.Equal(t, 10*time.Second, normalizeForwardIdlePollInterval(10*time.Second, 0))
	// When activeInterval is 0, return idleInterval
	assert.Equal(t, 10*time.Second, normalizeForwardIdlePollInterval(0, 10*time.Second))
}

func TestWaitForwardBackgroundCycleStopsOnContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan bool, 1)

	go func() {
		done <- waitForwardBackgroundCycle(ctx, time.Hour)
	}()

	time.Sleep(10 * time.Millisecond)
	cancel()

	select {
	case ok := <-done:
		assert.False(t, ok)
	case <-time.After(time.Second):
		t.Fatal("background cycle wait did not stop after context cancellation")
	}
}

func TestWaitForwardBackgroundCycleSkipsImmediateCycleWhenCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	assert.False(t, waitForwardBackgroundCycle(ctx, 0))
}

func TestPanelForwardRuntimeJobExecutor_RequeueRunningJobs(t *testing.T) {
	appconfig.Set(&appconfig.Config{
		Database: appconfig.DatabaseConfig{
			Driver:   "sqlite3",
			Database: ":memory:",
		},
	})
	defer appconfig.Set(nil)

	if err := database.Init(&appconfig.DatabaseConfig{
		Driver: "sqlite3", Database: ":memory:",
	}); err != nil {
		t.Skipf("database init failed: %v", err)
	}
	// Note: do NOT close the database here — it's a shared connection used by other tests.

	db := database.Get()
	assert.NoError(t, database.AutoMigrate(&model.Forward{}, &model.ForwardRuntimeJob{}))
	db.Exec("DELETE FROM v2_forward")
	db.Exec("DELETE FROM v2_forward_runtime_job")

	forward := &model.Forward{
		UserID:         1,
		Name:           "requeue-forward",
		TunnelID:       1,
		InPort:         12000,
		RemoteAddr:     "127.0.0.1:8080",
		Status:         model.ForwardStatusActive,
		RuntimeBackend: model.ForwardRuntimeBackendNftablesAnsible,
		RuntimeStatus:  model.ForwardRuntimeJobStatusRunning,
		RuntimeMessage: "ansible runtime applying",
	}
	assert.NoError(t, db.Create(forward).Error)
	startedAt := time.Now()
	runningJob := &model.ForwardRuntimeJob{
		Backend:   model.ForwardRuntimeBackendNftablesAnsible,
		Action:    model.ForwardRuntimeJobActionCreate,
		ForwardID: uintPtr(forward.ID),
		Status:    model.ForwardRuntimeJobStatusRunning,
		StartedAt: &startedAt,
	}
	assert.NoError(t, db.Create(runningJob).Error)

	executor := NewPanelForwardRuntimeJobExecutor(db)
	assert.NoError(t, executor.requeueRunningJobs())

	var updated model.ForwardRuntimeJob
	assert.NoError(t, db.First(&updated, runningJob.ID).Error)
	assert.Equal(t, model.ForwardRuntimeJobStatusPending, updated.Status)
	assert.Nil(t, updated.StartedAt)

	var updatedForward model.Forward
	assert.NoError(t, db.First(&updatedForward, forward.ID).Error)
	assert.Equal(t, model.ForwardRuntimeJobStatusPending, updatedForward.RuntimeStatus)
	assert.Equal(t, "ansible runtime requeued after worker restart", updatedForward.RuntimeMessage)
	assert.NotNil(t, updatedForward.RuntimeLastSyncAt)
}

func TestResolveAnsiblePlaybookCommand(t *testing.T) {
	tests := []struct {
		name    string
		command string
		want    string
		wantErr string
	}{
		{
			name:    "empty uses default",
			command: "",
			want:    defaultAnsibleCommand,
		},
		{
			name:    "plain ansible playbook",
			command: " ansible-playbook ",
			want:    defaultAnsibleCommand,
		},
		{
			name:    "absolute ansible playbook",
			command: "/usr/bin/ansible-playbook",
			want:    "/usr/bin/ansible-playbook",
		},
		{
			name:    "reject shell",
			command: "sh",
			wantErr: "unsupported ansible command",
		},
		{
			name:    "reject relative path",
			command: "bin/ansible-playbook",
			wantErr: "ansible command path must be absolute",
		},
		{
			name:    "reject newline",
			command: "ansible-playbook\nwhoami",
			wantErr: "invalid ansible command",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveAnsiblePlaybookCommand(tt.command)
			if tt.wantErr != "" {
				assert.Empty(t, got)
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
