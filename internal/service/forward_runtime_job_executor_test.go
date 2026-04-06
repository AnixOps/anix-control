package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"testing"
	"time"

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
		ExtraVars: map[string]interface{}{
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
	assert.Equal(s.T(), "/opt/ansible", s.runner.lastWorkdir)
	assert.Equal(s.T(), "/etc/ansible/ansible.cfg", s.runner.lastEnv["ANSIBLE_CONFIG"])
	assert.Contains(s.T(), s.runner.lastArgs, "-i")
	assert.Contains(s.T(), s.runner.lastArgs, "/etc/ansible/hosts")
	assert.Contains(s.T(), s.runner.lastArgs, "--limit")
	assert.Contains(s.T(), s.runner.lastArgs, "relay-a.example.com")
	assert.Contains(s.T(), s.runner.lastArgs, "--become")
	assert.Contains(s.T(), s.runner.lastArgs, "/opt/ansible/apply.yml")

	var extraVarsJSON string
	for idx := 0; idx < len(s.runner.lastArgs)-1; idx++ {
		if s.runner.lastArgs[idx] == "--extra-vars" {
			extraVarsJSON = s.runner.lastArgs[idx+1]
			break
		}
	}
	assert.NotEmpty(s.T(), extraVarsJSON)

	var extraVars map[string]interface{}
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

func TestNewPanelForwardRuntimeJobExecutor_NormalizesIdleIntervalFromEnv(t *testing.T) {
	t.Setenv(forwardRuntimeJobPollIntervalEnvVar, "7s")
	t.Setenv(forwardRuntimeJobIdlePollIntervalEnvVar, "2s")
	t.Setenv(forwardRuntimeJobErrorLogIntervalEnvVar, "90s")

	executor := NewPanelForwardRuntimeJobExecutor(&gorm.DB{})
	assert.Equal(t, 7*time.Second, executor.pollInterval)
	assert.Equal(t, 7*time.Second, executor.idlePollInterval)
	assert.Equal(t, 90*time.Second, executor.errorLogger.interval)
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
