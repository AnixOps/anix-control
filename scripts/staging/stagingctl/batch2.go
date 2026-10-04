package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
)

// Batch 2: notification + platform + machine-telemetry + protocol-runtime.
//
// Network: the staging stack has no route out, so the routes that call out
// (the Telegram Bot API for notify, broadcast and the webhook; SMTP for the
// test e-mail) fail on both twins the same way. The seeded SMTP host is an
// IP literal from 192.0.2.0/24, so the test e-mail fails at once with
// "network is unreachable" instead of waiting for a DNS answer.

func init() {
	registerSeed(70, "notification templates and logs", seedB2Notifications)
	registerSeed(71, "telegram bot and bindings", seedB2Telegram)
	registerSeed(72, "e-mail notification configuration", seedB2EmailConfig)
	registerSeed(73, "system audit log, backup configuration and records", seedB2Platform)
	registerSeed(74, "agent diagnostic tasks", seedB2AgentTasks)
	registerSeed(75, "traffic log relative to now", seedB2Traffic)

	registerTables("notification",
		TableSpec{Table: "v2_notification_template", Ignore: timestamps()},
		TableSpec{Table: "v2_notification_log", Ignore: map[string]string{
			"read_at":    "set from the handler's clock (whether a row is read is compared by the read_at IS NULL spec)",
			"created_at": "rows a login adds are stamped from the handler's clock",
		}},
		// Only the unread rows: a row one twin marked read and the other
		// did not is on one side only.
		TableSpec{Table: "v2_notification_log", Where: "read_at IS NULL", Ignore: map[string]string{
			"created_at": "rows a login adds are stamped from the handler's clock",
		}},
		TableSpec{Table: "v2_telegram_bot", Ignore: timestamps()},
		TableSpec{Table: "v2_telegram_user", Ignore: timestamps()},
		TableSpec{Table: "v2_system_config", Key: "key", Where: "key = 'notification.email.config'", Ignore: timestamps(
			"id", "the row is keyed by its key")},
	)
	registerTables("platform",
		TableSpec{Table: "v2_backup_config", Ignore: timestamps()},
		// The system audit entries of the e-mail and backup configuration
		// updates. The native twin's route-mode switch records module
		// "kernel" entries from the same sequence, so the ids of the new
		// rows differ: rows are keyed by what they record instead.
		TableSpec{Table: "v2_operation_log", Key: "target_type,target_id,action,user_id,content", Where: "module = 'system'",
			Ignore: map[string]string{
				"id":         "the native twin's route-mode audit entries (module kernel) take ids from the same sequence",
				"created_at": "set from the handler's clock",
			}},
	)

	registerSpecs(b2NotificationSpecs()...)
	registerSpecs(b2PlatformSpecs()...)
	registerSpecs(b2TelemetrySpecs()...)
	registerSpecs(b2ProtocolSpecs()...)
	registerSpecs(b2ProtocolNodeSpecs()...)
}

// ---------------------------------------------------------------- seeds

// seedB2Notifications writes notification templates of every type (some
// disabled) and notification logs: the user persona's read and unread
// notifications, user2's, other members', an administrator's and system
// broadcasts without a user, in every delivery status. Every log has its
// own created_at, so the newest-first lists have no ties.
func seedB2Notifications(s *Seeder) {
	events := []string{
		model.EventUserRegister, model.EventUserExpire, model.EventUserTrafficLow, model.EventOrderPaid,
		model.EventTicketReplied, model.EventNodeOffline, model.EventSystemMaintenance,
	}
	types := []string{"email", "telegram", "webhook"}
	titles := []string{"欢迎加入 {{site_name}}", "Your plan expires on {{expired_at}}", "流量不足 {{remaining}}", "订单 {{trade_no}} 已支付",
		"工单回复 Ticket {{ticket_id}}", "Node {{node_name}} offline", "メンテナンスのお知らせ"}
	for i := 0; i < 15; i++ {
		created := s.At(Days(-90+i) + time.Duration(i)*time.Minute)
		template := model.NotificationTemplate{
			Name: fmt.Sprintf("%s-%s-%02d", types[i%3], events[i%len(events)], i+1), Type: types[i%3], Event: events[i%len(events)],
			Title: titles[i%len(titles)], Content: fmt.Sprintf("<p>模板 %d: {{user_email}} https://staging.example.com/</p>", i+1),
			Enabled: true, CreatedAt: created, UpdatedAt: created.Add(time.Hour),
		}
		s.Create(&template)
		if i%4 == 3 {
			// enabled defaults to true in the table: GORM leaves a false
			// value out of the insert.
			b2Fix(s, &model.NotificationTemplate{}, template.ID, map[string]any{"enabled": false})
			s.World.Add("notification.template.disabled", template.ID)
		}
		s.World.Add("notification.template", template.ID)
	}

	n := 0
	statuses := []int{1, 1, 1, 2, 0}
	log := func(userID *uint, i int, read bool, key string) {
		n++
		created := s.At(Days(-45) + time.Duration(n)*17*time.Minute)
		entry := model.NotificationLog{
			UserID: userID, Type: types[(i+n)%3], Event: events[(i*3+n)%len(events)],
			Title: fmt.Sprintf("通知 Notification %d", n), Content: fmt.Sprintf("synthetic notification %d, see https://staging.example.com/n/%d", n, n),
			Status: statuses[n%len(statuses)], CreatedAt: created,
		}
		switch entry.Status {
		case 1:
			entry.SentAt = ptr(created.Add(2 * time.Second))
		case 2:
			entry.Error = "dial tcp 192.0.2.25:465: connect: network is unreachable"
		}
		if read {
			entry.ReadAt = ptr(created.Add(time.Duration(1+i%5) * time.Hour))
		}
		s.Create(&entry)
		if key != "" {
			s.World.Add(key, entry.ID)
		}
	}
	userID, user2ID, adminID := s.World.PersonaID(User), s.World.PersonaID(User2), s.World.PersonaID(Admin)
	for i := 0; i < 24; i++ {
		read := i%3 == 0
		key := "notification.log.user.unread"
		if read {
			key = "notification.log.user.read"
		}
		log(&userID, i, read, key)
	}
	for i := 0; i < 10; i++ {
		log(&user2ID, i, i%2 == 0, "notification.log.user2")
	}
	members := s.World.IDs["user.member"]
	for j := 2; j < 22 && j < len(members); j++ {
		member := members[j]
		for i := 0; i < 3; i++ {
			log(&member, i, i == 0, "notification.log.other")
		}
	}
	if banned := s.World.IDs["user.banned"]; len(banned) > 0 {
		for i := 0; i < 2; i++ {
			log(&banned[0], i, false, "")
		}
	}
	for i := 0; i < 3; i++ {
		log(&adminID, i, i == 1, "notification.log.admin")
	}
	for i := 0; i < 8; i++ {
		log(nil, i, false, "notification.log.system")
	}
}

