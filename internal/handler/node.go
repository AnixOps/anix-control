package handler

import (
	"net/http"
	"strconv"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/service"
	"github.com/gin-gonic/gin"
)

// NodeHandler 节点处理器
type NodeHandler struct {
	nodeService    *service.NodeService
	nodeLogService *service.NodeLogService
}

// NewNodeHandler 创建节点处理器
func NewNodeHandler() *NodeHandler {
	return &NodeHandler{
		nodeService:    service.NewNodeService(),
		nodeLogService: service.NewNodeLogService(),
	}
}

// ========== 管理员节点管理 ==========

// CreateNode godoc
// @Summary 创建节点
// @Description 管理员手动创建节点
// @Tags 管理端-节点
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body model.Node true "节点信息"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 500 {object} map[string]any
// @Router /admin/nodes [post]
func (h *NodeHandler) CreateNode(c *gin.Context) {
	var node model.Node
	if err := c.ShouldBindJSON(&node); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "参数错误", "error": err.Error()})
		return
	}

	if err := h.nodeService.CreateNode(&node); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "创建失败", "error": err.Error()})
		return
	}

	panelSuccess(c, gin.H{"node_id": node.ID})
}

// UpdateNode godoc
// @Summary 更新节点
// @Description 管理员更新指定节点的信息
// @Tags 管理端-节点
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "节点ID"
// @Param request body map[string]any true "节点更新信息"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 500 {object} map[string]any
// @Router /admin/nodes/{id} [put]
func (h *NodeHandler) UpdateNode(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "无效的节点ID"})
		return
	}

	var updates map[string]any
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "参数错误"})
		return
	}

	// The service never updates the node's id or credentials (id, api_key,
	// api_key_hash, secret), in any spelling.
	if err := h.nodeService.UpdateNode(uint(id), updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "更新失败"})
		return
	}

	panelSuccess(c, gin.H{"message": "更新成功"})
}
