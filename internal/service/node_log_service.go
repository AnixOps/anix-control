package service

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

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
	if nodeID == 0 {
		return errors.New("node_id is required")
	}

	var exists int64
	if err := s.db.Model(&model.Node{}).Where("id = ?", nodeID).Count(&exists).Error; err != nil {
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

	return s.db.Create(&logs).Error
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
