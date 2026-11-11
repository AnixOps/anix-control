package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/sdk/v2compat"
	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/kernelnodeops"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/service"
	"github.com/gin-gonic/gin"
)

// NodeHandler 节点处理器
type NodeHandler struct {
	nodeService    *service.NodeService
	nodeLogService *service.NodeLogService
	agentControl   nodeAgentControl
}

// NewNodeHandler 创建节点处理器
func NewNodeHandler() *NodeHandler {
	return &NodeHandler{
		nodeService:    service.NewNodeService(),
		nodeLogService: service.NewNodeLogService(),
		agentControl:   defaultNodeAgentControl(),
	}
}

// ========== 管理员节点管理 ==========

// GetNodes godoc
// @Summary 获取节点列表
// @Description 管理员获取节点列表，支持分页和筛选
// @Tags 管理端-节点
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Param search query string false "搜索关键词"
// @Param status query int false "节点状态"
// @Param group_id query int false "分组ID"
// @Param sort query string false "排序字段: id, name, host, sort, created_at, last_check_at, cpu_usage, online_users; 缺省为管理员排序 (sort 升序, id 倒序)" Enums(id, name, host, sort, created_at, last_check_at, cpu_usage, online_users)
// @Param order query string false "排序方向, 仅与 sort 同用" Enums(asc, desc) default(asc)
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 500 {object} map[string]any
// @Router /admin/nodes [get]
func (h *NodeHandler) GetNodes(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	page, pageSize = ClampPagination(page, pageSize)
	search := c.Query("search")
	orderBy, err := v2compat.ParseListSort(c.Query("sort"), c.Query("order"), service.NodeSortColumns, service.NodeSortTiebreaker)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	var status *model.NodeStatus
	if s := c.Query("status"); s != "" {
		st, _ := strconv.Atoi(s)
		ns := model.NodeStatus(st)
		status = &ns
	}

	var groupID *uint
	if g := c.Query("group_id"); g != "" {
		gid, _ := strconv.ParseUint(g, 10, 32)
		gidVal := uint(gid)
		groupID = &gidVal
	}

	result, err := h.nodeService.GetNodes(service.NodeListParams{
		Page:     page,
		PageSize: pageSize,
		Status:   status,
		GroupID:  groupID,
		Search:   search,
		OrderBy:  orderBy,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "获取节点列表失败"})
		return
	}
	// Protocol and raw configuration secrets read as the placeholder.
	for i := range result.List {
		service.RedactNode(&result.List[i])
	}

	panelSuccess(c, result)
}

// GetNode godoc
// @Summary 获取节点详情
// @Description 管理员获取指定节点的详细信息
// @Tags 管理端-节点
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "节点ID"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 404 {object} map[string]any
// @Router /admin/nodes/{id} [get]
func (h *NodeHandler) GetNode(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "无效的节点ID"})
		return
	}

	node, err := h.nodeService.GetNode(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "节点不存在"})
		return
	}
	service.RedactNode(node)

	panelSuccess(c, node)
}

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

// DeleteNode godoc
// @Summary 删除节点
// @Description 管理员删除指定节点
// @Tags 管理端-节点
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "节点ID"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 500 {object} map[string]any
// @Router /admin/nodes/{id} [delete]
func (h *NodeHandler) DeleteNode(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "无效的节点ID"})
		return
	}

	if err := h.nodeService.DeleteNode(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "删除失败"})
		return
	}

	panelSuccess(c, gin.H{"message": "删除成功"})
}

// ========== 高级配置 (RawConfig) ==========

// ========== 协议管理 ==========

// GetProtocols godoc
// @Summary 获取节点协议列表
// @Description 管理员获取指定节点的协议配置列表
// @Tags 管理端-节点
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "节点ID"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 500 {object} map[string]any
// @Router /admin/nodes/{id}/protocols [get]
func (h *NodeHandler) GetProtocols(c *gin.Context) {
	nodeID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "无效的节点ID"})
		return
	}

	protocols, err := h.nodeService.GetProtocols(uint(nodeID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "获取协议列表失败"})
		return
	}
	// Secret settings (Reality and WireGuard private keys, passwords) read
	// as the placeholder; an update that sends it back keeps them.
	service.RedactNodeProtocols(protocols)

	panelSuccess(c, protocols)
}

