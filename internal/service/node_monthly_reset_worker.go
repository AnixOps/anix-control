package service

import (
	"context"
	"log"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
)

// NodeMonthlyResetWorker 按每个节点自己的 MonthlyResetDay 把 monthly_upload/
// monthly_download 清零, 用于月流量限额的周期性重置(仅统计层面, 不做任何
// 自动禁用/限速动作)。
type NodeMonthlyResetWorker struct {
	db       *gorm.DB
	location *time.Location
}

func NewNodeMonthlyResetWorker(db *gorm.DB) *NodeMonthlyResetWorker {
	if db == nil {
		db = database.Get()
	}
	return &NodeMonthlyResetWorker{
		db:       db,
		location: time.Local,
	}
}

func (w *NodeMonthlyResetWorker) Start(ctx context.Context) {
	for {
		nextRun := w.nextRunTime(time.Now())
		timer := time.NewTimer(time.Until(nextRun))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			if err := w.RunOnce(time.Now()); err != nil {
				log.Printf("node monthly reset worker failed: %v", err)
			}
		}
	}
}

func (w *NodeMonthlyResetWorker) RunOnce(now time.Time) error {
	now = now.In(w.location)
	currentDay := now.Day()
	lastDayOfMonth := daysInMonth(now)

	query := w.db.Model(&model.Node{})
	if currentDay == lastDayOfMonth {
		// 月末兜底: reset_day 设成了大于本月天数的日子(比如 30/31 但本月只有 28/29 天)
		query = query.Where("monthly_reset_day = ? OR monthly_reset_day > ?", currentDay, lastDayOfMonth)
	} else {
		query = query.Where("monthly_reset_day = ?", currentDay)
	}

	return query.Updates(map[string]any{
		"monthly_upload":   0,
		"monthly_download": 0,
	}).Error
}

func (w *NodeMonthlyResetWorker) nextRunTime(now time.Time) time.Time {
	localNow := now.In(w.location)
	next := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 10, 0, w.location)
	if !next.After(localNow) {
		next = next.Add(24 * time.Hour)
	}
	return next
}
