// Package notificationcompat proves the notification package's native routes
// answer exactly as the kernel's legacy handlers, on SQLite and PostgreSQL:
// the same bytes (times the handlers take from their clock masked) and the
// same resulting rows.
//
// No case reaches the Telegram Bot API: the routes that call it run without a
// bot row, so both sides fail the same way before any request is made.
package notificationcompat

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/internal/handler"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/packagestore"
	"github.com/AnixOps/anix-control/v4/internal/tests/packagecompat"
	"github.com/AnixOps/anix-control/v4/packages/notification/native"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

var (
	admin  = pluginhostsdk.Principal{ActorID: 1, Admin: true}
	member = pluginhostsdk.Principal{ActorID: 2}
	nobody = pluginhostsdk.Principal{ActorID: 9}
)

func route(method, pattern, routeID string, legacy gin.HandlerFunc) packagecompat.Route {
	return packagecompat.Route{
		Method: method, Pattern: pattern, RouteID: routeID, Legacy: legacy,
		Models: []any{
			&model.User{}, &model.NotificationTemplate{}, &model.NotificationLog{},
			&model.TelegramBot{}, &model.TelegramUser{},
		},
		Native: func(db *gorm.DB) pluginhostsdk.NativeHandler {
			service := &native.Service{Open: func(ctx context.Context) (*gorm.DB, error) { return db.WithContext(ctx), nil }}
			return service.Handlers()[routeID]
		},
	}
}

// The legacy handlers are built per request: the Telegram handler's service
// keeps the database it was built with, which the harness sets up per case.
func notifications(method func(*handler.NotificationHandler, *gin.Context)) gin.HandlerFunc {
	return func(c *gin.Context) { method(handler.NewNotificationHandler(), c) }
}

func telegram(method func(*handler.TelegramHandler, *gin.Context)) gin.HandlerFunc {
	return func(c *gin.Context) { method(handler.NewTelegramHandler(), c) }
}

var seeded = time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)

func uintPtr(v uint) *uint { return &v }

func timePtr(v time.Time) *time.Time { return &v }

