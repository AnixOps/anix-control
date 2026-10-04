package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/agentpki"
	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/forwardlegacy"
	"github.com/AnixOps/anix-control/v4/internal/lease"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/modulepki"
	"github.com/AnixOps/anix-control/v4/internal/nodesecrets"
	"github.com/AnixOps/anix-control/v4/internal/packagestore"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"gorm.io/gorm"
)

// bootstrapLockKey is the PostgreSQL advisory lock that serializes schema and
// seed work across Control processes ("anixboot" as ASCII bytes).
const bootstrapLockKey int64 = 0x616e6978626f6f74

// schemaModels is the full application schema created by AutoMigrate.
func schemaModels() []any {
	return []any{
		&model.User{},
		&model.Plan{},
		&model.Order{},
		&model.Payment{},
		&model.PaymentLog{},
		&model.ServerVMess{},
		&model.ServerVLESS{},
		&model.ServerTrojan{},
		&model.ServerShadowsocks{},
		// 新版节点管理
		&model.Node{},
		&model.NodeProtocol{},
		&model.WireGuardPeer{},
		&model.NodeGroup{},
		&model.AuthorizedKey{},
		// 流量与统计日志
		&model.TrafficLog{},
		&model.OnlineLog{},
		&model.StatUser{},
		&model.StatServer{},
		&model.NodeLog{},
		// 订阅分组和模板
		&model.SubscriptionGroup{},
		&model.SubscriptionTemplate{},
		&model.UserSubscriptionGroup{},
		&model.PlanSubscriptionGroup{},
		// 事件与新模型
		&model.Event{},
		// 工单系统
		&model.Ticket{},
		&model.TicketMessage{},
		// 优惠券系统
		&model.Coupon{},
		&model.CouponUsage{},
		// 知识库
		&model.Knowledge{},
		// 流量转发系统
		&model.ForwardNode{},
		&model.ForwardRule{},
		&model.ForwardRoute{},
		&model.ForwardLog{},
		&model.ForwardStats{},
		&model.ForwardTunnel{},
		&model.ForwardUserTunnel{},
		&model.Forward{},
		&model.ForwardPortBinding{},
		&model.SpeedLimit{},
		&model.ForwardRuntimeJob{},
		&model.ForwardTrafficCursor{},
		&model.ForwardCleanAgent{},
		&model.ForwardAgentBridgeTask{},
		&model.ForwardLatencyBucket{},
		// 支付网关
		&model.PaymentGateway{},
		&model.PaymentRecord{},
		// Telegram Bot
		&model.TelegramBot{},
		&model.TelegramUser{},
		&model.TelegramChat{},
		&model.TelegramCommand{},
		&model.TelegramNotification{},
		// 通知系统
		&model.NotificationTemplate{},
		&model.NotificationLog{},
		// MFA多因素认证
		&model.UserMFA{},
		&model.MFALoginAttempt{},
		// 邀请返利系统
		&model.UserLevel{},
		&model.InviteCode{},
		&model.CommissionRecord{},
		&model.CommissionWithdraw{},
		&model.InviteConfig{},
		// 系统管理
		&model.LoadBalancer{},
		&model.SystemConfig{},
		&model.BackupRecord{},
		&model.BackupConfig{},
		&model.OperationLog{},
		&model.AuditLog{},
	}
}

// bootstrapDatabase prepares the schema and seed data before the server (or
// the migrate command) uses the database. On PostgreSQL it holds a session
// advisory lock, so concurrent Control processes (a rolling update, a migrate
// job next to a starting server) run it one at a time instead of racing on
// DDL and seed rows.
func bootstrapDatabase(ctx context.Context, cfg *config.Config, env string) error {
	db := database.Get()
	release, err := acquireBootstrapLock(ctx, db, cfg.Database.Driver)
	if err != nil {
		return err
	}
	defer release()

	// The v4.2 forwarding upgrade's tables come first: once it dropped the
	// flux tables, the schema steps below must not create them again.
	if err := forwardlegacy.EnsureSchema(db); err != nil {
		return fmt.Errorf("ensure forward legacy upgrade schema: %w", err)
	}
	if err := migrateSchema(db, env); err != nil {
		return err
	}
	for _, step := range []struct {
		name string
		run  func(*gorm.DB) error
	}{
		{"WireGuard peer schema", service.EnsureWireGuardPeerSchema},
		{"node runtime health schema", service.EnsureNodeRuntimeHealthSchema},
		{"control kernel schema", service.EnsureKernelSchema},
		{"node secret split schema", nodesecrets.EnsureSchema},
		{"worker lease schema", lease.EnsureSchema},
	} {
		if err := step.run(db); err != nil {
			return fmt.Errorf("ensure %s: %w", step.name, err)
		}
	}
	if err := ensureModulePKI(ctx, cfg, db); err != nil {
		return err
	}
	// Kernel API views only serve package storage leases: a failure must not
	// stop Control, it only fails leases that ask for the missing view.
	if err := packagestore.EnsureKernelAPIViews(db); err != nil {
		log.Printf("Kernel API views were not created: %v. Package storage leases that grant them will fail.", err)
	}
	if err := ensureConfiguredPluginTrustRoot(db, cfg.Plugins.OfficialPublicKey); err != nil {
		return fmt.Errorf("apply configured plugin trust root: %w", err)
	}
	if err := service.BootstrapIdentityPlatformPackage(db, cfg.Plugins.OfficialPublicKey, cfg.Plugins.IdentityBootstrapPackageDir); err != nil {
		return fmt.Errorf("bootstrap identity platform package: %w", err)
	}

	// 初始化管理员账号、默认订阅分组、默认套餐、环境变量中的默认授权密钥
	service.InitAdmin(cfg)
	service.InitSubscriptionDefaults()
	service.InitDefaultPlan()
	service.InitDefaultAuthKeyFromEnv()

	if err := service.InitForwardRuntimeSystemConfig(db); err != nil {
		return fmt.Errorf("initialize forward runtime config: %w", err)
	}
	for _, step := range []struct {
		name string
		run  func(*gorm.DB) error
	}{
		{"observability schema", service.EnsureObservabilitySchema},
		{"stats schema", service.EnsureStatsSchema},
		{"forward bridge schema", forwardlegacy.UnlessDropped(service.EnsureForwardBridgeSchema)},
		{"forward runtime job schema", forwardlegacy.UnlessDropped(service.EnsureForwardRuntimeJobSchema)},
		{"forward port binding schema", forwardlegacy.UnlessDropped(service.EnsureForwardPortBindingSchema)},
		{"forward node metrics_port column", service.EnsureForwardNodeMetricsPortColumn},
		{"agent diagnostic task schema", service.EnsureAgentDiagnosticTaskSchema},
	} {
		if err := step.run(db); err != nil {
			return fmt.Errorf("ensure %s: %w", step.name, err)
		}
	}
	startupForwardLegacyArchive(ctx, db, cfg)
	return nil
}

