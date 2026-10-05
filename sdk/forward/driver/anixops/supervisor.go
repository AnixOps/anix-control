package anixops

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/AnixOps/anix-control/sdk/forward/driver"
)

// Supervisor runs the relay process that serves the driver's configuration
// file: SystemdSupervisor (the anixops-relay unit, decision P1) on a node,
// ProcessSupervisor where a test or container supervises the relay itself.
// Every method is safe for concurrent use.
type Supervisor interface {
	// Check answers nil when the supervisor can run the relay here, an error
	// wrapping driver.ErrNotOwned when the unit of the driver's name is
	// someone else's, and ErrNoUnit when it is missing.
	Check(ctx context.Context) error
	// Status answers whether the relay runs.
	Status(ctx context.Context) (Status, error)
	// Start starts the relay when it is not running. It returns once the
	// process is started, not once it answers.
	Start(ctx context.Context) error
	// Stop stops the relay (SIGTERM: every carrier gets a GOAWAY); stopping a
	// stopped relay succeeds.
	Stop(ctx context.Context) error
}

// ErrNoUnit is Check's error when the unit is not installed.
var ErrNoUnit = errors.New("anixops driver: the anixops-relay unit is not installed")

// Status is what a Supervisor knows about the relay.
type Status struct {
	Running bool
	// Starting is set while the supervisor (re)starts the relay (systemd's
	// "activating", an automatic restart after a crash included).
	Starting bool
}

// SystemdSupervisor manages UnitName (or Unit) with systemctl through a
// Runner. The unit is installed by the AnixOps installer from UnitFile; the
// Agent may start and stop it (a polkit rule the installer adds), nothing
// else. Its Description carries OwnerMark.
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
	out, err := s.Runner.Run(ctx, CmdSystemctl, []string{"show", "--property=" + strings.Join(props, ","), s.unit()}, nil)
	if err != nil {
		if cerr := ctx.Err(); cerr != nil {
			return nil, cerr
		}
		return nil, fmt.Errorf("anixops driver: %w", err)
	}
	m := map[string]string{}
	for line := range strings.Lines(string(out)) {
		if k, v, ok := strings.Cut(strings.TrimSpace(line), "="); ok {
			m[k] = v
		}
	}
	return m, nil
}

// Check implements Supervisor: the unit must be loaded and carry OwnerMark in
// its Description.
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

// Status implements Supervisor.
func (s SystemdSupervisor) Status(ctx context.Context) (Status, error) {
	m, err := s.show(ctx, "ActiveState")
	if err != nil {
		return Status{}, err
	}
	switch m["ActiveState"] {
	case "active", "reloading":
		return Status{Running: true}, nil
	case "activating":
		return Status{Starting: true}, nil
	}
	return Status{}, nil
}

func (s SystemdSupervisor) ctl(ctx context.Context, verb string) error {
	if _, err := s.Runner.Run(ctx, CmdSystemctl, []string{verb, s.unit()}, nil); err != nil {
		if cerr := ctx.Err(); cerr != nil {
			return cerr
		}
		return fmt.Errorf("anixops driver: %w", err)
	}
	return nil
}

// Start implements Supervisor.
func (s SystemdSupervisor) Start(ctx context.Context) error { return s.ctl(ctx, "start") }

// Stop implements Supervisor.
func (s SystemdSupervisor) Stop(ctx context.Context) error { return s.ctl(ctx, "stop") }

// ProcessSupervisor runs the relay as a child process: `Command... -config
// Config -socket Socket`. Tests run it in a network namespace (Command
// starting with "ip netns exec <ns>"), which needs no systemd. It must outlive
// the driver instances that use it (an Agent restart is a new driver on the
// same supervisor), so it suits tests and containers whose supervisor is the
// Agent's own process; nodes use SystemdSupervisor.
type ProcessSupervisor struct {
	// Command is the relay binary, after an optional prefix that runs it
	// elsewhere (ip netns exec <ns>).
	Command []string
	// Config is the configuration file (Config.Dir/relay.json) and Socket the
	// control socket (Config.RuntimeDir/control.sock).
	Config string
	Socket string
	// Log receives the relay's standard output and error; nil discards them.
	Log io.Writer

	mu   sync.Mutex
	cmd  *exec.Cmd
	done chan struct{}
}

var _ Supervisor = (*ProcessSupervisor)(nil)

// Check implements Supervisor.
func (p *ProcessSupervisor) Check(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(p.Command) == 0 || p.Config == "" || p.Socket == "" {
		return errors.New("anixops driver: ProcessSupervisor needs Command, Config and Socket")
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

// Status implements Supervisor.
func (p *ProcessSupervisor) Status(ctx context.Context) (Status, error) {
	if err := ctx.Err(); err != nil {
		return Status{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return Status{Running: p.running()}, nil
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
	args := append(append([]string{}, p.Command[1:]...), "-config", p.Config, "-socket", p.Socket)
	cmd := exec.Command(p.Command[0], args...) // #nosec G204 -- the supervisor's configured relay command
	out := p.Log
	if out == nil {
		out = io.Discard
	}
	cmd.Stdout, cmd.Stderr = out, out
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("anixops driver: start the relay: %w", err)
	}
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	p.cmd, p.done = cmd, done
	return nil
}

// Stop implements Supervisor: SIGTERM, then SIGKILL after fifteen seconds.
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
	case <-time.After(15 * time.Second):
		_ = p.cmd.Process.Kill()
		<-p.done
	}
	return nil
}

// Kill kills the relay without the driver knowing, as a crash would (tests).
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
