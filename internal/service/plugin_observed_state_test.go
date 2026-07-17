package service

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type nodePluginObservedStateFixture struct {
	db      *gorm.DB
	service *NodeService
	node    model.Node
	plugin  string
	version string
}

func newNodePluginObservedStateFixture(t *testing.T, capabilities []string) nodePluginObservedStateFixture {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, EnsureKernelSchema(db))
	require.NoError(t, db.AutoMigrate(&model.Node{}))
	node := model.Node{Name: "observed-state-node", Host: "127.0.0.1", APIKey: "observed-state-key"}
	require.NoError(t, db.Create(&node).Error)
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	manifest := PluginManifest{
		ID: "nftables-forward", Name: "nftables Forward", Version: "1.2.0", APIVersion: "v1", Publisher: "AnixOps",
		Targets: []string{"agent"}, ArtifactSHA256: strings.Repeat("a", 64), Capabilities: capabilities,
	}
	canonical, err := CanonicalPluginManifest(manifest)
	require.NoError(t, err)
	signature := base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, canonical))
	_, err = RegisterPluginRelease(db, string(canonical), signature, privateKey.Public().(ed25519.PublicKey))
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.NodeServiceAssignment{
		NodeID: node.ID, ServiceScope: "forward", PluginID: manifest.ID, Role: "cn_dedicated_nftables",
		DesiredVersion: manifest.Version, Enabled: true,
	}).Error)
	return nodePluginObservedStateFixture{db: db, service: &NodeService{db: db}, node: node, plugin: manifest.ID, version: manifest.Version}
}

func validNodePluginObservedSnapshot(fixture nodePluginObservedStateFixture, observedAt time.Time) NodePluginObservedSnapshot {
	return NodePluginObservedSnapshot{
		PluginID: fixture.plugin, Version: fixture.version, DesiredRevision: 12, ObservedRevision: 12,
		ConfigHash: strings.Repeat("b", 64), Health: "healthy", RulesetSHA256: strings.Repeat("c", 64),
		ObservedAt:   observedAt,
		RuleCounters: []NodePluginRuleCounter{{RuleID: "udp-443", Packets: 7, Bytes: 800}, {RuleID: "tcp-443", Packets: 3, Bytes: 400}},
	}
}

func TestRecordNodePluginObservedStatesPersistsOnlySignedAssignedCapability(t *testing.T) {
	fixture := newNodePluginObservedStateFixture(t, []string{"kernel.observed-state"})
	now := time.Now().Round(time.Millisecond)
	accepted, err := fixture.service.RecordNodePluginObservedStates(fixture.node.ID, []NodePluginObservedSnapshot{
		validNodePluginObservedSnapshot(fixture, now),
	}, now)
	require.NoError(t, err)
	require.Equal(t, 1, accepted)
	var state model.NodePluginObservedState
	require.NoError(t, fixture.db.First(&state, "node_id = ? AND plugin_id = ?", fixture.node.ID, fixture.plugin).Error)
	require.Equal(t, fixture.version, state.Version)
	require.Equal(t, int64(12), state.DesiredRevision)
	require.Equal(t, int64(12), state.ObservedRevision)
	require.Equal(t, strings.Repeat("b", 64), state.ConfigHash)
	require.Equal(t, "healthy", state.Health)
	require.Equal(t, strings.Repeat("c", 64), state.RulesetSHA256)
	require.JSONEq(t, `[{"rule_id":"tcp-443","packets":3,"bytes":400},{"rule_id":"udp-443","packets":7,"bytes":800}]`, state.CountersJSON)
}

func TestRecordNodePluginObservedStatesRejectsUnauthorizedAndMalformedSnapshots(t *testing.T) {
	fixture := newNodePluginObservedStateFixture(t, nil)
	now := time.Now().Round(time.Millisecond)
	snapshot := validNodePluginObservedSnapshot(fixture, now)
	accepted, err := fixture.service.RecordNodePluginObservedStates(fixture.node.ID, []NodePluginObservedSnapshot{snapshot}, now)
	require.Zero(t, accepted)
	require.ErrorContains(t, err, "not authorized for kernel observed state")

	fixture = newNodePluginObservedStateFixture(t, []string{"kernel.observed-state"})
	snapshot = validNodePluginObservedSnapshot(fixture, now)
	snapshot.RuleCounters = append(snapshot.RuleCounters, NodePluginRuleCounter{RuleID: "tcp-443"})
	accepted, err = fixture.service.RecordNodePluginObservedStates(fixture.node.ID, []NodePluginObservedSnapshot{snapshot}, now)
	require.Zero(t, accepted)
	require.ErrorContains(t, err, "duplicated")

	snapshot = validNodePluginObservedSnapshot(fixture, now.Add(-maxNodePluginObservedStateAge-time.Second))
	accepted, err = fixture.service.RecordNodePluginObservedStates(fixture.node.ID, []NodePluginObservedSnapshot{snapshot}, now)
	require.Zero(t, accepted)
	require.ErrorContains(t, err, "outside the accepted window")
}

func TestRecordNodePluginObservedStatesAllowsUnhealthyWithoutRulesetAndNeverRegresses(t *testing.T) {
	fixture := newNodePluginObservedStateFixture(t, []string{"kernel.observed-state"})
	now := time.Now().Round(time.Millisecond)
	healthy := validNodePluginObservedSnapshot(fixture, now)
	accepted, err := fixture.service.RecordNodePluginObservedStates(fixture.node.ID, []NodePluginObservedSnapshot{healthy}, now)
	require.NoError(t, err)
	require.Equal(t, 1, accepted)

	stale := validNodePluginObservedSnapshot(fixture, now.Add(-time.Second))
	stale.Health = "unhealthy"
	stale.RulesetSHA256 = ""
	stale.RuleCounters = nil
	accepted, err = fixture.service.RecordNodePluginObservedStates(fixture.node.ID, []NodePluginObservedSnapshot{stale}, now)
	require.NoError(t, err)
	require.Equal(t, 1, accepted)
	var state model.NodePluginObservedState
	require.NoError(t, fixture.db.First(&state, "node_id = ? AND plugin_id = ?", fixture.node.ID, fixture.plugin).Error)
	require.Equal(t, "healthy", state.Health)

	unhealthy := stale
	unhealthy.ObservedAt = now.Add(time.Second)
	accepted, err = fixture.service.RecordNodePluginObservedStates(fixture.node.ID, []NodePluginObservedSnapshot{unhealthy}, now.Add(time.Second))
	require.NoError(t, err)
	require.Equal(t, 1, accepted)
	require.NoError(t, fixture.db.First(&state, "node_id = ? AND plugin_id = ?", fixture.node.ID, fixture.plugin).Error)
	require.Equal(t, "unhealthy", state.Health)
	require.Empty(t, state.RulesetSHA256)
	require.JSONEq(t, `[]`, state.CountersJSON)
}

func TestNodePluginObservedRevisionBounds(t *testing.T) {
	revision, ok := nodePluginObservedRevision(maxNodePluginObservedRevision)
	require.True(t, ok)
	require.Equal(t, int64(1<<63-1), revision)

	_, ok = nodePluginObservedRevision(maxNodePluginObservedRevision + 1)
	require.False(t, ok)
}
