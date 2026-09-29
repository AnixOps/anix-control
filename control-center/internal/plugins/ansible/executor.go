package ansible

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Executor executes ansible commands
type Executor struct {
	config Config
}

// NewExecutor creates a new executor
func NewExecutor(config Config) *Executor {
	return &Executor{config: config}
}

// Execute runs an ansible command
func (e *Executor) Execute(ctx context.Context, command string, args ...string) ([]byte, error) {
	// Set timeout
	if e.config.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(e.config.Timeout)*time.Second)
		defer cancel()
	}

	// Create command
	cmd := exec.CommandContext(ctx, command, args...)

	// Set environment
	cmd.Env = os.Environ()
	for k, v := range e.config.EnvVars {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	// Set vault password file if configured
	if e.config.VaultPassword != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("ANSIBLE_VAULT_PASSWORD=%s", e.config.VaultPassword))
	}

	// Capture output
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Run
	err := cmd.Run()

	// Combine outputs
	output := stdout.Bytes()
	if len(stderr.Bytes()) > 0 {
		output = append(output, stderr.Bytes()...)
	}

	if err != nil {
		return output, fmt.Errorf("command failed: %w, output: %s", err, string(output))
	}

	return output, nil
}

// ExecuteWithOutput runs a command and streams output
func (e *Executor) ExecuteWithOutput(ctx context.Context, command string, args ...string) (<-chan string, <-chan error) {
	outputChan := make(chan string, 100)
	errChan := make(chan error, 1)

	go func() {
		defer close(outputChan)
		defer close(errChan)

		cmd := exec.CommandContext(ctx, command, args...)

		// Set environment
		cmd.Env = os.Environ()
		for k, v := range e.config.EnvVars {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}

		// Get pipes
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			errChan <- err
			return
		}
		stderr, err := cmd.StderrPipe()
		if err != nil {
			errChan <- err
			return
		}

		// Start command
		if err := cmd.Start(); err != nil {
			errChan <- err
			return
		}

		// Read outputs
		done := make(chan struct{})
		go func() {
			defer close(done)
			buf := make([]byte, 1024)
			for {
				n, err := stdout.Read(buf)
				if n > 0 {
					outputChan <- string(buf[:n])
				}
				if err != nil {
					break
				}
			}
			n, _ := stderr.Read(buf)
			if n > 0 {
				outputChan <- string(buf[:n])
			}
		}()

		// Wait for completion
		err = cmd.Wait()
		<-done

		if err != nil {
			errChan <- err
		}
	}()

	return outputChan, errChan
}

// CheckAnsible checks if ansible is installed
func CheckAnsible() error {
	if _, err := exec.LookPath("ansible"); err != nil {
		return fmt.Errorf("ansible not found: %w", err)
	}
	if _, err := exec.LookPath("ansible-playbook"); err != nil {
		return fmt.Errorf("ansible-playbook not found: %w", err)
	}
	return nil
}

// GetAnsibleVersion returns the ansible version
func GetAnsibleVersion() (string, error) {
	cmd := exec.Command("ansible", "--version")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	// Parse version from first line
	lines := strings.Split(string(output), "\n")
	if len(lines) > 0 {
		return strings.TrimSpace(lines[0]), nil
	}
	return "", fmt.Errorf("could not parse ansible version")
}
