//go:build unix

package pluginhost

import (
	"context"
	"errors"
	"os/exec"
	"sort"
	"time"
)

// Watchdog defaults. A host that exits on its own is restarted at the same
// generation after 1s, 2s, 4s, ... (capped at 30s); after five restarts within
// five minutes it is marked failed until the next explicit lifecycle
// operation.
const (
	defaultRestartBaseDelay = time.Second
	defaultRestartMaxDelay  = 30 * time.Second
	defaultRestartWindow    = 5 * time.Minute
	defaultMaxRestarts      = 5
)

// Host supervision states reported by HostStats.State.
const (
	HostStateRunning    = "running"
	HostStateRestarting = "restarting"
	HostStateFailed     = "failed"
	HostStateExited     = "exited"
	HostStateStopped    = "stopped"
)

// HostStats is a read-only snapshot of one package's host supervision
// counters since the supervisor was created.
type HostStats struct {
	PackageID  string
	Version    string
	Generation uint64
	// State is one of the HostState* constants.
	State string
	// Starts counts host processes that became healthy, explicit and
	// watchdog starts alike.
	Starts uint64
	// UnexpectedExits counts host exits not caused by Drain, Stop,
	// replacement, or Shutdown.
	UnexpectedExits uint64
	// Restarts counts watchdog restart attempts, successful or not.
	Restarts uint64
	// Failures counts how often the restart budget was exhausted and the
	// host was marked failed.
	Failures uint64
	// HealthDetailsJSON is the details document of the host's last successful
	// Health call made by PollHealth, and HealthCheckedAt its time.
	HealthDetailsJSON string
	HealthCheckedAt   time.Time
}

// HostStatsProvider is implemented by supervisors that expose per-package
// host counters, for example to a metrics endpoint.
type HostStatsProvider interface {
	Stats() []HostStats
}

var _ HostStatsProvider = (*Supervisor)(nil)

// Stats returns a snapshot of every package's supervision counters, sorted
// by package ID. It never waits for a lifecycle operation.
func (m *Supervisor) Stats() []HostStats {
	if m == nil {
		return nil
	}
	m.statsMu.Lock()
	defer m.statsMu.Unlock()
	stats := make([]HostStats, 0, len(m.stats))
	for _, entry := range m.stats {
		stats = append(stats, *entry)
	}
	sort.Slice(stats, func(i, j int) bool { return stats[i].PackageID < stats[j].PackageID })
	return stats
}

// healthPollTimeout bounds one host's Health call during PollHealth.
const healthPollTimeout = 5 * time.Second

// PollHealth calls Health on every running host each interval until ctx ends
// and records the returned details document in the host's stats, so metrics
// and diagnostics can read route modes and shadow counters. A failed call
// leaves the previous details in place.
func (m *Supervisor) PollHealth(ctx context.Context, interval time.Duration) {
	if m == nil || interval <= 0 {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.pollHealthOnce(ctx)
		}
	}
}

func (m *Supervisor) pollHealthOnce(ctx context.Context) {
	m.mu.RLock()
	hosts := make([]*hostProcess, 0, len(m.hosts))
	for _, host := range m.hosts {
		if host != nil && host.supervision == hostSupervisionNone {
			hosts = append(hosts, host)
		}
	}
	m.mu.RUnlock()
	for _, host := range hosts {
		if ctx.Err() != nil {
			return
		}
		healthCtx, cancel := context.WithTimeout(ctx, healthPollTimeout)
		health, err := m.Health(healthCtx, host.packageID, host.version, host.generation)
		cancel()
		if err != nil {
			continue
		}
		checkedAt := time.Now()
		m.updateStats(host, func(stats *HostStats) {
			stats.HealthDetailsJSON = health.DetailsJSON
			stats.HealthCheckedAt = checkedAt
		})
	}
}

// hostSupervisionState describes a host whose process has exited while it is
// still the registered host for its package.
type hostSupervisionState uint8

