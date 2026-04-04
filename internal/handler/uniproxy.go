package handler

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/anixops/v2board/internal/model"
	"github.com/anixops/v2board/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/vmihailenco/msgpack/v5"
)

// UniProxyHandler handles node polling APIs.
type UniProxyHandler struct {
	serverService *service.ServerService
	userService   *service.UserService
	nodeService   *service.NodeService
}

func NewUniProxyHandler() *UniProxyHandler {
	return &UniProxyHandler{
		serverService: service.NewServerService(),
		userService:   service.NewUserService(),
		nodeService:   service.NewNodeService(),
	}
}

// GetConfig returns node config.
// GET /api/v1/server/UniProxy/config
func (h *UniProxyHandler) GetConfig(c *gin.Context) {
	nodeType := normalizeNodeType(c.Query("node_type"))
	nodeIDStr := c.Query("node_id")

	nodeID, err := strconv.ParseUint(nodeIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid node_id"})
		return
	}

	var config map[string]interface{}

	if nodeType == "" {
		config, err = h.buildNewNodeConfig(uint(nodeID), "")
		if err == nil {
			h.sendConfigResponse(c, config)
			return
		}

		for _, serverType := range []model.ServerType{
			model.ServerTypeVMess,
			model.ServerTypeVLESS,
			model.ServerTypeTrojan,
			model.ServerTypeShadowsocks,
		} {
			config, err = h.serverService.BuildNodeConfig(serverType, uint(nodeID))
			if err == nil {
				h.sendConfigResponse(c, config)
				return
			}
		}

		c.JSON(http.StatusNotFound, gin.H{"error": "node not found"})
		return
	}

	config, err = h.buildNewNodeConfig(uint(nodeID), nodeType)
	if err == nil {
		h.sendConfigResponse(c, config)
		return
	}

	serverType := model.ServerType(nodeType)
	config, err = h.serverService.BuildNodeConfig(serverType, uint(nodeID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "node not found"})
		return
	}

	h.sendConfigResponse(c, config)
}

func (h *UniProxyHandler) buildNewNodeConfig(nodeID uint, preferredType string) (map[string]interface{}, error) {
	node, err := h.nodeService.GetNode(nodeID)
	if err != nil {
		return nil, err
	}

	preferredType = normalizeNodeType(preferredType)
	config := make(map[string]interface{})

	if node.RawConfig != nil && *node.RawConfig != "" {
		if err := json.Unmarshal([]byte(*node.RawConfig), &config); err != nil {
			return nil, fmt.Errorf("invalid raw_config JSON: %v", err)
		}
		h.ensureBaseConfig(config)
		return config, nil
	}

	protocols, _ := h.nodeService.GetProtocols(nodeID)
	if len(protocols) > 0 {
		protocol := selectNodeProtocol(protocols, preferredType)
		if protocol != nil {
			h.buildConfigFromProtocol(config, node, protocol)
			return config, nil
		}
		if preferredType != "" {
			return nil, fmt.Errorf("protocol %s not found for node %d", preferredType, nodeID)
		}
	}

	if preferredType != "" {
		return nil, fmt.Errorf("protocol %s not found for node %d", preferredType, nodeID)
	}

	h.buildMinimalConfig(config, node)
	return config, nil
}

