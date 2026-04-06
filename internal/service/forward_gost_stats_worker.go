package service

import (
	"context"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/gost"
	"github.com/anixops/v2board/internal/model"
	"gorm.io/gorm"
)

const defaultForwardGostStatsPollInterval = 30 * time.Second

type ForwardGostStatsWorker struct {
	db       *gorm.DB
	interval time.Duration
}

func NewForwardGostStatsWorker(db *gorm.DB) *ForwardGostStatsWorker {
	if db == nil {
		db = database.Get()
	}
	return &ForwardGostStatsWorker{
		db:       db,
		interval: defaultForwardGostStatsPollInterval,
	}
}

func (w *ForwardGostStatsWorker) Start(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := w.RunOnce(ctx); err != nil {
		log.Printf("forward gost stats initial poll failed: %v", err)
	}

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.RunOnce(ctx); err != nil {
				log.Printf("forward gost stats poll failed: %v", err)
			}
		}
	}
}

func (w *ForwardGostStatsWorker) RunOnce(ctx context.Context) error {
	var forwards []model.Forward
	if err := w.db.
		Preload("Tunnel").
		Where("runtime_backend = ? AND status = ?", model.ForwardRuntimeBackendGost, model.ForwardStatusActive).
		Order("id ASC").
		Find(&forwards).Error; err != nil {
		return err
	}
	if len(forwards) == 0 {
		return nil
	}

	panelService := NewPanelForwardService(w.db)
	manager := gost.NewManager(w.db)

	for i := range forwards {
		uploadTotal, downloadTotal, found, err := w.collectForwardTrafficTotals(ctx, manager, &forwards[i])
		if err != nil {
			log.Printf("forward gost stats collect failed for forward %d: %v", forwards[i].ID, err)
			continue
		}
		if !found {
			continue
		}
		if err := panelService.ApplyForwardTrafficSnapshots([]PanelForwardTrafficSnapshot{{
			ForwardID:     forwards[i].ID,
			Backend:       model.ForwardRuntimeBackendGost,
			UploadTotal:   uploadTotal,
			DownloadTotal: downloadTotal,
		}}); err != nil {
			log.Printf("forward gost stats apply failed for forward %d: %v", forwards[i].ID, err)
		}
	}

	return nil
}

func (w *ForwardGostStatsWorker) collectForwardTrafficTotals(ctx context.Context, manager *gost.Manager, forward *model.Forward) (int64, int64, bool, error) {
	if forward == nil || forward.Tunnel == nil || forward.Tunnel.InNodeID == 0 {
		return 0, 0, false, nil
	}

	client, err := manager.GetClient(forward.Tunnel.InNodeID)
	if err != nil {
		return 0, 0, false, err
	}

	var (
		found         bool
		uploadTotal   int64
		downloadTotal int64
	)
	for _, serviceName := range panelForwardGostServiceNames(forward.ID, forward.Tunnel.Protocol) {
		stats, err := client.GetServiceStats(ctx, serviceName)
		if err != nil {
			if isGostStatsMissing(err) {
				continue
			}
			return 0, 0, false, err
		}
		found = true
		uploadTotal += stats.Total.InBytes
		downloadTotal += stats.Total.OutBytes
	}

	return uploadTotal, downloadTotal, found, nil
}

func panelForwardGostServiceNames(forwardID uint, protocol string) []string {
	switch strings.ToLower(strings.TrimSpace(protocol)) {
	case "udp":
		return []string{"panel-forward-" + strconvFormatUint(forwardID) + "-udp"}
	case "both":
		return []string{
			"panel-forward-" + strconvFormatUint(forwardID),
			"panel-forward-" + strconvFormatUint(forwardID) + "-udp",
		}
	default:
		return []string{"panel-forward-" + strconvFormatUint(forwardID)}
	}
}

func isGostStatsMissing(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "404") || strings.Contains(message, "not found")
}

func strconvFormatUint(value uint) string {
	return strconv.FormatUint(uint64(value), 10)
}
