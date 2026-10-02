package model

import "time"

// Route-mode revision actions, recorded in RouteModeRevision.
const (
	RouteModeRevisionActionSet      = "set"
	RouteModeRevisionActionRollback = "rollback"
)

// RouteModeRevision records one route's mode change made by the route-mode
// administration (API, CLI or admin page). The routes of one request share
// GroupID; ConfigRevision is the package configuration revision that holds
// the change. The identity cutover records its own changes in
// IdentityCutoverEvent.
type RouteModeRevision struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	GroupID        string    `gorm:"size:36;not null;index" json:"group_id"`
	PackageID      string    `gorm:"size:120;not null;index" json:"package_id"`
	RouteID        string    `gorm:"size:191;not null" json:"route_id"`
	Action         string    `gorm:"size:16;not null" json:"action"`
	FromMode       string    `gorm:"size:16;not null" json:"from_mode"`
	ToMode         string    `gorm:"size:16;not null" json:"to_mode"`
	ActorUserID    uint      `gorm:"not null;default:0" json:"actor_user_id"`
	Actor          string    `gorm:"size:191;not null;default:''" json:"actor"`
	Reason         string    `gorm:"type:text;not null;default:''" json:"reason"`
	ConfigRevision int64     `gorm:"not null" json:"config_revision"`
	CreatedAt      time.Time `gorm:"not null;index" json:"created_at"`
}

func (RouteModeRevision) TableName() string { return "v4_kernel_route_mode_revision" }
