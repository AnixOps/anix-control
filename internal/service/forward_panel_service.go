package service

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"
	"time"

	"github.com/anixops/v2board/internal/model"
	"gorm.io/gorm"
)

const (
	defaultTunnelPortStart = 10000
	defaultTunnelPortEnd   = 60000
	diagnosisTimeout       = 3 * time.Second
	defaultRuntimeJobLimit = 50
	maxRuntimeJobLimit     = 200
	bytesPerGiB            = 1073741824
)

type PanelForwardService struct {
	db             *gorm.DB
	runtimeService *PanelForwardRuntimeService
}

type PanelRuntimeJobFilter struct {
	Backend   string
	Status    *int
	ForwardID *uint
	Limit     int
}

type panelForwardPermissionOptions struct {
	ExcludeForwardID        uint
	CheckUserTraffic        bool
	CheckTunnelTraffic      bool
	CheckTunnelForwardQuota bool
}

func NewPanelForwardService(db *gorm.DB) *PanelForwardService {
	return &PanelForwardService{
		db:             db,
		runtimeService: NewPanelForwardRuntimeService(db),
	}
}

type PanelForwardInput struct {
	Name          string `json:"name"`
	TunnelID      uint   `json:"tunnelId"`
	InPort        *int   `json:"inPort"`
	RemoteAddr    string `json:"remoteAddr"`
	InterfaceName string `json:"interfaceName"`
	Strategy      string `json:"strategy"`
}

type PanelForwardUpdateInput struct {
	ID            uint   `json:"id"`
	UserID        uint   `json:"userId"`
	Name          string `json:"name"`
	TunnelID      uint   `json:"tunnelId"`
	InPort        *int   `json:"inPort"`
	RemoteAddr    string `json:"remoteAddr"`
	InterfaceName string `json:"interfaceName"`
	Strategy      string `json:"strategy"`
}

type PanelForwardOrderUpdate struct {
	ID  uint `json:"id"`
	Inx int  `json:"inx"`
}

type PanelForwardListItem struct {
	ID                  uint   `json:"id"`
	Name                string `json:"name"`
	TunnelID            uint   `json:"tunnelId"`
	TunnelName          string `json:"tunnelName"`
	InIP                string `json:"inIp"`
	InPort              int    `json:"inPort"`
	RemoteAddr          string `json:"remoteAddr"`
	InterfaceName       string `json:"interfaceName"`
	Strategy            string `json:"strategy"`
	Status              int    `json:"status"`
	InFlow              int64  `json:"inFlow"`
	OutFlow             int64  `json:"outFlow"`
	RuntimeBackend      string `json:"runtimeBackend"`
	RuntimeStatus       int    `json:"runtimeStatus"`
	RuntimeMessage      string `json:"runtimeMessage"`
	LastRuntimeSyncTime int64  `json:"lastRuntimeSyncTime"`
	CreatedTime         int64  `json:"createdTime"`
	UpdatedTime         int64  `json:"updatedTime"`
	UserName            string `json:"userName"`
	UserID              uint   `json:"userId"`
	Inx                 int    `json:"inx"`
}

type PanelTunnelListItem struct {
	ID            uint   `json:"id"`
	Name          string `json:"name"`
	IP            string `json:"ip"`
	InIP          string `json:"inIp"`
	InNodePortSta *int   `json:"inNodePortSta"`
	InNodePortEnd *int   `json:"inNodePortEnd"`
	Type          int    `json:"type"`
	Protocol      string `json:"protocol"`
	Status        int    `json:"status"`
}

type PanelTunnelInput struct {
	Name          string   `json:"name"`
	InNodeID      uint     `json:"inNodeId"`
	OutNodeID     *uint    `json:"outNodeId"`
	Type          int      `json:"type"`
	Flow          int      `json:"flow"`
	TrafficRatio  *float64 `json:"trafficRatio"`
	InterfaceName string   `json:"interfaceName"`
	Protocol      string   `json:"protocol"`
	TCPListenAddr string   `json:"tcpListenAddr"`
	UDPListenAddr string   `json:"udpListenAddr"`
}

type PanelTunnelUpdateInput struct {
	ID            uint     `json:"id"`
	Name          string   `json:"name"`
	Flow          int      `json:"flow"`
	TrafficRatio  *float64 `json:"trafficRatio"`
	InterfaceName string   `json:"interfaceName"`
	Protocol      string   `json:"protocol"`
	TCPListenAddr string   `json:"tcpListenAddr"`
	UDPListenAddr string   `json:"udpListenAddr"`
}

type PanelAdminTunnelItem struct {
	ID            uint    `json:"id"`
	Name          string  `json:"name"`
	InNodeID      uint    `json:"inNodeId"`
	OutNodeID     *uint   `json:"outNodeId"`
	Type          int     `json:"type"`
	Flow          int     `json:"flow"`
	TrafficRatio  float64 `json:"trafficRatio"`
	InterfaceName string  `json:"interfaceName"`
	Protocol      string  `json:"protocol"`
	TCPListenAddr string  `json:"tcpListenAddr"`
	UDPListenAddr string  `json:"udpListenAddr"`
	InIP          string  `json:"inIp"`
	OutIP         string  `json:"outIp"`
	Status        int     `json:"status"`
}

type PanelUserTunnelInput struct {
	UserID        uint  `json:"userId"`
	TunnelID      uint  `json:"tunnelId"`
	Flow          int64 `json:"flow"`
	Num           int   `json:"num"`
	FlowResetTime int64 `json:"flowResetTime"`
	ExpTime       int64 `json:"expTime"`
	SpeedID       *uint `json:"speedId"`
}

