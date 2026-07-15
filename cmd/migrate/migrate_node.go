package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/AnixOps/anix-control/v3/internal/model"
)

// nodeProtocolGroups pairs a migrated NodeProtocol with the old-dump group
// IDs it should be associated with (old v2_server_group IDs, preserved
// verbatim as model.SubscriptionGroup IDs).
type nodeProtocolGroups struct {
	Protocol *model.NodeProtocol
	GroupIDs []uint
}

// buildNodesAndProtocols converts old v2_server_shadowsocks and
// v2_server_vless rows into model.Node + model.NodeProtocol. Each old server
// row becomes its own Node (one protocol per node), since the old schema has
// no concept of multiple protocols sharing one physical server entry.
//
// v2_server_vmess and v2_server_trojan are handled the same way when
// present, but this dump has no rows in either.
func buildNodesAndProtocols(tables map[string]*DumpTable) ([]*model.Node, []nodeProtocolGroups, error) {
	var nodes []*model.Node
	var links []nodeProtocolGroups

	if t, ok := tables["v2_server_shadowsocks"]; ok {
		for _, r := range t.Rows {
			node, protocol, groupIDs, err := buildShadowsocksNode(r)
			if err != nil {
				return nil, nil, err
			}
			nodes = append(nodes, node)
			links = append(links, nodeProtocolGroups{Protocol: protocol, GroupIDs: groupIDs})
		}
	}

	if t, ok := tables["v2_server_vless"]; ok {
		for _, r := range t.Rows {
			node, protocol, groupIDs, err := buildVlessNode(r)
			if err != nil {
				return nil, nil, err
			}
			nodes = append(nodes, node)
			links = append(links, nodeProtocolGroups{Protocol: protocol, GroupIDs: groupIDs})
		}
	}

	return nodes, links, nil
}

func buildShadowsocksNode(r row) (*model.Node, *model.NodeProtocol, []uint, error) {
	id, err := mustUint(r, "id")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("v2_server_shadowsocks: %w", err)
	}
	port, err := mustInt(r, "server_port")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("v2_server_shadowsocks id=%d: %w", id, err)
	}
	rate, err := mustFloat64(r, "rate")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("v2_server_shadowsocks id=%d: %w", id, err)
	}
	show, err := mustInt(r, "show")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("v2_server_shadowsocks id=%d: %w", id, err)
	}
	createdAt, err := unixTime(r, "created_at")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("v2_server_shadowsocks id=%d: %w", id, err)
	}
	updatedAt, err := unixTime(r, "updated_at")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("v2_server_shadowsocks id=%d: %w", id, err)
	}
	groupIDs, err := parseJSONIntArray(r, "group_id")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("v2_server_shadowsocks id=%d: %w", id, err)
	}

	apiKey, secret, err := generateNodeCredentials()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("v2_server_shadowsocks id=%d: %w", id, err)
	}

	settings, err := json.Marshal(map[string]any{"cipher": str(r, "cipher")})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("v2_server_shadowsocks id=%d: %w", id, err)
	}
	settingsStr := string(settings)

	node := &model.Node{
		ID:         id,
		Name:       str(r, "name"),
		Host:       str(r, "host"),
		Port:       port,
		APIKey:     apiKey,
		APIKeyHash: hashToken(apiKey),
		Secret:     secret,
		Status:     model.NodeStatusOnline,
		Rate:       rate,
		Sort:       intOrZero(r, "sort"),
		Show:       show,
		CreatedAt:  createdAt,
		UpdatedAt:  updatedAt,
	}
	protocol := &model.NodeProtocol{
		NodeID:    id,
		Name:      str(r, "name"),
		Type:      model.ProtocolShadowsocks,
		Port:      port,
		Enable:    1,
		Show:      show,
		Settings:  &settingsStr,
		Transport: strPtrLit("tcp"),
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}
	return node, protocol, groupIDs, nil
}

