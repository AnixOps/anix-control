package native

import (
	"context"
	"errors"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/packages/subscription/native/model"
	"github.com/gin-gonic/gin/binding"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Messages of the kernel's subscription handlers and service errors.
const (
	invalidID        = "ID 无效"
	groupNotFound    = "分组不存在"
	templateNotFound = "模板不存在"
)

// Formats is GET /api/v2/admin/subscription/formats: the formats a
// subscription link can be rendered in.
func (s *Service) Formats(context.Context, pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	return s.panel([]map[string]string{
		{"id": "auto", "name": "Auto (UA)", "description": "根据客户端 User-Agent 自动识别订阅格式"},
		{"id": "v2ray", "name": "V2Ray Base64", "description": "适用于 V2RayN, V2RayNG, V2Box 等"},
		{"id": "clash", "name": "Clash YAML", "description": "适用于 Clash, ClashX 等"},
		{"id": "stash", "name": "Stash YAML", "description": "适用于 Stash（Clash YAML 兼容）"},
		{"id": "egern", "name": "Egern YAML", "description": "适用于 Egern（Clash YAML 兼容）"},
		{"id": "surge", "name": "Surge", "description": "适用于 Surge iOS/macOS"},
		{"id": "loon", "name": "Loon", "description": "适用于 Loon iOS"},
		{"id": "shadowrocket", "name": "Shadowrocket", "description": "适用于 Shadowrocket iOS"},
		{"id": "quantumultx", "name": "Quantumult X", "description": "适用于 Quantumult X iOS"},
		{"id": "json", "name": "JSON", "description": "原始 JSON 格式"},
		{"id": "base64json", "name": "Base64 JSON", "description": "Base64 编码的分组 JSON 格式"},
		{"id": "sing-box", "name": "Sing-box", "description": "适用于 Sing-box, Nekobox 等"},
		{"id": "wireguard", "name": "WireGuard", "description": "原生 WireGuard 配置，适用于 WireGuard/Shadowrocket/Loon 等支持导入 .conf 的客户端"},
	})
}

// ProtocolTypes is GET /api/v2/admin/subscription/protocols: the protocol
// types a template can have.
func (s *Service) ProtocolTypes(context.Context, pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	return s.panel([]map[string]any{
		{"id": "vmess", "name": "VMess", "description": "V2Ray VMess 协议", "supports_tls": true, "supports_reality": false},
		{"id": "vless", "name": "VLESS", "description": "V2Ray VLESS 协议", "supports_tls": true, "supports_reality": true},
		{"id": "trojan", "name": "Trojan", "description": "Trojan 协议", "supports_tls": true, "supports_reality": false},
		{"id": "shadowsocks", "name": "Shadowsocks", "description": "Shadowsocks 协议", "supports_tls": false, "supports_reality": false},
		{"id": "hysteria2", "name": "Hysteria2", "description": "Hysteria2 协议", "supports_tls": true, "supports_reality": false},
		{"id": "tuic", "name": "TUIC", "description": "TUIC 协议", "supports_tls": true, "supports_reality": false},
		{"id": "wireguard", "name": "WireGuard", "description": "WireGuard 用户接入协议", "supports_tls": false, "supports_reality": false},
	})
}

// Groups is GET /api/v2/admin/subscription/groups.
func (s *Service) Groups(ctx context.Context, _ pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError(err.Error())
	}
	groups, err := listGroups(db)
	if err != nil {
		return s.panelError(err.Error())
	}
	return s.panel(groups)
}

func listGroups(db *gorm.DB) ([]*model.SubscriptionGroup, error) {
	var groups []*model.SubscriptionGroup
	err := db.Order("priority DESC, id ASC").Find(&groups).Error
	return groups, err
}

// CreateGroup is POST /api/v2/admin/subscription/groups. As in the kernel,
// only the group's own columns are written: templates and protocols in the
// body are dropped.
func (s *Service) CreateGroup(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	var group model.SubscriptionGroup
	if err := binding.JSON.BindBody(request.Body, &group); err != nil {
		return s.panelError(err.Error())
	}
	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError(err.Error())
	}
	group.Templates, group.Protocols = nil, nil
	if err := db.Omit(clause.Associations).Create(&group).Error; err != nil {
		return s.panelError(err.Error())
	}
	return s.panel(group)
}

// Group is GET /api/v2/admin/subscription/groups/:id, with its templates.
func (s *Service) Group(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	id, ok := pathUint(request, "id")
	if !ok {
		return s.panelError(invalidID)
	}
	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError(groupNotFound)
	}
	var group model.SubscriptionGroup
	if err := db.Preload("Templates").First(&group, id).Error; err != nil {
		return s.panelError(groupNotFound)
	}
	return s.panel(&group)
}