type PanelUserTunnelQueryInput struct {
	UserID uint `json:"userId"`
}

type PanelUserTunnelUpdateInput struct {
	ID            uint  `json:"id"`
	Flow          int64 `json:"flow"`
	Num           int   `json:"num"`
	FlowResetTime int64 `json:"flowResetTime"`
	ExpTime       int64 `json:"expTime"`
	Status        int   `json:"status"`
	SpeedID       *uint `json:"speedId"`
}

type PanelUserTunnelDetailItem struct {
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

type TunnelDiagnosisReport struct {
	TunnelName string             `json:"tunnelName"`
	Timestamp  int64              `json:"timestamp"`
	Results    []DiagnosisOutcome `json:"results"`
}

type DiagnosisReport struct {
	ForwardName string             `json:"forwardName"`
	Timestamp   int64              `json:"timestamp"`
	Results     []DiagnosisOutcome `json:"results"`
}

type DiagnosisOutcome struct {
	Success     bool    `json:"success"`
	Description string  `json:"description"`
	NodeName    string  `json:"nodeName"`
	NodeID      string  `json:"nodeId"`
	TargetIP    string  `json:"targetIp"`
	TargetPort  int     `json:"targetPort,omitempty"`
	Message     string  `json:"message,omitempty"`
	AverageTime float64 `json:"averageTime,omitempty"`
	PacketLoss  float64 `json:"packetLoss,omitempty"`
}

func (s *PanelForwardService) ListForwards(userID uint, isAdmin bool) ([]PanelForwardListItem, error) {
	var records []model.Forward
	query := s.db.Preload("Tunnel").Order("inx ASC").Order("created_at DESC")
	if !isAdmin {
		query = query.Where("user_id = ?", userID)
	}
	if err := query.Find(&records).Error; err != nil {
		return nil, err
	}

	items := make([]PanelForwardListItem, 0, len(records))
	for _, record := range records {
		items = append(items, buildPanelForwardItem(&record))
	}
	return items, nil
}

func (s *PanelForwardService) ListTunnels(userID uint, isAdmin bool) ([]PanelTunnelListItem, error) {
	tunnels, err := s.getAccessibleTunnels(userID, isAdmin)
	if err != nil {
		return nil, err
	}

	items := make([]PanelTunnelListItem, 0, len(tunnels))
	for _, tunnel := range tunnels {
		items = append(items, PanelTunnelListItem{
			ID:            tunnel.ID,
			Name:          tunnel.Name,
			IP:            tunnel.InIP,
			InIP:          tunnel.InIP,
			InNodePortSta: tunnel.InNodePortSta,
			InNodePortEnd: tunnel.InNodePortEnd,
			Type:          tunnel.Type,
			Protocol:      tunnel.Protocol,
			Status:        tunnel.Status,
		})
	}
	return items, nil
}

func (s *PanelForwardService) CreateForward(userID uint, isAdmin bool, input PanelForwardInput) (*PanelForwardListItem, error) {
	if err := validatePanelForwardInput(input); err != nil {
		return nil, err
	}

	var user model.User
	if err := s.db.First(&user, userID).Error; err != nil {
		return nil, errors.New("用户不存在")
	}

	tunnel, err := s.getActiveTunnel(input.TunnelID)
	if err != nil {
		return nil, err
	}
	if !isAdmin {
		if _, _, err := s.validateForwardPermission(userID, tunnel.ID, panelForwardPermissionOptions{
			CheckUserTraffic:        true,
			CheckTunnelTraffic:      true,
			CheckTunnelForwardQuota: true,
		}); err != nil {
			return nil, err
		}
	}

	inPort, err := s.resolvePort(tunnel, input.InPort, 0)
	if err != nil {
		return nil, err
	}

	nextInx, err := s.nextIndexForUser(userID)
	if err != nil {
		return nil, err
	}

	record := &model.Forward{
		UserID:        user.ID,
		UserName:      resolveForwardUserName(&user),
		Name:          strings.TrimSpace(input.Name),
		TunnelID:      tunnel.ID,
		InPort:        inPort,
		RemoteAddr:    normalizeRemoteAddr(input.RemoteAddr),
		InterfaceName: strings.TrimSpace(input.InterfaceName),
		Strategy:      normalizeStrategy(input.Strategy, input.RemoteAddr),
		Status:        model.ForwardStatusActive,
		Inx:           nextInx,
	}

	if err := s.db.Create(record).Error; err != nil {
		return nil, err
	}

	if err := s.db.Preload("Tunnel").First(record, record.ID).Error; err != nil {
		return nil, err
	}

	if runtimeErr := s.syncForwardRuntime(record, model.ForwardRuntimeJobActionCreate); runtimeErr != nil {
		record.Status = model.ForwardStatusError
		if saveErr := s.db.Save(record).Error; saveErr != nil {
			return nil, saveErr
		}
	}

	item := buildPanelForwardItem(record)
	return &item, nil
}

func (s *PanelForwardService) UpdateForward(userID uint, isAdmin bool, input PanelForwardUpdateInput) (*PanelForwardListItem, error) {
	if input.ID == 0 {
		return nil, errors.New("缺少转发 ID")
	}
	if err := validatePanelForwardInput(PanelForwardInput{
		Name:          input.Name,
		TunnelID:      input.TunnelID,
		InPort:        input.InPort,
		RemoteAddr:    input.RemoteAddr,
		InterfaceName: input.InterfaceName,
		Strategy:      input.Strategy,
	}); err != nil {
		return nil, err
	}

	record, err := s.getForwardForActor(input.ID, userID, isAdmin)
	if err != nil {
		return nil, err
	}

	tunnel, err := s.getActiveTunnel(input.TunnelID)
	if err != nil {
		return nil, err
	}
	if !isAdmin {
		if _, _, err := s.validateForwardPermission(userID, tunnel.ID, panelForwardPermissionOptions{
			ExcludeForwardID:        record.ID,
			CheckUserTraffic:        true,
			CheckTunnelTraffic:      true,
			CheckTunnelForwardQuota: true,
		}); err != nil {
			return nil, err
		}
	} else if record.UserID != userID && record.TunnelID != tunnel.ID {
		if _, _, err := s.validateForwardPermission(record.UserID, tunnel.ID, panelForwardPermissionOptions{
			ExcludeForwardID:        record.ID,
			CheckUserTraffic:        true,
			CheckTunnelTraffic:      true,
			CheckTunnelForwardQuota: true,
		}); err != nil {
			return nil, err
		}
	}

	inPort, err := s.resolvePort(tunnel, input.InPort, record.ID)
	if err != nil {
		return nil, err
	}

	record.Name = strings.TrimSpace(input.Name)
	record.TunnelID = tunnel.ID
	record.InPort = inPort
	record.RemoteAddr = normalizeRemoteAddr(input.RemoteAddr)
	record.InterfaceName = strings.TrimSpace(input.InterfaceName)
	record.Strategy = normalizeStrategy(input.Strategy, input.RemoteAddr)
	if record.Status == model.ForwardStatusError {
		record.Status = model.ForwardStatusActive
	}

	if err := s.db.Save(record).Error; err != nil {
		return nil, err
	}
	if err := s.db.Preload("Tunnel").First(record, record.ID).Error; err != nil {
		return nil, err
	}

	if record.Status == model.ForwardStatusActive {
		if runtimeErr := s.syncForwardRuntime(record, model.ForwardRuntimeJobActionUpdate); runtimeErr != nil {
			record.Status = model.ForwardStatusError
			if saveErr := s.db.Save(record).Error; saveErr != nil {
				return nil, saveErr
			}
		}
	}

	item := buildPanelForwardItem(record)
	return &item, nil
}

func (s *PanelForwardService) DeleteForward(userID uint, isAdmin bool, forwardID uint, force bool) error {
	record, err := s.getForwardForActor(forwardID, userID, isAdmin)
	if err != nil {
		return err
	}

	if !force && record.Status == model.ForwardStatusActive {
		return errors.New("转发服务正在运行，请先暂停或使用强制删除")
	}

	if runtimeErr := s.syncForwardRuntime(record, model.ForwardRuntimeJobActionDelete); runtimeErr != nil {
		return runtimeErr
	}

	return s.db.Delete(&model.Forward{}, record.ID).Error
}

func (s *PanelForwardService) SetForwardStatus(userID uint, isAdmin bool, forwardID uint, status int) error {
	record, err := s.getForwardForActor(forwardID, userID, isAdmin)
	if err != nil {
		return err
	}
	if !isAdmin && status == model.ForwardStatusActive {
		if _, _, err := s.validateForwardPermission(userID, record.TunnelID, panelForwardPermissionOptions{
			ExcludeForwardID:   record.ID,
			CheckUserTraffic:   true,
			CheckTunnelTraffic: true,
		}); err != nil {
			return err
		}
	}

	if status == model.ForwardStatusActive {
		record.Status = model.ForwardStatusActive
		if err := s.db.Save(record).Error; err != nil {
			return err
		}
		if runtimeErr := s.syncForwardRuntime(record, model.ForwardRuntimeJobActionResume); runtimeErr != nil {
			record.Status = model.ForwardStatusError
			if saveErr := s.db.Save(record).Error; saveErr != nil {
				return saveErr
			}
			return runtimeErr
		}
		return nil
	}

	if runtimeErr := s.syncForwardRuntime(record, model.ForwardRuntimeJobActionPause); runtimeErr != nil {
		return runtimeErr
	}

	record.Status = status
	return s.db.Save(record).Error
}

func (s *PanelForwardService) DiagnoseForward(userID uint, isAdmin bool, forwardID uint) (*DiagnosisReport, error) {
	record, err := s.getForwardForActor(forwardID, userID, isAdmin)
	if err != nil {
		return nil, err
	}
	if err := s.db.Preload("Tunnel").First(record, record.ID).Error; err != nil {
		return nil, err
	}

	targets := strings.Split(record.RemoteAddr, ",")
	results := make([]DiagnosisOutcome, 0, len(targets))
	for _, raw := range targets {
		target := strings.TrimSpace(raw)
		if target == "" {
			continue
		}

		host, port, err := splitTarget(target)
		if err != nil {
			results = append(results, DiagnosisOutcome{
				Success:     false,
				Description: "转发->目标",
				NodeName:    resolveTunnelName(record.Tunnel),
				NodeID:      fmt.Sprintf("%d", record.TunnelID),
				TargetIP:    target,
				Message:     err.Error(),
			})
			continue
		}

		start := time.Now()
		conn, dialErr := net.DialTimeout("tcp", target, diagnosisTimeout)
		elapsed := time.Since(start)
		if dialErr == nil {
			_ = conn.Close()
			results = append(results, DiagnosisOutcome{
				Success:     true,
				Description: "转发->目标",
				NodeName:    resolveTunnelName(record.Tunnel),
				NodeID:      fmt.Sprintf("%d", record.TunnelID),
				TargetIP:    host,
				TargetPort:  port,
				AverageTime: float64(elapsed.Milliseconds()),
				PacketLoss:  0,
			})
			continue
		}

		results = append(results, DiagnosisOutcome{
			Success:     false,
			Description: "转发->目标",
			NodeName:    resolveTunnelName(record.Tunnel),
			NodeID:      fmt.Sprintf("%d", record.TunnelID),
			TargetIP:    host,
			TargetPort:  port,
			Message:     dialErr.Error(),
		})
	}

	if len(results) == 0 {
		results = append(results, DiagnosisOutcome{
			Success:     false,
			Description: "转发->目标",
			NodeName:    resolveTunnelName(record.Tunnel),
			NodeID:      fmt.Sprintf("%d", record.TunnelID),
			TargetIP:    "-",
			Message:     "没有可诊断的目标地址",
		})
	}

	return &DiagnosisReport{
		ForwardName: record.Name,
		Timestamp:   time.Now().UnixMilli(),
		Results:     results,
	}, nil
}

func (s *PanelForwardService) UpdateOrder(userID uint, isAdmin bool, updates []PanelForwardOrderUpdate) error {
	if len(updates) == 0 {
		return errors.New("forwards 参数不能为空")
	}

	ids := make([]uint, 0, len(updates))
	for _, update := range updates {
		ids = append(ids, update.ID)
	}

	var records []model.Forward
	query := s.db.Where("id IN ?", ids)
	if !isAdmin {
		query = query.Where("user_id = ?", userID)
	}
	if err := query.Find(&records).Error; err != nil {
		return err
	}
	if len(records) != len(ids) {
		return errors.New("只能更新可访问的转发排序")
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		for _, update := range updates {
			if err := tx.Model(&model.Forward{}).Where("id = ?", update.ID).Update("inx", update.Inx).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *PanelForwardService) AssignUserTunnel(input PanelUserTunnelInput) error {
	if input.UserID == 0 || input.TunnelID == 0 {
		return errors.New("userId and tunnelId are required")
	}

	var user model.User
	if err := s.db.First(&user, input.UserID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("user not found")
		}
		return err
	}

	var tunnel model.ForwardTunnel
	if err := s.db.First(&tunnel, input.TunnelID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("tunnel not found")
		}
		return err
	}

	var count int64
	if err := s.db.Model(&model.ForwardUserTunnel{}).
		Where("user_id = ? AND tunnel_id = ?", input.UserID, input.TunnelID).
		Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return errors.New("user tunnel permission already exists")
	}

	speedLimit, err := s.resolveTunnelSpeedLimit(input.TunnelID, input.SpeedID)
	if err != nil {
		return err
	}

	var speedID *uint
	if speedLimit != nil {
		speedID = &speedLimit.ID
	}

	record := &model.ForwardUserTunnel{
		UserID:        input.UserID,
		TunnelID:      input.TunnelID,
		Flow:          input.Flow,
		Num:           input.Num,
		FlowResetTime: input.FlowResetTime,
		ExpTime:       input.ExpTime,
		SpeedID:       speedID,
		Status:        model.ForwardUserTunnelStatusActive,
	}
	return s.db.Create(record).Error
}

