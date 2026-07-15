package handler

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/AnixOps/anix-control/v3/internal/database"
	"github.com/AnixOps/anix-control/v3/internal/model"
	"github.com/AnixOps/anix-control/v3/internal/utils"
	"github.com/gin-gonic/gin"
)

var auditKeyValueSecretPattern = regexp.MustCompile(`(?i)((?:password|passwd|secret|token|api[_-]?key|private[_-]?key|access[_-]?key)\s*[=:]\s*)([^,\s;]+)`)

func sanitizeAuditLogContent(content string) string {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return ""
	}

	redactedJSON := utils.RedactJSON(trimmed)
	if redactedJSON != trimmed {
		return redactedJSON
	}

	return auditKeyValueSecretPattern.ReplaceAllString(trimmed, `${1}[REDACTED]`)
}

func auditLogResponse(entry *model.OperationLog) gin.H {
	if entry == nil {
		return gin.H{}
	}

	return gin.H{
		"id":          entry.ID,
		"user_id":     entry.UserID,
		"username":    entry.Username,
		"action":      entry.Action,
		"module":      entry.Module,
		"target_type": entry.TargetType,
		"target_id":   entry.TargetID,
		"content":     sanitizeAuditLogContent(entry.Content),
		"ip":          entry.IP,
		"user_agent":  entry.UserAgent,
		"status":      entry.Status,
		"created_at":  entry.CreatedAt,
	}
}

// GetAuditLogs godoc
// @Summary Get system audit logs
// @Description Admin query for system operation audit logs with optional filters.
// @Tags Admin System
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param target_type query string false "Filter by target type"
// @Param action query string false "Filter by action"
// @Param page query int false "Page number" default(1)
// @Param page_size query int false "Page size" default(20)
// @Success 200 {object} map[string]any
// @Failure 500 {object} map[string]any
// @Router /admin/system/audit-logs [get]
func (h *SystemHandler) GetAuditLogs(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 200 {
		pageSize = 200
	}

	targetType := strings.TrimSpace(c.Query("target_type"))
	action := strings.TrimSpace(c.Query("action"))

	query := database.Get().Model(&model.OperationLog{}).Where("module = ?", "system")
	if targetType != "" {
		query = query.Where("target_type = ?", targetType)
	}
	if action != "" {
		query = query.Where("action = ?", action)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		panelError(c, err.Error())
		return
	}

	var records []model.OperationLog
	offset := (page - 1) * pageSize
	if err := query.Order("id DESC").Limit(pageSize).Offset(offset).Find(&records).Error; err != nil {
		panelError(c, err.Error())
		return
	}

	list := make([]gin.H, 0, len(records))
	for i := range records {
		list = append(list, auditLogResponse(&records[i]))
	}

	panelSuccess(c, gin.H{
		"list":      list,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}
