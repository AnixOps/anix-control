package native

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/gin-gonic/gin/binding"
)

// storageUnavailable answers a request when the package's storage cannot be
// opened; the kernel's handlers have no such step, and the lease error is not
// shown.
const storageUnavailable = "load balancer storage unavailable"

// LoadBalancer is a v2_load_balancer row. Its column defaults are the
// kernel model's: GORM writes them for zero values on create, so a new load
// balancer always starts with health checks on and enabled.
type LoadBalancer struct {
	ID            uint   `gorm:"primaryKey"`
	Name          string `gorm:"size:100"`
	GroupID       uint   `gorm:"index"`
	Strategy      string `gorm:"size:20;default:round-robin"`
	HealthCheck   bool   `gorm:"default:true"`
	CheckInterval int    `gorm:"default:60"`
	CheckTimeout  int    `gorm:"default:10"`
	NodeWeights   string `gorm:"type:text"`
	Enabled       bool   `gorm:"default:true"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// TableName is the adopted kernel table.
func (LoadBalancer) TableName() string { return "v2_load_balancer" }

// loadBalancerResponse is the kernel's answer for one load balancer: the
// stored weights twice, as text and decoded (an empty object when absent or
// not JSON). The group name was never resolved.
func loadBalancerResponse(lb *LoadBalancer) map[string]any {
	weights := any(map[string]any{})
	if lb.NodeWeights != "" {
		var decoded any
		if err := json.Unmarshal([]byte(lb.NodeWeights), &decoded); err == nil {
			weights = decoded
		}
	}
	return map[string]any{
		"id":             lb.ID,
		"name":           lb.Name,
		"group_id":       lb.GroupID,
		"group_name":     "",
		"strategy":       lb.Strategy,
		"health_check":   lb.HealthCheck,
		"check_interval": lb.CheckInterval,
		"check_timeout":  lb.CheckTimeout,
		"enabled":        lb.Enabled,
		"node_weights":   lb.NodeWeights,
		"weights":        weights,
		"created_at":     lb.CreatedAt,
		"updated_at":     lb.UpdatedAt,
	}
}

// loadBalancerRequest is the kernel's request body, name included: binding
// errors name the type ("loadBalancerRequest.Name") and are returned as is.
type loadBalancerRequest struct {
	Name          string          `json:"name" binding:"omitempty,min=1,max=255"`
	GroupID       uint            `json:"group_id" binding:"omitempty,gt=0"`
	Strategy      string          `json:"strategy" binding:"omitempty,oneof=round-robin least-connections least-load weighted-random weight latency"`
	HealthCheck   *bool           `json:"health_check"`
	CheckInterval int             `json:"check_interval" binding:"omitempty,gt=0"`
	CheckTimeout  int             `json:"check_timeout" binding:"omitempty,gt=0"`
	Enabled       *bool           `json:"enabled"`
	NodeWeights   string          `json:"node_weights"`
	Weights       json.RawMessage `json:"weights"`
}

// apply sets what the request names; zero values leave a field as it is.
func (req *loadBalancerRequest) apply(lb *LoadBalancer) {
	if req.Name != "" {
		lb.Name = req.Name
	}
	if req.GroupID > 0 {
		lb.GroupID = req.GroupID
	}
	if req.Strategy != "" {
		lb.Strategy = req.Strategy
	}
	if req.HealthCheck != nil {
		lb.HealthCheck = *req.HealthCheck
	}
	if req.CheckInterval > 0 {
		lb.CheckInterval = req.CheckInterval
	}
	if req.CheckTimeout > 0 {
		lb.CheckTimeout = req.CheckTimeout
	}
	if req.Enabled != nil {
		lb.Enabled = *req.Enabled
	}
	if len(req.Weights) > 0 && string(req.Weights) != "null" {
		lb.NodeWeights = string(req.Weights)
	}
	if req.NodeWeights != "" {
		lb.NodeWeights = req.NodeWeights
	}
}

// ListLoadBalancers is GET /api/v2/admin/loadbalancers.
func (s *Service) ListLoadBalancers(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	groupID, _ := strconv.Atoi(query(request, "group_id"))
	if groupID < 0 {
		groupID = 0
	}
	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError(storageUnavailable)
	}
	var lbs []LoadBalancer
	tx := db.Model(&LoadBalancer{})
	if groupID > 0 {
		tx = tx.Where("group_id = ?", groupID)
	}
	if err := tx.Order("created_at DESC").Find(&lbs).Error; err != nil {
		return s.panelError(err.Error())
	}
	list := make([]map[string]any, 0, len(lbs))
	for i := range lbs {
		list = append(list, loadBalancerResponse(&lbs[i]))
	}
	return s.panel(map[string]any{"list": list, "total": len(list)})
}

// CreateLoadBalancer is POST /api/v2/admin/loadbalancers.
func (s *Service) CreateLoadBalancer(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	var req loadBalancerRequest
	if err := binding.JSON.BindBody(request.Body, &req); err != nil {
		return s.panelError(err.Error())
	}
	lb := LoadBalancer{HealthCheck: true, Enabled: true, Strategy: "round-robin"}
	req.apply(&lb)
	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError(storageUnavailable)
	}
	if err := db.Create(&lb).Error; err != nil {
		return s.panelError(err.Error())
	}
	return s.panel(loadBalancerResponse(&lb))
}

// find loads a load balancer, as the kernel's GetByID.
func (s *Service) find(ctx context.Context, id uint) (*LoadBalancer, error) {
	db, err := s.Open(ctx)
	if err != nil {
		return nil, err
	}
	var lb LoadBalancer
	if err := db.First(&lb, id).Error; err != nil {
		return nil, err
	}
	return &lb, nil
}

// GetLoadBalancer is GET /api/v2/admin/loadbalancers/:id.
func (s *Service) GetLoadBalancer(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	id, ok := pathID(request)
	if !ok {
		return s.panelError("invalid id")
	}
	lb, err := s.find(ctx, id)
	if err != nil {
		return s.panelError("load balancer not found")
	}
	return s.panel(loadBalancerResponse(lb))
}

// UpdateLoadBalancer is PUT /api/v2/admin/loadbalancers/:id: the request
// applied to the stored row, which is then saved whole.
func (s *Service) UpdateLoadBalancer(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	id, ok := pathID(request)
	if !ok {
		return s.panelError("invalid id")
	}
	lb, err := s.find(ctx, id)
	if err != nil {
		return s.panelError("load balancer not found")
	}
	var req loadBalancerRequest
	if err := binding.JSON.BindBody(request.Body, &req); err != nil {
		return s.panelError(err.Error())
	}
	req.apply(lb)
	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError(storageUnavailable)
	}
	if err := db.Save(lb).Error; err != nil {
		return s.panelError(err.Error())
	}
	return s.panel(loadBalancerResponse(lb))
}

// DeleteLoadBalancer is DELETE /api/v2/admin/loadbalancers/:id. Deleting a
// load balancer that does not exist succeeds, as in the kernel.
func (s *Service) DeleteLoadBalancer(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	id, ok := pathID(request)
	if !ok {
		return s.panelError("invalid id")
	}
	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError(storageUnavailable)
	}
	if err := db.Delete(&LoadBalancer{}, id).Error; err != nil {
		return s.panelError(err.Error())
	}
	return s.panel(map[string]any{"message": "deleted"})
}