// seedB2Telegram writes the bot (its webhook set, one switch off) and the
// Telegram bindings: the user persona's, other members' (some banned on
// Telegram, some taking no notifications), a banned member's and an
// expired member's. user2 and fresh are not bound.
func seedB2Telegram(s *Seeder) {
	created := s.At(Days(-160))
	bot := model.TelegramBot{
		Name: "AnixOps Staging Bot", Token: "1234567890:AAStagingSyntheticTokenNotReal000", Enabled: true,
		WebhookURL: "https://control.staging.example.com/api/v2/telegram/webhook", WebhookSet: true,
		AdminIDs: "[700000001,700000002]", WelcomeMsg: "欢迎使用 welcome to the staging bot",
		AllowBind: true, AllowSub: true, AllowTicket: true, AllowInfo: true, TotalUsers: 19, TotalChats: 240,
		CreatedAt: created, UpdatedAt: created.Add(Days(30)),
	}
	s.Create(&bot)
	b2Fix(s, &model.TelegramBot{}, bot.ID, map[string]any{"allow_ticket": false})
	s.World.Add("telegram.bot", bot.ID)

	owners := []uint{s.World.PersonaID(User)}
	members := s.World.IDs["user.member"]
	for j := 2; j < 18 && j < len(members); j++ {
		owners = append(owners, members[j])
	}
	if banned := s.World.IDs["user.banned"]; len(banned) > 0 {
		owners = append(owners, banned[0])
	}
	if expired := s.World.IDs["user.expired"]; len(expired) > 0 {
		owners = append(owners, expired[0])
	}
	languages := []string{"zh-hans", "en", "ja", ""}
	for j, owner := range owners {
		at := s.At(Days(-100+j) + time.Duration(j)*time.Minute)
		binding := model.TelegramUser{
			UserID: owner, TelegramID: int64(880000000 + j*13), Username: fmt.Sprintf("tg_staging_%02d", j),
			FirstName: []string{"小明", "Alice", "太郎", "Bob"}[j%4], LastName: []string{"王", "Example", "山田", ""}[j%4],
			LanguageCode: languages[j%len(languages)], NotifyExpire: true, NotifyTraffic: true, NotifyTicket: true,
			MessageCount: int64(3 + j*7), LastActive: at.Add(Days(20)), CreatedAt: at, UpdatedAt: at.Add(Days(1)),
		}
		if j%6 == 5 {
			binding.IsBanned, binding.BannedAt = true, ptr(at.Add(Days(10)))
		}
		s.Create(&binding)
		switch j % 4 {
		case 1:
			b2Fix(s, &model.TelegramUser{}, binding.ID, map[string]any{"notify_expire": false, "notify_traffic": false, "notify_ticket": false})
			s.World.Add("telegram.user.silent", binding.ID)
		case 2:
			b2Fix(s, &model.TelegramUser{}, binding.ID, map[string]any{"notify_traffic": false})
		}
		if j == 0 {
			s.World.Add("telegram.user.persona", binding.ID)
		}
		s.World.Add("telegram.user", binding.ID)
	}
}

// seedB2EmailConfig stores the SMTP configuration where both the legacy
// handler (SystemConfigService) and KernelSettings (namespace mail) read
// it: the v2_system_config key notification.email.config, plain JSON with
// the password. The host is an unroutable documentation address.
func seedB2EmailConfig(s *Seeder) {
	value, err := json.Marshal(model.EmailConfig{ // #nosec G101 G117 -- synthetic SMTP settings, not credentials
		Host: "192.0.2.25", Port: 465, Username: "mailer@staging.example.com", Password: "synthetic-smtp-password",
		FromName: "AnixOps Staging", FromAddress: "noreply@staging.example.com", Encryption: "ssl",
	})
	if err != nil {
		panic(err)
	}
	const key = "notification.email.config"
	if err := s.DB.Where("key = ?", key).Delete(&model.SystemConfig{}).Error; err != nil {
		panic(err)
	}
	s.Create(&model.SystemConfig{Key: key, Value: string(value), Type: "json", Group: "notification", Remark: "Email notification config",
		CreatedAt: s.At(Days(-150)), UpdatedAt: s.At(Days(-20))})
}

// seedB2Platform writes the system audit trail (v2_operation_log, module
// system, read through kapi_system_audit_log_v1) with entries of every
// target type and action, some whose content holds secrets the answer
// redacts, and entries of other modules the list must leave out; the
// backup configuration (replacing the default row Control may have
// created) and backup records in every status.
func seedB2Platform(s *Seeder) {
	admins := []uint{s.World.PersonaID(Admin), s.World.PersonaID(Admin2), s.World.PersonaID(Staff)}
	names := []string{"admin", "ops@staging.example.com", "staff@staging.example.com"}
	targets := []string{"system_config", "backup_config", "backup_record"}
	actions := []string{"create", "update", "update", "delete", "restore"}
	contents := []func(i int) string{
		func(i int) string {
			return fmt.Sprintf(`{"key":"site.name.%d","group":"site","type":"string","sensitive":false,"has_value":true,"preserve_existing":false}`, i)
		},
		func(i int) string {
			return fmt.Sprintf(`{"enabled":true,"schedule":"interval:%d","retention_days":%d,"s3_access_key":"AKIASTAGING%04d","storage_type":"s3"}`, 6+i%18, 3+i%20, i)
		},
		func(i int) string {
			return fmt.Sprintf(`{"name":"backup-%04d.sql.gz","path":"/var/lib/anix/backups/backup-%04d.sql.gz","size":%d}`, i, i, 1<<20+i*4096)
		},
		func(i int) string {
			return fmt.Sprintf("password=hunter%d, token: tok-%d; restored archive %d", i, i, i)
		},
		func(i int) string {
			return fmt.Sprintf(`{"nested":{"secret":"s3cr3t-%d","api_key":"k"},"note":"示例 %d"}`, i, i)
		},
		func(i int) string { return "" },
	}
	for i := 0; i < 130; i++ {
		who := i % len(admins)
		target := targets[i%len(targets)]
		entry := model.OperationLog{
			UserID: ptr(admins[who]), Username: names[who], Action: actions[(i/3)%len(actions)], Module: "system", TargetType: target,
			Content: contents[i%len(contents)](i), IP: fmt.Sprintf("203.0.113.%d", 10+i%40), UserAgent: "Mozilla/5.0 (staging synthetic)",
			Status: 1 + i%7/6, CreatedAt: s.At(Days(-80) + time.Duration(i)*53*time.Minute),
		}
		if i%5 != 0 {
			entry.TargetID = ptr(uint(1 + i%9))
		}
		if i%17 == 0 {
			entry.UserID, entry.Username = nil, ""
		}
		s.Create(&entry)
		s.World.Add("audit.system", entry.ID)
	}
	otherModules := []string{"user", "node", "order", "kernel"}
	for i := 0; i < 30; i++ {
		s.Create(&model.OperationLog{
			UserID: ptr(admins[i%len(admins)]), Username: names[i%len(names)], Action: "update", Module: otherModules[i%len(otherModules)],
			TargetType: otherModules[i%len(otherModules)], TargetID: ptr(uint(i + 1)), Content: fmt.Sprintf("non-system entry %d", i),
			IP: fmt.Sprintf("198.51.100.%d", 1+i), UserAgent: "curl/8.0", Status: 1, CreatedAt: s.At(Days(-40) + time.Duration(i)*time.Hour),
		})
	}

	if err := s.DB.Where("1 = 1").Delete(&model.BackupConfig{}).Error; err != nil {
		panic(err)
	}
	config := model.BackupConfig{ // #nosec G101 -- synthetic placeholders, not credentials
		Enabled: true, AutoBackup: true, Schedule: "interval:12", RetentionDays: 14, BackupDatabase: true, BackupFiles: true,
		StorageType: "s3", StoragePath: "backups/staging", S3Bucket: "anix-staging-backups", S3Region: "eu-west-1",
		S3Endpoint: "https://s3.staging.example.net", S3AccessKey: "AKIASTAGINGSYNTHETIC", S3SecretKey: "synthetic-s3-secret-key",
		CreatedAt: s.At(Days(-170)), UpdatedAt: s.At(Days(-10)),
	}
	s.Create(&config)
	s.World.Add("backup.config", config.ID)

	types := []string{"database", "full", "files"}
	for i := 0; i < 27; i++ {
		created := s.At(Days(-54+2*i) + time.Duration(i)*time.Minute)
		name := fmt.Sprintf("backup-%s-%s.tar.gz", types[i%3], created.Format("20060102-150405"))
		record := model.BackupRecord{
			Name: name, Type: types[i%3], Size: int64(50<<20 + i*1_048_573), Path: "backups/staging/" + name,
			Status: []int{1, 1, 1, 2, 0}[i%5], Auto: i%2 == 0, CreatedAt: created, UpdatedAt: created.Add(3 * time.Minute),
		}
		if !record.Auto {
			record.CreatedBy = ptr(admins[i%len(admins)])
		}
		switch record.Status {
		case 1:
			record.CompletedAt = ptr(created.Add(3 * time.Minute))
		case 2:
			record.Error, record.Size = "pg_dump: connection to 192.0.2.40 refused", 0
		}
		if i%9 == 4 {
			record.Path = ""
		}
		s.Create(&record)
		s.World.Add("backup.record", record.ID)
	}
}

