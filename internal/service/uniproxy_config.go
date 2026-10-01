package service

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
)

// BuildUniProxyNodeConfig builds the configuration UniProxy answers a proxy
// node (GET /api/v1/server/UniProxy/config, and the /api/v2 alias) for the
// node type it asks for, "" for none: the node's raw configuration when it
// has one, with the base fields filled in; else the enabled protocol of
// that type (the first enabled protocol for none) through
// BuildNodeProtocolConfig; else, for none, a minimal VLESS configuration.
// The answer is the map UniProxy encodes, field for field. The desired
// configuration of the Agent Control stream carries it too
// (kernelnodeops, node-ops-service.md section 5.5), so a node gets the
// same configuration on either transport. gorm.ErrRecordNotFound when the
// node does not exist.
func BuildUniProxyNodeConfig(db *gorm.DB, nodeID uint, preferredType string) (map[string]any, error) {
	return (&NodeService{db: db}).BuildUniProxyConfig(nodeID, preferredType)
}

// BuildUniProxyConfig is BuildUniProxyNodeConfig over the service's
// database.
func (s *NodeService) BuildUniProxyConfig(nodeID uint, preferredType string) (map[string]any, error) {
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
			rawType := NormalizeNodeType(uniProxyString(config["node_type"]))
			if rawType == "" {
				rawType = NormalizeNodeType(uniProxyString(config["type"]))
			}
			if rawType != preferredType {
				return nil, fmt.Errorf("raw_config protocol %q does not match requested protocol %q", rawType, preferredType)
			}
		}
		if err := ValidateWireGuardRuntimeConfig(config); err != nil {
			return nil, err
		}
		ensureUniProxyBaseConfig(config)
		return completeUniProxyConfig(config), nil
	}

	protocols, _ := s.GetProtocols(nodeID)
	if len(protocols) > 0 {
		if protocol := selectUniProxyProtocol(protocols, preferredType); protocol != nil {
			for k, v := range BuildNodeProtocolConfig(node, protocol) {
				config[k] = v
			}
			return completeUniProxyConfig(config), nil
		}
	}
	if preferredType != "" {
		return nil, fmt.Errorf("protocol %s not found for node %d", preferredType, nodeID)
	}
	buildMinimalUniProxyConfig(config, node)
	return completeUniProxyConfig(config), nil
}

// completeUniProxyConfig sets type from node_type when it is missing, as
// UniProxy does before it answers.
func completeUniProxyConfig(config map[string]any) map[string]any {
	if _, ok := config["type"]; !ok {
		if nodeType, ok := config["node_type"]; ok {
			config["type"] = nodeType
		}
	}
	return config
}

func ensureUniProxyBaseConfig(config map[string]any) {
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

func buildMinimalUniProxyConfig(config map[string]any, node *model.Node) {
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

func selectUniProxyProtocol(protocols []model.NodeProtocol, preferredType string) *model.NodeProtocol {
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

func uniProxyString(value any) string {
	text, _ := value.(string)
	return text
}
