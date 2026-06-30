package service

import (
	"context"
	"math"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	defaultForwardLatencyBucketInterval   = 60 * time.Second
	// Idle poll equals one active cycle so a newly registered node appears within
	// ~60s instead of waiting out a long idle sleep. Ops can raise it via config.
	defaultForwardLatencyIdleInterval     = 60 * time.Second
	defaultForwardLatencyErrorLogInterval = 5 * time.Minute
	defaultForwardLatencyDialsPerProbe    = 5
	defaultForwardLatencyDialTimeout      = 3 * time.Second
	defaultForwardLatencyConcurrency      = 16
	defaultForwardLatencyRetentionDays    = 7

	forwardLatencyCleanupInterval = time.Hour
	forwardLatencyWriteBatchSize  = 100
)

// ForwardLatencyProber periodically TCPing-probes forward/tunnel/node targets and
// stores one pre-aggregated bucket row per (target, time-bucket).
type ForwardLatencyProber struct {
	db           *gorm.DB
	interval     time.Duration
	idleInterval time.Duration
	dialTimeout  time.Duration
	retention    time.Duration
	dials        int
	concurrency  int
	errorLogger  *forwardBackgroundErrorLogger
	lastCleanup  time.Time
}

// probeTarget is one resolved dial endpoint, keyed for dedup.
type probeTarget struct {
	Key        string
	TargetType string
	TargetID   uint
	Label      string
	Host       string
	Port       int
}

func NewForwardLatencyProber(db *gorm.DB) *ForwardLatencyProber {
	if db == nil {
		db = database.Get()
	}
	settings := loadForwardLatencyProberSettings()
	return &ForwardLatencyProber{
		db:           db,
		interval:     settings.PollInterval,
		idleInterval: settings.IdlePollInterval,
		dialTimeout:  settings.DialTimeout,
		retention:    settings.Retention,
		dials:        settings.Dials,
		concurrency:  settings.Concurrency,
		errorLogger:  newForwardBackgroundErrorLogger(settings.ErrorLogInterval),
	}
}

func (w *ForwardLatencyProber) Start(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}

	nextDelay := time.Duration(0)
	for {
		if !waitForwardBackgroundCycle(ctx, nextDelay) {
			return
		}

		probed, err := w.runOnce(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			w.errorLogger.Logf("cycle", "forward latency prober cycle failed: %v", err)
			nextDelay = w.interval
			continue
		}
		w.errorLogger.Clear("cycle")

		if probed == 0 {
			nextDelay = w.idleInterval
			continue
		}
		nextDelay = w.interval
	}
}

// RunOnce executes a single probe cycle. Exposed for tests.
func (w *ForwardLatencyProber) RunOnce(ctx context.Context) error {
	_, err := w.runOnce(ctx)
	return err
}

func (w *ForwardLatencyProber) runOnce(ctx context.Context) (int, error) {
	targets, err := w.enumerateTargets(ctx)
	if err != nil {
		return 0, err
	}
	if len(targets) == 0 {
		w.cleanup(ctx)
		return 0, nil
	}

	bucketAt := time.Now().Truncate(w.interval)
	buckets := w.probeAll(ctx, targets, bucketAt)
	if ctx.Err() != nil {
		return len(targets), ctx.Err()
	}

	if len(buckets) > 0 {
		if err := w.db.WithContext(ctx).CreateInBatches(buckets, forwardLatencyWriteBatchSize).Error; err != nil {
			return len(targets), err
		}
	}

	w.cleanup(ctx)
	return len(targets), nil
}

