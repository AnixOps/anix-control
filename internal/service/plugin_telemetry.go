package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	pluginTelemetryPrefix          = "plugin."
	maxPluginTelemetryMetrics      = 128
	maxPluginTelemetryMetricLength = 96
	maxPluginTelemetrySampleAge    = 24 * time.Hour
	maxLegacyTelemetrySampleAge    = 5 * time.Minute
)

// RecordPluginTelemetry persists only metrics from enabled assignments on the
// authenticated node. Non-plugin heartbeat metrics remain kernel-owned and
// are intentionally excluded from this plugin state table.
func (s *NodeService) RecordPluginTelemetry(nodeID uint, heartbeatMetrics map[string]float64, receivedAt time.Time) (int, error) {
	if s == nil || s.db == nil {
		return 0, errors.New("database is not initialized")
	}
	if nodeID == 0 {
		return 0, errors.New("telemetry node_id is required")
	}
	if receivedAt.IsZero() {
		receivedAt = time.Now()
	}
	rawMetrics, err := validatePluginHeartbeatMetrics(heartbeatMetrics)
	if err != nil || len(rawMetrics) == 0 {
		return 0, err
	}

	authorized, authorizationErr := s.resolveTelemetryAssignments(nodeID)
	grouped, groupingErr := groupAssignedPluginTelemetry(rawMetrics, authorized)
	if groupingErr != nil {
		authorizationErr = errors.Join(authorizationErr, groupingErr)
	}
	if len(grouped) == 0 {
		return 0, authorizationErr
	}

	pluginIDs := make([]string, 0, len(grouped))
	for pluginID := range grouped {
		pluginIDs = append(pluginIDs, pluginID)
	}
	sort.Strings(pluginIDs)
	accepted := 0
	var result error = authorizationErr
	for _, pluginID := range pluginIDs {
		metrics := grouped[pluginID]
		if persistErr := s.persistPluginTelemetryState(nodeID, pluginID, metrics, receivedAt); persistErr != nil {
			result = errors.Join(result, persistErr)
			continue
		}
		accepted += len(metrics)
	}
	return accepted, result
}

