package plugincontrol

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"path/filepath"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
)

const (
	NftablesForwardPluginID          = "nftables-forward"
	NftablesForwardVersion           = "1.2.0"
	NftablesForwardLegacyVersion     = "1.0.0"
	NftablesForwardStatusRoute       = "/api/v3/plugins/nftables-forward/status"
	nftablesForwardRuntimeStaleAfter = 2 * time.Minute
)

type NftablesForwardExecutor struct {
	db      *gorm.DB
	now     func() time.Time
	version string
}

func NewNftablesForwardExecutor(db *gorm.DB) *NftablesForwardExecutor {
	return NewNftablesForwardExecutorVersion(db, NftablesForwardVersion)
}

func NewNftablesForwardExecutorVersion(db *gorm.DB, version string) *NftablesForwardExecutor {
	if strings.TrimSpace(version) == "" {
		version = NftablesForwardVersion
	}
	return &NftablesForwardExecutor{db: db, now: time.Now, version: version}
}

func (e *NftablesForwardExecutor) PluginID() string { return NftablesForwardPluginID }
func (e *NftablesForwardExecutor) Version() string  { return e.version }

// ValidateConfiguration is the Control-side semantic gate for the 1.2 package.
// JSON Schema checks shape; this method checks the nftables/runtime invariants
// that must hold before an Agent operation is admitted.
func (e *NftablesForwardExecutor) ValidateConfiguration(ctx context.Context, raw json.RawMessage) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if e.Version() != NftablesForwardVersion {
		return nil
	}
	var value map[string]json.RawMessage
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return fmt.Errorf("nftables-forward configuration must be an object: %w", err)
	}
	if value == nil {
		return errors.New("nftables-forward configuration must be an object")
	}
	var apply bool
	if data, ok := value["apply"]; ok {
		if err := json.Unmarshal(data, &apply); err != nil {
			return errors.New("apply must be a boolean")
		}
	}
	if data, ok := value["rollback_on_exit"]; ok {
		var rollback bool
		if err := json.Unmarshal(data, &rollback); err != nil || !rollback {
			return errors.New("rollback_on_exit must remain enabled")
		}
	}
	family := "inet"
	for key, target := range map[string]*string{"family": &family} {
		if data, ok := value[key]; ok {
			if err := json.Unmarshal(data, target); err != nil {
				return fmt.Errorf("%s must be a string", key)
			}
		}
	}
	if family != "inet" && family != "ip" && family != "ip6" {
		return errors.New("family must be inet, ip, or ip6")
	}
	if data, ok := value["nft_binary"]; ok {
		var binary string
		if err := json.Unmarshal(data, &binary); err != nil || strings.ContainsAny(binary, "\x00\r\n") {
			return errors.New("nft_binary is invalid")
		}
	}
	if data, ok := value["plan_path"]; ok {
		var planPath string
		if err := json.Unmarshal(data, &planPath); err != nil {
			return errors.New("plan_path must be a string")
		}
		if planPath != "" && (!filepath.IsAbs(planPath) || filepath.Clean(planPath) != planPath) {
			return errors.New("plan_path must be absolute and canonical")
		}
	}
	for _, key := range []string{"table", "chain"} {
		if data, ok := value[key]; ok {
			var identifier string
			if err := json.Unmarshal(data, &identifier); err != nil || !validNftIdentifier(identifier) {
				return fmt.Errorf("%s must be a safe nftables identifier", key)
			}
		}
	}
	if data, ok := value["priority"]; ok {
		var priority int
		if err := json.Unmarshal(data, &priority); err != nil || priority < -500 || priority > 500 {
			return errors.New("priority must be between -500 and 500")
		}
	}
	var rules []nftablesForwardRule
	if data, ok := value["rules"]; ok {
		if err := json.Unmarshal(data, &rules); err != nil {
			return fmt.Errorf("rules must be an array: %w", err)
		}
	}
	if apply && len(rules) == 0 {
		return errors.New("at least one forwarding rule is required when apply is enabled")
	}
	seen := make(map[string]struct{}, len(rules))
	for index, rule := range rules {
		if !validNftRuleID(rule.ID) {
			return fmt.Errorf("rules[%d].id is invalid", index)
		}
		if _, exists := seen[rule.ID]; exists {
			return fmt.Errorf("rule %q is duplicated", rule.ID)
		}
		seen[rule.ID] = struct{}{}
		if rule.Protocol != "tcp" && rule.Protocol != "udp" {
			return fmt.Errorf("rules[%d].protocol must be tcp or udp", index)
		}
		listen, err := netip.ParseAddr(rule.ListenAddress)
		if err != nil || listen.IsUnspecified() || listen.IsMulticast() {
			return fmt.Errorf("rules[%d].listen_address must be a unicast IP address", index)
		}
		target, err := netip.ParseAddr(rule.TargetAddress)
		if err != nil || target.IsUnspecified() || target.IsMulticast() {
			return fmt.Errorf("rules[%d].target_address must be a unicast IP address", index)
		}
		if listen.Is4() != target.Is4() {
			return fmt.Errorf("rules[%d] addresses must use the same IP family", index)
		}
		if (family == "ip" && !listen.Is4()) || (family == "ip6" && !listen.Is6()) {
			return fmt.Errorf("rules[%d] address family does not match %s", index, family)
		}
		if rule.ListenPort < 1 || rule.ListenPort > 65535 || rule.TargetPort < 1 || rule.TargetPort > 65535 {
			return fmt.Errorf("rules[%d] ports must be between 1 and 65535", index)
		}
		if len(rule.Comment) > 96 || strings.ContainsAny(rule.Comment, "\x00\r\n\"\\") {
			return fmt.Errorf("rules[%d].comment is invalid", index)
		}
	}
	return nil
}

