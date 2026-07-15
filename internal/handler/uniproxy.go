package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/AnixOps/anix-control/v3/internal/model"
	"github.com/AnixOps/anix-control/v3/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/vmihailenco/msgpack/v5"
)

// UniProxyHandler handles node polling APIs.
type UniProxyHandler struct {
	serverService       *service.ServerService
	userService         *service.UserService
	nodeService         *service.NodeService
	subscriptionService *service.SubscriptionService
}

func NewUniProxyHandler() *UniProxyHandler {
	return &UniProxyHandler{
		serverService:       service.NewServerService(),
		userService:         service.NewUserService(),
		nodeService:         service.NewNodeService(),
		subscriptionService: service.NewSubscriptionService(),
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

	var config map[string]any

	if nodeType == "" {
		config, err = h.buildNewNodeConfig(uint(nodeID), "")
		if err == nil {
			_ = h.nodeService.UpdateLastCheckAt(uint(nodeID))
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
				_ = h.nodeService.UpdateLastCheckAt(uint(nodeID))
				h.sendConfigResponse(c, config)
				return
			}
		}

		c.JSON(http.StatusNotFound, gin.H{"error": "node not found"})
		return
	}

	config, err = h.buildNewNodeConfig(uint(nodeID), nodeType)
	if err == nil {
		_ = h.nodeService.UpdateLastCheckAt(uint(nodeID))
		h.sendConfigResponse(c, config)
		return
	}
	if nodeType == string(model.ProtocolWireGuard) {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	serverType := model.ServerType(nodeType)
	config, err = h.serverService.BuildNodeConfig(serverType, uint(nodeID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "node not found"})
		return
	}

	_ = h.nodeService.UpdateLastCheckAt(uint(nodeID))
	h.sendConfigResponse(c, config)
}