const (
	hostSupervisionNone hostSupervisionState = iota
	hostSupervisionRestarting
	hostSupervisionFailed
	hostSupervisionExited
)

func (s hostSupervisionState) describe() string {
	switch s {
	case hostSupervisionRestarting:
		return "exited; watchdog restart pending"
	case hostSupervisionFailed:
		return "failed; restart budget exhausted, waiting for a lifecycle operation"
	default:
		return "exited"
	}
}

func (m *Supervisor) hostStartOptions() hostStartOptions {
	return hostStartOptions{
		startupTimeout: m.startupTimeout, bridgeFactory: m.bridgeFactory,
		stopGrace: m.stopGrace, logf: m.logger(), maxResponseBytes: m.maxResponse,
		packageEnvironment: m.packageEnvironment,
	}
}

func (m *Supervisor) logger() func(string, ...any) {
	if m.logf != nil {
		return m.logf
	}
	return func(string, ...any) {}
}

// installHostLocked registers a healthy host and starts watching its exit.
// The caller holds m.mu.
func (m *Supervisor) installHostLocked(key string, host *hostProcess) {
	m.hosts[key] = host
	m.updateStats(host, func(stats *HostStats) {
		stats.Starts++
		stats.State = HostStateRunning
	})
	go m.superviseHost(host)
}

func (m *Supervisor) recordStateLocked(host *hostProcess, state string) {
	m.updateStats(host, func(stats *HostStats) { stats.State = state })
}

func (m *Supervisor) updateStats(host *hostProcess, update func(*HostStats)) {
	if host == nil {
		return
	}
	m.statsMu.Lock()
	defer m.statsMu.Unlock()
	if m.stats == nil {
		m.stats = make(map[string]*HostStats)
	}
	key := hostKey(host.packageID, host.version)
	stats := m.stats[key]
	if stats == nil {
		stats = &HostStats{PackageID: host.packageID}
		m.stats[key] = stats
	}
	stats.Version = host.version
	stats.Generation = host.generation
	update(stats)
}

// cancelRecoveryLocked cancels a pending watchdog restart for host. The
// caller holds m.mu; a timer that already fired observes the change and
// returns without restarting.
func (m *Supervisor) cancelRecoveryLocked(host *hostProcess) {
	if host == nil {
		return
	}
	if host.recoveryTimer != nil {
		host.recoveryTimer.Stop()
		host.recoveryTimer = nil
	}
	host.recoveryToken = 0
}

func (m *Supervisor) superviseHost(host *hostProcess) {
	<-host.waitDone
	m.handleHostExit(host)
}

// handleHostExit classifies a host exit. An exit is unexpected only while the
// process is still the registered host for its package and no lifecycle
// operation has claimed it; only then is a restart scheduled.
func (m *Supervisor) handleHostExit(host *hostProcess) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := hostKey(host.packageID, host.version)
	if m.closed || m.hosts[key] != host || host.retiring.Load() {
		return
	}
	m.logger()("plugin host [pkg:%s v:%s gen:%d] exited unexpectedly: %s",
		host.packageID, host.version, host.generation, describeHostExit(host.waitErr))
	m.updateStats(host, func(stats *HostStats) { stats.UnexpectedExits++ })
	host.failWebSocketRelays(ErrHostUnavailable)
	m.scheduleRecoveryLocked(host)
}