func validNftIdentifier(value string) bool {
	if value == "" || len(value) > 63 {
		return false
	}
	for index, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || char == '_' || (index > 0 && char >= '0' && char <= '9') {
			continue
		}
		return false
	}
	return true
}

func validNftRuleID(value string) bool {
	if value == "" || len(value) > 80 {
		return false
	}
	for index, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '-' || char == '_' || (index > 0 && char == '.') {
			continue
		}
		return false
	}
	return true
}

type NftablesForwardSummary struct {
	Rules            int `json:"rules"`
	Ready            int `json:"ready"`
	Reconciling      int `json:"reconciling"`
	Degraded         int `json:"degraded"`
	RollbackRequired int `json:"rollback_required"`
}

type NftablesForwardRuleStatus struct {
	ID               string     `json:"id"`
	Name             string     `json:"name"`
	NodeID           uint       `json:"node_id"`
	NodeName         string     `json:"node_name"`
	Topology         string     `json:"topology,omitempty"`
	Protocol         string     `json:"protocol"`
	Listen           string     `json:"listen,omitempty"`
	Target           string     `json:"target,omitempty"`
	DesiredRevision  int64      `json:"desired_revision"`
	ObservedRevision int64      `json:"observed_revision"`
	Enabled          bool       `json:"enabled"`
	Ready            bool       `json:"ready"`
	Reconciling      bool       `json:"reconciling"`
	Degraded         bool       `json:"degraded"`
	RollbackRequired bool       `json:"rollback_required"`
	RuntimeHealth    string     `json:"runtime_health,omitempty"`
	RulesetSHA256    string     `json:"ruleset_sha256,omitempty"`
	Packets          uint64     `json:"packets"`
	Bytes            uint64     `json:"bytes"`
	ObservedAt       *time.Time `json:"observed_at,omitempty"`
	LastError        string     `json:"last_error,omitempty"`
}

type NftablesForwardStatus struct {
	PluginID    string                      `json:"plugin_id"`
	Version     string                      `json:"version"`
	GeneratedAt time.Time                   `json:"generated_at"`
	Summary     NftablesForwardSummary      `json:"summary"`
	Rules       []NftablesForwardRuleStatus `json:"rules"`
}

type nftablesForwardConfig struct {
	Family string                `json:"family"`
	Table  string                `json:"table"`
	Rules  []nftablesForwardRule `json:"rules"`
}

type nftablesForwardRule struct {
	ID            string `json:"id"`
	Protocol      string `json:"protocol"`
	ListenAddress string `json:"listen_address"`
	ListenPort    int    `json:"listen_port"`
	TargetAddress string `json:"target_address"`
	TargetPort    int    `json:"target_port"`
	Comment       string `json:"comment"`
}