func buildVlessNode(r row) (*model.Node, *model.NodeProtocol, []uint, error) {
	id, err := mustUint(r, "id")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("v2_server_vless: %w", err)
	}
	port, err := mustInt(r, "server_port")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("v2_server_vless id=%d: %w", id, err)
	}
	rate, err := mustFloat64(r, "rate")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("v2_server_vless id=%d: %w", id, err)
	}
	show, err := mustInt(r, "show")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("v2_server_vless id=%d: %w", id, err)
	}
	tls, err := mustInt(r, "tls")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("v2_server_vless id=%d: %w", id, err)
	}
	createdAt, err := unixTime(r, "created_at")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("v2_server_vless id=%d: %w", id, err)
	}
	updatedAt, err := unixTime(r, "updated_at")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("v2_server_vless id=%d: %w", id, err)
	}
	groupIDs, err := parseJSONIntArray(r, "group_id")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("v2_server_vless id=%d: %w", id, err)
	}

	apiKey, secret, err := generateNodeCredentials()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("v2_server_vless id=%d: %w", id, err)
	}

	// Old tls_settings blob mixes plain TLS fields (server_name,
	// allow_insecure) with Reality-only fields (public_key/private_key/
	// short_id). Split it into the new schema's separate TLSSettings and
	// RealitySettings JSON blobs.
	var oldTLS map[string]any
	if raw := str(r, "tls_settings"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &oldTLS); err != nil {
			return nil, nil, nil, fmt.Errorf("v2_server_vless id=%d: tls_settings: %w", id, err)
		}
	}

	var tlsSettingsStr, realitySettingsStr *string
	if oldTLS != nil {
		tlsSettings := map[string]any{}
		if v, ok := oldTLS["server_name"]; ok {
			tlsSettings["server_name"] = v
		}
		if v, ok := oldTLS["allow_insecure"]; ok {
			tlsSettings["allow_insecure"] = v
		}
		if len(tlsSettings) > 0 {
			b, err := json.Marshal(tlsSettings)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("v2_server_vless id=%d: %w", id, err)
			}
			s := string(b)
			tlsSettingsStr = &s
		}

		if tls == 2 {
			realitySettings := map[string]any{}
			if v, ok := oldTLS["public_key"]; ok {
				realitySettings["public_key"] = v
			}
			if v, ok := oldTLS["private_key"]; ok {
				realitySettings["private_key"] = v
			}
			if v, ok := oldTLS["short_id"]; ok {
				realitySettings["short_id"] = v
			}
			if len(realitySettings) > 0 {
				b, err := json.Marshal(realitySettings)
				if err != nil {
					return nil, nil, nil, fmt.Errorf("v2_server_vless id=%d: %w", id, err)
				}
				s := string(b)
				realitySettingsStr = &s
			}
		}
	}

	settings, err := json.Marshal(map[string]any{"flow": str(r, "flow")})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("v2_server_vless id=%d: %w", id, err)
	}
	settingsStr := string(settings)

	node := &model.Node{
		ID:         id,
		Name:       str(r, "name"),
		Host:       str(r, "host"),
		Port:       port,
		APIKey:     apiKey,
		APIKeyHash: hashToken(apiKey),
		Secret:     secret,
		Status:     model.NodeStatusOnline,
		Rate:       rate,
		Sort:       intOrZero(r, "sort"),
		Show:       show,
		CreatedAt:  createdAt,
		UpdatedAt:  updatedAt,
	}
	protocol := &model.NodeProtocol{
		NodeID:          id,
		Name:            str(r, "name"),
		Type:            model.ProtocolVLESS,
		Port:            port,
		Enable:          1,
		Show:            show,
		TLS:             tls,
		Settings:        &settingsStr,
		TLSSettings:     tlsSettingsStr,
		RealitySettings: realitySettingsStr,
		Transport:       strPtr(r, "network"),
		CreatedAt:       createdAt,
		UpdatedAt:       updatedAt,
	}
	return node, protocol, groupIDs, nil
}

func intOrZero(r row, col string) int {
	n, err := mustInt(r, col)
	if err != nil {
		return 0
	}
	return n
}

func strPtrLit(s string) *string {
	return &s
}

// generateNodeCredentials mirrors NodeService.CreateNode's key generation
// (internal/service/node_service.go) so migrated nodes carry the same
// APIKey/APIKeyHash/Secret shape as nodes created through the admin API.
func generateNodeCredentials() (apiKey, secret string, err error) {
	apiKey, err = generateSecureToken(32)
	if err != nil {
		return "", "", err
	}
	secret, err = generateSecureToken(32)
	if err != nil {
		return "", "", err
	}
	return apiKey, secret, nil
}

func generateSecureToken(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func hashToken(s string) string {
	h := sha256.New()
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}
