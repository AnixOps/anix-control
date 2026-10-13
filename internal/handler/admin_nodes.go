package handler

import (
	"net/http"
	"strconv"

	"github.com/AnixOps/anix-control/sdk/v2compat"
	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// AdminNodesHandler is the console's node list: the proxy nodes as GET
// /api/v2/admin/nodes answers them, with what the node groups add (5.0 node
// onboarding): each node's approval, the node group it joined and that
// group's name, and a filter on both. The /api/v2 list keeps its shape (the
// package gateway's compatibility contract); the console reads this one.
type AdminNodesHandler struct {
	nodes *service.NodeService
	db    func() *gorm.DB
}

// NewAdminNodesHandler reads the kernel database.
func NewAdminNodesHandler() *AdminNodesHandler {
	return &AdminNodesHandler{nodes: service.NewNodeService(), db: database.Get}
}

// adminNodeRow is a v2 node row with its placement. The model's own
// approval and node group fields stay out of the /api/v2 JSON (json:"-");
// these shadow them here.
type adminNodeRow struct {
	model.Node
	// Approval is approved, pending (waiting for an administrator) or
	// rejected.
	Approval string `json:"approval"`
	// NodeGroupID is the node group whose join token created the node;
	// null for a node an administrator created by hand.
	NodeGroupID   *uint  `json:"node_group_id"`
	NodeGroupName string `json:"node_group_name"`
}

// List godoc
// @Summary 节点列表 (含审批与节点组)
// @Description 与 GET /api/v2/admin/nodes 同口径的节点列表 (分页、搜索、状态与订阅分组筛选、排序、在线状态、协议, 协议与原始配置里的密钥为占位符), 每个节点另带 approval (approved、pending 或 rejected)、node_group_id 和 node_group_name。approval 与 node_group_id 可作筛选条件。/api/v2 的节点列表形状不变。
// @Tags 管理端-节点
// @Produce json
// @Security BearerAuth
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Param search query string false "名称或地址包含"
// @Param status query int false "节点状态"
// @Param group_id query int false "订阅分组 ID"
// @Param approval query string false "审批状态" Enums(pending, approved, rejected)
// @Param node_group_id query int false "节点组 ID"
// @Param sort query string false "排序字段: id, name, host, sort, created_at, last_check_at, cpu_usage, online_users; 缺省为管理员排序" Enums(id, name, host, sort, created_at, last_check_at, cpu_usage, online_users)
// @Param order query string false "排序方向, 仅与 sort 同用" Enums(asc, desc) default(asc)
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Router /api/v4/admin/nodes [get]
func (h *AdminNodesHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	page, pageSize = ClampPagination(page, pageSize)
	orderBy, err := v2compat.ParseListSort(c.Query("sort"), c.Query("order"), service.NodeSortColumns, service.NodeSortTiebreaker)
	if err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	params := service.NodeListParams{Page: page, PageSize: pageSize, Search: c.Query("search"), OrderBy: orderBy}
	if value := c.Query("status"); value != "" {
		parsed, _ := strconv.Atoi(value)
		status := model.NodeStatus(parsed)
		params.Status = &status
	}
	if value := c.Query("group_id"); value != "" {
		parsed, _ := strconv.ParseUint(value, 10, 32)
		id := uint(parsed)
		params.GroupID = &id
	}
	switch approval := c.Query("approval"); approval {
	case "":
	case model.NodeApprovalPending, model.NodeApprovalApproved, model.NodeApprovalRejected:
		params.Approval = approval
	default:
		kernelError(c, http.StatusBadRequest, "invalid_request", "approval must be pending, approved or rejected")
		return
	}
	if value := c.Query("node_group_id"); value != "" {
		parsed, err := strconv.ParseUint(value, 10, 32)
		if err != nil || parsed == 0 {
			kernelError(c, http.StatusBadRequest, "invalid_request", "node_group_id must be a node group id")
			return
		}
		id := uint(parsed)
		params.NodeGroupID = &id
	}
	result, err := h.nodes.GetNodes(params)
	if err != nil {
		kernelDBError(c, err)
		return
	}
	rows, err := h.rows(result.List)
	if err != nil {
		kernelDBError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	kernelData(c, http.StatusOK, gin.H{"total": result.Total, "list": rows})
}

// rows redacts the nodes as the /api/v2 list does and adds their node group
// names with one query.
func (h *AdminNodesHandler) rows(nodes []model.Node) ([]adminNodeRow, error) {
	ids := map[uint]bool{}
	for _, node := range nodes {
		if node.NodeGroupID != nil {
			ids[*node.NodeGroupID] = true
		}
	}
	names := map[uint]string{}
	if len(ids) > 0 {
		list := make([]uint, 0, len(ids))
		for id := range ids {
			list = append(list, id)
		}
		var groups []model.NodeGroup
		// Unscoped: a rejected node may outlive its deleted group.
		if err := h.db().Unscoped().Select("id", "name").Where("id IN ?", list).Find(&groups).Error; err != nil {
			return nil, err
		}
		for _, group := range groups {
			names[group.ID] = group.Name
		}
	}
	rows := make([]adminNodeRow, 0, len(nodes))
	for i := range nodes {
		node := nodes[i]
		service.RedactNode(&node)
		row := adminNodeRow{Node: node, Approval: node.Approval, NodeGroupID: node.NodeGroupID}
		if row.Approval == "" {
			row.Approval = model.NodeApprovalApproved
		}
		if node.NodeGroupID != nil {
			row.NodeGroupName = names[*node.NodeGroupID]
		}
		rows = append(rows, row)
	}
	return rows, nil
}
