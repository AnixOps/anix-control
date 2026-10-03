package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v4/internal/agentreports"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
)

const (
	NodeLogLevelDebug   = "debug"
	NodeLogLevelInfo    = "info"
	NodeLogLevelWarning = "warning"
	NodeLogLevelError   = "error"
)

type NodeLogInput struct {
	Level      string
	Source     string
	Message    string
	TraceID    string
	FieldsJSON string
	LoggedAt   *time.Time
}

type NodeLogListParams struct {
	NodeID   uint
	Page     int
	PageSize int
	Level    string
	Source   string
	Search   string
}

type NodeLogListResult struct {
	Total int64           `json:"total"`
	List  []model.NodeLog `json:"list"`
}

type NodeLogService struct {
	db *gorm.DB
}

func NewNodeLogService() *NodeLogService {
	return &NodeLogService{db: database.GetDB()}
}

func NormalizeNodeLogLevel(level string) string {
	level = strings.ToLower(strings.TrimSpace(level))
	if level == "" {
		return ""
	}

	switch level {
	case "dbg", "debug":
		return NodeLogLevelDebug
	case "information", "info":
		return NodeLogLevelInfo
	case "warn", "warning":
		return NodeLogLevelWarning
	case "err", "error", "fatal", "panic":
		return NodeLogLevelError
	default:
		return level
	}
}

func sanitizeNodeLogFieldsJSON(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}

	var decoded any
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return "", err
	}

	canonical, err := json.Marshal(decoded)
	if err != nil {
		return "", err
	}
	return string(canonical), nil
}

func (s *NodeLogService) RecordLogs(nodeID uint, inputs []NodeLogInput) error {
	return s.RecordLogsTx(s.db, nodeID, inputs)
}

// RecordAgentLogBatch records a LogBatch from the Agent Control stream
// (reports.v1) as RecordLogs records the legacy NodeLogService batch, once
// per node and batch id: the batch record (internal/agentreports) is claimed
// in the transaction that inserts the rows. It reports whether this call
// recorded the batch; false means a committed transaction recorded it
// before. gorm.ErrRecordNotFound means the node does not exist.
func (s *NodeLogService) RecordAgentLogBatch(nodeKind string, nodeID uint, batchID string, inputs []NodeLogInput) (bool, error) {
	applied := false
	err := s.db.Transaction(func(tx *gorm.DB) error {
		seen, err := agentreports.ClaimTx(tx, nodeKind, nodeID, batchID, agentreports.KindLogs, time.Now())
		if err != nil || seen {
			return err
		}
		applied = true
		return s.RecordLogsTx(tx, nodeID, inputs)
	})
	return applied, err
}

// RecordLogsTx is RecordLogs inside tx.
func (s *NodeLogService) RecordLogsTx(tx *gorm.DB, nodeID uint, inputs []NodeLogInput) error {
	if nodeID == 0 {
		return errors.New("node_id is required")
	}

	var exists int64
	if err := tx.Model(&model.Node{}).Where("id = ?", nodeID).Count(&exists).Error; err != nil {
		return err
	}
	if exists == 0 {
		return gorm.ErrRecordNotFound
	}
	if len(inputs) == 0 {
		return nil
	}

	logs := make([]model.NodeLog, 0, len(inputs))
	for _, input := range inputs {
		message := strings.TrimSpace(input.Message)
		if message == "" {
			continue
		}

		fieldsJSON, err := sanitizeNodeLogFieldsJSON(input.FieldsJSON)
		if err != nil {
			return err
		}

		logs = append(logs, model.NodeLog{
			NodeID: nodeID,
			Level: func() string {
				level := NormalizeNodeLogLevel(input.Level)
				if level == "" {
					return NodeLogLevelInfo
				}
				return level
			}(),
			Source:     strings.TrimSpace(input.Source),
			Message:    message,
			TraceID:    strings.TrimSpace(input.TraceID),
			FieldsJSON: fieldsJSON,
			LoggedAt:   input.LoggedAt,
		})
	}

	if len(logs) == 0 {
		return nil
	}

	return tx.Create(&logs).Error
}