func (s *PanelForwardService) ListUserTunnels(query PanelUserTunnelQueryInput) ([]PanelUserTunnelDetailItem, error) {
	if query.UserID == 0 {
		return nil, errors.New("userId is required")
	}

	var records []model.ForwardUserTunnel
	if err := s.db.
		Preload("User").
		Preload("Tunnel").
		Where("user_id = ?", query.UserID).
		Order("id ASC").
		Find(&records).Error; err != nil {
		return nil, err
	}

	speedLimitByID, err := s.listSpeedLimitsByIDs(collectUserTunnelSpeedLimitIDs(records))
	if err != nil {
		return nil, err
	}

	items := make([]PanelUserTunnelDetailItem, 0, len(records))
	for i := range records {
		record := &records[i]
		if err := s.syncUserTunnelTrafficSnapshot(record); err != nil {
			return nil, err
		}

		speedLimitName := ""
		speed := int64(0)
		if record.SpeedID != nil {
			if speedLimit, ok := speedLimitByID[*record.SpeedID]; ok {
				speedLimitName = speedLimit.Name
				speed = speedLimit.Speed
			}
		}
		if speed == 0 && record.User != nil {
			speed = record.User.GetSpeedLimit()
		}

		tunnelName := ""
		tunnelFlow := 0
		if record.Tunnel != nil {
			tunnelName = record.Tunnel.Name
			tunnelFlow = record.Tunnel.Flow
		}

		items = append(items, PanelUserTunnelDetailItem{
			ID:             record.ID,
			UserID:         record.UserID,
			TunnelID:       record.TunnelID,
			Flow:           record.Flow,
			Num:            record.Num,
			FlowResetTime:  record.FlowResetTime,
			ExpTime:        record.ExpTime,
			SpeedID:        record.SpeedID,
			SpeedLimitName: speedLimitName,
			Speed:          speed,
			TunnelName:     tunnelName,
			TunnelFlow:     tunnelFlow,
			InFlow:         record.InFlow,
			OutFlow:        record.OutFlow,
			Status:         record.Status,
		})
	}

	return items, nil
}

