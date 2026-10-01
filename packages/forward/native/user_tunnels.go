package native

import (
	"context"
	"errors"
	"fmt"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/gin-gonic/gin/binding"
	"gorm.io/gorm"
)

// userTunnelInput is the legacy PanelUserTunnelInput.
type userTunnelInput struct {
	UserID        uint  `json:"userId"`
	TunnelID      uint  `json:"tunnelId"`
	Flow          int64 `json:"flow"`
	Num           int   `json:"num"`
	FlowResetTime int64 `json:"flowResetTime"`
	ExpTime       int64 `json:"expTime"`
	SpeedID       *uint `json:"speedId"`
}

// AssignUserTunnel is POST /api/v2/tunnel/user/assign and its
// administrator route: it lets a user create forwards on a tunnel. A
// permission alone runs nothing on a node, so nothing is pushed; whether
// the user exists is read from kapi_user_directory_v1.
func (s *Service) AssignUserTunnel(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	var input userTunnelInput
	if err := binding.JSON.BindBody(request.Body, &input); err != nil {
		return s.panelError("参数错误")
	}
	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError(err.Error())
	}
	if err := assignUserTunnel(db, input); err != nil {
		return s.panelError(err.Error())
	}
	return s.panel("用户隧道权限分配成功")
}

func assignUserTunnel(db *gorm.DB, input userTunnelInput) error {
	if input.UserID == 0 || input.TunnelID == 0 {
		return errors.New("userId and tunnelId are required")
	}
	var user DirectoryUser
	if err := db.First(&user, input.UserID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("user not found")
		}
		return err
	}
	var tunnel Tunnel
	if err := db.First(&tunnel, input.TunnelID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("tunnel not found")
		}
		return err
	}
	var count int64
	if err := db.Model(&UserTunnel{}).Where("user_id = ? AND tunnel_id = ?", input.UserID, input.TunnelID).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return errors.New("user tunnel permission already exists")
	}
	speedLimit, err := tunnelSpeedLimit(db, input.TunnelID, input.SpeedID)
	if err != nil {
		return err
	}
	var speedID *uint
	if speedLimit != nil {
		speedID = &speedLimit.ID
	}
	return db.Create(&UserTunnel{
		UserID: input.UserID, TunnelID: input.TunnelID, Flow: input.Flow, Num: input.Num, FlowResetTime: input.FlowResetTime,
		ExpTime: input.ExpTime, SpeedID: speedID, Status: userTunnelActive,
	}).Error
}

// tunnelSpeedLimit is the speed limit a permission names, which must
// belong to the permission's tunnel.
func tunnelSpeedLimit(db *gorm.DB, tunnelID uint, speedID *uint) (*SpeedLimit, error) {
	if speedID == nil || *speedID == 0 {
		return nil, nil
	}
	var speedLimit SpeedLimit
	if err := db.First(&speedLimit, *speedID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("speed limit not found")
		}
		return nil, err
	}
	if speedLimit.TunnelID != tunnelID {
		return nil, fmt.Errorf("speed limit %d does not belong to tunnel %d", speedLimit.ID, tunnelID)
	}
	return &speedLimit, nil
}

// userTunnelItem is the legacy PanelUserTunnelDetailItem.
type userTunnelItem struct {
	ID             uint   `json:"id"`
	UserID         uint   `json:"userId"`
	TunnelID       uint   `json:"tunnelId"`
	Flow           int64  `json:"flow"`
	Num            int    `json:"num"`
	FlowResetTime  int64  `json:"flowResetTime"`
	ExpTime        int64  `json:"expTime"`
	SpeedID        *uint  `json:"speedId"`
	SpeedLimitName string `json:"speedLimitName"`
	Speed          int64  `json:"speed"`
	TunnelName     string `json:"tunnelName"`
	TunnelFlow     int    `json:"tunnelFlow"`
	InFlow         int64  `json:"inFlow"`
	OutFlow        int64  `json:"outFlow"`
	Status         int    `json:"status"`
}

// ListUserTunnels is POST /api/v2/tunnel/user/list and its administrator
// route: a user's tunnel permissions. As in the kernel, a permission's
// traffic catches up with its forwards' totals first. The speed is the
// permission's speed limit, else the subscriber's own
// (kapi_subscriber_entitlement_v1).
func (s *Service) ListUserTunnels(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	var input struct {
		UserID uint `json:"userId"`
	}
	if err := binding.JSON.BindBody(request.Body, &input); err != nil {
		return s.panelError("参数错误")
	}
	if input.UserID == 0 {
		return s.panelError("userId is required")
	}
	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError(err.Error())
	}
	items, err := listUserTunnels(db, input.UserID)
	if err != nil {
		return s.panelError(err.Error())
	}
	return s.panel(items)
}