// seedB2AgentTasks writes diagnostic tasks of several nodes in every
// status. The node ids need not exist (batch 4 seeds the nodes); the task
// routes do not join them.
func seedB2AgentTasks(s *Seeder) {
	actions := []string{"ping", "traceroute", "mtr", "speedtest", "service_status", "logs"}
	statuses := []string{model.AgentDiagnosticTaskStatusCompleted, model.AgentDiagnosticTaskStatusCompleted,
		model.AgentDiagnosticTaskStatusFailed, model.AgentDiagnosticTaskStatusDispatched, model.AgentDiagnosticTaskStatusPending}
	for i := 0; i < 70; i++ {
		created := s.At(Days(-30) + time.Duration(i)*91*time.Minute)
		task := model.AgentDiagnosticTask{
			TaskID: fmt.Sprintf("diag-staging-%04d", i+1), NodeID: uint(1 + i%6), Action: actions[i%len(actions)],
			Params: fmt.Sprintf(`{"target":"203.0.113.%d","count":%d}`, 1+i%50, 3+i%4), Status: statuses[i%len(statuses)],
			CreatedAt: created, UpdatedAt: created.Add(time.Duration(5+i%40) * time.Second),
		}
		switch task.Status {
		case model.AgentDiagnosticTaskStatusCompleted:
			task.Success, task.DurationMS = true, int64(800+i*37)
			task.Output = fmt.Sprintf("PING 203.0.113.%d: 3 packets transmitted, 3 received, 0%% loss\nrtt %d.%d ms", 1+i%50, 20+i%30, i%10)
		case model.AgentDiagnosticTaskStatusFailed:
			task.Error, task.DurationMS = "timeout after 30s", 30000
		}
		if i%11 == 0 {
			task.Params = ""
		}
		s.Create(&task)
		s.World.AddString("agent.task", task.TaskID)
		s.World.Add("agent.task.node", task.NodeID)
	}
}

// seedB2Traffic writes the node traffic log (v2_server_log) the hourly
// series, the user ranking and the dashboard read.
//
// Unlike every other step it is dated relative to time.Now(), not
// World.Base: the handlers window the log by the clock ("the last 24
// hours"), so rows dated from World.Base would leave every answer empty.
// Two seeds therefore differ in log_at; both twins and both shadow sides
// read the same rows, so the comparison is unaffected. The rows cover the
// 30 days before the seed; if the rehearsal runs days later, the short
// windows thin out (hours=720 still sees them).
//
// Every member gets one report every two hours, at the same hours for all
// members, of a fixed number of bytes times rate that differs per member,
// so in any window the members' totals are distinct and the ranking
// (ORDER BY traffic DESC, no tie-break) is deterministic.
func seedB2Traffic(s *Seeder) {
	members := s.World.IDs["user.member"]
	if len(members) > 30 {
		members = members[:30]
	}
	now := time.Now().UTC().Truncate(time.Hour)
	const mib = int64(1 << 20)
	types := []string{"vless", "vmess", "trojan", "shadowsocks", "hysteria2"}
	var batch []model.TrafficLog
	flush := func() {
		if len(batch) > 0 {
			s.Create(&batch)
			batch = nil
		}
	}
	for j, user := range members {
		effective := (512 + 37*int64(j)) * mib // bytes times rate, distinct per member
		rate, bytes := 1.0, effective
		if j%5 == 4 {
			rate, bytes = 2, effective/2
		}
		offset := time.Duration(60+(j*97)%3000) * time.Second
		for k := 1; k <= 30*12; k++ {
			at := now.Add(-time.Duration(2*k)*time.Hour + offset)
			upload := bytes / 4
			batch = append(batch, model.TrafficLog{
				UserID: user, ServerID: uint(1 + (j+k)%4), ServerType: types[(j+k)%len(types)],
				U: upload, D: bytes - upload, Rate: rate, LogAt: at.Unix(), CreatedAt: at,
			})
			if len(batch) == 1000 {
				flush()
			}
		}
		s.World.Add("traffic.user", user)
	}
	flush()
}

// b2Fix sets columns a GORM insert left at their table default (a false
// boolean whose column defaults to true).
func b2Fix(s *Seeder, value any, id uint, columns map[string]any) {
	if err := s.DB.Model(value).Where("id = ?", id).UpdateColumns(columns).Error; err != nil {
		panic(fmt.Errorf("seed: fix %T %d: %w", value, id, err))
	}
}

// ---------------------------------------------------------------- notification

