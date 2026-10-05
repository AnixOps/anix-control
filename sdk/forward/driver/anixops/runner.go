package anixops

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Runner runs the host commands the driver needs: the relay (Probe's version
// check) and systemctl (the SystemdSupervisor). The driver never runs anything
// else. ExecRunner runs them on the host; tests inject a fake.
type Runner interface {
	// Run runs name with args, feeding stdin when it is not nil, and answers
	// its standard output. A command that fails answers a *CommandError.
	Run(ctx context.Context, name string, args []string, stdin []byte) ([]byte, error)
}

// Command names Runner accepts.
const (
	CmdRelay     = "anixops-relay"
	CmdSystemctl = "systemctl"
)

// CommandError is a command that exited with an error.
type CommandError struct {
	Name   string
	Args   []string
	Stderr string
	Err    error
}

func (e *CommandError) Error() string {
	msg := strings.TrimSpace(e.Stderr)
	if msg == "" {
		msg = e.Err.Error()
	}
	return fmt.Sprintf("%s %s: %s", e.Name, strings.Join(e.Args, " "), msg)
}

func (e *CommandError) Unwrap() error { return e.Err }

// ExecRunner runs the relay and systemctl from the paths it is given, or from
// PATH.
type ExecRunner struct {
	Relay, Systemctl string
}

// Run implements Runner.
func (r ExecRunner) Run(ctx context.Context, name string, args []string, stdin []byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	bin := ""
	switch name {
	case CmdRelay:
		bin = r.Relay
	case CmdSystemctl:
		bin = r.Systemctl
	default:
		return nil, fmt.Errorf("anixops driver: refusing to run %q", name)
	}
	if bin == "" {
		bin = name
	}
	cmd := exec.CommandContext(ctx, bin, args...) // #nosec G204 -- the relay or systemctl with arguments the driver built from checked literals
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return stdout.Bytes(), &CommandError{Name: name, Args: args, Stderr: stderr.String(), Err: err}
	}
	return stdout.Bytes(), nil
}

// missingBinary reports whether the command could not be started at all.
func missingBinary(err error) bool { return errors.Is(err, exec.ErrNotFound) }

// firstLine answers the first line of s, clipped.
func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return clip(s)
}

// clip keeps printable ASCII and at most 200 bytes, for Config's text fields.
func clip(s string) string {
	b := []byte(s)
	out := b[:0]
	for _, c := range b {
		if c >= ' ' && c <= '~' {
			out = append(out, c)
		}
	}
	if len(out) > 200 {
		out = out[:200]
	}
	return string(out)
}
