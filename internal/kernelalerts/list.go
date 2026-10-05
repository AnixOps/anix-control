package kernelalerts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
)

// Statuses a listing filters by.
const (
	StatusActive   = "active"
	StatusResolved = "resolved"
	StatusAll      = "all"
)

// Listing limits.
const (
	DefaultLimit = 100
	MaxLimit     = 500
)

// ErrInvalidQuery wraps every reason a listing query is refused.
var ErrInvalidQuery = errors.New("invalid alert query")

// Query selects alerts: Status is active (the default), resolved or all.
type Query struct {
	Status string
	// Kind and Severity filter when set.
	Kind     string
	Severity string
	Limit    int
}

// Alert is an alert as the admin API answers it.
type Alert struct {
	ID             uint           `json:"id"`
	Key            string         `json:"key"`
	Kind           string         `json:"kind"`
	Severity       string         `json:"severity"`
	Status         string         `json:"status"`
	SubjectKind    string         `json:"subject_kind"`
	Subject        string         `json:"subject"`
	Message        string         `json:"message"`
	Detail         map[string]any `json:"detail"`
	ExpiresAt      *time.Time     `json:"expires_at"`
	FirstSeenAt    time.Time      `json:"first_seen_at"`
	LastSeenAt     time.Time      `json:"last_seen_at"`
	LastNotifiedAt *time.Time     `json:"last_notified_at"`
	NotifyCount    int            `json:"notify_count"`
	ResolvedAt     *time.Time     `json:"resolved_at"`
}

// Summary counts the active alerts.
type Summary struct {
	Active   int64 `json:"active"`
	Critical int64 `json:"critical"`
	Warning  int64 `json:"warning"`
}

// Page is a listing: the alerts and the counts of all active alerts.
type Page struct {
	Alerts  []Alert `json:"alerts"`
	Summary Summary `json:"summary"`
}

// ParseQuery validates raw query values.
func ParseQuery(status, kind, severity, limit string) (Query, error) {
	query := Query{Status: status, Kind: kind, Severity: severity, Limit: DefaultLimit}
	switch status {
	case "":
		query.Status = StatusActive
	case StatusActive, StatusResolved, StatusAll:
	default:
		return Query{}, fmt.Errorf("%w: status must be active, resolved or all", ErrInvalidQuery)
	}
	switch severity {
	case "", model.KernelAlertWarning, model.KernelAlertCritical:
	default:
		return Query{}, fmt.Errorf("%w: severity must be warning or critical", ErrInvalidQuery)
	}
	if len(kind) > 48 {
		return Query{}, fmt.Errorf("%w: kind is too long", ErrInvalidQuery)
	}
	if limit != "" {
		parsed, err := strconv.Atoi(limit)
		if err != nil || parsed < 1 || parsed > MaxLimit {
			return Query{}, fmt.Errorf("%w: limit must be between 1 and %d", ErrInvalidQuery, MaxLimit)
		}
		query.Limit = parsed
	}
	return query, nil
}

// List answers the alerts of query, critical first and, within a severity,
// the soonest to end first, with the counts of all active alerts. A
// database without the alert table has none.
func List(ctx context.Context, db *gorm.DB, query Query) (Page, error) {
	page := Page{Alerts: []Alert{}}
	db = db.WithContext(ctx)
	if !db.Migrator().HasTable(&model.KernelAlert{}) {
		return page, nil
	}
	for _, count := range []struct {
		target   *int64
		severity string
	}{{&page.Summary.Active, ""}, {&page.Summary.Critical, model.KernelAlertCritical}, {&page.Summary.Warning, model.KernelAlertWarning}} {
		counted := db.Model(&model.KernelAlert{}).Where("resolved_at IS NULL")
		if count.severity != "" {
			counted = counted.Where("severity = ?", count.severity)
		}
		if err := counted.Count(count.target).Error; err != nil {
			return page, err
		}
	}
	rows := db.Model(&model.KernelAlert{})
	switch query.Status {
	case StatusResolved:
		rows = rows.Where("resolved_at IS NOT NULL")
	case StatusAll:
	default:
		rows = rows.Where("resolved_at IS NULL")
	}
	if query.Kind != "" {
		rows = rows.Where("kind = ?", query.Kind)
	}
	if query.Severity != "" {
		rows = rows.Where("severity = ?", query.Severity)
	}
	limit := query.Limit
	if limit <= 0 || limit > MaxLimit {
		limit = DefaultLimit
	}
	var found []model.KernelAlert
	err := rows.
		Order("CASE WHEN severity = 'critical' THEN 0 ELSE 1 END, COALESCE(expires_at, first_seen_at), id DESC").
		Limit(limit).Find(&found).Error
	if err != nil {
		return page, err
	}
	for _, row := range found {
		page.Alerts = append(page.Alerts, view(row))
	}
	return page, nil
}

func view(row model.KernelAlert) Alert {
	detail := map[string]any{}
	if row.Detail != "" {
		_ = json.Unmarshal([]byte(row.Detail), &detail)
	}
	status := StatusActive
	if row.ResolvedAt != nil {
		status = StatusResolved
	}
	return Alert{
		ID: row.ID, Key: row.Key, Kind: row.Kind, Severity: row.Severity, Status: status,
		SubjectKind: row.SubjectKind, Subject: row.Subject, Message: row.Message, Detail: detail,
		ExpiresAt: row.ExpiresAt, FirstSeenAt: row.FirstSeenAt, LastSeenAt: row.LastSeenAt,
		LastNotifiedAt: row.LastNotifiedAt, NotifyCount: row.NotifyCount, ResolvedAt: row.ResolvedAt,
	}
}