func b2NotificationSpecs() []RouteSpec {
	return []RouteSpec{
		RouteSpec{RouteID: "notification.user.notifications.get", Reads: func(w *World) []Req {
			path := "/api/v2/user/notifications"
			return []Req{
				{Persona: User, Path: path, Label: "own notifications newest first"},
				{Persona: User, Path: path, Query: q("page", "2", "page_size", "5"), Label: "page 2"},
				{Persona: User, Path: path, Query: q("page", "3", "page_size", "10"), Label: "last partial page"},
				{Persona: User, Path: path, Query: q("page", "9"), Label: "page past the end"},
				{Persona: User, Path: path, Query: q("page_size", "0"), Label: "page size zero"},
				{Persona: User, Path: path, Query: q("page", "x"), Label: "page that is not a number"},
				{Persona: User2, Path: path, Label: "other member"},
				{Persona: Fresh, Path: path, Label: "no notifications"},
				{Persona: Expired, Path: path, Label: "expired member"},
				{Persona: Admin, Path: path, Label: "administrator's own"},
				{Persona: Anon, Path: path, Label: "anonymous"},
			}
		}},
		RouteSpec{RouteID: "notification.user.notifications.unread_count.get", Reads: func(w *World) []Req {
			path := "/api/v2/user/notifications/unread-count"
			return []Req{
				{Persona: User, Path: path, Label: "unread count"},
				{Persona: User2, Path: path, Label: "other member"},
				{Persona: Fresh, Path: path, Label: "nothing unread"},
				{Persona: Admin, Path: path, Label: "administrator"},
				{Persona: Anon, Path: path, Label: "anonymous"},
			}
		}},
		RouteSpec{RouteID: "notification.user.notifications.id.read.post", Writes: func(w *World) []Req {
			path := "/api/v2/user/notifications/:id/read"
			return []Req{
				{Persona: User, Path: fill(path, w.ID("notification.log.user.unread", 0)), Label: "mark own unread"},
				{Persona: User, Path: fill(path, w.ID("notification.log.user.unread", 1)), Label: "mark another own unread"},
				{Persona: User, Path: fill(path, w.ID("notification.log.user.read", 0)), Label: "mark one already read"},
				{Persona: User, Path: fill(path, w.ID("notification.log.user2", 1)), Label: "another member's notification"},
				{Persona: User, Path: fill(path, w.ID("notification.log.system", 0)), Label: "system notification without a user"},
				{Persona: User, Path: fill(path, Missing), Label: "not found"},
				{Persona: User, Path: "/api/v2/user/notifications/x/read", Label: "id that is not a number"},
				{Persona: Anon, Path: fill(path, w.ID("notification.log.user.unread", 2)), Label: "anonymous"},
			}
		}},
		RouteSpec{RouteID: "notification.user.notifications.read_all.post", Writes: func(w *World) []Req {
			path := "/api/v2/user/notifications/read-all"
			return []Req{
				{Persona: User2, Path: path, Label: "mark all"},
				{Persona: User2, Path: path, Label: "mark all again"},
				{Persona: Fresh, Path: path, Label: "nothing to mark"},
				{Persona: Anon, Path: path, Label: "anonymous"},
			}
		}},

		RouteSpec{RouteID: "notification.admin.notification.templates.get", Reads: func(w *World) []Req {
			path := "/api/v2/admin/notification/templates"
			return append([]Req{
				{Persona: Admin, Path: path, Label: "every template"},
				{Persona: Admin, Path: path, Query: q("type", "email"), Label: "by type email"},
				{Persona: Admin2, Path: path, Query: q("type", "telegram"), Label: "by type telegram"},
				{Persona: Staff, Path: path, Query: q("type", "webhook"), Label: "by type webhook"},
				{Persona: Admin, Path: path, Query: q("type", "sms"), Label: "no template of the type"},
				{Persona: Admin, Path: path, Query: q("page", "2", "enabled", "false"), Label: "ignored filters"},
			}, forbidden(path)...)
		}},
		RouteSpec{RouteID: "notification.admin.notification.templates.post", Writes: func(w *World) []Req {
			path := "/api/v2/admin/notification/templates"
			created := []string{"data.created_at", "data.updated_at"}
			return []Req{
				{Persona: Admin, Path: path, Body: map[string]any{"name": "staging-new", "type": "email", "event": "order.paid", "title": "订单已支付", "content": "<p>{{trade_no}}</p>", "enabled": true}, Mask: created, Label: "create"},
				{Persona: Admin, Path: path, Body: map[string]any{"name": "disabled", "type": "telegram", "event": "user.expire", "title": "t", "content": "c", "enabled": false}, Mask: created, Label: "create disabled keeps the column default"},
				{Persona: Staff, Path: path, Body: map[string]any{"name": "sms", "type": "sms", "event": "e", "title": "t", "content": "c"}, Mask: created, Label: "unknown type is not validated"},
				{Persona: Admin, Path: path, Body: map[string]any{"name": "n", "content": "c"}, Label: "missing fields"},
				{Persona: Admin, Path: path, Body: `{"name":1}`, Label: "wrong field type"},
				{Persona: Admin, Path: path, Body: `{"name":`, Label: "invalid JSON"},
				{Persona: Admin, Path: path, Label: "no body"},
				{Persona: User, Path: path, Body: map[string]any{"name": "n", "type": "email", "event": "e", "title": "t", "content": "c"}, Label: "member on admin route"},
			}
		}},
		RouteSpec{RouteID: "notification.admin.notification.templates.id.put", Writes: func(w *World) []Req {
			path := "/api/v2/admin/notification/templates/:id"
			updated := []string{"data.updated_at"}
			return []Req{
				{Persona: Admin, Path: fill(path, w.ID("notification.template", 0)), Body: map[string]any{"type": "webhook", "event": "ev", "name": "renamed 改名", "title": "T", "content": "C", "enabled": false}, Mask: updated, Label: "update every field"},
				{Persona: Admin, Path: fill(path, w.ID("notification.template.disabled", 0)), Body: map[string]any{"enabled": true}, Mask: updated, Label: "enable only"},
				{Persona: Admin, Path: fill(path, w.ID("notification.template", 1)), Body: map[string]any{}, Mask: updated, Label: "empty body keeps everything"},
				{Persona: Admin, Path: fill(path, w.ID("notification.template", 2)), Body: map[string]any{"type": "sms"}, Label: "invalid type"},
				{Persona: Admin, Path: fill(path, w.ID("notification.template", 2)), Body: map[string]any{"name": strings.Repeat("n", 256)}, Label: "name too long"},
				{Persona: Admin, Path: fill(path, w.ID("notification.template", 2)), Body: `{"enabled":"yes"}`, Label: "wrong field type"},
				{Persona: Admin, Path: fill(path, Missing), Body: map[string]any{"name": "x"}, Label: "not found"},
				{Persona: Admin, Path: fill(path, Missing), Body: map[string]any{"type": "sms"}, Label: "not found with a bad body"},
				{Persona: Admin, Path: "/api/v2/admin/notification/templates/x", Body: map[string]any{"name": "x"}, Label: "id that is not a number"},
				{Persona: Admin, Path: "/api/v2/admin/notification/templates/0%20OR%201=1", Body: map[string]any{"name": "x"}, Label: "id that is SQL"},
				{Persona: User, Path: fill(path, w.ID("notification.template", 3)), Body: map[string]any{"name": "x"}, Label: "member on admin route"},
			}
		}},
		RouteSpec{RouteID: "notification.admin.notification.templates.id.delete", Writes: func(w *World) []Req {
			path := "/api/v2/admin/notification/templates/:id"
			return []Req{
				{Persona: Admin, Path: fill(path, w.ID("notification.template", 10)), Label: "delete"},
				{Persona: Admin, Path: fill(path, w.ID("notification.template", 10)), Label: "delete again"},
				{Persona: Staff, Path: fill(path, w.ID("notification.template", 11)), Label: "staff delete"},
				{Persona: Admin, Path: fill(path, Missing), Label: "not found"},
				{Persona: Admin, Path: "/api/v2/admin/notification/templates/x", Label: "id that is not a number"},
				{Persona: Admin, Path: "/api/v2/admin/notification/templates/0%20OR%201=1", Label: "id that is SQL"},
				{Persona: User, Path: fill(path, w.ID("notification.template", 12)), Label: "member on admin route"},
			}
		}},
		RouteSpec{RouteID: "notification.admin.notification.logs.get", Reads: func(w *World) []Req {
			path := "/api/v2/admin/notification/logs"
			return append([]Req{
				{Persona: Admin, Path: path, Label: "every log newest first"},
				{Persona: Admin, Path: path, Query: q("type", "email"), Label: "by type"},
				{Persona: Admin2, Path: path, Query: q("status", "failed"), Label: "by status name"},
				{Persona: Admin, Path: path, Query: q("status", "1"), Label: "by status code"},
				{Persona: Admin, Path: path, Query: q("status", "PENDING", "type", "telegram"), Label: "status name and type"},
				{Persona: Admin, Path: path, Query: q("status", "bogus"), Label: "unknown status is ignored"},
				{Persona: Staff, Path: path, Query: q("page", "2", "page_size", "10"), Label: "page 2"},
				{Persona: Admin, Path: path, Query: q("page", "0", "page_size", "500"), Label: "clamped page"},
				{Persona: Admin, Path: path, Query: q("page", "99"), Label: "page past the end"},
				{Persona: Admin, Path: path, Query: q("type", "sms"), Label: "no logs of the type"},
			}, forbidden(path)...)
		}},
		RouteSpec{RouteID: "notification.admin.notification.test.post", Writes: func(w *World) []Req {
			path := "/api/v2/admin/notification/test"
			return []Req{
				// Dials the seeded SMTP host 192.0.2.25:465, which the
				// staging network cannot reach: both twins fail alike.
				{Persona: Admin, Path: path, Body: map[string]any{"type": "email", "to": "member001@example.com", "title": "测试 test"}, Label: "e-mail to an unreachable SMTP host"},
				{Persona: Admin, Path: path, Body: map[string]any{"type": "email"}, Label: "e-mail without a recipient"},
				{Persona: Admin, Path: path, Body: map[string]any{"type": "telegram", "recipient": "700000001", "content": "hi"}, Label: "telegram stub"},
				{Persona: Staff, Path: path, Body: map[string]any{"type": "webhook", "subject": "s"}, Label: "webhook stub"},
				{Persona: Admin, Path: path, Body: map[string]any{"type": "sms", "to": "x"}, Label: "invalid type"},
				{Persona: Admin, Path: path, Body: map[string]any{"to": "member001@example.com"}, Label: "missing type"},
				{Persona: Admin, Path: path, Body: `{"type":`, Label: "invalid JSON"},
				{Persona: User, Path: path, Body: map[string]any{"type": "telegram"}, Label: "member on admin route"},
			}
		}},
		RouteSpec{RouteID: "notification.admin.notification.email.config.get", Reads: func(w *World) []Req {
			path := "/api/v2/admin/notification/email/config"
			return append([]Req{
				{Persona: Admin, Path: path, Label: "stored configuration, password masked"},
				{Persona: Admin2, Path: path, Label: "second administrator"},
				{Persona: Staff, Path: path, Label: "staff"},
			}, forbidden(path)...)
		}},
		RouteSpec{RouteID: "notification.admin.notification.email.config.put", Writes: func(w *World) []Req {
			path := "/api/v2/admin/notification/email/config"
			return []Req{
				{Persona: Admin, Path: path, Body: map[string]any{"host": "192.0.2.26", "port": 587, "username": "mailer2@staging.example.com", "password": "********", "from_address": "noreply@staging.example.com", "from_name": "Staging 通知", "encryption_type": "starttls"}, Label: "update keeping the password"},
				{Persona: Admin, Path: path, Body: map[string]any{"host": " 192.0.2.27 ", "port": "2525", "password": "rotated-synthetic-password", "from_address": "alerts@staging.example.org", "encryption": false}, Label: "rotate the password, string port, encryption off"},
				{Persona: Admin2, Path: path, Body: map[string]any{"host": "192.0.2.28", "port": 465.0, "password": "", "from_address": "noreply@staging.example.net", "encryption": true, "encryption_type": "ssl"}, Label: "blank password keeps it, encryption_type wins"},
				{Persona: Admin, Path: path, Body: map[string]any{"port": 25, "from_address": "a@example.com"}, Label: "missing host"},
				{Persona: Admin, Path: path, Body: map[string]any{"host": "192.0.2.29", "port": 0, "from_address": "a@example.com"}, Label: "port zero"},
				{Persona: Admin, Path: path, Body: map[string]any{"host": "192.0.2.29", "port": 25}, Label: "missing from_address"},
				{Persona: Admin, Path: path, Body: map[string]any{"host": "192.0.2.29", "port": 25, "from_address": "a@example.com", "encryption_type": "rot13"}, Label: "invalid encryption_type"},
				{Persona: Admin, Path: path, Body: map[string]any{"host": "192.0.2.29", "port": 25, "from_address": "a@example.com", "encryption": 7}, Label: "invalid encryption"},
				{Persona: Admin, Path: path, Body: `[1,2]`, Label: "body that is not an object"},
				{Persona: User, Path: path, Body: map[string]any{"host": "192.0.2.30", "port": 25, "from_address": "a@example.com"}, Label: "member on admin route"},
			}
		}},

		RouteSpec{RouteID: "notification.admin.telegram.bot.get", Reads: func(w *World) []Req {
			path := "/api/v2/admin/telegram/bot"
			return append([]Req{
				{Persona: Admin, Path: path, Label: "bot"},
				{Persona: Staff, Path: path, Label: "staff"},
			}, forbidden(path)...)
		}},
		RouteSpec{RouteID: "notification.admin.telegram.bot.put", Writes: func(w *World) []Req {
			path := "/api/v2/admin/telegram/bot"
			updated := []string{"data.created_at", "data.updated_at"}
			return []Req{
				{Persona: Admin, Path: path, Body: map[string]any{"name": "Renamed 机器人", "welcome_msg": "w", "admin_ids": []any{1, "2"}, "allow_sub": true, "allow_ticket": true, "allow_info": false}, Mask: updated, Label: "update"},
				{Persona: Admin, Path: path, Body: map[string]any{"token": "1234567890:AAStagingRotatedTokenNotReal111", "welcome_message": "hi", "admin_ids": "5, 6,x", "allow_bind": false}, Mask: updated, Label: "rotate the token, comma separated admin ids"},
				{Persona: Admin, Path: path, Body: map[string]any{}, Mask: updated, Label: "empty body"},
				{Persona: Admin, Path: path, Body: map[string]any{"admin_ids": []any{"x"}}, Label: "invalid admin id"},
				{Persona: Admin, Path: path, Body: map[string]any{"admin_ids": []any{true}}, Label: "invalid admin id element"},
				{Persona: Admin, Path: path, Body: map[string]any{"admin_ids": map[string]any{"a": 1}}, Label: "invalid admin ids"},
				{Persona: Admin, Path: path, Body: map[string]any{"name": strings.Repeat("n", 256)}, Label: "name too long"},
				{Persona: Admin, Path: path, Body: map[string]any{"allow_bind": "yes"}, Label: "wrong field type"},
				{Persona: User, Path: path, Body: map[string]any{"name": "x"}, Label: "member on admin route"},
			}
		}},
		RouteSpec{RouteID: "notification.admin.telegram.webhook.post", Writes: func(w *World) []Req {
			path := "/api/v2/admin/telegram/webhook"
			// setWebhook calls api.telegram.org, which the staging network
			// cannot reach: both twins answer the same transport error.
			return []Req{
				{Persona: Admin, Path: path, Body: map[string]any{"url": "https://control.staging.example.com/api/v2/telegram/webhook"}, Label: "explicit url (Bot API unreachable)"},
				{Persona: Admin, Path: path, Label: "default url from the request (Bot API unreachable)"},
				{Persona: Admin, Path: path, Body: map[string]any{"url": "not a url"}, Label: "invalid url"},
				{Persona: Admin, Path: path, Body: map[string]any{"url": "https://example.com/" + strings.Repeat("a", 520)}, Label: "url too long"},
				{Persona: Admin, Path: path, Body: `{"url":`, Label: "invalid JSON"},
				{Persona: User, Path: path, Body: map[string]any{"url": "https://example.com/hook"}, Label: "member on admin route"},
			}
		}},
		RouteSpec{RouteID: "notification.admin.telegram.webhook.delete", Writes: func(w *World) []Req {
			path := "/api/v2/admin/telegram/webhook"
			return []Req{
				{Persona: Admin, Path: path, Label: "delete (Bot API unreachable)"},
				{Persona: User, Path: path, Label: "member on admin route"},
				{Persona: Anon, Path: path, Label: "anonymous"},
			}
		}},
		RouteSpec{RouteID: "notification.admin.telegram.notify.post", Writes: func(w *World) []Req {
			path := "/api/v2/admin/telegram/notify"
			return []Req{
				{Persona: Admin, Path: path, Body: map[string]any{"telegram_id": 880000000, "message": "hi"}, Label: "message (Bot API unreachable)"},
				{Persona: Admin, Path: path, Body: map[string]any{"telegram_id": "880000013", "title": "T", "content": "C"}, Label: "title and content (Bot API unreachable)"},
				{Persona: Admin, Path: path, Body: map[string]any{"telegram_id": 880000026, "message": map[string]any{"message": " hi "}}, Label: "nested message (Bot API unreachable)"},
				{Persona: Admin, Path: path, Body: map[string]any{"message": "hi"}, Label: "missing telegram id"},
				{Persona: Admin, Path: path, Body: map[string]any{"telegram_id": "abc", "message": "hi"}, Label: "telegram id that is not a number"},
				{Persona: Admin, Path: path, Body: map[string]any{"telegram_id": 0, "message": "hi"}, Label: "telegram id zero"},
				{Persona: Admin, Path: path, Body: map[string]any{"telegram_id": 880000000, "title": " ", "content": ""}, Label: "no message"},
				{Persona: Admin, Path: path, Body: `{"telegram_id":`, Label: "invalid JSON"},
				{Persona: User, Path: path, Body: map[string]any{"telegram_id": 1, "message": "x"}, Label: "member on admin route"},
			}
		}},
		RouteSpec{RouteID: "notification.admin.telegram.broadcast.post", Writes: func(w *World) []Req {
			path := "/api/v2/admin/telegram/broadcast"
			// One send per subscribed binding (about a dozen), each failing
			// at once without a route out.
			return []Req{
				{Persona: Admin, Path: path, Body: map[string]any{"message": "维护通知 maintenance tonight"}, Label: "broadcast (Bot API unreachable)"},
				{Persona: Admin, Path: path, Body: map[string]any{"message": map[string]any{"message": "  "}}, Label: "blank nested message"},
				{Persona: Admin, Path: path, Body: map[string]any{}, Label: "missing message"},
				{Persona: Admin, Path: path, Body: `[]`, Label: "body that is not an object"},
				{Persona: User, Path: path, Body: map[string]any{"message": "x"}, Label: "member on admin route"},
			}
		}},
		RouteSpec{RouteID: "notification.admin.telegram.users.get", Reads: func(w *World) []Req {
			path := "/api/v2/admin/telegram/users"
			return append([]Req{
				{Persona: Admin, Path: path, Label: "bindings newest first"},
				{Persona: Admin, Path: path, Query: q("page", "2", "page_size", "5"), Label: "page 2"},
				{Persona: Admin2, Path: path, Query: q("all", "true"), Label: "every binding"},
				{Persona: Admin, Path: path, Query: q("all", "TRUE", "page", "3"), Label: "every binding, case-insensitive"},
				{Persona: Admin, Path: path, Query: q("all", "1"), Label: "all=1 is paged"},
				{Persona: Staff, Path: path, Query: q("page", "0", "page_size", "500"), Label: "clamped page"},
				{Persona: Admin, Path: path, Query: q("page", "x", "page_size", "y"), Label: "page that is not a number"},
				{Persona: Admin, Path: path, Query: q("page", "50"), Label: "page past the end"},
			}, forbidden(path)...)
		}},
		RouteSpec{RouteID: "notification.admin.telegram.users.id.notify.put", Writes: func(w *World) []Req {
			path := "/api/v2/admin/telegram/users/:id/notify"
			updated := []string{"data.updated_at"}
			return []Req{
				{Persona: Admin, Path: fill(path, w.ID("telegram.user", 2)), Body: map[string]any{"notify_enabled": false}, Mask: updated, Label: "disable every switch"},
				{Persona: Admin, Path: fill(path, w.ID("telegram.user.silent", 0)), Body: map[string]any{"notify_enabled": true}, Mask: updated, Label: "enable every switch"},
				{Persona: Admin, Path: fill(path, w.ID("telegram.user", 3)), Body: map[string]any{"notify_enabled": true, "notify_ticket": false}, Mask: updated, Label: "one switch wins over notify_enabled"},
				{Persona: Staff, Path: fill(path, w.ID("telegram.user", 4)), Body: map[string]any{"notify_expire": false, "notify_traffic": true}, Mask: updated, Label: "two switches"},
				{Persona: Admin, Path: fill(path, w.ID("telegram.user", 4)), Body: map[string]any{}, Label: "no notify fields"},
				{Persona: Admin, Path: fill(path, w.ID("telegram.user", 4)), Body: map[string]any{"notify_enabled": "yes"}, Label: "notify_enabled not boolean"},
				{Persona: Admin, Path: fill(path, w.ID("telegram.user", 4)), Body: map[string]any{"notify_traffic": 1}, Label: "notify_traffic not boolean"},
				{Persona: Admin, Path: fill(path, Missing), Body: map[string]any{"notify_enabled": true}, Label: "not found"},
				{Persona: Admin, Path: "/api/v2/admin/telegram/users/x/notify", Body: map[string]any{"notify_enabled": true}, Label: "invalid id"},
				{Persona: Admin, Path: fill(path, w.ID("telegram.user", 5)), Body: `{"notify_enabled":`, Label: "invalid JSON"},
				{Persona: User, Path: fill(path, w.ID("telegram.user", 5)), Body: map[string]any{"notify_enabled": true}, Label: "member on admin route"},
			}
		}},
		RouteSpec{RouteID: "notification.user.telegram.status.get", Reads: func(w *World) []Req {
			path := "/api/v2/user/telegram/status"
			return []Req{
				{Persona: User, Path: path, Label: "bound"},
				{Persona: User2, Path: path, Label: "not bound"},
				{Persona: Fresh, Path: path, Label: "member without data"},
				{Persona: Expired, Path: path, Label: "expired member, bound"},
				{Persona: Anon, Path: path, Label: "anonymous"},
			}
		}},
		RouteSpec{RouteID: "notification.user.telegram.unbind.post", Writes: func(w *World) []Req {
			path := "/api/v2/user/telegram/unbind"
			return []Req{
				{Persona: User, Path: path, Label: "unbind (a stub: nothing is deleted)"},
				{Persona: User2, Path: path, Label: "not bound"},
				{Persona: Anon, Path: path, Label: "anonymous"},
			}
		}},
		RouteSpec{RouteID: "notification.user.telegram.notify.post", Writes: func(w *World) []Req {
			path := "/api/v2/user/telegram/notify"
			return []Req{
				{Persona: User, Path: path, Body: map[string]any{"notify_expire": false, "notify_traffic": true}, Label: "settings (a stub: nothing changes)"},
				{Persona: User, Path: path, Body: map[string]any{}, Label: "empty body"},
				{Persona: User, Path: path, Body: map[string]any{"notify_ticket": "no"}, Label: "wrong field type"},
				{Persona: User, Path: path, Body: `{"notify_expire":`, Label: "invalid JSON"},
				{Persona: Fresh, Path: path, Label: "no body"},
				{Persona: Anon, Path: path, Body: map[string]any{"notify_expire": true}, Label: "anonymous"},
			}
		}},
	}
}

