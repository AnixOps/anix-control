package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrInvalidNodeOverride is an edit a subscription group's node cannot take.
var ErrInvalidNodeOverride = errors.New("invalid node override")

// overridableFields are the subscription node fields a group may edit, by
// JSON name. The protocol type and the credentials stay the source's: a node
// shown as another protocol, or with another user's secret, would not connect.
var overridableFields = map[string]string{
	"name": "string", "server": "string", "port": "int",
	"server_name": "string", "alpn": "string", "tls_fingerprint": "string",
	"skip_cert_verify": "bool", "flow": "string", "transport": "string",
	"transport_settings": "object", "settings": "object",
	"reality_public_key": "string", "reality_short_id": "string", "reality_spider_x": "string",
}

// GroupNodeOverrides is one node (a protocol's, or one entry's of it) of a
// group, with the group's edits and what the source produces.
type GroupNodeOverrides struct {
	Key       string         `json:"key"`
	Name      string         `json:"name"`
	Overrides map[string]any `json:"overrides"`
	Defaults  map[string]any `json:"defaults,omitempty"`
}

// nodeKey is the part of a node id after the protocol's own "node-N-P".
func nodeKey(id string, nodeID, protocolID uint) (string, bool) {
	base := fmt.Sprintf("node-%d-%d", nodeID, protocolID)
	if !strings.HasPrefix(id, base) {
		return "", false
	}
	return strings.TrimPrefix(id, base), true
}

func parseOverrides(raw string) map[string]map[string]any {
	out := map[string]map[string]any{}
	if strings.TrimSpace(raw) == "" {
		return out
	}
	_ = json.Unmarshal([]byte(raw), &out)
	return out
}

// applyNodeOverrides edits parsed with the group's edits of its key.
func applyNodeOverrides(parsed *model.ParsedNode, nodeID, protocolID uint, raw string) {
	if raw == "" {
		return
	}
	key, ok := nodeKey(parsed.ID, nodeID, protocolID)
	if !ok {
		return
	}
	edit := parseOverrides(raw)[key]
	if len(edit) == 0 {
		return
	}
	cur, err := json.Marshal(parsed)
	if err != nil {
		return
	}
	merged := map[string]any{}
	if json.Unmarshal(cur, &merged) != nil {
		return
	}
	for name, value := range edit {
		if _, allowed := overridableFields[name]; allowed {
			merged[name] = value
		}
	}
	buf, err := json.Marshal(merged)
	if err != nil {
		return
	}
	var out model.ParsedNode
	if json.Unmarshal(buf, &out) == nil {
		*parsed = out
	}
}

func cleanOverrides(edits map[string]map[string]any) (map[string]map[string]any, error) {
	clean := map[string]map[string]any{}
	for key, fields := range edits {
		if len(fields) == 0 {
			continue
		}
		one := map[string]any{}
		for name, value := range fields {
			kind, ok := overridableFields[name]
			if !ok {
				return nil, fmt.Errorf("%w: field %q cannot be edited", ErrInvalidNodeOverride, name)
			}
			switch kind {
			case "string":
				v, ok := value.(string)
				if !ok || !utf8.ValidString(v) || len(v) > 512 || strings.ContainsFunc(v, unicode.IsControl) {
					return nil, fmt.Errorf("%w: %s must be a short text", ErrInvalidNodeOverride, name)
				}
				if name == "name" && strings.TrimSpace(v) == "" {
					return nil, fmt.Errorf("%w: name is empty", ErrInvalidNodeOverride)
				}
				value = strings.TrimSpace(v)
			case "int":
				v, ok := value.(float64)
				if !ok || v != float64(int(v)) || v < 1 || v > 65535 {
					return nil, fmt.Errorf("%w: port must be 1-65535", ErrInvalidNodeOverride)
				}
				value = int(v)
			case "bool":
				if _, ok := value.(bool); !ok {
					return nil, fmt.Errorf("%w: %s must be true or false", ErrInvalidNodeOverride, name)
				}
			case "object":
				if _, ok := value.(map[string]any); !ok {
					return nil, fmt.Errorf("%w: %s must be an object", ErrInvalidNodeOverride, name)
				}
			}
			one[name] = value
		}
		clean[key] = one
	}
	return clean, nil
}