// seed writes members, templates, notification logs and Telegram bindings,
// but no bot: nothing can reach the Bot API.
func seed(t testing.TB, db *gorm.DB) {
	seedUsers(t, db)
	require.NoError(t, db.Create(&[]model.NotificationTemplate{
		{ID: 1, Name: "welcome", Type: "email", Event: "user.register", Title: "Hi", Content: "Welcome", Enabled: true, CreatedAt: seeded, UpdatedAt: seeded},
		{ID: 2, Name: "expire", Type: "telegram", Event: "user.expire", Title: "Soon", Content: "Renew", Enabled: true, CreatedAt: seeded, UpdatedAt: seeded},
		{ID: 3, Name: "paid", Type: "email", Event: "order.paid", Title: "Paid", Content: "Thanks", Enabled: true, CreatedAt: seeded, UpdatedAt: seeded},
	}).Error)
	require.NoError(t, db.Model(&model.NotificationTemplate{}).Where("id = ?", 3).UpdateColumn("enabled", false).Error)
	require.NoError(t, db.Create(&[]model.NotificationLog{
		{ID: 1, UserID: uintPtr(2), Type: "email", Event: "user.expire", Title: "t1", Content: "c1", Status: 1, SentAt: timePtr(seeded), CreatedAt: seeded},
		{ID: 2, UserID: uintPtr(2), Type: "telegram", Event: "user.traffic_low", Title: "t2", Content: "c2", Status: 2, Error: "boom", CreatedAt: seeded.Add(time.Hour)},
		{ID: 3, UserID: uintPtr(2), Type: "email", Event: "order.paid", Title: "t3", Content: "c3", Status: 1, SentAt: timePtr(seeded), ReadAt: timePtr(seeded.Add(3 * time.Hour)), CreatedAt: seeded.Add(2 * time.Hour)},
		{ID: 4, UserID: uintPtr(3), Type: "email", Event: "user.expire", Title: "t4", Content: "c4", Status: 0, CreatedAt: seeded.Add(3 * time.Hour)},
		{ID: 5, Type: "webhook", Event: "node.offline", Title: "t5", Content: "c5", Status: 1, SentAt: timePtr(seeded), CreatedAt: seeded.Add(4 * time.Hour)},
	}).Error)
	require.NoError(t, db.Create(&[]model.TelegramUser{
		{ID: 1, UserID: 2, TelegramID: 1002, Username: "two", FirstName: "Two", LanguageCode: "en", NotifyExpire: true, NotifyTraffic: true, NotifyTicket: true, MessageCount: 3, LastActive: seeded, CreatedAt: seeded, UpdatedAt: seeded},
		{ID: 2, UserID: 3, TelegramID: 1003, Username: "three", NotifyExpire: true, NotifyTraffic: true, NotifyTicket: true, CreatedAt: seeded.Add(time.Hour), UpdatedAt: seeded.Add(time.Hour)},
		{ID: 3, UserID: 4, TelegramID: 1004, Username: "four", IsBanned: true, BannedAt: timePtr(seeded), NotifyExpire: true, NotifyTraffic: true, NotifyTicket: true, CreatedAt: seeded.Add(2 * time.Hour), UpdatedAt: seeded.Add(2 * time.Hour)},
		{ID: 4, UserID: 5, TelegramID: 1005, Username: "five", NotifyExpire: true, NotifyTraffic: true, NotifyTicket: true, CreatedAt: seeded.Add(3 * time.Hour), UpdatedAt: seeded.Add(3 * time.Hour)},
	}).Error)
	// Member 3 takes only ticket notifications, member 5 none: the broadcast
	// skips member 5 (and the banned member 4).
	require.NoError(t, db.Model(&model.TelegramUser{}).Where("id = ?", 2).UpdateColumns(map[string]any{"notify_expire": false, "notify_traffic": false}).Error)
	require.NoError(t, db.Model(&model.TelegramUser{}).Where("id = ?", 4).UpdateColumns(map[string]any{"notify_expire": false, "notify_traffic": false, "notify_ticket": false}).Error)
	syncSequences(t, db)
}

// seedUsers writes the members (PostgreSQL enforces the kernel's foreign key
// from v2_telegram_user) and the kernel view the package reads them through.
func seedUsers(t testing.TB, db *gorm.DB) {
	for id := uint(1); id <= 5; id++ {
		require.NoError(t, db.Create(&model.User{ID: id, Email: fmt.Sprintf("u%d@example.test", id), Token: fmt.Sprintf("t%d", id), UUID: fmt.Sprintf("u%d", id)}).Error)
	}
	require.NoError(t, packagestore.EnsureKernelAPIViews(db))
	syncSequences(t, db)
}

// seedBot adds a bot row to seed, for the routes that only read or store it.
func seedBot(adminIDs string) func(t testing.TB, db *gorm.DB) {
	return func(t testing.TB, db *gorm.DB) {
		seed(t, db)
		require.NoError(t, db.Create(&model.TelegramBot{
			ID: 1, Name: "Bot", Token: "123:abc", Enabled: true, WebhookURL: "https://panel.example.test/api/v2/telegram/webhook", WebhookSet: true,
			AdminIDs: adminIDs, WelcomeMsg: "hello", AllowBind: true, AllowSub: true, AllowTicket: true, AllowInfo: true,
			TotalUsers: 4, TotalChats: 7, CreatedAt: seeded, UpdatedAt: seeded,
		}).Error)
		require.NoError(t, db.Model(&model.TelegramBot{}).Where("id = ?", 1).UpdateColumn("allow_sub", false).Error)
		syncSequences(t, db)
	}
}

