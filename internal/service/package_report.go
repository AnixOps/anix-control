package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/sdk/telemetry/systemdreport"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Package reports (package-reports.v1; docs/architecture/package-reports.md).
// An Agent plugin package reports its latest observation of one kind for the
// node. The kernel stores it only when the node's assigned release of the
// package is a signed official Agent release whose manifest declares the
// capability the kind requires, after the kind's sanitizer has reduced the
// payload to its whitelisted fields. One row per node, plugin and kind; a
// newer report replaces it and an older one is dropped.

// Why a package report was refused: the reason label of
// anixops_agent_package_reports_refused_total.
const (
	// PackageReportRefusedInvalid: plugin_id, kind or version is malformed.
	PackageReportRefusedInvalid = "invalid"
	// PackageReportRefusedOversize: payload_json exceeds 256 KiB.
	PackageReportRefusedOversize = "oversize"
	// PackageReportRefusedFuture: observed_at is more than a minute ahead.
	PackageReportRefusedFuture = "future"
	// PackageReportRefusedUnknownKind: the kernel knows no such kind.
	PackageReportRefusedUnknownKind = "unknown_kind"
	// PackageReportRefusedNotAssigned: the plugin is not enabled on the node.
	PackageReportRefusedNotAssigned = "not_assigned"
	// PackageReportRefusedVersionMismatch: the node is assigned another
	// version of the plugin.
	PackageReportRefusedVersionMismatch = "version_mismatch"
	// PackageReportRefusedUnsigned: the release is not a signed official
	// (AnixOps) release, or its signature no longer verifies.
	PackageReportRefusedUnsigned = "unsigned"
	// PackageReportRefusedMissingCapability: the release is not an Agent
	// release declaring the kind's capability.
	PackageReportRefusedMissingCapability = "missing_capability"
	// PackageReportRefusedBadPayload: the kind's sanitizer refused the
	// payload.
	PackageReportRefusedBadPayload = "bad_payload"
	// PackageReportRefusedUnnegotiated: a forward report (plugin_id
	// "forward", kind "forward.report") on a session that did not negotiate
	// forward.v1.
	PackageReportRefusedUnnegotiated = "unnegotiated"
)

// PackageReportRefusalReasons lists every refusal reason, in metric order.
var PackageReportRefusalReasons = []string{
	PackageReportRefusedInvalid, PackageReportRefusedOversize, PackageReportRefusedFuture,
	PackageReportRefusedUnknownKind, PackageReportRefusedNotAssigned, PackageReportRefusedVersionMismatch,
	PackageReportRefusedUnsigned, PackageReportRefusedMissingCapability, PackageReportRefusedBadPayload,
	PackageReportRefusedUnnegotiated,
}

const (
	maxPackageReportVersionLength = 64
	maxPackageReportFutureSkew    = time.Minute
	defaultPackageReportStaleness = 25 * time.Minute
)

// PackageReportRefusedError is a permanent refusal of one report.
type PackageReportRefusedError struct {
	Reason string
	Detail string
}

func (e *PackageReportRefusedError) Error() string {
	return "package report refused (" + e.Reason + "): " + e.Detail
}

func refusePackageReport(reason, format string, args ...any) error {
	return &PackageReportRefusedError{Reason: reason, Detail: fmt.Sprintf(format, args...)}
}

// PackageReportRefusal returns the reason of a refusal, or "" for any other
// error (one the kernel could not record for now).
func PackageReportRefusal(err error) string {
	var refused *PackageReportRefusedError
	if errors.As(err, &refused) {
		return refused.Reason
	}
	return ""
}

// packageReportKind is what the kernel requires of one report kind.
type packageReportKind struct {
	// capability must be declared by the reporting release's manifest.
	capability string
	// sanitize returns the payload to store: only the kind's whitelisted
	// fields, re-encoded.
	sanitize func(payload []byte) ([]byte, error)
	// staleAfter is how old a report may be before it reads as stale.
	staleAfter time.Duration
}

// packageReportKinds are the kinds the kernel accepts. A package cannot add
// one: each is reviewed here with its capability and sanitizer.
var packageReportKinds = map[string]packageReportKind{
	systemdreport.Kind: {
		capability: systemdreport.Capability,
		sanitize: func(payload []byte) ([]byte, error) {
			report, err := systemdreport.Sanitize(payload)
			if err != nil {
				return nil, err
			}
			return json.Marshal(report)
		},
		staleAfter: systemdreport.StaleAfter,
	},
}

