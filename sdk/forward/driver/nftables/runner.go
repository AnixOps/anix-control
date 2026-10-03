package nftables

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Runner runs the host commands the driver needs: nft and tc. The driver
// never runs anything else. ExecRunner runs them on the host; tests inject
// a fake, and the network namespace harness one that runs them inside a
// namespace.
type Runner interface {
	// Run runs name ("nft" or "tc") with args, feeding stdin when it is not
	// nil, and answers its standard output. A command that fails answers a
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

// ExecRunner runs nft and tc from PATH (or the paths it is given).
type ExecRunner struct {
	// NFT and TC are the binaries; empty means "nft" and "tc" from PATH.
	NFT, TC string
}

// Run implements Runner.
func (r ExecRunner) Run(ctx context.Context, name string, args []string, stdin []byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	bin := name
	switch name {
	case "nft":
		if r.NFT != "" {
			bin = r.NFT
		}
	case "tc":
		if r.TC != "" {
			bin = r.TC
		}
	default:
		return nil, fmt.Errorf("nftables driver: refusing to run %q", name)
	}
	cmd := exec.CommandContext(ctx, bin, args...) // #nosec G204 -- nft or tc with arguments the driver built from checked literals
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

// stderrOf answers the standard error of a failed command, or "".
func stderrOf(err error) string {
	var ce *CommandError
	if errors.As(err, &ce) {
		return ce.Stderr
	}
	return ""
}

// notFound reports whether a failed nft command says the table (or another
// object) does not exist.
func notFound(err error) bool {
	return strings.Contains(stderrOf(err), "No such file or directory")
}

// notPermitted reports whether a failed command lacked CAP_NET_ADMIN.
func notPermitted(err error) bool {
	s := stderrOf(err)
	return strings.Contains(s, "Operation not permitted") || strings.Contains(s, "Permission denied")
}

// missingBinary reports whether the command could not be started at all.
func missingBinary(err error) bool {
	return errors.Is(err, exec.ErrNotFound)
}
