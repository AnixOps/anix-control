package model

// SubscriptionGroupProtocol is how a subscription group presents one of its
// node protocols to subscribers: the name the subscription shows in place of
// "<node> - <protocol>" and the protocol's place in the group's list. The
// link itself stays v2_subscription_group_node_protocols (adopted by the
// subscription package); a linked protocol without a row comes after the
// ordered ones, by id, under its default name. A row whose protocol is no
// longer linked is ignored.
type SubscriptionGroupProtocol struct {
	GroupID        uint `gorm:"primaryKey;autoIncrement:false" json:"group_id"`
	NodeProtocolID uint `gorm:"primaryKey;autoIncrement:false" json:"protocol_id"`
	// DisplayName is the node's name in the subscription; empty keeps
	// "<node> - <protocol>". A node with entries appends " | <entry>".
	DisplayName string `gorm:"size:128;not null;default:''" json:"display_name"`
	Sort        int    `gorm:"not null;default:0" json:"sort"`
}

func (SubscriptionGroupProtocol) TableName() string { return "v5_subscription_group_protocol" }
