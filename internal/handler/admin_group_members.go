package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/AnixOps/anix-control/v5/internal/service"
	"github.com/gin-gonic/gin"
)

// AdminGroupMembersHandler lists the users of a subscription group and
// sets how the group presents its node protocols.
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

type groupProtocolLayoutRequest struct {
	Protocols []service.GroupProtocolLayoutItem `json:"protocols"`
}

// ProtocolLayout godoc
// @Summary 订阅分组节点协议的显示名称与顺序
// @Description 列出关联到该分组的全部节点协议 (v2_subscription_group_node_protocols), 按分组顺序: 已排序的在前, 其余按协议 ID。display_name 为订阅中显示的节点名称, 空表示默认的 "<节点> - <协议>"。
// @Tags 管理端-订阅
// @Produce json
// @Security BearerAuth
// @Param id path int true "订阅分组 ID"
// @Success 200 {object} map[string]any
// @Failure 404 {object} map[string]any
// @Router /api/v4/admin/subscription-groups/{id}/protocol-layout [get]
func (h *AdminGroupMembersHandler) ProtocolLayout(c *gin.Context) {
	groupID, ok := parseKernelID(c, "id")
	if !ok {
		return
	}
	items, err := h.subscriptions.GroupProtocolLayout(groupID)
	h.layoutResult(c, groupID, items, err)
}

// ReplaceProtocolLayout godoc
// @Summary 设置订阅分组节点协议的显示名称与顺序
// @Description 按列表顺序排列该分组的节点协议并设置显示名称 (去除首尾空白, 最多 128 个字符, 不含控制字符; 空表示默认名称)。每项必须是已关联到该分组的协议且只出现一次; 未列出的已关联协议排在其后, 使用默认名称。节点有入口时, 名称后仍追加 " | <入口>"。订阅立即生效。
// @Tags 管理端-订阅
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "订阅分组 ID"
// @Param request body groupProtocolLayoutRequest true "协议顺序与显示名称"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 404 {object} map[string]any
// @Router /api/v4/admin/subscription-groups/{id}/protocol-layout [put]
func (h *AdminGroupMembersHandler) ReplaceProtocolLayout(c *gin.Context) {
	groupID, ok := parseKernelID(c, "id")
	if !ok {
		return
	}
	var request groupProtocolLayoutRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	items, err := h.subscriptions.ReplaceGroupProtocolLayout(groupID, request.Protocols)
	h.layoutResult(c, groupID, items, err)
}

func (h *AdminGroupMembersHandler) layoutResult(c *gin.Context, groupID uint, items []service.GroupProtocolLayoutItem, err error) {
	switch {
	case errors.Is(err, service.ErrInvalidGroupProtocolLayout):
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
	kernelData(c, http.StatusOK, gin.H{"group_id": groupID, "protocols": items})
}