// syncSequences moves PostgreSQL's id sequences past explicitly seeded ids,
// so both sides create the next row with the same id.
func syncSequences(t testing.TB, db *gorm.DB) {
	if db.Name() != "postgres" {
		return
	}
	for _, table := range []string{"v2_user", "v2_notification_template", "v2_notification_log", "v2_telegram_bot", "v2_telegram_user"} {
		require.NoError(t, db.Exec("SELECT setval(pg_get_serial_sequence(?, 'id'), COALESCE((SELECT MAX(id) FROM "+table+"), 0) + 1, false)", table).Error)
	}
}

// rows is the state after a write, without the times handlers take from
// their clock.
func rows(t testing.TB, db *gorm.DB) any {
	var templates []struct {
		ID      uint
		Name    string
		Type    string
		Event   string
		Title   string
		Content string
		Enabled bool
	}
	require.NoError(t, db.Model(&model.NotificationTemplate{}).Order("id").Find(&templates).Error)
	var logs []struct {
		ID     uint
		UserID *uint
		Status int
		Read   bool
	}
	require.NoError(t, db.Model(&model.NotificationLog{}).Select("id", "user_id", "status", "read_at IS NOT NULL AS read").Order("id").Scan(&logs).Error)
	var bots []struct {
		ID          uint
		Name        string
		Token       string
		Enabled     bool
		WebhookURL  string
		WebhookSet  bool
		AdminIDs    string
		WelcomeMsg  string
		AllowBind   bool
		AllowSub    bool
		AllowTicket bool
		AllowInfo   bool
		TotalUsers  int64
		TotalChats  int64
	}
	require.NoError(t, db.Model(&model.TelegramBot{}).Order("id").Find(&bots).Error)
	var bindings []struct {
		ID            uint
		UserID        uint
		TelegramID    int64
		Username      string
		IsBanned      bool
		NotifyExpire  bool
		NotifyTraffic bool
		NotifyTicket  bool
		MessageCount  int64
	}
	require.NoError(t, db.Model(&model.TelegramUser{}).Order("id").Find(&bindings).Error)
	var users []struct {
		ID    uint
		Email string
	}
	require.NoError(t, db.Model(&model.User{}).Order("id").Find(&users).Error)
	return map[string]any{"templates": templates, "logs": logs, "bots": bots, "bindings": bindings, "users": users}
}

func read(t *testing.T, r packagecompat.Route, cases []packagecompat.Case) {
	for _, c := range cases {
		if c.Seed == nil {
			c.Seed = seed
		}
		packagecompat.RunRead(t, r, c)
	}
}

func write(t *testing.T, r packagecompat.Route, cases []packagecompat.Case) {
	for _, c := range cases {
		if c.Seed == nil {
			c.Seed = seed
		}
		c.Snapshot = rows
		packagecompat.RunWrite(t, r, c)
	}
}

func TestUserNotificationRoutesParity(t *testing.T) {
	read(t, route("GET", "/api/v2/user/notifications", "notification.user.notifications.get", notifications((*handler.NotificationHandler).GetUserNotifications)), []packagecompat.Case{
		{Name: "own notifications newest first", Path: "/api/v2/user/notifications", Principal: member},
		{Name: "second page", Path: "/api/v2/user/notifications?page=2&page_size=2", Principal: member},
		{Name: "page size zero", Path: "/api/v2/user/notifications?page_size=0", Principal: member},
		{Name: "page that is not a number", Path: "/api/v2/user/notifications?page=x", Principal: member},
		{Name: "no notifications", Path: "/api/v2/user/notifications", Principal: nobody},
	})
	read(t, route("GET", "/api/v2/user/notifications/unread-count", "notification.user.notifications.unread_count.get", notifications((*handler.NotificationHandler).GetUnreadCount)), []packagecompat.Case{
		{Name: "unread count", Path: "/api/v2/user/notifications/unread-count", Principal: member},
		{Name: "nothing unread", Path: "/api/v2/user/notifications/unread-count", Principal: nobody},
	})
	write(t, route("POST", "/api/v2/user/notifications/:id/read", "notification.user.notifications.id.read.post", notifications((*handler.NotificationHandler).MarkAsRead)), []packagecompat.Case{
		{Name: "mark own notification", Path: "/api/v2/user/notifications/1/read", Principal: member},
		{Name: "mark one already read", Path: "/api/v2/user/notifications/3/read", Principal: member},
		{Name: "another member's notification", Path: "/api/v2/user/notifications/4/read", Principal: member},
		{Name: "unknown notification", Path: "/api/v2/user/notifications/99/read", Principal: member},
		{Name: "id that is not a number", Path: "/api/v2/user/notifications/x/read", Principal: member},
	})
	write(t, route("POST", "/api/v2/user/notifications/read-all", "notification.user.notifications.read_all.post", notifications((*handler.NotificationHandler).MarkAllAsRead)), []packagecompat.Case{
		{Name: "mark all", Path: "/api/v2/user/notifications/read-all", Principal: member},
		{Name: "nothing to mark", Path: "/api/v2/user/notifications/read-all", Principal: nobody},
	})
}