// UpdateGroup is PUT /api/v2/admin/subscription/groups/:id. The body
// replaces every column, as the kernel's Save does: a missing field is
// written as its zero value, and an unknown id creates the group.
func (s *Service) UpdateGroup(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	id, ok := pathUint(request, "id")
	if !ok {
		return s.panelError(invalidID)
	}
	var group model.SubscriptionGroup
	if err := binding.JSON.BindBody(request.Body, &group); err != nil {
		return s.panelError(err.Error())
	}
	group.ID = id
	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError(err.Error())
	}
	group.Templates, group.Protocols = nil, nil
	if err := db.Omit(clause.Associations).Save(&group).Error; err != nil {
		return s.panelError(err.Error())
	}
	return s.panel(group)
}

// groupStats is a group's statistics, with the kernel's field order and
// tags.
type groupStats struct {
	GroupID       uint   `json:"group_id"`
	GroupName     string `json:"group_name"`
	UserCount     int64  `json:"user_count"`
	TemplateCount int64  `json:"template_count"`
	ProtocolCount int64  `json:"protocol_count"`
	OnlineNodes   int64  `json:"online_nodes"`
	TotalTraffic  int64  `json:"total_traffic"`
	EnabledUsers  int64  `json:"enabled_users"`
	PlanCount     int64  `json:"plan_count"`
}

// Stats is GET /api/v2/admin/subscription/stats. Members and their expiry
// come from kapi_user_subscription_group_v1, their traffic from
// kapi_subscriber_entitlement_v1, and a protocol's node and its last report
// from kapi_node_protocol_v1 and kapi_node_heartbeat_v1. As in the kernel,
// a count that fails stays zero.
func (s *Service) Stats(ctx context.Context, _ pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	db, err := s.Open(ctx)
	if err != nil {
		return jsonAnswer(500, map[string]any{"message": "获取统计失败", "error": err.Error()})
	}
	groups, err := listGroups(db)
	if err != nil {
		return jsonAnswer(500, map[string]any{"message": "获取统计失败", "error": err.Error()})
	}
	now := s.now().Unix()
	fiveMinutesAgo := now - 300
	stats := make([]groupStats, 0, len(groups))
	for _, g := range groups {
		gs := groupStats{GroupID: g.ID, GroupName: g.Name}
		db.Model(&UserGroup{}).Where("group_id = ?", g.ID).Count(&gs.UserCount)
		db.Model(&UserGroup{}).Where("group_id = ? AND (expire_at IS NULL OR expire_at > ?)", g.ID, now).Count(&gs.EnabledUsers)
		db.Model(&model.SubscriptionTemplate{}).Where("group_id = ?", g.ID).Count(&gs.TemplateCount)
		db.Model(&model.GroupProtocol{}).Where("subscription_group_id = ?", g.ID).Count(&gs.ProtocolCount)
		db.Table(model.GroupProtocol{}.TableName()+" AS sgnp").
			Joins("JOIN "+NodeProtocol{}.TableName()+" AS np ON np.id = sgnp.node_protocol_id").
			Joins("JOIN "+NodeHeartbeat{}.TableName()+" AS n ON n.id = np.node_id").
			Where("sgnp.subscription_group_id = ? AND n.last_check_at > ?", g.ID, fiveMinutesAgo).
			Distinct("n.id").
			Count(&gs.OnlineNodes)
		var traffic struct{ Total int64 }
		db.Raw(`
			SELECT COALESCE(SUM(u.u + u.d), 0) AS total
			FROM `+Entitlement{}.TableName()+` u
			INNER JOIN `+UserGroup{}.TableName()+` usg ON usg.user_id = u.id
			WHERE usg.group_id = ? AND (usg.expire_at IS NULL OR usg.expire_at > ?)
		`, g.ID, now).Scan(&traffic)
		gs.TotalTraffic = traffic.Total
		db.Model(&model.PlanSubscriptionGroup{}).Where("group_id = ?", g.ID).Count(&gs.PlanCount)
		stats = append(stats, gs)
	}
	return s.panel(stats)
}

// groupExists is the kernel's subscriptionGroupExists.
func groupExists(db *gorm.DB, groupID uint) error {
	var group model.SubscriptionGroup
	if err := db.Select("id").First(&group, groupID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errGroupNotFound
		}
		return err
	}
	return nil
}
