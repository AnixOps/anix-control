package service

import (
	"context"
	"errors"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"gorm.io/gorm"
	"log"
	"time"
)

// MaintenanceEventWorker runs one catch-up pass on startup and returns only when
// the active pass has stopped. All work is durable and safe across instances.
type MaintenanceEventWorker struct {
	DB       *gorm.DB
	Sender   MaintenanceSender
	Interval time.Duration
	Batch    int
}

func (w MaintenanceEventWorker) RunOnce(ctx context.Context, now time.Time) error {
	db := w.DB
	if db == nil {
		db = database.Get()
	}
	if db == nil {
		return errors.New("database unavailable")
	}
	db = db.WithContext(ctx)
	_, consumeErr := ProcessMaintenanceEvents(db, w.Batch, now)
	ticketErr := TickMaintenanceTickets(db, now)
	sender := w.Sender
	if sender == nil {
		sender = MaintenanceTransport{}
	}
	_, sendErr := DeliverMaintenanceNotifications(ctx, db, sender, now, w.Batch)
	retentionErr := CleanupMaintenance(db, now)
	return errors.Join(consumeErr, ticketErr, sendErr, retentionErr)
}
func (w MaintenanceEventWorker) Start(ctx context.Context) {
	interval := w.Interval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		if err := w.RunOnce(ctx, time.Now()); err != nil && ctx.Err() == nil {
			log.Print("maintenance worker pass failed; durable work will retry")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
