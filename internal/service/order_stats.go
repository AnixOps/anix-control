package service

// The order and income part of the dashboard statistics. It belongs to the
// commercial set (config/commercial-boundary.json) and reaches the dashboard
// through RegisterDashboardContributor.

import (
	"fmt"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
)

func init() {
	RegisterDashboardContributor(orderDashboardStats)
}

func orderDashboardStats(db *gorm.DB, stats *DashboardStats, todayStart, monthStart time.Time) error {
	checkQuery := func(label string, tx *gorm.DB) error {
		if tx.Error != nil {
			return fmt.Errorf("%s: %w", label, tx.Error)
		}
		return nil
	}

	if err := checkQuery("count total orders", db.Model(&model.Order{}).Count(&stats.TotalOrders)); err != nil {
		return err
	}
	if err := checkQuery("count pending orders", db.Model(&model.Order{}).Where("status = 0").Count(&stats.PendingOrders)); err != nil {
		return err
	}
	if err := checkQuery("count paid orders", db.Model(&model.Order{}).Where("status IN (1, 3)").Count(&stats.PaidOrders)); err != nil {
		return err
	}
	if err := checkQuery("sum total revenue", db.Model(&model.Order{}).
		Where("status IN (1, 3)").
		Select("COALESCE(SUM(total_amount), 0)").
		Scan(&stats.TotalRevenue)); err != nil {
		return err
	}
	if err := checkQuery("sum monthly income", db.Model(&model.Order{}).
		Where("status IN (1, 3) AND paid_at >= ?", monthStart.Unix()).
		Select("COALESCE(SUM(total_amount), 0)").
		Scan(&stats.MonthlyIncome)); err != nil {
		return err
	}
	if err := checkQuery("sum today income", db.Model(&model.Order{}).
		Where("status IN (1, 3) AND paid_at >= ?", todayStart.Unix()).
		Select("COALESCE(SUM(total_amount), 0)").
		Scan(&stats.TodayIncome)); err != nil {
		return err
	}
	return nil
}
