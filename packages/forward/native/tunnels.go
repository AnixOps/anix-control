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

// adminTunnelItem is the legacy PanelAdminTunnelItem.
type adminTunnelItem struct {
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

func buildAdminTunnelItem(record *Tunnel) adminTunnelItem {
	item := adminTunnelItem{
		ID: record.ID, Name: record.Name, InNodeID: record.InNodeID, OutNodeID: record.OutNodeID, Type: record.Type,
		Flow: record.Flow, TrafficRatio: record.TrafficRatio, InterfaceName: record.InterfaceName, Protocol: record.Protocol,
		TCPListenAddr: record.TCPListenAddr, UDPListenAddr: record.UDPListenAddr, InIP: record.InIP, OutIP: record.OutIP,
		Status: record.Status, CreatedTime: record.CreatedAt.UnixMilli(), UpdatedTime: record.UpdatedAt.UnixMilli(),
	}
	if item.TrafficRatio <= 0 {
		item.TrafficRatio = 1
	}
	return item
}

// tunnelItem is the legacy PanelTunnelListItem, a tunnel a forward can use.
type tunnelItem struct {
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

// ListAdminTunnels is POST /api/v2/admin/tunnel/list: every tunnel.
func (s *Service) ListAdminTunnels(ctx context.Context, _ pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError(err.Error())
	}
	var tunnels []Tunnel
	if err := db.Order("name ASC").Order("id ASC").Find(&tunnels).Error; err != nil {
		return s.panelError(err.Error())
	}
	items := make([]adminTunnelItem, 0, len(tunnels))
	for i := range tunnels {
		items = append(items, buildAdminTunnelItem(&tunnels[i]))
	}
	return s.panel(items)
}

// ListTunnels is POST /api/v2/tunnel/user/tunnel and its administrator
// route: the active tunnels the caller may use (every one for an
// administrator), as far as they suit the configured runtime backend.
func (s *Service) ListTunnels(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	userID, isAdmin := actor(request)
	return s.listTunnels(ctx, userID, isAdmin)
}

// listTunnels is the kernel's PanelForwardService.ListTunnels.
func (s *Service) listTunnels(ctx context.Context, userID uint, isAdmin bool) (pluginhostsdk.NativeResponse, error) {
	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError(err.Error())
	}
	tunnels, err := accessibleTunnels(db, userID, isAdmin)
	if err != nil {
		return s.panelError(err.Error())
	}
	backend, err := resolveRuntimeBackend(db)
	if err != nil {
		return s.panelError(err.Error())
	}
	items := make([]tunnelItem, 0, len(tunnels))
	for i := range tunnels {
		tunnel := &tunnels[i]
		if validateForwardTunnelCompatibility(tunnel, backend) != nil {
			continue
		}
		items = append(items, tunnelItem{
			ID: tunnel.ID, Name: tunnel.Name, IP: tunnel.InIP, InIP: tunnel.InIP, InNodePortSta: tunnel.InNodePortSta,
			InNodePortEnd: tunnel.InNodePortEnd, Type: tunnel.Type, Protocol: tunnel.Protocol, Status: tunnel.Status,
		})
	}
	return s.panel(items)
}

