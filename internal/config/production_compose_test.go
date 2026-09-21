package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func writeComposeEnvironment(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "production.env")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func completeComposeEnvironment() string {
	digest := strings.Repeat("a", 64)
	return "DOCKER_IMAGE=registry.acme.test/control:v1@sha256:" + digest + "\n" +
		"POSTGRES_IMAGE=postgres:16-alpine@sha256:" + digest + "\n" +
		"NGINX_IMAGE=nginx:alpine@sha256:" + digest + "\n" +
		"PROMETHEUS_IMAGE=prom/prometheus:v3@sha256:" + digest + "\n" +
		"GRAFANA_IMAGE=grafana/grafana:v11@sha256:" + digest + "\n" +
		"DB_USER=anixops\nDB_PASSWORD=" + strings.Repeat("d", 32) + "\nDB_NAME=anixops\n" +
		"GRAFANA_PASSWORD=" + strings.Repeat("g", 32) + "\n"
}

func TestValidateProductionComposeEnvironmentAcceptsPinnedMatchingValues(t *testing.T) {
	cfg := &Config{Database: DatabaseConfig{Username: "anixops", Password: strings.Repeat("d", 32), Database: "anixops"}}
	require.NoError(t, ValidateProductionComposeEnvironmentFile(writeComposeEnvironment(t, completeComposeEnvironment()), cfg))
}

func TestValidateProductionComposeEnvironmentRejectsPlaceholderAndMismatch(t *testing.T) {
	cfg := &Config{Database: DatabaseConfig{Username: "anixops", Password: strings.Repeat("d", 32), Database: "anixops"}}
	content := strings.Replace(completeComposeEnvironment(), strings.Repeat("a", 64), "REPLACE_WITH_DIGEST", 1)
	content = strings.Replace(content, "DB_NAME=anixops", "DB_NAME=wrong", 1)
	err := ValidateProductionComposeEnvironmentFile(writeComposeEnvironment(t, content), cfg)
	require.ErrorContains(t, err, "DOCKER_IMAGE")
	require.ErrorContains(t, err, "DB_NAME")
}
