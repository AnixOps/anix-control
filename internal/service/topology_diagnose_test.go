package service

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func signedTopologyPreviewRelease(t *testing.T, db *gorm.DB, pluginID string, dependencies []string) model.PluginRelease {
	t.Helper()
	artifact := []byte("signed-topology-preview-artifact:" + pluginID)
	digest := sha256.Sum256(artifact)
	manifest := PluginManifest{
		ID: pluginID, Name: pluginID, Version: "1.0.0", APIVersion: "v1", Publisher: "AnixOps",
		Targets: []string{"agent"}, ArtifactSHA256: hex.EncodeToString(digest[:]),
		Dependencies: dependencies, ConfigSchema: []byte(`{"type":"object"}`),
	}
	canonical, err := CanonicalPluginManifest(manifest)
	require.NoError(t, err)
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	release, err := RegisterPluginRelease(db, string(canonical), base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, canonical)), publicKey)
	require.NoError(t, err)
	_, err = StorePluginArtifact(db, release.ID, artifact)
	require.NoError(t, err)
	return *release
}

func topologyPreviewIssueCodes(preview *TopologyDeploymentPreview) map[string]bool {
	codes := make(map[string]bool)
	for _, issue := range preview.Issues {
		codes[issue.Code] = true
	}
	return codes
}

func TestPreviewTopologyDeploymentIsReadOnlyAndVerifiesSignedDependencyClosure(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, EnsureKernelSchema(db))
	require.NoError(t, db.AutoMigrate(&model.Node{}))
	dependency := signedTopologyPreviewRelease(t, db, "preview-dependency", nil)
	root := signedTopologyPreviewRelease(t, db, "preview-root", []string{dependency.PluginID})
	node := model.Node{Name: "preview-node", Host: "127.0.0.1", APIKey: "preview-key"}
	require.NoError(t, db.Create(&node).Error)
	topology := model.Topology{Name: "preview-topology", ServiceScope: "forward"}
	require.NoError(t, db.Create(&topology).Error)
	revision := model.TopologyRevision{TopologyID: topology.ID, Revision: 1, State: "draft", ContentHash: strings.Repeat("a", 64), CreatedBy: 1}
	require.NoError(t, db.Create(&revision).Error)
	require.NoError(t, db.Create(&model.NodeServiceAssignment{
		NodeID: node.ID, ServiceScope: "forward", PluginID: root.PluginID, Role: "entry",
		DesiredVersion: root.Version, Enabled: true,
	}).Error)
	vertex := model.TopologyVertex{
		RevisionID: revision.ID, Key: "entry", Kind: "plugin", NodeID: &node.ID,
		PluginID: root.PluginID, Role: "entry",
		ConfigJSON: `{"rules":[{"id":"https","protocol":"tcp","listen_address":"0.0.0.0","listen_port":443,"target_address":"198.51.100.10","target_port":443}]}`,
	}
	require.NoError(t, db.Create(&vertex).Error)

	var deploymentsBefore, operationsBefore int64
	require.NoError(t, db.Model(&model.TopologyDeployment{}).Count(&deploymentsBefore).Error)
	require.NoError(t, db.Model(&model.KernelOperation{}).Count(&operationsBefore).Error)
	preview, err := PreviewTopologyDeployment(db, TopologyDeploymentPreviewInput{TopologyID: topology.ID, RevisionID: revision.ID})
	require.NoError(t, err)
	require.True(t, preview.Valid, preview.Issues)
	require.Len(t, preview.Steps, 1)
	require.Equal(t, []string{"plugin.configure", "plugin.enable"}, preview.Steps[0].Operations)
	require.NotEmpty(t, preview.Steps[0].ConfigHash)
	require.Empty(t, preview.Issues)
	var deploymentsAfter, operationsAfter int64
	require.NoError(t, db.Model(&model.TopologyDeployment{}).Count(&deploymentsAfter).Error)
	require.NoError(t, db.Model(&model.KernelOperation{}).Count(&operationsAfter).Error)
	require.Equal(t, deploymentsBefore, deploymentsAfter)
	require.Equal(t, operationsBefore, operationsAfter)
}