// ---------------------------------------------------------------- platform

func b2PlatformSpecs() []RouteSpec {
	return []RouteSpec{
		RouteSpec{RouteID: "platform.admin.system.audit_logs.get", Reads: func(w *World) []Req {
			path := "/api/v2/admin/system/audit-logs"
			return append([]Req{
				{Persona: Admin, Path: path, Label: "newest system entries, secrets redacted"},
				{Persona: Admin, Path: path, Query: q("page", "2", "page_size", "15"), Label: "page 2"},
				{Persona: Admin2, Path: path, Query: q("page", "7", "page_size", "20"), Label: "last partial page"},
				{Persona: Admin, Path: path, Query: q("target_type", "backup_config"), Label: "by target type"},
				{Persona: Admin, Path: path, Query: q("action", "update"), Label: "by action"},
				{Persona: Staff, Path: path, Query: q("target_type", " system_config ", "action", "create"), Label: "target type and action, trimmed"},
				{Persona: Admin, Path: path, Query: q("target_type", "user"), Label: "other modules' entries are left out"},
				{Persona: Admin, Path: path, Query: q("page_size", "500"), Label: "page size capped at 200"},
				{Persona: Admin, Path: path, Query: q("page", "-1", "page_size", "0"), Label: "page and size below range"},
				{Persona: Admin, Path: path, Query: q("page", "x"), Label: "page that is not a number"},
				{Persona: Admin, Path: path, Query: q("page", "40"), Label: "page past the end"},
			}, forbidden(path)...)
		}},
		RouteSpec{RouteID: "platform.admin.system.backup.config.get", Reads: func(w *World) []Req {
			path := "/api/v2/admin/system/backup/config"
			return append([]Req{
				{Persona: Admin, Path: path, Label: "configuration, S3 keys masked"},
				{Persona: Admin2, Path: path, Label: "second administrator"},
				{Persona: Staff, Path: path, Label: "staff"},
			}, forbidden(path)...)
		}},
		RouteSpec{RouteID: "platform.admin.system.backup.config.put", Writes: func(w *World) []Req {
			path := "/api/v2/admin/system/backup/config"
			stamped := []string{"data.created_at", "data.updated_at"}
			return []Req{
				{Persona: Admin, Path: path, Body: map[string]any{"enabled": true, "auto_backup": false, "schedule": "0 3 * * *", "retention_days": 30, "s3_access_key": "********", "s3_secret_key": "********"}, Mask: stamped, Label: "update keeping the S3 keys"},
				{Persona: Admin, Path: path, Body: map[string]any{"interval": 6, "keep_count": 5, "retention_days": 9, "storage_type": "local", "storage_path": "backups/rehearsal"}, Mask: stamped, Label: "frontend aliases win"},
				{Persona: Admin2, Path: path, Body: map[string]any{"preserve_existing_sensitive": true, "s3_access_key": "", "s3_secret_key": nil, "s3_bucket": "anix-staging-2", "s3_endpoint": "https://s3.staging.example.org"}, Mask: stamped, Label: "preserve blank and null S3 keys"},
				{Persona: Admin, Path: path, Body: map[string]any{"s3_access_key": "AKIAROTATEDSYNTHETIC", "s3_secret_key": nil}, Mask: stamped, Label: "rotate one key, clear the other"}, // #nosec G101 -- synthetic placeholder, not a credential
				{Persona: Admin, Path: path, Body: map[string]any{"enabled": "yes", "retention_days": "10", "interval": 0, "schedule": 5, "backup_files": false}, Mask: stamped, Label: "fields of another type are ignored"},
				{Persona: Admin, Path: path, Body: map[string]any{"retention_days": 7.9, "interval": 2.5}, Mask: stamped, Label: "float numbers are truncated"},
				{Persona: Admin, Path: path, Body: map[string]any{}, Mask: stamped, Label: "empty body"},
				{Persona: Admin, Path: path, Body: `{"enabled":`, Label: "invalid JSON"},
				{Persona: User, Path: path, Body: map[string]any{"enabled": false}, Label: "member on admin route"},
			}
		}},
		RouteSpec{RouteID: "platform.admin.system.backups.get", Reads: func(w *World) []Req {
			path := "/api/v2/admin/system/backups"
			return append([]Req{
				{Persona: Admin, Path: path, Label: "records newest first"},
				{Persona: Admin, Path: path, Query: q("page", "2", "page_size", "10"), Label: "page 2"},
				{Persona: Admin2, Path: path, Query: q("page", "0", "page_size", "500"), Label: "clamped page"},
				{Persona: Admin, Path: path, Query: q("page", "9"), Label: "page past the end"},
				{Persona: Staff, Path: path, Query: q("page_size", "x"), Label: "page size that is not a number"},
			}, forbidden(path)...)
		}},
		RouteSpec{RouteID: "platform.admin.system.backup.stats.get", Reads: func(w *World) []Req {
			path := "/api/v2/admin/system/backup/stats"
			return append([]Req{
				{Persona: Admin, Path: path, Label: "stats of the successful backups"},
				{Persona: Staff, Path: path, Label: "staff"},
			}, forbidden(path)...)
		}},
	}
}

