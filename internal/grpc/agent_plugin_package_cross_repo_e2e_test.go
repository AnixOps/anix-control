package grpc_test

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	agentv1pb "github.com/AnixOps/anix-agent/sdk/api/grpc/agent/v1"
	agentplugin "github.com/AnixOps/anix-agent/sdk/plugincontrol"
	"github.com/AnixOps/anix-control/v4/internal/cache"
	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/database"
	controlgrpc "github.com/AnixOps/anix-control/v4/internal/grpc"
	"github.com/AnixOps/anix-control/v4/internal/handler"
	"github.com/AnixOps/anix-control/v4/internal/middleware"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/plugincontrol"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	googlegrpc "google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"gorm.io/gorm"
)

const crossRepoPluginE2EArtifactMinimum = 4 << 20

func TestAgentPluginPackageCrossRepoContract(t *testing.T) {
	operation := agentplugin.Operation{
		ID:             "agent-package-operation-42",
		PackageID:      "wireguard",
		PackageVersion: "4.0.0",
		Generation:     9,
		IdempotencyKey: "agent-package-key-42",
		Kind:           "configure",
		ConfigJSON:     []byte(`{"interface":"wg0"}`),
	}
	require.NoError(t, agentplugin.ValidateOperation(operation))

	status := agentplugin.RuntimeStatus{
		PackageID: operation.PackageID, PackageVersion: operation.PackageVersion,
		Generation: operation.Generation, Healthy: true, ObservedHash: "wireguard-config-sha256",
	}
	require.Equal(t, operation.PackageID, status.PackageID)
	require.Equal(t, operation.PackageVersion, status.PackageVersion)
	require.Equal(t, operation.Generation, status.Generation)
	require.True(t, status.Healthy)
}

