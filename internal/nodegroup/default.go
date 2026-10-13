package nodegroup

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
)

// The default node group (owner decision N22: the first node needs no
// input). The first time an administrator opens the join dialog of a panel
// without node groups, Control creates one: a proxy group that gives each
// machine one VLESS Reality protocol on a random port, attached to the
// default subscription group so the first node is sellable without further
// steps. Everything in it can be edited afterwards.
const (
	// DefaultGroupName is the default node group's name.
	DefaultGroupName = "默认"
	// DefaultSubscriptionGroupName is the subscription group the default
	// node group joins; it is created when the panel has none by this name
	// or by LegacyDefaultSubscriptionGroupName.
	DefaultSubscriptionGroupName = "默认"
	// LegacyDefaultSubscriptionGroupName is the name of the subscription
	// group a fresh panel seeds at startup (service.InitSubscriptionDefaults).
	LegacyDefaultSubscriptionGroupName = "default"

	// DefaultRealityServerName is the site a default node's Reality
	// protocol presents to probes and forwards unauthenticated handshakes
	// to. It must answer TLS 1.3 with HTTP/2 on port 443 from the node's
	// network and stay up; an operator whose nodes sit where this host is
	// slow or blocked edits the group's protocol (节点组 > 编辑 > 协议),
	// which applies to machines that join afterwards.
	DefaultRealityServerName = "www.microsoft.com"

	// DefaultPortRangeFrom and DefaultPortRangeTo bound the random port of
	// the default protocol, clear of the well-known ports and of the
	// ephemeral range the kernel hands out for outgoing connections.
	DefaultPortRangeFrom = 20000
	DefaultPortRangeTo   = 40000

	defaultSubscriptionGroupDescription = "默认订阅分组，节点组接入的节点会加入这里"
)

// DefaultTemplate is the protocol template of the default node group: one
// VLESS Reality protocol on a random port of the default range, with the
// vision flow. Control generates each node's Reality key pair and short ID.
func DefaultTemplate() []TemplateEntry {
	return []TemplateEntry{{
		Name:      "VLESS Reality",
		Type:      string(model.ProtocolVLESS),
		PortRange: []int{DefaultPortRangeFrom, DefaultPortRangeTo},
		Transport: "tcp",
		TLS:       TemplateTLSReality,
		Reality: &RealityEntry{
			ServerName: DefaultRealityServerName,
			Dest:       DefaultRealityServerName + ":443",
		},
		Settings: json.RawMessage(`{"flow":"xtls-rprx-vision"}`),
	}}
}

// DefaultGroupInput is the default node group as a create request that
// joins the subscription groups.
func DefaultGroupInput(subscriptionGroupIDs []uint) (GroupInput, error) {
	template, err := json.Marshal(DefaultTemplate())
	if err != nil {
		return GroupInput{}, err
	}
	return GroupInput{
		Name: DefaultGroupName, Kind: model.NodeGroupKindProxy, ProtocolTemplate: template,
		SubscriptionGroupIDs: subscriptionGroupIDs,
	}, nil
}

// EnsureDefault makes sure the panel has a node group to join machines to:
// when no live node group exists it creates the default one (and the
// default subscription group when the panel has none), otherwise it changes
// nothing and answers the first group by sort and id. created tells which.
// It is safe to call concurrently: of two racing creates one wins and the
// other answers the winner's group.
func (s *Service) EnsureDefault(ctx context.Context, actor Actor) (view GroupView, created bool, err error) {
	var existing int64
	if err := s.DB.WithContext(ctx).Model(&model.NodeGroup{}).Count(&existing).Error; err != nil {
		return GroupView{}, false, err
	}
	if existing > 0 {
		view, err = s.first(ctx)
		return view, false, err
	}
	// Fail on a missing key-encryption key before creating anything.
	if _, err := s.sealer(); err != nil {
		return GroupView{}, false, err
	}
	subscriptionGroup, err := s.defaultSubscriptionGroup(ctx)
	if err != nil {
		return GroupView{}, false, err
	}
	input, err := DefaultGroupInput([]uint{subscriptionGroup.ID})
	if err != nil {
		return GroupView{}, false, err
	}
	view, err = s.Create(ctx, input, actor)
	if errors.Is(err, ErrNameTaken) {
		// Another request created it first.
		view, err = s.first(ctx)
		return view, false, err
	}
	if err != nil {
		return GroupView{}, false, err
	}
	return view, true, nil
}

// first returns the first live group by sort, then id.
func (s *Service) first(ctx context.Context) (GroupView, error) {
	var group model.NodeGroup
	if err := s.DB.WithContext(ctx).Order("sort").Order("id").Take(&group).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return GroupView{}, ErrNotFound
		}
		return GroupView{}, err
	}
	return s.Get(ctx, group.ID)
}

// defaultSubscriptionGroup returns the subscription group the default node
// group joins: the one named DefaultSubscriptionGroupName, else the one a
// fresh panel seeds, else a new one. An existing group is used as it is.
func (s *Service) defaultSubscriptionGroup(ctx context.Context) (model.SubscriptionGroup, error) {
	db := s.DB.WithContext(ctx)
	for _, name := range []string{DefaultSubscriptionGroupName, LegacyDefaultSubscriptionGroupName} {
		var group model.SubscriptionGroup
		err := db.Where("name = ?", name).Take(&group).Error
		if err == nil {
			return group, nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return model.SubscriptionGroup{}, err
		}
	}
	description := defaultSubscriptionGroupDescription
	group := model.SubscriptionGroup{Name: DefaultSubscriptionGroupName, Description: &description, Enable: 1}
	if err := db.Create(&group).Error; err != nil {
		// A concurrent create of the same name: use that row.
		var found model.SubscriptionGroup
		if lookup := db.Where("name = ?", DefaultSubscriptionGroupName).Take(&found).Error; lookup == nil {
			return found, nil
		}
		return model.SubscriptionGroup{}, err
	}
	return group, nil
}