func (e *NftablesForwardExecutor) HandleRoute(_ context.Context, request RouteRequest) (RouteResponse, error) {
	if request.Path != NftablesForwardStatusRoute {
		return RouteResponse{}, ErrRouteNotFound
	}
	if request.Method != http.MethodGet {
		return RouteResponse{}, ErrMethodNotAllowed
	}
	if e.db == nil {
		return RouteResponse{}, errors.New("nftables-forward database is not initialized")
	}
	limit, err := gostMeshLimit(request.Query.Get("limit"))
	if err != nil {
		return RouteResponse{}, err
	}
	status, err := e.status(limit)
	if err != nil {
		return RouteResponse{}, err
	}
	return RouteResponse{Status: http.StatusOK, Data: status}, nil
}

func (e *NftablesForwardExecutor) ExecuteLifecycle(ctx context.Context, request LifecycleRequest) (json.RawMessage, error) {
	return executeReadOnlyAssignmentLifecycle(ctx, request, func(ctx context.Context) (any, error) {
		response, err := e.HandleRoute(ctx, RouteRequest{Method: http.MethodGet, Path: NftablesForwardStatusRoute})
		return response.Data, err
	})
}

func (e *NftablesForwardExecutor) status(limit int) (NftablesForwardStatus, error) {
	assignments, nodes, cursors, operations, err := loadPluginAssignmentState(e.db, NftablesForwardPluginID, limit)
	if err != nil {
		return NftablesForwardStatus{}, err
	}
	runtimeStates, err := loadNftablesForwardRuntimeStates(e.db, assignments)
	if err != nil {
		return NftablesForwardStatus{}, err
	}
	now := time.Now()
	if e.now != nil {
		now = e.now()
	}
	status := NftablesForwardStatus{
		PluginID: NftablesForwardPluginID, Version: NftablesForwardVersion, GeneratedAt: now,
		Rules: make([]NftablesForwardRuleStatus, 0, len(assignments)),
	}
	for _, assignment := range assignments {
		node := nodes[assignment.NodeID]
		operation := operations[assignment.NodeID]
		observed := decodeObservedPluginResult(operation.ResultJSON)
		runtimeState := runtimeStates[assignment.NodeID]
		var config nftablesForwardConfig
		_ = json.Unmarshal([]byte(operation.ConfigJSON), &config)
		if len(config.Rules) == 0 {
			status.Rules = append(status.Rules, nftablesForwardRuleRow(assignment, node, cursors[assignment.NodeID], operation, observed, runtimeState, config, nil, now))
			continue
		}
		for index := range config.Rules {
			status.Rules = append(status.Rules, nftablesForwardRuleRow(assignment, node, cursors[assignment.NodeID], operation, observed, runtimeState, config, &config.Rules[index], now))
		}
	}
	for _, rule := range status.Rules {
		status.Summary.Rules++
		if rule.Ready {
			status.Summary.Ready++
		}
		if rule.Reconciling {
			status.Summary.Reconciling++
		}
		if rule.Degraded {
			status.Summary.Degraded++
		}
		if rule.RollbackRequired {
			status.Summary.RollbackRequired++
		}
	}
	return status, nil
}

