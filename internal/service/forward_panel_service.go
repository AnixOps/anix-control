package service

import (
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
)

type PanelForwardService struct {
	db *gorm.DB
}

func NewPanelForwardService(db *gorm.DB) *PanelForwardService {
	return &PanelForwardService{db: db}
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
	ID            uint   `json:"id"`
	Name          string `json:"name"`
	TunnelID      uint   `json:"tunnelId"`
	TunnelName    string `json:"tunnelName"`
	InIP          string `json:"inIp"`
	InPort        int    `json:"inPort"`
	RemoteAddr    string `json:"remoteAddr"`
	InterfaceName string `json:"interfaceName"`
	Strategy      string `json:"strategy"`
	Status        int    `json:"status"`
	InFlow        int64  `json:"inFlow"`
	OutFlow       int64  `json:"outFlow"`
	CreatedTime   int64  `json:"createdTime"`
	UpdatedTime   int64  `json:"updatedTime"`
	UserName      string `json:"userName"`
	UserID        uint   `json:"userId"`
	Inx           int    `json:"inx"`
}

type PanelTunnelListItem struct {
	ID            uint   `json:"id"`
	Name          string `json:"name"`
	InIP          string `json:"inIp"`
	InNodePortSta *int   `json:"inNodePortSta"`
	InNodePortEnd *int   `json:"inNodePortEnd"`
	Status        int    `json:"status"`
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

func (s *PanelForwardService) ListTunnels(_ uint, _ bool) ([]PanelTunnelListItem, error) {
	var tunnels []model.ForwardTunnel
	if err := s.db.Where("status = ?", model.ForwardTunnelStatusActive).Order("name ASC").Find(&tunnels).Error; err != nil {
		return nil, err
	}

	items := make([]PanelTunnelListItem, 0, len(tunnels))
	for _, tunnel := range tunnels {
		items = append(items, PanelTunnelListItem{
			ID:            tunnel.ID,
			Name:          tunnel.Name,
			InIP:          tunnel.InIP,
			InNodePortSta: tunnel.InNodePortSta,
			InNodePortEnd: tunnel.InNodePortEnd,
			Status:        tunnel.Status,
		})
	}
	return items, nil
}

func (s *PanelForwardService) CreateForward(userID uint, input PanelForwardInput) (*PanelForwardListItem, error) {
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

	if err := s.db.Save(record).Error; err != nil {
		return nil, err
	}
	if err := s.db.Preload("Tunnel").First(record, record.ID).Error; err != nil {
		return nil, err
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

	return s.db.Delete(&model.Forward{}, record.ID).Error
}

func (s *PanelForwardService) SetForwardStatus(userID uint, isAdmin bool, forwardID uint, status int) error {
	record, err := s.getForwardForActor(forwardID, userID, isAdmin)
	if err != nil {
		return err
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

func buildPanelForwardItem(record *model.Forward) PanelForwardListItem {
	item := PanelForwardListItem{
		ID:            record.ID,
		Name:          record.Name,
		TunnelID:      record.TunnelID,
		InPort:        record.InPort,
		RemoteAddr:    record.RemoteAddr,
		InterfaceName: record.InterfaceName,
		Strategy:      normalizeStrategy(record.Strategy, record.RemoteAddr),
		Status:        record.Status,
		InFlow:        record.InFlow,
		OutFlow:       record.OutFlow,
		CreatedTime:   record.CreatedAt.UnixMilli(),
		UpdatedTime:   record.UpdatedAt.UnixMilli(),
		UserName:      record.UserName,
		UserID:        record.UserID,
		Inx:           record.Inx,
	}
	if record.Tunnel != nil {
		item.TunnelName = record.Tunnel.Name
		item.InIP = record.Tunnel.InIP
	}
	return item
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
