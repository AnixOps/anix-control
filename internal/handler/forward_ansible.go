package handler

import (
	"encoding/json"
	"strconv"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/gin-gonic/gin"
)

type CreateAnsibleMachineRequest struct {
	Name   string `json:"name" binding:"required"`
	Host   string `json:"host" binding:"required"`
	Port   int    `json:"port" binding:"required,min=1,max=65535"`
	Region string `json:"region"`
	ISP    string `json:"isp"`
	Weight int    `json:"weight"`
}

type UpdateAnsibleMachineRequest struct {
	Name    string `json:"name" binding:"omitempty,min=1,max=255"`
	Host    string `json:"host" binding:"omitempty,min=1,max=255"`
	Port    int    `json:"port" binding:"omitempty,min=1,max=65535"`
	Region  string `json:"region" binding:"omitempty,max=128"`
	ISP     string `json:"isp" binding:"omitempty,max=128"`
	Weight  int    `json:"weight" binding:"omitempty,gte=0"`
	Enabled *bool  `json:"enabled"`
}

type ansibleMachineResponse struct {
	ID            uint      `json:"id"`
	Name          string    `json:"name"`
	Type          string    `json:"type"`
	Host          string    `json:"host"`
	Port          int       `json:"port"`
	Region        string    `json:"region"`
	ISP           string    `json:"isp"`
	Status        int       `json:"status"`
	LastCheck     time.Time `json:"last_check"`
	Latency       int       `json:"latency"`
	Uptime        float64   `json:"uptime"`
	Weight        int       `json:"weight"`
	Enabled       bool      `json:"enabled"`
	TotalUpload   int64     `json:"total_upload"`
	TotalDownload int64     `json:"total_download"`
	CurrentConn   int       `json:"current_conn"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func (h *ForwardHandler) ListAnsibleMachines(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	page, pageSize = ClampPagination(page, pageSize)
	var status *int
	if rawStatus := c.Query("status"); rawStatus != "" {
		parsedStatus, err := strconv.Atoi(rawStatus)
		if err != nil {
			panelError(c, "invalid status")
			return
		}
		status = &parsedStatus
	}

	nodes, total, err := h.nodeService.ListByInventoryScope(service.ForwardNodeInventoryScopeAnsible, model.ForwardNodeTypeRelay, status, page, pageSize)
	if err != nil {
		panelError(c, err.Error())
		return
	}

	for _, node := range nodes {
		if ensureErr := h.ensureAnsibleMachineTag(node); ensureErr == nil {
			node.APIPort = 0
			node.APIToken = ""
		}
	}

	list := make([]ansibleMachineResponse, 0, len(nodes))
	for _, node := range nodes {
		list = append(list, toAnsibleMachineResponse(node))
	}

	panelSuccess(c, gin.H{
		"list":      list,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

func (h *ForwardHandler) CreateAnsibleMachine(c *gin.Context) {
	var req CreateAnsibleMachineRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		panelError(c, err.Error())
		return
	}

	node := &model.ForwardNode{
		Name:     req.Name,
		Type:     model.ForwardNodeTypeRelay,
		Host:     req.Host,
		Port:     req.Port,
		Region:   req.Region,
		ISP:      req.ISP,
		Weight:   req.Weight,
		Enabled:  true,
		APIPort:  0,
		APIToken: "",
	}
	node.Tags = h.mergeForwardNodeTag(node.Tags, service.ForwardNodeInventoryTagAnsibleMachine)

	if err := h.nodeService.Create(node); err != nil {
		panelError(c, err.Error())
		return
	}

	panelSuccess(c, toAnsibleMachineResponse(node))
}

func (h *ForwardHandler) GetAnsibleMachine(c *gin.Context) {
	node, ok := h.loadAnsibleMachine(c)
	if !ok {
		return
	}
	panelSuccess(c, toAnsibleMachineResponse(node))
}

func (h *ForwardHandler) UpdateAnsibleMachine(c *gin.Context) {
	node, ok := h.loadAnsibleMachine(c)
	if !ok {
		return
	}

	var req UpdateAnsibleMachineRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		panelError(c, err.Error())
		return
	}

	if req.Name != "" {
		node.Name = req.Name
	}
	if req.Host != "" {
		node.Host = req.Host
	}
	if req.Port > 0 {
		node.Port = req.Port
	}
	if req.Region != "" {
		node.Region = req.Region
	}
	if req.ISP != "" {
		node.ISP = req.ISP
	}
	if req.Weight > 0 {
		node.Weight = req.Weight
	}
	if req.Enabled != nil {
		node.Enabled = *req.Enabled
	}

	node.Type = model.ForwardNodeTypeRelay
	node.APIPort = 0
	node.APIToken = ""
	node.Tags = h.mergeForwardNodeTag(node.Tags, service.ForwardNodeInventoryTagAnsibleMachine)

	if err := h.nodeService.Update(node); err != nil {
		panelError(c, err.Error())
		return
	}

	panelSuccess(c, toAnsibleMachineResponse(node))
}

func (h *ForwardHandler) DeleteAnsibleMachine(c *gin.Context) {
	node, ok := h.loadAnsibleMachine(c)
	if !ok {
		return
	}

	if err := h.nodeService.Delete(node.ID); err != nil {
		panelError(c, err.Error())
		return
	}

	panelSuccess(c, "deleted")
}

func (h *ForwardHandler) CheckAnsibleMachine(c *gin.Context) {
	node, ok := h.loadAnsibleMachine(c)
	if !ok {
		return
	}

	result, err := h.nodeService.HealthCheck(c.Request.Context(), node.ID)
	if err != nil {
		panelError(c, err.Error())
		return
	}

	panelSuccess(c, result)
}

func (h *ForwardHandler) ToggleAnsibleMachine(c *gin.Context) {
	node, ok := h.loadAnsibleMachine(c)
	if !ok {
		return
	}

	var req ToggleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		panelError(c, err.Error())
		return
	}

	if req.Enabled {
		node.Enabled = true
		node.Status = model.ForwardNodeStatusOnline
	} else {
		node.Enabled = false
		node.Status = model.ForwardNodeStatusOffline
	}

	node.Type = model.ForwardNodeTypeRelay
	node.APIPort = 0
	node.APIToken = ""
	node.Tags = h.mergeForwardNodeTag(node.Tags, service.ForwardNodeInventoryTagAnsibleMachine)

	if err := h.nodeService.Update(node); err != nil {
		panelError(c, err.Error())
		return
	}

	panelSuccess(c, "updated")
}

func (h *ForwardHandler) SyncAnsibleMachineStats(c *gin.Context) {
	node, ok := h.loadAnsibleMachine(c)
	if !ok {
		return
	}

	panelSuccess(c, gin.H{
		"message": "Ansible machines do not expose gost management API stats; keeping panel-side counters",
		"stats": gin.H{
			"current_conn":   node.CurrentConn,
			"total_upload":   node.TotalUpload,
			"total_download": node.TotalDownload,
		},
	})
}

func (h *ForwardHandler) loadAnsibleMachine(c *gin.Context) (*model.ForwardNode, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		panelError(c, "invalid id")
		return nil, false
	}

	node, err := h.nodeService.GetByIDForInventoryScope(uint(id), service.ForwardNodeInventoryScopeAnsible)
	if err != nil {
		panelError(c, "ansible machine not found")
		return nil, false
	}

	if ensureErr := h.ensureAnsibleMachineTag(node); ensureErr == nil {
		node.APIPort = 0
		node.APIToken = ""
	}

	return node, true
}

func (h *ForwardHandler) ensureAnsibleMachineTag(node *model.ForwardNode) error {
	if node == nil {
		return nil
	}
	merged := h.mergeForwardNodeTag(node.Tags, service.ForwardNodeInventoryTagAnsibleMachine)
	if merged == node.Tags {
		return nil
	}
	node.Tags = merged
	node.APIPort = 0
	node.APIToken = ""
	return h.nodeService.Update(node)
}

func (h *ForwardHandler) mergeForwardNodeTag(rawTags, extraTag string) string {
	tags := h.nodeService.ParseTags(rawTags)
	for _, tag := range tags {
		if tag == extraTag {
			encoded, err := json.Marshal(tags)
			if err != nil {
				return rawTags
			}
			return string(encoded)
		}
	}
	tags = append(tags, extraTag)
	encoded, err := json.Marshal(tags)
	if err != nil {
		return rawTags
	}
	return string(encoded)
}

func toAnsibleMachineResponse(node *model.ForwardNode) ansibleMachineResponse {
	return ansibleMachineResponse{
		ID:            node.ID,
		Name:          node.Name,
		Type:          node.Type,
		Host:          node.Host,
		Port:          node.Port,
		Region:        node.Region,
		ISP:           node.ISP,
		Status:        node.Status,
		LastCheck:     node.LastCheck,
		Latency:       node.Latency,
		Uptime:        node.Uptime,
		Weight:        node.Weight,
		Enabled:       node.Enabled,
		TotalUpload:   node.TotalUpload,
		TotalDownload: node.TotalDownload,
		CurrentConn:   node.CurrentConn,
		CreatedAt:     node.CreatedAt,
		UpdatedAt:     node.UpdatedAt,
	}
}
