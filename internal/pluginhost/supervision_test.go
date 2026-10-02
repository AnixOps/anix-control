//go:build unix

package pluginhost

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const supervisionTestTimeout = 10 * time.Second

func TestWatchdogRestartsKilledHostAtSameGeneration(t *testing.T) {
	manager, logs := newSupervisedTestManager(t)
	ref := writeHostArtifactRef(t, "knowledge", "4.0.0")
	require.NoError(t, manager.Start(context.Background(), ref, 7))
	first := currentTestHost(manager, ref.PackageID)
	require.NotNil(t, first)

	killTestHostGroup(t, first)
	second := waitForReplacementHost(t, manager, ref.PackageID, first)

	require.EqualValues(t, 7, second.generation)
	require.Equal(t, ref.Version, second.version)
	require.True(t, sameArtifactRelease(ref, second.ref))
	require.NotEqual(t, first.command.Process.Pid, second.command.Process.Pid)
	require.NoError(t, dispatchTestRequest(manager, ref, 7))
	require.Nil(t, first.runtimeDir, "the crashed host's runtime directory is released")

	stats := hostStatsFor(t, manager, ref.PackageID)
	require.Equal(t, HostStats{
		PackageID: "knowledge", Version: "4.0.0", Generation: 7, State: HostStateRunning,
		Starts: 2, UnexpectedExits: 1, Restarts: 1,
	}, stats)
	require.True(t, logs.contains("plugin host [pkg:knowledge v:4.0.0 gen:7] exited unexpectedly: signal: killed"), logs.String())
	require.True(t, logs.contains("plugin host [pkg:knowledge v:4.0.0 gen:7] restarted by watchdog"), logs.String())
}

func TestWatchdogRestartKeepsConfiguredResponseLimit(t *testing.T) {
	manager, _ := newSupervisedTestManager(t)
	manager.maxResponse = 3 << 20
	ref := writeHostArtifactRef(t, "knowledge", "4.0.0")
	require.NoError(t, manager.Start(context.Background(), ref, 7))
	first := currentTestHost(manager, ref.PackageID)
	require.NotNil(t, first)
	const want = hostMaxResponseBytesEnvironment + "=3145728"
	require.Contains(t, first.command.Env, want)

	killTestHostGroup(t, first)
	second := waitForReplacementHost(t, manager, ref.PackageID, first)
	require.Contains(t, second.command.Env, want, "a watchdog restart reuses the configured limit")
	require.EqualValues(t, 3<<20, second.maxResponseBytes)
}

func TestWatchdogMarksHostFailedAfterRestartBudget(t *testing.T) {
	manager, logs := newSupervisedTestManager(t)
	manager.maxRestarts = 2
	ref := writeHostArtifactRef(t, "knowledge", "4.0.0")
	require.NoError(t, manager.Start(context.Background(), ref, 7))
	host := currentTestHost(manager, ref.PackageID)

	for range manager.maxRestarts {
		killTestHostGroup(t, host)
		host = waitForReplacementHost(t, manager, ref.PackageID, host)
	}
	killTestHostGroup(t, host)
	require.Eventually(t, func() bool {
		return hostStatsFor(t, manager, ref.PackageID).State == HostStateFailed
	}, supervisionTestTimeout, 5*time.Millisecond)

	// Longer than any backoff delay: no further restart may happen.
	time.Sleep(10 * manager.restartMaxDelay)
	require.Same(t, host, currentTestHost(manager, ref.PackageID))
	stats := hostStatsFor(t, manager, ref.PackageID)
	require.EqualValues(t, 2, stats.Restarts)
	require.EqualValues(t, 3, stats.UnexpectedExits)
	require.EqualValues(t, 1, stats.Failures)
	err := dispatchTestRequest(manager, ref, 7)
	require.ErrorIs(t, err, ErrHostUnavailable)
	require.ErrorContains(t, err, "restart budget exhausted")
	require.True(t, logs.contains("plugin host [pkg:knowledge v:4.0.0 gen:7] failed: 2 restarts within"), logs.String())

	// An explicit lifecycle Start revives the host and resets the budget.
	require.NoError(t, manager.Start(context.Background(), ref, 7))
	revived := currentTestHost(manager, ref.PackageID)
	require.NotSame(t, host, revived)
	require.NoError(t, dispatchTestRequest(manager, ref, 7))
	require.Equal(t, HostStateRunning, hostStatsFor(t, manager, ref.PackageID).State)

	killTestHostGroup(t, revived)
	waitForReplacementHost(t, manager, ref.PackageID, revived)
	require.NoError(t, dispatchTestRequest(manager, ref, 7))
	require.EqualValues(t, 3, hostStatsFor(t, manager, ref.PackageID).Restarts)
}

