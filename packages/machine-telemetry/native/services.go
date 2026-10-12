package native

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/sdk/telemetry/systemdreport"
	"gorm.io/gorm"
)

// The per-node systemd services table (docs/architecture/package-reports.md):
// GET /api/v3/plugins/machine-telemetry/nodes/:id/services, read-only, and
// the summary of several nodes for the node list,
// GET /api/v3/plugins/machine-telemetry/nodes/services?ids=1,2,3. The
// Agent's collector reports the table as a systemd.services package report;
// the kernel sanitizes and keeps the latest one per node, which the package
// reads through kapi_package_report_v1. Whether the node is enabled, and its
// unit globs, come from the package's Agent installation configuration,
// read through kapi_plugin_configuration_v1.
const (
	// PackageID is the package's id.
	PackageID = "machine-telemetry"
	// NodesRoute is the control route that serves the table: the kernel
	// matches the request path against it and dispatches the request with
	// NodesRouteID.
	NodesRoute = "/api/v3/plugins/machine-telemetry/nodes/*"

	packageReportView       = "kapi_package_report_v1"
	pluginConfigurationView = "kapi_plugin_configuration_v1"
	proxyNodeKind           = "proxy"
	agentTarget             = "agent"
)

// NodesRouteID is the route id the kernel gives NodesRoute: the package id,
// ".control." and the hex SHA-256 of the package id, a NUL byte and the
// route (the kernel's service.PluginControlBridgeRouteID).
var NodesRouteID = controlRouteID(NodesRoute)

func controlRouteID(route string) string {
	digest := sha256.Sum256([]byte(PackageID + "\x00" + route))
	return PackageID + ".control." + hex.EncodeToString(digest[:])
}

// servicesPath is the table of one node: a decimal proxy node id without
// leading zeros.
var servicesPath = regexp.MustCompile(`^/api/v3/plugins/machine-telemetry/nodes/([1-9][0-9]{0,9})/services$`)

// summaryPath is the summary of the nodes the query parameter ids lists
// (comma separated, or repeated), at most MaxSummaryNodes of them.
const summaryPath = "/api/v3/plugins/machine-telemetry/nodes/services"

// MaxSummaryNodes is the most nodes one summary request may list: a page of
// the node list is 20 rows, its largest page size 100.
const MaxSummaryNodes = 200

// nodeIDPattern is a proxy node id: decimal, without leading zeros.
var nodeIDPattern = regexp.MustCompile(`^[1-9][0-9]{0,9}$`)

// ServiceUnit is one unit of the table.
type ServiceUnit struct {
	Name            string  `json:"name"`
	ActiveState     string  `json:"active_state"`
	SubState        string  `json:"sub_state"`
	CPUAvgPercent   float64 `json:"cpu_avg_percent"`
	CPUPeakPercent  float64 `json:"cpu_peak_percent"`
	MemoryBytes     uint64  `json:"memory_bytes"`
	MemoryPeakBytes uint64  `json:"memory_peak_bytes"`
}

// ServicesSummary counts the units by ActiveState. Units activating,
// deactivating or reloading count only in Total.
type ServicesSummary struct {
	Total    int `json:"total"`
	Failed   int `json:"failed"`
	Active   int `json:"active"`
	Inactive int `json:"inactive"`
}

// NodeServices is the route's answer.
type NodeServices struct {
	NodeID uint64 `json:"node_id"`
	// Enabled is the node's setting; without it the collector is off and
	// the table is empty.
	Enabled bool `json:"enabled"`
	// Include and Exclude are the node's unit globs.
	Include []string `json:"include"`
	Exclude []string `json:"exclude"`
	// Reported is true once the node has a stored report.
	Reported bool `json:"reported"`
	// Supported and UnsupportedReason are the report's: false, with a
	// reason, on a node without systemd or cgroup v2.
	Supported         bool   `json:"supported"`
	UnsupportedReason string `json:"unsupported_reason"`
	// Stale is true when the report is older than 25 minutes.
	Stale         bool            `json:"stale"`
	ObservedAt    *time.Time      `json:"observed_at"`
	Version       string          `json:"version"`
	WindowSeconds int             `json:"window_seconds"`
	Summary       ServicesSummary `json:"summary"`
	Units         []ServiceUnit   `json:"units"`
}

// NodeServicesSummary is one node of the summary: NodeServices without the
// units and the node's globs.
type NodeServicesSummary struct {
	NodeID            uint64          `json:"node_id"`
	Enabled           bool            `json:"enabled"`
	Reported          bool            `json:"reported"`
	Supported         bool            `json:"supported"`
	UnsupportedReason string          `json:"unsupported_reason"`
	Stale             bool            `json:"stale"`
	ObservedAt        *time.Time      `json:"observed_at"`
	Summary           ServicesSummary `json:"summary"`
}

