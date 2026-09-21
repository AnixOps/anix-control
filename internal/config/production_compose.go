package config

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"
)

var pinnedImagePattern = regexp.MustCompile(`^[^[:space:]@]+@sha256:[0-9a-f]{64}$`)

func ValidateProductionComposeEnvironmentFile(path string, cfg *Config) error {
	file, err := os.Open(path) // #nosec G304 -- operator explicitly supplies the local environment file.
	if err != nil {
		return fmt.Errorf("open production Compose environment: %w", err)
	}
	defer func() { _ = file.Close() }()

	values := make(map[string]string)
	scanner := bufio.NewScanner(file)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			return fmt.Errorf("production Compose environment line %d is not KEY=VALUE", lineNumber)
		}
		if _, duplicate := values[key]; duplicate {
			return fmt.Errorf("production Compose environment repeats %s", key)
		}
		values[key] = strings.TrimSpace(value)
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read production Compose environment: %w", err)
	}

	var problems []string
	require := func(ok bool, message string) {
		if !ok {
			problems = append(problems, message)
		}
	}
	for _, key := range []string{"DOCKER_IMAGE", "POSTGRES_IMAGE", "NGINX_IMAGE", "PROMETHEUS_IMAGE", "GRAFANA_IMAGE"} {
		require(pinnedImagePattern.MatchString(values[key]), key+" must be pinned with @sha256:<64 lowercase hex>")
	}
	if cfg == nil {
		require(false, "application configuration is unavailable")
	} else {
		require(values["DB_USER"] == cfg.Database.Username, "DB_USER must exactly match database.username")
		require(values["DB_PASSWORD"] == cfg.Database.Password, "DB_PASSWORD must exactly match database.password")
		require(values["DB_NAME"] == cfg.Database.Database, "DB_NAME must exactly match database.database")
	}
	require(len(values["GRAFANA_PASSWORD"]) >= 16 && nonPlaceholder(values["GRAFANA_PASSWORD"]), "GRAFANA_PASSWORD must be at least 16 characters and not a placeholder")

	if len(problems) > 0 {
		return fmt.Errorf("production Compose environment is not ready: %s", strings.Join(problems, "; "))
	}
	return nil
}
