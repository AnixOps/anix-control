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

	appconfig "github.com/anixops/v2board/internal/config"
	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"
)

type stubPanelForwardRuntimeCommandRunner struct {
	output      string
	err         error
	runs        int
	lastCommand string
	lastArgs    []string
	lastWorkdir string
	lastEnv     map[string]string
}

func (r *stubPanelForwardRuntimeCommandRunner) Run(ctx context.Context, command string, args []string, workdir string, env map[string]string) (string, error) {
	_ = ctx
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
	return r.output, r.err
}

type PanelForwardRuntimeJobExecutorTestSuite struct {
	ServiceTestSuite
	executor *PanelForwardRuntimeJobExecutor
	runner   *stubPanelForwardRuntimeCommandRunner
}

func (s *PanelForwardRuntimeJobExecutorTestSuite) SetupSuite() {
	s.ServiceTestSuite.SetupSuite()
	database.AutoMigrate(
		&model.Forward{},
		&model.ForwardRuntimeJob{},
	)
}

func (s *PanelForwardRuntimeJobExecutorTestSuite) SetupTest() {
	s.ServiceTestSuite.SetupTest()
	appconfig.Set(nil)

	db := database.Get()
	db.Exec("DELETE FROM v2_forward_runtime_job")
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

func (s *PanelForwardRuntimeJobExecutorTestSuite) TestRunPendingJobs_NoPendingJobsSkipsRunner() {
	err := s.executor.RunPendingJobs(context.Background())
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), 0, s.runner.runs)
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
	database.AutoMigrate(&model.ForwardRuntimeJob{})
	db.Exec("DELETE FROM v2_forward_runtime_job")

	runningJob := &model.ForwardRuntimeJob{
		Backend: model.ForwardRuntimeBackendNftablesAnsible,
		Action:  model.ForwardRuntimeJobActionCreate,
		Status:  model.ForwardRuntimeJobStatusRunning,
	}
	assert.NoError(t, db.Create(runningJob).Error)

	executor := NewPanelForwardRuntimeJobExecutor(db)
	assert.NoError(t, executor.requeueRunningJobs())

	var updated model.ForwardRuntimeJob
	assert.NoError(t, db.First(&updated, runningJob.ID).Error)
	assert.Equal(t, model.ForwardRuntimeJobStatusPending, updated.Status)
	assert.Nil(t, updated.StartedAt)
}
