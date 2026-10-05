package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/gin-gonic/gin"
)

// AdminGroupMembersHandler lists the users of a subscription group.
type AdminGroupMembersHandler struct {
	subscriptions *service.SubscriptionService
}

// NewAdminGroupMembersHandler reads the kernel database.
func NewAdminGroupMembersHandler() *AdminGroupMembersHandler {
	return &AdminGroupMembersHandler{subscriptions: service.NewSubscriptionService()}
}

// List godoc
// @Summary 订阅分组成员
// @Description 分页列出被直接授予该订阅分组的用户 (v2_user_subscription_group), 与分组统计的 user_count / enabled_users 同口径; 通过套餐或主分组到达该分组的用户不在其中。按授予时间倒序。不含订阅 token 与 uuid。
// @Tags 管理端-订阅
// @Produce json
// @Security BearerAuth
// @Param id path int true "订阅分组 ID"
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量 (最多 100)" default(20)
// @Param q query string false "邮箱包含"
// @Param status query string false "成员资格: active (未过期) 或 expired" Enums(active, expired)
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 404 {object} map[string]any
// @Router /api/v4/admin/subscription-groups/{id}/members [get]
func (h *AdminGroupMembersHandler) List(c *gin.Context) {
	groupID, ok := parseKernelID(c, "id")
	if !ok {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	page, pageSize = ClampPagination(page, pageSize)
	result, err := h.subscriptions.ListGroupMembers(groupID, service.GroupMembersParams{
		Page: page, PageSize: pageSize, Email: c.Query("q"), Status: c.Query("status"),
	})
	switch {
	case errors.Is(err, service.ErrInvalidMemberStatus):
		kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	case errors.Is(err, service.ErrSubscriptionGroupNotFound):
		kernelError(c, http.StatusNotFound, "not_found", "subscription group not found")
		return
	case err != nil:
		kernelDBError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	kernelData(c, http.StatusOK, result)
}
