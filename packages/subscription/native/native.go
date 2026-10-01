// Package native implements the subscription package's v2 administrator
// routes in the package itself, on the kernel's v2_subscription_group,
// v2_subscription_template, v2_plan_subscription_group and
// v2_subscription_group_node_protocols tables adopted in place
// (kernel.storage.adopt). Legacy handlers and native routes share the
// tables, so a route can switch between them at any time. Responses are
// byte-compatible with the legacy handlers
// (internal/tests/subscriptioncompat); the bound and answered types mirror
// the kernel model (package model).
//
// Subscription group membership (v2_user_subscription_group) is subscriber
// state, which only the kernel writes: granting a user a group, taking it
// away and deleting a group's members go through KernelSubscriber
// (kernel.subscriber.groups.v1), and without it those routes stay legacy.
//
// Other domains are read through kernel views only:
//   - kapi_plan_catalog_v1 for whether a plan exists;
//   - kapi_subscriber_entitlement_v1 for whether a user exists and the
//     traffic group members used;
//   - kapi_user_subscription_group_v1 for the subscription groups a user
//     holds and until when;
//   - kapi_node_protocol_v1 and kapi_node_heartbeat_v1 for which node a
//     protocol belongs to and when it last reported.
//
// Five routes have no native handler and stay bridged; see bridgedRoutes in
// packages/subscription/control.
package native

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/sdk/v2compat"
	"gorm.io/gorm"
)

// PlanCatalog is the part of a kapi_plan_catalog_v1 row the routes read.
type PlanCatalog struct {
	ID uint `gorm:"primaryKey"`
}

// TableName is the kernel view.
func (PlanCatalog) TableName() string { return "kapi_plan_catalog_v1" }

// Service holds what the native routes need.
type Service struct {
	// Open returns the package's storage connection, on which the adopted
	// tables and the granted views are visible.
	Open func(ctx context.Context) (*gorm.DB, error)
	// Subscriber is the kernel's KernelSubscriber; without it the routes
	// that change subscription group membership have no native handler and
	// stay legacy.
	Subscriber Subscriber
	// Now defaults to time.Now.
	Now func() time.Time
	// NewToken names a request that carries neither an Idempotency-Key nor
	// an X-Request-ID; it defaults to a random UUID.
	NewToken func() string
}

// Handlers returns the native handlers by route id.
func (s *Service) Handlers() map[string]pluginhostsdk.NativeHandler {
	handlers := map[string]pluginhostsdk.NativeHandler{
		"subscription.admin.subscription.formats.get":                          s.Formats,
		"subscription.admin.subscription.protocols.get":                        s.ProtocolTypes,
		"subscription.admin.subscription.groups.get":                           s.Groups,
		"subscription.admin.subscription.groups.post":                          s.CreateGroup,
		"subscription.admin.subscription.groups.id.get":                        s.Group,
		"subscription.admin.subscription.groups.id.put":                        s.UpdateGroup,
		"subscription.admin.subscription.groups.id.templates.get":              s.GroupTemplates,
		"subscription.admin.subscription.groups.id.templates.post":             s.CreateTemplate,
		"subscription.admin.subscription.groups.id.protocols.post":             s.LinkProtocols,
		"subscription.admin.subscription.templates.id.get":                     s.Template,
		"subscription.admin.subscription.templates.id.put":                     s.UpdateTemplate,
		"subscription.admin.subscription.templates.id.delete":                  s.DeleteTemplate,
		"subscription.admin.subscription.plans.plan_id.groups.get":             s.PlanGroups,
		"subscription.admin.subscription.plans.plan_id.groups.post":            s.AssignPlanGroup,
		"subscription.admin.subscription.plans.plan_id.groups.group_id.delete": s.RemovePlanGroup,
		"subscription.admin.subscription.users.user_id.groups.get":             s.UserGroups,
		"subscription.admin.subscription.stats.get":                            s.Stats,
	}
	if s.Subscriber != nil {
		handlers[DeleteGroupRouteID] = s.DeleteGroup
		handlers[GrantUserGroupRouteID] = s.GrantUserGroup
		handlers[RevokeUserGroupRouteID] = s.RevokeUserGroup
	}
	return handlers
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Service) panel(data any) (pluginhostsdk.NativeResponse, error) {
	return pluginhostsdk.PanelJSON(v2compat.PanelSuccess(data, s.now()))
}

func (s *Service) panelError(message string) (pluginhostsdk.NativeResponse, error) {
	return pluginhostsdk.PanelJSON(v2compat.PanelError(message, s.now()))
}

// pathUint parses a path parameter as the legacy handlers do.
func pathUint(request pluginhostsdk.NativeRequest, name string) (uint, bool) {
	id, err := strconv.ParseUint(request.Metadata.PathParams[name], 10, 32)
	return uint(id), err == nil
}

// UserGroup is a row of kapi_user_subscription_group_v1: a subscription
// group a user holds, until expire_at (null: no expiry). The kernel is the
// only writer of v2_user_subscription_group.
type UserGroup struct {
	UserID   uint
	GroupID  uint
	ExpireAt *int64
}

// TableName is the kernel view.
func (UserGroup) TableName() string { return "kapi_user_subscription_group_v1" }

// Entitlement is the part of a kapi_subscriber_entitlement_v1 row the
// routes read: that the user exists, and the traffic they used.
type Entitlement struct {
	ID uint `gorm:"primaryKey"`
	U  int64
	D  int64
}

// TableName is the kernel view.
func (Entitlement) TableName() string { return "kapi_subscriber_entitlement_v1" }

// NodeProtocol is a row of kapi_node_protocol_v1: a node protocol and its
// node, without its settings.
type NodeProtocol struct {
	ID     uint `gorm:"primaryKey"`
	NodeID uint
}

// TableName is the kernel view.
func (NodeProtocol) TableName() string { return "kapi_node_protocol_v1" }

// NodeHeartbeat is a row of kapi_node_heartbeat_v1: when a node last
// reported.
type NodeHeartbeat struct {
	ID          uint `gorm:"primaryKey"`
	LastCheckAt *int64
}

// TableName is the kernel view.
func (NodeHeartbeat) TableName() string { return "kapi_node_heartbeat_v1" }

// jsonAnswer is the legacy handlers' c.JSON(code, body): encoding/json, as
// gin uses, with gin's content type.
func jsonAnswer(code int, body any) (pluginhostsdk.NativeResponse, error) {
	encoded, err := json.Marshal(body)
	if err != nil {
		return pluginhostsdk.NativeResponse{}, err
	}
	return pluginhostsdk.NativeResponse{
		StatusCode: uint32(code), Body: encoded, // #nosec G115 -- HTTP status codes.
		Headers: []pluginhostsdk.Header{{Name: "Content-Type", Value: "application/json; charset=utf-8"}},
	}, nil
}
