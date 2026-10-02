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
//
// nftables 路径在 forward hook 的记账链里用两个具名计数器统计每个转发：
// ct direction original 记为 upload（客户端到目标），ct direction reply 记为
// download（目标回到客户端），与 gost 路径的 u/d 同义，再经同一套
// traffic_ratio / flow 计费换算。具名计数器在重新下发时保留，只在删除转发时清除。
//
// 尚未迁移到 inet 表的转发（以及 iptables 路径）只上报 nat 链计数，
// 那是每条连接首包的字节数，只能当作 upload，download 为 0。两种快照使用
// 不同的游标键，迁移时不会互相抵消。
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
		snapshot, found, err := w.collectForwardTrafficTotal(ctx, &forwards[i])
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
		if err := panelService.ApplyForwardTrafficSnapshots([]PanelForwardTrafficSnapshot{
			snapshot.trafficSnapshot(forwards[i].ID, forwards[i].RuntimeBackend),
		}); err != nil {
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

func (w *ForwardAnsibleStatsWorker) collectForwardTrafficTotal(ctx context.Context, forward *model.Forward) (forwardAnsibleStatsTotals, bool, error) {
	if forward == nil || forward.Tunnel == nil {
		return forwardAnsibleStatsTotals{}, false, nil
	}
	backend := forward.RuntimeBackend

	node, err := w.runtimeSvc.loadExecutionNode(forward.Tunnel)
	if err != nil {
		return forwardAnsibleStatsTotals{}, false, err
	}

	payload, err := w.runtimeSvc.buildAnsibleRuntimePayload(backend, model.ForwardRuntimeJobActionSync, forward, forward.Tunnel, node)
	if err != nil {
		return forwardAnsibleStatsTotals{}, false, err
	}
	payload.Playbook = forwardAnsibleStatsPlaybookPathForBackend(backend)

	args, err := payload.commandArgs()
	if err != nil {
		return forwardAnsibleStatsTotals{}, false, err
	}

	jobCtx, cancel := context.WithTimeout(ctx, payload.timeout(defaultForwardRuntimeJobTimeout))
	defer cancel()

	output, runErr := w.runner.Run(jobCtx, payload.commandName(), args, payload.workingDirectory(), payload.environment())
	if runErr != nil {
		return forwardAnsibleStatsTotals{}, false, fmt.Errorf("run ansible stats playbook: %w (%s)", runErr, strings.TrimSpace(output))
	}

	totals, found := parseForwardAnsibleStatsOutput(output)
	return totals, found, nil
}

func forwardAnsibleStatsPlaybookPathForBackend(backend string) string {
	if backend == model.ForwardRuntimeBackendIptablesAnsible {
		return forwardAnsibleStatsIptablesPlaybookPath
	}
	return forwardAnsibleStatsNftablesPlaybookPath
}

// forwardAnsibleStatsCtCursorSuffix keys the traffic cursor of the
// ct-direction counters apart from the legacy nat-chain cursor, so the
// switch from one counter to the other on migration is not read as a
// counter reset or a jump.
const forwardAnsibleStatsCtCursorSuffix = ":ct"

// forwardAnsibleStatsTotals is the cumulative traffic of one forward over all
// its protocols. Legacy is set when the node only had the old nat-chain
// counter (upload only).
type forwardAnsibleStatsTotals struct {
	Upload   int64
	Download int64
	Legacy   bool
}

func (t forwardAnsibleStatsTotals) trafficSnapshot(forwardID uint, backend string) PanelForwardTrafficSnapshot {
	cursor := backend
	if !t.Legacy {
		cursor = backend + forwardAnsibleStatsCtCursorSuffix
	}
	return PanelForwardTrafficSnapshot{
		ForwardID:     forwardID,
		Backend:       cursor,
		UploadTotal:   t.Upload,
		DownloadTotal: t.Download,
	}
}

type forwardAnsibleStatsEntry struct {
	Protocol string `json:"protocol"`
	Upload   *int64 `json:"upload"`
	Download *int64 `json:"download"`
	Bytes    *int64 `json:"bytes"`
	Legacy   bool   `json:"legacy"`
}

// parseForwardAnsibleStatsOutput reads the STATS_JSON lines of a stats
// playbook run. A line is either the raw script output or the same text
// inside an Ansible debug message (quotes escaped as \"). Each line carries a
// cumulative total for one protocol, so a protocol printed twice (for
// example raw and in the debug task with -v) counts once. Lines with
// upload/download are the ct-direction counters; lines with only bytes are
// the legacy nat-chain counters and count as upload. When both kinds are
// present the ct-direction counters win.
func parseForwardAnsibleStatsOutput(output string) (forwardAnsibleStatsTotals, bool) {
	type protoTotals struct{ upload, download int64 }
	current := map[string]protoTotals{}
	legacy := map[string]protoTotals{}

	scanner := bufio.NewScanner(strings.NewReader(output))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		idx := strings.Index(line, "STATS_JSON ")
		if idx == -1 {
			continue
		}
		jsonPart := strings.ReplaceAll(line[idx+len("STATS_JSON "):], `\"`, `"`)
		var entry forwardAnsibleStatsEntry
		if err := json.NewDecoder(strings.NewReader(jsonPart)).Decode(&entry); err != nil {
			continue
		}
		protocol := strings.ToLower(strings.TrimSpace(entry.Protocol))
		switch {
		case entry.Upload != nil || entry.Download != nil:
			var totals protoTotals
			if entry.Upload != nil {
				totals.upload = *entry.Upload
			}
			if entry.Download != nil {
				totals.download = *entry.Download
			}
			if totals.upload < 0 || totals.download < 0 {
				continue
			}
			current[protocol] = totals
		case entry.Bytes != nil:
			if *entry.Bytes < 0 {
				continue
			}
			legacy[protocol] = protoTotals{upload: *entry.Bytes}
		}
	}

	source, isLegacy := current, false
	if len(current) == 0 {
		source, isLegacy = legacy, true
	}
	if len(source) == 0 {
		return forwardAnsibleStatsTotals{}, false
	}
	result := forwardAnsibleStatsTotals{Legacy: isLegacy}
	for _, totals := range source {
		result.Upload += totals.upload
		result.Download += totals.download
	}
	return result, true
}
