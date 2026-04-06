package service

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
	"gorm.io/gorm"
)

type ForwardFlowResetWorker struct {
	db       *gorm.DB
	location *time.Location
}

func NewForwardFlowResetWorker(db *gorm.DB) *ForwardFlowResetWorker {
	if db == nil {
		db = database.Get()
	}
	return &ForwardFlowResetWorker{
		db:       db,
		location: time.Local,
	}
}

func (w *ForwardFlowResetWorker) Start(ctx context.Context) {
	for {
		nextRun := w.nextRunTime(time.Now())
		timer := time.NewTimer(time.Until(nextRun))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			if err := w.RunOnce(time.Now()); err != nil {
				log.Printf("forward flow reset worker failed: %v", err)
			}
		}
	}
}

func (w *ForwardFlowResetWorker) RunOnce(now time.Time) error {
	now = now.In(w.location)
	currentDay := now.Day()
	lastDayOfMonth := daysInMonth(now)
	nowUnix := now.Unix()
	nowUnixMilli := now.UnixMilli()

	return w.db.Transaction(func(tx *gorm.DB) error {
		if err := w.resetUserTunnelTraffic(tx, currentDay, lastDayOfMonth); err != nil {
			return err
		}
		if err := w.resetUserTraffic(tx, currentDay, lastDayOfMonth); err != nil {
			return err
		}
		if err := w.pauseExpiredUserForwards(tx, nowUnix); err != nil {
			return err
		}
		return w.disableExpiredUserTunnels(tx, nowUnixMilli)
	})
}

func (w *ForwardFlowResetWorker) nextRunTime(now time.Time) time.Time {
	localNow := now.In(w.location)
	next := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 5, 0, w.location)
	if !next.After(localNow) {
		next = next.Add(24 * time.Hour)
	}
	return next
}

func (w *ForwardFlowResetWorker) resetUserTunnelTraffic(tx *gorm.DB, currentDay, lastDayOfMonth int) error {
	var permissions []model.ForwardUserTunnel
	query := tx.Model(&model.ForwardUserTunnel{}).Where("flow_reset_time <> 0")
	if currentDay == lastDayOfMonth {
		query = query.Where("(flow_reset_time = ? OR flow_reset_time > ?)", currentDay, lastDayOfMonth)
	} else {
		query = query.Where("flow_reset_time = ?", currentDay)
	}

	if err := query.Find(&permissions).Error; err != nil {
		return err
	}

	for _, permission := range permissions {
		if err := tx.Model(&model.ForwardUserTunnel{}).
			Where("id = ?", permission.ID).
			Updates(map[string]any{
				"in_flow":  0,
				"out_flow": 0,
			}).Error; err != nil {
			return fmt.Errorf("reset user tunnel %d: %w", permission.ID, err)
		}
		if err := tx.Model(&model.Forward{}).
			Where("user_id = ? AND tunnel_id = ?", permission.UserID, permission.TunnelID).
			Updates(map[string]any{
				"in_flow":  0,
				"out_flow": 0,
			}).Error; err != nil {
			return fmt.Errorf("reset forwards for user %d tunnel %d: %w", permission.UserID, permission.TunnelID, err)
		}
	}
	return nil
}

func (w *ForwardFlowResetWorker) resetUserTraffic(tx *gorm.DB, currentDay, lastDayOfMonth int) error {
	if !tx.Migrator().HasColumn(&model.User{}, "flow_reset_time") {
		return nil
	}

	var users []model.User
	query := tx.Model(&model.User{}).Where("flow_reset_time <> 0")
	if currentDay == lastDayOfMonth {
		query = query.Where("(flow_reset_time = ? OR flow_reset_time > ?)", currentDay, lastDayOfMonth)
	} else {
		query = query.Where("flow_reset_time = ?", currentDay)
	}

	if err := query.Find(&users).Error; err != nil {
		return err
	}
	if len(users) == 0 {
		return nil
	}

	updates := make(map[string]any)
	if tx.Migrator().HasColumn(&model.User{}, "u") {
		updates["u"] = 0
	}
	if tx.Migrator().HasColumn(&model.User{}, "d") {
		updates["d"] = 0
	}
	if tx.Migrator().HasColumn(&model.User{}, "in_flow") {
		updates["in_flow"] = 0
	}
	if tx.Migrator().HasColumn(&model.User{}, "out_flow") {
		updates["out_flow"] = 0
	}
	if len(updates) == 0 {
		return nil
	}

	for _, user := range users {
		if err := tx.Model(&model.User{}).Where("id = ?", user.ID).Updates(updates).Error; err != nil {
			return fmt.Errorf("reset user flow %d: %w", user.ID, err)
		}
	}
	return nil
}

func (w *ForwardFlowResetWorker) disableExpiredUserTunnels(tx *gorm.DB, nowUnixMilli int64) error {
	var expiredPermissions []model.ForwardUserTunnel
	if err := tx.Model(&model.ForwardUserTunnel{}).
		Select("id", "user_id", "tunnel_id").
		Where("status = ? AND exp_time > 0 AND exp_time <= ?", model.ForwardUserTunnelStatusActive, nowUnixMilli).
		Find(&expiredPermissions).Error; err != nil {
		return err
	}
	if len(expiredPermissions) == 0 {
		return nil
	}

	panelService := NewPanelForwardService(tx)
	for _, permission := range expiredPermissions {
		forwards, err := panelService.listUserTunnelForwards(permission.UserID, permission.TunnelID, model.ForwardStatusActive)
		if err != nil {
			return fmt.Errorf("list expired user tunnel forwards %d: %w", permission.ID, err)
		}
		for i := range forwards {
			if err := panelService.pauseManagedForward(&forwards[i]); err != nil {
				return fmt.Errorf("pause expired user tunnel forward %d: %w", forwards[i].ID, err)
			}
		}
		if err := tx.Model(&model.ForwardUserTunnel{}).
			Where("id = ? AND status = ?", permission.ID, model.ForwardUserTunnelStatusActive).
			Update("status", model.ForwardUserTunnelStatusDisabled).Error; err != nil {
			return fmt.Errorf("disable expired user tunnel %d: %w", permission.ID, err)
		}
	}
	return nil
}

func (w *ForwardFlowResetWorker) pauseExpiredUserForwards(tx *gorm.DB, nowUnix int64) error {
	var expiredUsers []model.User
	if err := tx.Model(&model.User{}).
		Select("id").
		Where("expired_at IS NOT NULL AND expired_at > 0 AND expired_at <= ?", nowUnix).
		Find(&expiredUsers).Error; err != nil {
		return err
	}
	if len(expiredUsers) == 0 {
		return nil
	}

	userIDs := make([]uint, 0, len(expiredUsers))
	for _, user := range expiredUsers {
		userIDs = append(userIDs, user.ID)
	}

	var forwards []model.Forward
	if err := tx.Where("user_id IN ? AND status = ?", userIDs, model.ForwardStatusActive).Order("id ASC").Find(&forwards).Error; err != nil {
		return fmt.Errorf("list expired user forwards: %w", err)
	}
	if len(forwards) == 0 {
		return nil
	}

	panelService := NewPanelForwardService(tx)
	for i := range forwards {
		if err := panelService.pauseManagedForward(&forwards[i]); err != nil {
			return fmt.Errorf("pause expired user forward %d: %w", forwards[i].ID, err)
		}
	}
	return nil
}

func daysInMonth(now time.Time) int {
	return time.Date(now.Year(), now.Month()+1, 0, 0, 0, 0, 0, now.Location()).Day()
}
