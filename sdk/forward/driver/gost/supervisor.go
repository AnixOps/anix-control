package gost

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/AnixOps/anix-control/sdk/forward/driver"
)

// Supervisor runs the gost process that serves the driver's configuration
// file: SystemdSupervisor (the anixops-gost unit, H20) on a node,
// ProcessSupervisor where a test or container supervises gost itself.
// Every method is safe for concurrent use.
type Supervisor interface {
	// Check answers nil when the supervisor can run gost here, an error
	// wrapping driver.ErrNotOwned when the unit of the driver's name is
	// someone else's, and ErrNoUnit when it is missing.
	Check(ctx context.Context) error
	// Status answers whether gost runs, and the identity of the running
	// instance, which changes every time gost (re)starts.
	Status(ctx context.Context) (Status, error)
	// Start starts gost when it is not running. It returns once the
	// process is started, not once it serves.
	Start(ctx context.Context) error
	// Reload makes the running gost read its configuration file again
	// (SIGHUP): established TCP connections survive; UDP sessions and mux
	// carriers of a re-created service may not.
	Reload(ctx context.Context) error
	// Stop stops gost; stopping a stopped gost succeeds.
	Stop(ctx context.Context) error
}

// ErrNoUnit is Check's error when the unit is not installed.
var ErrNoUnit = errors.New("gost driver: the gost unit is not installed")

// Status is what a Supervisor knows about gost.
type Status struct {
	Running  bool
	Instance string
	// Starting is set while the supervisor (re)starts gost (systemd's
	// "activating", an automatic restart after a crash included): it is
	// not serving yet, but the sockets of its configuration are its own.
	Starting bool
}

// SystemdSupervisor manages UnitName (or Unit) with systemctl through a
// Runner. The unit is installed by the AnixOps installer from UnitFile;
// the Agent may start, reload and stop it (a polkit rule the installer
// adds), nothing else. Its Description carries OwnerMark.
type SystemdSupervisor struct {
	Runner Runner
	// Unit defaults to UnitName.
	Unit string
}

func (s SystemdSupervisor) unit() string {
	if s.Unit != "" {
		return s.Unit
	}
	return UnitName
}

func (s SystemdSupervisor) show(ctx context.Context, props ...string) (map[string]string, error) {
	out, err := s.Runner.Run(ctx, "systemctl", []string{"show", "--property=" + strings.Join(props, ","), s.unit()}, nil)
	if err != nil {
		if cerr := ctx.Err(); cerr != nil {
			return nil, cerr
		}
		return nil, fmt.Errorf("gost driver: %w", err)
	}
	m := map[string]string{}
	for line := range strings.Lines(string(out)) {
		if k, v, ok := strings.Cut(strings.TrimSpace(line), "="); ok {
			m[k] = v
		}
	}
	return m, nil
}

// Check implements Supervisor: the unit must be loaded and carry
// OwnerMark in its Description.
func (s SystemdSupervisor) Check(ctx context.Context) error {
	m, err := s.show(ctx, "LoadState", "Description")
	if err != nil {
		return err
	}
	if m["LoadState"] != "loaded" {
		return fmt.Errorf("%w: %s has LoadState %q", ErrNoUnit, s.unit(), m["LoadState"])
	}
	if !strings.Contains(m["Description"], OwnerMark) {
		return fmt.Errorf("%w: %s does not carry %q in its Description", driver.ErrNotOwned, s.unit(), OwnerMark)
	}
	return nil
}

// Status implements Supervisor: the instance is the unit's InvocationID.
func (s SystemdSupervisor) Status(ctx context.Context) (Status, error) {
	m, err := s.show(ctx, "ActiveState", "InvocationID")
	if err != nil {
		return Status{}, err
	}
	switch m["ActiveState"] {
	case "active", "reloading":
		return Status{Running: true, Instance: m["InvocationID"]}, nil
	case "activating":
		return Status{Starting: true}, nil
	}
	return Status{}, nil
}