func TestPreviewTopologyDeploymentReturnsStructuredPreflightIssues(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, EnsureKernelSchema(db))
	require.NoError(t, db.AutoMigrate(&model.Node{}))
	node := model.Node{Name: "invalid-preview-node", Host: "127.0.0.1", APIKey: "invalid-preview-key"}
	require.NoError(t, db.Create(&node).Error)
	topology := model.Topology{Name: "invalid-preview-topology", ServiceScope: "forward"}
	require.NoError(t, db.Create(&topology).Error)
	revision := model.TopologyRevision{TopologyID: topology.ID, Revision: 1, State: "draft", ContentHash: strings.Repeat("b", 64), CreatedBy: 1}
	require.NoError(t, db.Create(&revision).Error)
	// No assignment/release is intentionally created. The malformed runtime
	// config and secure edge exercise independent diagnostic checks.
	vertex := model.TopologyVertex{
		RevisionID: revision.ID, Key: "entry", Kind: "plugin", NodeID: &node.ID,
		PluginID: "missing-preview-plugin", Role: "entry",
		ConfigJSON: `{"address_family":"ipx","mtu":500,"rules":[{"protocol":"tcp","listen_port":0,"target_port":443}]}`,
	}
	require.NoError(t, db.Create(&vertex).Error)
	require.NoError(t, db.Create(&model.TopologyEdge{
		RevisionID: revision.ID, SourceKey: "entry", TargetKey: "entry", Protocol: "wss", ConfigJSON: `{}`,
	}).Error)

	preview, err := PreviewTopologyDeployment(db, TopologyDeploymentPreviewInput{TopologyID: topology.ID, RevisionID: revision.ID})
	require.NoError(t, err)
	require.False(t, preview.Valid)
	codes := topologyPreviewIssueCodes(preview)
	for _, code := range []string{"self_loop", "assignment_missing", "invalid_address_family", "invalid_mtu", "invalid_port", "secret_required"} {
		require.True(t, codes[code], "missing diagnostic code %s: %#v", code, preview.Issues)
	}
	require.NotEmpty(t, preview.Checks)
	require.NotEmpty(t, preview.Steps, "the editor should still receive a step skeleton when graph ordering is invalid")
}