// scheduleRecoveryLocked arms the next restart of the exited host, or marks
// it failed once the restart budget is spent. The caller holds m.mu.
func (m *Supervisor) scheduleRecoveryLocked(host *hostProcess) {
	now := time.Now()
	history := host.restartHistory[:0:0]
	for _, attempt := range host.restartHistory {
		if now.Sub(attempt) < m.restartWindow {
			history = append(history, attempt)
		}
	}
	host.restartHistory = history
	if len(history) >= m.maxRestarts {
		host.supervision = hostSupervisionFailed
		m.updateStats(host, func(stats *HostStats) {
			stats.Failures++
			stats.State = HostStateFailed
		})
		m.logger()("plugin host [pkg:%s v:%s gen:%d] failed: %d restarts within %s; not restarting until the next lifecycle operation",
			host.packageID, host.version, host.generation, len(history), m.restartWindow)
		// Reap orphaned descendants and release the runtime directory now.
		m.armRecoveryLocked(host, 0)
		return
	}
	delay := m.restartDelay(len(history))
	host.supervision = hostSupervisionRestarting
	m.recordStateLocked(host, HostStateRestarting)
	m.logger()("plugin host [pkg:%s v:%s gen:%d] restarting in %s (restart %d of %d within %s)",
		host.packageID, host.version, host.generation, delay, len(history)+1, m.maxRestarts, m.restartWindow)
	m.armRecoveryLocked(host, delay)
}

func (m *Supervisor) armRecoveryLocked(host *hostProcess, delay time.Duration) {
	m.recoverySeq++
	token := m.recoverySeq
	host.recoveryToken = token
	host.recoveryTimer = time.AfterFunc(delay, func() { m.recoverHost(host, token) })
}

func (m *Supervisor) restartDelay(previousRestarts int) time.Duration {
	delay := m.restartBaseDelay
	for i := 0; i < previousRestarts && delay < m.restartMaxDelay; i++ {
		delay *= 2
	}
	if delay > m.restartMaxDelay {
		delay = m.restartMaxDelay
	}
	return delay
}

// recoverHost runs when a recovery timer fires. It holds lifecycleMu so no
// explicit lifecycle operation interleaves, but boots the replacement without
// mu so dispatch to other packages continues. The exited host stays
// registered (failing requests fast) until the replacement is healthy.
func (m *Supervisor) recoverHost(exited *hostProcess, token uint64) {
	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()
	key := hostKey(exited.packageID, exited.version)
	m.mu.Lock()
	if m.closed || m.runtimeRoot == nil || m.hosts[key] != exited || exited.recoveryToken != token || exited.retiring.Load() {
		m.mu.Unlock()
		return
	}
	exited.recoveryTimer = nil
	exited.recoveryToken = 0
	failed := exited.supervision == hostSupervisionFailed
	if !failed {
		exited.restartHistory = append(exited.restartHistory, time.Now())
		m.updateStats(exited, func(stats *HostStats) { stats.Restarts++ })
	}
	runtimeRoot, options, ctx := m.runtimeRoot, m.hostStartOptions(), m.restartCtx
	m.mu.Unlock()
	if ctx == nil {
		ctx = context.Background()
	}

	// Kill descendants the crashed leader left behind and release its
	// runtime directory and bridge before booting a replacement. The leader
	// has exited, so this does not wait for a grace period.
	if err := exited.stop(context.Background()); err != nil {
		m.logger()("plugin host [pkg:%s v:%s gen:%d] cleanup after exit: %v", exited.packageID, exited.version, exited.generation, err)
	}
	if failed {
		return
	}

	replacement, err := startHostProcess(ctx, runtimeRoot, exited.ref, exited.generation, options)

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.hosts[key] != exited {
		if replacement != nil {
			_ = replacement.stop(context.Background())
		}
		return
	}
	if err != nil {
		m.logger()("plugin host [pkg:%s v:%s gen:%d] restart failed: %v", exited.packageID, exited.version, exited.generation, err)
		if errors.Is(ctx.Err(), context.Canceled) {
			return
		}
		m.scheduleRecoveryLocked(exited)
		return
	}
	replacement.restartHistory = exited.restartHistory
	m.installHostLocked(key, replacement)
	m.logger()("plugin host [pkg:%s v:%s gen:%d] restarted by watchdog", exited.packageID, exited.version, exited.generation)
}

func describeHostExit(err error) string {
	if err == nil {
		return "exit status 0"
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.Error()
	}
	return err.Error()
}