// accessibleTunnels are the active tunnels a user holds a permission for,
// whatever the permission's state, or every active one for an
// administrator.
func accessibleTunnels(db *gorm.DB, userID uint, isAdmin bool) ([]Tunnel, error) {
	var tunnels []Tunnel
	if isAdmin {
		if err := db.Where("status = ?", tunnelActive).Order("name ASC").Find(&tunnels).Error; err != nil {
			return nil, err
		}
		return tunnels, nil
	}
	if userID == 0 {
		return []Tunnel{}, nil
	}
	var permissions []UserTunnel
	if err := db.Where("user_id = ?", userID).Order("id ASC").Find(&permissions).Error; err != nil {
		return nil, err
	}
	if len(permissions) == 0 {
		return []Tunnel{}, nil
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
	if err := db.Where("id IN ? AND status = ?", tunnelIDs, tunnelActive).Order("name ASC").Find(&tunnels).Error; err != nil {
		return nil, err
	}
	return tunnels, nil
}

// tunnelInput is the legacy PanelTunnelInput.
type tunnelInput struct {
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

// CreateTunnel is POST /api/v2/admin/tunnel/create. A tunnel alone runs
// nothing on a node: forwards do, so nothing is pushed. Its nodes are read
// through kapi_forward_node_v1.
func (s *Service) CreateTunnel(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	var input tunnelInput
	if err := binding.JSON.BindBody(request.Body, &input); err != nil {
		return s.panelError("参数错误")
	}
	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError(err.Error())
	}
	item, err := createTunnel(db, input)
	if err != nil {
		return s.panelError(err.Error())
	}
	return s.panel(item)
}

func createTunnel(db *gorm.DB, input tunnelInput) (*adminTunnelItem, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.InterfaceName = strings.TrimSpace(input.InterfaceName)
	input.Protocol = strings.TrimSpace(input.Protocol)
	input.TCPListenAddr = normalizeTunnelListenAddr(input.TCPListenAddr)
	input.UDPListenAddr = normalizeTunnelListenAddr(input.UDPListenAddr)

	trafficRatio := normalizeTunnelTrafficRatio(input.TrafficRatio)
	backend, err := resolveRuntimeBackend(db)
	if err != nil {
		return nil, err
	}
	if isExecutionNodeBackend(backend) {
		if (input.OutNodeID == nil || *input.OutNodeID == 0) && input.InNodeID != 0 {
			executionNodeID := input.InNodeID
			input.OutNodeID = &executionNodeID
		}
		if input.InNodeID == 0 {
			input.InNodeID = selectTunnelExecutionNodeID(input.InNodeID, input.OutNodeID)
		}
	}
	if err := validateTunnelCreateInput(input, trafficRatio, backend); err != nil {
		return nil, err
	}
	if err := ensureTunnelNameUnique(db, input.Name, 0); err != nil {
		return nil, err
	}

	record := &Tunnel{
		Name: input.Name, Type: input.Type, Flow: input.Flow, TrafficRatio: trafficRatio, InterfaceName: input.InterfaceName,
		TCPListenAddr: input.TCPListenAddr, UDPListenAddr: input.UDPListenAddr, Status: tunnelActive,
	}
	if isExecutionNodeBackend(backend) {
		executionNode, err := enabledForwardNode(db, selectTunnelExecutionNodeID(input.InNodeID, input.OutNodeID), "中转执行节点")
		if err != nil {
			return nil, err
		}
		if err := validateForwardNodeType(executionNode, nodeTypeRelay, "中转执行节点"); err != nil {
			return nil, err
		}
		record.InNodeID = 0
		record.InIP = strings.TrimSpace(executionNode.Host)
		record.OutNodeID = &executionNode.ID
		record.OutIP = strings.TrimSpace(executionNode.Host)
		record.Protocol = ""
	} else {
		inNode, err := enabledForwardNode(db, input.InNodeID, "入口节点")
		if err != nil {
			return nil, err
		}
		if err := validateForwardNodeType(inNode, nodeTypeRelay, "入口节点"); err != nil {
			return nil, err
		}
		record.InNodeID = inNode.ID
		record.InIP = strings.TrimSpace(inNode.Host)
		if input.Type == 2 {
			var outNodeID uint
			if input.OutNodeID != nil {
				outNodeID = *input.OutNodeID
			}
			outNode, err := enabledForwardNode(db, outNodeID, "出口节点")
			if err != nil {
				return nil, err
			}
			if err := validateForwardNodeType(outNode, nodeTypeExit, "出口节点"); err != nil {
				return nil, err
			}
			if outNode.ID == inNode.ID {
				return nil, errors.New("隧道转发模式下，入口和出口不能是同一个节点")
			}
			record.OutNodeID = &outNode.ID
			record.OutIP = strings.TrimSpace(outNode.Host)
			record.Protocol = normalizeTunnelProtocol(input.Protocol)
		} else {
			record.OutNodeID = &inNode.ID
			record.OutIP = strings.TrimSpace(inNode.Host)
		}
	}

	if err := db.Create(record).Error; err != nil {
		return nil, err
	}
	item := buildAdminTunnelItem(record)
	return &item, nil
}

// DeleteTunnel is POST /api/v2/admin/tunnel/delete: a tunnel no forward
// and no user permission uses, so no node runs anything for it.
func (s *Service) DeleteTunnel(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
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
	record, err := tunnelByID(db, req.ID)
	if err != nil {
		return s.panelError(err.Error())
	}
	var forwardCount int64
	if err := db.Model(&Forward{}).Where("tunnel_id = ?", record.ID).Count(&forwardCount).Error; err != nil {
		return s.panelError(err.Error())
	}
	if forwardCount > 0 {
		return s.panelError(fmt.Sprintf("该隧道还有 %d 个转发在使用，请先删除相关转发", forwardCount))
	}
	var permissionCount int64
	if err := db.Model(&UserTunnel{}).Where("tunnel_id = ?", record.ID).Count(&permissionCount).Error; err != nil {
		return s.panelError(err.Error())
	}
	if permissionCount > 0 {
		return s.panelError(fmt.Sprintf("该隧道还有 %d 个用户权限关联，请先取消用户授权", permissionCount))
	}
	if err := db.Delete(&Tunnel{}, record.ID).Error; err != nil {
		return s.panelError(err.Error())
	}
	return s.panel(true)
}

func tunnelByID(db *gorm.DB, id uint) (*Tunnel, error) {
	var tunnel Tunnel
	if err := db.First(&tunnel, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("隧道不存在")
		}
		return nil, err
	}
	return &tunnel, nil
}