// NodeServices answers the routes under NodesRoute:
// GET /api/v3/plugins/machine-telemetry/nodes/:id/services, the node's
// latest systemd services table, and
// GET /api/v3/plugins/machine-telemetry/nodes/services?ids=..., the summary
// of those nodes, both for administrators.
func (s *Service) NodeServices(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	if request.Metadata.Path == summaryPath {
		return s.nodeServicesSummaryRoute(ctx, request)
	}
	match := servicesPath.FindStringSubmatch(request.Metadata.Path)
	if match == nil {
		return kernelError(http.StatusNotFound, "not_found", "route not found")
	}
	nodeID, err := strconv.ParseUint(match[1], 10, 32)
	if err != nil || nodeID == 0 {
		return kernelError(http.StatusNotFound, "not_found", "route not found")
	}
	if refused, answer, err := refuseRequest(request); refused {
		return answer, err
	}
	db, err := s.Open(ctx)
	if err != nil {
		return kernelError(http.StatusServiceUnavailable, "storage_unavailable", "package storage is unavailable")
	}
	answer, err := s.nodeServices(db, nodeID)
	if err != nil {
		return kernelError(http.StatusInternalServerError, "services_unavailable", "the services table could not be read")
	}
	return jsonAnswer(http.StatusOK, map[string]any{"data": answer})
}

// refuseRequest answers a request that is not an administrator's GET.
func refuseRequest(request pluginhostsdk.NativeRequest) (bool, pluginhostsdk.NativeResponse, error) {
	if request.Method != http.MethodGet {
		answer, err := kernelError(http.StatusMethodNotAllowed, "method_not_allowed", "only GET is allowed")
		return true, answer, err
	}
	if !request.Principal.Admin {
		answer, err := kernelError(http.StatusForbidden, "forbidden", "the services table is for administrators")
		return true, answer, err
	}
	return false, pluginhostsdk.NativeResponse{}, nil
}

func (s *Service) nodeServicesSummaryRoute(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	if refused, answer, err := refuseRequest(request); refused {
		return answer, err
	}
	nodeIDs, ok := summaryNodeIDs(request.Metadata.Query["ids"])
	if !ok {
		return kernelError(http.StatusBadRequest, "invalid_request",
			"ids must list 1 to "+strconv.Itoa(MaxSummaryNodes)+" proxy node ids, comma separated")
	}
	db, err := s.Open(ctx)
	if err != nil {
		return kernelError(http.StatusServiceUnavailable, "storage_unavailable", "package storage is unavailable")
	}
	nodes, err := s.nodeServicesSummary(db, nodeIDs)
	if err != nil {
		return kernelError(http.StatusInternalServerError, "services_unavailable", "the services summary could not be read")
	}
	return jsonAnswer(http.StatusOK, map[string]any{"data": map[string]any{"nodes": nodes}})
}

// summaryNodeIDs parses the ids query values: decimal proxy node ids without
// leading zeros, comma separated or repeated, in their first order, without
// duplicates, 1 to MaxSummaryNodes of them.
func summaryNodeIDs(values []string) ([]uint64, bool) {
	seen := make(map[uint64]bool)
	var ids []uint64
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			if !nodeIDPattern.MatchString(part) {
				return nil, false
			}
			id, err := strconv.ParseUint(part, 10, 32)
			if err != nil {
				return nil, false
			}
			if seen[id] {
				continue
			}
			seen[id] = true
			ids = append(ids, id)
			if len(ids) > MaxSummaryNodes {
				return nil, false
			}
		}
	}
	return ids, len(ids) > 0
}

// nodeServicesSummary reads the settings once and the reports of the
// enabled nodes in one query, and answers one entry per node, in order.
func (s *Service) nodeServicesSummary(db *gorm.DB, nodeIDs []uint64) ([]NodeServicesSummary, error) {
	config, err := servicesConfig(db)
	if err != nil {
		return nil, err
	}
	answers := make([]NodeServicesSummary, len(nodeIDs))
	var enabled []uint64
	for index, nodeID := range nodeIDs {
		answers[index] = NodeServicesSummary{NodeID: nodeID, Enabled: config.Node(nodeID).Enabled}
		if answers[index].Enabled {
			enabled = append(enabled, nodeID)
		}
	}
	if len(enabled) == 0 {
		return answers, nil
	}
	var rows []struct {
		NodeID      uint64
		PayloadJSON string
		ObservedAt  time.Time
	}
	if err := db.Table(packageReportView).Select("node_id, payload_json, observed_at").
		Where("node_kind = ? AND plugin_id = ? AND kind = ? AND node_id IN ?", proxyNodeKind, PackageID, systemdreport.Kind, enabled).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	byNode := make(map[uint64]int, len(nodeIDs))
	for index, answer := range answers {
		byNode[answer.NodeID] = index
	}
	now := s.now()
	for _, row := range rows {
		index, ok := byNode[row.NodeID]
		if !ok {
			continue
		}
		report, err := systemdreport.Sanitize([]byte(row.PayloadJSON))
		if err != nil {
			return nil, err
		}
		observedAt := row.ObservedAt.UTC()
		answer := &answers[index]
		answer.Reported = true
		answer.ObservedAt = &observedAt
		answer.Stale = systemdreport.IsStale(observedAt, now)
		answer.Supported = report.Supported
		answer.UnsupportedReason = report.UnsupportedReason
		_, answer.Summary = selectedUnits(config.Node(row.NodeID), report)
	}
	return answers, nil
}