func TestDispatchToExitedHostFailsFast(t *testing.T) {
	manager, _ := newSupervisedTestManager(t)
	manager.restartBaseDelay = time.Hour
	ref := writeHostArtifactRef(t, "knowledge", "4.0.0")
	require.NoError(t, manager.Start(context.Background(), ref, 7))
	host := currentTestHost(manager, ref.PackageID)

	killTestHostGroup(t, host)
	require.Eventually(t, func() bool {
		return hostStatsFor(t, manager, ref.PackageID).State == HostStateRestarting
	}, supervisionTestTimeout, 5*time.Millisecond)

	started := time.Now()
	err := dispatchTestRequest(manager, ref, 7)
	require.Less(t, time.Since(started), time.Second)
	require.ErrorIs(t, err, ErrHostUnavailable)
	require.ErrorContains(t, err, "watchdog restart pending")

	_, err = manager.Health(context.Background(), ref.PackageID, ref.Version, 7)
	require.ErrorIs(t, err, ErrHostUnavailable)
	_, err = manager.OpenWebSocket(context.Background(), WebSocketInput{
		PackageID: ref.PackageID, Version: ref.Version, Generation: 7, RouteID: "knowledge.ws",
		RequestID: "ws-1", PrincipalJSON: []byte(`{}`), Deadline: time.Now().Add(time.Second),
	})
	require.ErrorIs(t, err, ErrHostUnavailable)
	// A stale generation still reports the generation mismatch first.
	require.ErrorIs(t, dispatchTestRequest(manager, ref, 6), ErrGenerationUnavailable)
}

func TestHostStderrAndLocaleReachKernelLog(t *testing.T) {
	t.Setenv("TZ", "Europe/Berlin")
	t.Setenv("LANG", "C.UTF-8")
	manager, logs := newSupervisedTestManager(t)
	ref := writeHostArtifactRef(t, "knowledge", "4.0.0")

	require.NoError(t, manager.Start(context.Background(), ref, 7))

	want := "[pkg:knowledge v:4.0.0 gen:7 stderr] " + hostTestStartedLine + " TZ=Europe/Berlin LANG=C.UTF-8 KEK=false"
	require.Eventually(t, func() bool { return logs.contains(want) }, supervisionTestTimeout, 5*time.Millisecond, logs.String())
}

func TestPackageEnvironmentReachesOnlyItsPackage(t *testing.T) {
	manager, logs := newSupervisedTestManager(t)
	manager.packageEnvironment = map[string][]string{"knowledge": {"ANIX_IDENTITY_KEK=test-kek"}}

	require.NoError(t, manager.Start(context.Background(), writeHostArtifactRef(t, "knowledge", "4.0.0"), 7))
	require.NoError(t, manager.Start(context.Background(), writeHostArtifactRef(t, "ticket", "4.0.0"), 3))

	require.Eventually(t, func() bool {
		return logs.contains("[pkg:knowledge v:4.0.0 gen:7 stderr] "+hostTestStartedLine) &&
			logs.contains("[pkg:ticket v:4.0.0 gen:3 stderr] "+hostTestStartedLine)
	}, supervisionTestTimeout, 5*time.Millisecond, logs.String())
	require.True(t, logs.contains("[pkg:knowledge v:4.0.0 gen:7 stderr] "+hostTestStartedLine+" TZ="), logs.String())
	for _, line := range strings.Split(logs.String(), "\n") {
		if strings.Contains(line, hostTestStartedLine) {
			require.Equal(t, strings.HasPrefix(line, "[pkg:knowledge"), strings.HasSuffix(line, "KEK=true"), line)
		}
	}
}

