package native

import (
	"context"
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"gorm.io/gorm"
)

// The kernel's StatsService windows and limits.
const (
	hourSeconds          = int64(3600)
	maxHourlyWindow      = 24 * 30
	defaultHours         = 24
	defaultRankingLimit  = 20
	maxRankingLimit      = 200
	maxZeroRankingLimit  = 1000
	maxInt64SQL          = "9223372036854775807"
	trafficLogView       = "kapi_traffic_log_v1"
	userDirectoryView    = "kapi_user_directory_v1"
	postgresDialectName  = "postgres"
	hourlyFailureMessage = "获取小时流量失败"
	metaFailureMessage   = "获取流量状态失败"
	rankFailureMessage   = "获取用户流量排行失败"
)

// HourlyTraffic is one hour of the series, as the kernel's
// service.HourlyTraffic.
type HourlyTraffic struct {
	HourTs  int64 `json:"hour_ts"`
	Traffic int64 `json:"traffic"`
}

// TrafficLogMeta is the kernel's service.TrafficLogMeta.
type TrafficLogMeta struct {
	LatestLogAt int64 `json:"latest_log_at"`
}

// UserTrafficRank is one ranking entry, as the kernel's
// service.UserTrafficRank.
type UserTrafficRank struct {
	UserID  uint   `json:"user_id"`
	Email   string `json:"email"`
	Traffic int64  `json:"traffic"`
}

// trafficAggregate is the kernel's StatsService.trafficLogAggregateExpr:
// the bytes of a set of reports times their rate, clamped to int64.
func trafficAggregate(db *gorm.DB, alias string) string {
	prefix := ""
	if alias != "" {
		prefix = alias + "."
	}
	u := prefix + "u"
	d := prefix + "d"
	rate := prefix + "rate"
	if db.Name() == postgresDialectName {
		return "FLOOR(LEAST(COALESCE(SUM((" +
			"GREATEST(COALESCE(" + u + ", 0), 0)::numeric + " +
			"GREATEST(COALESCE(" + d + ", 0), 0)::numeric" +
			") * (CASE WHEN " + rate + " > 0 THEN " + rate + "::numeric ELSE 1::numeric END)), 0), " +
			maxInt64SQL + "::numeric))::bigint"
	}
	return "CAST(MIN(COALESCE(SUM((" +
		"CAST(CASE WHEN " + u + " > 0 THEN " + u + " ELSE 0 END AS REAL) + " +
		"CAST(CASE WHEN " + d + " > 0 THEN " + d + " ELSE 0 END AS REAL)" +
		") * (CASE WHEN " + rate + " > 0 THEN " + rate + " ELSE 1 END)), 0), " +
		maxInt64SQL + ") AS INTEGER)"
}

// hourWindow is the kernel's window of the last hours whole local hours,
// the current one included: its first and last bucket and its end.
func hourWindow(now time.Time, hours int) (startHour, currentHour, endHour int64) {
	currentHour = time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, time.Local).Unix()
	startHour = currentHour - int64(hours-1)*hourSeconds
	return startHour, currentHour, currentHour + hourSeconds
}

// windowHours reads the hours query as the legacy handlers do: a positive
// number, else 24; the service caps it at 30 days.
func windowHours(request pluginhostsdk.NativeRequest) int {
	hours := defaultHours
	if v := query(request, "hours"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			hours = parsed
		}
	}
	if hours > maxHourlyWindow {
		hours = maxHourlyWindow
	}
	return hours
}

