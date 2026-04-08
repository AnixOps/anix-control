package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveConfigPathPrefersExplicitFile(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("env: test\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	resolved, err := resolveConfigPath(configPath)
	if err != nil {
		t.Fatalf("resolveConfigPath() error = %v", err)
	}
	if resolved != configPath {
		t.Fatalf("resolveConfigPath() = %q, want %q", resolved, configPath)
	}
}

func TestResolveRuntimePathPrefersProjectRootWhenConfigIsUnderConfigDir(t *testing.T) {
	tempDir := t.TempDir()
	projectRoot := filepath.Join(tempDir, "repo")
	configDir := filepath.Join(projectRoot, "config")
	targetPath := filepath.Join(projectRoot, "config", "deploy", "ansible", "inventory.ini")
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(targetPath, []byte("[forward_nodes]\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	resolved := resolveRuntimePath(
		filepath.Join("config", "deploy", "ansible", "inventory.ini"),
		filepath.Join(configDir, "config.yaml"),
	)
	if resolved != targetPath {
		t.Fatalf("resolveRuntimePath() = %q, want %q", resolved, targetPath)
	}
}