func (s *NodeLogService) GetLogs(params NodeLogListParams) (*NodeLogListResult, error) {
	var (
		total int64
		logs  []model.NodeLog
	)

	query := s.db.Model(&model.NodeLog{}).Where("node_id = ?", params.NodeID)

	if level := NormalizeNodeLogLevel(params.Level); level != "" {
		query = query.Where("level = ?", level)
	}
	if source := strings.TrimSpace(params.Source); source != "" {
		query = query.Where("source = ?", source)
	}
	if search := strings.TrimSpace(params.Search); search != "" {
		like := "%" + search + "%"
		query = query.Where("message LIKE ? OR source LIKE ? OR trace_id LIKE ?", like, like, like)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}

	offset := (params.Page - 1) * params.PageSize
	if err := query.Order("COALESCE(logged_at, created_at) DESC, id DESC").
		Limit(params.PageSize).
		Offset(offset).
		Find(&logs).Error; err != nil {
		return nil, err
	}

	return &NodeLogListResult{
		Total: total,
		List:  logs,
	}, nil
}

// NodeLogSourceMaintenance is the source of the node log rows that record
// an Agent's maintenance events (maintenance.v1).
const NodeLogSourceMaintenance = "maintenance"

// RecordAgentMaintenanceEvent records one maintenance event of the Agent
// Control stream (maintenance.v1) once per node and event id: a v2_node_log
// row of source "maintenance" whose fields_json is the event, inserted in
// the transaction that claims the event's record (internal/agentreports,
// kind maintenance). It reports whether this call recorded the event; false
// means a committed transaction recorded it before. gorm.ErrRecordNotFound
// means the node does not exist.
func (s *NodeLogService) RecordAgentMaintenanceEvent(nodeKind string, nodeID uint, event agentcontrol.MaintenanceEvent) (bool, error) {
	fields, err := json.Marshal(event)
	if err != nil {
		return false, err
	}
	occurredAt := event.OccurredAt.UTC()
	input := NodeLogInput{
		Level: maintenanceLogLevel(event), Source: NodeLogSourceMaintenance, Message: maintenanceLogMessage(event),
		FieldsJSON: string(fields), LoggedAt: &occurredAt,
	}
	if len(event.EventID) <= 120 {
		input.TraceID = event.EventID
	}
	applied := false
	err = s.db.Transaction(func(tx *gorm.DB) error {
		seen, err := agentreports.ClaimTx(tx, nodeKind, nodeID, agentreports.MaintenanceBatchID(event.EventID), agentreports.KindMaintenance, time.Now())
		if err != nil || seen {
			return err
		}
		applied = true
		return s.RecordLogsTx(tx, nodeID, []NodeLogInput{input})
	})
	if err != nil {
		applied = false
	}
	return applied, err
}

// maintenanceLogLevel is the node log level of a maintenance event: a
// recovery is info; an incident error at P0 and P1, warning at P2, info at
// P3.
func maintenanceLogLevel(event agentcontrol.MaintenanceEvent) string {
	switch {
	case event.Status == "recovered", event.Severity == "P3":
		return NodeLogLevelInfo
	case event.Severity == "P0", event.Severity == "P1":
		return NodeLogLevelError
	}
	return NodeLogLevelWarning
}

// maintenanceLogMessage is the node log line of a maintenance event.
func maintenanceLogMessage(event agentcontrol.MaintenanceEvent) string {
	message := fmt.Sprintf("maintenance %s: plugin %s instance %s %s (%s)", event.Status, event.PluginID, event.InstanceID, event.ErrorCode, event.Severity)
	if summary := strings.TrimSpace(event.RedactedSummary); summary != "" {
		message += ": " + summary
	}
	return message
}
