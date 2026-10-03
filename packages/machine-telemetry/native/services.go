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
	"time"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/sdk/telemetry/systemdreport"
	"gorm.io/gorm"
)

// The per-node systemd services table (docs/architecture/package-reports.md):
// GET /api/v3/plugins/machine-telemetry/nodes/:id/services, read-only. The
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

// servicesPath is the one path NodesRoute serves: a decimal proxy node id
// without leading zeros.
var servicesPath = regexp.MustCompile(`^/api/v3/plugins/machine-telemetry/nodes/([1-9][0-9]{0,9})/services$`)

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

// NodeServices is GET /api/v3/plugins/machine-telemetry/nodes/:id/services:
// the node's latest systemd services table, for administrators.
func (s *Service) NodeServices(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	match := servicesPath.FindStringSubmatch(request.Metadata.Path)
	if match == nil {
		return kernelError(http.StatusNotFound, "not_found", "route not found")
	}
	nodeID, err := strconv.ParseUint(match[1], 10, 32)
	if err != nil || nodeID == 0 {
		return kernelError(http.StatusNotFound, "not_found", "route not found")
	}
	if request.Method != http.MethodGet {
		return kernelError(http.StatusMethodNotAllowed, "method_not_allowed", "only GET is allowed")
	}
	if !request.Principal.Admin {
		return kernelError(http.StatusForbidden, "forbidden", "the services table is for administrators")
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
	for _, unit := range report.Units {
		// The node's current globs apply at once, before the collector
		// sends its next report with them.
		if !settings.Selected(unit.Name) {
			continue
		}
		answer.Units = append(answer.Units, ServiceUnit(unit))
		answer.Summary.Total++
		switch unit.ActiveState {
		case "failed":
			answer.Summary.Failed++
		case "active":
			answer.Summary.Active++
		case "inactive":
			answer.Summary.Inactive++
		}
	}
	return answer, nil
}

// nodeSettings reads the node's services settings from the package's Agent
// installation configuration: off without an installation, a configuration
// or an entry for the node.
func nodeSettings(db *gorm.DB, nodeID uint64) (systemdreport.NodeConfig, error) {
	var document sql.NullString
	result := db.Table(pluginConfigurationView).Select("config_json").
		Where("plugin_id = ? AND target = ?", PackageID, agentTarget).Limit(1).Scan(&document)
	if result.Error != nil {
		return systemdreport.NodeConfig{}, result.Error
	}
	if result.RowsAffected == 0 || !document.Valid {
		return systemdreport.NodeConfig{}, nil
	}
	config, err := systemdreport.ParseConfig([]byte(document.String))
	if err != nil {
		// The kernel validates the settings when they are saved; a
		// document it stored before this release could still be anything.
		if errors.Is(err, systemdreport.ErrInvalidConfig) {
			return systemdreport.NodeConfig{}, nil
		}
		return systemdreport.NodeConfig{}, err
	}
	return config.Node(nodeID), nil
}

// kernelError is the kernel API's error answer,
// {"error": {"code": ..., "message": ...}}.
func kernelError(code int, errorCode, message string) (pluginhostsdk.NativeResponse, error) {
	return jsonAnswer(code, map[string]any{"error": map[string]string{"code": errorCode, "message": message}})
}