// ---------------------------------------------------------------- machine-telemetry

func b2TelemetrySpecs() []RouteSpec {
	return []RouteSpec{
		// refresh=true is left out: each shadow side would rebuild the
		// kernel's snapshot and answer its own cached_at.
		RouteSpec{RouteID: "telemetry.admin.dashboard.get", Reads: func(w *World) []Req {
			path := "/api/v2/admin/dashboard"
			return append([]Req{
				{Persona: Admin, Path: path, Label: "cached snapshot"},
				{Persona: Admin2, Path: path, Query: q("refresh", "false"), Label: "refresh=false"},
				{Persona: Staff, Path: path, Query: q("refresh", "1"), Label: "refresh=1 is not a refresh"},
				{Persona: Admin, Path: path, Query: q("refresh", "TRUE"), Label: "refresh=TRUE is not a refresh"},
			}, forbidden(path)...)
		}},
		RouteSpec{RouteID: "telemetry.admin.traffic.hourly.get", Reads: func(w *World) []Req {
			path := "/api/v2/admin/traffic/hourly"
			return append([]Req{
				{Persona: Admin, Path: path, Label: "last 24 hours"},
				{Persona: Admin, Path: path, Query: q("hours", "72"), Label: "last 72 hours"},
				{Persona: Admin2, Path: path, Query: q("hours", "720"), Label: "30 days"},
				{Persona: Admin, Path: path, Query: q("hours", "100000"), Label: "window capped at 720"},
				{Persona: Admin, Path: path, Query: q("hours", "-5"), Label: "negative hours"},
				{Persona: Admin, Path: path, Query: q("hours", "abc"), Label: "hours that is not a number"},
				{Persona: Staff, Path: path, Query: q("user_id", fmt.Sprint(w.PersonaID(User)), "hours", "48"), Label: "one user"},
				{Persona: Admin, Path: path, Query: q("user_id", fmt.Sprint(w.PersonaID(Fresh))), Label: "user without traffic"},
				{Persona: Admin, Path: path, Query: q("user_id", strconv.Itoa(Missing)), Label: "unknown user"},
				{Persona: Admin, Path: path, Query: q("user_id", "abc"), Label: "user id that is not a number"},
			}, forbidden(path)...)
		}},
		RouteSpec{RouteID: "telemetry.admin.traffic.user_ranking.get", Reads: func(w *World) []Req {
			path := "/api/v2/admin/traffic/user-ranking"
			return append([]Req{
				{Persona: Admin, Path: path, Label: "top 20 of 24 hours"},
				{Persona: Admin, Path: path, Query: q("limit", "5"), Label: "top 5"},
				{Persona: Admin2, Path: path, Query: q("limit", "500", "hours", "168"), Label: "limit capped at 200, a week"},
				{Persona: Admin, Path: path, Query: q("limit", "0", "hours", "0"), Label: "limit and hours zero"},
				{Persona: Admin, Path: path, Query: q("include_zero_users", "true"), Label: "every user"},
				{Persona: Staff, Path: path, Query: q("include_zero_users", "1", "limit", "2000"), Label: "every user, limit capped at 1000"},
				{Persona: Admin, Path: path, Query: q("include_zero_users", "true", "hours", "1", "limit", "40"), Label: "every user, last hour"},
				{Persona: Admin, Path: path, Query: q("include_zero_users", "yes"), Label: "include_zero_users=yes is ignored"},
			}, forbidden(path)...)
		}},
	}
}

