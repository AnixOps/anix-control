// Package shadowsamples stores the sanitized shadow-mode mismatch samples
// package hosts report, and serves them to the route-mode administration.
//
// A package host (sdk/pluginhostsdk.Router) keeps its latest mismatches,
// sanitized by sdk/shadowsample, in the shadow_samples list of its Health
// details document. The kernel polls Health; Collector reads the samples
// from each poll, drops those for routes the package does not own in the
// package extraction map, sanitizes every field again, replaces the path
// with the route's path template, and stores each sample once. A host has
// no other way to write the table, and only the fixed sample columns.
//
// Retention: samples are deleted after Retention, and only the newest
// MaxPerRoute of a route are kept.
package shadowsamples

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/AnixOps/anix-control/sdk/shadowsample"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	// Retention is how long a sample is kept.
	Retention = 7 * 24 * time.Hour
	// MaxPerRoute is how many samples of one route are kept.
	MaxPerRoute = 100
	// MaxPerReport bounds the samples read from one Health details
	// document; a host reports at most 32.
	MaxPerReport = 32
	// maxDetailsBytes bounds the Health details document that is parsed.
	maxDetailsBytes = 2 << 20
	// futureSkew is how far in the future a sample's time may be before
	// it is clamped to the collection time.
	futureSkew = time.Minute
)

// RouteInfo is what the kernel knows about a package route from the package
// extraction map.
type RouteInfo struct {
	PackageID string
	// Path is the route's legacy path template.
	Path string
}

// Report is one package host's Health details document.
type Report struct {
	PackageID   string
	Version     string
	DetailsJSON string
	// CheckedAt is when the kernel read the document; Collect skips a
	// report it has already read.
	CheckedAt time.Time
}

// Collector stores the samples of Health reports.
type Collector struct {
	DB *gorm.DB
	// Route looks a route up in the package extraction map.
	Route func(routeID string) (RouteInfo, bool)
	// Now defaults to time.Now.
	Now func() time.Time

	mu   sync.Mutex
	read map[string]time.Time
}

// Collect ingests every report not read before and returns how many samples
// it stored. A failing report does not stop the others; the first error is
// returned.
func (c *Collector) Collect(ctx context.Context, reports []Report) (int, error) {
	if c == nil {
		return 0, errors.New("shadow sample collector is not configured")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.read == nil {
		c.read = map[string]time.Time{}
	}
	stored := 0
	var first error
	current := make(map[string]time.Time, len(reports))
	for _, report := range reports {
		key := report.PackageID + "@" + report.Version
		if previous, ok := c.read[key]; ok && !report.CheckedAt.IsZero() && report.CheckedAt.Equal(previous) {
			current[key] = previous
			continue
		}
		count, err := c.Ingest(ctx, report)
		stored += count
		if err != nil {
			if first == nil {
				first = err
			}
			continue
		}
		current[key] = report.CheckedAt
	}
	c.read = current
	return stored, first
}

func (c *Collector) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

type details struct {
	ShadowSamples []json.RawMessage `json:"shadow_samples"`
}

// Ingest stores the new samples of one report and returns how many it
// stored. Samples of routes the reporting package does not own, malformed
// samples, and samples older than Retention are dropped.
func (c *Collector) Ingest(ctx context.Context, report Report) (int, error) {
	if c == nil || c.DB == nil || c.Route == nil {
		return 0, errors.New("shadow sample collector is not configured")
	}
	if report.PackageID == "" || report.DetailsJSON == "" || len(report.DetailsJSON) > maxDetailsBytes {
		return 0, nil
	}
	var document details
	if err := json.Unmarshal([]byte(report.DetailsJSON), &document); err != nil || len(document.ShadowSamples) == 0 {
		return 0, nil
	}
	now := c.now().UTC()
	oldest := now.Add(-Retention)
	rows := make([]model.ShadowMismatchSample, 0, len(document.ShadowSamples))
	seen := map[string]struct{}{}
	for _, raw := range document.ShadowSamples {
		row, ok := c.sanitize(report, raw, now, oldest)
		if !ok {
			continue
		}
		if _, duplicate := seen[row.SampleID]; duplicate {
			continue
		}
		seen[row.SampleID] = struct{}{}
		rows = append(rows, row)
	}
	// Keep the newest when a report carries more than MaxPerReport.
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].ObservedAt.After(rows[j].ObservedAt) })
	if len(rows) > MaxPerReport {
		rows = rows[:MaxPerReport]
	}
	if len(rows) == 0 {
		return 0, nil
	}
	db := c.DB.WithContext(ctx)
	ids := make([]string, len(rows))
	for index, row := range rows {
		ids[index] = row.SampleID
	}
	var existing []string
	if err := db.Model(&model.ShadowMismatchSample{}).Where("package_id = ? AND sample_id IN ?", report.PackageID, ids).Pluck("sample_id", &existing).Error; err != nil {
		return 0, err
	}
	known := make(map[string]struct{}, len(existing))
	for _, id := range existing {
		known[id] = struct{}{}
	}
	fresh := rows[:0]
	for _, row := range rows {
		if _, ok := known[row.SampleID]; !ok {
			fresh = append(fresh, row)
		}
	}
	if len(fresh) == 0 {
		return 0, nil
	}
	if err := db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "package_id"}, {Name: "sample_id"}}, DoNothing: true,
	}).Create(&fresh).Error; err != nil {
		return 0, err
	}
	routes := map[string]struct{}{}
	for _, row := range fresh {
		routes[row.RouteID] = struct{}{}
	}
	for route := range routes {
		if err := trimRoute(db, report.PackageID, route); err != nil {
			return len(fresh), err
		}
	}
	return len(fresh), nil
}