func (s *PanelForwardService) RemoveUserTunnel(id uint) error {
	record, err := s.getUserTunnelByID(id)
	if err != nil {
		return err
	}

	forwards, err := s.listUserTunnelForwards(record.UserID, record.TunnelID, 0)
	if err != nil {
		return err
	}
	for i := range forwards {
		_ = s.syncForwardRuntime(&forwards[i], model.ForwardRuntimeJobActionDelete)
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ? AND tunnel_id = ?", record.UserID, record.TunnelID).
			Delete(&model.Forward{}).Error; err != nil {
			return err
		}
		return tx.Delete(&model.ForwardUserTunnel{}, record.ID).Error
	})
}

func (s *PanelForwardService) UpdateUserTunnel(input PanelUserTunnelUpdateInput) error {
	record, err := s.getUserTunnelByID(input.ID)
	if err != nil {
		return err
	}

	if input.Status != model.ForwardUserTunnelStatusActive && input.Status != model.ForwardUserTunnelStatusDisabled {
		return errors.New("status must be 0 or 1")
	}

	speedLimit, err := s.resolveTunnelSpeedLimit(record.TunnelID, input.SpeedID)
	if err != nil {
		return err
	}

	var speedID *uint
	if speedLimit != nil {
		speedID = &speedLimit.ID
	}

	record.Flow = input.Flow
	record.Num = input.Num
	record.FlowResetTime = input.FlowResetTime
	record.ExpTime = input.ExpTime
	record.Status = input.Status
	record.SpeedID = speedID
	if err := s.db.Save(record).Error; err != nil {
		return err
	}
	return s.reconcileUserTunnelForwards(record)
}

