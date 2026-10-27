package model

import "time"

// ForwardRouteSubscription publishes a forward route in subscriptions
// (internal/routesubscription, node-onboarding design Phase 10): a route
// whose target is a protocol of a proxy node is offered to the protocol's
// subscription groups once per entry node, at the entry's address and port,
// with the protocol's settings and keys. A route without a row is not
// published.
type ForwardRouteSubscription struct {
	RouteID string `gorm:"primaryKey;size:64" json:"route_id"`
	// DisplayName replaces the route's name after " | " in the subscription;
	// empty uses the route's name.
	DisplayName string    `gorm:"size:128;not null;default:''" json:"display_name"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (ForwardRouteSubscription) TableName() string { return "v5_forward_route_subscription" }