// CreateProtocol godoc
// @Summary 创建节点协议
// @Description 管理员为指定节点创建协议配置
// @Tags 管理端-节点
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "节点ID"
// @Param request body model.NodeProtocol true "协议配置"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 500 {object} map[string]any
// @Router /admin/nodes/{id}/protocols [post]
func (h *NodeHandler) CreateProtocol(c *gin.Context) {
	nodeID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "无效的节点ID"})
		return
	}

	var protocol model.NodeProtocol
	if err := c.ShouldBindJSON(&protocol); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "参数错误", "error": err.Error()})
		return
	}

	protocol.NodeID = uint(nodeID)

	if err := h.nodeService.CreateProtocol(&protocol); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, service.ErrInvalidNodeProtocol) {
			status = http.StatusBadRequest
		}
		c.JSON(status, gin.H{"message": "创建失败", "error": err.Error()})
		return
	}
	service.RedactNodeProtocol(&protocol)

	panelSuccess(c, protocol)
}

// UpdateProtocol godoc
// @Summary 更新节点协议
// @Description 管理员更新指定的节点协议配置
// @Tags 管理端-节点
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param protocol_id path int true "协议ID"
// @Param request body map[string]any true "协议更新信息"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 500 {object} map[string]any
// @Router /admin/nodes/protocols/{protocol_id} [put]
func (h *NodeHandler) UpdateProtocol(c *gin.Context) {
	protocolID, err := strconv.ParseUint(c.Param("protocol_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "无效的协议ID"})
		return
	}

	var updates map[string]any
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "参数错误"})
		return
	}

	// The service never updates the protocol's id or node, in any spelling.
	if err := h.nodeService.UpdateProtocol(uint(protocolID), updates); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, service.ErrInvalidNodeProtocol) {
			status = http.StatusBadRequest
		}
		c.JSON(status, gin.H{"message": "更新失败", "error": err.Error()})
		return
	}

	panelSuccess(c, gin.H{"message": "更新成功"})
}

// DeleteProtocol godoc
// @Summary 删除节点协议
// @Description 管理员删除指定的节点协议配置
// @Tags 管理端-节点
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param protocol_id path int true "协议ID"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 500 {object} map[string]any
// @Router /admin/nodes/protocols/{protocol_id} [delete]
func (h *NodeHandler) DeleteProtocol(c *gin.Context) {
	protocolID, err := strconv.ParseUint(c.Param("protocol_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "无效的协议ID"})
		return
	}

	if err := h.nodeService.DeleteProtocol(uint(protocolID)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "删除失败"})
		return
	}

	panelSuccess(c, gin.H{"message": "删除成功"})
}

// SyncProtocol godoc
// @Summary 同步协议到节点
// @Description 管理员将协议配置同步到指定节点
// @Tags 管理端-节点
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "节点ID"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 500 {object} map[string]any
// @Router /admin/nodes/{id}/sync [post]
func (h *NodeHandler) SyncProtocol(c *gin.Context) {
	nodeID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "无效的节点ID"})
		return
	}

	if _, err := h.nodeService.GetNode(uint(nodeID)); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "节点不存在"})
		return
	}

	// The sync is kernelnodeops.SyncNode, the function the node.sync
	// executor runs: it rebuilds and stores the node's desired
	// configuration, then pushes it to an agent on the Control stream (a
	// ConfigSnapshot with config.v1, else node.reload); a node on the
	// legacy transports pulls it.
	sync, err := kernelnodeops.SyncNode(c.Request.Context(), database.Get(), h.streams(),
		agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: uint32(nodeID)},
		kernelnodeops.SyncNodeOptions{Force: true, AckTimeout: defaultAgentControlOperationTimeout})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "同步失败"})
		return
	}
	sync.Release()
	if sync.Pushed {
		if sync.DispatchErr != nil {
			c.JSON(http.StatusBadGateway, gin.H{"message": "Agent Control 同步下发失败", "error": sync.DispatchErr.Error()})
			return
		}
		if sync.Snapshot != nil {
			// The agent negotiated config.v1: the configuration went out
			// as a snapshot, which the agent answers with ConfigStatus
			// (recorded as the node's applied revision).
			panelSuccess(c, gin.H{
				"message":         "配置快照已通过 AnixOps Agent Control 下发",
				"transport":       "agent-control-grpc",
				"config_revision": sync.Snapshot.GetConfigRevision(),
				"config_hash":     sync.Snapshot.GetConfigHash(),
			})
			return
		}
		panelSuccess(c, gin.H{
			"message":      "同步操作已由 AnixOps Agent 接收",
			"transport":    "agent-control-grpc",
			"operation_id": sync.Desired.OperationId,
			"revision":     sync.Desired.Revision,
			"ack":          sync.Ack,
		})
		return
	}

	panelSuccess(c, gin.H{
		"message":   "节点未连接 Agent Control，将保留旧版周期拉取同步",
		"transport": "legacy-poll",
	})
}
