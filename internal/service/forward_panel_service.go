package service

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
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

var errForwardRuntimeJobInProgress = errors.New("转发运行时任务处理中，请稍后再试")

type PanelForwardService struct {
	db             *gorm.DB
	runtimeService *PanelForwardRuntimeService
}

func (s *PanelForwardService) RuntimeService() *PanelForwardRuntimeService {
	return s.runtimeService
}

type PanelRuntimeJobFilter struct {
	Backend   string
	Status    *int
	ForwardID *uint
	Limit     int
}

func NewPanelForwardService(db *gorm.DB) *PanelForwardService {
	return &PanelForwardService{
		db:             db,
		runtimeService: NewPanelForwardRuntimeService(db),
	}
}

func (s *PanelForwardService) resolveRuntimeBackend() (string, error) {
	if s.runtimeService == nil {
		return model.ForwardRuntimeBackendGost, nil
	}
	return s.runtimeService.resolveBackend()
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
	CreatedTime   int64   `json:"createdTime"`
	UpdatedTime   int64   `json:"updatedTime"`
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
	TunnelID   uint               `json:"tunnelId"`
	TunnelName string             `json:"tunnelName"`
	TunnelType string             `json:"tunnelType"`
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
	backend, err := s.resolveRuntimeBackend()
	if err != nil {
		return nil, err
	}

	items := make([]PanelTunnelListItem, 0, len(tunnels))
	for _, tunnel := range tunnels {
		if err := validatePanelForwardTunnelCompatibility(&tunnel, backend); err != nil {
			continue
		}
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

func (s *PanelForwardService) ListAdminTunnels() ([]PanelAdminTunnelItem, error) {
	var tunnels []model.ForwardTunnel
	if err := s.db.Order("name ASC").Order("id ASC").Find(&tunnels).Error; err != nil {
		return nil, err
	}

	items := make([]PanelAdminTunnelItem, 0, len(tunnels))
	for i := range tunnels {
		items = append(items, buildPanelAdminTunnelItem(&tunnels[i]))
	}
	return items, nil
}

func (s *PanelForwardService) CreateTunnel(input PanelTunnelInput) (*PanelAdminTunnelItem, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.InterfaceName = strings.TrimSpace(input.InterfaceName)
	input.Protocol = strings.TrimSpace(input.Protocol)
	input.TCPListenAddr = normalizePanelTunnelListenAddr(input.TCPListenAddr)
	input.UDPListenAddr = normalizePanelTunnelListenAddr(input.UDPListenAddr)

	trafficRatio := normalizePanelTunnelTrafficRatio(input.TrafficRatio)
	backend, err := s.resolveRuntimeBackend()
	if err != nil {
		return nil, err
	}
	if isForwardRuntimeExecutionNodeBackend(backend) {
		if (input.OutNodeID == nil || *input.OutNodeID == 0) && input.InNodeID != 0 {
			executionNodeID := input.InNodeID
			input.OutNodeID = &executionNodeID
		}
		if input.InNodeID == 0 {
			input.InNodeID = selectPanelTunnelExecutionNodeID(input.InNodeID, input.OutNodeID)
		}
	}
	if err := validatePanelTunnelCreateInput(input, trafficRatio, backend); err != nil {
		return nil, err
	}
	if err := s.ensureTunnelNameUnique(input.Name, 0); err != nil {
		return nil, err
	}

	record := &model.ForwardTunnel{
		Name:          input.Name,
		Type:          input.Type,
		Flow:          input.Flow,
		TrafficRatio:  trafficRatio,
		InterfaceName: input.InterfaceName,
		TCPListenAddr: input.TCPListenAddr,
		UDPListenAddr: input.UDPListenAddr,
		Status:        model.ForwardTunnelStatusActive,
	}

	if isForwardRuntimeExecutionNodeBackend(backend) {
		executionNodeID := selectPanelTunnelExecutionNodeID(input.InNodeID, input.OutNodeID)
		executionNode, err := s.getEnabledForwardNode(executionNodeID, "中转执行节点")
		if err != nil {
			return nil, err
		}
		if err := validateForwardNodeType(executionNode, model.ForwardNodeTypeRelay, "中转执行节点"); err != nil {
			return nil, err
		}

		record.InNodeID = 0
		record.InIP = strings.TrimSpace(executionNode.Host)
		record.OutNodeID = &executionNode.ID
		record.OutIP = strings.TrimSpace(executionNode.Host)
		record.Protocol = ""
	} else {
		inNode, err := s.getEnabledForwardNode(input.InNodeID, "入口节点")
		if err != nil {
			return nil, err
		}
		if err := validateForwardNodeType(inNode, model.ForwardNodeTypeRelay, "入口节点"); err != nil {
			return nil, err
		}

		record.InNodeID = inNode.ID
		record.InIP = strings.TrimSpace(inNode.Host)
		if input.Type == 2 {
			outNode, err := s.getEnabledForwardNode(pointerUintValue(input.OutNodeID), "出口节点")
			if err != nil {
				return nil, err
			}
			if err := validateForwardNodeType(outNode, model.ForwardNodeTypeExit, "出口节点"); err != nil {
				return nil, err
			}
			if outNode.ID == inNode.ID {
				return nil, errors.New("隧道转发模式下，入口和出口不能是同一个节点")
			}
			record.OutNodeID = &outNode.ID
			record.OutIP = strings.TrimSpace(outNode.Host)
			record.Protocol = normalizePanelTunnelProtocol(input.Protocol)
		} else {
			record.OutNodeID = &inNode.ID
			record.OutIP = strings.TrimSpace(inNode.Host)
		}
	}

	if err := s.db.Create(record).Error; err != nil {
		return nil, err
	}

	item := buildPanelAdminTunnelItem(record)
	return &item, nil
}

func (s *PanelForwardService) DeleteTunnel(id uint) error {
	record, err := s.getTunnelByID(id)
	if err != nil {
		return err
	}

	var forwardCount int64
	if err := s.db.Model(&model.Forward{}).Where("tunnel_id = ?", record.ID).Count(&forwardCount).Error; err != nil {
		return err
	}
	if forwardCount > 0 {
		return fmt.Errorf("该隧道还有 %d 个转发在使用，请先删除相关转发", forwardCount)
	}

	var permissionCount int64
	if err := s.db.Model(&model.ForwardUserTunnel{}).Where("tunnel_id = ?", record.ID).Count(&permissionCount).Error; err != nil {
		return err
	}
	if permissionCount > 0 {
		return fmt.Errorf("该隧道还有 %d 个用户权限关联，请先取消用户授权", permissionCount)
	}

	return s.db.Delete(&model.ForwardTunnel{}, record.ID).Error
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

func (s *PanelForwardService) ResetUserTunnelTraffic(id uint) error {
	record, err := s.getUserTunnelByID(id)
	if err != nil {
		return err
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.Forward{}).
			Where("user_id = ? AND tunnel_id = ?", record.UserID, record.TunnelID).
			Updates(map[string]any{
				"in_flow":  0,
				"out_flow": 0,
			}).Error; err != nil {
			return err
		}

		return tx.Model(&model.ForwardUserTunnel{}).
			Where("id = ?", record.ID).
			Updates(map[string]any{
				"in_flow":  0,
				"out_flow": 0,
			}).Error
	})
}

func (s *PanelForwardService) getForwardForActor(forwardID, userID uint, isAdmin bool) (*model.Forward, error) {
	var record model.Forward
	query := s.db.Preload("Tunnel").Where("id = ?", forwardID)
	if !isAdmin {
		query = query.Where("user_id = ?", userID)
	}
	if err := query.First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPanelForwardNotFound
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

func buildForwardPortBindings(forwardID uint, tunnel *model.ForwardTunnel, backend string, inPort int) []model.ForwardPortBinding {
	if forwardID == 0 || tunnel == nil || inPort <= 0 {
		return nil
	}

	scope := buildPanelForwardPortScope(tunnel, backend)
	if scope.NodeID == 0 {
		return nil
	}

	bindings := make([]model.ForwardPortBinding, 0, 2)
	for _, transport := range []string{"tcp", "udp"} {
		if !panelForwardProtocolIncludes(scope.Protocol, transport) {
			continue
		}
		bindings = append(bindings, model.ForwardPortBinding{
			ForwardID:  forwardID,
			NodeID:     scope.NodeID,
			Transport:  transport,
			ListenAddr: normalizePanelForwardListenAddrForConflict(panelForwardListenAddrForTransport(scope, transport)),
			InPort:     inPort,
		})
	}
	return bindings
}

func (s *PanelForwardService) ensureForwardPortBindingsAvailableTx(tx *gorm.DB, bindings []model.ForwardPortBinding, excludeForwardID uint) error {
	for _, binding := range bindings {
		var existing []model.ForwardPortBinding
		query := tx.
			Where("node_id = ? AND transport = ? AND in_port = ?", binding.NodeID, binding.Transport, binding.InPort)
		if excludeForwardID > 0 {
			query = query.Where("forward_id <> ?", excludeForwardID)
		}
		if err := query.Find(&existing).Error; err != nil {
			return err
		}
		for _, item := range existing {
			if panelForwardListenAddrsOverlap(item.ListenAddr, binding.ListenAddr) {
				return errors.New("入口端口已被占用")
			}
		}
	}
	return nil
}

type panelForwardPortScope struct {
	NodeID        uint
	Protocol      string
	TCPListenAddr string
	UDPListenAddr string
}

func buildPanelForwardPortScope(tunnel *model.ForwardTunnel, backend string) panelForwardPortScope {
	if tunnel == nil {
		return panelForwardPortScope{}
	}
	nodeID := tunnel.InNodeID
	if isForwardRuntimeExecutionNodeBackend(backend) {
		nodeID = storedPanelTunnelExecutionNodeID(tunnel)
	}
	return panelForwardPortScope{
		NodeID:        nodeID,
		Protocol:      normalizePanelRuntimeProtocol(tunnel.Protocol),
		TCPListenAddr: normalizePanelForwardListenAddrForConflict(tunnel.TCPListenAddr),
		UDPListenAddr: normalizePanelForwardListenAddrForConflict(tunnel.UDPListenAddr),
	}
}

func panelForwardProtocolIncludes(protocol, transport string) bool {
	switch normalizePanelRuntimeProtocol(protocol) {
	case "both":
		return transport == "tcp" || transport == "udp"
	default:
		return normalizePanelRuntimeProtocol(protocol) == transport
	}
}

func panelForwardListenAddrForTransport(scope panelForwardPortScope, transport string) string {
	if transport == "udp" {
		return scope.UDPListenAddr
	}
	return scope.TCPListenAddr
}

func normalizePanelForwardListenAddrForConflict(raw string) string {
	addr := strings.ToLower(strings.TrimSpace(raw))
	if addr == "" {
		addr = normalizePanelTunnelListenAddr(addr)
	}
	if strings.HasPrefix(addr, "[") && strings.HasSuffix(addr, "]") {
		addr = strings.TrimPrefix(strings.TrimSuffix(addr, "]"), "[")
	}
	return addr
}

func panelForwardListenAddrsOverlap(left, right string) bool {
	left = normalizePanelForwardListenAddrForConflict(left)
	right = normalizePanelForwardListenAddrForConflict(right)
	return left == right || isPanelForwardWildcardListenAddr(left) || isPanelForwardWildcardListenAddr(right)
}

func isPanelForwardWildcardListenAddr(addr string) bool {
	switch normalizePanelForwardListenAddrForConflict(addr) {
	case "", "*", "::", "0.0.0.0":
		return true
	default:
		return false
	}
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
		Updates(map[string]any{
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

	if _, err := s.syncForwardRuntime(record, model.ForwardRuntimeJobActionPause); err != nil {
		record.Status = model.ForwardStatusError
		if saveErr := s.db.Model(&model.Forward{}).Where("id = ?", record.ID).Update("status", model.ForwardStatusError).Error; saveErr != nil {
			return saveErr
		}
		return err
	}

	record.Status = model.ForwardStatusPaused
	return s.db.Model(&model.Forward{}).Where("id = ?", record.ID).Update("status", model.ForwardStatusPaused).Error
}

func (s *PanelForwardService) getTunnelByID(id uint) (*model.ForwardTunnel, error) {
	var tunnel model.ForwardTunnel
	if err := s.db.First(&tunnel, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPanelTunnelNotFound
		}
		return nil, err
	}
	return &tunnel, nil
}

func (s *PanelForwardService) ensureTunnelNameUnique(name string, excludeID uint) error {
	var count int64
	query := s.db.Model(&model.ForwardTunnel{}).Where("name = ?", name)
	if excludeID > 0 {
		query = query.Where("id <> ?", excludeID)
	}
	if err := query.Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return errors.New("隧道名称已存在")
	}
	return nil
}

func (s *PanelForwardService) getForwardNodeByID(id uint) (*model.ForwardNode, error) {
	var node model.ForwardNode
	if err := s.db.First(&node, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPanelForwardNodeNotFound
		}
		return nil, err
	}
	return &node, nil
}

func (s *PanelForwardService) getEnabledForwardNode(id uint, label string) (*model.ForwardNode, error) {
	if id == 0 {
		return nil, fmt.Errorf("%s不能为空", label)
	}

	node, err := s.getForwardNodeByID(id)
	if err != nil {
		if err.Error() == "节点不存在" {
			return nil, fmt.Errorf("%s不存在", label)
		}
		return nil, err
	}
	if !node.Enabled {
		return nil, fmt.Errorf("%s当前已禁用，请先启用节点", label)
	}
	return node, nil
}

func validateForwardNodeType(node *model.ForwardNode, expectedType, label string) error {
	if node == nil {
		return fmt.Errorf("%s不存在", label)
	}
	if strings.EqualFold(strings.TrimSpace(node.Type), expectedType) {
		return nil
	}
	if expectedType == model.ForwardNodeTypeRelay {
		return fmt.Errorf("%s必须是转发中继节点", label)
	}
	return fmt.Errorf("%s必须是转发出口节点", label)
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

func buildPanelAdminTunnelItem(record *model.ForwardTunnel) PanelAdminTunnelItem {
	item := PanelAdminTunnelItem{
		ID:            record.ID,
		Name:          record.Name,
		InNodeID:      record.InNodeID,
		OutNodeID:     record.OutNodeID,
		Type:          record.Type,
		Flow:          record.Flow,
		TrafficRatio:  record.TrafficRatio,
		InterfaceName: record.InterfaceName,
		Protocol:      record.Protocol,
		TCPListenAddr: record.TCPListenAddr,
		UDPListenAddr: record.UDPListenAddr,
		InIP:          record.InIP,
		OutIP:         record.OutIP,
		Status:        record.Status,
		CreatedTime:   record.CreatedAt.UnixMilli(),
		UpdatedTime:   record.UpdatedAt.UnixMilli(),
	}
	if item.TrafficRatio <= 0 {
		item.TrafficRatio = 1
	}
	return item
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

// ForwardRuntimeOutcome is what applying a forward on its node reported:
// the backend in force, the runtime status and message the forward's row
// records, whether the work was queued (a job backend), and the runtime job
// row the change recorded.
type ForwardRuntimeOutcome struct {
	Backend string
	Status  int
	Message string
	Async   bool
	JobID   uint
}

// GetForwardWithTunnel loads a panel forward with its tunnel, for the
// runtime: ErrPanelForwardNotFound when there is none.
func (s *PanelForwardService) GetForwardWithTunnel(id uint) (*model.Forward, error) {
	var record model.Forward
	if err := s.db.Preload("Tunnel").First(&record, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPanelForwardNotFound
		}
		return nil, err
	}
	return &record, nil
}

// ActiveTunnelForwardIDs lists the active forwards of a tunnel, oldest
// first: what a tunnel change re-applies.
func (s *PanelForwardService) ActiveTunnelForwardIDs(tunnelID uint) ([]uint, error) {
	var ids []uint
	err := s.db.Model(&model.Forward{}).Where("tunnel_id = ? AND status = ?", tunnelID, model.ForwardStatusActive).Order("id ASC").Pluck("id", &ids).Error
	return ids, err
}

// ApplyForwardRuntime applies one action for a forward on its node through
// the runtime backend in force (NodeX synchronously, or a local Ansible or
// clean agent job) and records the runtime columns of the forward's row,
// as every legacy forward change does. The legacy routes and the
// KernelNodeOps forward.apply executor run it alike; the status column is
// the caller's.
func (s *PanelForwardService) ApplyForwardRuntime(ctx context.Context, record *model.Forward, action string) (*ForwardRuntimeOutcome, error) {
	result, err := s.syncForwardRuntimeContext(ctx, record, action)
	if result == nil {
		return nil, err
	}
	return &ForwardRuntimeOutcome{Backend: result.Backend, Status: result.Status, Message: result.Message, Async: result.Async, JobID: result.JobID}, err
}

func (s *PanelForwardService) syncForwardRuntime(record *model.Forward, action string) (*panelForwardRuntimeResult, error) {
	return s.syncForwardRuntimeContext(context.Background(), record, action)
}

func (s *PanelForwardService) syncForwardRuntimeContext(ctx context.Context, record *model.Forward, action string) (*panelForwardRuntimeResult, error) {
	if record == nil {
		return nil, errors.New("forward record is required")
	}

	var tunnel model.ForwardTunnel
	if record.Tunnel != nil {
		tunnel = *record.Tunnel
	} else if err := s.db.First(&tunnel, record.TunnelID).Error; err != nil {
		return nil, err
	}

	result, err := s.runtimeService.Apply(ctx, action, record, &tunnel)
	if result != nil {
		record.RuntimeBackend = result.Backend
		record.RuntimeStatus = result.Status
		record.RuntimeMessage = result.Message
		now := time.Now()
		record.RuntimeLastSyncAt = &now
		if saveErr := s.db.Model(&model.Forward{}).Where("id = ?", record.ID).Updates(map[string]any{
			"runtime_backend":      record.RuntimeBackend,
			"runtime_status":       record.RuntimeStatus,
			"runtime_message":      record.RuntimeMessage,
			"runtime_last_sync_at": record.RuntimeLastSyncAt,
		}).Error; saveErr != nil {
			return result, saveErr
		}
	}
	if err != nil {
		return result, err
	}

	record.Tunnel = &tunnel
	return result, nil
}

func validatePanelTunnelCreateInput(input PanelTunnelInput, trafficRatio float64, backend string) error {
	if strings.TrimSpace(input.Name) == "" {
		return errors.New("请输入隧道名称")
	}
	if input.Type != 1 && input.Type != 2 {
		return errors.New("隧道类型必须是 1 或 2")
	}
	if input.Flow != 1 && input.Flow != 2 {
		return errors.New("流量计算方式必须是 1 或 2")
	}
	if trafficRatio <= 0 || trafficRatio > 100 {
		return errors.New("流量倍率必须在 0.0-100.0 之间")
	}
	if isForwardRuntimeExecutionNodeBackend(backend) {
		if input.Type != 1 {
			return errors.New("ansible 转发模式仅支持端口转发")
		}
		if selectPanelTunnelExecutionNodeID(input.InNodeID, input.OutNodeID) == 0 {
			return errors.New("请选择中转执行节点")
		}
		return nil
	}
	if input.InNodeID == 0 {
		return errors.New("请选择入口节点")
	}
	if input.Type == 2 {
		if input.OutNodeID == nil || *input.OutNodeID == 0 {
			return errors.New("请选择出口节点")
		}
		if *input.OutNodeID == input.InNodeID {
			return errors.New("隧道转发模式下，入口和出口不能是同一个节点")
		}
	}
	return nil
}

func normalizePanelTunnelTrafficRatio(value *float64) float64 {
	if value == nil || *value == 0 {
		return 1
	}
	return *value
}

func normalizePanelTunnelProtocol(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return "tls"
	}
	return strings.TrimSpace(raw)
}

func normalizePanelTunnelListenAddr(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return "[::]"
	}
	return strings.TrimSpace(raw)
}

