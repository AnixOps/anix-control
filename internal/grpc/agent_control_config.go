package grpc

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	agentcontrol "github.com/AnixOps/anix-control/sdk/agentcontrol"
	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	"github.com/AnixOps/anix-control/v4/internal/agentstreams"
	"github.com/AnixOps/anix-control/v4/internal/kernelnodeops"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Configuration push on the Agent Control stream (A2-3; PROTOCOL.md,
// "Data plane"). When an agent's Hello lists config.v1, the HelloAck lists
// it back, for proxy and forward nodes, and the kernel sends the node's
// stored desired configuration (v4_kernel_node_desired_config) as a
// ConfigSnapshot:
//
//   - after the HelloAck, when Hello.config_revision differs from the
//     desired revision (0, older, or from another database), with the
//     desired configuration rebuilt from the node's rows first;
//   - when node.sync stores or forces a configuration
//     (kernelnodeops.SyncNode, through AgentStreams.PushConfig), in place
//     of node.reload;
//   - every minute (defaultConfigRefreshInterval), when a rebuild from the node's rows
//     moved the desired revision: the kernel writes that change what a node
//     runs reach the agent as the legacy pull would read them.
//
// The agent answers ConfigStatus, which the kernel records in
// v4_kernel_node_config_status (kernelnodeops.RecordConfigStatus) and
// hands to the node.sync waiting for it. A session is never sent a
// revision older than one it was already sent.

// defaultConfigRefreshInterval is how often a config.v1 session's desired
// configuration is rebuilt: as often as an agent pulls on the legacy
// transports (pull_interval).
const defaultConfigRefreshInterval = time.Minute

// configRefreshNanos holds the refresh interval; tests lower it
// (setConfigRefreshInterval) while sessions run.
var configRefreshNanos = func() *atomic.Int64 {
	nanos := &atomic.Int64{}
	nanos.Store(int64(defaultConfigRefreshInterval))
	return nanos
}()

func setConfigRefreshInterval(interval time.Duration) { configRefreshNanos.Store(int64(interval)) }

func configRefreshInterval() time.Duration { return time.Duration(configRefreshNanos.Load()) }

// Why a snapshot was sent: the label of anixops_agent_config_snapshots_sent_total.
const (
	configTriggerHello   = "hello"
	configTriggerSync    = "sync"
	configTriggerRefresh = "refresh"
	// configTriggerForward: a plan moved the node's forwarding generation
	// (agent_control_forward.go).
	configTriggerForward = "forward"
)

// configStatusUnrecorded labels a ConfigStatus the kernel could not record.
const configStatusUnrecorded = "unrecorded"

// servesConfig tells whether the HelloAck advertises config.v1: when the
// agent lists it, for a proxy or a forward node, both of which have a
// desired configuration.
func (s *AgentControlGRPCServer) servesConfig(node agentcontrol.AgentNode, agent []*agentv1pb.Capability) bool {
	return (node.Kind == agentcontrol.NodeKindProxy || node.Kind == agentcontrol.NodeKindForward) &&
		agentcontrol.HasCapabilityVersion(agent, agentcontrol.CapabilityConfig, agentcontrol.CapabilityVersionV1)
}

// sendConfig sends snapshot on the session unless the session was sent a
// newer revision, or, without force, the agent already has this revision
// (it reported it in Hello, or was sent it). It reports whether it sent.
func (c *AgentControlConnection) sendConfig(snapshot *agentv1pb.ConfigSnapshot, force bool, trigger string) (bool, error) {
	c.configMu.Lock()
	defer c.configMu.Unlock()
	revision := snapshot.GetConfigRevision()
	if revision < c.configSentMax || (!force && revision == c.configRevision) {
		return false, nil
	}
	if err := c.send(&agentv1pb.ControlToAgent{
		RequestId:    newAgentControlID("config"),
		NodeId:       c.NodeID,
		SentAtUnixMs: time.Now().UnixMilli(),
		Payload:      &agentv1pb.ControlToAgent_Config{Config: snapshot},
	}); err != nil {
		return false, fmt.Errorf("send config snapshot: %w", err)
	}
	c.configRevision, c.configSentMax = revision, revision
	agentConfigMetrics.sent(trigger)
	return true, nil
}

// pushDesiredConfig rebuilds the node's desired configuration from its rows
// (stored when it changed) and sends it unless the agent has it. A database
// error is logged and nothing is sent; only a send error is returned.
func pushDesiredConfig(ctx context.Context, connection *AgentControlConnection, node agentcontrol.AgentNode, trigger string) error {
	row, _, err := kernelnodeops.RefreshDesiredConfig(ctx, databaseForAgentChecks(), node, time.Now())
	if err != nil {
		if ctx.Err() == nil {
			slog.Warn("agent config: the desired configuration could not be built", "component", "agent-control", "node", node.String(), "trigger", trigger, "error", err)
		}
		return nil
	}
	_, err = connection.sendConfig(kernelnodeops.ConfigSnapshotOf(row), false, trigger)
	return err
}

// startConfigRefresh rebuilds the session's desired configuration every
// configRefreshInterval() and sends it when its revision moved. The returned
// function stops the refresher and waits for it.
func (s *AgentControlGRPCServer) startConfigRefresh(ctx context.Context, connection *AgentControlConnection, node agentcontrol.AgentNode) (stop func()) {
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(configRefreshInterval())
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := pushDesiredConfig(ctx, connection, node, configTriggerRefresh); err != nil {
					// The stream is gone; the session ends on its own.
					return
				}
			}
		}
	}()
	return func() {
		cancel()
		<-done
	}
}

