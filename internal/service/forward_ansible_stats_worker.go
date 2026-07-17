package service

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	defaultForwardAnsibleStatsPollInterval     = 60 * time.Second
	defaultForwardAnsibleStatsIdlePollInterval = 3 * time.Minute
	defaultForwardAnsibleStatsErrorLogInterval = 5 * time.Minute

	forwardAnsibleStatsNftablesPlaybookPath = "config/deploy/ansible/playbooks/forward_collect_stats_nftables.yml"
	forwardAnsibleStatsIptablesPlaybookPath = "config/deploy/ansible/playbooks/forward_collect_stats_iptables.yml"
)

var forwardAnsibleStatsBackends = []string{
	model.ForwardRuntimeBackendNftablesAnsible,
	model.ForwardRuntimeBackendIptablesAnsible,
}

// ForwardAnsibleStatsWorker 定期为 nftables_ansible/iptables_ansible 后端的转发采集流量统计。
// nftables/iptables 的 NAT 计数器只统计经过 DNAT 链的字节数（近似上行方向），没有独立的回程方向
// 计数，因此这里只记录 uploadTotal，downloadTotal 恒为 0，这是计数器本身的限制而非实现遗漏。
type ForwardAnsibleStatsWorker struct {
	db           *gorm.DB
	runtimeSvc   *PanelForwardRuntimeService
	runner       panelForwardRuntimeCommandRunner
	interval     time.Duration
	idleInterval time.Duration
	errorLogger  *forwardBackgroundErrorLogger
}

func NewForwardAnsibleStatsWorker(db *gorm.DB) *ForwardAnsibleStatsWorker {
	if db == nil {
		db = database.Get()
	}
	return &ForwardAnsibleStatsWorker{
		db:           db,
		runtimeSvc:   NewPanelForwardRuntimeService(db),
		runner:       osExecPanelForwardRuntimeCommandRunner{},
		interval:     defaultForwardAnsibleStatsPollInterval,
		idleInterval: defaultForwardAnsibleStatsIdlePollInterval,
		errorLogger:  newForwardBackgroundErrorLogger(defaultForwardAnsibleStatsErrorLogInterval),
	}
}

func (w *ForwardAnsibleStatsWorker) Start(ctx context.Context) {
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
			w.errorLogger.Logf("cycle", "forward ansible stats poll failed: %v", err)
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

func (w *ForwardAnsibleStatsWorker) RunOnce(ctx context.Context) error {
	_, err := w.runOnce(ctx)
	return err
}

func (w *ForwardAnsibleStatsWorker) runOnce(ctx context.Context) (int, error) {
	var forwards []model.Forward
	if err := w.queryDB().
		Preload("Tunnel").
		Where("runtime_backend IN ? AND status = ?", forwardAnsibleStatsBackends, model.ForwardStatusActive).
		Order("id ASC").
		Find(&forwards).Error; err != nil {
		return 0, err
	}
	if len(forwards) == 0 {
		return 0, nil
	}

	panelService := NewPanelForwardService(w.db)

	for i := range forwards {
		uploadTotal, found, err := w.collectForwardTrafficTotal(ctx, &forwards[i])
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return len(forwards), err
			}
			w.errorLogger.Logf("collect:"+strconvFormatUint(forwards[i].ID), "forward ansible stats collect failed for forward %d: %v", forwards[i].ID, err)
			continue
		}
		w.errorLogger.Clear("collect:" + strconvFormatUint(forwards[i].ID))
		if !found {
			continue
		}
		if err := panelService.ApplyForwardTrafficSnapshots([]PanelForwardTrafficSnapshot{{
			ForwardID:     forwards[i].ID,
			Backend:       forwards[i].RuntimeBackend,
			UploadTotal:   uploadTotal,
			DownloadTotal: 0,
		}}); err != nil {
			w.errorLogger.Logf("apply:"+strconvFormatUint(forwards[i].ID), "forward ansible stats apply failed for forward %d: %v", forwards[i].ID, err)
			continue
		}
		w.errorLogger.Clear("apply:" + strconvFormatUint(forwards[i].ID))
	}

	return len(forwards), nil
}

func (w *ForwardAnsibleStatsWorker) queryDB() *gorm.DB {
	if w.db == nil {
		return nil
	}
	return w.db.Session(&gorm.Session{
		Logger: logger.Default.LogMode(logger.Silent),
	})
}

func (w *ForwardAnsibleStatsWorker) collectForwardTrafficTotal(ctx context.Context, forward *model.Forward) (int64, bool, error) {
	if forward == nil || forward.Tunnel == nil {
		return 0, false, nil
	}
	backend := forward.RuntimeBackend

	node, err := w.runtimeSvc.loadExecutionNode(forward.Tunnel)
	if err != nil {
		return 0, false, err
	}

	payload, err := w.runtimeSvc.buildAnsibleRuntimePayload(backend, model.ForwardRuntimeJobActionSync, forward, forward.Tunnel, node)
	if err != nil {
		return 0, false, err
	}
	payload.Playbook = forwardAnsibleStatsPlaybookPathForBackend(backend)

	args, err := payload.commandArgs()
	if err != nil {
		return 0, false, err
	}

	jobCtx, cancel := context.WithTimeout(ctx, payload.timeout(defaultForwardRuntimeJobTimeout))
	defer cancel()

	output, runErr := w.runner.Run(jobCtx, payload.commandName(), args, payload.workingDirectory(), payload.environment())
	if runErr != nil {
		return 0, false, fmt.Errorf("run ansible stats playbook: %w (%s)", runErr, strings.TrimSpace(output))
	}

	total, found := parseForwardAnsibleStatsOutput(output)
	return total, found, nil
}

func forwardAnsibleStatsPlaybookPathForBackend(backend string) string {
	if backend == model.ForwardRuntimeBackendIptablesAnsible {
		return forwardAnsibleStatsIptablesPlaybookPath
	}
	return forwardAnsibleStatsNftablesPlaybookPath
}

func parseForwardAnsibleStatsOutput(output string) (int64, bool) {
	var total int64
	found := false
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		idx := strings.Index(line, "STATS_JSON ")
		if idx == -1 {
			continue
		}
		jsonPart := strings.TrimSpace(line[idx+len("STATS_JSON "):])
		var entry struct {
			Protocol string `json:"protocol"`
			Bytes    int64  `json:"bytes"`
		}
		if err := json.Unmarshal([]byte(jsonPart), &entry); err != nil {
			continue
		}
		found = true
		total += entry.Bytes
	}
	return total, found
}