// HourlyTraffic is GET /api/v2/admin/traffic/hourly: the traffic of each of
// the last hours (default 24, at most 720), of all users or of user_id,
// and when traffic was last reported.
func (s *Service) HourlyTraffic(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	hours := windowHours(request)
	var userID uint
	if v := query(request, "user_id"); v != "" {
		if parsed, err := strconv.ParseUint(v, 10, 32); err == nil {
			userID = uint(parsed)
		}
	}
	db, err := s.Open(ctx)
	if err != nil {
		return failure(hourlyFailureMessage, err)
	}

	startHour, currentHour, endHour := hourWindow(s.now(), hours)
	var rows []struct {
		Hour    int64
		Traffic int64
	}
	buckets := db.Table(trafficLogView).Where("log_at >= ? AND log_at < ?", startHour, endHour)
	if userID > 0 {
		buckets = buckets.Where("user_id = ?", userID)
	}
	if err := buckets.
		Select("(log_at / ?) * ? AS hour, "+trafficAggregate(db, "")+" AS traffic", hourSeconds, hourSeconds).
		Group("hour").
		Order("hour ASC").
		Scan(&rows).Error; err != nil {
		return failure(hourlyFailureMessage, err)
	}
	byHour := make(map[int64]int64, len(rows))
	for _, row := range rows {
		byHour[row.Hour] = row.Traffic
	}
	series := make([]HourlyTraffic, 0, hours)
	for hour := startHour; hour <= currentHour; hour += hourSeconds {
		series = append(series, HourlyTraffic{HourTs: hour, Traffic: byHour[hour]})
	}

	var meta TrafficLogMeta
	var latest sql.NullInt64
	last := db.Table(trafficLogView)
	if userID > 0 {
		last = last.Where("user_id = ?", userID)
	}
	if err := last.Select("MAX(log_at)").Scan(&latest).Error; err != nil {
		return failure(metaFailureMessage, err)
	}
	if latest.Valid {
		meta.LatestLogAt = latest.Int64
	}
	return s.panel(map[string]any{"list": series, "meta": meta})
}

// UserTrafficRanking is GET /api/v2/admin/traffic/user-ranking: users by
// their traffic over the last hours (default 24), the top limit (default
// 20, at most 200). With include_zero_users it ranks every user, those
// without traffic too (at most 1000).
func (s *Service) UserTrafficRanking(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	hours := windowHours(request)
	limit := defaultRankingLimit
	if v := query(request, "limit"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	includeZeroUsers := query(request, "include_zero_users") == "true" || query(request, "include_zero_users") == "1"
	maxLimit := maxRankingLimit
	if includeZeroUsers {
		maxLimit = maxZeroRankingLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	db, err := s.Open(ctx)
	if err != nil {
		return failure(rankFailureMessage, err)
	}

	startHour, _, endHour := hourWindow(s.now(), hours)
	var ranks []UserTrafficRank
	if includeZeroUsers {
		traffic := db.Table(trafficLogView).
			Select("user_id, "+trafficAggregate(db, "")+" AS traffic").
			Where("log_at >= ? AND log_at < ?", startHour, endHour).
			Group("user_id")
		err = db.Table(userDirectoryView+" AS u").
			Select("u.id AS user_id, u.email AS email, COALESCE(t.traffic, 0) AS traffic").
			Joins("LEFT JOIN (?) AS t ON t.user_id = u.id", traffic).
			Order("traffic DESC, u.id ASC").
			Limit(limit).
			Scan(&ranks).Error
	} else {
		err = db.Table(trafficLogView+" AS l").
			Select("l.user_id AS user_id, COALESCE(u.email, '') AS email, "+trafficAggregate(db, "l")+" AS traffic").
			Joins("LEFT JOIN "+userDirectoryView+" AS u ON u.id = l.user_id").
			Where("l.log_at >= ? AND l.log_at < ?", startHour, endHour).
			Group("l.user_id, u.email").
			Order("traffic DESC").
			Limit(limit).
			Scan(&ranks).Error
	}
	if err != nil {
		return failure(rankFailureMessage, err)
	}
	return s.panel(map[string]any{"list": ranks})
}

// failure is the legacy handlers' 500 answer:
// c.JSON(500, gin.H{"message": message, "error": err.Error()}).
func failure(message string, err error) (pluginhostsdk.NativeResponse, error) {
	return jsonAnswer(http.StatusInternalServerError, map[string]any{"message": message, "error": err.Error()})
}