func TestAdminTemplateRoutesParity(t *testing.T) {
	read(t, route("GET", "/api/v2/admin/notification/templates", "notification.admin.notification.templates.get", notifications((*handler.NotificationHandler).ListTemplates)), []packagecompat.Case{
		{Name: "every template", Path: "/api/v2/admin/notification/templates", Principal: admin},
		{Name: "by type", Path: "/api/v2/admin/notification/templates?type=email", Principal: admin},
		{Name: "no template of the type", Path: "/api/v2/admin/notification/templates?type=webhook", Principal: admin},
		{Name: "no templates", Path: "/api/v2/admin/notification/templates", Principal: admin, Seed: seedUsers},
	})
	write(t, route("POST", "/api/v2/admin/notification/templates", "notification.admin.notification.templates.post", notifications((*handler.NotificationHandler).CreateTemplate)), []packagecompat.Case{
		{Name: "create", Path: "/api/v2/admin/notification/templates", Principal: admin, Mask: []string{"data.created_at", "data.updated_at"},
			Body: []byte(`{"name":"n","type":"email","event":"e","title":"t","content":"c","enabled":true}`)},
		{Name: "create disabled keeps the column default", Path: "/api/v2/admin/notification/templates", Principal: admin, Mask: []string{"data.created_at", "data.updated_at"},
			Body: []byte(`{"name":"n","type":"telegram","event":"e","title":"t","content":"c","enabled":false}`)},
		{Name: "missing fields", Path: "/api/v2/admin/notification/templates", Principal: admin, Body: []byte(`{"name":"n","content":"c"}`)},
		{Name: "wrong field type", Path: "/api/v2/admin/notification/templates", Principal: admin, Body: []byte(`{"name":1}`)},
		{Name: "no body", Path: "/api/v2/admin/notification/templates", Principal: admin},
	})
	write(t, route("PUT", "/api/v2/admin/notification/templates/:id", "notification.admin.notification.templates.id.put", notifications((*handler.NotificationHandler).UpdateTemplate)), []packagecompat.Case{
		{Name: "update", Path: "/api/v2/admin/notification/templates/1", Principal: admin, Mask: []string{"data.updated_at"},
			Body: []byte(`{"type":"webhook","event":"ev","name":"renamed","title":"T","content":"C","enabled":false}`)},
		{Name: "enable only", Path: "/api/v2/admin/notification/templates/3", Principal: admin, Mask: []string{"data.updated_at"}, Body: []byte(`{"enabled":true}`)},
		{Name: "empty body keeps everything", Path: "/api/v2/admin/notification/templates/2", Principal: admin, Mask: []string{"data.updated_at"}, Body: []byte(`{}`)},
		{Name: "invalid type", Path: "/api/v2/admin/notification/templates/1", Principal: admin, Body: []byte(`{"type":"sms"}`)},
		{Name: "unknown template", Path: "/api/v2/admin/notification/templates/99", Principal: admin, Body: []byte(`{"name":"x"}`)},
		{Name: "unknown template with a bad body", Path: "/api/v2/admin/notification/templates/99", Principal: admin, Body: []byte(`{"type":"sms"}`)},
		{Name: "id that is not a number", Path: "/api/v2/admin/notification/templates/x", Principal: admin, Body: []byte(`{"name":"x"}`)},
	})
	write(t, route("DELETE", "/api/v2/admin/notification/templates/:id", "notification.admin.notification.templates.id.delete", notifications((*handler.NotificationHandler).DeleteTemplate)), []packagecompat.Case{
		{Name: "delete", Path: "/api/v2/admin/notification/templates/2", Principal: admin},
		{Name: "delete an unknown template", Path: "/api/v2/admin/notification/templates/99", Principal: admin},
		{Name: "delete by an id that is not a number", Path: "/api/v2/admin/notification/templates/x", Principal: admin},
	})
}

