package model

// DestinationStat is a row of v5_destination_stat: one user's traffic to one
// destination through one node in one UTC hour, summed from the Agents'
// DestinationReport batches (destinations.v1, internal/destinationstats,
// docs/reference/destination-stats.md). Domain is a registrable domain
// (example.com), an address network (149.154.167.0/24) or empty for the rest
// of a user's traffic beyond the Agent's per-window limit. Bytes are raw,
// without the node's traffic rate.
type DestinationStat struct {
	ID uint `gorm:"primaryKey" json:"id"`
	// HourAt is the hour's start, Unix seconds.
	HourAt   int64  `gorm:"not null;uniqueIndex:idx_v5_destination_stat_key,priority:1;index:idx_v5_destination_stat_user,priority:2" json:"hour_at"`
	NodeID   uint   `gorm:"not null;uniqueIndex:idx_v5_destination_stat_key,priority:2" json:"node_id"`
	UserID   uint   `gorm:"not null;uniqueIndex:idx_v5_destination_stat_key,priority:3;index:idx_v5_destination_stat_user,priority:1" json:"user_id"`
	Domain   string `gorm:"size:255;not null;default:'';uniqueIndex:idx_v5_destination_stat_key,priority:4" json:"domain"`
	Protocol string `gorm:"size:32;not null;default:'';uniqueIndex:idx_v5_destination_stat_key,priority:5" json:"protocol"`
	Upload   int64  `gorm:"not null;default:0" json:"upload"`
	Download int64  `gorm:"not null;default:0" json:"download"`
	// Connections counts the connections opened in the hour.
	Connections int64 `gorm:"not null;default:0" json:"connections"`
}

// TableName is v5_destination_stat.
func (DestinationStat) TableName() string { return "v5_destination_stat" }
