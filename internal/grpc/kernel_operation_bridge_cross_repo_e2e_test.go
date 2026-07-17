package grpc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	agentv1pb "github.com/AnixOps/anix-agent/sdk/api/grpc/agent/v1"
	"github.com/AnixOps/anix-control/v4/internal/cache"
	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

// TestKernelOperationBridgeCrossRepositoryAgentProcess is an opt-in release
// gate. It builds the sibling V2bX_AnixOps Agent fixture, connects it to the
// real Control AgentControlGRPCServer, and drives one durable DB operation
// through KernelOperationBridge to a terminal observed state. Keeping this
// opt-in avoids making ordinary unit-test runs depend on a sibling checkout or
// a second Go module's toolchain.
func TestKernelOperationBridgeCrossRepositoryAgentProcess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("cross-repository Agent process harness requires Unix process signals")
	}
	if os.Getenv("ANIXOPS_CROSS_REPO_E2E") != "1" {
		t.Skip("set ANIXOPS_CROSS_REPO_E2E=1 to run the cross-repository Agent process gate")
	}

	agentRoot := siblingAgentRoot(t)
	if _, err := os.Stat(filepath.Join(agentRoot, "go.mod")); err != nil {
		t.Skipf("sibling V2bX_AnixOps checkout is unavailable: %v", err)
	}

	cache.InitMemory()
	databasePath := filepath.Join(t.TempDir(), "cross-repository.db")
	require.NoError(t, database.Init(&config.DatabaseConfig{
		Driver: "sqlite", Database: databasePath, LogLevel: "silent",
	}))
	requireAutoMigrate(t, &model.Node{})
	db := database.GetDB()
	require.NoError(t, service.EnsureKernelSchema(db))
	t.Cleanup(func() { requireDatabaseClosed(t) })

	const apiKey = "cross-repository-agent-key"
	nodeID := uint32(0)
	hash := sha256.Sum256([]byte(apiKey))
	node := model.Node{
		Name:       "cross-repository-agent",
		Host:       "127.0.0.1",
		APIKeyHash: hex.EncodeToString(hash[:]),
		Status:     model.NodeStatusOnline,
	}
	require.NoError(t, db.Create(&node).Error)
	nodeID = uint32(node.ID)

	pluginID := "machine-telemetry"
	artifactDigest := strings.Repeat("a", 64)
	manifest := service.PluginManifest{
		ID: pluginID, Name: "Machine Telemetry", Version: "1.0.0", APIVersion: "v1", Publisher: "AnixOps",
		Targets: []string{"agent"}, ArtifactSHA256: artifactDigest,
	}
	canonical, err := service.CanonicalPluginManifest(manifest)
	require.NoError(t, err)
	release := model.PluginRelease{
		PluginID: pluginID, Version: manifest.Version, APIVersion: manifest.APIVersion,
		ManifestJSON: string(canonical), ArtifactSHA256: artifactDigest, Signature: "cross-repo-test",
	}
	require.NoError(t, db.Create(&release).Error)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	manager := NewAgentControlManager()
	grpcServer := grpc.NewServer(grpc.ChainStreamInterceptor(StreamAuthInterceptor("", "")))
	agentv1pb.RegisterAgentControlServiceServer(grpcServer, NewAgentControlGRPCServer(manager))
	serverErr := serveGRPCServerForTest(t, grpcServer, listener)
	t.Cleanup(func() { stopGRPCServerForTest(t, grpcServer, serverErr) })

	fixtureBinary := buildCrossRepositoryAgentFixture(t, agentRoot)
	fixtureDir := t.TempDir()
	readyFile := filepath.Join(fixtureDir, "ready.json")
	resultFile := filepath.Join(fixtureDir, "result.json")
	var fixtureOutput synchronizedCrossRepoBuffer
	fixture := exec.Command(fixtureBinary,
		"--target", listener.Addr().String(), "--node-id", fmt.Sprint(nodeID), "--api-key", apiKey,
		"--ready-file", readyFile, "--result-file", resultFile, "--timeout", "45s",
	)
	fixture.Stdout = &fixtureOutput
	fixture.Stderr = &fixtureOutput
	require.NoError(t, fixture.Start())
	fixtureDone := make(chan error, 1)
	go func() { fixtureDone <- fixture.Wait() }()
	t.Cleanup(func() {
		if fixture.Process != nil {
			_ = fixture.Process.Signal(os.Interrupt)
		}
		select {
		case <-fixtureDone:
		case <-time.After(5 * time.Second):
			if fixture.Process != nil {
				_ = fixture.Process.Kill()
			}
			<-fixtureDone
		}
	})
	waitForCrossRepoFile(t, readyFile, 10*time.Second, &fixtureOutput)

	deadline := time.Now().Add(30 * time.Second)
	operation, _, err := service.CreateKernelOperation(db, model.KernelOperation{
		ID: uuid.NewString(), IdempotencyKey: "cross-repository-agent-health-1", NodeID: &node.ID,
		PluginID: pluginID, TargetVersion: manifest.Version, Kind: "plugin.health", ConfigJSON: `{}`,
		DeadlineAt: &deadline,
	})
	require.NoError(t, err)

	bridge, err := NewKernelOperationBridge(db, manager)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	var stored model.KernelOperation
	for {
		_, err = bridge.RunOnce(ctx)
		require.NoError(t, err)
		require.NoError(t, db.First(&stored, "id = ?", operation.ID).Error)
		if isTerminalCrossRepoOperation(stored.State) {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatalf("operation did not reach terminal state: state=%s error=%s fixture=%s", stored.State, stored.LastError, fixtureOutput.String())
		case <-time.After(50 * time.Millisecond):
		}
	}
	require.Equal(t, "succeeded", stored.State, fixtureOutput.String())
	require.JSONEq(t, `{"fixture":"agent-control","status":"ok"}`, stored.ResultJSON)

	var result struct {
		OperationID   string          `json:"operation_id"`
		Kind          string          `json:"kind"`
		SessionID     string          `json:"session_id"`
		Revision      uint64          `json:"revision"`
		PluginID      string          `json:"plugin_id"`
		TargetVersion string          `json:"target_version"`
		ConfigHash    string          `json:"config_hash"`
		Config        json.RawMessage `json:"config"`
	}
	waitForCrossRepoFile(t, resultFile, 5*time.Second, &fixtureOutput)
	resultBytes, err := os.ReadFile(resultFile)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(resultBytes, &result))
	require.Equal(t, operation.ID, result.OperationID)
	require.Equal(t, operation.Kind, result.Kind)
	require.NotEmpty(t, result.SessionID)
	require.Equal(t, uint64(operation.Revision), result.Revision)
	require.Equal(t, pluginID, result.PluginID)
	require.Equal(t, manifest.Version, result.TargetVersion)
	require.Equal(t, operation.ConfigHash, result.ConfigHash)
	require.JSONEq(t, `{}`, string(result.Config))

	snapshot, connected := manager.Connection(nodeID)
	require.True(t, connected)
	require.Equal(t, snapshot.SessionID, result.SessionID)
}

