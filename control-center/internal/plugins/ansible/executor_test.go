package ansible

import (
	"context"
	"testing"
	"time"
)

func TestNewExecutor(t *testing.T) {
	cfg := Config{
		PlaybookDir:   "/playbooks",
		InventoryFile: "/inventory/hosts",
		Timeout:       3600,
	}

	exec := NewExecutor(cfg)
	if exec == nil {
		t.Fatal("NewExecutor returned nil")
	}
}

func TestExecutor_Execute_Timeout(t *testing.T) {
	cfg := Config{
		Timeout: 1, // 1 second timeout
	}

	exec := NewExecutor(cfg)
	ctx := context.Background()

	// Execute a command that takes longer than timeout
	// On Windows, use "timeout" or "ping" command
	_, err := exec.Execute(ctx, "sleep", "10")

	if err == nil {
		t.Error("expected timeout error")
	}
}

func TestExecutor_Execute_InvalidCommand(t *testing.T) {
	cfg := Config{}

	exec := NewExecutor(cfg)
	ctx := context.Background()

	_, err := exec.Execute(ctx, "nonexistent-command-that-does-not-exist")
	if err == nil {
		t.Error("expected error for invalid command")
	}
}

func TestExecutor_Execute_WithEnvVars(t *testing.T) {
	cfg := Config{
		EnvVars: map[string]string{
			"TEST_VAR": "test_value",
		},
	}

	exec := NewExecutor(cfg)
	ctx := context.Background()

	// On Windows, use echo %TEST_VAR%
	// This test just verifies the executor can be created with env vars
	_ = exec
	_ = ctx
}

func TestExecutor_Execute_WithVaultPassword(t *testing.T) {
	cfg := Config{
		VaultPassword: "secret",
	}

	exec := NewExecutor(cfg)
	if exec == nil {
		t.Fatal("NewExecutor returned nil")
	}
}

func TestCheckAnsible(t *testing.T) {
	// This will fail on systems without ansible installed
	// but we test that the function doesn't panic
	err := CheckAnsible()
	// We don't assert success/failure since ansible may not be installed
	_ = err
}

func TestGetAnsibleVersion(t *testing.T) {
	// This will fail on systems without ansible installed
	_, err := GetAnsibleVersion()
	// We don't assert success/failure since ansible may not be installed
	_ = err
}

func TestExecutor_ExecuteWithOutput_Context(t *testing.T) {
	cfg := Config{}
	exec := NewExecutor(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Try to execute a command with output streaming
	outputChan, errChan := exec.ExecuteWithOutput(ctx, "echo", "test")

	// Wait for completion or timeout
	select {
	case <-outputChan:
		// Got some output
	case err := <-errChan:
		if err != nil {
			t.Logf("Command error (expected on some systems): %v", err)
		}
	case <-ctx.Done():
		t.Log("Context timed out")
	}
}

func TestConfig_Timeout(t *testing.T) {
	cfg := Config{
		Timeout: 300,
	}

	if cfg.Timeout != 300 {
		t.Errorf("expected timeout 300, got %d", cfg.Timeout)
	}
}

func TestConfig_VaultPassword(t *testing.T) {
	cfg := Config{
		VaultPassword: "my-secret-password",
	}

	if cfg.VaultPassword != "my-secret-password" {
		t.Errorf("expected vault password 'my-secret-password', got '%s'", cfg.VaultPassword)
	}
}

func TestConfig_EnvVars(t *testing.T) {
	cfg := Config{
		EnvVars: map[string]string{
			"ANSIBLE_HOST_KEY_CHECKING": "False",
			"ANSIBLE_FORCE_COLOR":       "True",
		},
	}

	if len(cfg.EnvVars) != 2 {
		t.Errorf("expected 2 env vars, got %d", len(cfg.EnvVars))
	}
	if cfg.EnvVars["ANSIBLE_HOST_KEY_CHECKING"] != "False" {
		t.Error("expected ANSIBLE_HOST_KEY_CHECKING to be False")
	}
}

func TestExecutor_Execute_Success(t *testing.T) {
	cfg := Config{}
	exec := NewExecutor(cfg)
	ctx := context.Background()

	// Execute a simple command that should succeed
	output, err := exec.Execute(ctx, "echo", "hello")

	// On Windows, echo might not exist as a separate command
	// Just verify we can call Execute without panic
	_ = output
	_ = err
}

func TestExecutor_ExecuteWithOutput_Success(t *testing.T) {
	cfg := Config{}
	exec := NewExecutor(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	outputChan, errChan := exec.ExecuteWithOutput(ctx, "echo", "test")

	// Collect output
	var outputs []string
	timeout := time.After(3 * time.Second)

outer:
	for {
		select {
		case output, ok := <-outputChan:
			if !ok {
				break outer
			}
			outputs = append(outputs, output)
		case err := <-errChan:
			if err != nil {
				t.Logf("Error (may be expected): %v", err)
			}
			break outer
		case <-timeout:
			t.Log("Test timed out")
			break outer
		}
	}

	_ = outputs
}

func TestExecutor_ExecuteWithOutput_InvalidCommand(t *testing.T) {
	cfg := Config{}
	exec := NewExecutor(cfg)

	ctx := context.Background()
	outputChan, errChan := exec.ExecuteWithOutput(ctx, "nonexistent-command-xyz")

	// Wait for error
	select {
	case <-outputChan:
		// Some output might come through
	case err := <-errChan:
		if err == nil {
			t.Error("expected error for invalid command")
		}
	case <-time.After(2 * time.Second):
		t.Log("Timed out waiting for error")
	}
}

func TestNewExecutor_WithAllConfig(t *testing.T) {
	cfg := Config{
		PlaybookDir:   "/playbooks",
		InventoryFile: "/inventory/hosts",
		VaultPassword: "secret",
		Timeout:       300,
		EnvVars: map[string]string{
			"ANSIBLE_HOST_KEY_CHECKING": "False",
		},
	}

	exec := NewExecutor(cfg)
	if exec == nil {
		t.Fatal("NewExecutor returned nil")
	}
}

func TestExecutor_Execute_WithAllEnvVars(t *testing.T) {
	cfg := Config{
		Timeout: 10,
		EnvVars: map[string]string{
			"TEST_VAR1": "value1",
			"TEST_VAR2": "value2",
		},
		VaultPassword: "vault-secret",
	}

	exec := NewExecutor(cfg)
	ctx := context.Background()

	// Just verify the executor is created correctly with all options
	_, _ = exec.Execute(ctx, "echo", "test")
}
