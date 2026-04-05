package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	forwardRuntimeAnsibleHostAliasEnvVar      = "FORWARD_RUNTIME_ANSIBLE_HOST_ALIAS"
	forwardRuntimeAnsibleHostEnvVar           = "FORWARD_RUNTIME_ANSIBLE_HOST"
	forwardRuntimeAnsiblePortEnvVar           = "FORWARD_RUNTIME_ANSIBLE_PORT"
	forwardRuntimeAnsibleUserEnvVar           = "FORWARD_RUNTIME_ANSIBLE_USER"
	forwardRuntimeAnsiblePasswordEnvVar       = "FORWARD_RUNTIME_ANSIBLE_PASSWORD"
	forwardRuntimeAnsibleBecomePasswordEnvVar = "FORWARD_RUNTIME_ANSIBLE_BECOME_PASSWORD"
	forwardRuntimeAnsibleInventoryGroup       = "forward_nodes"
)

func InitForwardRuntimeAnsibleInventoryFromEnv() (string, error) {
	hostAlias := strings.TrimSpace(os.Getenv(forwardRuntimeAnsibleHostAliasEnvVar))
	host := strings.TrimSpace(os.Getenv(forwardRuntimeAnsibleHostEnvVar))
	port := strings.TrimSpace(os.Getenv(forwardRuntimeAnsiblePortEnvVar))
	user := strings.TrimSpace(os.Getenv(forwardRuntimeAnsibleUserEnvVar))
	password := os.Getenv(forwardRuntimeAnsiblePasswordEnvVar)
	becomePassword := os.Getenv(forwardRuntimeAnsibleBecomePasswordEnvVar)

	hasPasswordAuthConfig := hostAlias != "" ||
		host != "" ||
		port != "" ||
		user != "" ||
		password != "" ||
		becomePassword != ""
	if !hasPasswordAuthConfig {
		return "", nil
	}

	if host == "" {
		return "", fmt.Errorf("%s is required when using env-based ansible password auth", forwardRuntimeAnsibleHostEnvVar)
	}
	if user == "" {
		return "", fmt.Errorf("%s is required when using env-based ansible password auth", forwardRuntimeAnsibleUserEnvVar)
	}
	if password == "" {
		return "", fmt.Errorf("%s is required when using env-based ansible password auth", forwardRuntimeAnsiblePasswordEnvVar)
	}

	if port == "" {
		port = "22"
	}
	if hostAlias == "" {
		hostAlias = host
	}

	inventoryPath := strings.TrimSpace(os.Getenv(forwardRuntimeAnsibleInventoryEnvVar))
	if inventoryPath == "" {
		inventoryPath = defaultForwardRuntimeGeneratedInventoryPath()
	}

	if shouldEnableAnsibleBecome(user) && strings.TrimSpace(os.Getenv(forwardRuntimeAnsibleBecomeEnvVar)) == "" {
		if err := os.Setenv(forwardRuntimeAnsibleBecomeEnvVar, "true"); err != nil {
			return "", err
		}
	}
	if shouldEnableAnsibleBecome(user) && becomePassword == "" {
		becomePassword = password
	}
	if err := os.Setenv(forwardRuntimeAnsibleInventoryEnvVar, inventoryPath); err != nil {
		return "", err
	}

	content := buildForwardRuntimePasswordInventory(hostAlias, host, user, port, password, becomePassword)
	if err := os.MkdirAll(filepath.Dir(inventoryPath), 0o755); err != nil {
		return "", fmt.Errorf("create ansible inventory dir: %w", err)
	}
	if err := os.WriteFile(inventoryPath, []byte(content), 0o600); err != nil {
		return "", fmt.Errorf("write ansible inventory: %w", err)
	}
	return inventoryPath, nil
}

func defaultForwardRuntimeGeneratedInventoryPath() string {
	homeDir, err := os.UserHomeDir()
	if err == nil && strings.TrimSpace(homeDir) != "" {
		return filepath.Join(homeDir, ".config", "v2board", "forward-runtime", "inventory.ini")
	}
	return filepath.Join(os.TempDir(), "v2board-forward-runtime", "inventory.ini")
}

func shouldEnableAnsibleBecome(user string) bool {
	value := strings.TrimSpace(os.Getenv(forwardRuntimeAnsibleBecomeEnvVar))
	if value != "" {
		return strings.EqualFold(value, "true") || value == "1"
	}
	return !strings.EqualFold(strings.TrimSpace(user), "root")
}

func buildForwardRuntimePasswordInventory(hostAlias, host, user, port, password, becomePassword string) string {
	fields := []string{
		hostAlias,
		"ansible_host=" + quoteAnsibleInventoryValue(host),
		"ansible_user=" + quoteAnsibleInventoryValue(user),
		"ansible_port=" + quoteAnsibleInventoryValue(port),
		"ansible_password=" + quoteAnsibleInventoryValue(password),
	}
	if shouldEnableAnsibleBecome(user) {
		fields = append(fields, "ansible_become=true", "ansible_become_method=sudo")
		if becomePassword != "" {
			fields = append(fields, "ansible_become_password="+quoteAnsibleInventoryValue(becomePassword))
		}
	}
	return fmt.Sprintf("[%s]\n%s\n", forwardRuntimeAnsibleInventoryGroup, strings.Join(fields, " "))
}

func quoteAnsibleInventoryValue(value string) string {
	escaped := strings.NewReplacer(
		"\\", "\\\\",
		"\"", "\\\"",
		"\n", "\\n",
		"\r", "\\r",
	).Replace(value)
	return `"` + escaped + `"`
}