// GroupProtocolNodes lists the nodes the protocol produces in the group, with
// the group's edits and the unedited values.
func (s *SubscriptionService) GroupProtocolNodes(groupID, protocolID uint) ([]GroupNodeOverrides, error) {
	if err := subscriptionGroupExists(s.db, groupID); err != nil {
		return nil, err
	}
	var group model.SubscriptionGroup
	if err := s.db.First(&group, groupID).Error; err != nil {
		return nil, err
	}
	layouts, err := groupProtocolLayouts(s.db, []uint{groupID})
	if err != nil {
		return nil, err
	}
	edits := parseOverrides(layouts[groupProtocolKey{groupID, protocolID}].Overrides)
	nodes, err := s.internalNodes(&model.TemplateRenderContext{}, []*model.SubscriptionGroup{&group}, false)
	if err != nil {
		return nil, err
	}
	result := []GroupNodeOverrides{}
	seen := map[string]bool{}
	for _, node := range nodes {
		if node.SourceType != "node" {
			continue
		}
		key, ok := nodeKey(node.ID, node.SourceID, protocolID)
		if !ok {
			continue
		}
		seen[key] = true
		defaults := map[string]any{}
		raw, _ := json.Marshal(node)
		all := map[string]any{}
		_ = json.Unmarshal(raw, &all)
		for name := range overridableFields {
			if v, ok := all[name]; ok {
				defaults[name] = v
			}
		}
		item := GroupNodeOverrides{Key: key, Name: node.Name, Overrides: edits[key], Defaults: defaults}
		if item.Overrides == nil {
			item.Overrides = map[string]any{}
		}
		result = append(result, item)
	}
	for key, fields := range edits { // edits of a node that is offline or gone
		if !seen[key] {
			result = append(result, GroupNodeOverrides{Key: key, Overrides: fields})
		}
	}
	return result, nil
}

// ReplaceGroupProtocolOverrides sets the group's edits of the protocol's
// nodes; keys not listed (or listed empty) are reset to the source.
func (s *SubscriptionService) ReplaceGroupProtocolOverrides(groupID, protocolID uint, edits map[string]map[string]any) error {
	if err := subscriptionGroupExists(s.db, groupID); err != nil {
		return err
	}
	clean, err := cleanOverrides(edits)
	if err != nil {
		return err
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		linked, err := linkedGroupProtocols(tx, groupID)
		if err != nil {
			return err
		}
		found := false
		for _, id := range linked {
			found = found || id == protocolID
		}
		if !found {
			return fmt.Errorf("%w: protocol %d is not linked to the group", ErrInvalidNodeOverride, protocolID)
		}
		raw := ""
		if len(clean) > 0 {
			buf, err := json.Marshal(clean)
			if err != nil {
				return err
			}
			raw = string(buf)
		}
		// A protocol with no layout row yet comes after the ordered ones; the
		// new row must not move it ahead of them.
		var next int
		if err := tx.Model(&model.SubscriptionGroupProtocol{}).Where("group_id = ?", groupID).
			Select("COALESCE(MAX(sort) + 1, 0)").Scan(&next).Error; err != nil {
			return err
		}
		row := model.SubscriptionGroupProtocol{GroupID: groupID, NodeProtocolID: protocolID, Sort: next, Overrides: raw}
		return tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "group_id"}, {Name: "node_protocol_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"overrides"}),
		}).Create(&row).Error
	})
}

// ResetGroupOverrides clears every edit of the group's nodes.
func (s *SubscriptionService) ResetGroupOverrides(groupID uint) error {
	if err := subscriptionGroupExists(s.db, groupID); err != nil {
		return err
	}
	return s.db.Model(&model.SubscriptionGroupProtocol{}).Where("group_id = ?", groupID).Update("overrides", "").Error
}
