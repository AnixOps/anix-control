package service

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/gost"
	"github.com/anixops/v2board/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	defaultForwardGostStatsPollInterval     = 30 * time.Second
	defaultForwardGostStatsIdlePollInterval = 2 * time.Minute
	defaultForwardGostStatsErrorLogInterval = 5 * time.Minute

	forwardGostStatsPollIntervalEnvVar     = "FORWARD_GOST_STATS_POLL_INTERVAL"
	forwardGostStatsIdlePollIntervalEnvVar = "FORWARD_GOST_STATS_IDLE_POLL_INTERVAL"
	forwardGostStatsErrorLogIntervalEnvVar = "FORWARD_GOST_STATS_ERROR_LOG_INTERVAL"
)

type ForwardGostStatsWorker struct {
	db           *gorm.DB
	interval     time.Duration
	idleInterval time.Duration
	errorLogger  *forwardBackgroundErrorLogger
}

func NewForwardGostStatsWorker(db *gorm.DB) *ForwardGostStatsWorker {
	if db == nil {
		db = database.Get()
	}
	interval := loadForwardBackgroundIntervalFromEnv(forwardGostStatsPollIntervalEnvVar, defaultForwardGostStatsPollInterval)
	idleInterval := normalizeForwardIdlePollInterval(
		interval,
		loadForwardBackgroundIntervalFromEnv(forwardGostStatsIdlePollIntervalEnvVar, defaultForwardGostStatsIdlePollInterval),
	)
	errorLogInterval := loadForwardBackgroundIntervalFromEnv(forwardGostStatsErrorLogIntervalEnvVar, defaultForwardGostStatsErrorLogInterval)
	return &ForwardGostStatsWorker{
		db:           db,
		interval:     interval,
		idleInterval: idleInterval,
		errorLogger:  newForwardBackgroundErrorLogger(errorLogInterval),
	}
}

func (w *ForwardGostStatsWorker) Start(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}

	nextDelay := time.Duration(0)
	for {
		if !waitForwardBackgroundCycle(ctx, nextDelay) {
			return
		}

		activeForwards, err := w.runOnce(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			w.errorLogger.Logf("cycle", "forward gost stats poll failed: %v", err)
			nextDelay = w.interval
			continue
		}
		w.errorLogger.Clear("cycle")

		if activeForwards == 0 {
			nextDelay = w.idleInterval
			continue
		}
		nextDelay = w.interval
	}
}

func (w *ForwardGostStatsWorker) RunOnce(ctx context.Context) error {
	_, err := w.runOnce(ctx)
	return err
}

func (w *ForwardGostStatsWorker) runOnce(ctx context.Context) (int, error) {
	var forwards []model.Forward
	if err := w.queryDB().
		Preload("Tunnel").
		Where("runtime_backend = ? AND status = ?", model.ForwardRuntimeBackendGost, model.ForwardStatusActive).
		Order("id ASC").
		Find(&forwards).Error; err != nil {
		return 0, err
	}
	if len(forwards) == 0 {
		return 0, nil
	}

	panelService := NewPanelForwardService(w.db)
	manager := gost.NewManager(w.db)

	for i := range forwards {
		uploadTotal, downloadTotal, found, err := w.collectForwardTrafficTotals(ctx, manager, &forwards[i])
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return len(forwards), err
			}
			w.errorLogger.Logf("collect:"+strconvFormatUint(forwards[i].ID), "forward gost stats collect failed for forward %d: %v", forwards[i].ID, err)
			continue
		}
		w.errorLogger.Clear("collect:" + strconvFormatUint(forwards[i].ID))
		if !found {
			continue
		}
		if err := panelService.ApplyForwardTrafficSnapshots([]PanelForwardTrafficSnapshot{{
			ForwardID:     forwards[i].ID,
			Backend:       model.ForwardRuntimeBackendGost,
			UploadTotal:   uploadTotal,
			DownloadTotal: downloadTotal,
		}}); err != nil {
			w.errorLogger.Logf("apply:"+strconvFormatUint(forwards[i].ID), "forward gost stats apply failed for forward %d: %v", forwards[i].ID, err)
			continue
		}
		w.errorLogger.Clear("apply:" + strconvFormatUint(forwards[i].ID))
	}

	return len(forwards), nil
}

func (w *ForwardGostStatsWorker) queryDB() *gorm.DB {
	if w.db == nil {
		return nil
	}
	return w.db.Session(&gorm.Session{
		Logger: logger.Default.LogMode(logger.Silent),
	})
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
