package handler

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/anixops/v2board/internal/model"
	"github.com/anixops/v2board/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/vmihailenco/msgpack/v5"
)

// UniProxyHandler UniProxy API处理器
type UniProxyHandler struct {
	serverService *service.ServerService
	userService   *service.UserService
	nodeService   *service.NodeService
}

// NewUniProxyHandler 创建UniProxy处理器
func NewUniProxyHandler() *UniProxyHandler {
	return &UniProxyHandler{
		serverService: service.NewServerService(),
		userService:   service.NewUserService(),
		nodeService:   service.NewNodeService(),
	}
}

// GetConfig 获取节点配置
// GET /api/v1/server/UniProxy/config
func (h *UniProxyHandler) GetConfig(c *gin.Context) {
	nodeType := c.Query("node_type")
	nodeIDStr := c.Query("node_id")

	nodeID, err := strconv.ParseUint(nodeIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid node_id"})
		return
	}

	var config map[string]interface{}

	// 如果没有指定 node_type，先尝试查询新版节点表
	if nodeType == "" {
		config, err = h.buildNewNodeConfig(uint(nodeID))
		if err == nil {
			h.sendConfigResponse(c, config)
			return
		}
		// 新版节点未找到，尝试旧版（依次尝试各种类型）
		for _, serverType := range []model.ServerType{model.ServerTypeVMess, model.ServerTypeVLESS, model.ServerTypeTrojan, model.ServerTypeShadowsocks} {
			config, err = h.serverService.BuildNodeConfig(serverType, uint(nodeID))
			if err == nil {
				h.sendConfigResponse(c, config)
				return
			}
		}
		c.JSON(http.StatusNotFound, gin.H{"error": "node not found"})
		return
	}

	// 指定了 node_type，使用旧版查询
	serverType := model.ServerType(nodeType)
	config, err = h.serverService.BuildNodeConfig(serverType, uint(nodeID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "node not found"})
		return
	}

	h.sendConfigResponse(c, config)
}

// buildNewNodeConfig 从新版节点表构建配置
// 配置优先级: RawConfig (原始JSON) > 协议配置 > 默认配置
func (h *UniProxyHandler) buildNewNodeConfig(nodeID uint) (map[string]interface{}, error) {
	node, err := h.nodeService.GetNode(nodeID)
	if err != nil {
		return nil, err
	}

	config := make(map[string]interface{})

	// 优先级1: 如果有 RawConfig，直接使用（管理员高级模式）
	if node.RawConfig != nil && *node.RawConfig != "" {
		if err := json.Unmarshal([]byte(*node.RawConfig), &config); err != nil {
			return nil, fmt.Errorf("invalid raw_config JSON: %v", err)
		}
		// 确保基础配置存在
		h.ensureBaseConfig(config, node)
		return config, nil
	}

	// 优先级2: 从协议配置构建
	protocols, _ := h.nodeService.GetProtocols(nodeID)
	if len(protocols) > 0 {
		// 使用第一个启用的协议配置
		var protocol *model.NodeProtocol
		for i := range protocols {
			if protocols[i].Enable == 1 {
				protocol = &protocols[i]
				break
			}
		}
		if protocol != nil {
			h.buildConfigFromProtocol(config, node, protocol)
			return config, nil
		}
	}

	// 优先级3: 无协议配置，返回基础节点信息（允许节点先连接）
	h.buildMinimalConfig(config, node)
	return config, nil
}

// ensureBaseConfig 确保基础配置存在
func (h *UniProxyHandler) ensureBaseConfig(config map[string]interface{}, node *model.Node) {
	// V2bX 必需字段
	if _, ok := config["node_type"]; !ok {
		config["node_type"] = "vless" // 默认类型
	}
	if _, ok := config["send_through"]; !ok {
		config["send_through"] = "0.0.0.0"
	}
	if _, ok := config["routes"]; !ok {
		config["routes"] = []interface{}{}
	}
	if _, ok := config["base_config"]; !ok {
		config["base_config"] = map[string]interface{}{
			"push_interval": 60,
			"pull_interval": 60,
		}
	}
}

