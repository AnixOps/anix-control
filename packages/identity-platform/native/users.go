package native

import (
	"context"
	"strconv"
	"time"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
)

// Route ids of the administrator's user directory.
const (
	AdminUsersRouteID     = "identity.admin.users.get"
	AdminUserStatsRouteID = "identity.admin.users.stats.get"
)

// The v2 handlers' page sizes (handler.DefaultPageSize, handler.MaxPageSize).
const (
	defaultPageSize = 20
	maxPageSize     = 100
)

// AdminUsers is GET /api/v2/admin/users: one page of the user directory
// (UserDirectory.Search), filtered by e-mail, plan and status as the v2
// handler reads them, newest first. Each user carries the account and the
// subscription summary, never the subscription token or proxy uuid.
func (s *Service) AdminUsers(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	page, _ := strconv.Atoi(defaultQuery(request, "page", "1"))
	pageSize, _ := strconv.Atoi(defaultQuery(request, "page_size", "20"))
	page, pageSize = clampPagination(page, pageSize)
	query := UserQuery{
		Email: queryValue(request, "email"), Status: queryValue(request, "status"), Now: s.now(),
		Offset: (page - 1) * pageSize, Limit: pageSize,
	}
	if raw := queryValue(request, "plan_id"); raw != "" {
		// As v2: an id that does not parse is what ParseUint returns.
		planID, _ := strconv.ParseUint(raw, 10, 32)
		query.PlanID = &planID
	}
	stores, err := s.Open(ctx)
	if err != nil {
		return s.panelError("获取用户列表失败: " + err.Error())
	}
	total, users, err := stores.directory().Search(ctx, query)
	if err != nil {
		return s.panelError("获取用户列表失败: " + err.Error())
	}
	return s.panel(map[string]any{"total": total, "list": users})
}

// AdminUserStats is GET /api/v2/admin/users/stats: the directory's counts
// (UserDirectory.Count). "Active" is not banned in identity and not expired
// in Control; "today" starts at midnight UTC, as v2 counts it.
func (s *Service) AdminUserStats(ctx context.Context, _ pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	stores, err := s.Open(ctx)
	if err != nil {
		return s.panelError("获取统计失败: " + err.Error())
	}
	now := s.now()
	counts, err := stores.directory().Count(ctx, now, now.Truncate(24*time.Hour))
	if err != nil {
		return s.panelError("获取统计失败: " + err.Error())
	}
	return s.panel(map[string]any{
		"total_users":     counts.Total,
		"active_users":    counts.Active,
		"expired_users":   counts.Expired,
		"banned_users":    counts.Banned,
		"today_new_users": counts.Since,
	})
}

// getQuery is gin's Context.GetQuery on the forwarded query string.
func getQuery(request pluginhostsdk.NativeRequest, key string) (string, bool) {
	if values, ok := request.Metadata.Query[key]; ok && len(values) > 0 {
		return values[0], true
	}
	return "", false
}

// queryValue is gin's Context.Query.
func queryValue(request pluginhostsdk.NativeRequest, key string) string {
	value, _ := getQuery(request, key)
	return value
}

// defaultQuery is gin's Context.DefaultQuery: fallback only when the key is
// absent.
func defaultQuery(request pluginhostsdk.NativeRequest, key, fallback string) string {
	if value, ok := getQuery(request, key); ok {
		return value
	}
	return fallback
}

// clampPagination is the kernel's handler.ClampPagination.
func clampPagination(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = defaultPageSize
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}
	return page, pageSize
}