func TestHostEnvironmentPassesOnlySetLocaleVariables(t *testing.T) {
	t.Setenv("TZ", "Asia/Shanghai")
	t.Setenv("LANG", "")
	t.Setenv("LC_ALL", "C")
	t.Setenv(hostTestForbiddenSecretEnvironment, "server-secret")

	environment := hostEnvironment("/sock", "/dir", true)

	require.Contains(t, environment, "TZ=Asia/Shanghai")
	for _, entry := range environment {
		require.False(t, strings.HasPrefix(entry, "LANG="), entry)
		require.False(t, strings.HasPrefix(entry, "LC_ALL="), entry)
		require.False(t, strings.HasPrefix(entry, hostTestForbiddenSecretEnvironment+"="), entry)
	}
	require.Len(t, environment, 6)
}

func TestManagerStopSendsSIGTERMBeforeKill(t *testing.T) {
	manager, logs := newSupervisedTestManager(t)
	ref := writeHostArtifactRef(t, "knowledge", "4.0.0")
	require.NoError(t, manager.Start(context.Background(), ref, 7))
	host := currentTestHost(manager, ref.PackageID)

	require.NoError(t, manager.Stop(context.Background(), ref.PackageID, ref.Version, 8))

	require.True(t, hostExited(host))
	require.NoError(t, host.waitErr, "the host handled SIGTERM and exited cleanly")
	require.Eventually(t, func() bool {
		return logs.contains("[pkg:knowledge v:4.0.0 gen:7 stderr] " + hostTestTerminatedLine)
	}, supervisionTestTimeout, 5*time.Millisecond, logs.String())
	require.False(t, logs.contains("exited unexpectedly"), logs.String())
	require.Equal(t, HostStateStopped, hostStatsFor(t, manager, ref.PackageID).State)
}

func TestManagerStopKillsHostIgnoringSIGTERMAfterGrace(t *testing.T) {
	manager, _ := newSupervisedTestManager(t)
	manager.stopGrace = 200 * time.Millisecond
	ref := writeHostArtifactRefWithPrelude(t, "knowledge", "4.0.0", "export "+hostTestIgnoreTermEnvironment+"=1\n")
	require.NoError(t, manager.Start(context.Background(), ref, 7))
	host := currentTestHost(manager, ref.PackageID)

	started := time.Now()
	require.NoError(t, manager.Stop(context.Background(), ref.PackageID, ref.Version, 8))

	require.GreaterOrEqual(t, time.Since(started), manager.stopGrace)
	var exitErr *exec.ExitError
	require.True(t, errors.As(host.waitErr, &exitErr), "%v", host.waitErr)
	status, ok := exitErr.Sys().(syscall.WaitStatus)
	require.True(t, ok)
	require.True(t, status.Signaled())
	require.Equal(t, syscall.SIGKILL, status.Signal())
}

func TestShutdownCancelsPendingRestart(t *testing.T) {
	manager, _ := newSupervisedTestManager(t)
	manager.restartBaseDelay = 150 * time.Millisecond
	ref := writeHostArtifactRef(t, "knowledge", "4.0.0")
	require.NoError(t, manager.Start(context.Background(), ref, 7))
	host := currentTestHost(manager, ref.PackageID)

	killTestHostGroup(t, host)
	waitForTestHostState(t, manager, ref.PackageID, HostStateRestarting)
	require.NoError(t, manager.Shutdown(context.Background()))

	time.Sleep(3 * manager.restartBaseDelay)
	stats := hostStatsFor(t, manager, ref.PackageID)
	require.Zero(t, stats.Restarts)
	require.EqualValues(t, 1, stats.Starts)
	require.Equal(t, HostStateStopped, stats.State)
	require.Nil(t, currentTestHost(manager, ref.PackageID))
}