// enumerateTargets gathers probe endpoints from the three sources and dedups by Key.
func (w *ForwardLatencyProber) enumerateTargets(ctx context.Context) ([]probeTarget, error) {
	db := w.queryDB().WithContext(ctx)
	deduped := make(map[string]probeTarget)

	add := func(targetType string, id uint, label, host string, port int) {
		host = strings.Trim(strings.TrimSpace(host), "[]")
		if host == "" || port <= 0 || port > 65535 {
			return
		}
		key := targetType + ":" + strconv.FormatUint(uint64(id), 10) + ":" + host + ":" + strconv.Itoa(port)
		if _, ok := deduped[key]; ok {
			return
		}
		deduped[key] = probeTarget{
			Key:        key,
			TargetType: targetType,
			TargetID:   id,
			Label:      label,
			Host:       host,
			Port:       port,
		}
	}

	// 1. forward -> remote_addr targets
	var forwards []model.Forward
	if err := db.Where("status = ?", model.ForwardStatusActive).Order("id ASC").Find(&forwards).Error; err != nil {
		return nil, err
	}
	for i := range forwards {
		for _, raw := range strings.FieldsFunc(forwards[i].RemoteAddr, func(r rune) bool { return r == ',' || r == '\n' || r == '\r' }) {
			host, port, err := splitTarget(strings.TrimSpace(raw))
			if err != nil {
				continue
			}
			add(model.LatencyTargetTypeForward, forwards[i].ID, forwards[i].Name, host, port)
		}
	}

	// 2. tunnel relay/exit node endpoints
	var tunnels []model.ForwardTunnel
	if err := db.Where("status = ?", model.ForwardTunnelStatusActive).Order("id ASC").Find(&tunnels).Error; err != nil {
		return nil, err
	}
	for i := range tunnels {
		if host, port, ok := w.resolveNodeEndpoint(db, tunnels[i].InNodeID, tunnels[i].InIP); ok {
			add(model.LatencyTargetTypeTunnelNode, tunnels[i].InNodeID, tunnels[i].Name+" 入口", host, port)
		}
		if tunnels[i].OutNodeID != nil {
			if host, port, ok := w.resolveNodeEndpoint(db, *tunnels[i].OutNodeID, tunnels[i].OutIP); ok {
				add(model.LatencyTargetTypeTunnelNode, *tunnels[i].OutNodeID, tunnels[i].Name+" 出口", host, port)
			}
		}
	}

	// 3. enabled ForwardNode management endpoints
	var nodes []model.ForwardNode
	if err := db.Where("enabled = ?", true).Order("id ASC").Find(&nodes).Error; err != nil {
		return nil, err
	}
	for i := range nodes {
		add(model.LatencyTargetTypeForwardNode, nodes[i].ID, nodes[i].Name, nodes[i].Host, nodes[i].Port)
	}

	// 4. regular proxy nodes (v2_node, registered by V2bX) that are not disabled
	var proxyNodes []model.Node
	if err := db.Where("status <> ?", model.NodeStatusDisabled).Order("id ASC").Find(&proxyNodes).Error; err != nil {
		return nil, err
	}
	for i := range proxyNodes {
		add(model.LatencyTargetTypeNode, proxyNodes[i].ID, proxyNodes[i].Name, proxyNodes[i].Host, proxyNodes[i].Port)
	}

	targets := make([]probeTarget, 0, len(deduped))
	for _, t := range deduped {
		targets = append(targets, t)
	}
	sort.Slice(targets, func(i, j int) bool { return targets[i].Key < targets[j].Key })
	return targets, nil
}

// resolveNodeEndpoint returns the host:port to probe for a tunnel node, preferring the
// tunnel's configured IP and falling back to the ForwardNode record.
func (w *ForwardLatencyProber) resolveNodeEndpoint(db *gorm.DB, nodeID uint, fallbackIP string) (string, int, bool) {
	if nodeID == 0 {
		return "", 0, false
	}
	var node model.ForwardNode
	if err := db.First(&node, nodeID).Error; err != nil {
		return "", 0, false
	}
	host := strings.TrimSpace(fallbackIP)
	if host == "" {
		host = node.Host
	}
	if host == "" || node.Port <= 0 {
		return "", 0, false
	}
	return host, node.Port, true
}

// probeAll probes targets with bounded concurrency and returns one bucket per target.
func (w *ForwardLatencyProber) probeAll(ctx context.Context, targets []probeTarget, bucketAt time.Time) []model.ForwardLatencyBucket {
	concurrency := w.concurrency
	if concurrency <= 0 {
		concurrency = 1
	}

	sem := make(chan struct{}, concurrency)
	var (
		mu      sync.Mutex
		wg      sync.WaitGroup
		buckets = make([]model.ForwardLatencyBucket, 0, len(targets))
	)

	for i := range targets {
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(t probeTarget) {
			defer wg.Done()
			defer func() { <-sem }()

			bucket := w.probeOne(ctx, t, bucketAt)
			mu.Lock()
			buckets = append(buckets, bucket)
			mu.Unlock()
		}(targets[i])
	}

	wg.Wait()
	return buckets
}

