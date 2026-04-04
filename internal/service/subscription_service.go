package service

import (
	"bytes"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"text/template"
	"time"

	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
	"github.com/anixops/v2board/internal/parser"
	"gorm.io/gorm"
)

// SubscriptionService 订阅服务
type SubscriptionService struct {
	db       *gorm.DB
	registry *parser.Registry
}

// NewSubscriptionService 创建订阅服务
func NewSubscriptionService() *SubscriptionService {
	return &SubscriptionService{
		db:       database.Get(),
		registry: parser.GetDefaultRegistry(),
	}
}

// GetUserSubscription 获取用户订阅
// token: 用户 token
// format: 订阅格式
// groups: 指定分组 (可选)
// include: 包含关键词
// exclude: 排除关键词
func (s *SubscriptionService) GetUserSubscription(req *model.SubscriptionRequest) (*model.SubscriptionResponse, error) {
	// 1. 根据 token 获取用户
	user, err := s.getUserByToken(req.Token)
	if err != nil {
		return nil, fmt.Errorf("invalid token")
	}

	// 2. 检查用户有效性
	if !user.IsValid() {
		return nil, fmt.Errorf("user is not valid or expired")
	}

	// 3. 获取用户可用的订阅分组
	groups, err := s.getUserSubscriptionGroups(user)
	if err != nil {
		return nil, err
	}

	// 过滤指定的分组
	if len(req.Groups) > 0 {
		filtered := make([]*model.SubscriptionGroup, 0)
		for _, g := range groups {
			for _, gid := range req.Groups {
				if g.ID == gid {
					filtered = append(filtered, g)
					break
				}
			}
		}
		groups = filtered
	}

	// 4. 获取所有分组的模板
	templates, err := s.getTemplatesForGroups(groups)
	if err != nil {
		return nil, err
	}

	// 5. 构建渲染上下文
	ctx := s.buildRenderContext(user, req)

	// 6. 将模板渲染为 ParsedNode
	nodes := s.renderTemplates(templates, ctx, groups)

	// 7. 应用过滤规则
	nodes = s.filterNodes(nodes, req.Include, req.Exclude)

	// 8. 获取内部节点 (从 Node 表, 基于分组关联)
	internalNodes, err := s.getInternalNodes(user, ctx, groups)
	if err == nil && len(internalNodes) > 0 {
		nodes = append(internalNodes, nodes...)
	}

	// 9. 格式化输出
	format := req.Format
	if format == "" {
		format = model.FormatV2Ray
	}

	// 9. 去重 (针对非 V2Ray 分组模式)
	if !(format == model.FormatV2Ray && len(req.Groups) > 0) {
		uniqueNodes := make([]*model.ParsedNode, 0, len(nodes))
		nodeMap := make(map[string]bool)
		for _, n := range nodes {
			if !nodeMap[n.ID] {
				uniqueNodes = append(uniqueNodes, n)
				nodeMap[n.ID] = true
			}
		}
		nodes = uniqueNodes
	}

	formatter, ok := s.registry.GetFormatter(format)
	if !ok {
		return nil, fmt.Errorf("unsupported format: %s", format)
	}

	// 如果请求了分组且格式为 V2Ray，按分组生成：先对每组分别格式化（得到各自的编码结果），
	// 对于 V2Ray 格式解码每组得到原始行，再合并为一个整体字符串后统一 base64 编码返回。
	var content []byte
	if format == model.FormatV2Ray && len(req.Groups) > 0 {
		var groupPlainParts []string

		// 获取全部内部节点一次，用于按组过滤
		internalNodes, _ := s.getInternalNodes(user, ctx, groups)

		for _, g := range groups {
			// 模板节点
			tplList, _ := s.GetTemplatesByGroup(g.ID)
			var groupTemplates []*model.SubscriptionTemplate
			for _, t := range tplList {
				if t.Enable == 1 {
					groupTemplates = append(groupTemplates, t)
				}
			}

			// 渲染该组的模板
			groupNodes := make([]*model.ParsedNode, 0)
			for _, tpl := range groupTemplates {
				n := s.renderTemplate(tpl, ctx)
				if n != nil {
					n.GroupID = tpl.GroupID
					n.GroupName = g.Name
					groupNodes = append(groupNodes, n)
				}
			}

			// 添加属于该组的内部节点
			for _, in := range internalNodes {
				if in.GroupID == g.ID {
					groupNodes = append(groupNodes, in)
				}
			}

			// 使用 formatter 对该组格式化（可能对 V2Ray 返回 base64 编码）
			partBytes, err := formatter.Format(groupNodes, ctx)
			if err != nil {
				continue
			}

			// 对 V2Ray，formatter 返回的是 base64 编码的内容，解码得到明文行
			decoded, err := base64.StdEncoding.DecodeString(string(partBytes))
			if err != nil {
				// 如果解码失败，尝试直接将内容当作明文使用
				groupPlainParts = append(groupPlainParts, string(partBytes))
			} else {
				groupPlainParts = append(groupPlainParts, string(decoded))
			}
		}

		combinedPlain := strings.Join(groupPlainParts, "\n")
		content = []byte(base64.StdEncoding.EncodeToString([]byte(combinedPlain)))
	} else {
		content, err = formatter.Format(nodes, ctx)
		if err != nil {
			return nil, err
		}
	}

	// 10. 构建响应
	expiredAt := int64(0)
	if user.ExpiredAt != nil {
		expiredAt = *user.ExpiredAt
	}

	return &model.SubscriptionResponse{
		Content:      string(content),
		ContentType:  formatter.ContentType(),
		Filename:     fmt.Sprintf("subscription.%s", formatter.FileExtension()),
		UpdatedAt:    time.Now().Unix(),
		ExpireAt:     expiredAt,
		UsedTraffic:  user.U + user.D,
		TotalTraffic: user.TransferEnable,
	}, nil
}