// sanitize turns one reported sample into a row, or rejects it.
func (c *Collector) sanitize(report Report, raw json.RawMessage, now, oldest time.Time) (model.ShadowMismatchSample, bool) {
	var reported shadowsample.Sample
	if err := json.Unmarshal(raw, &reported); err != nil {
		return model.ShadowMismatchSample{}, false
	}
	if !shadowsample.ValidID(reported.ID) || !shadowsample.ValidRouteID(reported.RouteID) {
		return model.ShadowMismatchSample{}, false
	}
	route, ok := c.Route(reported.RouteID)
	if !ok || route.PackageID != report.PackageID {
		return model.ShadowMismatchSample{}, false
	}
	observedAt := time.Unix(reported.ObservedAt, 0).UTC()
	if observedAt.After(now.Add(futureSkew)) {
		observedAt = now
	}
	if observedAt.Before(oldest) {
		return model.ShadowMismatchSample{}, false
	}
	clean := shadowsample.Sanitize(reported)
	// The route's path template replaces whatever path the host reported;
	// only the query keys are kept, with masked values.
	if route.Path != "" {
		clean.Path = route.Path + shadowsample.MaskedQuery(shadowsample.QueryKeys(clean.Path))
		clean = shadowsample.Sanitize(clean)
	}
	diff, err := json.Marshal(clean.Diff)
	if err != nil {
		return model.ShadowMismatchSample{}, false
	}
	return model.ShadowMismatchSample{
		PackageID: report.PackageID, SampleID: clean.ID, RouteID: clean.RouteID,
		PackageVersion: truncate(shadowsample.SanitizeString(report.Version), 64),
		Method:         clean.Method, Path: clean.Path,
		LegacyStatus: statusCode(clean.LegacyStatus), NativeStatus: statusCode(clean.NativeStatus),
		DiffJSON: string(diff), DiffTruncated: clean.DiffTruncated, RequestID: clean.RequestID,
		ObservedAt: observedAt, CreatedAt: now,
	}, true
}

func statusCode(code uint32) int {
	if code > 999 {
		return 0
	}
	return int(code)
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return strings.ToValidUTF8(value[:limit], "")
}

// trimRoute keeps the newest MaxPerRoute samples of a route.
func trimRoute(db *gorm.DB, packageID, routeID string) error {
	var cutoff model.ShadowMismatchSample
	err := db.Select("id", "observed_at").
		Where("package_id = ? AND route_id = ?", packageID, routeID).
		Order("observed_at DESC").Order("id DESC").Offset(MaxPerRoute - 1).Limit(1).
		Take(&cutoff).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return db.Where("package_id = ? AND route_id = ? AND (observed_at < ? OR (observed_at = ? AND id < ?))",
		packageID, routeID, cutoff.ObservedAt, cutoff.ObservedAt, cutoff.ID).
		Delete(&model.ShadowMismatchSample{}).Error
}

// Prune deletes samples older than Retention and trims every route to
// MaxPerRoute samples.
func Prune(db *gorm.DB, now time.Time) (int64, error) {
	result := db.Where("observed_at < ?", now.UTC().Add(-Retention)).Delete(&model.ShadowMismatchSample{})
	if result.Error != nil {
		return 0, result.Error
	}
	var crowded []struct {
		PackageID string
		RouteID   string
	}
	if err := db.Model(&model.ShadowMismatchSample{}).Select("package_id, route_id").
		Group("package_id, route_id").Having("COUNT(*) > ?", MaxPerRoute).Scan(&crowded).Error; err != nil {
		return result.RowsAffected, err
	}
	for _, route := range crowded {
		if err := trimRoute(db, route.PackageID, route.RouteID); err != nil {
			return result.RowsAffected, err
		}
	}
	return result.RowsAffected, nil
}