// startupForwardLegacyArchive archives the flux forwarding data at the
// first v4.2 start (forward-sdk.md section 10, F5c), under the bootstrap
// lock so two processes never both write one. A failure is logged loudly
// but does not stop Control: the drop refuses without an archive anyway.
func startupForwardLegacyArchive(ctx context.Context, db *gorm.DB, cfg *config.Config) {
	written, err := forwardlegacy.StartupArchive(ctx, db, forwardLegacyArchiveDir(cfg), version, time.Now())
	switch {
	case err != nil:
		log.Printf("WARNING: the v4.1 forwarding data was not archived: %v. Run `anix-control forward legacy archive -o <file>`; the old tables stay until `forward legacy drop`.", err)
	case written != nil:
		log.Printf("Archived the v4.1 forwarding data to %s (mode 0600, SHA-256 %s). The old tables stay until `anix-control forward legacy drop` (docs/UPGRADE.md).", written.Record.Path, written.Record.SHA256)
	}
}

// migrateSchema creates the application tables. Development and test run a
// full AutoMigrate. Production never alters existing tables: an empty
// database (a fresh install) gets the full schema, and an existing one only
// gets tables it does not have yet.
func migrateSchema(db *gorm.DB, env string) error {
	// Once the v4.2 upgrade dropped the flux tables they stay dropped.
	models := forwardlegacy.KeepModels(db, schemaModels())
	if env == "development" || env == "test" {
		if err := db.AutoMigrate(models...); err != nil {
			return fmt.Errorf("migrate database: %w", err)
		}
		return nil
	}

	migrator := db.Migrator()
	var missing []any
	for _, value := range models {
		if !migrator.HasTable(value) {
			missing = append(missing, value)
		}
	}
	switch {
	case len(missing) == 0:
		log.Println("Production mode: schema present; existing tables are not altered.")
	case len(missing) == len(models):
		log.Println("Production mode: empty database, creating the full schema.")
		if err := db.AutoMigrate(models...); err != nil {
			return fmt.Errorf("create schema: %w", err)
		}
	default:
		names := make([]string, 0, len(missing))
		for _, value := range missing {
			if err := migrator.CreateTable(value); err != nil {
				return fmt.Errorf("create table for %T: %w", value, err)
			}
			names = append(names, fmt.Sprintf("%T", value))
		}
		log.Printf("Production mode: created missing tables %s; existing tables are not altered.", strings.Join(names, ", "))
	}
	return nil
}

// acquireBootstrapLock takes the bootstrap advisory lock on a dedicated
// PostgreSQL connection. The lock belongs to that session, so it is released
// by the returned function or, if the process dies, when the connection
// closes. SQLite needs no lock: it has a single writer.
func acquireBootstrapLock(ctx context.Context, db *gorm.DB, driver string) (func(), error) {
	switch strings.ToLower(strings.TrimSpace(driver)) {
	case "postgres", "postgresql":
	default:
		return func() {}, nil
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("bootstrap lock: %w", err)
	}
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("bootstrap lock connection: %w", err)
	}
	log.Println("Waiting for the database bootstrap lock...")
	if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", bootstrapLockKey); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("acquire bootstrap lock: %w", err)
	}
	return func() { releaseBootstrapLock(conn) }, nil
}

func releaseBootstrapLock(conn *sql.Conn) {
	if _, err := conn.ExecContext(context.Background(), "SELECT pg_advisory_unlock($1)", bootstrapLockKey); err != nil {
		log.Printf("release bootstrap lock: %v", err)
	}
	_ = conn.Close()
}

// ensureModulePKI creates the built-in module CA and the forward link CA on
// first start. It runs
// under the bootstrap lock, so concurrent kernels create one CA.
func ensureModulePKI(ctx context.Context, cfg *config.Config, db *gorm.DB) error {
	authority, err := modulepki.FromConfig(cfg.ModuleRuntime, db)
	if errors.Is(err, modulepki.ErrBuiltinPKIDisabled) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("module PKI: %w", err)
	}
	if err := authority.Ensure(ctx); err != nil {
		return fmt.Errorf("create the module CA: %w", err)
	}
	// The forward link CA (H28) is a separate root sealed with the same
	// key-encryption key.
	link, err := agentpki.LinkAuthorityFromConfig(cfg, db)
	if err != nil {
		return fmt.Errorf("forward link PKI: %w", err)
	}
	if err := link.Ensure(ctx); err != nil {
		return fmt.Errorf("create the forward link CA: %w", err)
	}
	return nil
}
