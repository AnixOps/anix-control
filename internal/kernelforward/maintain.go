package kernelforward

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
)

// MaintenanceInterval is how often the singleton worker maintains the
// forwarding state.
const MaintenanceInterval = time.Minute

// Maintain does the forwarding state's timed work: it replans when a
// route's expiry passed (Control pauses it, forward-sdk.md section 5.3),
// and forgets request ids after RequestRetention. Allocations whose grace
// period ended are released by the next plan.
func (s *Service) Maintain(ctx context.Context) error {
	db, err := s.db(ctx)
	if err != nil {
		return err
	}
	now := s.now()
	if err := db.Where("created_at < ?", now.Add(-RequestRetention)).Delete(&model.KernelForwardRequest{}).Error; err != nil {
		return fmt.Errorf("kernel forward: prune requests: %w", err)
	}
	var rows []model.KernelForwardRoute
	if err := db.Where("enforced <> ?", EnforcedExpired).Find(&rows).Error; err != nil {
		return fmt.Errorf("kernel forward: load routes: %w", err)
	}
	for _, row := range rows {
		route, err := decodeRoute(row)
		if err != nil {
			return err
		}
		if expires := route.GetLimits().GetExpiresAtUnixMs(); expires > 0 && now.UnixMilli() >= expires {
			_, err := s.Replan(ctx, "expiry")
			return err
		}
	}
	return nil
}

// Run maintains the forwarding state every MaintenanceInterval until ctx
// ends. Only the singleton-worker lease holder runs it.
func (s *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(MaintenanceInterval)
	defer ticker.Stop()
	for {
		if err := s.Maintain(ctx); err != nil && ctx.Err() == nil {
			slog.Warn("forward maintenance failed", "component", "kernel-forward", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