// ---------------------------------------------------------------- protocol-runtime

func b2ProtocolSpecs() []RouteSpec {
	return []RouteSpec{
		RouteSpec{RouteID: "protocol.admin.agent.tasks.get", Reads: func(w *World) []Req {
			path := "/api/v2/admin/agent/tasks"
			return append([]Req{
				{Persona: Admin, Path: path, Label: "newest 50 tasks"},
				{Persona: Admin, Path: path, Query: q("node_id", fmt.Sprint(w.ID("agent.task.node", 0))), Label: "one node"},
				{Persona: Admin2, Path: path, Query: q("node_id", fmt.Sprint(w.ID("agent.task.node", 2)), "limit", "5"), Label: "one node, limit 5"},
				{Persona: Admin, Path: path, Query: q("limit", "200"), Label: "limit 200"},
				{Persona: Admin, Path: path, Query: q("limit", "500"), Label: "limit over 200 falls back to 50"},
				{Persona: Staff, Path: path, Query: q("limit", "-1", "node_id", "abc"), Label: "invalid limit and node"},
				{Persona: Admin, Path: path, Query: q("node_id", strconv.Itoa(Missing)), Label: "node without tasks"},
				{Persona: Admin, Path: path, Query: q("node_id", "-3"), Label: "negative node id"},
			}, forbidden(path)...)
		}},
		RouteSpec{RouteID: "protocol.admin.agent.tasks.task_id.get", Reads: func(w *World) []Req {
			path := "/api/v2/admin/agent/tasks/:task_id"
			var reqs []Req
			for i := 0; i < 5; i++ {
				reqs = append(reqs, Req{Persona: Admin, Path: fill(path, w.Str("agent.task", i*13+i)), Label: "task"})
			}
			return append(reqs,
				Req{Persona: Admin, Path: fill(path, w.Str("agent.task", 0)), Label: "task without params"},
				Req{Persona: Admin, Path: fill(path, "diag-missing-0000"), Label: "not found"},
				Req{Persona: Admin, Path: fill(path, "1"), Label: "numeric task id"},
				Req{Persona: User, Path: fill(path, w.Str("agent.task", 1)), Label: "member on admin route"},
				Req{Persona: Anon, Path: fill(path, w.Str("agent.task", 1)), Label: "anonymous"},
			)
		}},
		RouteSpec{RouteID: "protocol.admin.protocol_templates.get", Reads: func(w *World) []Req {
			path := "/api/v2/admin/protocol-templates"
			return append([]Req{
				{Persona: Admin, Path: path, Label: "templates"},
				{Persona: Staff, Path: path, Label: "staff"},
			}, forbidden(path)...)
		}},
	}
}

