package agente2e

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// process is one real binary of the suite (Control or the Agent): started,
// signalled, killed and restarted by the scenarios, with its output kept in
// a log file under the run's log directory (uploaded by CI on failure) and
// in memory for assertions.
type process struct {
	name    string
	binary  string
	args    []string
	env     []string
	dir     string
	logPath string

	mu      sync.Mutex
	cmd     *exec.Cmd
	done    chan struct{}
	exitErr error
	output  lockedBuffer
	starts  int
}

func newProcess(name, binary, dir, logPath string, env []string, args ...string) *process {
	return &process{name: name, binary: binary, args: args, env: env, dir: dir, logPath: logPath}
}

func (p *process) Start(t *testing.T) {
	t.Helper()
	p.mu.Lock()
	defer p.mu.Unlock()
	require.Nil(t, p.cmd, "%s is already running", p.name)
	logFile, err := os.OpenFile(p.logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	require.NoError(t, err)
	p.starts++
	_, _ = fmt.Fprintf(logFile, "\n===== %s start #%d at %s =====\n", p.name, p.starts, time.Now().UTC().Format(time.RFC3339Nano))
	cmd := exec.Command(p.binary, p.args...)
	cmd.Dir = p.dir
	cmd.Env = p.env
	writer := io.MultiWriter(logFile, &p.output)
	cmd.Stdout = writer
	cmd.Stderr = writer
	// Its own process group, so a kill reaches the plugin children too.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	require.NoError(t, cmd.Start(), "start %s", p.name)
	done := make(chan struct{})
	p.cmd, p.done = cmd, done
	go func() {
		err := cmd.Wait()
		_ = logFile.Close()
		p.mu.Lock()
		p.exitErr = err
		p.mu.Unlock()
		close(done)
	}()
}

// Running tells whether the process has not exited.
func (p *process) Running() bool {
	p.mu.Lock()
	done := p.done
	p.mu.Unlock()
	if done == nil {
		return false
	}
	select {
	case <-done:
		return false
	default:
		return true
	}
}

// Stop sends SIGTERM and waits; SIGKILL after the grace period.
func (p *process) Stop(t *testing.T, grace time.Duration) {
	t.Helper()
	p.signalAndWait(t, syscall.SIGTERM, grace)
}

// Kill sends SIGKILL to the process group at once (a crash).
func (p *process) Kill(t *testing.T) {
	t.Helper()
	p.signalAndWait(t, syscall.SIGKILL, 10*time.Second)
}

func (p *process) signalAndWait(t *testing.T, signal syscall.Signal, grace time.Duration) {
	t.Helper()
	p.mu.Lock()
	cmd, done := p.cmd, p.done
	p.mu.Unlock()
	if cmd == nil {
		return
	}
	if signal == syscall.SIGKILL {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	} else {
		_ = cmd.Process.Signal(signal)
	}
	select {
	case <-done:
	case <-time.After(grace):
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-done
	}
	// Plugin children left in the group after a graceful stop.
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	p.mu.Lock()
	p.cmd, p.done = nil, nil
	p.mu.Unlock()
}

// Output is everything the process printed, over every start.
func (p *process) Output() string { return p.output.String() }

// OutputSince is what the process printed after the mark (Mark).
func (p *process) OutputSince(mark int) string {
	out := p.output.String()
	if mark > len(out) {
		return ""
	}
	return out[mark:]
}

// Mark is the current length of the output, for OutputSince.
func (p *process) Mark() int { return p.output.Len() }

// Tail is the end of the output, for failure messages.
func (p *process) Tail() string {
	var kept []string
	for _, line := range strings.Split(p.output.String(), "\n") {
		if !strings.Contains(line, "[GIN]") {
			kept = append(kept, line)
		}
	}
	out := strings.Join(kept, "\n")
	const max = 6000
	if len(out) > max {
		out = out[len(out)-max:]
	}
	return fmt.Sprintf("--- %s output (tail) ---\n%s", p.name, out)
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func (b *lockedBuffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Len()
}

// eventually polls cond until it holds or the timeout passes; on timeout
// it fails with the message and the given processes' output.
func eventually(t *testing.T, timeout time.Duration, cond func() (bool, string), procs ...*process) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	last := ""
	for {
		ok, detail := cond()
		if ok {
			return
		}
		last = detail
		if time.Now().After(deadline) {
			var tails []string
			for _, p := range procs {
				if p != nil {
					tails = append(tails, p.Tail())
				}
			}
			t.Fatalf("condition not met within %s: %s\n%s", timeout, last, strings.Join(tails, "\n"))
		}
		time.Sleep(250 * time.Millisecond)
	}
}

// goBuild builds a main package into out.
func goBuild(t *testing.T, goBinary, dir, out string, env []string, args ...string) {
	t.Helper()
	started := time.Now()
	full := append([]string{"build", "-o", out}, args...)
	cmd := exec.Command(goBinary, full...)
	cmd.Dir = dir
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "go %s in %s: %s", strings.Join(full, " "), dir, output)
	t.Logf("built %s in %s", filepath.Base(out), time.Since(started).Round(time.Millisecond))
}
