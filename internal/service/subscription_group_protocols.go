package service

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
)

// MaxGroupProtocolDisplayName is the longest display name, in characters.
const MaxGroupProtocolDisplayName = 128

// ErrInvalidGroupProtocolLayout is a layout the group cannot take: a
// protocol not linked to the group, one listed twice, or a display name
// that is too long or holds a control character.
var ErrInvalidGroupProtocolLayout = errors.New("invalid group protocol layout")

// GroupProtocolLayoutItem is one of a group's node protocols, in the
// group's order, with the name subscribers see ("" for the default
// "<node> - <protocol>").
type GroupProtocolLayoutItem struct {
	ProtocolID  uint   `json:"protocol_id"`
	DisplayName string `json:"display_name"`
}

// groupProtocolKey is a (group, node protocol) pair.
type groupProtocolKey struct{ group, protocol uint }

// groupProtocolLayouts reads the display names and order of the groups'
// protocols.
func groupProtocolLayouts(db *gorm.DB, groupIDs []uint) (map[groupProtocolKey]model.SubscriptionGroupProtocol, error) {
	layouts := map[groupProtocolKey]model.SubscriptionGroupProtocol{}
	if len(groupIDs) == 0 {
		return layouts, nil
	}
	var rows []model.SubscriptionGroupProtocol
	if err := db.Where("group_id IN ?", groupIDs).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		layouts[groupProtocolKey{row.GroupID, row.NodeProtocolID}] = row
	}
	return layouts, nil
}

// groupProtocolLess orders a group's protocols: the ordered ones by their
// place, then the others by id.
func groupProtocolLess(layouts map[groupProtocolKey]model.SubscriptionGroupProtocol, groupID, a, b uint) bool {
	la, oka := layouts[groupProtocolKey{groupID, a}]
	lb, okb := layouts[groupProtocolKey{groupID, b}]
	switch {
	case oka != okb:
		return oka
	case oka && la.Sort != lb.Sort:
		return la.Sort < lb.Sort
	}
	return a < b
}

// linkedGroupProtocols lists the ids of the node protocols linked to the
// group (v2_subscription_group_node_protocols).
func linkedGroupProtocols(db *gorm.DB, groupID uint) ([]uint, error) {
	var ids []uint
	err := db.Table("v2_subscription_group_node_protocols").
		Where("subscription_group_id = ?", groupID).
		Pluck("node_protocol_id", &ids).Error
	return ids, err
}

// GroupProtocolLayout lists every node protocol linked to the group in the
// group's order, with its display name. An unknown group is
// ErrSubscriptionGroupNotFound.
func (s *SubscriptionService) GroupProtocolLayout(groupID uint) ([]GroupProtocolLayoutItem, error) {
	if err := subscriptionGroupExists(s.db, groupID); err != nil {
		return nil, err
	}
	return s.groupProtocolLayout(s.db, groupID)
}

func (s *SubscriptionService) groupProtocolLayout(db *gorm.DB, groupID uint) ([]GroupProtocolLayoutItem, error) {
	ids, err := linkedGroupProtocols(db, groupID)
	if err != nil {
		return nil, err
	}
	layouts, err := groupProtocolLayouts(db, []uint{groupID})
	if err != nil {
		return nil, err
	}
	sort.Slice(ids, func(i, j int) bool { return groupProtocolLess(layouts, groupID, ids[i], ids[j]) })
	items := make([]GroupProtocolLayoutItem, 0, len(ids))
	for _, id := range ids {
		items = append(items, GroupProtocolLayoutItem{ProtocolID: id, DisplayName: layouts[groupProtocolKey{groupID, id}].DisplayName})
	}
	return items, nil
}

// ReplaceGroupProtocolLayout makes items the group's order and display
// names. Every item must be a protocol linked to the group, at most once;
// linked protocols left out come after them, by id, under their default
// names. Display names are trimmed; "" restores the default. It answers
// the group's layout afterwards.
func (s *SubscriptionService) ReplaceGroupProtocolLayout(groupID uint, items []GroupProtocolLayoutItem) ([]GroupProtocolLayoutItem, error) {
	if err := subscriptionGroupExists(s.db, groupID); err != nil {
		return nil, err
	}
	var result []GroupProtocolLayoutItem
	err := s.db.Transaction(func(tx *gorm.DB) error {
		linked, err := linkedGroupProtocols(tx, groupID)
		if err != nil {
			return err
		}
		isLinked := make(map[uint]bool, len(linked))
		for _, id := range linked {
			isLinked[id] = true
		}
		rows := make([]model.SubscriptionGroupProtocol, 0, len(items))
		seen := make(map[uint]bool, len(items))
		for i, item := range items {
			if !isLinked[item.ProtocolID] {
				return fmt.Errorf("%w: protocol %d is not linked to the group", ErrInvalidGroupProtocolLayout, item.ProtocolID)
			}
			if seen[item.ProtocolID] {
				return fmt.Errorf("%w: protocol %d is listed twice", ErrInvalidGroupProtocolLayout, item.ProtocolID)
			}
			seen[item.ProtocolID] = true
			name, err := cleanGroupProtocolDisplayName(item.DisplayName)
			if err != nil {
				return err
			}
			rows = append(rows, model.SubscriptionGroupProtocol{GroupID: groupID, NodeProtocolID: item.ProtocolID, DisplayName: name, Sort: i})
		}
		if err := tx.Where("group_id = ?", groupID).Delete(&model.SubscriptionGroupProtocol{}).Error; err != nil {
			return err
		}
		if len(rows) > 0 {
			if err := tx.Create(&rows).Error; err != nil {
				return err
			}
		}
		result, err = s.groupProtocolLayout(tx, groupID)
		return err
	})
	return result, err
}

func cleanGroupProtocolDisplayName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if !utf8.ValidString(name) {
		return "", fmt.Errorf("%w: display name is not valid UTF-8", ErrInvalidGroupProtocolLayout)
	}
	if utf8.RuneCountInString(name) > MaxGroupProtocolDisplayName {
		return "", fmt.Errorf("%w: display name is longer than %d characters", ErrInvalidGroupProtocolLayout, MaxGroupProtocolDisplayName)
	}
	if strings.ContainsFunc(name, unicode.IsControl) {
		return "", fmt.Errorf("%w: display name holds a control character", ErrInvalidGroupProtocolLayout)
	}
	return name, nil
}