func TestAdminLogRoutesParity(t *testing.T) {
	read(t, route("GET", "/api/v2/admin/notification/logs", "notification.admin.notification.logs.get", notifications((*handler.NotificationHandler).ListLogs)), []packagecompat.Case{
		{Name: "every log newest first", Path: "/api/v2/admin/notification/logs", Principal: admin},
		{Name: "by type", Path: "/api/v2/admin/notification/logs?type=email", Principal: admin},
		{Name: "by status name", Path: "/api/v2/admin/notification/logs?status=failed", Principal: admin},
		{Name: "by status code", Path: "/api/v2/admin/notification/logs?status=1", Principal: admin},
		{Name: "unknown status is ignored", Path: "/api/v2/admin/notification/logs?status=bogus", Principal: admin},
		{Name: "second page", Path: "/api/v2/admin/notification/logs?page=2&page_size=2", Principal: admin},
		{Name: "clamped page", Path: "/api/v2/admin/notification/logs?page=0&page_size=500", Principal: admin},
		{Name: "no logs", Path: "/api/v2/admin/notification/logs", Principal: admin, Seed: seedUsers},
	})
}

func TestAdminTelegramBotRoutesParity(t *testing.T) {
	read(t, route("GET", "/api/v2/admin/telegram/bot", "notification.admin.telegram.bot.get", telegram((*handler.TelegramHandler).GetBot)), []packagecompat.Case{
		{Name: "no bot answers the defaults", Path: "/api/v2/admin/telegram/bot", Principal: admin},
		{Name: "bot", Path: "/api/v2/admin/telegram/bot", Principal: admin, Seed: seedBot("[11,22]")},
		{Name: "comma separated admin ids", Path: "/api/v2/admin/telegram/bot", Principal: admin, Seed: seedBot("11, 22,x")},
		{Name: "unreadable admin ids", Path: "/api/v2/admin/telegram/bot", Principal: admin, Seed: seedBot("abc")},
	})
	write(t, route("PUT", "/api/v2/admin/telegram/bot", "notification.admin.telegram.bot.put", telegram((*handler.TelegramHandler).UpdateBot)), []packagecompat.Case{
		{Name: "first update creates the bot", Path: "/api/v2/admin/telegram/bot", Principal: admin, Mask: []string{"data.created_at", "data.updated_at"},
			Body: []byte(`{"token":"1:x","welcome_message":"hi","admin_ids":"5,6","allow_bind":false}`)},
		{Name: "first update without a name", Path: "/api/v2/admin/telegram/bot", Principal: admin, Mask: []string{"data.created_at", "data.updated_at"}, Body: []byte(`{}`)},
		{Name: "update", Path: "/api/v2/admin/telegram/bot", Principal: admin, Seed: seedBot("[11]"), Mask: []string{"data.updated_at"},
			Body: []byte(`{"name":"Renamed","welcome_msg":"w","admin_ids":[1,"2"],"allow_sub":true,"allow_ticket":false,"allow_info":false}`)},
		{Name: "invalid admin id", Path: "/api/v2/admin/telegram/bot", Principal: admin, Seed: seedBot("[11]"), Body: []byte(`{"admin_ids":["x"]}`)},
		{Name: "invalid admin id element", Path: "/api/v2/admin/telegram/bot", Principal: admin, Seed: seedBot("[11]"), Body: []byte(`{"admin_ids":[true]}`)},
		{Name: "invalid admin ids", Path: "/api/v2/admin/telegram/bot", Principal: admin, Seed: seedBot("[11]"), Body: []byte(`{"admin_ids":{"a":1}}`)},
		{Name: "name too long", Path: "/api/v2/admin/telegram/bot", Principal: admin, Body: []byte(`{"name":"` + strings.Repeat("n", 256) + `"}`)},
		{Name: "wrong field type", Path: "/api/v2/admin/telegram/bot", Principal: admin, Body: []byte(`{"allow_bind":"yes"}`)},
	})
	write(t, route("DELETE", "/api/v2/admin/telegram/webhook", "notification.admin.telegram.webhook.delete", telegram((*handler.TelegramHandler).DeleteWebhook)), []packagecompat.Case{
		{Name: "no bot", Path: "/api/v2/admin/telegram/webhook", Principal: admin},
	})
}