// Sample is a stored sample as the administration shows it.
type Sample struct {
	ID             uint                     `json:"id"`
	SampleID       string                   `json:"sample_id"`
	PackageID      string                   `json:"package_id"`
	PackageVersion string                   `json:"package_version"`
	RouteID        string                   `json:"route_id"`
	Method         string                   `json:"method"`
	Path           string                   `json:"path"`
	LegacyStatus   int                      `json:"legacy_status"`
	NativeStatus   int                      `json:"native_status"`
	Diff           []shadowsample.DiffEntry `json:"diff"`
	DiffTruncated  bool                     `json:"diff_truncated"`
	RequestID      string                   `json:"request_id"`
	ObservedAt     time.Time                `json:"observed_at"`
}

// Filter selects samples.
type Filter struct {
	PackageID string
	RouteID   string
	// Limit defaults to DefaultLimit and is at most MaxPerRoute.
	Limit int
}

// DefaultLimit is how many samples List returns without a limit.
const DefaultLimit = 50

// List returns the newest samples matching filter.
func List(db *gorm.DB, filter Filter) ([]Sample, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = DefaultLimit
	}
	limit = min(limit, MaxPerRoute)
	query := db.Model(&model.ShadowMismatchSample{})
	if filter.PackageID != "" {
		query = query.Where("package_id = ?", filter.PackageID)
	}
	if filter.RouteID != "" {
		query = query.Where("route_id = ?", filter.RouteID)
	}
	var rows []model.ShadowMismatchSample
	if err := query.Order("observed_at DESC").Order("id DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	samples := make([]Sample, 0, len(rows))
	for _, row := range rows {
		diff := []shadowsample.DiffEntry{}
		if row.DiffJSON != "" {
			if err := json.Unmarshal([]byte(row.DiffJSON), &diff); err != nil {
				diff = []shadowsample.DiffEntry{}
			}
		}
		samples = append(samples, Sample{
			ID: row.ID, SampleID: row.SampleID, PackageID: row.PackageID, PackageVersion: row.PackageVersion,
			RouteID: row.RouteID, Method: row.Method, Path: row.Path, LegacyStatus: row.LegacyStatus,
			NativeStatus: row.NativeStatus, Diff: diff, DiffTruncated: row.DiffTruncated, RequestID: row.RequestID,
			ObservedAt: row.ObservedAt,
		})
	}
	return samples, nil
}

// Summary is what is stored about one route's mismatches.
type Summary struct {
	Stored         int64     `json:"stored"`
	LastObservedAt time.Time `json:"last_observed_at"`
}

// Summaries returns, by package and route, how many samples are stored and
// when the newest was observed. An empty packageID means every package.
func Summaries(db *gorm.DB, packageID string) (map[string]map[string]Summary, error) {
	query := db.Model(&model.ShadowMismatchSample{}).Select("package_id, route_id, COUNT(*) AS stored, MAX(observed_at) AS last_observed_at")
	if packageID != "" {
		query = query.Where("package_id = ?", packageID)
	}
	rows, err := query.Group("package_id, route_id").Rows()
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[string]map[string]Summary{}
	for rows.Next() {
		var packageID, routeID string
		var stored int64
		var last any
		if err := rows.Scan(&packageID, &routeID, &stored, &last); err != nil {
			return nil, err
		}
		if out[packageID] == nil {
			out[packageID] = map[string]Summary{}
		}
		out[packageID][routeID] = Summary{Stored: stored, LastObservedAt: scanTime(last)}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// scanTime reads MAX(observed_at): PostgreSQL returns a time, SQLite the
// stored text.
func scanTime(value any) time.Time {
	switch typed := value.(type) {
	case time.Time:
		return typed.UTC()
	case string:
		return parseTime(typed)
	case []byte:
		return parseTime(string(typed))
	}
	return time.Time{}
}

func parseTime(value string) time.Time {
	for _, layout := range []string{"2006-01-02 15:04:05.999999999-07:00", time.RFC3339Nano, "2006-01-02 15:04:05.999999999", "2006-01-02T15:04:05.999999999"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC()
		}
	}
	return time.Time{}
}