func (s *Service) nodeServices(db *gorm.DB, nodeID uint64) (NodeServices, error) {
	answer := NodeServices{NodeID: nodeID, Include: []string{}, Exclude: []string{}, Units: []ServiceUnit{}}
	settings, err := nodeSettings(db, nodeID)
	if err != nil {
		return NodeServices{}, err
	}
	answer.Enabled = settings.Enabled
	if settings.Include != nil {
		answer.Include = settings.Include
	}
	if settings.Exclude != nil {
		answer.Exclude = settings.Exclude
	}
	if !answer.Enabled {
		return answer, nil
	}

	var row struct {
		Version     string
		PayloadJSON string
		ObservedAt  time.Time
	}
	result := db.Table(packageReportView).Select("version, payload_json, observed_at").
		Where("node_kind = ? AND node_id = ? AND plugin_id = ? AND kind = ?", proxyNodeKind, nodeID, PackageID, systemdreport.Kind).
		Limit(1).Scan(&row)
	if result.Error != nil {
		return NodeServices{}, result.Error
	}
	if result.RowsAffected == 0 {
		return answer, nil
	}
	// The kernel stored the sanitizer's encoding; sanitizing it again
	// costs little and keeps the answer to the whitelisted fields whatever
	// the view shows.
	report, err := systemdreport.Sanitize([]byte(row.PayloadJSON))
	if err != nil {
		return NodeServices{}, err
	}
	observedAt := row.ObservedAt.UTC()
	answer.Reported = true
	answer.Version = row.Version
	answer.ObservedAt = &observedAt
	answer.Stale = systemdreport.IsStale(observedAt, s.now())
	answer.Supported = report.Supported
	answer.UnsupportedReason = report.UnsupportedReason
	answer.WindowSeconds = report.WindowSeconds
	units, summary := selectedUnits(settings, report)
	answer.Units = append(answer.Units, units...)
	answer.Summary = summary
	return answer, nil
}

// selectedUnits applies the node's current globs to a report, at once,
// before the collector sends its next report with them, and counts the
// units by ActiveState.
func selectedUnits(settings systemdreport.NodeConfig, report systemdreport.Report) ([]ServiceUnit, ServicesSummary) {
	var units []ServiceUnit
	var summary ServicesSummary
	for _, unit := range report.Units {
		if !settings.Selected(unit.Name) {
			continue
		}
		units = append(units, ServiceUnit(unit))
		summary.Total++
		switch unit.ActiveState {
		case "failed":
			summary.Failed++
		case "active":
			summary.Active++
		case "inactive":
			summary.Inactive++
		}
	}
	return units, summary
}

// nodeSettings reads the node's services settings from the package's Agent
// installation configuration: off without an installation, a configuration
// or an entry for the node.
func nodeSettings(db *gorm.DB, nodeID uint64) (systemdreport.NodeConfig, error) {
	config, err := servicesConfig(db)
	if err != nil {
		return systemdreport.NodeConfig{}, err
	}
	return config.Node(nodeID), nil
}

// servicesConfig reads the services settings of every node: empty without
// an installation or a configuration.
func servicesConfig(db *gorm.DB) (systemdreport.Config, error) {
	var document sql.NullString
	result := db.Table(pluginConfigurationView).Select("config_json").
		Where("plugin_id = ? AND target = ?", PackageID, agentTarget).Limit(1).Scan(&document)
	if result.Error != nil {
		return systemdreport.Config{}, result.Error
	}
	if result.RowsAffected == 0 || !document.Valid {
		return systemdreport.Config{}, nil
	}
	config, err := systemdreport.ParseConfig([]byte(document.String))
	if err != nil {
		// The kernel validates the settings when they are saved; a
		// document it stored before this release could still be anything.
		if errors.Is(err, systemdreport.ErrInvalidConfig) {
			return systemdreport.Config{}, nil
		}
		return systemdreport.Config{}, err
	}
	return config, nil
}

// kernelError is the kernel API's error answer,
// {"error": {"code": ..., "message": ...}}.
func kernelError(code int, errorCode, message string) (pluginhostsdk.NativeResponse, error) {
	return jsonAnswer(code, map[string]any{"error": map[string]string{"code": errorCode, "message": message}})
}