func TestAdminTelegramMessageRoutesParity(t *testing.T) {
	write(t, route("POST", "/api/v2/admin/telegram/notify", "notification.admin.telegram.notify.post", telegram((*handler.TelegramHandler).SendNotification)), []packagecompat.Case{
		{Name: "no bot", Path: "/api/v2/admin/telegram/notify", Principal: admin, Body: []byte(`{"telegram_id":1002,"message":"hi"}`)},
		{Name: "title and content without a bot", Path: "/api/v2/admin/telegram/notify", Principal: admin, Body: []byte(`{"telegram_id":"1002","title":"T","content":"C"}`)},
		{Name: "nested message without a bot", Path: "/api/v2/admin/telegram/notify", Principal: admin, Body: []byte(`{"telegram_id":1002,"message":{"message":" hi "}}`)},
		{Name: "missing telegram id", Path: "/api/v2/admin/telegram/notify", Principal: admin, Body: []byte(`{"message":"hi"}`)},
		{Name: "telegram id that is not a number", Path: "/api/v2/admin/telegram/notify", Principal: admin, Body: []byte(`{"telegram_id":"abc","message":"hi"}`)},
		{Name: "zero telegram id", Path: "/api/v2/admin/telegram/notify", Principal: admin, Body: []byte(`{"telegram_id":0,"message":"hi"}`)},
		{Name: "no message", Path: "/api/v2/admin/telegram/notify", Principal: admin, Body: []byte(`{"telegram_id":1002,"title":" ","content":""}`)},
		{Name: "not an object", Path: "/api/v2/admin/telegram/notify", Principal: admin, Body: []byte(`[]`)},
	})
	write(t, route("POST", "/api/v2/admin/telegram/broadcast", "notification.admin.telegram.broadcast.post", telegram((*handler.TelegramHandler).Broadcast)), []packagecompat.Case{
		{Name: "every eligible member fails without a bot", Path: "/api/v2/admin/telegram/broadcast", Principal: admin, Body: []byte(`{"message":"hi"}`)},
		{Name: "no eligible member", Path: "/api/v2/admin/telegram/broadcast", Principal: admin, Seed: seedUsers, Body: []byte(`{"message":{"message":"hi"}}`)},
		{Name: "empty broadcast", Path: "/api/v2/admin/telegram/broadcast", Principal: admin, Body: []byte(`{"message":"  "}`)},
		{Name: "invalid json", Path: "/api/v2/admin/telegram/broadcast", Principal: admin, Body: []byte(`{`)},
	})
}