// getUserByToken 根据 token 获取用户
func (s *SubscriptionService) getUserByToken(token string) (*model.User, error) {
	var user model.User
	err := s.db.Preload("Plan").Where("token = ?", token).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// getUserSubscriptionGroups 获取用户可用的订阅分组
func (s *SubscriptionService) getUserSubscriptionGroups(user *model.User) ([]*model.SubscriptionGroup, error) {
	var groups []*model.SubscriptionGroup
	now := time.Now().Unix()

	// 1. 通过用户直接关联获取
	var userGroups []model.UserSubscriptionGroup
	s.db.Where("user_id = ? AND (expire_at IS NULL OR expire_at > ?)", user.ID, now).Find(&userGroups)

	groupIDs := make([]uint, 0)
	for _, ug := range userGroups {
		groupIDs = append(groupIDs, ug.GroupID)
	}

	// 2. 通过套餐关联获取
	if user.PlanID != nil {
		var planGroups []model.PlanSubscriptionGroup
		s.db.Where("plan_id = ?", *user.PlanID).Find(&planGroups)
		for _, pg := range planGroups {
			exists := false
			for _, gid := range groupIDs {
				if gid == pg.GroupID {
					exists = true
					break
				}
			}
			if !exists {
				groupIDs = append(groupIDs, pg.GroupID)
			}
		}
	}

	// 3. 查询分组详情
	if len(groupIDs) > 0 {
		s.db.Where("id IN ? AND enable = 1", groupIDs).Order("priority DESC, id ASC").Find(&groups)
	}

	// 4. 如果没有分组，检查是否有默认分组
	if len(groups) == 0 {
		var defaultGroup model.SubscriptionGroup
		if err := s.db.Where("name = ? AND enable = 1", "default").First(&defaultGroup).Error; err == nil {
			groups = append(groups, &defaultGroup)
		}
	}

	return groups, nil
}

// getTemplatesForGroups 获取分组的模板
func (s *SubscriptionService) getTemplatesForGroups(groups []*model.SubscriptionGroup) ([]*model.SubscriptionTemplate, error) {
	if len(groups) == 0 {
		return nil, nil
	}

	groupIDs := make([]uint, len(groups))
	for i, g := range groups {
		groupIDs[i] = g.ID
	}

	var templates []*model.SubscriptionTemplate
	err := s.db.Where("group_id IN ? AND enable = 1", groupIDs).
		Order("sort ASC, id ASC").
		Find(&templates).Error

	return templates, err
}

// buildRenderContext 构建渲染上下文
func (s *SubscriptionService) buildRenderContext(user *model.User, req *model.SubscriptionRequest) *model.TemplateRenderContext {
	expiredAt := int64(0)
	if user.ExpiredAt != nil {
		expiredAt = *user.ExpiredAt
	}

	ctx := &model.TemplateRenderContext{
		UUID:           user.UUID,
		UserID:         user.ID,
		Email:          user.Email,
		ExpiredAt:      expiredAt,
		SpeedLimit:     user.GetSpeedLimit(),
		DeviceLimit:    user.GetDeviceLimit(),
		TransferEnable: user.TransferEnable,
		UsedTraffic:    user.U + user.D,
		Custom:         make(map[string]interface{}),
	}

	if req != nil {
		ctx.SubscribeURL = req.SubscribeURL
		ctx.SubscribeDomain = req.SubscribeDomain
	}

	// 如果用户没有设置限制，从套餐获取
	if user.Plan != nil {
		if ctx.SpeedLimit == 0 {
			ctx.SpeedLimit = user.Plan.GetSpeedLimit()
		}
		if ctx.DeviceLimit == 0 {
			ctx.DeviceLimit = user.Plan.GetDeviceLimit()
		}
	}

	return ctx
}

// renderTemplates 将模板渲染为 ParsedNode
func (s *SubscriptionService) renderTemplates(templates []*model.SubscriptionTemplate, ctx *model.TemplateRenderContext, groups []*model.SubscriptionGroup) []*model.ParsedNode {
	nodes := make([]*model.ParsedNode, 0, len(templates))

	// 创建分组 ID -> 名称映射
	groupNames := make(map[uint]string)
	for _, g := range groups {
		groupNames[g.ID] = g.Name
	}

	for _, tpl := range templates {
		node := s.renderTemplate(tpl, ctx)
		if node != nil {
			node.GroupID = tpl.GroupID
			node.GroupName = groupNames[tpl.GroupID]
			node.SourceType = "template"
			node.SourceID = tpl.ID
			node.SourceName = tpl.Name
			nodes = append(nodes, node)
		}
	}

	return nodes
}

// renderTemplate 渲染单个模板
// 将 SubscriptionTemplate 转换为 ParsedNode
func (s *SubscriptionService) renderTemplate(tpl *model.SubscriptionTemplate, ctx *model.TemplateRenderContext) *model.ParsedNode {
	// 渲染模板名称
	name := s.renderString(tpl.Name, ctx)

	// 基础节点
	node := &model.ParsedNode{
		ID:        s.generateNodeID(tpl),
		Name:      name,
		Type:      tpl.Type,
		Server:    tpl.Server,
		Port:      tpl.Port,
		UUID:      ctx.UUID,
		Password:  ctx.UUID, // 默认使用 UUID 作为密码
		TLSMode:   tpl.TLS,
		TLS:       tpl.TLS > 0,
		Transport: tpl.Transport,
		Settings:  make(map[string]interface{}),
	}

	// TLS 配置
	if tpl.ServerName != nil {
		node.ServerName = *tpl.ServerName
	}
	if tpl.TLSFingerprint != nil {
		node.TLSFingerprint = *tpl.TLSFingerprint
	}
	if tpl.ALPN != nil {
		node.ALPN = *tpl.ALPN
	}

	// Reality 配置 (TLS=2)
	if tpl.TLS == 2 {
		if tpl.RealityPublicKey != nil {
			node.RealityPublicKey = *tpl.RealityPublicKey
		}
		if tpl.RealityShortID != nil {
			node.RealityShortID = *tpl.RealityShortID
		}
		if tpl.RealitySpiderX != nil {
			node.RealitySpiderX = *tpl.RealitySpiderX
		}
	}

	// VLESS 特有配置
	if tpl.Flow != nil && *tpl.Flow != "" {
		node.Flow = *tpl.Flow
	}
	if tpl.Encryption != nil && *tpl.Encryption != "" {
		node.Encryption = *tpl.Encryption
	}

	// Shadowsocks 配置
	if tpl.SSCipher != nil && *tpl.SSCipher != "" {
		node.Cipher = *tpl.SSCipher
	}
	if tpl.SSServerKey != nil && *tpl.SSServerKey != "" {
		node.ServerKey = *tpl.SSServerKey
	}

	// 传输层配置
	node.TransportSettings = tpl.GetTransportSettings()

	// 协议配置
	protocolSettings := tpl.GetProtocolSettings()
	for k, v := range protocolSettings {
		node.Settings[k] = v
	}

	// 如果有自定义模板 JSON，使用它覆盖
	if tpl.TemplateJSON != "" {
		s.applyTemplateJSON(node, tpl.TemplateJSON, ctx)
	}

	return node
}

// renderString 渲染字符串模板
func (s *SubscriptionService) renderString(str string, ctx *model.TemplateRenderContext) string {
	if !strings.Contains(str, "{{") {
		return str
	}

	tmpl, err := template.New("str").Parse(str)
	if err != nil {
		return str
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, ctx); err != nil {
		return str
	}

	return buf.String()
}

// applyTemplateJSON 应用自定义模板 JSON
func (s *SubscriptionService) applyTemplateJSON(node *model.ParsedNode, templateJSON string, ctx *model.TemplateRenderContext) {
	// 先渲染模板变量
	rendered := s.renderString(templateJSON, ctx)

	// 解析 JSON
	var override map[string]interface{}
	if err := json.Unmarshal([]byte(rendered), &override); err != nil {
		return
	}

	// 应用覆盖
	if name, ok := override["name"].(string); ok {
		node.Name = name
	}
	if server, ok := override["server"].(string); ok {
		node.Server = server
	}
	if port, ok := override["port"].(float64); ok {
		node.Port = int(port)
	}
	if uuid, ok := override["uuid"].(string); ok {
		node.UUID = uuid
	}
	if password, ok := override["password"].(string); ok {
		node.Password = password
	}
	if settings, ok := override["settings"].(map[string]interface{}); ok {
		for k, v := range settings {
			node.Settings[k] = v
		}
	}
}

// generateNodeID 生成节点 ID
func (s *SubscriptionService) generateNodeID(tpl *model.SubscriptionTemplate) string {
	data := fmt.Sprintf("%d:%s:%s:%d", tpl.ID, tpl.Type, tpl.Server, tpl.Port)
	hash := md5.Sum([]byte(data))
	return hex.EncodeToString(hash[:8])
}

// filterNodes 过滤节点
func (s *SubscriptionService) filterNodes(nodes []*model.ParsedNode, include, exclude string) []*model.ParsedNode {
	if include == "" && exclude == "" {
		return nodes
	}

	var result []*model.ParsedNode

	var includeRe, excludeRe *regexp.Regexp
	if include != "" {
		includeRe, _ = regexp.Compile(include)
	}
	if exclude != "" {
		excludeRe, _ = regexp.Compile(exclude)
	}

	for _, node := range nodes {
		// 检查排除规则
		if excludeRe != nil && excludeRe.MatchString(node.Name) {
			continue
		}

		// 检查包含规则
		if includeRe != nil && !includeRe.MatchString(node.Name) {
			continue
		}

		result = append(result, node)
	}

	return result
}

// getInternalNodes 获取内部节点 (从 Node 表)
func (s *SubscriptionService) getInternalNodes(user *model.User, ctx *model.TemplateRenderContext, groups []*model.SubscriptionGroup) ([]*model.ParsedNode, error) {
	if len(groups) == 0 {
		return nil, nil
	}

	groupIDs := make([]uint, len(groups))
	groupMap := make(map[uint]bool)
	for i, g := range groups {
		groupIDs[i] = g.ID
		groupMap[g.ID] = true
	}

	var protocols []model.NodeProtocol
	// 查询关联到这些分组的协议
	// 调试日志：打印正在查询的分组 ID
	fmt.Printf("[Subscription] Requesting internal nodes for groupIDs: %v\n", groupIDs)

	if err := s.db.Preload("Node").Preload("SubscriptionGroups").
		Joins("JOIN v2_subscription_group_node_protocols ON v2_subscription_group_node_protocols.node_protocol_id = v2_node_protocol.id").
		Where("v2_subscription_group_node_protocols.subscription_group_id IN ? AND v2_node_protocol.enable = 1", groupIDs).
		Find(&protocols).Error; err != nil {
		fmt.Printf("[Subscription] DB Query Error: %v\n", err)
		return nil, err
	}

	fmt.Printf("[Subscription] Found %d candidate protocols\n", len(protocols))

	var result []*model.ParsedNode
	for _, p := range protocols {
		if p.Node == nil {
			fmt.Printf("[Subscription] Protocol %d has no Node associated\n", p.ID)
			continue
		}

		fmt.Printf("[Subscription] Checking node %s (Online: %v, LastCheck: %v)\n", p.Node.Name, p.Node.IsOnline(), p.Node.LastCheckAt)

		// 调试期间放宽在线检查，先让东西出来
		// if !p.Node.IsOnline() { continue }

		// 遍历协议所属的所有分组
		for _, g := range p.SubscriptionGroups {
			// 只处理用户当前请求的分组
			if !groupMap[g.ID] {
				continue
			}

			parsed := s.nodeProtocolToParsedNode(p.Node, &p, ctx)
			if parsed != nil {
				parsed.GroupID = g.ID
				parsed.GroupName = g.Name
				result = append(result, parsed)
			}
		}
	}

	return result, nil
}

// nodeProtocolToParsedNode 将 NodeProtocol 转换为 ParsedNode
func (s *SubscriptionService) nodeProtocolToParsedNode(node *model.Node, protocol *model.NodeProtocol, ctx *model.TemplateRenderContext) *model.ParsedNode {
	host := node.Host
	if protocol.Host != nil && *protocol.Host != "" {
		host = *protocol.Host
	}

	parsed := &model.ParsedNode{
		ID:         fmt.Sprintf("node-%d-%d", node.ID, protocol.ID),
		Name:       fmt.Sprintf("%s - %s", node.Name, protocol.Name),
		Type:       string(protocol.Type),
		Server:     host,
		Port:       protocol.Port,
		UUID:       ctx.UUID,
		Password:   ctx.UUID,
		TLSMode:    protocol.TLS,
		TLS:        protocol.TLS > 0,
		SourceType: "node",
		SourceID:   node.ID,
		SourceName: node.Name,
		Settings:   make(map[string]interface{}),
	}

	// 设置分组信息
	if node.GroupID != nil {
		parsed.GroupID = *node.GroupID
		parsed.GroupName = ""
	}

	// ALPN
	if protocol.ALPN != nil {
		parsed.ALPN = *protocol.ALPN
	}

	// Reality
	if protocol.TLS == 2 && protocol.RealitySettings != nil {
		var realitySettings map[string]interface{}
		if err := json.Unmarshal([]byte(*protocol.RealitySettings), &realitySettings); err == nil {
			if pk, ok := realitySettings["public_key"].(string); ok {
				parsed.RealityPublicKey = pk
			}
			if pk, ok := realitySettings["pbk"].(string); ok && parsed.RealityPublicKey == "" {
				parsed.RealityPublicKey = pk
			}
			if sid, ok := realitySettings["short_id"].(string); ok {
				parsed.RealityShortID = sid
			}
			if sid, ok := realitySettings["sid"].(string); ok && parsed.RealityShortID == "" {
				parsed.RealityShortID = sid
			}
		}
	}

	// TLS Settings
	if protocol.TLSSettings != nil {
		var tlsSettings map[string]interface{}
		if err := json.Unmarshal([]byte(*protocol.TLSSettings), &tlsSettings); err == nil {
			if sni, ok := tlsSettings["server_name"].(string); ok {
				parsed.ServerName = sni
			}
			if fp, ok := tlsSettings["fingerprint"].(string); ok {
				parsed.TLSFingerprint = fp
			}
			if pk, ok := tlsSettings["public_key"].(string); ok && parsed.RealityPublicKey == "" {
				parsed.RealityPublicKey = pk
			}
		}
	}

	// 传输层
	if protocol.Transport != nil {
		parsed.Transport = *protocol.Transport
	}
	if protocol.TransportSettings != nil {
		var transportSettings map[string]interface{}
		if err := json.Unmarshal([]byte(*protocol.TransportSettings), &transportSettings); err == nil {
			parsed.TransportSettings = transportSettings
		}
	}

	// 协议配置
	if protocol.Settings != nil {
		var settings map[string]interface{}
		if err := json.Unmarshal([]byte(*protocol.Settings), &settings); err == nil {
			for k, v := range settings {
				parsed.Settings[k] = v
			}
		}
	}

	// Shadowsocks: support both "cipher" and "method" in protocol settings.
	if parsed.Type == string(model.ProtocolShadowsocks) {
		if c, ok := parsed.Settings["cipher"].(string); ok && c != "" {
			parsed.Cipher = c
		} else if m, ok := parsed.Settings["method"].(string); ok && m != "" {
			parsed.Cipher = m
		}
		if sk, ok := parsed.Settings["server_key"].(string); ok && sk != "" {
			parsed.ServerKey = sk
		}
	}

	return parsed
}

// ============ 管理员方法 ============

// CreateGroup 创建订阅分组
func (s *SubscriptionService) CreateGroup(group *model.SubscriptionGroup) error {
	return s.db.Create(group).Error
}

// UpdateGroup 更新订阅分组
func (s *SubscriptionService) UpdateGroup(group *model.SubscriptionGroup) error {
	return s.db.Save(group).Error
}

// DeleteGroup 删除订阅分组
func (s *SubscriptionService) DeleteGroup(id uint) error {
	// 先删除关联的模板
	s.db.Where("group_id = ?", id).Delete(&model.SubscriptionTemplate{})
	// 删除用户关联
	s.db.Where("group_id = ?", id).Delete(&model.UserSubscriptionGroup{})
	// 删除套餐关联
	s.db.Where("group_id = ?", id).Delete(&model.PlanSubscriptionGroup{})
	// 删除分组
	return s.db.Delete(&model.SubscriptionGroup{}, id).Error
}

// GetGroup 获取订阅分组
func (s *SubscriptionService) GetGroup(id uint) (*model.SubscriptionGroup, error) {
	var group model.SubscriptionGroup
	err := s.db.Preload("Templates").First(&group, id).Error
	return &group, err
}

// GetGroups 获取所有订阅分组
func (s *SubscriptionService) GetGroups() ([]*model.SubscriptionGroup, error) {
	var groups []*model.SubscriptionGroup
	err := s.db.Order("priority DESC, id ASC").Find(&groups).Error
	return groups, err
}

// CreateTemplate 创建订阅模板
func (s *SubscriptionService) CreateTemplate(template *model.SubscriptionTemplate) error {
	return s.db.Create(template).Error
}

// UpdateTemplate 更新订阅模板
func (s *SubscriptionService) UpdateTemplate(template *model.SubscriptionTemplate) error {
	return s.db.Save(template).Error
}

// UpdateTemplateFields 按字段局部更新订阅模板
func (s *SubscriptionService) UpdateTemplateFields(id uint, fields map[string]interface{}) error {
	if len(fields) == 0 {
		return nil
	}
	delete(fields, "id")
	delete(fields, "created_at")
	delete(fields, "updated_at")

	return s.db.Model(&model.SubscriptionTemplate{}).
		Where("id = ?", id).
		Updates(fields).Error
}

// DeleteTemplate 删除订阅模板
func (s *SubscriptionService) DeleteTemplate(id uint) error {
	return s.db.Delete(&model.SubscriptionTemplate{}, id).Error
}

// GetTemplate 获取订阅模板
func (s *SubscriptionService) GetTemplate(id uint) (*model.SubscriptionTemplate, error) {
	var template model.SubscriptionTemplate
	err := s.db.First(&template, id).Error
	return &template, err
}

// GetTemplatesByGroup 获取分组的模板
func (s *SubscriptionService) GetTemplatesByGroup(groupID uint) ([]*model.SubscriptionTemplate, error) {
	var templates []*model.SubscriptionTemplate
	err := s.db.Where("group_id = ?", groupID).Order("sort ASC, id ASC").Find(&templates).Error
	return templates, err
}

// AssignGroupToUser 为用户分配订阅分组
func (s *SubscriptionService) AssignGroupToUser(userID, groupID uint, expireAt *int64, transferEnable *int64, nextRenewPrice *int64) error {
	ug := model.UserSubscriptionGroup{
		UserID:         userID,
		GroupID:        groupID,
		ExpireAt:       expireAt,
		TransferEnable: transferEnable,
		NextRenewPrice: nextRenewPrice,
	}
	// Upsert (assign will update specified fields on conflict)
	return s.db.Where("user_id = ? AND group_id = ?", userID, groupID).
		Assign(ug).FirstOrCreate(&ug).Error
}

// RemoveGroupFromUser 移除用户的订阅分组
func (s *SubscriptionService) RemoveGroupFromUser(userID, groupID uint) error {
	return s.db.Where("user_id = ? AND group_id = ?", userID, groupID).
		Delete(&model.UserSubscriptionGroup{}).Error
}

// AssignGroupToPlan 为套餐分配订阅分组
func (s *SubscriptionService) AssignGroupToPlan(planID, groupID uint) error {
	pg := model.PlanSubscriptionGroup{
		PlanID:  planID,
		GroupID: groupID,
	}
	return s.db.Where("plan_id = ? AND group_id = ?", planID, groupID).
		FirstOrCreate(&pg).Error
}

// RemoveGroupFromPlan 移除套餐的订阅分组
func (s *SubscriptionService) RemoveGroupFromPlan(planID, groupID uint) error {
	return s.db.Where("plan_id = ? AND group_id = ?", planID, groupID).
		Delete(&model.PlanSubscriptionGroup{}).Error
}

// GetUserGroups 获取用户的订阅分组
func (s *SubscriptionService) GetUserGroups(userID uint) ([]*model.SubscriptionGroup, error) {
	var groups []*model.SubscriptionGroup
	now := time.Now().Unix()

	err := s.db.Joins("JOIN v2_user_subscription_group ON v2_subscription_group.id = v2_user_subscription_group.group_id").
		Where("v2_user_subscription_group.user_id = ? AND (v2_user_subscription_group.expire_at IS NULL OR v2_user_subscription_group.expire_at > ?)", userID, now).
		Find(&groups).Error

	return groups, err
}

// GetPlanGroups 获取套餐的订阅分组
func (s *SubscriptionService) GetPlanGroups(planID uint) ([]*model.SubscriptionGroup, error) {
	var groups []*model.SubscriptionGroup

	err := s.db.Joins("JOIN v2_plan_subscription_group ON v2_subscription_group.id = v2_plan_subscription_group.group_id").
		Where("v2_plan_subscription_group.plan_id = ?", planID).
		Find(&groups).Error

	return groups, err
}
