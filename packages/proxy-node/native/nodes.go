package native

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
)

// nodeStatusView is the kernel view of the nodes: id, status, last check
// and traffic counters, never a node's credentials.
const nodeStatusView = "kapi_node_status_v1"

// nodeStatusPending is the kernel's model.NodeStatusPending.
const nodeStatusPending = 0

// onlineWindowSeconds is how recent a node's last check must be for the
// statistics to count it online, as in the kernel.
const onlineWindowSeconds = 300

// NodeLog is a v2_node_log row: a runtime log line a node reported.
type NodeLog struct {
	ID         uint `gorm:"primaryKey"`
	NodeID     uint
	Level      string
	Source     string
	Message    string
	TraceID    string
	FieldsJSON string
	LoggedAt   *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// TableName is the adopted kernel table.
func (NodeLog) TableName() string { return "v2_node_log" }

// GetNodeStats is GET /api/v2/admin/nodes/stats. A node is online when it
// checked in within five minutes, whatever its status; offline is what is
// neither online nor pending.
func (s *Service) GetNodeStats(ctx context.Context, _ pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	db, err := s.Open(ctx)
	if err != nil {
		return jsonAnswer(http.StatusInternalServerError, map[string]any{"message": "获取统计失败"})
	}
	var total, online, pending, up, down int64
	fiveMinutesAgo := s.now().Unix() - onlineWindowSeconds
	for _, step := range []func() error{
		func() error { return db.Table(nodeStatusView).Count(&total).Error },
		func() error {
			return db.Table(nodeStatusView).Where("last_check_at > ?", fiveMinutesAgo).Count(&online).Error
		},
		func() error {
			return db.Table(nodeStatusView).Where("status = ?", nodeStatusPending).Count(&pending).Error
		},
		func() error {
			return db.Table(nodeStatusView).Select("COALESCE(SUM(total_upload), 0)").Scan(&up).Error
		},
		func() error {
			return db.Table(nodeStatusView).Select("COALESCE(SUM(total_download), 0)").Scan(&down).Error
		},
	} {
		if err := step(); err != nil {
			return jsonAnswer(http.StatusInternalServerError, map[string]any{"message": "获取统计失败"})
		}
	}
	return s.panel(map[string]any{
		"total":   total,
		"online":  online,
		"pending": pending,
		"offline": total - online - pending,
		"up":      up,
		"down":    down,
	})
}

// GetNodeLogs is GET /api/v2/admin/nodes/:id/logs: a node's runtime logs,
// newest first, filtered by level (normalized as the kernel stores it),
// source and a search in the message, source and trace id.
func (s *Service) GetNodeLogs(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	nodeID, ok := pathID(request)
	if !ok {
		return jsonAnswer(http.StatusBadRequest, map[string]any{"message": "无效的节点ID"})
	}
	page, pageSize := pagination(request)
	db, err := s.Open(ctx)
	if err != nil {
		return jsonAnswer(http.StatusInternalServerError, map[string]any{"message": "获取节点日志失败"})
	}
	var nodes int64
	if err := db.Table(nodeStatusView).Where("id = ?", nodeID).Count(&nodes).Error; err != nil || nodes == 0 {
		return jsonAnswer(http.StatusNotFound, map[string]any{"message": "节点不存在"})
	}

	var (
		total int64
		logs  []NodeLog
	)
	tx := db.Model(&NodeLog{}).Where("node_id = ?", nodeID)
	if level := normalizeNodeLogLevel(query(request, "level")); level != "" {
		tx = tx.Where("level = ?", level)
	}
	if source := strings.TrimSpace(query(request, "source")); source != "" {
		tx = tx.Where("source = ?", source)
	}
	if search := strings.TrimSpace(query(request, "search")); search != "" {
		like := "%" + search + "%"
		tx = tx.Where("message LIKE ? OR source LIKE ? OR trace_id LIKE ?", like, like, like)
	}
	if err := tx.Count(&total).Error; err != nil {
		return jsonAnswer(http.StatusInternalServerError, map[string]any{"message": "获取节点日志失败", "error": err.Error()})
	}
	offset := (page - 1) * pageSize
	if err := tx.Order("COALESCE(logged_at, created_at) DESC, id DESC").Limit(pageSize).Offset(offset).Find(&logs).Error; err != nil {
		return jsonAnswer(http.StatusInternalServerError, map[string]any{"message": "获取节点日志失败", "error": err.Error()})
	}

	list := make([]map[string]any, 0, len(logs))
	for _, item := range logs {
		var fields any
		if item.FieldsJSON != "" {
			_ = json.Unmarshal([]byte(item.FieldsJSON), &fields)
		}
		list = append(list, map[string]any{
			"id":          item.ID,
			"node_id":     item.NodeID,
			"level":       item.Level,
			"source":      item.Source,
			"message":     item.Message,
			"trace_id":    item.TraceID,
			"fields":      fields,
			"fields_json": item.FieldsJSON,
			"logged_at":   item.LoggedAt,
			"created_at":  item.CreatedAt,
		})
	}
	return s.panel(map[string]any{
		"list":      list,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// normalizeNodeLogLevel is the kernel's service.NormalizeNodeLogLevel: the
// level the kernel stores for a reported one.
func normalizeNodeLogLevel(level string) string {
	level = strings.ToLower(strings.TrimSpace(level))
	switch level {
	case "":
		return ""
	case "dbg", "debug":
		return "debug"
	case "information", "info":
		return "info"
	case "warn", "warning":
		return "warning"
	case "err", "error", "fatal", "panic":
		return "error"
	default:
		return level
	}
}
