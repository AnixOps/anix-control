package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveGeneratedConfigsWritesPrivateFiles(t *testing.T) {
	outputDir := filepath.Join(t.TempDir(), "output")
	if err := saveGeneratedConfigs(outputDir, []byte("{}"), []byte("proxies: []\n")); err != nil {
		t.Fatalf("saveGeneratedConfigs() error = %v", err)
	}

	dirInfo, err := os.Stat(outputDir)
	if err != nil {
		t.Fatalf("stat output dir: %v", err)
	}
	if got, want := dirInfo.Mode().Perm(), os.FileMode(0o750); got != want {
		t.Fatalf("output dir permissions = %v, want %v", got, want)
	}

	for _, name := range []string{"xray-config.json", "mihomo-config.yaml"} {
		path := filepath.Join(outputDir, name)
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", name, err)
		}
		if got, want := info.Mode().Perm(), os.FileMode(0o600); got != want {
			t.Fatalf("%s permissions = %v, want %v", name, got, want)
		}
	}
}

func TestSaveGeneratedConfigsReturnsCreateDirectoryError(t *testing.T) {
	base := t.TempDir()
	outputPath := filepath.Join(base, "output")
	if err := os.WriteFile(outputPath, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := saveGeneratedConfigs(outputPath, []byte("{}"), []byte("proxies: []\n")); err == nil {
		t.Fatal("saveGeneratedConfigs() expected error for file output path")
	}
}