// buildMinimalConfig 构建最小配置（无协议时）
func (h *UniProxyHandler) buildMinimalConfig(config map[string]interface{}, node *model.Node) {
	// V2bX 必需字段
	config["node_type"] = "vless" // 默认类型，管理员可通过 RawConfig 覆盖
	config["server_port"] = node.Port
	config["host"] = node.Host
	config["server_name"] = node.Host
	config["send_through"] = "0.0.0.0"
	config["routes"] = []interface{}{}
	config["base_config"] = map[string]interface{}{
		"push_interval": 60,
		"pull_interval": 60,
	}
	// 标记为无协议配置状态
	config["_no_protocol"] = true
}

// buildConfigFromProtocol 从协议配置构建
func (h *UniProxyHandler) buildConfigFromProtocol(config map[string]interface{}, node *model.Node, protocol *model.NodeProtocol) {
	// V2bX 必需字段：node_type (如果协议类型为空，使用默认值)
	nodeType := string(protocol.Type)
	if nodeType == "" {
		nodeType = "vless" // 默认类型
	}
	config["node_type"] = nodeType
	config["server_port"] = protocol.Port
	if protocol.Host != nil && *protocol.Host != "" {
		config["host"] = *protocol.Host
		config["server_name"] = *protocol.Host
	} else {
		config["host"] = node.Host
		config["server_name"] = node.Host
	}

	// 解析协议配置
	var protocolConfig map[string]interface{}
	if protocol.Settings != nil && *protocol.Settings != "" {
		json.Unmarshal([]byte(*protocol.Settings), &protocolConfig)
	}

	// TLS 配置
	config["tls"] = protocol.TLS
	if protocol.TLSSettings != nil && *protocol.TLSSettings != "" {
		var tlsSettings map[string]interface{}
		json.Unmarshal([]byte(*protocol.TLSSettings), &tlsSettings)
		config["tls_settings"] = tlsSettings
	}

	// 传输层配置
	if protocol.Transport != nil && *protocol.Transport != "" {
		config["network"] = *protocol.Transport
	} else {
		config["network"] = "tcp"
	}
	if protocol.TransportSettings != nil && *protocol.TransportSettings != "" {
		var transportSettings map[string]interface{}
		json.Unmarshal([]byte(*protocol.TransportSettings), &transportSettings)
		config["network_settings"] = transportSettings
	}

	// 针对特定内核的特殊处理
	switch protocol.Type {
	case "vmess":
		// VMess 不需要额外配置，大部分都在 Settings 中
	case "vless":
		config["flow"] = getConfigValue(protocolConfig, "flow", "")
	case "shadowsocks":
		config["cipher"] = getConfigValue(protocolConfig, "cipher", "aes-256-gcm")
		if serverKey, ok := protocolConfig["server_key"]; ok {
			config["server_key"] = serverKey
		}
	}

	// Reality 配置
	if protocol.TLS == 2 && protocol.RealitySettings != nil && *protocol.RealitySettings != "" {
		var realitySettings map[string]interface{}
		if err := json.Unmarshal([]byte(*protocol.RealitySettings), &realitySettings); err == nil {
			// V2bX 将 reality 配置放在 tls_settings 内部或者顶层，取决于内核
			// 这里我们参考主流做法，合并到 tls_settings 中
			if config["tls_settings"] == nil {
				config["tls_settings"] = make(map[string]interface{})
			}
			tlsSettings := config["tls_settings"].(map[string]interface{})
			for k, v := range realitySettings {
				tlsSettings[k] = v
			}
		}
	}

	// 优先级最高: 自定义全量配置 (如果设置了，将尝试合并或覆盖现有配置)
	if protocol.CustomConfig != nil && *protocol.CustomConfig != "" {
		var customConfig map[string]interface{}
		if err := json.Unmarshal([]byte(*protocol.CustomConfig), &customConfig); err == nil {
			for k, v := range customConfig {
				config[k] = v
			}
		}
	}

	// 添加基础配置
	config["send_through"] = "0.0.0.0"
	config["routes"] = []interface{}{}
	config["base_config"] = map[string]interface{}{
		"push_interval": 60,
		"pull_interval": 60,
	}
}

