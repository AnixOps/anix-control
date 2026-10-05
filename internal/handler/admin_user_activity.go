package handler

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/subscriber"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// MaxAdminIDs is the most ids one administrator request names: the activity
// lookup's ids and a bulk action's ids.
const MaxAdminIDs = 200

// parseAdminIDs reads a comma separated list of positive ids, in order and
// without repeats. It refuses an empty list, a value that is not a positive
// id, and more than MaxAdminIDs ids.
func parseAdminIDs(raw string) ([]uint, error) {
	var ids []uint
	seen := map[uint]struct{}{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, err := strconv.ParseUint(part, 10, 32)
		if err != nil || id == 0 {
			return nil, fmt.Errorf("%q is not a positive id", part)
		}
		if _, repeated := seen[uint(id)]; repeated {
			continue
		}
		seen[uint(id)] = struct{}{}
		ids = append(ids, uint(id))
		if len(ids) > MaxAdminIDs {
			return nil, fmt.Errorf("at most %d ids", MaxAdminIDs)
		}
	}
	if len(ids) == 0 {
		return nil, errors.New("ids is required")
	}
	return ids, nil
}

// AdminUserActivityHandler answers when users were last seen online.
type AdminUserActivityHandler struct {
	db func() *gorm.DB
}

// NewAdminUserActivityHandler reads the kernel database.
func NewAdminUserActivityHandler() *AdminUserActivityHandler {
	return &AdminUserActivityHandler{db: database.Get}
}

// UserActivityEntry is one user's activity: when the user was last seen
// online, a Unix time in seconds, or null for a user never seen.
type UserActivityEntry struct {
	UserID       uint   `json:"user_id"`
	LastOnlineAt *int64 `json:"last_online_at"`
}

// List godoc
// @Summary 用户最近在线时间
// @Description 按 ids 返回用户最近一次在线的 Unix 时间 (秒)。在线指节点上报了该用户的流量或在线连接, 时间滞后上报不超过一分钟; 从未在线的用户 last_online_at 为 null。ids 最多 200 个, 按请求顺序去重返回。
// @Tags 管理端-用户
// @Produce json
// @Security BearerAuth
// @Param ids query string true "用户 ID, 逗号分隔, 最多 200 个"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Router /api/v4/admin/users/activity [get]
func (h *AdminUserActivityHandler) List(c *gin.Context) {
	ids, err := parseAdminIDs(c.Query("ids"))
	if err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	db := h.db()
	if db == nil {
		kernelError(c, http.StatusServiceUnavailable, "database_unavailable", "database is not initialized")
		return
	}
	seen, err := subscriber.LastOnline(db.WithContext(c.Request.Context()), ids)
	if err != nil {
		kernelDBError(c, err)
		return
	}
	users := make([]UserActivityEntry, 0, len(ids))
	for _, id := range ids {
		entry := UserActivityEntry{UserID: id}
		if at, ok := seen[id]; ok {
			entry.LastOnlineAt = &at
		}
		users = append(users, entry)
	}
	c.Header("Cache-Control", "no-store")
	kernelData(c, http.StatusOK, gin.H{"users": users})
}