// resolveTelemetryAssignments admits a plugin only when the exact desired
// release assigned to the node is an AnixOps-signed Agent package declaring
// telemetry.read. Checking the release rather than just the plugin ID prevents
// an unrelated official plugin, or a stale/tampered assignment, from claiming
// the machine telemetry namespace.
func (s *NodeService) resolveTelemetryAssignments(nodeID uint) ([]string, error) {
	var assignments []model.NodeServiceAssignment
	if err := s.db.Where("node_id = ? AND enabled = ? AND delete_pending = ?", nodeID, true, false).
		Order("plugin_id, desired_version, id").Find(&assignments).Error; err != nil {
		return nil, fmt.Errorf("resolve telemetry plugin assignments: %w", err)
	}
	if len(assignments) == 0 {
		return nil, nil
	}
	pluginIDs := make([]string, 0, len(assignments))
	seenPluginIDs := make(map[string]struct{}, len(assignments))
	for _, assignment := range assignments {
		if _, seen := seenPluginIDs[assignment.PluginID]; seen {
			continue
		}
		seenPluginIDs[assignment.PluginID] = struct{}{}
		pluginIDs = append(pluginIDs, assignment.PluginID)
	}
	var official []string
	if err := s.db.Model(&model.Plugin{}).
		Where("official = ? AND publisher = ? AND id IN ?", true, "AnixOps", pluginIDs).
		Pluck("id", &official).Error; err != nil {
		return nil, fmt.Errorf("resolve official telemetry plugins: %w", err)
	}
	officialSet := make(map[string]struct{}, len(official))
	for _, pluginID := range official {
		officialSet[pluginID] = struct{}{}
	}
	authorizedSet := make(map[string]struct{})
	var result error
	checked := make(map[string]struct{}, len(assignments))
	for _, assignment := range assignments {
		if _, official := officialSet[assignment.PluginID]; !official {
			continue
		}
		checkKey := assignment.PluginID + "\x00" + assignment.DesiredVersion
		if _, done := checked[checkKey]; done {
			continue
		}
		checked[checkKey] = struct{}{}
		if strings.TrimSpace(assignment.DesiredVersion) == "" {
			result = errors.Join(result, fmt.Errorf("telemetry assignment %s has no desired plugin version", assignment.PluginID))
			continue
		}
		var release model.PluginRelease
		if err := s.db.Where("plugin_id = ? AND version = ?", assignment.PluginID, assignment.DesiredVersion).First(&release).Error; err != nil {
			result = errors.Join(result, fmt.Errorf("resolve telemetry plugin %s release %s: %w", assignment.PluginID, assignment.DesiredVersion, err))
			continue
		}
		manifest, err := VerifyStoredPluginRelease(s.db, release, nil)
		if err != nil {
			result = errors.Join(result, fmt.Errorf("verify telemetry plugin %s release %s: %w", assignment.PluginID, assignment.DesiredVersion, err))
			continue
		}
		if !manifestSupportsTarget(*manifest, "agent") {
			result = errors.Join(result, fmt.Errorf("telemetry plugin %s release %s is not Agent-compatible", assignment.PluginID, assignment.DesiredVersion))
			continue
		}
		if !containsPluginCapability(manifest.Capabilities, "telemetry.read") {
			result = errors.Join(result, fmt.Errorf("telemetry plugin %s release %s does not declare telemetry.read", assignment.PluginID, assignment.DesiredVersion))
			continue
		}
		authorizedSet[assignment.PluginID] = struct{}{}
	}
	authorized := make([]string, 0, len(authorizedSet))
	for pluginID := range authorizedSet {
		authorized = append(authorized, pluginID)
	}
	sort.Slice(authorized, func(left, right int) bool {
		if len(authorized[left]) == len(authorized[right]) {
			return authorized[left] < authorized[right]
		}
		return len(authorized[left]) > len(authorized[right])
	})
	return authorized, result
}

func containsPluginCapability(capabilities []string, wanted string) bool {
	for _, capability := range capabilities {
		if capability == wanted {
			return true
		}
	}
	return false
}

func (s *NodeService) persistPluginTelemetryState(nodeID uint, pluginID string, metrics map[string]float64, receivedAt time.Time) error {
	observedAt := receivedAt
	legacyFresh := true
	if sampleAge, ok := metrics["sample_age_seconds"]; ok {
		if sampleAge < 0 || sampleAge > maxPluginTelemetrySampleAge.Seconds() {
			return fmt.Errorf("telemetry plugin %s sample age is invalid", pluginID)
		}
		observedAt = receivedAt.Add(-time.Duration(sampleAge * float64(time.Second)))
		legacyFresh = sampleAge <= maxLegacyTelemetrySampleAge.Seconds()
	}
	encoded, err := json.Marshal(metrics)
	if err != nil {
		return fmt.Errorf("encode telemetry plugin %s metrics: %w", pluginID, err)
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		state := model.PluginTelemetryState{
			NodeID: nodeID, PluginID: pluginID, MetricsJSON: string(encoded),
			ObservedAt: observedAt, ReceivedAt: receivedAt, UpdatedAt: receivedAt,
		}
		result := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "node_id"}, {Name: "plugin_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"metrics_json", "observed_at", "received_at", "updated_at"}),
			Where:     clause.Where{Exprs: []clause.Expression{clause.Expr{SQL: "excluded.observed_at >= observed_at"}}},
		}).Create(&state)
		if result.Error != nil {
			return fmt.Errorf("persist telemetry plugin %s metrics: %w", pluginID, result.Error)
		}
		if result.RowsAffected == 0 {
			return nil
		}
		if pluginID == "machine-telemetry" && legacyFresh && tx.Migrator().HasTable(&model.Node{}) {
			updates := machineTelemetryLegacyUpdates(metrics)
			if len(updates) > 0 {
				if updateErr := tx.Model(&model.Node{}).Where("id = ?", nodeID).Updates(updates).Error; updateErr != nil {
					return fmt.Errorf("update legacy node telemetry: %w", updateErr)
				}
			}
		}
		return nil
	})
}