// probeOne performs w.dials sequential TCP dials and aggregates into a single bucket row.
func (w *ForwardLatencyProber) probeOne(ctx context.Context, t probeTarget, bucketAt time.Time) model.ForwardLatencyBucket {
	addr := net.JoinHostPort(t.Host, strconv.Itoa(t.Port))
	attempts := w.dials
	if attempts <= 0 {
		attempts = defaultForwardLatencyDialsPerProbe
	}

	rtts := make([]float64, 0, attempts)
	for i := 0; i < attempts; i++ {
		if ctx.Err() != nil {
			break
		}
		rtt, err := dialAndMeasureTCP(ctx, addr, w.dialTimeout)
		if err != nil {
			w.errorLogger.Logf("probe:"+t.Key, "forward latency probe failed for %s: %v", addr, err)
			continue
		}
		rtts = append(rtts, rtt)
	}
	if len(rtts) > 0 {
		w.errorLogger.Clear("probe:" + t.Key)
	}

	agg := aggregateRTTs(rtts, attempts)
	return model.ForwardLatencyBucket{
		TargetKey:       t.Key,
		TargetType:      t.TargetType,
		TargetID:        t.TargetID,
		Label:           t.Label,
		Host:            t.Host,
		Port:            t.Port,
		BucketAt:        bucketAt,
		IntervalSeconds: int(w.interval / time.Second),
		SampleCount:     attempts,
		SuccessCount:    len(rtts),
		MinRTT:          agg.Min,
		AvgRTT:          agg.Avg,
		MaxRTT:          agg.Max,
		P95RTT:          agg.P95,
		LossPct:         agg.Loss,
	}
}

// cleanup deletes buckets older than the retention window, at most once per hour.
func (w *ForwardLatencyProber) cleanup(ctx context.Context) {
	if w.retention <= 0 {
		return
	}
	now := time.Now()
	if !w.lastCleanup.IsZero() && now.Sub(w.lastCleanup) < forwardLatencyCleanupInterval {
		return
	}
	w.lastCleanup = now
	cutoff := now.Add(-w.retention)
	if err := w.db.WithContext(ctx).
		Where("bucket_at < ?", cutoff).
		Delete(&model.ForwardLatencyBucket{}).Error; err != nil {
		w.errorLogger.Logf("cleanup", "forward latency bucket cleanup failed: %v", err)
		return
	}
	w.errorLogger.Clear("cleanup")
}

func (w *ForwardLatencyProber) queryDB() *gorm.DB {
	if w.db == nil {
		return nil
	}
	return w.db.Session(&gorm.Session{
		Logger: logger.Default.LogMode(logger.Silent),
	})
}

// latencyAggregate holds computed bucket statistics (ms).
type latencyAggregate struct {
	Min  float64
	Avg  float64
	Max  float64
	P95  float64
	Loss float64
}

// aggregateRTTs computes min/avg/max/p95 over successful RTTs and the loss percentage
// over the total attempts. P95 uses nearest-rank: index = ceil(0.95*n)-1.
func aggregateRTTs(rtts []float64, attempts int) latencyAggregate {
	agg := latencyAggregate{}
	if attempts <= 0 {
		return agg
	}
	if len(rtts) == 0 {
		agg.Loss = 100
		return agg
	}

	sorted := make([]float64, len(rtts))
	copy(sorted, rtts)
	sort.Float64s(sorted)

	sum := 0.0
	for _, v := range sorted {
		sum += v
	}
	n := len(sorted)
	agg.Min = sorted[0]
	agg.Max = sorted[n-1]
	agg.Avg = sum / float64(n)

	rank := int(math.Ceil(0.95*float64(n))) - 1
	if rank < 0 {
		rank = 0
	}
	if rank >= n {
		rank = n - 1
	}
	agg.P95 = sorted[rank]

	agg.Loss = float64(attempts-n) / float64(attempts) * 100
	return agg
}

// dialAndMeasureTCP performs a single TCP dial and returns the elapsed time in ms.
func dialAndMeasureTCP(ctx context.Context, addr string, timeout time.Duration) (float64, error) {
	d := net.Dialer{Timeout: timeout}
	start := time.Now()
	conn, err := d.DialContext(ctx, "tcp", addr)
	elapsed := time.Since(start)
	if err != nil {
		return 0, err
	}
	_ = conn.Close()
	return float64(elapsed.Milliseconds()), nil
}