func resolvePanelTunnelTypeName(tunnelType int) string {
	switch tunnelType {
	case 2:
		return "隧道转发"
	default:
		return "端口转发"
	}
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

func resolveTunnelName(tunnel *model.ForwardTunnel) string {
	if tunnel == nil || strings.TrimSpace(tunnel.Name) == "" {
		return "系统"
	}
	return tunnel.Name
}

func pointerUintValue(value *uint) uint {
	if value == nil {
		return 0
	}
	return *value
}

func selectPanelTunnelExecutionNodeID(inNodeID uint, outNodeID *uint) uint {
	if outNodeID != nil && *outNodeID != 0 {
		return *outNodeID
	}
	return inNodeID
}

func storedPanelTunnelExecutionNodeID(tunnel *model.ForwardTunnel) uint {
	if tunnel == nil {
		return 0
	}
	return selectPanelTunnelExecutionNodeID(tunnel.InNodeID, tunnel.OutNodeID)
}

func validatePanelForwardTunnelCompatibility(tunnel *model.ForwardTunnel, backend string) error {
	if tunnel == nil {
		return errors.New("隧道不存在")
	}
	switch backend {
	case model.ForwardRuntimeBackendNftablesAnsible, model.ForwardRuntimeBackendIptablesAnsible, model.ForwardRuntimeBackendCleanAgent:
		if tunnel.Type != 1 {
			return errors.New("当前为 Ansible/iptables 模式，仅可使用端口转发隧道")
		}
		if storedPanelTunnelExecutionNodeID(tunnel) == 0 {
			return errors.New("当前隧道未配置中转执行节点，无法用于 Ansible/iptables 模式")
		}
	case model.ForwardRuntimeBackendGost:
		if tunnel.InNodeID == 0 {
			return errors.New("当前为 NodeX/Gost 模式，所选隧道未配置入口节点")
		}
		if tunnel.Type == 2 && (tunnel.OutNodeID == nil || *tunnel.OutNodeID == 0) {
			return errors.New("当前为 NodeX/Gost 模式，隧道转发缺少出口节点")
		}
	default:
		return fmt.Errorf("unsupported forward runtime backend: %s", backend)
	}
	return nil
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