func TestLifecycleOperationsCancelPendingRestart(t *testing.T) {
	tests := []struct {
		name    string
		operate func(t *testing.T, manager *Supervisor, ref ArtifactRef)
		check   func(t *testing.T, manager *Supervisor, ref ArtifactRef)
	}{
		{
			name: "stop",
			operate: func(t *testing.T, manager *Supervisor, ref ArtifactRef) {
				require.NoError(t, manager.Stop(context.Background(), ref.PackageID, ref.Version, 8))
			},
			check: func(t *testing.T, manager *Supervisor, ref ArtifactRef) {
				require.Nil(t, currentTestHost(manager, ref.PackageID))
			},
		},
		{
			name: "drain",
			operate: func(t *testing.T, manager *Supervisor, ref ArtifactRef) {
				require.NoError(t, manager.Drain(context.Background(), ref.PackageID, ref.Version, 8, time.Now().Add(time.Second)))
			},
			check: func(t *testing.T, manager *Supervisor, ref ArtifactRef) {
				require.Equal(t, HostStateExited, hostStatsFor(t, manager, ref.PackageID).State)
				require.NoError(t, manager.Stop(context.Background(), ref.PackageID, ref.Version, 8))
				require.Nil(t, currentTestHost(manager, ref.PackageID))
			},
		},
		{
			name: "newer generation",
			operate: func(t *testing.T, manager *Supervisor, _ ArtifactRef) {
				replacement := writeHostArtifactRef(t, "knowledge", "4.1.0")
				require.NoError(t, manager.Start(context.Background(), replacement, 8))
			},
			check: func(t *testing.T, manager *Supervisor, ref ArtifactRef) {
				host := currentTestHost(manager, ref.PackageID)
				require.NotNil(t, host)
				require.EqualValues(t, 8, host.generation)
				require.False(t, hostExited(host))
				require.EqualValues(t, 2, hostStatsFor(t, manager, ref.PackageID).Starts)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manager, _ := newSupervisedTestManager(t)
			manager.restartBaseDelay = 150 * time.Millisecond
			ref := writeHostArtifactRef(t, "knowledge", "4.0.0")
			require.NoError(t, manager.Start(context.Background(), ref, 7))
			host := currentTestHost(manager, ref.PackageID)

			killTestHostGroup(t, host)
			waitForTestHostState(t, manager, ref.PackageID, HostStateRestarting)
			test.operate(t, manager, ref)

			time.Sleep(3 * manager.restartBaseDelay)
			require.Zero(t, hostStatsFor(t, manager, ref.PackageID).Restarts)
			test.check(t, manager, ref)
		})
	}
}

func TestWatchdogIgnoresExpectedExits(t *testing.T) {
	manager, logs := newSupervisedTestManager(t)
	ref := writeHostArtifactRef(t, "knowledge", "4.0.0")
	require.NoError(t, manager.Start(context.Background(), ref, 7))
	replacement := writeHostArtifactRef(t, "knowledge", "4.1.0")
	require.NoError(t, manager.Start(context.Background(), replacement, 8))
	require.NoError(t, manager.Stop(context.Background(), replacement.PackageID, replacement.Version, 9))

	time.Sleep(3 * manager.restartMaxDelay)
	stats := hostStatsFor(t, manager, ref.PackageID)
	require.Zero(t, stats.UnexpectedExits)
	require.Zero(t, stats.Restarts)
	require.False(t, logs.contains("exited unexpectedly"), logs.String())
}

func TestRestartDelayBacksOffExponentiallyWithCap(t *testing.T) {
	manager := &Supervisor{restartBaseDelay: time.Second, restartMaxDelay: 30 * time.Second}
	var delays []time.Duration
	for previous := range 7 {
		delays = append(delays, manager.restartDelay(previous))
	}
	require.Equal(t, []time.Duration{
		time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 30 * time.Second, 30 * time.Second,
	}, delays)
}