func (h *UniProxyHandler) ensureBaseConfig(config map[string]interface{}) {
	if _, ok := config["node_type"]; !ok {
		config["node_type"] = "vless"
	}
	if _, ok := config["type"]; !ok {
		config["type"] = config["node_type"]
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

func (h *UniProxyHandler) buildMinimalConfig(config map[string]interface{}, node *model.Node) {
	config["node_type"] = "vless"
	config["type"] = "vless"
	config["server_port"] = node.Port
	config["host"] = node.Host
	config["server_name"] = node.Host
	config["send_through"] = "0.0.0.0"
	config["routes"] = []interface{}{}
	config["base_config"] = map[string]interface{}{
		"push_interval": 60,
		"pull_interval": 60,
	}
	config["_no_protocol"] = true
}

func (h *UniProxyHandler) buildConfigFromProtocol(config map[string]interface{}, node *model.Node, protocol *model.NodeProtocol) {
	nodeType := normalizeNodeType(string(protocol.Type))
	if nodeType == "" {
		nodeType = "vless"
	}

	config["node_type"] = nodeType
	config["type"] = nodeType
	config["server_port"] = protocol.Port

	if protocol.Host != nil && *protocol.Host != "" {
		config["host"] = *protocol.Host
		config["server_name"] = *protocol.Host
	} else {
		config["host"] = node.Host
		config["server_name"] = node.Host
	}

	var protocolConfig map[string]interface{}
	if protocol.Settings != nil && *protocol.Settings != "" {
		_ = json.Unmarshal([]byte(*protocol.Settings), &protocolConfig)
	}

	config["tls"] = protocol.TLS
	if protocol.TLSSettings != nil && *protocol.TLSSettings != "" {
		var tlsSettings map[string]interface{}
		_ = json.Unmarshal([]byte(*protocol.TLSSettings), &tlsSettings)
		config["tls_settings"] = tlsSettings
	}

	if protocol.Transport != nil && *protocol.Transport != "" {
		config["network"] = *protocol.Transport
	} else {
		config["network"] = "tcp"
	}
	if protocol.TransportSettings != nil && *protocol.TransportSettings != "" {
		var transportSettings map[string]interface{}
		_ = json.Unmarshal([]byte(*protocol.TransportSettings), &transportSettings)
		config["network_settings"] = transportSettings
	}

	switch nodeType {
	case "vless":
		config["flow"] = getConfigValue(protocolConfig, "flow", "")
	case "shadowsocks":
		cipher := getConfigValue(protocolConfig, "cipher", "")
		if s, ok := cipher.(string); ok && s != "" {
			config["cipher"] = s
		} else if method, ok := protocolConfig["method"]; ok {
			config["cipher"] = method
		} else {
			config["cipher"] = "aes-256-gcm"
		}
		if serverKey, ok := protocolConfig["server_key"]; ok {
			config["server_key"] = serverKey
		}
	}

	if protocol.TLS == 2 && protocol.RealitySettings != nil && *protocol.RealitySettings != "" {
		var realitySettings map[string]interface{}
		if err := json.Unmarshal([]byte(*protocol.RealitySettings), &realitySettings); err == nil {
			if config["tls_settings"] == nil {
				config["tls_settings"] = make(map[string]interface{})
			}
			tlsSettings, _ := config["tls_settings"].(map[string]interface{})
			if tlsSettings == nil {
				tlsSettings = make(map[string]interface{})
				config["tls_settings"] = tlsSettings
			}
			for k, v := range realitySettings {
				tlsSettings[k] = v
			}
		}
	}

	if protocol.CustomConfig != nil && *protocol.CustomConfig != "" {
		var customConfig map[string]interface{}
		if err := json.Unmarshal([]byte(*protocol.CustomConfig), &customConfig); err == nil {
			for k, v := range customConfig {
				config[k] = v
			}
		}
	}

	config["send_through"] = "0.0.0.0"
	config["routes"] = []interface{}{}
	config["base_config"] = map[string]interface{}{
		"push_interval": 60,
		"pull_interval": 60,
	}
}

func (h *UniProxyHandler) sendConfigResponse(c *gin.Context, config map[string]interface{}) {
	if _, ok := config["type"]; !ok {
		if nt, ok := config["node_type"]; ok {
			config["type"] = nt
		}
	}

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

func getConfigValue(config map[string]interface{}, key string, defaultValue interface{}) interface{} {
	if config == nil {
		return defaultValue
	}
	if val, ok := config[key]; ok {
		return val
	}
	return defaultValue
}

func selectNodeProtocol(protocols []model.NodeProtocol, preferredType string) *model.NodeProtocol {
	for i := range protocols {
		if protocols[i].Enable != 1 {
			continue
		}
		if preferredType == "" {
			return &protocols[i]
		}
		if normalizeNodeType(string(protocols[i].Type)) == preferredType {
			return &protocols[i]
		}
	}
	return nil
}

func normalizeNodeType(nodeType string) string {
	t := strings.ToLower(strings.TrimSpace(nodeType))
	switch t {
	case "v2ray", "vmess-aead", "vmessaead":
		return "vmess"
	default:
		return t
	}
}

// GetUsers returns active users.
// GET /api/v1/server/UniProxy/user
func (h *UniProxyHandler) GetUsers(c *gin.Context) {
	nodeType := normalizeNodeType(c.Query("node_type"))
	nodeIDStr := c.Query("node_id")

	nodeID, err := strconv.ParseUint(nodeIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid node_id"})
		return
	}

	users, err := h.getNewNodeUsers(uint(nodeID))
	if err == nil {
		h.sendUsersResponse(c, users)
		return
	}

	if nodeType == "" {
		for _, serverType := range []model.ServerType{
			model.ServerTypeVMess,
			model.ServerTypeVLESS,
			model.ServerTypeTrojan,
			model.ServerTypeShadowsocks,
		} {
			oldUsers, getErr := h.serverService.GetServerUsers(serverType, uint(nodeID))
			if getErr == nil && len(oldUsers) > 0 {
				users = make([]*model.User, len(oldUsers))
				for i := range oldUsers {
					users[i] = &oldUsers[i]
				}
				h.sendUsersResponse(c, users)
				return
			}
		}
	} else {
		oldUsers, getErr := h.serverService.GetServerUsers(model.ServerType(nodeType), uint(nodeID))
		if getErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get users"})
			return
		}
		users = make([]*model.User, len(oldUsers))
		for i := range oldUsers {
			users[i] = &oldUsers[i]
		}
	}

	h.sendUsersResponse(c, users)
}

func (h *UniProxyHandler) getNewNodeUsers(nodeID uint) ([]*model.User, error) {
	node, err := h.nodeService.GetNode(nodeID)
	if err != nil {
		return nil, err
	}

	users, err := h.userService.GetActiveUsersForNode(node.GroupID)
	if err != nil {
		return nil, err
	}

	return users, nil
}

func (h *UniProxyHandler) sendUsersResponse(c *gin.Context, users []*model.User) {
	userList := make([]map[string]interface{}, 0, len(users))
	for _, user := range users {
		speedLimit := user.GetSpeedLimit()
		deviceLimit := user.GetDeviceLimit()

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

	responseJSON, _ := json.Marshal(response)
	etag := generateETag(responseJSON)
	ifNoneMatch := c.GetHeader("If-None-Match")
	if ifNoneMatch == etag {
		c.Status(http.StatusNotModified)
		return
	}

	c.Header("ETag", etag)

	if c.GetHeader("X-Response-Format") == "msgpack" {
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

// GetAliveList returns online counts.
// GET /api/v1/server/UniProxy/alivelist
func (h *UniProxyHandler) GetAliveList(c *gin.Context) {
	aliveMap, err := h.serverService.GetAllUsersOnlineCount()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get alive list"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"alive": aliveMap})
}

// PushTraffic receives traffic report.
// POST /api/v1/server/UniProxy/push
func (h *UniProxyHandler) PushTraffic(c *gin.Context) {
	nodeType := normalizeNodeType(c.Query("node_type"))
	nodeIDStr := c.Query("node_id")

	nodeID, err := strconv.ParseUint(nodeIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid node_id"})
		return
	}

	if nodeType == "" {
		_ = h.nodeService.UpdateLastCheckAt(uint(nodeID))
	}

	serverType := model.ServerType(nodeType)

	var trafficData map[string]interface{}
	if err := c.ShouldBindJSON(&trafficData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	traffics, err := service.ParseTrafficData(trafficData)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid traffic data"})
		return
	}

	if len(traffics) == 0 {
		c.JSON(http.StatusOK, gin.H{"status": "success"})
		return
	}

	rate := h.serverService.GetServerRate(serverType, uint(nodeID))
	if err := h.serverService.BatchRecordTrafficLog(serverType, uint(nodeID), traffics, rate); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to record traffic log"})
		return
	}

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

// PushAlive receives online IP report.
// POST /api/v1/server/UniProxy/alive
func (h *UniProxyHandler) PushAlive(c *gin.Context) {
	nodeType := normalizeNodeType(c.Query("node_type"))
	nodeIDStr := c.Query("node_id")

	nodeID, err := strconv.ParseUint(nodeIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid node_id"})
		return
	}

	if nodeType == "" {
		_ = h.nodeService.UpdateLastCheckAt(uint(nodeID))
	}

	serverType := model.ServerType(nodeType)

	var onlineData map[string]interface{}
	if err := c.ShouldBindJSON(&onlineData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	userIPs, err := service.ParseOnlineData(onlineData)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid online data"})
		return
	}

	if err := h.serverService.UpdateOnlineStatus(serverType, uint(nodeID), userIPs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update online status"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

func generateETag(data []byte) string {
	hash := md5.Sum(data)
	return hex.EncodeToString(hash[:])
}