func (s SystemdSupervisor) ctl(ctx context.Context, verb string) error {
	if _, err := s.Runner.Run(ctx, "systemctl", []string{verb, s.unit()}, nil); err != nil {
		if cerr := ctx.Err(); cerr != nil {
			return cerr
		}
		return fmt.Errorf("gost driver: %w", err)
	}
	return nil
}

// Start implements Supervisor.
func (s SystemdSupervisor) Start(ctx context.Context) error { return s.ctl(ctx, "start") }

// Reload implements Supervisor (the unit's ExecReload sends SIGHUP).
func (s SystemdSupervisor) Reload(ctx context.Context) error { return s.ctl(ctx, "reload") }

// Stop implements Supervisor.
func (s SystemdSupervisor) Stop(ctx context.Context) error { return s.ctl(ctx, "stop") }

// ProcessSupervisor runs gost as a child process: `Command... -C Config`.
// Tests run it in a network namespace (Command starting with "ip netns
// exec <ns>"), which needs no systemd. It must outlive the driver
// instances that use it (an Agent restart is a new driver on the same
// supervisor), so it suits tests and containers whose supervisor is the
// Agent's own process; nodes use SystemdSupervisor.
type ProcessSupervisor struct {
	// Command is the gost binary, after an optional prefix that runs it
	// elsewhere (ip netns exec <ns>).
	Command []string
	// Config is the configuration file (Config.Dir/gost.json).
	Config string
	// Log receives gost's standard output and error; nil discards them.
	Log io.Writer

	mu       sync.Mutex
	cmd      *exec.Cmd
	done     chan struct{}
	starts   int
	instance string
}

var _ Supervisor = (*ProcessSupervisor)(nil)

// Check implements Supervisor.
func (p *ProcessSupervisor) Check(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(p.Command) == 0 || p.Config == "" {
		return errors.New("gost driver: ProcessSupervisor needs Command and Config")
	}
	return nil
}

func (p *ProcessSupervisor) running() bool {
	if p.cmd == nil {
		return false
	}
	select {
	case <-p.done:
		return false
	default:
		return true
	}
}

// Status implements Supervisor: the instance is the start count and pid.
func (p *ProcessSupervisor) Status(ctx context.Context) (Status, error) {
	if err := ctx.Err(); err != nil {
		return Status{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.running() {
		return Status{}, nil
	}
	return Status{Running: true, Instance: p.instance}, nil
}

// Start implements Supervisor.
func (p *ProcessSupervisor) Start(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.running() {
		return nil
	}
	args := append(append([]string{}, p.Command[1:]...), "-C", p.Config)
	cmd := exec.Command(p.Command[0], args...) // #nosec G204 -- the supervisor's configured gost command
	out := p.Log
	if out == nil {
		out = io.Discard
	}
	cmd.Stdout, cmd.Stderr = out, out
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("gost driver: start gost: %w", err)
	}
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	p.cmd, p.done = cmd, done
	p.starts++
	p.instance = "p" + strconv.Itoa(p.starts) + "-" + strconv.Itoa(cmd.Process.Pid)
	return nil
}

// Reload implements Supervisor.
func (p *ProcessSupervisor) Reload(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.running() {
		return errors.New("gost driver: reload: gost is not running")
	}
	if err := p.cmd.Process.Signal(syscall.SIGHUP); err != nil {
		return fmt.Errorf("gost driver: reload: %w", err)
	}
	return nil
}

// Stop implements Supervisor: SIGTERM, then SIGKILL after five seconds.
func (p *ProcessSupervisor) Stop(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.running() {
		return nil
	}
	_ = p.cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-p.done:
	case <-time.After(5 * time.Second):
		_ = p.cmd.Process.Kill()
		<-p.done
	}
	return nil
}

// Kill kills gost without the driver knowing, as a crash would (tests).
func (p *ProcessSupervisor) Kill() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.running() {
		_ = p.cmd.Process.Kill()
		<-p.done
	}
}

// writeFileAtomic writes data to path through a temporary file in the same
// directory and a rename, so a reader sees the old or the new file.
func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(dirOf(path), ".tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer func() { _ = os.Remove(name) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

func dirOf(path string) string {
	if i := strings.LastIndexByte(path, '/'); i > 0 {
		return path[:i]
	}
	return "/"
}