func ensureTunnelNameUnique(db *gorm.DB, name string, excludeID uint) error {
	var count int64
	query := db.Model(&Tunnel{}).Where("name = ?", name)
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

// enabledForwardNode reads a forward node from kapi_forward_node_v1 and
// requires it to be enabled.
func enabledForwardNode(db *gorm.DB, id uint, label string) (*Node, error) {
	if id == 0 {
		return nil, fmt.Errorf("%s不能为空", label)
	}
	var node Node
	if err := db.First(&node, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%s不存在", label)
		}
		return nil, err
	}
	if !node.Enabled {
		return nil, fmt.Errorf("%s当前已禁用，请先启用节点", label)
	}
	return &node, nil
}

func validateForwardNodeType(node *Node, expectedType, label string) error {
	if strings.EqualFold(strings.TrimSpace(node.Type), expectedType) {
		return nil
	}
	if expectedType == nodeTypeRelay {
		return fmt.Errorf("%s必须是转发中继节点", label)
	}
	return fmt.Errorf("%s必须是转发出口节点", label)
}

func validateTunnelCreateInput(input tunnelInput, trafficRatio float64, backend string) error {
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
	if isExecutionNodeBackend(backend) {
		if input.Type != 1 {
			return errors.New("ansible 转发模式仅支持端口转发")
		}
		if selectTunnelExecutionNodeID(input.InNodeID, input.OutNodeID) == 0 {
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

func normalizeTunnelTrafficRatio(value *float64) float64 {
	if value == nil || *value == 0 {
		return 1
	}
	return *value
}

func normalizeTunnelProtocol(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return "tls"
	}
	return strings.TrimSpace(raw)
}

func normalizeTunnelListenAddr(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return "[::]"
	}
	return strings.TrimSpace(raw)
}

func selectTunnelExecutionNodeID(inNodeID uint, outNodeID *uint) uint {
	if outNodeID != nil && *outNodeID != 0 {
		return *outNodeID
	}
	return inNodeID
}

// validateForwardTunnelCompatibility reports whether a forward on the
// tunnel can run on the backend.
func validateForwardTunnelCompatibility(tunnel *Tunnel, backend string) error {
	switch backend {
	case backendNftablesAnsible, backendIptablesAnsible, backendCleanAgent:
		if tunnel.Type != 1 {
			return errors.New("当前为 Ansible/iptables 模式，仅可使用端口转发隧道")
		}
		if selectTunnelExecutionNodeID(tunnel.InNodeID, tunnel.OutNodeID) == 0 {
			return errors.New("当前隧道未配置中转执行节点，无法用于 Ansible/iptables 模式")
		}
	case backendGost:
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