// PackageReportInput is one PackageReport from an authenticated node.
type PackageReportInput struct {
	NodeKind string
	NodeID   uint
	PluginID string
	Kind     string
	Version  string
	Payload  []byte
	// ObservedAt is zero when the Agent did not say; the kernel then uses
	// the receive time.
	ObservedAt time.Time
}

// RecordPackageReport authorizes, sanitizes and stores a package report as
// the latest of its node, plugin and kind. It returns true when the report
// was stored and false when a newer one was stored before (the report is
// dropped). A *PackageReportRefusedError refuses the report for good; any
// other error means the kernel could not record it now.
func (s *NodeService) RecordPackageReport(input PackageReportInput, receivedAt time.Time) (bool, error) {
	if s == nil || s.db == nil {
		return false, errors.New("database is not initialized")
	}
	if receivedAt.IsZero() {
		receivedAt = time.Now()
	}
	if input.NodeID == 0 {
		return false, refusePackageReport(PackageReportRefusedInvalid, "node_id is required")
	}
	if !safePluginSegment(input.PluginID) {
		return false, refusePackageReport(PackageReportRefusedInvalid, "plugin_id is invalid")
	}
	if !safePluginSegment(input.Version) || len(input.Version) > maxPackageReportVersionLength {
		return false, refusePackageReport(PackageReportRefusedInvalid, "version is invalid")
	}
	if err := agentcontrol.ValidatePackageReportKind(input.Kind); err != nil {
		return false, refusePackageReport(PackageReportRefusedInvalid, "%v", err)
	}
	if err := agentcontrol.ValidatePackageReportPayloadSize(input.Payload); err != nil {
		return false, refusePackageReport(PackageReportRefusedOversize, "%v", err)
	}
	observedAt := input.ObservedAt
	if observedAt.IsZero() {
		observedAt = receivedAt
	}
	if observedAt.After(receivedAt.Add(maxPackageReportFutureSkew)) {
		return false, refusePackageReport(PackageReportRefusedFuture, "observed_at is ahead of the kernel clock")
	}
	kind, known := packageReportKinds[input.Kind]
	if !known {
		return false, refusePackageReport(PackageReportRefusedUnknownKind, "kind %q is not known", input.Kind)
	}
	if err := s.authorizePackageReport(input, kind.capability); err != nil {
		return false, err
	}
	payload, err := kind.sanitize(input.Payload)
	if err != nil {
		return false, refusePackageReport(PackageReportRefusedBadPayload, "%v", err)
	}
	return s.persistPackageReport(input, payload, observedAt, receivedAt)
}

// authorizePackageReport requires an enabled assignment of the plugin on
// the node at the reported version, and that release to be a verifiable,
// official Agent release declaring capability.
func (s *NodeService) authorizePackageReport(input PackageReportInput, capability string) error {
	if input.NodeKind != agentcontrol.NodeKindProxy {
		// Forward nodes have no plugin assignments yet; they join with
		// the forward package reports.
		return refusePackageReport(PackageReportRefusedNotAssigned, "%s nodes have no plugin assignments", input.NodeKind)
	}
	var versions []string
	if err := s.db.Model(&model.NodeServiceAssignment{}).
		Where("node_id = ? AND plugin_id = ? AND enabled = ? AND delete_pending = ?", input.NodeID, input.PluginID, true, false).
		Pluck("desired_version", &versions).Error; err != nil {
		return fmt.Errorf("authorize package report assignment: %w", err)
	}
	if len(versions) == 0 {
		return refusePackageReport(PackageReportRefusedNotAssigned, "%s is not enabled on the node", input.PluginID)
	}
	if !slices.Contains(versions, input.Version) {
		return refusePackageReport(PackageReportRefusedVersionMismatch, "the node is assigned %s %s, not %s", input.PluginID, strings.Join(versions, ", "), input.Version)
	}
	var official int64
	if err := s.db.Model(&model.Plugin{}).Where("id = ? AND official = ? AND publisher = ?", input.PluginID, true, "AnixOps").
		Count(&official).Error; err != nil {
		return fmt.Errorf("authorize package report plugin: %w", err)
	}
	if official == 0 {
		return refusePackageReport(PackageReportRefusedUnsigned, "%s is not an official plugin", input.PluginID)
	}
	var release model.PluginRelease
	if err := s.db.First(&release, "plugin_id = ? AND version = ?", input.PluginID, input.Version).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return refusePackageReport(PackageReportRefusedUnsigned, "%s %s has no stored release", input.PluginID, input.Version)
		}
		return fmt.Errorf("load package report release: %w", err)
	}
	manifest, err := VerifyStoredPluginRelease(s.db, release, nil)
	if err != nil {
		return refusePackageReport(PackageReportRefusedUnsigned, "verify %s %s: %v", input.PluginID, input.Version, err)
	}
	if !manifestSupportsTarget(*manifest, "agent") || !containsPluginCapability(manifest.Capabilities, capability) {
		return refusePackageReport(PackageReportRefusedMissingCapability, "%s %s is not an Agent release declaring %s", input.PluginID, input.Version, capability)
	}
	return nil
}

