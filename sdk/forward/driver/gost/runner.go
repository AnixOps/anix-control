package gost

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Runner runs the host commands the driver needs: gost (Probe's version
// check), ss (the listening sockets, read only) and systemctl (the
// SystemdSupervisor). The driver never runs anything else. ExecRunner runs
// them on the host; tests inject a fake, and the network namespace
// harness one that runs them inside a namespace.
type Runner interface {
	// Run runs name with args, feeding stdin when it is not nil, and
	// answers its standard output. A command that fails answers a
	// *CommandError.
	Run(ctx context.Context, name string, args []string, stdin []byte) ([]byte, error)
}

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

// ExecRunner runs gost, ss and systemctl from PATH, or from the paths it
// is given.
type ExecRunner struct {
	Gost, SS, Systemctl string
}

// Run implements Runner.
func (r ExecRunner) Run(ctx context.Context, name string, args []string, stdin []byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	bin := ""
	switch name {
	case "gost":
		bin = r.Gost
	case "ss":
		bin = r.SS
	case "systemctl":
		bin = r.Systemctl
	default:
		return nil, fmt.Errorf("gost driver: refusing to run %q", name)
	}
	if bin == "" {
		bin = name
	}
	cmd := exec.CommandContext(ctx, bin, args...) // #nosec G204 -- gost, ss or systemctl with arguments the driver built from checked literals
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
func missingBinary(err error) bool {
	return errors.Is(err, exec.ErrNotFound)
}

// firstLine answers the first line of s, clipped.
func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return clip(s)
}

// clip keeps printable ASCII and at most 200 bytes, for Config's text
// fields.
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