func TestScanHostOutputLinesBoundsLongLines(t *testing.T) {
	long := strings.Repeat("x", 3*maxHostOutputLineBytes+17)
	input := "first\r\n" + long + "\nsecond\x1b[31m\tred\n" + strings.Repeat("y", maxHostOutputLineBytes) + "\nlast"
	var lines []string

	scanHostOutputLines(strings.NewReader(input), func(line string) { lines = append(lines, line) })

	require.Len(t, lines, 5)
	require.Equal(t, "first", lines[0])
	require.Equal(t, strings.Repeat("x", maxHostOutputLineBytes)+hostOutputTruncatedMarker, lines[1])
	require.Equal(t, "second\uFFFD[31m\tred", lines[2])
	require.Equal(t, strings.Repeat("y", maxHostOutputLineBytes)+hostOutputTruncatedMarker, lines[3])
	require.Equal(t, "last", lines[4])
}

type testLogCapture struct {
	mu      sync.Mutex
	builder strings.Builder
}

func (c *testLogCapture) logf(format string, args ...any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, _ = fmt.Fprintf(&c.builder, format, args...)
	c.builder.WriteByte('\n')
}

func (c *testLogCapture) contains(value string) bool {
	return strings.Contains(c.String(), value)
}

func (c *testLogCapture) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.builder.String()
}

func newSupervisedTestManager(t *testing.T) (*Supervisor, *testLogCapture) {
	t.Helper()
	t.Setenv(hostTestChildEnvironment, "1")
	manager, err := NewManager(ManagerConfig{RuntimeDir: filepath.Join(shortHostTempDir(t), "runtime")})
	require.NoError(t, err)
	logs := &testLogCapture{}
	manager.logf = logs.logf
	manager.restartBaseDelay = 10 * time.Millisecond
	manager.restartMaxDelay = 40 * time.Millisecond
	manager.restartWindow = time.Minute
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })
	return manager, logs
}

func currentTestHost(manager *Supervisor, packageID string) *hostProcess {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	return manager.hosts[hostKey(packageID, "")]
}

func killTestHostGroup(t *testing.T, host *hostProcess) {
	t.Helper()
	require.NoError(t, syscall.Kill(-host.command.Process.Pid, syscall.SIGKILL))
}

func waitForReplacementHost(t *testing.T, manager *Supervisor, packageID string, previous *hostProcess) *hostProcess {
	t.Helper()
	var replacement *hostProcess
	require.Eventually(t, func() bool {
		replacement = currentTestHost(manager, packageID)
		return replacement != nil && replacement != previous && !hostExited(replacement)
	}, supervisionTestTimeout, 5*time.Millisecond)
	return replacement
}

func waitForTestHostState(t *testing.T, manager *Supervisor, packageID, state string) {
	t.Helper()
	require.Eventually(t, func() bool {
		return hostStatsFor(t, manager, packageID).State == state
	}, supervisionTestTimeout, 5*time.Millisecond)
}

func hostStatsFor(t *testing.T, manager *Supervisor, packageID string) HostStats {
	t.Helper()
	for _, stats := range manager.Stats() {
		if stats.PackageID == packageID {
			return stats
		}
	}
	return HostStats{}
}

func dispatchTestRequest(manager *Supervisor, ref ArtifactRef, generation uint64) error {
	response, err := manager.Dispatch(context.Background(), DispatchInput{
		PackageID: ref.PackageID, Version: ref.Version, Generation: generation,
		RequestID: "request-supervision", RouteID: "knowledge.article.list", Method: "GET",
		PrincipalJSON: []byte(`{"actor_id":7}`), Deadline: time.Now().Add(time.Second),
	})
	if err != nil {
		return err
	}
	if response.StatusCode != 202 {
		return fmt.Errorf("unexpected status %d", response.StatusCode)
	}
	return nil
}