// handleConfigStatus records an agent's ConfigStatus and hands it to the
// node syncs waiting for it. It returns an error only when the stream must
// end: the session did not negotiate config.v1, or the status is malformed.
// A status the kernel cannot record (its database failed) is still handed
// on, without a verdict.
func (s *AgentControlGRPCServer) handleConfigStatus(ctx context.Context, manager *AgentControlManager, connection *AgentControlConnection, node agentcontrol.AgentNode, configStatus *agentv1pb.ConfigStatus) error {
	if !agentcontrol.Negotiated(connection.Capabilities, connection.ServerCapabilities, agentcontrol.CapabilityConfig) {
		return unnegotiatedPayload("config_status", agentcontrol.CapabilityConfig)
	}
	if configStatus == nil {
		return status.Error(codes.InvalidArgument, "config_status payload is required")
	}
	if configStatus.GetConfigRevision() == 0 {
		return status.Error(codes.InvalidArgument, "config_status config_revision is required")
	}
	verdict, err := kernelnodeops.RecordConfigStatus(ctx, databaseForAgentChecks(), node, connection.SessionID, configStatus, time.Now())
	if err != nil {
		slog.Warn("agent config: the configuration status could not be recorded", "component", "agent-control", "node", node.String(), "config_revision", configStatus.GetConfigRevision(), "error", err)
		verdict = ""
	}
	if verdict == model.ConfigVerdictFailed {
		slog.Warn("agent config: the agent could not apply the configuration", "component", "agent-control", "node", node.String(), "config_revision", configStatus.GetConfigRevision(), "error", configStatus.GetError())
	}
	agentConfigMetrics.status(verdict)
	manager.deliverConfigStatus(connection.NodeID, connection.SessionID, configStatus, verdict)
	return nil
}

// configStatusHandler receives every ConfigStatus of a manager's nodes.
type configStatusHandler func(nodeID uint32, sessionID string, status *agentv1pb.ConfigStatus, verdict string)

// addConfigStatusHandler registers handler for every ConfigStatus.
func (m *AgentControlManager) addConfigStatusHandler(handler configStatusHandler) {
	m.mu.Lock()
	m.configHandlers = append(m.configHandlers, handler)
	m.mu.Unlock()
}

func (m *AgentControlManager) deliverConfigStatus(nodeID uint32, sessionID string, configStatus *agentv1pb.ConfigStatus, verdict string) {
	m.mu.RLock()
	handlers := append([]configStatusHandler(nil), m.configHandlers...)
	m.mu.RUnlock()
	for _, handler := range handlers {
		handler(nodeID, sessionID, configStatus, verdict)
	}
}

// ConfigNegotiated reports whether the node's current session negotiated
// config.v1 (agentstreams.ConfigStreams).
func (s *AgentStreams) ConfigNegotiated(node agentcontrol.AgentNode) bool {
	connection := s.connection(node)
	return connection != nil && connection.configNegotiated
}

// PushConfig sends snapshot on the node's session (agentstreams.ConfigStreams).
func (s *AgentStreams) PushConfig(ctx context.Context, node agentcontrol.AgentNode, snapshot *agentv1pb.ConfigSnapshot) (string, bool, error) {
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	connection := s.connection(node)
	if connection == nil {
		return "", false, classified{sentinel: agentstreams.ErrNotConnected, cause: fmt.Errorf("agent node %s is not connected", node)}
	}
	if !connection.configNegotiated {
		return "", false, classified{sentinel: agentstreams.ErrCapabilityMissing, cause: fmt.Errorf("agent node %s does not advertise capability %q", node, agentcontrol.CapabilityConfig+"."+agentcontrol.CapabilityVersionV1)}
	}
	sent, err := connection.sendConfig(snapshot, true, configTriggerSync)
	if err != nil {
		return "", false, classified{sentinel: agentstreams.ErrNotConnected, cause: err}
	}
	return connection.SessionID, sent, nil
}