func TestTopologyDeploymentStatusIncludesSafeOperationTimeline(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, EnsureKernelSchema(db))
	now := time.Now().UTC().Truncate(time.Microsecond)
	deployment := model.TopologyDeployment{
		TopologyID: 1, RevisionID: 1, State: "failed", FailurePolicy: "stop_and_rollback", CreatedBy: 1,
		LastError: "deployment-token-must-not-leak", CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, db.Create(&deployment).Error)
	nodeID := uint(7)
	step := model.TopologyDeploymentStep{
		DeploymentID: deployment.ID, VertexID: 1, VertexKey: "entry", NodeID: nodeID,
		PluginID: "timeline-plugin", Role: "entry", TargetVersion: "1.0.0", ApplyOrder: 1,
		ApplyAction: "configure_enable", ConfigJSON: `{"token":"step-config-token-must-not-leak"}`,
		RollbackMode: "disable", RollbackConfigJSON: `{"secret":"rollback-secret-must-not-leak"}`,
		State: "failed", LastError: "step-error-must-not-leak", ConfigureOperationID: "timeline-operation",
	}
	require.NoError(t, db.Create(&step).Error)
	require.NoError(t, db.Create(&model.KernelOperation{
		ID: "timeline-operation", IdempotencyKey: "timeline-idempotency", NodeID: &nodeID,
		PluginID: "timeline-plugin", TargetVersion: "1.0.0", Kind: "plugin.configure", State: "failed",
		TopologyDeploymentID: &deployment.ID, TopologyStepID: &step.ID, TopologyRevision: 1,
		ConfigJSON: `{"token":"operation-config-token-must-not-leak"}`,
		ResultJSON: `{"agent_secret":"agent-result-secret-must-not-leak"}`,
		LastError:  "agent-error-must-not-leak", CreatedAt: now, UpdatedAt: now,
	}).Error)
	require.NoError(t, db.Create(&model.TopologyObservedState{
		DeploymentID: deployment.ID, NodeID: nodeID, DesiredRevision: 1, ObservedRevision: 0,
		State: "failed", HealthJSON: `{"agent_token":"observed-health-token-must-not-leak"}`,
		LastError: "observed-error-must-not-leak", UpdatedAt: now,
	}).Error)

	status, err := GetTopologyDeploymentStatus(db, deployment.ID)
	require.NoError(t, err)
	require.Len(t, status.Operations, 1)
	require.Equal(t, "timeline-operation", status.Operations[0].OperationID)
	require.True(t, status.Operations[0].HasError)
	require.Equal(t, "plugin configuration failed", status.Operations[0].LastError)
	require.NotEmpty(t, status.Events)
	for _, event := range status.Events {
		require.NotContains(t, event.Message, "must-not-leak")
	}
	// The public projection is also a defensive boundary for internal callers:
	// a pre-populated timeline string must not escape if an upstream loader is
	// ever changed or a caller constructs a status object directly.
	status.Operations[0].LastError = "manually-injected-operation-error-must-not-leak"
	status.Events[0].Message = "manually-injected-event-error-must-not-leak"

	encoded, err := json.Marshal(PublicTopologyDeploymentStatus(*status))
	require.NoError(t, err)
	payload := string(encoded)
	for _, secret := range []string{
		"deployment-token-must-not-leak", "step-config-token-must-not-leak",
		"rollback-secret-must-not-leak", "step-error-must-not-leak",
		"operation-config-token-must-not-leak", "agent-result-secret-must-not-leak",
		"agent-error-must-not-leak", "observed-health-token-must-not-leak",
		"observed-error-must-not-leak", "manually-injected-operation-error-must-not-leak",
		"manually-injected-event-error-must-not-leak",
	} {
		require.NotContains(t, payload, secret)
	}
	for _, field := range []string{`"config":`, `"rollback_config":`, `"health":`, `"result":`} {
		require.NotContains(t, payload, field)
	}
	require.Contains(t, payload, `"last_error":"deployment failed"`)
	require.Contains(t, payload, `"last_error":"plugin configuration failed"`)
}

func TestNftablesForwardPreviewGateMatchesAgentRuntimeContract(t *testing.T) {
	valid := `{"apply":true,"rollback_on_exit":true,"family":"ip","table":"anixops_forward","chain":"prerouting","priority":-90,"plan_path":"/run/anixops/forward.nft","rules":[{"id":"tcp_443","protocol":"tcp","listen_address":"198.51.100.10","listen_port":443,"target_address":"203.0.113.10","target_port":8443}]}`
	require.Empty(t, validateNftablesForwardTopologyConfig(valid, "vertices[0].config"))
	invalid := `{"apply":true,"rollback_on_exit":false,"family":"ip","table":"bad-name","chain":"bad-chain","priority":900,"plan_path":"relative.nft","rules":[{"id":"bad:id","protocol":"icmp","listen_address":"0.0.0.0","listen_port":0,"target_address":"2001:db8::10","target_port":70000},{"id":"dup","protocol":"udp","listen_address":"198.51.100.11","listen_port":53,"target_address":"203.0.113.11","target_port":53},{"id":"dup","protocol":"udp","listen_address":"198.51.100.12","listen_port":54,"target_address":"203.0.113.12","target_port":54}]}`
	issues := validateNftablesForwardTopologyConfig(invalid, "vertices[0].config")
	codes := make(map[string]bool)
	for _, issue := range issues {
		codes[issue.Code] = true
	}
	for _, code := range []string{"nftables_rollback_required", "nftables_table_invalid", "nftables_chain_invalid", "nftables_priority_invalid", "nftables_plan_path_invalid", "nftables_rule_id_invalid", "nftables_rule_id_duplicate", "nftables_rule_protocol_invalid", "nftables_rule_port_invalid", "nftables_rule_address_invalid", "nftables_rule_address_family_mismatch"} {
		require.True(t, codes[code], "missing nftables gate issue %s: %#v", code, issues)
	}
}