// sendConfigResponse 发送配置响应
func (h *UniProxyHandler) sendConfigResponse(c *gin.Context, config map[string]interface{}) {
	configJSON, _ := json.Marshal(config)
	etag := generateETag(configJSON)

	ifNoneMatch := c.GetHeader("If-None-Match")
	if ifNoneMatch == etag {
		c.Status(http.StatusNotModified)
		return
	}

	c.Header("ETag", etag)
	c.JSON(http.StatusOK, config)
}

// getConfigValue 从配置中获取值，如果不存在则返回默认值
func getConfigValue(config map[string]interface{}, key string, defaultValue interface{}) interface{} {
	if config == nil {
		return defaultValue
	}
	if val, ok := config[key]; ok {
		return val
	}
	return defaultValue
}

// GetUsers 获取用户列表
// GET /api/v1/server/UniProxy/user
func (h *UniProxyHandler) GetUsers(c *gin.Context) {
	nodeType := c.Query("node_type")
	nodeIDStr := c.Query("node_id")

	nodeID, err := strconv.ParseUint(nodeIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid node_id"})
		return
	}

	var users []*model.User

	// 如果没有指定 node_type，先尝试新版节点
	if nodeType == "" {
		// 新版节点：获取所有有效订阅的用户
		users, err = h.getNewNodeUsers(uint(nodeID))
		if err == nil {
			h.sendUsersResponse(c, users)
			return
		}
		// 回退到旧版（尝试各种类型）
		for _, serverType := range []model.ServerType{model.ServerTypeVMess, model.ServerTypeVLESS, model.ServerTypeTrojan, model.ServerTypeShadowsocks} {
			oldUsers, err := h.serverService.GetServerUsers(serverType, uint(nodeID))
			if err == nil && len(oldUsers) > 0 {
				// 转换为指针切片
				users = make([]*model.User, len(oldUsers))
				for i := range oldUsers {
					users[i] = &oldUsers[i]
				}
				h.sendUsersResponse(c, users)
				return
			}
		}
	} else {
		serverType := model.ServerType(nodeType)
		oldUsers, err := h.serverService.GetServerUsers(serverType, uint(nodeID))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get users"})
			return
		}
		// 转换为指针切片
		users = make([]*model.User, len(oldUsers))
		for i := range oldUsers {
			users[i] = &oldUsers[i]
		}
	}

	h.sendUsersResponse(c, users)
}

// getNewNodeUsers 获取新版节点的用户
func (h *UniProxyHandler) getNewNodeUsers(nodeID uint) ([]*model.User, error) {
	// 验证节点存在
	node, err := h.nodeService.GetNode(nodeID)
	if err != nil {
		return nil, err
	}

	// 获取所有有效用户（已付费且未过期）
	users, err := h.userService.GetActiveUsersForNode(node.GroupID)
	if err != nil {
		return nil, err
	}

	return users, nil
}

