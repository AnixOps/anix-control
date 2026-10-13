package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	envReferenceBegin = "<!-- BEGIN GENERATED: anix-control -print-env -->\n"
	envReferenceEnd   = "<!-- END GENERATED -->"
)

// The environment variable reference is generated from the configuration
// structure. Regenerate it with:
//
//	UPDATE_ENV_REFERENCE=1 GOWORK=off go test ./internal/config -run TestEnvironmentVariableReferenceIsCurrent
func TestEnvironmentVariableReferenceIsCurrent(t *testing.T) {
	path := filepath.Join("..", "..", "docs", "reference", "environment-variables.md")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	text := string(data)
	start := strings.Index(text, envReferenceBegin)
	end := strings.Index(text, envReferenceEnd)
	require.True(t, start >= 0 && end > start, "generated markers are missing in %s", path)

	want := EnvMarkdownTable(Defaults())
	got := text[start+len(envReferenceBegin) : end]
	if got == want {
		return
	}
	if os.Getenv("UPDATE_ENV_REFERENCE") == "1" {
		updated := text[:start+len(envReferenceBegin)] + want + text[end:]
		require.NoError(t, os.WriteFile(path, []byte(updated), 0o644))
		return
	}
	t.Fatalf("%s is out of date; run UPDATE_ENV_REFERENCE=1 GOWORK=off go test ./internal/config -run TestEnvironmentVariableReferenceIsCurrent", path)
}

const (
	deprecatedReferenceBegin = "<!-- BEGIN GENERATED: deprecated configuration keys -->\n"
	deprecatedReferenceEnd   = "<!-- END GENERATED -->"
)

// The deprecated key table of configuration.md is generated from
// deprecated_keys.yaml. Regenerate it with:
//
//	UPDATE_ENV_REFERENCE=1 GOWORK=off go test ./internal/config -run TestDeprecatedKeyReferenceIsCurrent
func TestDeprecatedKeyReferenceIsCurrent(t *testing.T) {
	path := filepath.Join("..", "..", "docs", "reference", "configuration.md")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	text := string(data)
	start := strings.Index(text, deprecatedReferenceBegin)
	require.True(t, start >= 0, "generated markers are missing in %s", path)
	end := strings.Index(text[start:], deprecatedReferenceEnd)
	require.True(t, end > 0, "generated markers are missing in %s", path)
	end += start

	want := DeprecatedKeysMarkdownTable()
	got := text[start+len(deprecatedReferenceBegin) : end]
	if got == want {
		return
	}
	if os.Getenv("UPDATE_ENV_REFERENCE") == "1" {
		updated := text[:start+len(deprecatedReferenceBegin)] + want + text[end:]
		require.NoError(t, os.WriteFile(path, []byte(updated), 0o644))
		return
	}
	t.Fatalf("%s is out of date; run UPDATE_ENV_REFERENCE=1 GOWORK=off go test ./internal/config -run TestDeprecatedKeyReferenceIsCurrent", path)
}