func (h *UniProxyHandler) buildNewNodeConfig(nodeID uint, preferredType string) (map[string]any, error) {
	node, err := h.nodeService.GetNode(nodeID)
	if err != nil {
		return nil, err
	}

	preferredType = normalizeNodeType(preferredType)
	config := make(map[string]any)

	if node.RawConfig != nil && *node.RawConfig != "" {
		if err := json.Unmarshal([]byte(*node.RawConfig), &config); err != nil {
			return nil, fmt.Errorf("invalid raw_config JSON: %v", err)
		}
		if config == nil {
			return nil, fmt.Errorf("raw_config must be a JSON object")
		}
		if preferredType != "" {
			rawType := normalizeNodeType(stringValue(config["node_type"]))
			if rawType == "" {
				rawType = normalizeNodeType(stringValue(config["type"]))
			}
			if rawType != preferredType {
				return nil, fmt.Errorf("raw_config protocol %q does not match requested protocol %q", rawType, preferredType)
			}
		}
		if err := service.ValidateWireGuardRuntimeConfig(config); err != nil {
			return nil, err
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

func (h *UniProxyHandler) ensureBaseConfig(config map[string]any) {
	if _, ok := config["node_type"]; !ok {
		if protocolType, exists := config["type"]; exists {
			config["node_type"] = protocolType
		} else {
			config["node_type"] = "vless"
		}
	}
	if _, ok := config["type"]; !ok {
		config["type"] = config["node_type"]
	}
	if _, ok := config["send_through"]; !ok {
		config["send_through"] = "0.0.0.0"
	}
	if _, ok := config["routes"]; !ok {
		config["routes"] = []any{}
	}
	if _, ok := config["base_config"]; !ok {
		config["base_config"] = map[string]any{
			"push_interval": 60,
			"pull_interval": 60,
		}
	}
}

func (h *UniProxyHandler) buildMinimalConfig(config map[string]any, node *model.Node) {
	config["node_type"] = "vless"
	config["type"] = "vless"
	config["server_port"] = node.Port
	config["host"] = node.Host
	config["server_name"] = node.Host
	config["send_through"] = "0.0.0.0"
	config["routes"] = []any{}
	config["base_config"] = map[string]any{
		"push_interval": 60,
		"pull_interval": 60,
	}
	config["_no_protocol"] = true
}

func (h *UniProxyHandler) buildConfigFromProtocol(config map[string]any, node *model.Node, protocol *model.NodeProtocol) {
	// 配置构建的唯一真源在 service.BuildNodeProtocolConfig, gRPC 也走同一套,
	// 避免 SS2022 server_key / reality / tls_settings 等字段两处不一致。
	for k, v := range service.BuildNodeProtocolConfig(node, protocol) {
		config[k] = v
	}
}

func (h *UniProxyHandler) sendConfigResponse(c *gin.Context, config map[string]any) {
	if _, ok := config["type"]; !ok {
		if nt, ok := config["node_type"]; ok {
			config["type"] = nt
		}
	}

	configJSON, err := json.Marshal(config)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to encode config"})
		return
	}
	etag := generateETag(configJSON)
	ifNoneMatch := c.GetHeader("If-None-Match")
	if ifNoneMatch == etag {
		c.Status(http.StatusNotModified)
		return
	}

	c.Header("ETag", etag)
	c.JSON(http.StatusOK, config)
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

func stringValue(value any) string {
	text, _ := value.(string)
	return text
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
		_ = h.nodeService.UpdateLastCheckAt(uint(nodeID))
		h.sendUsersResponse(c, users, uint(nodeID), nodeType)
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
				_ = h.nodeService.UpdateLastCheckAt(uint(nodeID))
				h.sendUsersResponse(c, users, uint(nodeID), nodeType)
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

	_ = h.nodeService.UpdateLastCheckAt(uint(nodeID))
	h.sendUsersResponse(c, users, uint(nodeID), nodeType)
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

func (h *UniProxyHandler) sendUsersResponse(c *gin.Context, users []*model.User, nodeID uint, nodeType string) {
	wireGuardExtras, wireGuardExit, err := h.buildWireGuardUserExtras(nodeID, nodeType, users)
	if err != nil {
		if nodeType == string(model.ProtocolWireGuard) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to build wireguard users"})
		}
		return
	}
	if wireGuardExit {
		users = nil
	}

	userList := make([]map[string]any, 0, len(users))
	for _, user := range users {
		speedLimit := user.GetSpeedLimit()
		deviceLimit := user.GetDeviceLimit()

		if speedLimit == 0 && user.Plan != nil {
			speedLimit = user.Plan.GetSpeedLimit()
		}
		if deviceLimit == 0 && user.Plan != nil {
			deviceLimit = user.Plan.GetDeviceLimit()
		}

		item := map[string]any{
			"id":           user.ID,
			"uuid":         user.UUID,
			"speed_limit":  speedLimit,
			"device_limit": deviceLimit,
		}
		if extra, ok := wireGuardExtras[user.ID]; ok {
			item["wireguard_peer_ip"] = extra["wireguard_peer_ip"]
			item["wireguard_public_key"] = extra["wireguard_public_key"]
			item["wireguard_preshared_key"] = extra["wireguard_preshared_key"]
			item["wireguard_protocol_id"] = extra["wireguard_protocol_id"]
			item["extra"] = extra
		}
		userList = append(userList, item)
	}

	response := map[string]any{
		"users": userList,
	}

	responseJSON, err := json.Marshal(response)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to encode response"})
		return
	}
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

func (h *UniProxyHandler) buildWireGuardUserExtras(nodeID uint, nodeType string, users []*model.User) (map[uint]map[string]string, bool, error) {
	if nodeID == 0 || len(users) == 0 {
		return nil, false, nil
	}
	protocol, err := h.selectRuntimeProtocol(nodeID, nodeType)
	if err != nil || protocol == nil || protocol.Type != model.ProtocolWireGuard {
		return nil, false, err
	}
	if service.IsWireGuardExitProtocol(protocol) {
		return nil, true, nil
	}
	extras, err := h.subscriptionService.BuildWireGuardRuntimeUserExtras(protocol, users)
	return extras, false, err
}

func (h *UniProxyHandler) selectRuntimeProtocol(nodeID uint, preferredType string) (*model.NodeProtocol, error) {
	protocols, err := h.nodeService.GetProtocols(nodeID)
	if err != nil {
		return nil, err
	}
	preferredType = normalizeNodeType(preferredType)
	protocol := selectNodeProtocol(protocols, preferredType)
	if protocol == nil && preferredType != "" {
		return nil, fmt.Errorf("protocol %s not found for node %d", preferredType, nodeID)
	}
	return protocol, nil
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

	_ = h.nodeService.UpdateLastCheckAt(uint(nodeID))

	serverType := model.ServerType(nodeType)

	var trafficData map[string]any
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
	if err := h.serverService.RecordNodeTrafficReport(serverType, uint(nodeID), traffics, rate); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to record traffic report"})
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

	_ = h.nodeService.UpdateLastCheckAt(uint(nodeID))

	serverType := model.ServerType(nodeType)

	var onlineData map[string]any
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
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}