// TestAgentPluginPackageCrossRepositoryE2E is the release gate for the real
// Control package transport and Agent Supervisor. It intentionally builds the
// sibling Agent binaries instead of substituting an in-process fake.
func TestAgentPluginPackageCrossRepositoryE2E(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the production plugin runtime requires Unix sockets")
	}
	if testing.Short() {
		t.Skip("builds and launches real sibling Agent binaries")
	}
	if os.Getenv("ANIXOPS_CROSS_REPO_E2E") != "1" {
		t.Skip("set ANIXOPS_CROSS_REPO_E2E=1 to run the cross-repository package gate")
	}

	agentRoot := crossRepositorySiblingAgentRoot(t)
	require.FileExists(t, filepath.Join(agentRoot, "go.mod"))
	fixtureBinary := buildCrossRepositoryAgentFixtureBinary(t, agentRoot)
	telemetryBinary := buildCrossRepositoryMachineTelemetry(t, agentRoot)
	artifact, entrypoint, webUI := buildCrossRepositoryMachineTelemetryTar(t, telemetryBinary)
	require.Greater(t, len(artifact), crossRepoPluginE2EArtifactMinimum)

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	previousConfig := config.Get()
	config.Set(&config.Config{Plugins: config.PluginConfig{
		OfficialPublicKey: base64.StdEncoding.EncodeToString(publicKey), ControlExecutionEnabled: true,
	}})
	t.Cleanup(func() { config.Set(previousConfig) })
	manifest := crossRepositoryMachineTelemetryManifest(artifact, entrypoint, webUI)
	canonicalManifest, err := service.CanonicalPluginManifest(manifest)
	require.NoError(t, err)
	signature := base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, canonicalManifest))

	cache.InitMemory()
	databasePath := filepath.Join(t.TempDir(), "agent-package-cross-repository.db")
	require.NoError(t, database.Init(&config.DatabaseConfig{
		Driver: "sqlite", Database: databasePath, LogLevel: "silent",
	}))
	require.NoError(t, database.AutoMigrate(&model.Node{}))
	db := database.GetDB()
	require.NoError(t, service.EnsureKernelSchema(db))
	t.Cleanup(func() { require.NoError(t, database.Close()) })

	release, err := service.RegisterPluginRelease(db, string(canonicalManifest), signature, publicKey)
	require.NoError(t, err)
	storedArtifact, err := service.StorePluginArtifact(db, release.ID, artifact)
	require.NoError(t, err)
	require.Greater(t, storedArtifact.SizeBytes, int64(crossRepoPluginE2EArtifactMinimum))
	require.NoError(t, db.Create(&model.PluginInstallation{
		PluginID: manifest.ID, Target: "control", DesiredVersion: manifest.Version,
		ObservedVersion: manifest.Version, State: "healthy", Enabled: true,
	}).Error)

	const (
		nodeAPIKey      = "cross-repository-package-node-key"
		wrongNodeAPIKey = "cross-repository-wrong-node-key"
	)
	node := createCrossRepositoryPluginNode(t, db, "cross-repository-package-node", nodeAPIKey)
	createCrossRepositoryPluginNode(t, db, "cross-repository-wrong-node", wrongNodeAPIKey)
	installation, assignment, configJSON := createCrossRepositoryPluginAssignment(t, db, node, manifest)
	chain, err := service.QueueAgentAssignmentLifecycle(db, assignment, time.Now())
	require.NoError(t, err)
	require.NotNil(t, chain.Install)
	require.NotNil(t, chain.Update)
	require.NotNil(t, chain.Enable)
	require.Equal(t, chain.Install.ID, chain.Update.DependsOnOperationID)
	require.Equal(t, chain.Update.ID, chain.Enable.DependsOnOperationID)

	httpServer := newCrossRepositoryPluginHTTPServer(t)
	installConfig, err := service.BuildAgentPluginInstallConfig(db, node.ID, manifest.ID, manifest.Version)
	require.NoError(t, err)
	assertCrossRepositoryWrongNodeRejected(t, httpServer.URL, wrongNodeAPIKey, installConfig)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	manager := controlgrpc.NewAgentControlManager()
	grpcServer := googlegrpc.NewServer(googlegrpc.ChainStreamInterceptor(controlgrpc.StreamAuthInterceptor("", "")))
	// The production server advertises a conservative 20-second heartbeat. The
	// process fixture uses this test-only stream wrapper to reduce the interval
	// to one second, keeping this release gate deterministic without changing
	// the production heartbeat contract.
	agentv1pb.RegisterAgentControlServiceServer(grpcServer, &crossRepositoryFastHeartbeatServer{
		delegate: controlgrpc.NewAgentControlGRPCServer(manager),
	})
	serverErr := serveCrossRepositoryGRPCServer(grpcServer, listener)
	t.Cleanup(func() { stopCrossRepositoryGRPCServer(t, grpcServer, serverErr) })

	bridge, err := controlgrpc.NewKernelOperationBridge(db, manager)
	require.NoError(t, err)
	fixtureRoot := t.TempDir()
	socketDir, err := os.MkdirTemp("", "anixops-plugin-e2e-sockets-")
	require.NoError(t, err)
	require.NoError(t, os.Chmod(socketDir, 0o700))
	t.Cleanup(func() { require.NoError(t, os.RemoveAll(socketDir)) })
	readyFile := filepath.Join(fixtureRoot, "ready.json")
	resultFile := filepath.Join(fixtureRoot, "result.json")
	pluginRoot := filepath.Join(fixtureRoot, "plugins")
	var fixtureOutput crossRepositorySynchronizedBuffer
	fixture := exec.Command(fixtureBinary,
		"--target", listener.Addr().String(), "--node-id", fmt.Sprint(node.ID), "--api-key", nodeAPIKey,
		"--ready-file", readyFile, "--result-file", resultFile, "--timeout", "2m",
		"--plugin-root", pluginRoot, "--plugin-socket-dir", socketDir,
		"--plugin-public-key", base64.StdEncoding.EncodeToString(publicKey), "--plugin-base-url", httpServer.URL,
	)
	fixture.Stdout = &fixtureOutput
	fixture.Stderr = &fixtureOutput
	require.NoError(t, fixture.Start())
	fixtureDone := make(chan error, 1)
	go func() { fixtureDone <- fixture.Wait() }()
	t.Cleanup(func() { stopCrossRepositoryPluginFixture(t, fixture, fixtureDone, &fixtureOutput) })
	waitForCrossRepositoryFile(t, readyFile, 15*time.Second, &fixtureOutput)

	waitForCrossRepositoryOperationChain(t, db, bridge, resultFile, &fixtureOutput, chain.Install, chain.Update, chain.Enable)
	pluginDir := filepath.Join(pluginRoot, manifest.ID, manifest.Version)
	assertCrossRepositoryImmutablePluginPackage(t, pluginDir, manifest.Version, artifact, canonicalManifest, signature, configJSON)
	assertCrossRepositoryPluginHealth(t, filepath.Join(socketDir, manifest.ID+".sock"))
	state := readCrossRepositoryPluginState(t, pluginRoot, manifest.ID)
	require.Equal(t, manifest.Version, state.DesiredVersion)
	require.Equal(t, manifest.Version, state.ObservedVersion)
	require.True(t, state.Enabled)
	require.Equal(t, "healthy", state.Health)
	require.Equal(t, uint64(chain.Enable.Revision), state.ObservedRevision)
	require.Empty(t, state.LastError)

	waitForCrossRepositoryPluginTelemetry(t, db, node.ID, 15*time.Second)
	status := getCrossRepositoryMachineTelemetryStatus(t, httpServer.URL)
	var telemetryNode *plugincontrol.MachineTelemetryNode
	for index := range status.Nodes {
		if status.Nodes[index].ID == node.ID {
			telemetryNode = &status.Nodes[index]
			break
		}
	}
	require.NotNil(t, telemetryNode)
	require.True(t, telemetryNode.TelemetryAvailable)
	require.Equal(t, "plugin", telemetryNode.TelemetrySource)
	require.False(t, telemetryNode.TelemetryStale)
	require.GreaterOrEqual(t, telemetryNode.CPUUsage, float64(0))
	require.LessOrEqual(t, telemetryNode.CPUUsage, float64(100))
	require.GreaterOrEqual(t, telemetryNode.MemoryUsage, float64(0))
	require.LessOrEqual(t, telemetryNode.MemoryUsage, float64(100))
	require.GreaterOrEqual(t, telemetryNode.DiskUsage, float64(0))
	require.LessOrEqual(t, telemetryNode.DiskUsage, float64(100))
	require.GreaterOrEqual(t, telemetryNode.Uptime, int64(0))

	require.NoError(t, db.Model(&model.NodeServiceAssignment{}).Where("id = ?", assignment.ID).
		Updates(map[string]any{"enabled": false, "lifecycle_generation": assignment.LifecycleGeneration + 1}).Error)
	require.NoError(t, db.First(&assignment, assignment.ID).Error)
	disableChain, err := service.QueueAgentAssignmentLifecycle(db, assignment, time.Now())
	require.NoError(t, err)
	require.NotNil(t, disableChain.Disable)
	waitForCrossRepositoryOperationChain(t, db, bridge, resultFile, &fixtureOutput, disableChain.Disable)
	waitForCrossRepositorySocketRemoval(t, filepath.Join(socketDir, manifest.ID+".sock"), 5*time.Second)
	state = readCrossRepositoryPluginState(t, pluginRoot, manifest.ID)
	require.False(t, state.Enabled)
	require.Equal(t, "disabled", state.Health)
	require.Equal(t, uint64(disableChain.Disable.Revision), state.ObservedRevision)
	require.Empty(t, state.LastError)

	var persistedInstallation model.PluginInstallation
	require.NoError(t, db.First(&persistedInstallation, installation.ID).Error)
	require.Equal(t, manifest.Version, persistedInstallation.DesiredVersion)
}