// b2ProtocolNodeSpecs are the protocol-runtime routes of M3-1. The node
// protocol routes answer natively only once v2_node_protocol is finalized
// (staging-rehearsal.md, "Limits"); the staging nodes have no agent, so the
// agent routes answer what a node without one gets.
func b2ProtocolNodeSpecs() []RouteSpec {
	node := func(w *World, i int) uint { return w.ID("node", i) }
	nodePath := func(id any, rest string) string { return fmt.Sprintf("/api/v2/admin/nodes/%v%s", id, rest) }
	protocolPath := func(w *World, i int) string {
		return fmt.Sprintf("/api/v2/admin/nodes/%d/protocols/%d", node(w, 0), w.ID("node_protocol", i))
	}
	stamped := []string{"data.created_at", "data.updated_at", "data.id"}
	return []RouteSpec{
		{RouteID: "protocol.admin.nodes.id.protocols.get", Reads: func(w *World) []Req {
			return append([]Req{
				{Persona: Admin, Path: nodePath(node(w, 0), "/protocols"), Label: "a node's protocols, secrets masked"},
				{Persona: Admin, Path: nodePath(node(w, 7), "/protocols"), Label: "another node"},
				{Persona: Admin, Path: nodePath(Missing, "/protocols"), Label: "an unknown node"},
				{Persona: Admin, Path: nodePath("x", "/protocols"), Label: "invalid id"},
			}, forbidden(nodePath(node(w, 0), "/protocols"))...)
		}},
		{RouteID: "protocol.admin.nodes.id.protocols.post", Writes: func(w *World) []Req {
			path := nodePath(node(w, 1), "/protocols")
			return []Req{
				{Persona: Admin, Path: path, Mask: stamped, Label: "reality with a typed key",
					Body: `{"name":"staging reality","type":"vless","port":24443,"tls":2,"reality_settings":"{\"dest\":\"www.example.com:443\",\"private_key\":\"staging-typed-private\",\"short_ids\":[\"ab\"]}"}`},
				{Persona: Admin, Path: path, Mask: stamped, Label: "a placeholder stores nothing",
					Body: `{"name":"staging masked","type":"trojan","port":24444,"tls_settings":"{\"private_key\":\"********\"}"}`},
				{Persona: Admin, Path: nodePath(Missing, "/protocols"), Body: `{"name":"x","type":"vless","port":1}`, Label: "an unknown node"},
				{Persona: Admin, Path: path, Body: `{"name":"x","port":"443"}`, Label: "a port of the wrong type"},
				{Persona: Admin, Path: path, Body: `{"name":`, Label: "invalid JSON"},
			}
		}},
		{RouteID: "protocol.admin.nodes.id.protocols.protocol_id.put", Writes: func(w *World) []Req {
			return []Req{
				{Persona: Admin, Path: protocolPath(w, 0), Body: `{"name":"staging renamed","sort":7}`, Label: "fields without secrets"},
				{Persona: Admin, Path: protocolPath(w, 1), Body: `{"reality_settings":"{\"dest\":\"other.example.com:443\",\"private_key\":\"********\"}"}`, Label: "the placeholder keeps a key"},
				{Persona: Admin, Path: protocolPath(w, 2), Body: `{"name":"a","Name":"b"}`, Label: "a key named twice"},
				{Persona: Admin, Path: fmt.Sprintf("/api/v2/admin/nodes/%d/protocols/%d", node(w, 0), Missing), Body: `{"name":"x"}`, Label: "an unknown protocol"},
			}
		}},
		{RouteID: "protocol.admin.nodes.id.protocols.protocol_id.delete", Writes: func(w *World) []Req {
			return []Req{
				{Persona: Admin, Path: protocolPath(w, 9), Label: "a protocol with its links"},
				{Persona: Admin, Path: fmt.Sprintf("/api/v2/admin/nodes/%d/protocols/%d", node(w, 0), Missing), Label: "an unknown protocol"},
				{Persona: Admin, Path: fmt.Sprintf("/api/v2/admin/nodes/%d/protocols/x", node(w, 0)), Label: "invalid id"},
			}
		}},
		{RouteID: "protocol.admin.nodes.id.sync.post", Writes: func(w *World) []Req {
			return []Req{
				{Persona: Admin, Path: nodePath(node(w, 2), "/sync"), Label: "a node on the legacy transports"},
				{Persona: Admin, Path: nodePath(Missing, "/sync"), Label: "an unknown node"},
				{Persona: Admin, Path: nodePath("x", "/sync"), Label: "invalid id"},
			}
		}},
		{RouteID: "protocol.admin.nodes.id.agent_control.get", Reads: func(w *World) []Req {
			return append([]Req{
				{Persona: Admin, Path: nodePath(node(w, 0), "/agent-control"), Label: "not connected"},
				{Persona: Admin, Path: nodePath(Missing, "/agent-control"), Label: "an unknown node"},
				{Persona: Admin, Path: nodePath("x", "/agent-control"), Label: "invalid id"},
			}, forbidden(nodePath(node(w, 0), "/agent-control"))...)
		}},
		{RouteID: "protocol.admin.nodes.id.agent_control.operations.post", Writes: func(w *World) []Req {
			path := nodePath(node(w, 3), "/agent-control/operations")
			return []Req{
				{Persona: Admin, Path: path, Body: `{"kind":"agent.ping"}`, Label: "a node without a stream"},
				{Persona: Admin, Path: path, Body: `{"kind":"agent.upgrade"}`, Label: "an operation the route does not send"},
				{Persona: Admin, Path: path, Body: `{}`, Label: "no kind"},
				{Persona: Admin, Path: nodePath(Missing, "/agent-control/operations"), Body: `{"kind":"agent.ping"}`, Label: "an unknown node"},
			}
		}},
		{RouteID: "protocol.admin.agent.list.get", Reads: func(w *World) []Req {
			path := "/api/v2/admin/agent/list"
			return append([]Req{{Persona: Admin, Path: path, Label: "WebSocket agents"}}, forbidden(path)...)
		}},
		{RouteID: "protocol.admin.agent.monitor.get", Reads: func(w *World) []Req {
			path := "/api/v2/admin/agent/monitor"
			return append([]Req{
				{Persona: Admin, Path: path, Query: q("node_id", fmt.Sprint(node(w, 0))), Label: "no snapshot"},
				{Persona: Admin, Path: path, Query: q("node_id", "0"), Label: "node zero"},
				{Persona: Admin, Path: path, Label: "no node"},
			}, forbidden(path)...)
		}},
		{RouteID: "protocol.admin.agent.tasks.post", Writes: func(w *World) []Req {
			path := "/api/v2/admin/agent/tasks"
			return []Req{
				{Persona: Admin, Path: path, Body: fmt.Sprintf(`{"node_id":%d,"type":"diagnostic","action":"service_status","params":{"service":"gost"}}`, node(w, 0)), Label: "an offline node"},
				{Persona: Admin, Path: path, Body: `{"node_id":1,"type":"shell","action":"service_status"}`, Label: "a type that is not diagnostic"},
				{Persona: Admin, Path: path, Body: `{"node_id":1}`, Label: "no action"},
			}
		}},
		{RouteID: "protocol.admin.agent.execute.post", Writes: func(w *World) []Req {
			path := "/api/v2/admin/agent/execute"
			return []Req{
				{Persona: Admin, Path: path, Body: fmt.Sprintf(`{"node_id":%d,"action":"service_status","params":{"service":"gost"}}`, node(w, 0)), Label: "an offline node"},
				{Persona: Admin, Path: path, Body: `{"action":"service_status"}`, Label: "no node"},
			}
		}},
	}
}