func siblingAgentRoot(t *testing.T) string {
	t.Helper()
	if configured := strings.TrimSpace(os.Getenv("ANIXOPS_AGENT_ROOT")); configured != "" {
		return filepath.Clean(configured)
	}
	_, sourceFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	controlRoot := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", ".."))
	candidates := []string{
		filepath.Join(filepath.Dir(controlRoot), "V2bX_AnixOps"),
		filepath.Join(controlRoot, "V2bX_AnixOps"),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(filepath.Join(candidate, "go.mod")); err == nil {
			return candidate
		}
	}
	return candidates[0]
}

func crossRepositoryAgentGo() string {
	if configured := strings.TrimSpace(os.Getenv("ANIXOPS_AGENT_GO")); configured != "" {
		return configured
	}
	return "go"
}

func crossRepositoryAgentBuildEnv() []string {
	env := append([]string(nil), os.Environ()...)
	if strings.TrimSpace(os.Getenv("GOWORK")) == "" {
		env = append(env, "GOWORK=off")
	}
	return append(env, "GOEXPERIMENT=jsonv2")
}

func buildCrossRepositoryAgentFixture(t *testing.T, agentRoot string) string {
	t.Helper()
	binaryPath := filepath.Join(t.TempDir(), "agent-control-fixture")
	command := exec.Command(crossRepositoryAgentGo(), "build", "-o", binaryPath, "./cmd/agent-control-fixture")
	command.Dir = agentRoot
	command.Env = crossRepositoryAgentBuildEnv()
	output, err := command.CombinedOutput()
	require.NoError(t, err, string(output))
	return binaryPath
}

func TestCrossRepositoryAgentBuildConfiguration(t *testing.T) {
	t.Setenv("ANIXOPS_AGENT_GO", "")
	require.Equal(t, "go", crossRepositoryAgentGo())
	t.Setenv("ANIXOPS_AGENT_GO", "/opt/anix-agent-go/bin/go")
	require.Equal(t, "/opt/anix-agent-go/bin/go", crossRepositoryAgentGo())

	t.Setenv("GOWORK", "")
	env := crossRepositoryAgentBuildEnv()
	require.Equal(t, "off", crossRepositoryBuildEnvironmentValue(env, "GOWORK"))
	require.Equal(t, "jsonv2", crossRepositoryBuildEnvironmentValue(env, "GOEXPERIMENT"))

	t.Setenv("GOWORK", "/tmp/anixops-sync.go.work")
	env = crossRepositoryAgentBuildEnv()
	require.Equal(t, "/tmp/anixops-sync.go.work", crossRepositoryBuildEnvironmentValue(env, "GOWORK"))
}

func crossRepositoryBuildEnvironmentValue(environment []string, key string) string {
	prefix := key + "="
	value := ""
	for _, entry := range environment {
		if strings.HasPrefix(entry, prefix) {
			value = strings.TrimPrefix(entry, prefix)
		}
	}
	return value
}

func waitForCrossRepoFile(t *testing.T, path string, timeout time.Duration, output interface{ String() string }) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s; fixture output: %s", path, output.String())
}

func isTerminalCrossRepoOperation(state string) bool {
	switch state {
	case "succeeded", "failed", "superseded", "cancelled", "timed_out":
		return true
	default:
		return false
	}
}

type synchronizedCrossRepoBuffer struct {
	mu   sync.Mutex
	data []byte
}

func (b *synchronizedCrossRepoBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.data = append(b.data, data...)
	return len(data), nil
}

func (b *synchronizedCrossRepoBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(append([]byte(nil), b.data...))
}
