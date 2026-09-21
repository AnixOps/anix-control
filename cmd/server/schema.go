package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"gorm.io/gorm"
)

func applicationSchemaModels() []any {
	return []any{
		&model.User{}, &model.Plan{}, &model.Order{}, &model.Payment{}, &model.PaymentLog{},
		&model.ServerVMess{}, &model.ServerVLESS{}, &model.ServerTrojan{}, &model.ServerShadowsocks{},
		&model.ServerHysteria{}, &model.ServerTUIC{}, &model.ServerAnyTLS{}, &model.ServerGroup{}, &model.ServerRoute{},
		&model.Node{}, &model.NodeProtocol{}, &model.WireGuardPeer{}, &model.NodeGroup{}, &model.AuthorizedKey{},
		&model.TrafficLog{}, &model.OnlineLog{}, &model.StatUser{}, &model.StatServer{}, &model.NodeLog{},
		&model.SubscriptionGroup{}, &model.SubscriptionTemplate{}, &model.UserSubscriptionGroup{}, &model.PlanSubscriptionGroup{},
		&model.Event{}, &model.Ticket{}, &model.TicketMessage{}, &model.Coupon{}, &model.CouponUsage{}, &model.Knowledge{},
		&model.ForwardNode{}, &model.ForwardRule{}, &model.ForwardRoute{}, &model.ForwardLog{}, &model.ForwardStats{},
		&model.ForwardTunnel{}, &model.ForwardUserTunnel{}, &model.Forward{}, &model.ForwardPortBinding{}, &model.SpeedLimit{},
		&model.ForwardRuntimeJob{}, &model.ForwardTrafficCursor{}, &model.ForwardCleanAgent{}, &model.ForwardAgentBridgeTask{},
		&model.ForwardLatencyBucket{}, &model.AgentDiagnosticTask{},
		&model.PaymentGateway{}, &model.PaymentRecord{},
		&model.TelegramBot{}, &model.TelegramUser{}, &model.TelegramChat{}, &model.TelegramCommand{}, &model.TelegramNotification{},
		&model.NotificationTemplate{}, &model.NotificationLog{}, &model.UserMFA{}, &model.MFALoginAttempt{},
		&model.UserLevel{}, &model.InviteCode{}, &model.CommissionRecord{}, &model.CommissionWithdraw{}, &model.InviteConfig{},
		&model.LoadBalancer{}, &model.SystemConfig{}, &model.BackupRecord{}, &model.BackupConfig{}, &model.OperationLog{}, &model.AuditLog{},
	}
}

func migrateApplicationSchema(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("database is not initialized")
	}
	if err := db.AutoMigrate(applicationSchemaModels()...); err != nil {
		return fmt.Errorf("application tables: %w", err)
	}
	steps := []struct {
		name string
		fn   func(*gorm.DB) error
	}{
		{"wireguard peer", service.EnsureWireGuardPeerSchema},
		{"node runtime health", service.EnsureNodeRuntimeHealthSchema},
		{"control kernel and maintenance", service.EnsureKernelSchema},
		{"observability", service.EnsureObservabilitySchema},
		{"traffic statistics", service.EnsureStatsSchema},
		{"forward bridge", service.EnsureForwardBridgeSchema},
		{"forward runtime job", service.EnsureForwardRuntimeJobSchema},
		{"forward port binding", service.EnsureForwardPortBindingSchema},
		{"forward node metrics port", service.EnsureForwardNodeMetricsPortColumn},
		{"agent diagnostics", service.EnsureAgentDiagnosticTaskSchema},
	}
	for _, step := range steps {
		if err := step.fn(db); err != nil {
			return fmt.Errorf("%s schema: %w", step.name, err)
		}
	}
	return nil
}

func verifyApplicationSchema(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("database is not initialized")
	}
	models := append([]any{}, applicationSchemaModels()...)
	models = append(models, model.KernelModels()...)
	models = append(models, model.MaintenanceModels()...)
	models = append(models, model.MaintenanceOperationModels()...)

	missing := make([]string, 0)
	checkedTables := make(map[string]struct{})
	for _, value := range models {
		statement := &gorm.Statement{DB: db}
		if err := statement.Parse(value); err != nil {
			return fmt.Errorf("parse schema model %T: %w", value, err)
		}
		table := statement.Schema.Table
		if _, checked := checkedTables[table]; !checked {
			if !db.Migrator().HasTable(value) {
				missing = append(missing, "table "+table)
				checkedTables[table] = struct{}{}
				continue
			}
			checkedTables[table] = struct{}{}
		}
		for _, field := range statement.Schema.Fields {
			if field.DBName != "" && !db.Migrator().HasColumn(value, field.DBName) {
				missing = append(missing, table+"."+field.DBName)
			}
		}
	}
	if !db.Migrator().HasTable("v2_subscription_group_node_protocols") {
		missing = append(missing, "table v2_subscription_group_node_protocols")
	}
	if db.Migrator().HasTable(&model.ForwardRuntimeJob{}) && !db.Migrator().HasIndex(&model.ForwardRuntimeJob{}, "idx_forward_runtime_job_active_forward") {
		missing = append(missing, "index idx_forward_runtime_job_active_forward")
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("missing or outdated schema objects: %s", strings.Join(missing, ", "))
	}
	return nil
}
