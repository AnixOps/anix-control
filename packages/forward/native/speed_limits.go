package native

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/gin-gonic/gin/binding"
	"gorm.io/gorm"
)

// Route ids of the speed limit routes with a native handler. The update
// stays bridged: it re-applies the forwards of every permission that names
// the limit on their nodes.
const (
	SpeedLimitCreateRouteID  = "forward.speed_limit.create.post"
	SpeedLimitListRouteID    = "forward.speed_limit.list.post"
	SpeedLimitDeleteRouteID  = "forward.speed_limit.delete.post"
	SpeedLimitTunnelsRouteID = "forward.speed_limit.tunnels.post"
)

// speedLimitActive is a new speed limit's status, the kernel's
// speedLimitStatusActive.
const speedLimitActive = 1

// speedLimitInput is the kernel's SpeedLimitInput.
type speedLimitInput struct {
	Name       string `json:"name"`
	Speed      int64  `json:"speed"`
	TunnelID   uint   `json:"tunnelId"`
	TunnelName string `json:"tunnelName"`
}

// CreateSpeedLimit is POST /api/v2/speed-limit/create: a named speed limit
// for a tunnel, which no permission uses yet, so nothing runs on a node.
func (s *Service) CreateSpeedLimit(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	var input speedLimitInput
	if err := binding.JSON.BindBody(request.Body, &input); err != nil {
		return s.panelError("参数错误")
	}
	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError(err.Error())
	}
	tunnel, err := speedLimitTunnel(db, input.TunnelID, input.TunnelName)
	if err != nil {
		return s.panelError(err.Error())
	}
	if err := validateSpeedLimitInput(input.Name, input.Speed); err != nil {
		return s.panelError(err.Error())
	}
	now := s.now().UnixMilli()
	record := &SpeedLimit{
		Name: strings.TrimSpace(input.Name), Speed: input.Speed, TunnelID: tunnel.ID, TunnelName: tunnel.Name,
		Status: speedLimitActive, CreatedTime: now, UpdatedTime: now,
	}
	if err := db.Create(record).Error; err != nil {
		return s.panelError(err.Error())
	}
	return s.panel(nil)
}

// ListSpeedLimits is POST /api/v2/speed-limit/list: every speed limit by
// id.
func (s *Service) ListSpeedLimits(ctx context.Context, _ pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError(err.Error())
	}
	var records []SpeedLimit
	if err := db.Order("id ASC").Find(&records).Error; err != nil {
		return s.panelError(err.Error())
	}
	return s.panel(records)
}

// DeleteSpeedLimit is POST /api/v2/speed-limit/delete: a speed limit no
// permission uses, so no forward runs with it.
func (s *Service) DeleteSpeedLimit(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	var req struct {
		ID uint `json:"id" binding:"required"`
	}
	if err := binding.JSON.BindBody(request.Body, &req); err != nil {
		return s.panelError("参数错误")
	}
	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError(err.Error())
	}
	var record SpeedLimit
	if err := db.First(&record, req.ID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return s.panelError("speed limit not found")
		}
		return s.panelError(err.Error())
	}
	var assigned int64
	if err := db.Model(&UserTunnel{}).Where("speed_id = ?", req.ID).Count(&assigned).Error; err != nil {
		return s.panelError(err.Error())
	}
	if assigned > 0 {
		return s.panelError("speed limit is still assigned to user tunnels")
	}
	if err := db.Delete(&SpeedLimit{}, req.ID).Error; err != nil {
		return s.panelError(err.Error())
	}
	return s.panel("限速规则删除成功")
}

// SpeedLimitTunnels is POST /api/v2/speed-limit/tunnels: the tunnels a
// speed limit may name, every active one as for an administrator, as far
// as they suit the configured runtime backend.
func (s *Service) SpeedLimitTunnels(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	userID, _ := actor(request)
	return s.listTunnels(ctx, userID, true)
}

// speedLimitTunnel is the kernel's SpeedLimitService.validateTunnel: the
// tunnel by id, whose name must match.
func speedLimitTunnel(db *gorm.DB, tunnelID uint, tunnelName string) (*Tunnel, error) {
	if tunnelID == 0 {
		return nil, errors.New("tunnelId is required")
	}
	trimmedName := strings.TrimSpace(tunnelName)
	if trimmedName == "" {
		return nil, errors.New("tunnelName is required")
	}
	var tunnel Tunnel
	if err := db.First(&tunnel, tunnelID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("tunnel not found")
		}
		return nil, err
	}
	if tunnel.Name != trimmedName {
		return nil, fmt.Errorf("tunnelName does not match tunnelId %d", tunnelID)
	}
	return &tunnel, nil
}

// validateSpeedLimitInput is the kernel's.
func validateSpeedLimitInput(name string, speed int64) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("name is required")
	}
	if speed <= 0 {
		return errors.New("speed must be greater than 0")
	}
	return nil
}