// sendUsersResponse 发送用户列表响应
func (h *UniProxyHandler) sendUsersResponse(c *gin.Context, users []*model.User) {
	userList := make([]map[string]interface{}, 0, len(users))
	for _, user := range users {
		speedLimit := user.GetSpeedLimit()
		deviceLimit := user.GetDeviceLimit()

		// 如果用户没有设置，则从套餐获取
		if speedLimit == 0 && user.Plan != nil {
			speedLimit = user.Plan.GetSpeedLimit()
		}
		if deviceLimit == 0 && user.Plan != nil {
			deviceLimit = user.Plan.GetDeviceLimit()
		}

		userList = append(userList, map[string]interface{}{
			"id":           user.ID,
			"uuid":         user.UUID,
			"speed_limit":  speedLimit,
			"device_limit": deviceLimit,
		})
	}

	response := map[string]interface{}{
		"users": userList,
	}

	// 生成ETag
	responseJSON, _ := json.Marshal(response)
	etag := generateETag(responseJSON)

	// 检查ETag
	ifNoneMatch := c.GetHeader("If-None-Match")
	if ifNoneMatch == etag {
		c.Status(http.StatusNotModified)
		return
	}

	c.Header("ETag", etag)

	// 检查响应格式
	responseFormat := c.GetHeader("X-Response-Format")
	if responseFormat == "msgpack" {
		msgpackData, err := msgpack.Marshal(response)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to encode msgpack"})
			return
		}
		c.Data(http.StatusOK, "application/x-msgpack", msgpackData)
		return
	}

	c.JSON(http.StatusOK, response)
}

// GetAliveList 获取用户在线状态
// GET /api/v1/server/UniProxy/alivelist
func (h *UniProxyHandler) GetAliveList(c *gin.Context) {
	aliveMap, err := h.serverService.GetAllUsersOnlineCount()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get alive list"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"alive": aliveMap,
	})
}

// PushTraffic 上报用户流量
// POST /api/v1/server/UniProxy/push
func (h *UniProxyHandler) PushTraffic(c *gin.Context) {
	nodeType := c.Query("node_type")
	nodeIDStr := c.Query("node_id")

	nodeID, err := strconv.ParseUint(nodeIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid node_id"})
		return
	}

	// 更新节点心跳时间 (新版节点)
	if nodeType == "" {
		h.nodeService.UpdateLastCheckAt(uint(nodeID))
	}

	serverType := model.ServerType(nodeType)

	// 解析请求体
	var trafficData map[string]interface{}
	if err := c.ShouldBindJSON(&trafficData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	// 解析流量数据
	traffics, err := service.ParseTrafficData(trafficData)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid traffic data"})
		return
	}

	if len(traffics) == 0 {
		c.JSON(http.StatusOK, gin.H{"status": "success"})
		return
	}

	// 获取服务器流量倍率
	rate := h.serverService.GetServerRate(serverType, uint(nodeID))

	// 记录流量日志
	if err := h.serverService.BatchRecordTrafficLog(serverType, uint(nodeID), traffics, rate); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to record traffic log"})
		return
	}

	// 更新用户流量（按倍率计算）
	userTraffics := make(map[uint][2]int64)
	for userID, traffic := range traffics {
		userTraffics[userID] = [2]int64{
			int64(float64(traffic[0]) * rate),
			int64(float64(traffic[1]) * rate),
		}
	}

	if err := h.userService.BatchUpdateTraffic(userTraffics); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update user traffic"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

// PushAlive 上报用户在线状态
// POST /api/v1/server/UniProxy/alive
func (h *UniProxyHandler) PushAlive(c *gin.Context) {
	nodeType := c.Query("node_type")
	nodeIDStr := c.Query("node_id")

	nodeID, err := strconv.ParseUint(nodeIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid node_id"})
		return
	}

	// 更新节点心跳时间 (新版节点)
	if nodeType == "" {
		h.nodeService.UpdateLastCheckAt(uint(nodeID))
	}

	serverType := model.ServerType(nodeType)

	// 解析请求体
	var onlineData map[string]interface{}
	if err := c.ShouldBindJSON(&onlineData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	// 解析在线数据
	userIPs, err := service.ParseOnlineData(onlineData)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid online data"})
		return
	}

	// 更新在线状态
	if err := h.serverService.UpdateOnlineStatus(serverType, uint(nodeID), userIPs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update online status"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

// generateETag 生成ETag
func generateETag(data []byte) string {
	hash := md5.Sum(data)
	return hex.EncodeToString(hash[:])
}