func validatePluginHeartbeatMetrics(heartbeatMetrics map[string]float64) (map[string]float64, error) {
	raw := make(map[string]float64)
	count := 0
	for key, value := range heartbeatMetrics {
		if !strings.HasPrefix(key, pluginTelemetryPrefix) {
			continue
		}
		count++
		if count > maxPluginTelemetryMetrics {
			return nil, fmt.Errorf("plugin telemetry exceeds %d metrics", maxPluginTelemetryMetrics)
		}
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return nil, fmt.Errorf("plugin telemetry metric %q is not finite", key)
		}
		remainder := strings.TrimPrefix(key, pluginTelemetryPrefix)
		if separator := strings.IndexByte(remainder, '.'); separator < 1 || separator == len(remainder)-1 || !safeTelemetryNamespaceRemainder(remainder) {
			return nil, fmt.Errorf("plugin telemetry metric %q has an invalid namespace", key)
		}
		raw[remainder] = value
	}
	return raw, nil
}

func safeTelemetryNamespaceRemainder(value string) bool {
	if value == "" || strings.HasPrefix(value, ".") || strings.HasSuffix(value, ".") || strings.Contains(value, "..") {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || character == '_' || character == '.' || character == '-' {
			continue
		}
		return false
	}
	return true
}

func groupAssignedPluginTelemetry(raw map[string]float64, assigned []string) (map[string]map[string]float64, error) {
	pluginIDs := make([]string, 0, len(assigned))
	for _, pluginID := range assigned {
		if safePluginSegment(pluginID) {
			pluginIDs = append(pluginIDs, pluginID)
		}
	}
	sort.Slice(pluginIDs, func(left, right int) bool {
		if len(pluginIDs[left]) == len(pluginIDs[right]) {
			return pluginIDs[left] < pluginIDs[right]
		}
		return len(pluginIDs[left]) > len(pluginIDs[right])
	})
	grouped := make(map[string]map[string]float64)
	for namespacedKey, value := range raw {
		for _, pluginID := range pluginIDs {
			prefix := pluginID + "."
			if !strings.HasPrefix(namespacedKey, prefix) {
				continue
			}
			metricKey := strings.TrimPrefix(namespacedKey, prefix)
			if !safePluginTelemetryMetricKey(metricKey) {
				return nil, fmt.Errorf("plugin telemetry metric %q is invalid", pluginTelemetryPrefix+namespacedKey)
			}
			if grouped[pluginID] == nil {
				grouped[pluginID] = make(map[string]float64)
			}
			grouped[pluginID][metricKey] = value
			break
		}
	}
	return grouped, nil
}

func safePluginTelemetryMetricKey(value string) bool {
	if value == "" || len(value) > maxPluginTelemetryMetricLength {
		return false
	}
	for index, character := range value {
		if (character >= 'a' && character <= 'z') ||
			(character >= '0' && character <= '9') || character == '_' || character == '.' || character == '-' {
			if index == 0 && character >= '0' && character <= '9' {
				return false
			}
			continue
		}
		return false
	}
	return true
}

func machineTelemetryLegacyUpdates(metrics map[string]float64) map[string]any {
	updates := make(map[string]any)
	if value, ok := metrics["cpu_usage_percent"]; ok && value >= 0 && value <= 100 {
		updates["cpu_usage"] = value
	}
	if value, ok := metrics["memory_usage_percent"]; ok && value >= 0 && value <= 100 {
		updates["memory_usage"] = value
	}
	if value, ok := metrics["disk_usage_percent"]; ok && value >= 0 && value <= 100 {
		updates["disk_usage"] = value
	}
	if value, ok := metrics["uptime_seconds"]; ok && value >= 0 && value <= math.MaxInt64 {
		updates["uptime"] = int64(value)
	}
	return updates
}
