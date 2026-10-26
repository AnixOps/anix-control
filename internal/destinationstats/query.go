package destinationstats

import (
	"sort"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
)

// Query bounds.
const (
	DefaultHours = 24
	MaxHours     = MaxRetentionDays * 24
	DefaultLimit = 50
	MaxLimit     = 500
)

// Query selects the rows of a summary: the hours [Since, Until), one user
// or all (UserID 0), one node or all (NodeID 0), and at most Limit domains.
type Query struct {
	Since, Until time.Time
	UserID       uint
	NodeID       uint
	Limit        int
}

// LastHours is the query of the hours whole hours up to and including the
// current one.
func LastHours(hours int, now time.Time) Query {
	if hours <= 0 {
		hours = DefaultHours
	}
	if hours > MaxHours {
		hours = MaxHours
	}
	until := now.UTC().Truncate(time.Hour).Add(time.Hour)
	return Query{Since: until.Add(-time.Duration(hours) * time.Hour), Until: until}
}

// Totals are bytes and connections.
type Totals struct {
	Upload      int64 `json:"upload"`
	Download    int64 `json:"download"`
	Connections int64 `json:"connections"`
}

func (t *Totals) add(other Totals) {
	t.Upload += other.Upload
	t.Download += other.Download
	t.Connections += other.Connections
}

func (t Totals) bytes() int64 { return t.Upload + t.Download }

// CategoryTotal is one category's traffic.
type CategoryTotal struct {
	Category string `json:"category"`
	Totals
}

// DomainTotal is one destination's traffic; Domain is empty for the traffic
// beyond the Agents' per-window limit.
type DomainTotal struct {
	Domain   string `json:"domain"`
	Category string `json:"category"`
	Totals
}

// Summary is the traffic of a query by category and by destination.
type Summary struct {
	SinceUnixMs int64 `json:"since_unix_ms"`
	UntilUnixMs int64 `json:"until_unix_ms"`
	Totals
	// Categories lists every category with traffic, by bytes, largest
	// first.
	Categories []CategoryTotal `json:"categories"`
	// Domains lists the destinations with the most bytes, largest first,
	// at most the query's limit; DomainCount counts them all.
	Domains     []DomainTotal `json:"domains"`
	DomainCount int           `json:"domain_count"`
}

type summaryRow struct {
	Domain      string
	Protocol    string
	Upload      int64
	Download    int64
	Connections int64
}

// Summarize answers a query. Categories are judged now, so a rule change
// applies to the history too.
func Summarize(db *gorm.DB, query Query) (Summary, error) {
	limit := query.Limit
	if limit <= 0 {
		limit = DefaultLimit
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}
	summary := Summary{
		SinceUnixMs: query.Since.UnixMilli(), UntilUnixMs: query.Until.UnixMilli(),
		Categories: []CategoryTotal{}, Domains: []DomainTotal{},
	}
	scope := db.Model(&model.DestinationStat{}).
		Select("domain, protocol, SUM(upload) AS upload, SUM(download) AS download, SUM(connections) AS connections").
		Where("hour_at >= ? AND hour_at < ?", query.Since.Unix(), query.Until.Unix())
	if query.UserID != 0 {
		scope = scope.Where("user_id = ?", query.UserID)
	}
	if query.NodeID != 0 {
		scope = scope.Where("node_id = ?", query.NodeID)
	}
	var rows []summaryRow
	if err := scope.Group("domain, protocol").Scan(&rows).Error; err != nil {
		return summary, err
	}

	categories := map[string]*CategoryTotal{}
	type domainParts struct {
		total    Totals
		category string
		largest  int64
	}
	domains := map[string]*domainParts{}
	for _, row := range rows {
		totals := Totals{Upload: row.Upload, Download: row.Download, Connections: row.Connections}
		category := Classify(row.Domain, row.Protocol)
		summary.add(totals)
		if categories[category] == nil {
			categories[category] = &CategoryTotal{Category: category}
		}
		categories[category].add(totals)
		parts := domains[row.Domain]
		if parts == nil {
			parts = &domainParts{largest: -1}
			domains[row.Domain] = parts
		}
		parts.total.add(totals)
		// A domain reached over several protocols takes the category of
		// the one with the most bytes.
		if totals.bytes() > parts.largest {
			parts.largest, parts.category = totals.bytes(), category
		}
	}
	for _, category := range Categories {
		if total := categories[category]; total != nil {
			summary.Categories = append(summary.Categories, *total)
		}
	}
	sort.SliceStable(summary.Categories, func(i, j int) bool {
		return summary.Categories[i].bytes() > summary.Categories[j].bytes()
	})
	for domain, parts := range domains {
		summary.Domains = append(summary.Domains, DomainTotal{Domain: domain, Category: parts.category, Totals: parts.total})
	}
	sort.Slice(summary.Domains, func(i, j int) bool {
		a, b := summary.Domains[i], summary.Domains[j]
		if a.bytes() != b.bytes() {
			return a.bytes() > b.bytes()
		}
		return a.Domain < b.Domain
	})
	summary.DomainCount = len(summary.Domains)
	if len(summary.Domains) > limit {
		summary.Domains = summary.Domains[:limit]
	}
	return summary, nil
}