func (s *PanelForwardService) ResetUserTunnelTraffic(id uint) error {
	record, err := s.getUserTunnelByID(id)
	if err != nil {
		return err
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.Forward{}).
			Where("user_id = ? AND tunnel_id = ?", record.UserID, record.TunnelID).
			Updates(map[string]interface{}{
				"in_flow":  0,
				"out_flow": 0,
			}).Error; err != nil {
			return err
		}

		return tx.Model(&model.ForwardUserTunnel{}).
			Where("id = ?", record.ID).
			Updates(map[string]interface{}{
				"in_flow":  0,
				"out_flow": 0,
			}).Error
	})
}

func (s *PanelForwardService) getActiveTunnel(tunnelID uint) (*model.ForwardTunnel, error) {
	var tunnel model.ForwardTunnel
	if err := s.db.First(&tunnel, tunnelID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("隧道不存在")
		}
		return nil, err
	}
	if tunnel.Status != model.ForwardTunnelStatusActive {
		return nil, errors.New("隧道已禁用")
	}
	return &tunnel, nil
}

func (s *PanelForwardService) getAccessibleTunnel(tunnelID, userID uint, isAdmin bool) (*model.ForwardTunnel, error) {
	tunnel, err := s.getActiveTunnel(tunnelID)
	if err != nil {
		return nil, err
	}
	if isAdmin {
		return tunnel, nil
	}

	allowed, err := s.userHasTunnelAccess(userID, tunnelID)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, errors.New("鏃犳潈浣跨敤璇ラ毀閬?")
	}
	return tunnel, nil
}

func (s *PanelForwardService) validateForwardPermission(userID, tunnelID uint, opts panelForwardPermissionOptions) (*model.User, *model.ForwardUserTunnel, error) {
	user, err := s.getActiveUser(userID)
	if err != nil {
		return nil, nil, err
	}

	permission, err := s.getActiveUserTunnelPermission(userID, tunnelID)
	if err != nil {
		return nil, nil, err
	}

	if opts.CheckUserTraffic && !user.HasTraffic() {
		return nil, nil, errors.New("user total traffic exhausted")
	}

	if opts.CheckTunnelTraffic && permission.Flow > 0 {
		totalTraffic, err := s.sumUserTunnelTraffic(userID, tunnelID)
		if err != nil {
			return nil, nil, err
		}
		if totalTraffic >= permission.Flow*bytesPerGiB {
			return nil, nil, errors.New("tunnel traffic exhausted")
		}
	}

	if opts.CheckTunnelForwardQuota && permission.Num > 0 {
		forwardCount, err := s.countUserTunnelForwards(userID, tunnelID, opts.ExcludeForwardID)
		if err != nil {
			return nil, nil, err
		}
		if forwardCount >= int64(permission.Num) {
			return nil, nil, fmt.Errorf("tunnel forward quota exceeded: %d", permission.Num)
		}
	}

	return user, permission, nil
}

func (s *PanelForwardService) getActiveUser(userID uint) (*model.User, error) {
	var user model.User
	if err := s.db.First(&user, userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("user not found")
		}
		return nil, err
	}
	if !user.IsValid() {
		return nil, errors.New("invalid user status")
	}
	return &user, nil
}

func (s *PanelForwardService) getActiveUserTunnelPermission(userID, tunnelID uint) (*model.ForwardUserTunnel, error) {
	if userID == 0 || tunnelID == 0 {
		return nil, errors.New("no active tunnel permission")
	}

	var permission model.ForwardUserTunnel
	if err := s.db.Where("user_id = ? AND tunnel_id = ?", userID, tunnelID).First(&permission).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("no active tunnel permission")
		}
		return nil, err
	}
	if permission.Status != model.ForwardUserTunnelStatusActive {
		return nil, errors.New("tunnel permission is disabled")
	}
	if permission.ExpTime > 0 && permission.ExpTime <= time.Now().UnixMilli() {
		return nil, errors.New("tunnel permission expired")
	}
	return &permission, nil
}