func nftablesForwardRuleRow(assignment model.NodeServiceAssignment, node model.Node, cursor model.NodeOperationRevision, operation model.KernelOperation, observed observedPluginResult, runtime *model.NodePluginObservedState, config nftablesForwardConfig, rule *nftablesForwardRule, now time.Time) NftablesForwardRuleStatus {
	row := NftablesForwardRuleStatus{
		ID: fmt.Sprintf("assignment-%d", assignment.ID), Name: node.Name + " / " + assignment.Role,
		NodeID: assignment.NodeID, NodeName: node.Name, Topology: assignment.Role, Protocol: "tcp+udp",
		DesiredRevision:  assignment.DesiredConfigRevision,
		ObservedRevision: maxInt64(cursor.ObservedRevision, observed.ObservedRevision), Enabled: assignment.Enabled,
	}
	if rule != nil {
		if rule.ID != "" {
			row.ID = rule.ID
		}
		row.Name = firstNonEmpty(rule.Comment, rule.ID, row.Name)
		row.Protocol = firstNonEmpty(rule.Protocol, row.Protocol)
		row.Listen = formatEndpoint(rule.ListenAddress, rule.ListenPort)
		row.Target = formatEndpoint(rule.TargetAddress, rule.TargetPort)
	} else if config.Table != "" || config.Family != "" {
		row.Topology = strings.TrimSpace(strings.Trim(strings.Join([]string{assignment.Role, config.Family, config.Table}, " / "), "/"))
	}
	if runtime != nil {
		row.RuntimeHealth = runtime.Health
		row.RulesetSHA256 = runtime.RulesetSHA256
		observedAt := runtime.ObservedAt
		row.ObservedAt = &observedAt
		row.ObservedRevision = maxInt64(row.ObservedRevision, runtime.ObservedRevision)
		if rule != nil {
			row.Packets, row.Bytes = nftablesForwardCounter(runtime.CountersJSON, rule.ID)
		}
	}
	observedVersion := observed.ObservedVersion
	if observedVersion == "" && operation.State == "succeeded" {
		observedVersion = operation.TargetVersion
	}
	row.RollbackRequired = operation.State == "failed" && (operation.Kind == "plugin.update" || operation.Kind == "plugin.rollback")
	runtimeStale := nftablesForwardRuntimeObservationStale(runtime, now)
	requiresRuntimeObservation := assignment.DesiredVersion == NftablesForwardVersion
	runtimeReady := !requiresRuntimeObservation || (runtime != nil && !runtimeStale && runtime.Version == assignment.DesiredVersion && runtime.Health == "healthy")
	row.Degraded = assignment.Enabled && runtime != nil && (runtime.Health != "healthy" || runtimeStale)
	row.Reconciling = assignment.Enabled && (operationInFlight(operation.State) || (!row.RollbackRequired && row.ObservedRevision < row.DesiredRevision) || (requiresRuntimeObservation && runtime == nil))
	health := firstNonEmpty(row.RuntimeHealth, observed.Health, boolHealth(node.RuntimeHealthy))
	if row.RollbackRequired {
		row.LastError = "operation requires rollback"
	} else if runtimeStale {
		row.LastError = "runtime observation is stale"
	} else if row.Degraded {
		row.LastError = "runtime observation is unhealthy"
	} else if row.Reconciling {
		row.LastError = "awaiting desired runtime state"
	}
	row.Ready = assignment.Enabled && !row.RollbackRequired && !row.Reconciling && !row.Degraded && runtimeReady && health == "healthy" &&
		observedVersion == assignment.DesiredVersion && row.ObservedRevision >= row.DesiredRevision
	return row
}

func nftablesForwardRuntimeObservationStale(runtime *model.NodePluginObservedState, now time.Time) bool {
	if runtime == nil || now.IsZero() || runtime.ReceivedAt.IsZero() {
		return runtime != nil
	}
	if runtime.ReceivedAt.After(now.Add(time.Minute)) {
		return true
	}
	return now.Sub(runtime.ReceivedAt) >= nftablesForwardRuntimeStaleAfter
}

func loadNftablesForwardRuntimeStates(db *gorm.DB, assignments []model.NodeServiceAssignment) (map[uint]*model.NodePluginObservedState, error) {
	states := make(map[uint]*model.NodePluginObservedState)
	if db == nil || len(assignments) == 0 || !db.Migrator().HasTable(&model.NodePluginObservedState{}) {
		return states, nil
	}
	nodeIDs := make([]uint, 0, len(assignments))
	seen := make(map[uint]struct{}, len(assignments))
	for _, assignment := range assignments {
		if _, exists := seen[assignment.NodeID]; !exists {
			seen[assignment.NodeID] = struct{}{}
			nodeIDs = append(nodeIDs, assignment.NodeID)
		}
	}
	var rows []model.NodePluginObservedState
	if err := db.Where("plugin_id = ? AND node_id IN ?", NftablesForwardPluginID, nodeIDs).Find(&rows).Error; err != nil {
		return nil, err
	}
	for index := range rows {
		row := rows[index]
		states[row.NodeID] = &row
	}
	return states, nil
}

func nftablesForwardCounter(countersJSON, ruleID string) (uint64, uint64) {
	var counters []struct {
		RuleID  string `json:"rule_id"`
		Packets uint64 `json:"packets"`
		Bytes   uint64 `json:"bytes"`
	}
	if json.Unmarshal([]byte(countersJSON), &counters) != nil {
		return 0, 0
	}
	for _, counter := range counters {
		if counter.RuleID == ruleID {
			return counter.Packets, counter.Bytes
		}
	}
	return 0, 0
}
