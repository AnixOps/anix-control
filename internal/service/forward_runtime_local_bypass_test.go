package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anixops/v2board/internal/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type minimalForwardRuntimeNodeXExecutor struct {
	calls int
}

func (e *minimalForwardRuntimeNodeXExecutor) Execute(context.Context, nodeXForwardExecuteRequest) (*nodeXForwardExecuteResult, error) {
	e.calls++
	return &nodeXForwardExecuteResult{
		Backend: model.ForwardRuntimeBackendGost,
		Status:  model.ForwardRuntimeJobStatusSuccess,
		Message: "unexpected nodex execution",
	}, nil
}

type minimalForwardRuntimeHTTPDoer struct {
	lastRequest *http.Request
}

func (d *minimalForwardRuntimeHTTPDoer) Do(req *http.Request) (*http.Response, error) {
	d.lastRequest = req
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body: io.NopCloser(strings.NewReader(
			`{"data":{"backend":"iptables_ansible","status":2,"message":"ok"}}`,
		)),
	}, nil
}

func newMinimalForwardRuntimeTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}

	if err := db.AutoMigrate(
		&model.SystemConfig{},
		&model.ForwardNode{},
		&model.ForwardTunnel{},
		&model.ForwardUserTunnel{},
		&model.SpeedLimit{},
		&model.Forward{},
		&model.ForwardRuntimeJob{},
	); err != nil {
		t.Fatalf("auto migrate runtime tables: %v", err)
	}

	return db
}

func TestPanelForwardRuntimeService_LocalAnsibleBypassQueuesDirectPayload(t *testing.T) {
	db := newMinimalForwardRuntimeTestDB(t)
	pathExists = func(raw string) bool { return true }
	configService := NewSystemConfigService(db)

	tempDir := t.TempDir()
	inventoryPath := filepath.Join(tempDir, "inventory.ini")
	applyPlaybookPath := filepath.Join(tempDir, "apply.yml")
	removePlaybookPath := filepath.Join(tempDir, "remove.yml")
	if err := os.WriteFile(inventoryPath, []byte("[forward_nodes]\n"), 0o600); err != nil {
		t.Fatalf("write inventory: %v", err)
	}
	if err := os.WriteFile(applyPlaybookPath, []byte("---\n- hosts: all\n"), 0o600); err != nil {
		t.Fatalf("write playbook: %v", err)
	}
	if err := os.WriteFile(removePlaybookPath, []byte("---\n- hosts: all\n"), 0o600); err != nil {
		t.Fatalf("write remove playbook: %v", err)
	}

	if err := configService.Set(
		forwardRuntimeBackendConfigKey,
		model.ForwardRuntimeBackendIptablesAnsible,
		"string",
		forwardRuntimeConfigGroup,
		"forward runtime backend",
	); err != nil {
		t.Fatalf("set runtime backend: %v", err)
	}
	if err := configService.SetJSON(
		forwardRuntimeAnsibleConfigJSONKey,
		panelForwardAnsibleConfig{
			Inventory:      inventoryPath,
			ApplyPlaybook:  applyPlaybookPath,
			RemovePlaybook: removePlaybookPath,
			WorkingDir:     tempDir,
		},
		forwardRuntimeConfigGroup,
		"forward runtime ansible config",
	); err != nil {
		t.Fatalf("set ansible config: %v", err)
	}

	node := &model.ForwardNode{
		Name:     "Local Queue Node",
		Type:     model.ForwardNodeTypeRelay,
		Host:     "203.0.113.10",
		Port:     22,
		APIPort:  19080,
		APIToken: "node-token",
		Enabled:  true,
	}
	if err := db.Create(node).Error; err != nil {
		t.Fatalf("create node: %v", err)
	}

	tunnel := &model.ForwardTunnel{
		Name:          "Queue Tunnel",
		InNodeID:      node.ID,
		Protocol:      "tcp",
		TCPListenAddr: "0.0.0.0",
		Status:        model.ForwardTunnelStatusActive,
	}
	if err := db.Create(tunnel).Error; err != nil {
		t.Fatalf("create tunnel: %v", err)
	}

	forward := &model.Forward{
		UserID:         1,
		UserName:       "queue@example.com",
		Name:           "Queued Forward",
		TunnelID:       tunnel.ID,
		InPort:         20001,
		RemoteAddr:     "example.com:443",
		Status:         model.ForwardStatusActive,
		RuntimeBackend: model.ForwardRuntimeBackendIptablesAnsible,
	}
	if err := db.Create(forward).Error; err != nil {
		t.Fatalf("create forward: %v", err)
	}

	executor := &minimalForwardRuntimeNodeXExecutor{}
	svc := NewPanelForwardRuntimeService(db)
	svc.client = executor

	result, err := svc.Apply(context.Background(), model.ForwardRuntimeJobActionCreate, forward, tunnel)
	if err != nil {
		t.Fatalf("apply runtime: %v", err)
	}
	if executor.calls != 0 {
		t.Fatalf("expected local ansible bypass without NodeX execution, got %d calls", executor.calls)
	}
	if result == nil || result.Status != model.ForwardRuntimeJobStatusPending || !result.Async {
		t.Fatalf("expected pending async runtime result, got %+v", result)
	}

	var job model.ForwardRuntimeJob
	if err := db.Last(&job).Error; err != nil {
		t.Fatalf("load runtime job: %v", err)
	}
	if job.Status != model.ForwardRuntimeJobStatusPending {
		t.Fatalf("expected pending job status, got %d", job.Status)
	}
	if job.StartedAt != nil {
		t.Fatalf("expected queued job without started_at, got %v", job.StartedAt)
	}
	if strings.Contains(job.Payload, "\"ansibleRuntime\"") {
		t.Fatalf("expected direct ansible payload, got wrapped payload: %s", job.Payload)
	}

	var payload panelForwardAnsibleRuntimePayload
	if err := json.Unmarshal([]byte(job.Payload), &payload); err != nil {
		t.Fatalf("decode queued ansible payload: %v", err)
	}
	if payload.Inventory == "" || payload.Playbook == "" {
		t.Fatalf("expected ansible inventory and playbook paths, got: %+v", payload)
	}
	if payload.Forward.ID != forward.ID || payload.Node.ID != node.ID {
		t.Fatalf("unexpected payload references: %+v", payload)
	}
}