func listUserTunnels(db *gorm.DB, userID uint) ([]userTunnelItem, error) {
	var records []UserTunnel
	if err := db.Preload("Tunnel").Where("user_id = ?", userID).Order("id ASC").Find(&records).Error; err != nil {
		return nil, err
	}
	var entitlements []Entitlement
	if len(records) > 0 {
		if err := db.Where("id = ?", userID).Find(&entitlements).Error; err != nil {
			return nil, err
		}
	}
	userSpeed := int64(0)
	if len(entitlements) > 0 && entitlements[0].SpeedLimit != nil && *entitlements[0].SpeedLimit > 0 {
		userSpeed = *entitlements[0].SpeedLimit
	}
	speedLimits, err := speedLimitsByID(db, records)
	if err != nil {
		return nil, err
	}

	items := make([]userTunnelItem, 0, len(records))
	for i := range records {
		record := &records[i]
		if err := syncUserTunnelTraffic(db, record); err != nil {
			return nil, err
		}
		speedLimitName, speed := "", int64(0)
		if record.SpeedID != nil {
			if speedLimit, ok := speedLimits[*record.SpeedID]; ok {
				speedLimitName, speed = speedLimit.Name, speedLimit.Speed
			}
		}
		if speed == 0 && len(entitlements) > 0 {
			speed = userSpeed
		}
		tunnelName, tunnelFlow := "", 0
		if record.Tunnel != nil {
			tunnelName, tunnelFlow = record.Tunnel.Name, record.Tunnel.Flow
		}
		items = append(items, userTunnelItem{
			ID: record.ID, UserID: record.UserID, TunnelID: record.TunnelID, Flow: record.Flow, Num: record.Num,
			FlowResetTime: record.FlowResetTime, ExpTime: record.ExpTime, SpeedID: record.SpeedID,
			SpeedLimitName: speedLimitName, Speed: speed, TunnelName: tunnelName, TunnelFlow: tunnelFlow,
			InFlow: record.InFlow, OutFlow: record.OutFlow, Status: record.Status,
		})
	}
	return items, nil
}

func speedLimitsByID(db *gorm.DB, records []UserTunnel) (map[uint]SpeedLimit, error) {
	result := make(map[uint]SpeedLimit)
	ids := make([]uint, 0, len(records))
	seen := make(map[uint]struct{}, len(records))
	for _, record := range records {
		if record.SpeedID == nil || *record.SpeedID == 0 {
			continue
		}
		if _, ok := seen[*record.SpeedID]; ok {
			continue
		}
		seen[*record.SpeedID] = struct{}{}
		ids = append(ids, *record.SpeedID)
	}
	if len(ids) == 0 {
		return result, nil
	}
	var speedLimits []SpeedLimit
	if err := db.Where("id IN ?", ids).Find(&speedLimits).Error; err != nil {
		return nil, err
	}
	for _, speedLimit := range speedLimits {
		result[speedLimit.ID] = speedLimit
	}
	return result, nil
}

// syncUserTunnelTraffic raises a permission's traffic to its forwards'
// totals when they are higher, as the kernel's
// syncUserTunnelTrafficSnapshot does.
func syncUserTunnelTraffic(db *gorm.DB, permission *UserTunnel) error {
	if permission.ID == 0 {
		return nil
	}
	var traffic struct {
		InFlow  int64 `gorm:"column:in_flow"`
		OutFlow int64 `gorm:"column:out_flow"`
	}
	if err := db.Model(&Forward{}).
		Select("COALESCE(SUM(in_flow), 0) AS in_flow, COALESCE(SUM(out_flow), 0) AS out_flow").
		Where("user_id = ? AND tunnel_id = ?", permission.UserID, permission.TunnelID).
		Scan(&traffic).Error; err != nil {
		return err
	}
	nextInFlow, nextOutFlow := max(permission.InFlow, traffic.InFlow), max(permission.OutFlow, traffic.OutFlow)
	if nextInFlow == permission.InFlow && nextOutFlow == permission.OutFlow {
		return nil
	}
	if err := db.Model(&UserTunnel{}).Where("id = ?", permission.ID).
		Updates(map[string]any{"in_flow": nextInFlow, "out_flow": nextOutFlow}).Error; err != nil {
		return err
	}
	permission.InFlow, permission.OutFlow = nextInFlow, nextOutFlow
	return nil
}