func TestAdminTelegramBindingRoutesParity(t *testing.T) {
	read(t, route("GET", "/api/v2/admin/telegram/users", "notification.admin.telegram.users.get", telegram((*handler.TelegramHandler).GetUserBindings)), []packagecompat.Case{
		{Name: "first page newest first", Path: "/api/v2/admin/telegram/users", Principal: admin},
		{Name: "every binding", Path: "/api/v2/admin/telegram/users?all=TRUE", Principal: admin},
		{Name: "second page", Path: "/api/v2/admin/telegram/users?page=2&page_size=3", Principal: admin},
		{Name: "clamped page", Path: "/api/v2/admin/telegram/users?page=-1&page_size=1000", Principal: admin},
		{Name: "no bindings", Path: "/api/v2/admin/telegram/users", Principal: admin, Seed: seedUsers},
		{Name: "no bindings at all", Path: "/api/v2/admin/telegram/users?all=true", Principal: admin, Seed: seedUsers},
	})
	write(t, route("PUT", "/api/v2/admin/telegram/users/:id/notify", "notification.admin.telegram.users.id.notify.put", telegram((*handler.TelegramHandler).UpdateUserNotify)), []packagecompat.Case{
		{Name: "switch all off", Path: "/api/v2/admin/telegram/users/1/notify", Principal: admin, Mask: []string{"data.updated_at"}, Body: []byte(`{"notify_enabled":false}`)},
		{Name: "one switch wins over notify_enabled", Path: "/api/v2/admin/telegram/users/2/notify", Principal: admin, Mask: []string{"data.updated_at"},
			Body: []byte(`{"notify_enabled":false,"notify_expire":true}`)},
		{Name: "each switch", Path: "/api/v2/admin/telegram/users/4/notify", Principal: admin, Mask: []string{"data.updated_at"},
			Body: []byte(`{"notify_expire":true,"notify_traffic":false,"notify_ticket":true}`)},
		{Name: "not a boolean", Path: "/api/v2/admin/telegram/users/1/notify", Principal: admin, Body: []byte(`{"notify_ticket":"x"}`)},
		{Name: "notify_enabled not a boolean", Path: "/api/v2/admin/telegram/users/1/notify", Principal: admin, Body: []byte(`{"notify_enabled":1}`)},
		{Name: "no fields", Path: "/api/v2/admin/telegram/users/1/notify", Principal: admin, Body: []byte(`{}`)},
		{Name: "unknown binding", Path: "/api/v2/admin/telegram/users/99/notify", Principal: admin, Body: []byte(`{"notify_enabled":true}`)},
		{Name: "invalid id", Path: "/api/v2/admin/telegram/users/x/notify", Principal: admin, Body: []byte(`{"notify_enabled":true}`)},
		{Name: "invalid json", Path: "/api/v2/admin/telegram/users/1/notify", Principal: admin, Body: []byte(`{`)},
	})
}

func TestUserTelegramRoutesParity(t *testing.T) {
	read(t, route("GET", "/api/v2/user/telegram/status", "notification.user.telegram.status.get", telegram((*handler.TelegramHandler).GetTelegramStatus)), []packagecompat.Case{
		{Name: "bound", Path: "/api/v2/user/telegram/status", Principal: member},
		{Name: "not bound", Path: "/api/v2/user/telegram/status", Principal: nobody},
	})
	write(t, route("POST", "/api/v2/user/telegram/unbind", "notification.user.telegram.unbind.post", telegram((*handler.TelegramHandler).UnbindTelegram)), []packagecompat.Case{
		{Name: "unbind changes nothing", Path: "/api/v2/user/telegram/unbind", Principal: member},
	})
	write(t, route("POST", "/api/v2/user/telegram/notify", "notification.user.telegram.notify.post", telegram((*handler.TelegramHandler).UpdateNotifySettings)), []packagecompat.Case{
		{Name: "settings change nothing", Path: "/api/v2/user/telegram/notify", Principal: member, Body: []byte(`{"notify_expire":false}`)},
		{Name: "wrong field type", Path: "/api/v2/user/telegram/notify", Principal: member, Body: []byte(`{"notify_expire":"no"}`)},
		{Name: "no body", Path: "/api/v2/user/telegram/notify", Principal: member},
	})
}
