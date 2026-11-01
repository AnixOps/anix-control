package service

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
)

// BuildNodeRuntimeConfig builds the runtime configuration of a proxy node
// for the node type given, "" for none: the node's raw configuration when it
// has one, with the base fields filled in; else the enabled protocol of
// that type (the first enabled protocol for none) through
// BuildNodeProtocolConfig; else, for none, a minimal VLESS configuration.
// The desired configuration of the Agent Control stream carries it
// (kernelnodeops, node-ops-service.md section 5.5). gorm.ErrRecordNotFound
// when the node does not exist.
func BuildNodeRuntimeConfig(db *gorm.DB, nodeID uint, preferredType string) (map[string]any, error) {
	return (&NodeService{db: db}).BuildRuntimeConfig(nodeID, preferredType)
}

// BuildRuntimeConfig is BuildNodeRuntimeConfig over the service's
// database.
func (s *NodeService) BuildRuntimeConfig(nodeID uint, preferredType string) (map[string]any, error) {
	node, err := s.GetNode(nodeID)
	if err != nil {
		return nil, err
	}

	preferredType = NormalizeNodeType(preferredType)
	config := make(map[string]any)

	if node.RawConfig != nil && *node.RawConfig != "" {
		s.PrepareNodeRawConfig(node)
		if err := json.Unmarshal([]byte(*node.RawConfig), &config); err != nil {
			return nil, fmt.Errorf("invalid raw_config JSON: %v", err)
		}
		if config == nil {
			return nil, errors.New("raw_config must be a JSON object")
		}
		if preferredType != "" {
			rawType := NormalizeNodeType(configString(config["node_type"]))
			if rawType == "" {
				rawType = NormalizeNodeType(configString(config["type"]))
			}
			if rawType != preferredType {
				return nil, fmt.Errorf("raw_config protocol %q does not match requested protocol %q", rawType, preferredType)
			}
		}
		if err := ValidateWireGuardRuntimeConfig(config); err != nil {
			return nil, err
		}
		ensureRuntimeBaseConfig(config)
		return completeRuntimeConfig(config), nil
	}

	protocols, _ := s.GetProtocols(nodeID)
	if len(protocols) > 0 {
		if protocol := selectRuntimeProtocol(protocols, preferredType); protocol != nil {
			for k, v := range BuildNodeProtocolConfig(node, protocol) {
				config[k] = v
			}
			return completeRuntimeConfig(config), nil
		}
	}
	if preferredType != "" {
		return nil, fmt.Errorf("protocol %s not found for node %d", preferredType, nodeID)
	}
	buildMinimalRuntimeConfig(config, node)
	return completeRuntimeConfig(config), nil
}

// ErrProtocolNotServed reports a protocol the node does not serve on its
// own: it is not one of the node's enabled protocols, or the node has a raw
// configuration, which it runs instead of its protocols.
var ErrProtocolNotServed = errors.New("the node does not serve this protocol on its own")

// BuildNodeRuntimeProtocolConfig builds the runtime configuration of one
// of a proxy node's enabled protocols: what BuildNodeRuntimeConfig builds
// for that protocol's type when it is the first of its type. A node runs each of its protocols this way, also two
// of one type (shadowsocks and shadowsocks 2022 on two ports).
// ErrProtocolNotServed when the node has a raw configuration or no
// such enabled protocol; gorm.ErrRecordNotFound when the node does not
// exist.
func BuildNodeRuntimeProtocolConfig(db *gorm.DB, nodeID, protocolID uint) (map[string]any, error) {
	return (&NodeService{db: db}).BuildRuntimeProtocolConfig(nodeID, protocolID)
}

// BuildRuntimeProtocolConfig is BuildNodeRuntimeProtocolConfig over the
// service's database.
func (s *NodeService) BuildRuntimeProtocolConfig(nodeID, protocolID uint) (map[string]any, error) {
	node, err := s.GetNode(nodeID)
	if err != nil {
		return nil, err
	}
	if node.RawConfig != nil && *node.RawConfig != "" {
		return nil, ErrProtocolNotServed
	}
	protocols, err := s.GetProtocols(nodeID)
	if err != nil {
		return nil, err
	}
	for i := range protocols {
		if protocols[i].ID != protocolID || protocols[i].Enable != 1 {
			continue
		}
		config := make(map[string]any)
		for k, v := range BuildNodeProtocolConfig(node, &protocols[i]) {
			config[k] = v
		}
		return completeRuntimeConfig(config), nil
	}
	return nil, ErrProtocolNotServed
}

// completeRuntimeConfig sets type from node_type when it is missing.
func completeRuntimeConfig(config map[string]any) map[string]any {
	if _, ok := config["type"]; !ok {
		if nodeType, ok := config["node_type"]; ok {
			config["type"] = nodeType
		}
	}
	return config
}

func ensureRuntimeBaseConfig(config map[string]any) {
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

func buildMinimalRuntimeConfig(config map[string]any, node *model.Node) {
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

func selectRuntimeProtocol(protocols []model.NodeProtocol, preferredType string) *model.NodeProtocol {
	for i := range protocols {
		if protocols[i].Enable != 1 {
			continue
		}
		if preferredType == "" || NormalizeNodeType(string(protocols[i].Type)) == preferredType {
			return &protocols[i]
		}
	}
	return nil
}

func configString(value any) string {
	text, _ := value.(string)
	return text
}