func (s *NodeService) persistPackageReport(input PackageReportInput, payload []byte, observedAt, receivedAt time.Time) (bool, error) {
	state := model.PackageReportState{
		NodeKind: input.NodeKind, NodeID: input.NodeID, PluginID: input.PluginID, Kind: input.Kind,
		Version: input.Version, PayloadJSON: string(payload),
		ObservedAt: observedAt.UTC(), ReceivedAt: receivedAt.UTC(), UpdatedAt: receivedAt.UTC(),
	}
	result := s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "node_kind"}, {Name: "node_id"}, {Name: "plugin_id"}, {Name: "kind"}},
		DoUpdates: clause.AssignmentColumns([]string{"version", "payload_json", "observed_at", "received_at", "updated_at"}),
		Where:     clause.Where{Exprs: []clause.Expression{clause.Expr{SQL: "excluded.observed_at >= " + model.PackageReportState{}.TableName() + ".observed_at"}}},
	}).Create(&state)
	if result.Error != nil {
		return false, fmt.Errorf("persist package report %s %s: %w", input.PluginID, input.Kind, result.Error)
	}
	return result.RowsAffected > 0, nil
}

// PackageReport is the latest report of a node, plugin and kind, as the
// kernel stored it.
type PackageReport struct {
	NodeKind   string          `json:"node_kind"`
	NodeID     uint            `json:"node_id"`
	PluginID   string          `json:"plugin_id"`
	Kind       string          `json:"kind"`
	Version    string          `json:"version"`
	Payload    json.RawMessage `json:"payload"`
	ObservedAt time.Time       `json:"observed_at"`
	ReceivedAt time.Time       `json:"received_at"`
	// Stale is true when the report is older than the kind allows (25
	// minutes for systemd.services) at the time it was read.
	Stale bool `json:"stale"`
}

// LatestPackageReport returns the latest report of a node, plugin and kind,
// or gorm.ErrRecordNotFound.
func (s *NodeService) LatestPackageReport(nodeKind string, nodeID uint, pluginID, kind string, now time.Time) (PackageReport, error) {
	if s == nil || s.db == nil {
		return PackageReport{}, errors.New("database is not initialized")
	}
	var state model.PackageReportState
	if err := s.db.First(&state, "node_kind = ? AND node_id = ? AND plugin_id = ? AND kind = ?", nodeKind, nodeID, pluginID, kind).Error; err != nil {
		return PackageReport{}, err
	}
	if now.IsZero() {
		now = time.Now()
	}
	return PackageReport{
		NodeKind: state.NodeKind, NodeID: state.NodeID, PluginID: state.PluginID, Kind: state.Kind,
		Version: state.Version, Payload: json.RawMessage(state.PayloadJSON),
		ObservedAt: state.ObservedAt, ReceivedAt: state.ReceivedAt,
		Stale: PackageReportStale(kind, state.ObservedAt, now),
	}, nil
}

// PackageReportStale reports whether a report of kind observed at
// observedAt is stale at now.
func PackageReportStale(kind string, observedAt, now time.Time) bool {
	staleAfter := defaultPackageReportStaleness
	if known, ok := packageReportKinds[kind]; ok && known.staleAfter > 0 {
		staleAfter = known.staleAfter
	}
	return now.Sub(observedAt) > staleAfter
}