func (s *PanelForwardService) getForwardForActor(forwardID, userID uint, isAdmin bool) (*model.Forward, error) {
	var record model.Forward
	query := s.db.Preload("Tunnel").Where("id = ?", forwardID)
	if !isAdmin {
		query = query.Where("user_id = ?", userID)
	}
	if err := query.First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("转发不存在")
		}
		return nil, err
	}
	return &record, nil
}

func (s *PanelForwardService) getUserTunnelByID(id uint) (*model.ForwardUserTunnel, error) {
	if id == 0 {
		return nil, errors.New("user tunnel id is required")
	}

	var record model.ForwardUserTunnel
	if err := s.db.First(&record, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("user tunnel permission not found")
		}
		return nil, err
	}
	return &record, nil
}

func (s *PanelForwardService) resolveTunnelSpeedLimit(tunnelID uint, speedID *uint) (*model.SpeedLimit, error) {
	if speedID == nil || *speedID == 0 {
		return nil, nil
	}

	var speedLimit model.SpeedLimit
	if err := s.db.First(&speedLimit, *speedID).Error; err != nil {
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

func collectUserTunnelSpeedLimitIDs(records []model.ForwardUserTunnel) []uint {
	if len(records) == 0 {
		return nil
	}

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
	return ids
}

func (s *PanelForwardService) listSpeedLimitsByIDs(ids []uint) (map[uint]model.SpeedLimit, error) {
	result := make(map[uint]model.SpeedLimit)
	if len(ids) == 0 {
		return result, nil
	}

	var records []model.SpeedLimit
	if err := s.db.Where("id IN ?", ids).Find(&records).Error; err != nil {
		return nil, err
	}
	for _, record := range records {
		result[record.ID] = record
	}
	return result, nil
}

func (s *PanelForwardService) resolvePort(tunnel *model.ForwardTunnel, requested *int, excludeForwardID uint) (int, error) {
	start, end := tunnelPortRange(tunnel)
	usedPorts, err := s.usedPortsForTunnel(tunnel.ID, excludeForwardID)
	if err != nil {
		return 0, err
	}

	if requested != nil && *requested > 0 {
		if *requested < start || *requested > end {
			return 0, fmt.Errorf("端口号必须在 %d-%d 范围内", start, end)
		}
		if usedPorts[*requested] {
			return 0, errors.New("入口端口已被占用")
		}
		return *requested, nil
	}

	for port := start; port <= end; port++ {
		if !usedPorts[port] {
			return port, nil
		}
	}
	return 0, errors.New("没有可用的入口端口")
}

func (s *PanelForwardService) usedPortsForTunnel(tunnelID, excludeForwardID uint) (map[int]bool, error) {
	var records []model.Forward
	query := s.db.Select("in_port").Where("tunnel_id = ?", tunnelID)
	if excludeForwardID > 0 {
		query = query.Where("id <> ?", excludeForwardID)
	}
	if err := query.Find(&records).Error; err != nil {
		return nil, err
	}

	used := make(map[int]bool, len(records))
	for _, record := range records {
		used[record.InPort] = true
	}
	return used, nil
}

func (s *PanelForwardService) nextIndexForUser(userID uint) (int, error) {
	var records []model.Forward
	if err := s.db.Select("inx").Where("user_id = ?", userID).Find(&records).Error; err != nil {
		return 0, err
	}
	if len(records) == 0 {
		return 0, nil
	}
	sort.Slice(records, func(i, j int) bool {
		return records[i].Inx < records[j].Inx
	})
	return records[len(records)-1].Inx + 1, nil
}

func (s *PanelForwardService) countUserTunnelForwards(userID, tunnelID, excludeForwardID uint) (int64, error) {
	query := s.db.Model(&model.Forward{}).Where("user_id = ? AND tunnel_id = ?", userID, tunnelID)
	if excludeForwardID != 0 {
		query = query.Where("id <> ?", excludeForwardID)
	}

	var count int64
	if err := query.Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (s *PanelForwardService) sumUserTunnelTraffic(userID, tunnelID uint) (int64, error) {
	var permission model.ForwardUserTunnel
	if err := s.db.Where("user_id = ? AND tunnel_id = ?", userID, tunnelID).First(&permission).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, nil
		}
		return 0, err
	}

	if err := s.syncUserTunnelTrafficSnapshot(&permission); err != nil {
		return 0, err
	}
	return permission.InFlow + permission.OutFlow, nil
}

func (s *PanelForwardService) listUserTunnelForwards(userID, tunnelID uint, status int) ([]model.Forward, error) {
	query := s.db.Where("user_id = ? AND tunnel_id = ?", userID, tunnelID).Order("id ASC")
	if status != 0 {
		query = query.Where("status = ?", status)
	}

	var forwards []model.Forward
	if err := query.Find(&forwards).Error; err != nil {
		return nil, err
	}
	return forwards, nil
}

func (s *PanelForwardService) reconcileUserTunnelForwards(permission *model.ForwardUserTunnel) error {
	if permission == nil {
		return nil
	}

	shouldPause, err := s.userTunnelRequiresPause(permission)
	if err != nil {
		return err
	}
	if !shouldPause {
		return nil
	}

	forwards, err := s.listUserTunnelForwards(permission.UserID, permission.TunnelID, model.ForwardStatusActive)
	if err != nil {
		return err
	}

	var firstErr error
	for i := range forwards {
		if err := s.pauseManagedForward(&forwards[i]); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (s *PanelForwardService) userTunnelRequiresPause(permission *model.ForwardUserTunnel) (bool, error) {
	if permission.Status != model.ForwardUserTunnelStatusActive {
		return true, nil
	}
	if permission.ExpTime > 0 && permission.ExpTime <= time.Now().UnixMilli() {
		return true, nil
	}
	if permission.Flow > 0 {
		totalTraffic, err := s.sumUserTunnelTraffic(permission.UserID, permission.TunnelID)
		if err != nil {
			return false, err
		}
		if totalTraffic >= permission.Flow*bytesPerGiB {
			return true, nil
		}
	}
	return false, nil
}

func (s *PanelForwardService) syncUserTunnelTrafficSnapshot(permission *model.ForwardUserTunnel) error {
	if permission == nil || permission.ID == 0 {
		return nil
	}

	var traffic struct {
		InFlow  int64 `gorm:"column:in_flow"`
		OutFlow int64 `gorm:"column:out_flow"`
	}
	if err := s.db.Model(&model.Forward{}).
		Select("COALESCE(SUM(in_flow), 0) AS in_flow, COALESCE(SUM(out_flow), 0) AS out_flow").
		Where("user_id = ? AND tunnel_id = ?", permission.UserID, permission.TunnelID).
		Scan(&traffic).Error; err != nil {
		return err
	}

	nextInFlow := permission.InFlow
	if traffic.InFlow > nextInFlow {
		nextInFlow = traffic.InFlow
	}
	nextOutFlow := permission.OutFlow
	if traffic.OutFlow > nextOutFlow {
		nextOutFlow = traffic.OutFlow
	}

	if nextInFlow == permission.InFlow && nextOutFlow == permission.OutFlow {
		return nil
	}

	if err := s.db.Model(&model.ForwardUserTunnel{}).
		Where("id = ?", permission.ID).
		Updates(map[string]interface{}{
			"in_flow":  nextInFlow,
			"out_flow": nextOutFlow,
		}).Error; err != nil {
		return err
	}

	permission.InFlow = nextInFlow
	permission.OutFlow = nextOutFlow
	return nil
}

func (s *PanelForwardService) pauseManagedForward(record *model.Forward) error {
	if record == nil || record.ID == 0 {
		return nil
	}

	if err := s.syncForwardRuntime(record, model.ForwardRuntimeJobActionPause); err != nil {
		record.Status = model.ForwardStatusError
		if saveErr := s.db.Model(&model.Forward{}).Where("id = ?", record.ID).Update("status", model.ForwardStatusError).Error; saveErr != nil {
			return saveErr
		}
		return err
	}

	record.Status = model.ForwardStatusPaused
	return s.db.Model(&model.Forward{}).Where("id = ?", record.ID).Update("status", model.ForwardStatusPaused).Error
}

func (s *PanelForwardService) getAccessibleTunnels(userID uint, isAdmin bool) ([]model.ForwardTunnel, error) {
	var tunnels []model.ForwardTunnel
	if isAdmin {
		if err := s.db.Where("status = ?", model.ForwardTunnelStatusActive).Order("name ASC").Find(&tunnels).Error; err != nil {
			return nil, err
		}
		return tunnels, nil
	}

	if userID == 0 {
		return []model.ForwardTunnel{}, nil
	}

	var permissions []model.ForwardUserTunnel
	if err := s.db.
		Where("user_id = ?", userID).
		Order("id ASC").
		Find(&permissions).Error; err != nil {
		return nil, err
	}
	if len(permissions) == 0 {
		return []model.ForwardTunnel{}, nil
	}

	tunnelIDs := make([]uint, 0, len(permissions))
	seen := make(map[uint]struct{}, len(permissions))
	for _, permission := range permissions {
		if _, ok := seen[permission.TunnelID]; ok {
			continue
		}
		seen[permission.TunnelID] = struct{}{}
		tunnelIDs = append(tunnelIDs, permission.TunnelID)
	}

	if err := s.db.
		Where("id IN ? AND status = ?", tunnelIDs, model.ForwardTunnelStatusActive).
		Order("name ASC").
		Find(&tunnels).Error; err != nil {
		return nil, err
	}
	return tunnels, nil
}

func (s *PanelForwardService) userHasTunnelAccess(userID, tunnelID uint) (bool, error) {
	if userID == 0 || tunnelID == 0 {
		return false, nil
	}

	var count int64
	now := time.Now().UnixMilli()
	if err := s.db.Model(&model.ForwardUserTunnel{}).
		Where("user_id = ? AND tunnel_id = ? AND status = ? AND (exp_time = 0 OR exp_time > ?)",
			userID, tunnelID, model.ForwardUserTunnelStatusActive, now).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *PanelForwardService) ListRuntimeJobs(filter PanelRuntimeJobFilter) ([]model.ForwardRuntimeJob, error) {
	query := s.db.Model(&model.ForwardRuntimeJob{}).Order("id DESC")
	if strings.TrimSpace(filter.Backend) != "" {
		query = query.Where("backend = ?", strings.TrimSpace(filter.Backend))
	}
	if filter.Status != nil {
		query = query.Where("status = ?", *filter.Status)
	}
	if filter.ForwardID != nil {
		query = query.Where("forward_id = ?", *filter.ForwardID)
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = defaultRuntimeJobLimit
	}
	if limit > maxRuntimeJobLimit {
		limit = maxRuntimeJobLimit
	}

	var jobs []model.ForwardRuntimeJob
	err := query.Limit(limit).Find(&jobs).Error
	return jobs, err
}

func buildPanelForwardItem(record *model.Forward) PanelForwardListItem {
	item := PanelForwardListItem{
		ID:             record.ID,
		Name:           record.Name,
		TunnelID:       record.TunnelID,
		InPort:         record.InPort,
		RemoteAddr:     record.RemoteAddr,
		InterfaceName:  record.InterfaceName,
		Strategy:       normalizeStrategy(record.Strategy, record.RemoteAddr),
		Status:         record.Status,
		InFlow:         record.InFlow,
		OutFlow:        record.OutFlow,
		RuntimeBackend: record.RuntimeBackend,
		RuntimeStatus:  record.RuntimeStatus,
		RuntimeMessage: record.RuntimeMessage,
		CreatedTime:    record.CreatedAt.UnixMilli(),
		UpdatedTime:    record.UpdatedAt.UnixMilli(),
		UserName:       record.UserName,
		UserID:         record.UserID,
		Inx:            record.Inx,
	}
	if record.RuntimeLastSyncAt != nil {
		item.LastRuntimeSyncTime = record.RuntimeLastSyncAt.UnixMilli()
	}
	if record.Tunnel != nil {
		item.TunnelName = record.Tunnel.Name
		item.InIP = record.Tunnel.InIP
	}
	return item
}

func (s *PanelForwardService) syncForwardRuntime(record *model.Forward, action string) error {
	if record == nil {
		return errors.New("forward record is required")
	}

	var tunnel model.ForwardTunnel
	if record.Tunnel != nil {
		tunnel = *record.Tunnel
	} else if err := s.db.First(&tunnel, record.TunnelID).Error; err != nil {
		return err
	}

	result, err := s.runtimeService.Apply(context.Background(), action, record, &tunnel)
	if result != nil {
		record.RuntimeBackend = result.Backend
		record.RuntimeStatus = result.Status
		record.RuntimeMessage = result.Message
		now := time.Now()
		record.RuntimeLastSyncAt = &now
		if saveErr := s.db.Model(&model.Forward{}).Where("id = ?", record.ID).Updates(map[string]interface{}{
			"runtime_backend":      record.RuntimeBackend,
			"runtime_status":       record.RuntimeStatus,
			"runtime_message":      record.RuntimeMessage,
			"runtime_last_sync_at": record.RuntimeLastSyncAt,
		}).Error; saveErr != nil {
			return saveErr
		}
	}
	if err != nil {
		return err
	}

	record.Tunnel = &tunnel
	return nil
}

func tunnelPortRange(tunnel *model.ForwardTunnel) (int, int) {
	start := defaultTunnelPortStart
	end := defaultTunnelPortEnd
	if tunnel.InNodePortSta != nil && *tunnel.InNodePortSta > 0 {
		start = *tunnel.InNodePortSta
	}
	if tunnel.InNodePortEnd != nil && *tunnel.InNodePortEnd >= start {
		end = *tunnel.InNodePortEnd
	}
	return start, end
}

func validatePanelForwardInput(input PanelForwardInput) error {
	if strings.TrimSpace(input.Name) == "" {
		return errors.New("请输入转发名称")
	}
	if input.TunnelID == 0 {
		return errors.New("请选择关联隧道")
	}
	if strings.TrimSpace(input.RemoteAddr) == "" {
		return errors.New("请输入远程地址")
	}
	for _, raw := range strings.Split(normalizeRemoteAddr(input.RemoteAddr), ",") {
		target := strings.TrimSpace(raw)
		if target == "" {
			continue
		}
		if _, _, err := splitTarget(target); err != nil {
			return fmt.Errorf("目标地址格式错误: %s", target)
		}
	}
	if input.InPort != nil && (*input.InPort < 1 || *input.InPort > 65535) {
		return errors.New("端口号必须在 1-65535 范围内")
	}
	return nil
}

func normalizeRemoteAddr(raw string) string {
	lines := strings.Split(raw, "\n")
	items := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		items = append(items, line)
	}
	return strings.Join(items, ",")
}

func normalizeStrategy(strategy, remoteAddr string) string {
	count := 0
	for _, raw := range strings.Split(normalizeRemoteAddr(remoteAddr), ",") {
		if strings.TrimSpace(raw) != "" {
			count++
		}
	}
	if count <= 1 {
		return "fifo"
	}

	switch strategy {
	case "fifo", "round", "rand", "hash":
		return strategy
	default:
		return "fifo"
	}
}

func resolveForwardUserName(user *model.User) string {
	if user == nil {
		return ""
	}
	if strings.TrimSpace(user.Email) != "" {
		return user.Email
	}
	return fmt.Sprintf("user-%d", user.ID)
}

func resolveTunnelName(tunnel *model.ForwardTunnel) string {
	if tunnel == nil || strings.TrimSpace(tunnel.Name) == "" {
		return "系统"
	}
	return tunnel.Name
}

func splitTarget(target string) (string, int, error) {
	host, portStr, err := net.SplitHostPort(target)
	if err != nil {
		return "", 0, errors.New("无法解析目标地址")
	}
	host = strings.Trim(host, "[]")
	var port int
	if _, scanErr := fmt.Sscanf(portStr, "%d", &port); scanErr != nil || port <= 0 || port > 65535 {
		return "", 0, errors.New("无法解析目标端口")
	}
	return host, port, nil
}
