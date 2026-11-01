package model

import "time"

// TrafficCategoryRule is an administrator's rule of the traffic categories
// (internal/destinationstats, docs/reference/destination-stats.md): the
// destinations Value names fall in Category. Kind is domain (a registrable
// domain or a name under one; the rule covers its subdomains), network (an
// address or CIDR) or protocol (a sniffed protocol such as bittorrent). The
// administrator's rules win over the remote sources' and the built-in ones;
// a rule to web takes a domain out of a built-in category.
type TrafficCategoryRule struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Kind      string    `gorm:"size:16;not null;uniqueIndex:idx_v5_traffic_category_rule_key,priority:1" json:"kind"`
	Value     string    `gorm:"size:255;not null;uniqueIndex:idx_v5_traffic_category_rule_key,priority:2" json:"value"`
	Category  string    `gorm:"size:32;not null" json:"category"`
	Note      string    `gorm:"size:255;not null;default:''" json:"note"`
	CreatedAt time.Time `gorm:"not null" json:"created_at"`
	UpdatedAt time.Time `gorm:"not null" json:"updated_at"`
}

// TableName is v5_traffic_category_rule.
func (TrafficCategoryRule) TableName() string { return "v5_traffic_category_rule" }

// TrafficCategorySource is a remote list of domains and networks that fall
// in Category, fetched from URL every RefreshHours hours. EntriesJSON keeps
// the entries of the last successful fetch ({"domains": [...], "networks":
// [...]}), so a failed fetch (FetchError) keeps the previous list.
type TrafficCategorySource struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	Name         string     `gorm:"size:100;not null;uniqueIndex" json:"name"`
	URL          string     `gorm:"size:1024;not null" json:"url"`
	Category     string     `gorm:"size:32;not null" json:"category"`
	Enabled      bool       `gorm:"not null;default:true" json:"enabled"`
	RefreshHours int        `gorm:"not null;default:24" json:"refresh_hours"`
	EntriesJSON  string     `gorm:"type:text;not null;default:''" json:"-"`
	EntryCount   int        `gorm:"not null;default:0" json:"entry_count"`
	FetchedAt    *time.Time `json:"fetched_at"`
	FetchError   string     `gorm:"type:text;not null;default:''" json:"fetch_error"`
	CreatedAt    time.Time  `gorm:"not null" json:"created_at"`
	UpdatedAt    time.Time  `gorm:"not null" json:"updated_at"`
}

// TableName is v5_traffic_category_source.
func (TrafficCategorySource) TableName() string { return "v5_traffic_category_source" }