// OnConfigStatus registers handler with both managers
// (agentstreams.ConfigStreams).
func (s *AgentStreams) OnConfigStatus(handler agentstreams.ConfigStatusHandler) {
	if handler == nil {
		return
	}
	for _, entry := range []struct {
		kind    string
		manager *AgentControlManager
	}{{agentcontrol.NodeKindProxy, s.proxy}, {agentcontrol.NodeKindForward, s.forward}} {
		if entry.manager == nil {
			continue
		}
		kind := entry.kind
		entry.manager.addConfigStatusHandler(func(nodeID uint32, sessionID string, configStatus *agentv1pb.ConfigStatus, verdict string) {
			handler(agentstreams.ConfigStatusReport{
				Node: agentcontrol.AgentNode{Kind: kind, ID: nodeID}, SessionID: sessionID, Status: configStatus, Verdict: verdict,
			})
		})
	}
}

// connection returns the node's current connection, nil when none.
func (s *AgentStreams) connection(node agentcontrol.AgentNode) *AgentControlConnection {
	manager := s.manager(node)
	if manager == nil {
		return nil
	}
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	return manager.connections[node.ID]
}

var _ agentstreams.ConfigStreams = (*AgentStreams)(nil)

// configMetrics counts what the configuration push sent and received.
type configMetrics struct {
	snapshots map[string]*atomic.Uint64
	statuses  map[string]*atomic.Uint64
}

// configTriggers and configResults are the label values, in the order the
// metrics are written.
var (
	configTriggers = []string{configTriggerHello, configTriggerSync, configTriggerRefresh, configTriggerForward}
	configResults  = []string{model.ConfigVerdictApplied, model.ConfigVerdictFailed, model.ConfigVerdictStale, model.ConfigVerdictMismatch, configStatusUnrecorded}
)

var agentConfigMetrics = newConfigMetrics()

func newConfigMetrics() *configMetrics {
	metrics := &configMetrics{snapshots: map[string]*atomic.Uint64{}, statuses: map[string]*atomic.Uint64{}}
	for _, trigger := range configTriggers {
		metrics.snapshots[trigger] = &atomic.Uint64{}
	}
	for _, result := range configResults {
		metrics.statuses[result] = &atomic.Uint64{}
	}
	return metrics
}

func (m *configMetrics) sent(trigger string) {
	if counter := m.snapshots[trigger]; counter != nil {
		counter.Add(1)
	}
}

func (m *configMetrics) status(verdict string) {
	if verdict == "" {
		verdict = configStatusUnrecorded
	}
	if counter := m.statuses[verdict]; counter != nil {
		counter.Add(1)
	}
}

// WriteAgentConfigPrometheus renders the configuration push counters and
// the lagging-node gauge in the Prometheus text format.
func WriteAgentConfigPrometheus(body *strings.Builder) {
	body.WriteString("# HELP anixops_agent_config_snapshots_sent_total ConfigSnapshot messages sent on the Agent Control stream, by trigger: hello (reconcile), sync (node.sync), refresh (a rebuild moved the revision) or forward (a plan moved the node's forwarding generation).\n")
	body.WriteString("# TYPE anixops_agent_config_snapshots_sent_total counter\n")
	for _, trigger := range configTriggers {
		body.WriteString("anixops_agent_config_snapshots_sent_total{trigger=\"" + trigger + "\"} " + strconv.FormatUint(agentConfigMetrics.snapshots[trigger].Load(), 10) + "\n")
	}
	body.WriteString("# HELP anixops_agent_config_statuses_total ConfigStatus messages received, by result: applied, failed, stale, mismatch or unrecorded.\n")
	body.WriteString("# TYPE anixops_agent_config_statuses_total counter\n")
	for _, result := range configResults {
		body.WriteString("anixops_agent_config_statuses_total{result=\"" + result + "\"} " + strconv.FormatUint(agentConfigMetrics.statuses[result].Load(), 10) + "\n")
	}
	db := databaseForAgentChecks()
	if db == nil {
		return
	}
	lagging, err := kernelnodeops.ConfigLaggingNodes(context.Background(), db)
	if err != nil {
		// The database is down or not migrated: no gauge.
		slog.Debug("agent config: lagging nodes could not be counted", "component", "agent-control", "error", err)
		return
	}
	body.WriteString("# HELP anixops_agent_config_lagging_nodes Nodes whose agent's applied configuration revision is behind the desired one.\n")
	body.WriteString("# TYPE anixops_agent_config_lagging_nodes gauge\n")
	body.WriteString("anixops_agent_config_lagging_nodes " + strconv.FormatInt(lagging, 10) + "\n")
}