func TestNodeXForwardRuntimeClient_AnsiblePanelForwardFallsBackToIngressNode(t *testing.T) {
	db := newMinimalForwardRuntimeTestDB(t)
	doer := &minimalForwardRuntimeHTTPDoer{}
	client := &nodeXForwardRuntimeClient{
		configService: NewSystemConfigService(db),
		httpClient:    doer,
	}

	_, err := client.Execute(context.Background(), nodeXForwardExecuteRequest{
		ResourceType: nodeXForwardResourceTypePanelForward,
		Backend:      model.ForwardRuntimeBackendIptablesAnsible,
		Action:       model.ForwardRuntimeJobActionCreate,
		PanelForward: &nodeXPanelForwardRequest{
			Forward: nodeXPanelForwardPayload{
				ID:         1,
				UserID:     2,
				Name:       "panel-forward",
				InPort:     10001,
				RemoteAddr: "198.51.100.20:443",
				Strategy:   "fifo",
				Status:     model.ForwardStatusActive,
			},
			Tunnel: nodeXPanelTunnelPayload{
				ID:            3,
				Name:          "tunnel",
				InNodeID:      4,
				Protocol:      "tcp",
				TCPListenAddr: "0.0.0.0",
			},
			IngressNode: nodeXForwardNodePayload{
				ID:       4,
				Name:     "relay",
				Host:     "203.0.113.10",
				Port:     22,
				APIPort:  19090,
				APIToken: "relay-token",
			},
		},
		AnsibleRuntime: &panelForwardAnsibleRuntimePayload{
			Action:    model.ForwardRuntimeJobActionCreate,
			Inventory: "/etc/ansible/hosts",
			Playbook:  "/opt/ansible/apply.yml",
			Node: panelForwardAnsibleNodePayload{
				ID:      4,
				Name:    "relay",
				Host:    "203.0.113.10",
				Port:    22,
				APIPort: 19090,
			},
		},
	})
	if err == nil {
		t.Fatalf("expected NodeX control plane error")
	}
	if doer.lastRequest != nil {
		t.Fatalf("expected no HTTP request when NodeX control plane is not configured")
	}
}