type crossRepositoryFastHeartbeatServer struct {
	agentv1pb.UnimplementedAgentControlServiceServer
	delegate *controlgrpc.AgentControlGRPCServer
}

func (s *crossRepositoryFastHeartbeatServer) ControlStream(stream agentv1pb.AgentControlService_ControlStreamServer) error {
	return s.delegate.ControlStream(&crossRepositoryFastHeartbeatStream{AgentControlService_ControlStreamServer: stream})
}

type crossRepositoryFastHeartbeatStream struct {
	agentv1pb.AgentControlService_ControlStreamServer
}

func (s *crossRepositoryFastHeartbeatStream) Send(message *agentv1pb.ControlToAgent) error {
	if helloAck := message.GetHelloAck(); helloAck != nil {
		helloAck.HeartbeatIntervalSeconds = 1
	}
	return s.AgentControlService_ControlStreamServer.Send(message)
}

func buildCrossRepositoryMachineTelemetry(t *testing.T, agentRoot string) string {
	t.Helper()
	binaryPath := filepath.Join(t.TempDir(), "machine-telemetry")
	command := exec.Command(crossRepositoryAgentGo(), "build", "-o", binaryPath, "./cmd/machine-telemetry")
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

func buildCrossRepositoryMachineTelemetryTar(t *testing.T, binaryPath string) ([]byte, string, []byte) {
	t.Helper()
	binary, err := os.ReadFile(binaryPath)
	require.NoError(t, err)
	entrypoint := filepath.ToSlash(filepath.Join("agent", runtime.GOOS+"-"+runtime.GOARCH, "plugin"))
	webUI := []byte(`export default { name: "MachineTelemetry" }`)
	var artifact bytes.Buffer
	writer := tar.NewWriter(&artifact)
	writeCrossRepositoryTarEntry(t, writer, entrypoint, 0o755, binary)
	writeCrossRepositoryTarEntry(t, writer, "webui/index.mjs", 0o644, webUI)
	if artifact.Len() <= crossRepoPluginE2EArtifactMinimum {
		padding := bytes.Repeat([]byte{0x5a}, crossRepoPluginE2EArtifactMinimum-artifact.Len()+64*1024)
		writeCrossRepositoryTarEntry(t, writer, "payload/e2e-padding.bin", 0o600, padding)
	}
	require.NoError(t, writer.Close())
	require.Greater(t, artifact.Len(), crossRepoPluginE2EArtifactMinimum)
	return artifact.Bytes(), entrypoint, webUI
}

func writeCrossRepositoryTarEntry(t *testing.T, writer *tar.Writer, name string, mode int64, contents []byte) {
	t.Helper()
	require.NoError(t, writer.WriteHeader(&tar.Header{
		Name: name, Mode: mode, Size: int64(len(contents)), Typeflag: tar.TypeReg,
	}))
	_, err := writer.Write(contents)
	require.NoError(t, err)
}

func crossRepositoryMachineTelemetryManifest(artifact []byte, entrypoint string, webUI []byte) service.PluginManifest {
	artifactDigest := sha256.Sum256(artifact)
	webUIDigest := sha256.Sum256(webUI)
	return service.PluginManifest{
		ID: "machine-telemetry", Name: "Machine Telemetry", Version: "1.1.0", APIVersion: "v1", Publisher: "AnixOps",
		Targets: []string{"control", "agent"}, Architectures: []string{runtime.GOOS + "/" + runtime.GOARCH},
		ArtifactSHA256: hex.EncodeToString(artifactDigest[:]), Capabilities: []string{"telemetry.read"},
		Permissions:    []string{"machine-telemetry.view", service.PluginAPIPermission("machine-telemetry")},
		ControlRoutes:  []string{plugincontrol.MachineTelemetryStatusRoute},
		ConfigSchema:   json.RawMessage(`{"additionalProperties":false,"properties":{"interval_seconds":{"maximum":3600,"minimum":5,"type":"integer"}},"type":"object"}`),
		Entrypoints:    map[string]string{"agent-" + runtime.GOOS + "-" + runtime.GOARCH: entrypoint},
		FrontendSHA256: hex.EncodeToString(webUIDigest[:]),
		WebUI: &service.PluginWebUI{
			Bundle:      service.PluginWebUIBundle{Path: "webui/index.mjs", SHA256: hex.EncodeToString(webUIDigest[:])},
			Permissions: []string{"machine-telemetry.view"},
			Menus: []service.PluginWebUIMenu{{
				ID: "machine-telemetry.main", Parent: "services", Label: "Machine Telemetry", Icon: "activity",
				Route: "/admin/extensions/machine-telemetry", Permission: "machine-telemetry.view", Order: 100,
			}},
			Routes: []service.PluginWebUIRoute{{
				ID: "machine-telemetry.main", Path: "/admin/extensions/machine-telemetry", Export: "default",
				Permission: "machine-telemetry.view",
			}},
		},
	}
}

func createCrossRepositoryPluginNode(t *testing.T, db *gorm.DB, name, apiKey string) model.Node {
	t.Helper()
	digest := sha256.Sum256([]byte(apiKey))
	node := model.Node{
		Name: name, Host: "127.0.0.1", APIKey: apiKey,
		APIKeyHash: hex.EncodeToString(digest[:]), Status: model.NodeStatusOnline,
	}
	require.NoError(t, db.Create(&node).Error)
	return node
}

func createCrossRepositoryPluginAssignment(t *testing.T, db *gorm.DB, node model.Node, manifest service.PluginManifest) (model.PluginInstallation, model.NodeServiceAssignment, string) {
	t.Helper()
	installation := model.PluginInstallation{
		PluginID: manifest.ID, Target: "agent", DesiredVersion: manifest.Version,
		State: "pending", Enabled: true, LifecycleGeneration: 1,
	}
	require.NoError(t, db.Create(&installation).Error)
	configJSON, err := service.CanonicalKernelOperationConfig(`{"interval_seconds":5}`)
	require.NoError(t, err)
	configHash, err := service.HashKernelOperationConfig(configJSON)
	require.NoError(t, err)
	configuration := model.PluginConfiguration{
		InstallationID: installation.ID, Revision: 1, ConfigJSON: configJSON, ConfigHash: configHash, UpdatedBy: 1,
	}
	require.NoError(t, db.Create(&configuration).Error)
	assignment := model.NodeServiceAssignment{
		NodeID: node.ID, ServiceScope: "subscription", PluginID: manifest.ID, Role: "telemetry",
		DesiredVersion: manifest.Version, DesiredConfigRevision: configuration.Revision,
		Enabled: true, LifecycleGeneration: 1,
	}
	require.NoError(t, db.Create(&assignment).Error)
	return installation, assignment, configJSON
}

func newCrossRepositoryPluginHTTPServer(t *testing.T) *httptest.Server {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	packages := router.Group("/api/v3/agent/plugin-releases")
	packages.Use(middleware.NodeAPIKeyHeaderAuth())
	kernel := handler.NewKernelHandler()
	packages.GET("/:plugin_id/:version/artifact", kernel.ServeAgentPluginArtifact)
	packages.GET("/:plugin_id/:version/manifest", kernel.ServeAgentPluginManifest)
	router.GET("/api/v3/plugins/:plugin_id/*route", func(c *gin.Context) {
		c.Set("user_id", uint(1))
		c.Set("is_admin", true)
		kernel.PluginRouteGateway(c)
	})
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	return server
}

func getCrossRepositoryMachineTelemetryStatus(t *testing.T, origin string) plugincontrol.MachineTelemetryStatus {
	t.Helper()
	client := &http.Client{Timeout: 5 * time.Second}
	response, err := client.Get(origin + plugincontrol.MachineTelemetryStatusRoute + "?limit=200")
	require.NoError(t, err)
	defer func() { require.NoError(t, response.Body.Close()) }()
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.StatusCode, string(body))
	var payload struct {
		Data plugincontrol.MachineTelemetryStatus `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &payload))
	require.Equal(t, plugincontrol.MachineTelemetryPluginID, payload.Data.PluginID)
	return payload.Data
}

func assertCrossRepositoryWrongNodeRejected(t *testing.T, origin, apiKey, installConfig string) {
	t.Helper()
	var payload service.AgentPluginInstallConfig
	require.NoError(t, json.Unmarshal([]byte(installConfig), &payload))
	request, err := http.NewRequest(http.MethodGet, origin+payload.Artifact.URL, nil)
	require.NoError(t, err)
	request.Header.Set("X-API-Key", apiKey)
	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, response.Body.Close())
	}()
	_, err = io.Copy(io.Discard, response.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusForbidden, response.StatusCode)
}

func waitForCrossRepositoryOperationChain(t *testing.T, db *gorm.DB, bridge *controlgrpc.KernelOperationBridge, resultFile string, output interface{ String() string }, operations ...*model.KernelOperation) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
	defer cancel()
	for {
		runCtx, runCancel := context.WithTimeout(ctx, 10*time.Second)
		_, err := bridge.RunOnce(runCtx)
		runCancel()
		if err != nil {
			states := make([]string, 0, len(operations))
			for _, operation := range operations {
				var stored model.KernelOperation
				if loadErr := db.First(&stored, "id = ?", operation.ID).Error; loadErr == nil {
					states = append(states, stored.Kind+"="+stored.State+":"+stored.LastError)
				}
			}
			result, _ := os.ReadFile(resultFile)
			t.Fatalf("bridge dispatch failed: %v; states=%v; fixture_result=%s; fixture=%s", err, states, result, output.String())
		}
		complete := true
		for _, operation := range operations {
			var stored model.KernelOperation
			require.NoError(t, db.First(&stored, "id = ?", operation.ID).Error)
			if stored.State == "failed" || stored.State == "superseded" || stored.State == "cancelled" || stored.State == "timed_out" {
				t.Fatalf("operation %s (%s) ended in %s: %s; fixture=%s", stored.ID, stored.Kind, stored.State, stored.LastError, output.String())
			}
			if stored.State != "succeeded" {
				complete = false
			}
		}
		if complete {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("operation chain did not complete: %v; fixture=%s", ctx.Err(), output.String())
		case <-time.After(30 * time.Millisecond):
		}
	}
}

func waitForCrossRepositoryPluginTelemetry(t *testing.T, db *gorm.DB, nodeID uint, timeout time.Duration) model.PluginTelemetryState {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var state model.PluginTelemetryState
		if err := db.Where("node_id = ? AND plugin_id = ?", nodeID, "machine-telemetry").First(&state).Error; err == nil {
			var metrics map[string]float64
			decodeErr := json.Unmarshal([]byte(state.MetricsJSON), &metrics)
			cpu, hasCPU := metrics["cpu_usage_percent"]
			memory, hasMemory := metrics["memory_usage_percent"]
			disk, hasDisk := metrics["disk_usage_percent"]
			uptime, hasUptime := metrics["uptime_seconds"]
			if decodeErr == nil && hasCPU && hasMemory && hasDisk && hasUptime &&
				cpu >= 0 && cpu <= 100 && memory >= 0 && memory <= 100 &&
				disk >= 0 && disk <= 100 && uptime >= 0 {
				return state
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	var state model.PluginTelemetryState
	err := db.Where("node_id = ? AND plugin_id = ?", nodeID, "machine-telemetry").First(&state).Error
	t.Fatalf("timed out waiting for machine-telemetry heartbeat: state=%+v err=%v", state, err)
	return state
}

func assertCrossRepositoryImmutablePluginPackage(t *testing.T, pluginDir, version string, artifact, manifest []byte, signature, configJSON string) {
	t.Helper()
	checks := []struct {
		name     string
		expected []byte
		mode     os.FileMode
	}{
		{name: "artifact.pkg", expected: artifact, mode: 0o600},
		{name: "manifest.json", expected: manifest, mode: 0o600},
		{name: "manifest.sig", expected: []byte(signature), mode: 0o600},
		{name: "config.json", expected: []byte(configJSON), mode: 0o600},
	}
	for _, check := range checks {
		path := filepath.Join(pluginDir, check.name)
		contents, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, check.expected, contents, check.name)
		info, err := os.Lstat(path)
		require.NoError(t, err)
		require.True(t, info.Mode().IsRegular())
		require.Equal(t, check.mode, info.Mode().Perm(), check.name)
	}
	pluginInfo, err := os.Lstat(filepath.Join(pluginDir, "plugin"))
	require.NoError(t, err)
	require.True(t, pluginInfo.Mode().IsRegular())
	require.Equal(t, os.FileMode(0o750), pluginInfo.Mode().Perm())
	staging, err := filepath.Glob(filepath.Join(filepath.Dir(pluginDir), "."+version+".staging-*"))
	require.NoError(t, err)
	require.Empty(t, staging)
}

func assertCrossRepositoryPluginHealth(t *testing.T, socketPath string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	connection, err := googlegrpc.NewClient("unix://"+socketPath,
		googlegrpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer func() {
		require.NoError(t, connection.Close())
	}()
	response, err := healthpb.NewHealthClient(connection).Check(ctx, &healthpb.HealthCheckRequest{Service: "machine-telemetry"})
	require.NoError(t, err)
	require.Equal(t, healthpb.HealthCheckResponse_SERVING, response.Status)
}

type crossRepositoryPluginState struct {
	DesiredVersion   string `json:"desired_version"`
	ObservedVersion  string `json:"observed_version"`
	Enabled          bool   `json:"enabled"`
	Health           string `json:"health"`
	ObservedRevision uint64 `json:"observed_revision"`
	LastError        string `json:"last_error"`
}

func readCrossRepositoryPluginState(t *testing.T, root, pluginID string) crossRepositoryPluginState {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join(root, "state.json"))
	require.NoError(t, err)
	var persisted struct {
		Plugins map[string]crossRepositoryPluginState `json:"plugins"`
	}
	require.NoError(t, json.Unmarshal(contents, &persisted))
	state, ok := persisted.Plugins[pluginID]
	require.True(t, ok)
	return state
}

func waitForCrossRepositorySocketRemoval(t *testing.T, socketPath string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		_, err := os.Lstat(socketPath)
		if errors.Is(err, os.ErrNotExist) {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("plugin socket %s was not removed", socketPath)
}

func stopCrossRepositoryPluginFixture(t *testing.T, fixture *exec.Cmd, done <-chan error, output interface{ String() string }) {
	t.Helper()
	if fixture.Process != nil {
		_ = fixture.Process.Signal(os.Interrupt)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Logf("Agent plugin fixture exit: %v: %s", err, output.String())
		}
	case <-time.After(7 * time.Second):
		if fixture.Process != nil {
			_ = fixture.Process.Kill()
		}
		<-done
	}
}

func crossRepositorySiblingAgentRoot(t *testing.T) string {
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

func buildCrossRepositoryAgentFixtureBinary(t *testing.T, agentRoot string) string {
	t.Helper()
	binaryPath := filepath.Join(t.TempDir(), "agent-control-fixture")
	command := exec.Command(crossRepositoryAgentGo(), "build", "-o", binaryPath, "./cmd/agent-control-fixture")
	command.Dir = agentRoot
	command.Env = crossRepositoryAgentBuildEnv()
	output, err := command.CombinedOutput()
	require.NoError(t, err, string(output))
	return binaryPath
}

func serveCrossRepositoryGRPCServer(server *googlegrpc.Server, listener net.Listener) <-chan error {
	result := make(chan error, 1)
	go func() { result <- server.Serve(listener) }()
	return result
}

func stopCrossRepositoryGRPCServer(t *testing.T, server *googlegrpc.Server, result <-chan error) {
	t.Helper()
	server.GracefulStop()
	err := <-result
	if err != nil && !errors.Is(err, googlegrpc.ErrServerStopped) {
		require.NoError(t, err)
	}
}

func waitForCrossRepositoryFile(t *testing.T, path string, timeout time.Duration, output interface{ String() string }) {
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

type crossRepositorySynchronizedBuffer struct {
	mu   sync.Mutex
	data []byte
}

func (b *crossRepositorySynchronizedBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.data = append(b.data, data...)
	return len(data), nil
}

func (b *crossRepositorySynchronizedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(append([]byte(nil), b.data...))
}
